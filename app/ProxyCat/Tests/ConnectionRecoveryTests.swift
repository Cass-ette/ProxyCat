import Foundation

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
    let suite = "proxycat-recovery-tests-\(UUID().uuidString)"
    let helper: HelperClient

    lazy var viewModel = StatusViewModel(
        helper: helper, defaults: defaults,
        wifiSSIDProvider: { [weak self] in
            guard let self else { return nil }
            return self.has("campus-arrived") ? "Campus" : "Home"
        }
    )

    init(coreRunning: Bool = true) throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        defaults = UserDefaults(suiteName: suite)!
        defaults.set(["Campus"], forKey: "protectedWiFiSSIDs")
        defaults.set("disconnect", forKey: "campusNetworkAction")
        let executable = directory.appendingPathComponent("proxyctl")
        try #"""
        #!/bin/sh
        fixture_dir=${0%/*}
        log="$fixture_dir/commands.log"
        echo "$*" >> "$log"
        case "$1 $2" in
            'core status')
                active=false
                if [ -f "$fixture_dir/core-active" ]; then active=true; fi
                printf '{"running":%s,"pid":42}\n' "$active"
                ;;
            'core start')
                if [ -f "$fixture_dir/fail-start" ]; then
                    echo 'saved config failed to start' >&2
                    exit 7
                fi
                touch "$fixture_dir/core-active"
                ;;
            'core stop') rm -f "$fixture_dir/core-active" ;;
            'system-proxy status')
                active=false
                if [ -f "$fixture_dir/proxy-active" ]; then active=true; fi
                printf '{"httpEnabled":false,"httpHost":"","httpPort":0,"httpsEnabled":%s,"httpsHost":"","httpsPort":0,"socksEnabled":false,"socksHost":"","socksPort":0}\n' "$active"
                ;;
            'system-proxy on') touch "$fixture_dir/proxy-active" ;;
            'system-proxy off') rm -f "$fixture_dir/proxy-active" ;;
            'mode status')
                if [ -f "$fixture_dir/hold-mode" ]; then
                    touch "$fixture_dir/mode-held"
                    while [ -f "$fixture_dir/hold-mode" ]; do sleep 0.02; done
                fi
                mode=rule
                if [ -f "$fixture_dir/mode" ]; then mode=$(cat "$fixture_dir/mode"); fi
                printf '{"mode":"%s"}\n' "$mode"
                ;;
            'mode set')
                if [ -f "$fixture_dir/fail-mode" ]; then
                    echo 'global selection unavailable' >&2
                    exit 8
                fi
                printf '%s' "$3" > "$fixture_dir/mode"
                ;;
            'groups list')
                if [ -f "$fixture_dir/enter-campus" ]; then
                    touch "$fixture_dir/campus-arrived"
                    echo '{}'
                    exit 0
                fi
                if [ -f "$fixture_dir/transient-groups" ]; then
                    rm "$fixture_dir/transient-groups"
                    touch "$fixture_dir/empty-once"
                    echo 'controller temporarily unavailable' >&2
                    exit 8
                fi
                if [ -f "$fixture_dir/empty-once" ]; then
                    rm "$fixture_dir/empty-once"
                    echo '{}'
                    exit 0
                fi
                if [ -f "$fixture_dir/fail-groups" ]; then
                    echo 'controller temporarily unavailable' >&2
                    exit 8
                fi
                if [ -f "$fixture_dir/empty-groups" ] || [ ! -f "$fixture_dir/core-active" ]; then
                    echo '{}'
                    exit 0
                fi
                echo 'groups ready' >> "$log"
                if [ -f "$fixture_dir/multi-groups" ]; then
                    echo '{"GLOBAL":{"name":"GLOBAL","type":"Selector","now":"Auto","all":["DIRECT","Auto"]},"Auto":{"name":"Auto","type":"URLTest","now":"Node A","all":["Node A","Node B"]},"Proxy":{"name":"Proxy","type":"Selector","now":"Auto","all":["Auto","DIRECT"]}}'
                    exit 0
                fi
                echo '{"Auto":{"name":"Auto","type":"Selector","now":"Node A","all":["Node A","Node B"]}}'
                ;;
            'profile list')
                if [ -f "$fixture_dir/fail-profiles" ]; then exit 8; fi
                echo '[{"id":"saved","name":"Saved","url":"https://example.invalid/subscription","createdAt":"","updatedAt":"","active":true}]'
                ;;
            'proxy-delay Auto') echo '{"delay":42}' ;;
            'proxy-delay Proxy') echo '{"delay":43}' ;;
            'proxy-delay GLOBAL') echo '{"delay":44}' ;;
            'proxy-delay DIRECT') echo '{"delay":45}' ;;
            *) echo 'unexpected command' >&2; exit 9 ;;
        esac
        """#.write(to: executable, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: executable.path)
        helper = HelperClient(proxyctlPath: executable.path)
        if coreRunning { try set("core-active") }
    }

    deinit {
        defaults.removePersistentDomain(forName: suite)
        try? FileManager.default.removeItem(at: directory)
    }

    func has(_ flag: String) -> Bool {
        FileManager.default.fileExists(atPath: directory.appendingPathComponent(flag).path)
    }

    func set(_ flag: String) throws {
        try Data().write(to: directory.appendingPathComponent(flag))
    }

    func remove(_ flag: String) throws {
        try FileManager.default.removeItem(at: directory.appendingPathComponent(flag))
    }

    var commands: [String] {
        let text = (try? String(contentsOf: directory.appendingPathComponent("commands.log"), encoding: .utf8)) ?? ""
        return text.split(separator: "\n").map(String.init)
    }
}

@main
struct ConnectionRecoveryTests {
    @MainActor
    static func main() async throws {
        try await startsSavedCoreBeforeEnablingProxy()
        try await retriesTransientControllerFailures()
        try await unavailableControllerNeverEnablesProxy()
        try await refreshKeepsNodesUntilCoreStops()
        try await savedProfilesSurviveHelperFailure()
        try await failedCoreStartLeavesProxyOff()
        try await campusArrivalCancelsRecovery()
        try await enablingDuringRefreshPublishesRealProxyStatus()
        try await repeatedEnableDoesNotStartTwice()
        try await selectionAndDelayFollowMode()
        try await failedModeChangeKeepsSelection()
        print("Passed 11 connection recovery and mode regression tests")
    }

    @MainActor
    static func startsSavedCoreBeforeEnablingProxy() async throws {
        let fixture = try Fixture(coreRunning: false)
        await fixture.viewModel.enableSystemProxy()
        let commands = fixture.commands
        let start = commands.firstIndex(of: "core start")
        let ready = commands.firstIndex(of: "groups ready")
        let enable = commands.firstIndex(of: "system-proxy on")
        try expect(start != nil && ready != nil && enable != nil, "Start saved core, read groups, enable proxy")
        try expect(start! < ready! && ready! < enable!, "Never enable proxy before the saved core is ready")
        try expect(fixture.viewModel.systemProxyEnabled && fixture.viewModel.toggleTestResult?.delay == 42,
                   "Verify real proxy status and test the selected group")
        try expect(!commands.contains { $0.hasPrefix("bootstrap") || $0.hasPrefix("subscription") },
                   "Recovery must not download or reimport the subscription")
    }

    @MainActor
    static func retriesTransientControllerFailures() async throws {
        let fixture = try Fixture()
        try fixture.set("transient-groups")
        await fixture.viewModel.enableSystemProxy()
        let commands = fixture.commands
        let enable = commands.firstIndex(of: "system-proxy on")!
        try expect(commands[..<enable].filter { $0 == "groups list --json" }.count == 3,
                   "Retry both failed and empty controller responses before enabling")
        try expect(fixture.viewModel.lastError == nil && fixture.viewModel.proxyGroupsError == nil,
                   "Clear temporary errors after recovery")
        try expect(!commands.contains("core start"), "Do not restart a healthy core")
    }

    @MainActor
    static func unavailableControllerNeverEnablesProxy() async throws {
        for flag in ["fail-groups", "empty-groups"] {
            let fixture = try Fixture()
            await fixture.viewModel.refreshStatus()
            try fixture.set(flag)
            await fixture.viewModel.enableSystemProxy()
            try expect(!fixture.has("proxy-active") && !fixture.commands.contains("system-proxy on"),
                       "Cached nodes must never establish readiness")
            try expect(fixture.viewModel.lastError?.contains("无需重新粘贴订阅") == true,
                       "Explain temporary readiness failures without requesting a new subscription")
            try expect(!fixture.viewModel.isToggleTesting, "Allow a later retry")
            try fixture.remove(flag)
            await fixture.viewModel.enableSystemProxy()
            try expect(fixture.viewModel.systemProxyEnabled && fixture.viewModel.lastError == nil,
                       "Recover on retry with the existing configuration")
        }
    }

    @MainActor
    static func refreshKeepsNodesUntilCoreStops() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        for flag in ["fail-groups", "empty-groups"] {
            try fixture.set(flag)
            await fixture.viewModel.refreshStatus()
            try expect(fixture.viewModel.proxyGroups.count == 1 && fixture.viewModel.currentNode == "Node A",
                       "Keep the node list and selection during transient controller failures")
            try expect(fixture.viewModel.proxyGroupsError != nil, "Mark cached nodes as temporarily unavailable")
            try fixture.remove(flag)
            await fixture.viewModel.refreshStatus()
            try expect(fixture.viewModel.proxyGroupsError == nil, "Clear stale-data warning after recovery")
        }
        try fixture.remove("core-active")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.viewModel.proxyGroups.isEmpty && fixture.viewModel.currentGroup.isEmpty,
                   "Clear selections after the core actually stops")
    }

    @MainActor
    static func savedProfilesSurviveHelperFailure() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.loadProfiles()
        try fixture.set("fail-profiles")
        await fixture.viewModel.loadProfiles()
        try expect(fixture.viewModel.profiles.map(\.id) == ["saved"] && fixture.viewModel.activeProfileID == "saved",
                   "Keep saved profiles and active selection when listing temporarily fails")
    }

    @MainActor
    static func failedCoreStartLeavesProxyOff() async throws {
        let fixture = try Fixture(coreRunning: false)
        try fixture.set("fail-start")
        await fixture.viewModel.enableSystemProxy()
        try expect(!fixture.commands.contains("system-proxy on") && !fixture.viewModel.coreRunning,
                   "A core start failure must not enable a dead proxy")
        try expect(fixture.viewModel.lastError?.contains("无法启动已保存的配置") == true,
                   "Report the actual saved-config startup error")
    }

    @MainActor
    static func campusArrivalCancelsRecovery() async throws {
        let fixture = try Fixture(coreRunning: false)
        try fixture.set("enter-campus")
        await fixture.viewModel.enableSystemProxy()
        try expect(!fixture.commands.contains("system-proxy on") && !fixture.has("core-active"),
                   "Arrival on protected campus Wi-Fi must cancel readiness and stop the newly started core")
        try expect(fixture.commands.suffix(2) == ["system-proxy off", "core stop"],
                   "Campus cancellation must clear proxy settings before stopping the core")
    }

    @MainActor
    static func enablingDuringRefreshPublishesRealProxyStatus() async throws {
        let fixture = try Fixture()
        try fixture.set("hold-mode")
        defer { try? fixture.remove("hold-mode") }
        let refresh = Task { await fixture.viewModel.refreshStatus() }
        for _ in 0..<200 where !fixture.has("mode-held") {
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        try expect(fixture.has("mode-held"), "Periodic refresh must be in flight for this regression")
        await fixture.viewModel.enableSystemProxy()
        try expect(fixture.viewModel.isRefreshing && fixture.viewModel.systemProxyEnabled,
                   "Publish proxy status even when the periodic refresh skips overlapping work")
        try expect(fixture.viewModel.toggleTestResult?.delay == 42, "Still test fresh groups during a refresh")
        try fixture.remove("hold-mode")
        await refresh.value
    }

    @MainActor
    static func repeatedEnableDoesNotStartTwice() async throws {
        let fixture = try Fixture(coreRunning: false)
        try fixture.set("transient-groups")
        let first = Task { await fixture.viewModel.enableSystemProxy() }
        for _ in 0..<200 where !fixture.viewModel.isToggleTesting {
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        await fixture.viewModel.enableSystemProxy()
        await first.value
        try expect(fixture.commands.filter { $0 == "core start" }.count == 1 &&
                   fixture.commands.filter { $0 == "system-proxy on" }.count == 1,
                   "Repeated clicks during recovery must not issue duplicate lifecycle commands")
    }

    @MainActor
    static func selectionAndDelayFollowMode() async throws {
        let fixture = try Fixture()
        try fixture.set("multi-groups")
        await fixture.viewModel.refreshStatus()
        try expect(fixture.viewModel.currentGroup == "Proxy" && fixture.viewModel.currentNode == "Auto",
                   "Rule mode must prefer its selector instead of the alphabetically first or GLOBAL group")
        for (mode, group, node, delay) in [("global", "GLOBAL", "Auto", 44), ("direct", "DIRECT", "DIRECT", 45), ("rule", "Proxy", "Auto", 43)] {
            fixture.viewModel.testResult = TestResult(googleOK: true, githubOK: true, error: nil)
            await fixture.viewModel.setMode(mode)
            try expect(fixture.viewModel.currentMode == mode && fixture.viewModel.currentGroup == group && fixture.viewModel.currentNode == node,
                       "Displayed selection must follow mode \(mode)")
            try expect(fixture.viewModel.toggleTestResult == nil && fixture.viewModel.testResult == nil,
                       "Changing modes must clear results from the previous route")
            await fixture.viewModel.enableSystemProxy()
            try expect(fixture.commands.contains("proxy-delay \(group) --json") && fixture.viewModel.toggleTestResult?.delay == delay,
                       "Enable test must use the actual \(mode) route")
        }
    }

    @MainActor
    static func failedModeChangeKeepsSelection() async throws {
        let fixture = try Fixture()
        await fixture.viewModel.refreshStatus()
        try fixture.set("fail-mode")
        await fixture.viewModel.setMode("global")
        try expect(fixture.viewModel.currentMode == "rule" && fixture.viewModel.currentGroup == "Auto" && fixture.viewModel.currentNode == "Node A",
                   "A failed global preparation must leave the displayed rule route intact")
        try expect(fixture.viewModel.lastError?.contains("global selection unavailable") == true && !fixture.viewModel.isChangingMode,
                   "Report failure and allow another mode change")
    }
}
