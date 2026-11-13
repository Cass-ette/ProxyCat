import Foundation

@main
struct InstallerTests {
    @MainActor
    static func main() async throws {
        var passed = 0
        func check(_ value: @autoclosure () throws -> Bool, _ message: String) throws {
            guard try value() else { throw InstallerFailure("Test failed: " + message) }
        }
        func fixture(_ root: URL, _ name: String, version: String) throws -> URL {
            let app = root.appendingPathComponent(name)
            try FileManager.default.createDirectory(at: app.appendingPathComponent("Contents"), withIntermediateDirectories: true)
            let info = ["CFBundleIdentifier": "com.cassette.proxycat", "CFBundleShortVersionString": version, "CFBundleVersion": "10"]
            try PropertyListSerialization.data(fromPropertyList: info, format: .xml, options: 0)
                .write(to: app.appendingPathComponent("Contents/Info.plist"))
            try Data(version.utf8).write(to: app.appendingPathComponent("payload"))
            return app
        }
        func version(_ app: URL) throws -> String { try InstallerBundle.inspect(app).version }
        let files = FileManager.default
        let root = files.temporaryDirectory.appendingPathComponent("proxycat-installer-tests-\(UUID().uuidString)")
        try files.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? files.removeItem(at: root) }

        for scenario in ["fresh", "replace", "move-failure", "verify-failure", "busy", "copy-invalid", "ownership-failure", "symlink", "unrelated"] {
            let directory = root.appendingPathComponent(scenario)
            try files.createDirectory(at: directory, withIntermediateDirectories: true)
            let source = try fixture(directory, "New.app", version: "0.17.0")
            let destination = directory.appendingPathComponent("Applications/ProxyCat.app")
            if scenario != "fresh" {
                try files.createDirectory(at: destination.deletingLastPathComponent(), withIntermediateDirectories: true)
                if scenario == "symlink" {
                    try files.createSymbolicLink(at: destination, withDestinationURL: source)
                } else if scenario == "unrelated" {
                    try files.createDirectory(at: destination, withIntermediateDirectories: true)
                    try Data("unrelated".utf8).write(to: destination.appendingPathComponent("keep"))
                } else { _ = try fixture(destination.deletingLastPathComponent(), "ProxyCat.app", version: "0.16.2") }
            }
            let settings = directory.appendingPathComponent("settings.json")
            try Data("settings must survive".utf8).write(to: settings)
            var transaction = InstallerTransaction(verify: { _ in }, ensureStopped: {})
            if scenario == "move-failure" {
                transaction.move = { from, to in
                    if from.path.contains(".ProxyCat-install-") { throw InstallerFailure("injected rename failure") }
                    try files.moveItem(at: from, to: to)
                }
            }
            if scenario == "verify-failure" { transaction.verify = { if $0 == destination { throw InstallerFailure("injected destination verification failure") } } }
            if scenario == "copy-invalid" { transaction.verify = { if $0 != source { throw InstallerFailure("injected copy verification failure") } } }
            if scenario == "busy" { transaction.ensureStopped = { throw InstallerFailure("still running") } }
            if scenario == "ownership-failure" { transaction.prepareOwnership = { _ in throw InstallerFailure("cannot preserve caller ownership") } }
            var failed = false
            do { try transaction.install(source: source, destination: destination) } catch { failed = true }
            try check(failed == !["fresh", "replace"].contains(scenario), "incorrect outcome for \(scenario)")
            if ["fresh", "replace"].contains(scenario) { try check(try version(destination) == "0.17.0", "new version not installed") }
            else if !["symlink", "unrelated"].contains(scenario) { try check(try version(destination) == "0.16.2", "old version was not retained for \(scenario)") }
            if scenario == "symlink" { try check((try files.destinationOfSymbolicLink(atPath: destination.path)) == source.path, "symlink target changed") }
            if scenario == "unrelated" { try check(files.fileExists(atPath: destination.appendingPathComponent("keep").path), "unrelated folder overwritten") }
            try check(try String(contentsOf: settings, encoding: .utf8) == "settings must survive", "settings changed")
            try check(try version(source) == "0.17.0", "source was consumed")
            passed += 1
        }

        for failure in ["none", "invalid-source", "quit-refused", "quit-timeout", "replace", "launch"] {
            var events: [String] = []
            let flow = InstallerFlow(
                validate: { events.append("validate"); if failure == "invalid-source" { throw InstallerFailure(failure) } },
                quitAndWait: { events.append("quit"); if failure.hasPrefix("quit-") { throw InstallerFailure(failure) } },
                replace: { events.append("replace"); if failure == "replace" { throw InstallerFailure(failure) } },
                launch: { events.append("launch"); if failure == "launch" { throw InstallerFailure(failure) } }
            )
            var failed = false
            do { try await flow.run() } catch { failed = true }
            try check(failed == (failure != "none"), "flow result differs")
            if failure == "invalid-source" { try check(events == ["validate"], "quit invalid package") }
            if failure.hasPrefix("quit-") { try check(events == ["validate", "quit"], "replaced a running app") }
            if failure == "replace" { try check(!events.contains("launch"), "opened incomplete replacement") }
            if failure == "none" { try check(events == ["validate", "quit", "replace", "launch"], "wrong installation order") }
            passed += 1
        }

        let injection = #"安装 ' `echo unsafe` $(echo unsafe) "payload\test"#
        let quoted = InstallerCommand.shellQuote(injection)
        let output = try InstallerCommand.run("/bin/sh", ["-c", "printf '%s' " + quoted])
        try check(output == injection, "shell quoting changed the literal path")
        passed += 1
        print("Passed \(passed) isolated installer transaction and safe-exit tests")
    }
}
