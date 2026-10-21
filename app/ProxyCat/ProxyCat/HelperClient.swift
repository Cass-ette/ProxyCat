import Foundation
import os.log

enum HelperError: Error, LocalizedError {
    case shuttingDown
    case campusDisconnectInProgress
    case executableNotFound
    case executionFailed(String)
    case invalidOutput
    case decodingFailed(Error)

    var errorDescription: String? {
        switch self {
        case .shuttingDown:
            return "ProxyCat is shutting down"
        case .campusDisconnectInProgress:
            return "校园网保护正在关闭 ProxyCat 代理"
        case .executableNotFound:
            return "proxyctl not found in bundle or PATH"
        case .executionFailed(let message):
            return "Command failed: \(message)"
        case .invalidOutput:
            return "Invalid command output"
        case .decodingFailed(let error):
            return "Failed to decode response: \(error.localizedDescription)"
        }
    }
}

private final class UpdateOutputReader: @unchecked Sendable {
    private let lock = NSLock()
    private let decoder = JSONDecoder()
    private let continuation: AsyncStream<UpdateEvent>.Continuation
    private var buffer = Data()
    private var finished = false

    init(continuation: AsyncStream<UpdateEvent>.Continuation) {
        self.continuation = continuation
    }

    func read(from handle: FileHandle, finishing: Bool = false) {
        // A final drain can overlap an already queued readability callback.
        lock.lock()
        defer { lock.unlock() }
        guard !finished else { return }
        buffer.append(finishing ? handle.readDataToEndOfFile() : handle.availableData)
        while let newline = buffer.firstIndex(of: 10) {
            let line = Data(buffer[..<newline])
            buffer.removeSubrange(...newline)
            emit(line)
        }
        if finishing {
            if !buffer.isEmpty { emit(buffer) }
            buffer.removeAll()
            finished = true
        }
    }

    private func emit(_ data: Data) {
        if let event = try? decoder.decode(UpdateEvent.self, from: data) {
            continuation.yield(event)
        } else {
            os_log("self-update: skipping malformed NDJSON line: %{public}@", String(decoding: data, as: UTF8.self))
        }
    }
}

