import Foundation

private struct TestFailure: Error, CustomStringConvertible {
    let description: String
}

private func expect(_ condition: Bool, _ message: String) throws {
    if !condition { throw TestFailure(description: message) }
}

private final class Fixture {
    let directory: URL
    let helper: HelperClient

    init() throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let executable = directory.appendingPathComponent("proxyctl")
        try #"""
        #!/bin/sh
        fixture_dir=${0%/*}
        log="$fixture_dir/commands.log"
        case "$1 $2" in
            'system-proxy on')
                echo 'enable started' >> "$log"
                if [ -f "$fixture_dir/slow-enable" ]; then sleep 0.3; fi
                echo 'enable finished' >> "$log"
                ;;
            'system-proxy off')
                echo 'system-proxy off' >> "$log"
                if [ -f "$fixture_dir/slow-disable" ]; then sleep 0.2; fi
                if [ -f "$fixture_dir/fail-disable" ]; then exit 9; fi
                ;;
            'core stop')
                echo 'core stop' >> "$log"
                if [ -f "$fixture_dir/fail-stop" ]; then exit 8; fi
                ;;
            'core status')
                echo '{"running":false,"pid":0}'
                ;;
            'system-proxy status')
                echo '{"httpEnabled":false,"httpHost":"","httpPort":0,"httpsEnabled":false,"httpsHost":"","httpsPort":0,"socksEnabled":false,"socksHost":"","socksPort":0}'
                ;;
            'self-update --json')
                if [ -f "$fixture_dir/many-update-events" ]; then
                    i=0
                    while [ "$i" -lt 32 ]; do
                        printf '{"stage":"downloading","message":"event-%d"}\n' "$i"
                        i=$((i + 1))
                    done
                fi
                printf '{"stage":"done","message":"fixture update complete"}'
                ;;
            'subscription update')
                if [ -f "$fixture_dir/large-io" ]; then
                    i=0
                    while [ "$i" -lt 20000 ]; do
                        printf 'output-%05d-abcdefghijklmnopqrstuvwxyz0123456789\n' "$i"
                        printf 'diagnostic-%05d-abcdefghijklmnopqrstuvwxyz0123456789\n' "$i" >&2
                        i=$((i + 1))
                    done
                fi
                ;;
            bootstrap*)
                if [ -f "$fixture_dir/short-bootstrap" ]; then
                    echo 'fixture bootstrap complete'
                    exit 0
                fi
                trap 'echo "bootstrap stopped" >> "$log"; exit 0' TERM
                echo 'bootstrap started' >> "$log"
                while :; do sleep 0.02; done
                ;;
            *) echo "$*" >> "$log" ;;
        esac
        """#.write(to: executable, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: executable.path)
        helper = HelperClient(proxyctlPath: executable.path)
    }

    deinit { try? FileManager.default.removeItem(at: directory) }

    var commands: [String] {
        let text = (try? String(contentsOf: directory.appendingPathComponent("commands.log"), encoding: .utf8)) ?? ""
        return text.split(separator: "\n").map(String.init)
    }

    func flag(_ name: String) throws {
        try Data().write(to: directory.appendingPathComponent(name))
    }

    func waitFor(_ command: String) async throws {
        for _ in 0..<500 {
            if commands.contains(command) { return }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        throw TestFailure(description: "Timed out waiting for \(command): \(commands)")
    }
}

@main
struct ShutdownTests {
    static func main() async throws {
        if CommandLine.arguments.contains("--closed-stdin") {
            let reportURL = URL(fileURLWithPath: CommandLine.arguments[2])
            func record(_ message: String) {
                try? message.write(to: reportURL, atomically: true, encoding: .utf8)
            }
            record("closing stdin")
            try FileHandle.standardInput.close()
            let fixture = try Fixture()
            record("enabling fixture proxy")
            try await fixture.helper.enableSystemProxy().get()
            record("reading fixture status")
            _ = try await fixture.helper.getCoreStatus().get()
            record("starting fixture bootstrap")
            try fixture.flag("short-bootstrap")
            let bootstrap = await fixture.helper.bootstrap(subscriptionURL: "https://example.invalid/test")
            for await _ in bootstrap {}
            record("starting fixture update")
            let update = await fixture.helper.installUpdate()
            var updateCompleted = false
            for await event in update {
                try expect(event.stage != "error", "Update must not inherit invalid stdin: \(event.message)")
                updateCompleted = updateCompleted || event.stage == "done"
            }
            try expect(updateCompleted, "Update must complete with closed application stdin")
            record("completed")
            return
        }
        try await cleanupIsOrderedAndShared()
        try await waitsForEnableAndRejectsNewActions()
        try await failureKeepsCoreAndAllowsRetry()
        try await coreStopFailureDoesNotLeaveProxyEnabled()
        try await commandFailureWithoutStderrIsDescriptive()
        try await largeCommandOutputDoesNotDeadlock()
        try await concurrentCommandsKeepDescriptorsValid()
        try commandsDoNotInheritClosedStandardInput()
        try await launchFailureIdentifiesCommand()
        try await streamingUpdateDeliversEachEventOnce()
        try await bootstrapStopsBeforeCleanup()
        try await campusDisconnectIsOrderedAndReusable()
        try await campusDisconnectWaitsAndRejectsNewActions()
        try await campusDisconnectFailureKeepsCore()
        try await campusDisconnectStopsBootstrap()
        try await shutdownDuringCampusDisconnect()
        try await campusCoreStopFailureIsReported()
        print("Passed 17 subprocess, shutdown and campus disconnect regression tests")
    }

    static func cleanupIsOrderedAndShared() async throws {
        let fixture = try Fixture()
        async let first = fixture.helper.shutdown()
        async let second = fixture.helper.shutdown()
        try await first.get()
        try await second.get()
        try await fixture.helper.shutdown().get()
        try expect(fixture.commands == ["system-proxy off", "core stop"], "Cleanup must run once, proxy first")
    }

    static func waitsForEnableAndRejectsNewActions() async throws {
        let fixture = try Fixture()
        try fixture.flag("slow-enable")
        try fixture.flag("slow-disable")
        let enable = Task { await fixture.helper.enableSystemProxy() }
        try await fixture.waitFor("enable started")
        let shutdown = Task { await fixture.helper.shutdown() }
        try await fixture.waitFor("system-proxy off")
        let rejected = await fixture.helper.enableSystemProxy()
        guard case .failure(.shuttingDown) = rejected else {
            throw TestFailure(description: "New proxy changes must be rejected during cleanup")
        }
        try await enable.value.get()
        try await shutdown.value.get()
        try expect(fixture.commands == ["enable started", "enable finished", "system-proxy off", "core stop"],
                   "In-flight enable must finish before cleanup: \(fixture.commands)")
    }

    static func failureKeepsCoreAndAllowsRetry() async throws {
        let fixture = try Fixture()
        try fixture.flag("fail-disable")
        let result = await fixture.helper.shutdown()
        guard case .failure = result else {
            throw TestFailure(description: "A nonzero exit without stderr must fail cleanup")
        }
        try expect(fixture.commands == ["system-proxy off"], "Core must remain available if disabling proxy fails")
        await fixture.helper.resumeAfterCancelledShutdown()
        try FileManager.default.removeItem(at: fixture.directory.appendingPathComponent("fail-disable"))
        try await fixture.helper.shutdown().get()
        try expect(fixture.commands == ["system-proxy off", "system-proxy off", "core stop"], "Retry must clean up")
    }

    static func coreStopFailureDoesNotLeaveProxyEnabled() async throws {
        let fixture = try Fixture()
        try fixture.flag("fail-stop")
        try await fixture.helper.shutdown().get()
        try expect(fixture.commands == ["system-proxy off", "core stop"], "Proxy must be off even when core stop fails")
    }

    static func commandFailureWithoutStderrIsDescriptive() async throws {
        let fixture = try Fixture()
        try fixture.flag("fail-stop")
        let result = await fixture.helper.stopCore()
        guard case .failure(.executionFailed(let message)) = result else {
            throw TestFailure(description: "A nonzero command exit without stderr must report a failure")
        }
        try expect(message.contains("core stop") && message.contains("status 8"),
                   "Command error should identify the command and exit code: \(message)")
    }

    static func largeCommandOutputDoesNotDeadlock() async throws {
        let fixture = try Fixture()
        try fixture.flag("large-io")
        try await fixture.helper.updateSubscription().get()
    }

    static func concurrentCommandsKeepDescriptorsValid() async throws {
        let fixture = try Fixture()
        try await withThrowingTaskGroup(of: Void.self) { group in
            for worker in 0..<8 {
                group.addTask {
                    for _ in 0..<32 {
                        switch worker % 6 {
                        case 0: try await fixture.helper.enableSystemProxy().get()
                        case 1: try await fixture.helper.disableSystemProxy().get()
                        case 2: try await fixture.helper.restartCore().get()
                        case 3: _ = try await fixture.helper.getCoreStatus().get()
                        case 4: _ = try await fixture.helper.getSystemProxyStatus().get()
                        default: try await fixture.helper.updateSubscription().get()
                        }
                    }
                }
            }
            try await group.waitForAll()
        }
    }

    static func commandsDoNotInheritClosedStandardInput() throws {
        let reportURL = FileManager.default.temporaryDirectory.appendingPathComponent("proxycat-stdin-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: reportURL) }
        let process = Process()
        process.executableURL = URL(fileURLWithPath: CommandLine.arguments[0])
        process.arguments = ["--closed-stdin", reportURL.path]
        process.standardInput = FileHandle.nullDevice
        process.standardOutput = FileHandle.nullDevice
        let errorPipe = Pipe()
        process.standardError = errorPipe
        try process.run()
        let errorData = errorPipe.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        let report = (try? String(contentsOf: reportURL, encoding: .utf8)) ?? "no progress"
        try expect(process.terminationStatus == 0,
                   "Commands must work with closed inherited stdin (status \(process.terminationStatus), \(report)): \(String(decoding: errorData, as: UTF8.self))")
    }

    static func launchFailureIdentifiesCommand() async throws {
        let fixture = try Fixture()
        let helper = HelperClient(proxyctlPath: fixture.directory.appendingPathComponent("missing-proxyctl").path)
        guard case .failure(.executionFailed(let message)) = await helper.restartCore() else {
            throw TestFailure(description: "Launching a missing helper must report a command failure")
        }
        try expect(message.contains("proxyctl core restart") && message.contains("NSCocoaErrorDomain"),
                   "Launch errors must identify the command and system error: \(message)")
    }

    static func streamingUpdateDeliversEachEventOnce() async throws {
        let fixture = try Fixture()
        try fixture.flag("many-update-events")
        let expected = (0..<32).map { "event-\($0)" } + ["fixture update complete"]
        for _ in 0..<12 {
            let stream = await fixture.helper.installUpdate()
            var messages: [String] = []
            for await event in stream { messages.append(event.message) }
            try expect(messages == expected,
                       "Concurrent output callbacks and exit must preserve every update event once: \(messages)")
        }
    }

    static func bootstrapStopsBeforeCleanup() async throws {
        let fixture = try Fixture()
        let stream = await fixture.helper.bootstrap(subscriptionURL: "https://example.invalid/test")
        try await fixture.waitFor("bootstrap started")
        try await fixture.helper.shutdown().get()
        for await _ in stream {}
        try expect(fixture.commands == ["bootstrap started", "bootstrap stopped", "system-proxy off", "core stop"],
                   "Bootstrap must stop before clearing settings: \(fixture.commands)")
    }

    static func campusDisconnectIsOrderedAndReusable() async throws {
        let fixture = try Fixture()
        try fixture.flag("slow-disable")
        async let first = fixture.helper.disconnectForCampusNetwork()
        async let second = fixture.helper.disconnectForCampusNetwork()
        try await first.get()
        try await second.get()
        try expect(fixture.commands == ["system-proxy off", "core stop"], "Campus cleanup must be shared and ordered")
        try await fixture.helper.enableSystemProxy().get()
        try await fixture.helper.disconnectForCampusNetwork().get()
        try expect(fixture.commands.suffix(2) == ["system-proxy off", "core stop"], "Campus cleanup must be reusable")
    }

    static func campusDisconnectWaitsAndRejectsNewActions() async throws {
        let fixture = try Fixture()
        try fixture.flag("slow-enable")
        try fixture.flag("slow-disable")
        let enable = Task { await fixture.helper.enableSystemProxy() }
        try await fixture.waitFor("enable started")
        let disconnect = Task { await fixture.helper.disconnectForCampusNetwork() }
        try await fixture.waitFor("system-proxy off")
        guard case .failure(.campusDisconnectInProgress) = await fixture.helper.startCore() else {
            throw TestFailure(description: "Campus cleanup must block new starts")
        }
        try await enable.value.get()
        try await disconnect.value.get()
        try expect(fixture.commands == ["enable started", "enable finished", "system-proxy off", "core stop"],
                   "Campus cleanup must wait for existing enable: \(fixture.commands)")
    }

    static func campusDisconnectFailureKeepsCore() async throws {
        let fixture = try Fixture()
        try fixture.flag("fail-disable")
        guard case .failure = await fixture.helper.disconnectForCampusNetwork() else {
            throw TestFailure(description: "Campus cleanup must report proxy disable failure")
        }
        try expect(fixture.commands == ["system-proxy off"], "Do not stop the core when disabling proxy fails")
        try FileManager.default.removeItem(at: fixture.directory.appendingPathComponent("fail-disable"))
        try await fixture.helper.disconnectForCampusNetwork().get()
    }

    static func campusDisconnectStopsBootstrap() async throws {
        let fixture = try Fixture()
        let stream = await fixture.helper.bootstrap(subscriptionURL: "https://example.invalid/test")
        try await fixture.waitFor("bootstrap started")
        try await fixture.helper.disconnectForCampusNetwork().get()
        for await _ in stream {}
        try expect(fixture.commands == ["bootstrap started", "bootstrap stopped", "system-proxy off", "core stop"],
                   "Campus cleanup must cancel bootstrap before clearing settings")
    }

    static func shutdownDuringCampusDisconnect() async throws {
        let fixture = try Fixture()
        try fixture.flag("slow-disable")
        let disconnect = Task { await fixture.helper.disconnectForCampusNetwork() }
        try await fixture.waitFor("system-proxy off")
        try await fixture.helper.shutdown().get()
        _ = await disconnect.value
        try expect(fixture.commands.suffix(2) == ["system-proxy off", "core stop"],
                   "Shutdown must finish cleanup even when campus disconnect is already running")
        guard case .failure(.shuttingDown) = await fixture.helper.startCore() else {
            throw TestFailure(description: "Campus disconnect must not release the shutdown fence")
        }
    }

    static func campusCoreStopFailureIsReported() async throws {
        let fixture = try Fixture()
        try fixture.flag("fail-stop")
        guard case .failure = await fixture.helper.disconnectForCampusNetwork() else {
            throw TestFailure(description: "Campus protection must report failure to stop the core")
        }
        try expect(fixture.commands == ["system-proxy off", "core stop"], "Clear proxies before attempting core stop")
    }
}
