import Foundation

// The standalone test runner does not include the app's @main entry point.
@MainActor
enum AppDelegate {
    static var relaunchOnExit = false
}

private struct TestFailure: Error, CustomStringConvertible {
    let description: String
}

private func expect(_ condition: Bool, _ message: String) throws {
    if !condition { throw TestFailure(description: message) }
}

@MainActor
private final class Fixture {
    let directory: URL
    let defaults: UserDefaults
    let suite = "proxycat-campus-tests-\(UUID().uuidString)"
    let helper: HelperClient
    var ssid: String? = "Campus"
    var prompts = 0
    var acceptPrompt = false

    lazy var viewModel = StatusViewModel(
        helper: helper, defaults: defaults,
        wifiSSIDProvider: { [weak self] in self?.ssid },
        campusPrompt: { [weak self] _ in
            guard let self = self else { return false }
            self.prompts += 1
            return self.acceptPrompt
        }
    )

    init(action: CampusNetworkAction = .disconnect, active: Bool = true, enabled: Bool = true) throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        defaults = UserDefaults(suiteName: suite)!
        defaults.set(["Campus"], forKey: "protectedWiFiSSIDs")
        defaults.set(action.rawValue, forKey: "campusNetworkAction")
        defaults.set(enabled, forKey: "campusProtectionEnabled")
        let executable = directory.appendingPathComponent("proxyctl")
        try #"""
        #!/bin/sh
        fixture_dir=${0%/*}
        log="$fixture_dir/commands.log"
        case "$1 $2" in
            'core status')
                active=false
                if [ -f "$fixture_dir/core-active" ]; then active=true; fi
                printf '{"running":%s,"pid":42}\n' "$active"
                ;;
            'system-proxy status')
                active=false
                if [ -f "$fixture_dir/proxy-active" ]; then active=true; fi
                printf '{"httpEnabled":false,"httpHost":"","httpPort":0,"httpsEnabled":%s,"httpsHost":"","httpsPort":0,"socksEnabled":false,"socksHost":"","socksPort":0}\n' "$active"
                ;;
            'system-proxy off')
                echo 'system-proxy off' >> "$log"
                if [ -f "$fixture_dir/fail-disable" ]; then exit 9; fi
                rm -f "$fixture_dir/proxy-active"
                ;;
            'core stop')
                echo 'core stop' >> "$log"
                rm -f "$fixture_dir/core-active"
                ;;
            'mode status') echo '{"mode":"rule"}' ;;
            'groups list') echo '{}' ;;
            'profile list') echo '[]' ;;
            *) echo "$*" >> "$log" ;;
        esac
        """#.write(to: executable, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: executable.path)
        helper = HelperClient(proxyctlPath: executable.path)
        if active {
            try Data().write(to: directory.appendingPathComponent("core-active"))
            try Data().write(to: directory.appendingPathComponent("proxy-active"))
        }
    }

    deinit {
        defaults.removePersistentDomain(forName: suite)
        try? FileManager.default.removeItem(at: directory)
    }

    var commands: [String] {
        let text = (try? String(contentsOf: directory.appendingPathComponent("commands.log"), encoding: .utf8)) ?? ""
        return text.split(separator: "\n").map(String.init)
    }
}

@main
struct CampusNetworkTests {
    @MainActor
    static func main() async throws {
        try await automaticallyDisconnectsOnce()
        try await skipsUnmatchedDisabledAndUnreadableNetworks()
        try await promptIsNotRepeatedAndRearmsAfterLeaving()
        try await promptCanDisconnect()
        try await blocksAllStartActionsOnCampus()
        try await leavingCampusDoesNotReconnect()
        try await settingsPersistAndKeepExactSSIDs()
        try await failureKeepsCoreAndCanRetry()
        try await httpsOnlyProxyIsProtected()
        try await inactiveCampusDoesNotPrompt()
        try await reenablingProxyPromptsAgain()
        print("Passed 11 campus network policy regression tests")
    }

    @MainActor
    static func automaticallyDisconnectsOnce() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        await fixture.viewModel.refreshStatus()
        try expect(fixture.commands == ["system-proxy off", "core stop"], "Disconnect once, proxy first")
        try expect(!fixture.viewModel.coreRunning && !fixture.viewModel.systemProxyEnabled, "Publish disconnected status")
        try expect(fixture.prompts == 0, "Automatic mode must not prompt")
    }

    @MainActor
    static func skipsUnmatchedDisabledAndUnreadableNetworks() async throws {
        for ssid: String? in ["Home", "campus", nil] {
            let fixture = try Fixture()
            fixture.ssid = ssid
            await fixture.viewModel.refreshStatus()
            try expect(fixture.commands.isEmpty, "Only match an exact configured SSID")
        }
        let fixture = try Fixture(enabled: false)
        await fixture.viewModel.refreshStatus()
        try expect(fixture.commands.isEmpty, "Disabled protection must not change proxy settings")
    }

    @MainActor
    static func promptIsNotRepeatedAndRearmsAfterLeaving() async throws {
        let fixture = try Fixture(action: .prompt)
        await fixture.viewModel.refreshStatus()
        await fixture.viewModel.refreshStatus()
        try expect(fixture.prompts == 1 && fixture.commands.isEmpty, "Dismissal must not repeat every refresh")
        fixture.ssid = "Home"
        await fixture.viewModel.refreshStatus()
        fixture.ssid = "Campus"
        await fixture.viewModel.refreshStatus()
        try expect(fixture.prompts == 2, "Returning to campus must prompt again")
    }

    @MainActor
    static func promptCanDisconnect() async throws {
        let fixture = try Fixture(action: .prompt)
        fixture.acceptPrompt = true
        await fixture.viewModel.refreshStatus()
        await fixture.viewModel.refreshStatus()
        try expect(fixture.prompts == 1, "Accepted prompt must not repeat after disconnecting")
        try expect(fixture.commands == ["system-proxy off", "core stop"], "Prompt action must stop proxy and core")
    }

    @MainActor
    static func blocksAllStartActionsOnCampus() async throws {
        let fixture = try Fixture(active: false)
        await fixture.viewModel.enableSystemProxy()
        await fixture.viewModel.startCore()
        await fixture.viewModel.restartCore()
        await fixture.viewModel.activateProfile(id: "test")
        await fixture.viewModel.bootstrap(subscriptionURL: "https://example.invalid/subscription")
        try expect(fixture.commands.isEmpty, "Auto protection must block every proxy start path")
        try expect(fixture.viewModel.campusProtectionNotice != nil, "Blocked actions must explain why")
    }

    @MainActor
    static func leavingCampusDoesNotReconnect() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        fixture.ssid = "Home"
        await fixture.viewModel.refreshStatus()
        try expect(fixture.commands == ["system-proxy off", "core stop"], "Leaving campus must not reconnect")
    }

    @MainActor
    static func settingsPersistAndKeepExactSSIDs() async throws {
        let fixture = try Fixture(active: false)
        fixture.viewModel.addCampusNetwork(" Campus ")
        fixture.viewModel.addCampusNetwork(" Campus ")
        fixture.viewModel.addCampusNetwork("")
        fixture.viewModel.campusNetworkAction = .prompt
        fixture.viewModel.campusProtectionEnabled = false
        await fixture.viewModel.refreshStatus()
        try expect(fixture.defaults.stringArray(forKey: "protectedWiFiSSIDs") == ["Campus", " Campus "],
                   "Preserve exact SSIDs and reject empty or duplicate entries")
        let restored = StatusViewModel(helper: fixture.helper, defaults: fixture.defaults, wifiSSIDProvider: { nil })
        try expect(restored.campusNetworkAction == .prompt && !restored.campusProtectionEnabled, "Restore user policy")
        fixture.viewModel.removeCampusNetwork(" Campus ")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.defaults.stringArray(forKey: "protectedWiFiSSIDs") == ["Campus"], "Persist removal")
    }

    @MainActor
    static func failureKeepsCoreAndCanRetry() async throws {
        let fixture = try Fixture()
        let flag = fixture.directory.appendingPathComponent("fail-disable")
        try Data().write(to: flag)
        await fixture.viewModel.refreshStatus()
        try expect(fixture.commands == ["system-proxy off"], "Do not stop the core after a proxy disable failure")
        try expect(fixture.viewModel.lastError != nil, "Explain disconnect errors")
        try FileManager.default.removeItem(at: flag)
        await fixture.viewModel.refreshStatus()
        try expect(fixture.commands == ["system-proxy off", "system-proxy off", "core stop"], "Retry on next refresh")
    }

    @MainActor
    static func httpsOnlyProxyIsProtected() async throws {
        let fixture = try Fixture()
        try FileManager.default.removeItem(at: fixture.directory.appendingPathComponent("core-active"))
        await fixture.viewModel.refreshStatus()
        try expect(fixture.commands == ["system-proxy off", "core stop"], "HTTPS-only proxy must trigger protection")
    }

    @MainActor
    static func inactiveCampusDoesNotPrompt() async throws {
        let fixture = try Fixture(action: .prompt, active: false)
        await fixture.viewModel.refreshStatus()
        try expect(fixture.prompts == 0 && fixture.commands.isEmpty, "Do not disturb users when proxy is inactive")
    }

    @MainActor
    static func reenablingProxyPromptsAgain() async throws {
        let fixture = try Fixture(action: .prompt)
        await fixture.viewModel.refreshStatus()
        try FileManager.default.removeItem(at: fixture.directory.appendingPathComponent("core-active"))
        try FileManager.default.removeItem(at: fixture.directory.appendingPathComponent("proxy-active"))
        await fixture.viewModel.refreshStatus()
        try Data().write(to: fixture.directory.appendingPathComponent("proxy-active"))
        await fixture.viewModel.refreshStatus()
        try expect(fixture.prompts == 2, "Reenabling a disconnected proxy on campus must warn again")
    }
}