actor HelperClient {
    static let shared = HelperClient()

    private var proxyctlPath: String?
    private var updateProcess: Process?
    private var bootstrapProcess: Process?
    private var lifecycleCommands: [UUID: Task<Result<Data, HelperError>, Never>] = [:]
    private var shutdownTask: Task<Result<Void, HelperError>, Never>?
    private var campusDisconnectTask: Task<Result<Void, HelperError>, Never>?

    init(proxyctlPath: String? = nil) {
        self.proxyctlPath = proxyctlPath
    }

    private func getProxyctlPath() -> String? {
        if let path = proxyctlPath { return path }

        // Check bundled in Resources
        if let bundled = Bundle.main.path(forResource: "proxyctl", ofType: nil) {
            proxyctlPath = bundled
            return bundled
        }

        // Check PATH
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/usr/bin/which")
        task.arguments = ["proxyctl"]
        guard let inputHandle = try? FileHandle(forReadingFrom: URL(fileURLWithPath: "/dev/null")) else {
            return nil
        }
        task.standardInput = inputHandle
        task.standardError = FileHandle.nullDevice
        defer { try? inputHandle.close() }
        let pipe = Pipe()
        task.standardOutput = pipe
        try? task.run()
        task.waitUntilExit()

        let data = pipe.fileHandleForReading.readDataToEndOfFile()
        if let path = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines),
           !path.isEmpty {
            proxyctlPath = path
            return path
        }

        return nil
    }

    private func runCommand(_ args: [String], duringShutdown: Bool = false, duringCampusDisconnect: Bool = false) async -> Result<Data, HelperError> {
        guard shutdownTask == nil || duringShutdown else {
            return .failure(.shuttingDown)
        }
        guard campusDisconnectTask == nil || duringCampusDisconnect || duringShutdown else {
            return .failure(.campusDisconnectInProgress)
        }
        guard let executable = getProxyctlPath() else {
            return .failure(.executableNotFound)
        }

        let process = Process()
        process.executableURL = URL(fileURLWithPath: executable)
        process.arguments = args

        let outputDirectory = FileManager.default.temporaryDirectory
            .appendingPathComponent("proxycat-command-\(UUID().uuidString)", isDirectory: true)
        let outputURL = outputDirectory.appendingPathComponent("stdout")
        let errorURL = outputDirectory.appendingPathComponent("stderr")
        let inputHandle: FileHandle
        let outputHandle: FileHandle
        let errorHandle: FileHandle
        do {
            // Open stdin before stdout so a closed descriptor 0 cannot be
            // reused for output and then overwritten during child launch.
            inputHandle = try FileHandle(forReadingFrom: URL(fileURLWithPath: "/dev/null"))
            process.standardInput = inputHandle
            try FileManager.default.createDirectory(at: outputDirectory, withIntermediateDirectories: true)
            guard FileManager.default.createFile(atPath: outputURL.path, contents: nil),
                  FileManager.default.createFile(atPath: errorURL.path, contents: nil) else {
                try? FileManager.default.removeItem(at: outputDirectory)
                return .failure(.executionFailed("Could not create command output files"))
            }
            outputHandle = try FileHandle(forWritingTo: outputURL)
            errorHandle = try FileHandle(forWritingTo: errorURL)
            process.standardOutput = outputHandle
            process.standardError = errorHandle
        } catch {
            try? FileManager.default.removeItem(at: outputDirectory)
            return .failure(.executionFailed(error.localizedDescription))
        }

        let command = Task { () -> Result<Data, HelperError> in
            await withCheckedContinuation { continuation in
                DispatchQueue.global(qos: .utility).async {
                    defer {
                        try? inputHandle.close()
                        try? outputHandle.close()
                        try? errorHandle.close()
                        try? FileManager.default.removeItem(at: outputDirectory)
                    }
                    do {
                        // Keep launch and wait on the same worker so Foundation's
                        // process termination monitor stays attached to its run loop.
                        try process.run()
                        process.waitUntilExit()

                        let data = try Data(contentsOf: outputURL)
                        let errorData = try Data(contentsOf: errorURL)
                        let stderr = String(decoding: errorData, as: UTF8.self)
                            .trimmingCharacters(in: .whitespacesAndNewlines)
                        let result: Result<Data, HelperError>
                        if process.terminationStatus == 0 {
                            result = .success(data)
                        } else {
                            let commandName = args.joined(separator: " ")
                            let message = stderr.isEmpty
                                ? "proxyctl \(commandName) exited with status \(process.terminationStatus)"
                                : stderr
                            result = .failure(.executionFailed(message))
                        }
                        continuation.resume(returning: result)
                    } catch {
                        let nsError = error as NSError
                        let commandName = args.joined(separator: " ")
                        let message = "proxyctl \(commandName): \(error.localizedDescription) (\(nsError.domain) \(nsError.code))"
                        os_log("command: %{public}@", message)
                        continuation.resume(returning: .failure(.executionFailed(message)))
                    }
                }
            }
        }

        let id = UUID()
        if ["core", "system-proxy"].contains(args.first ?? ""), args.dropFirst().first != "status" {
            lifecycleCommands[id] = command
        }
        defer { lifecycleCommands[id] = nil }
        return await command.value
    }

    func shutdown() async -> Result<Void, HelperError> {
        if let shutdownTask = shutdownTask {
            return await shutdownTask.value
        }

        let pendingCommands = Array(lifecycleCommands.values)
        let bootstrap = bootstrapProcess
        let campusDisconnect = campusDisconnectTask
        let task = Task { () -> Result<Void, HelperError> in
            if let campusDisconnect = campusDisconnect {
                _ = await campusDisconnect.value
            }
            // Bootstrap can enable proxies itself. Stop it before clearing settings.
            if let bootstrap = bootstrap, bootstrap.isRunning {
                bootstrap.terminate()
                await Task.detached { bootstrap.waitUntilExit() }.value
            }
            for command in pendingCommands {
                _ = await command.value
            }

            let disabled = await runCommand(["system-proxy", "off"], duringShutdown: true)
            if case .failure(let error) = disabled {
                // Keep the core available if macOS refused to clear its proxy settings.
                return .failure(error)
            }

            let stopped = await runCommand(["core", "stop"], duringShutdown: true)
            if case .failure(let error) = stopped {
                os_log("shutdown: could not stop core: %{public}@", error.localizedDescription)
            }
            return .success(())
        }
        // This also blocks UI actions and stream tasks that have not launched yet.
        shutdownTask = task
        return await task.value
    }

    func resumeAfterCancelledShutdown() {
        shutdownTask = nil
    }

    func disconnectForCampusNetwork() async -> Result<Void, HelperError> {
        guard shutdownTask == nil else { return .failure(.shuttingDown) }
        if let task = campusDisconnectTask { return await task.value }

        let pendingCommands = Array(lifecycleCommands.values)
        let bootstrap = bootstrapProcess
        let task = Task { () -> Result<Void, HelperError> in
            // Bootstrap may enable the proxy itself, so cancel it before cleanup.
            if let bootstrap = bootstrap, bootstrap.isRunning {
                bootstrap.terminate()
                await Task.detached { bootstrap.waitUntilExit() }.value
            }
            for command in pendingCommands {
                _ = await command.value
            }
            let disabled = await runCommand(["system-proxy", "off"], duringCampusDisconnect: true)
            if case .failure(let error) = disabled { return .failure(error) }
            let stopped = await runCommand(["core", "stop"], duringCampusDisconnect: true)
            return stopped.map { _ in () }
        }
        campusDisconnectTask = task
        defer { campusDisconnectTask = nil }
        return await task.value
    }

    func getCoreStatus() async -> Result<CoreStatus, HelperError> {
        let result = await runCommand(["core", "status", "--json"])
        return result.flatMap { data in
            do {
                let status = try JSONDecoder().decode(CoreStatus.self, from: data)
                return .success(status)
            } catch {
                return .failure(.decodingFailed(error))
            }
        }
    }

    func getSystemProxyStatus() async -> Result<SystemProxyStatus, HelperError> {
        let result = await runCommand(["system-proxy", "status", "--json"])
        return result.flatMap { data in
            do {
                let status = try JSONDecoder().decode(SystemProxyStatus.self, from: data)
                return .success(status)
            } catch {
                return .failure(.decodingFailed(error))
            }
        }
    }

    func enableSystemProxy() async -> Result<Void, HelperError> {
        let result = await runCommand(["system-proxy", "on"])
        return result.map { _ in () }
    }

    func disableSystemProxy() async -> Result<Void, HelperError> {
        let result = await runCommand(["system-proxy", "off"])
        return result.map { _ in () }
    }

    func startCore() async -> Result<Void, HelperError> {
        let result = await runCommand(["core", "start"])
        return result.map { _ in () }
    }

    func stopCore() async -> Result<Void, HelperError> {
        let result = await runCommand(["core", "stop"])
        return result.map { _ in () }
    }

    func restartCore() async -> Result<Void, HelperError> {
        let result = await runCommand(["core", "restart"])
        return result.map { _ in () }
    }

    func testConnection() async -> Result<TestResult, HelperError> {
        let result = await runCommand(["test", "--json"])
        return result.flatMap { data in
            do {
                let testResult = try JSONDecoder().decode(TestResult.self, from: data)
                return .success(testResult)
            } catch {
                return .failure(.decodingFailed(error))
            }
        }
    }

    func updateSubscription() async -> Result<Void, HelperError> {
        let result = await runCommand(["subscription", "update"])
        return result.map { _ in () }
    }

    func checkForUpdate() async -> Result<UpdateEvent, HelperError> {
        let result = await runCommand(["self-update", "--check-only", "--json"])
        return result.flatMap { data in
            decodeLastUpdateEvent(from: data)
        }
    }

    private func decodeLastUpdateEvent(from data: Data) -> Result<UpdateEvent, HelperError> {
        let decoder = JSONDecoder()
        var events: [UpdateEvent] = []
        let lines = String(data: data, encoding: .utf8)?
            .split(whereSeparator: { $0.isNewline }) ?? []
        for line in lines {
            if let event = try? decoder.decode(UpdateEvent.self, from: Data(line.utf8)) {
                events.append(event)
            } else {
                os_log("self-update: skipping malformed NDJSON line: %{public}@", String(line))
            }
        }
        guard let event = events.last else {
            return .failure(.invalidOutput)
        }
        return .success(event)
    }

    func installUpdate() -> AsyncStream<UpdateEvent> {
        AsyncStream { continuation in
            Task {
                guard shutdownTask == nil else {
                    continuation.finish()
                    return
                }
                guard let executable = getProxyctlPath() else {
                    continuation.yield(UpdateEvent(stage: "error", message: "proxyctl not found", progress: nil, newVersion: nil))
                    continuation.finish()
                    return
                }

                let process = Process()
                process.executableURL = URL(fileURLWithPath: executable)
                process.arguments = ["self-update", "--json"]
                let inputHandle: FileHandle
                do {
                    inputHandle = try FileHandle(forReadingFrom: URL(fileURLWithPath: "/dev/null"))
                    process.standardInput = inputHandle
                } catch {
                    continuation.yield(UpdateEvent(stage: "error", message: error.localizedDescription, progress: nil, newVersion: nil))
                    continuation.finish()
                    return
                }
                self.updateProcess = process

                let outputPipe = Pipe()
                let errorPipe = Pipe()
                process.standardOutput = outputPipe
                process.standardError = errorPipe

                let outputReader = UpdateOutputReader(continuation: continuation)
                outputPipe.fileHandleForReading.readabilityHandler = { handle in
                    outputReader.read(from: handle)
                }

                errorPipe.fileHandleForReading.readabilityHandler = { handle in
                    let data = handle.availableData
                    if let message = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines),
                       !message.isEmpty {
                        continuation.yield(UpdateEvent(stage: "error", message: message, progress: nil, newVersion: nil))
                    }
                }

                process.terminationHandler = { _ in
                    try? inputHandle.close()
                    outputPipe.fileHandleForReading.readabilityHandler = nil
                    errorPipe.fileHandleForReading.readabilityHandler = nil
                    outputReader.read(from: outputPipe.fileHandleForReading, finishing: true)
                    continuation.finish()
                }

                do {
                    try process.run()
                } catch {
                    try? inputHandle.close()
                    continuation.yield(UpdateEvent(stage: "error", message: error.localizedDescription, progress: nil, newVersion: nil))
                    continuation.finish()
                }
            }
        }
    }

    func cancelUpdate() {
        updateProcess?.terminate()
        updateProcess = nil
    }

    func runDiagnose() async -> Result<DiagnoseReport, HelperError> {
        let result = await runCommand(["diagnose", "--json"])
        return result.flatMap { data in
            do {
                let report = try JSONDecoder().decode(DiagnoseReport.self, from: data)
                return .success(report)
            } catch {
                return .failure(.decodingFailed(error))
            }
        }
    }

    func getMode() async -> Result<String, HelperError> {
        let result = await runCommand(["mode", "status", "--json"])
        return result.flatMap { data in
            do {
                let status = try JSONDecoder().decode(ModeStatus.self, from: data)
                return .success(status.mode)
            } catch {
                return .failure(.decodingFailed(error))
            }
        }
    }

    func setMode(_ mode: String) async -> Result<Void, HelperError> {
        let result = await runCommand(["mode", "set", mode])
        return result.map { _ in () }
    }

    func getProxyGroups() async -> Result<[ProxyGroup], HelperError> {
        let result = await runCommand(["groups", "list", "--json"])
        return result.flatMap { data in
            do {
                let dict = try JSONDecoder().decode([String: ProxyGroup].self, from: data)
                let groups = dict.map { name, group in
                    ProxyGroup(name: name, type: group.type, now: group.now, all: group.all)
                }.sorted { $0.name < $1.name }
                return .success(groups)
            } catch {
                return .failure(.decodingFailed(error))
            }
        }
    }

    func testProxyDelays() async -> Result<[String: ProxyDelayResult], HelperError> {
        let result = await runCommand(["groups", "delay", "--json"])
        return result.flatMap { data in
            do {
                let delays = try JSONDecoder().decode([String: ProxyDelayResult].self, from: data)
                return .success(delays)
            } catch {
                return .failure(.decodingFailed(error))
            }
        }
    }

    func proxyDelay(name: String) async -> Result<ProxyDelayResult, HelperError> {
        let result = await runCommand(["proxy-delay", name, "--json"])
        return result.flatMap { data in
            do {
                let r = try JSONDecoder().decode(ProxyDelayResult.self, from: data)
                return .success(r)
            } catch {
                return .failure(.decodingFailed(error))
            }
        }
    }

    func selectProxy(group: String, proxy: String) async -> Result<Void, HelperError> {
        let result = await runCommand(["groups", "select", group, proxy])
        return result.map { _ in () }
    }

    func getProfiles() async -> Result<[Profile], HelperError> {
        let result = await runCommand(["profile", "list", "--json"])
        return result.flatMap { data in
            do {
                let profiles = try JSONDecoder().decode([Profile].self, from: data)
                return .success(profiles)
            } catch {
                return .failure(.decodingFailed(error))
            }
        }
    }

    func activateProfile(id: String) async -> Result<Void, HelperError> {
        let result = await runCommand(["profile", "activate", id])
        return result.map { _ in () }
    }

    func deleteProfile(id: String) async -> Result<Void, HelperError> {
        let result = await runCommand(["profile", "delete", id])
        return result.map { _ in () }
    }

    func bootstrap(subscriptionURL: String) -> AsyncStream<String> {
        AsyncStream { continuation in
            Task {
                guard shutdownTask == nil, campusDisconnectTask == nil else {
                    continuation.finish()
                    return
                }
                guard let executable = getProxyctlPath() else {
                    continuation.yield("proxyctl not found")
                    continuation.finish()
                    return
                }

                let task = Process()
                task.executableURL = URL(fileURLWithPath: executable)
                task.arguments = ["bootstrap", subscriptionURL]
                let inputHandle: FileHandle
                do {
                    inputHandle = try FileHandle(forReadingFrom: URL(fileURLWithPath: "/dev/null"))
                    task.standardInput = inputHandle
                } catch {
                    continuation.yield("Failed to run: \(error.localizedDescription)")
                    continuation.finish()
                    return
                }

                let outputPipe = Pipe()
                let errorPipe = Pipe()
                task.standardOutput = outputPipe
                task.standardError = errorPipe

                outputPipe.fileHandleForReading.readabilityHandler = { handle in
                    let data = handle.availableData
                    if let line = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines),
                       !line.isEmpty {
                        continuation.yield(line)
                    }
                }

                errorPipe.fileHandleForReading.readabilityHandler = { handle in
                    let data = handle.availableData
                    if let line = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines),
                       !line.isEmpty {
                        continuation.yield(line)
                    }
                }

                task.terminationHandler = { _ in
                    try? inputHandle.close()
                    outputPipe.fileHandleForReading.readabilityHandler = nil
                    errorPipe.fileHandleForReading.readabilityHandler = nil
                    continuation.finish()
                }

                do {
                    try task.run()
                    bootstrapProcess = task
                } catch {
                    try? inputHandle.close()
                    continuation.yield("Failed to run: \(error.localizedDescription)")
                    continuation.finish()
                }
            }
        }
    }

}
