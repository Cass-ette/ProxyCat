import Foundation
import os.log

enum HelperError: Error, LocalizedError {
    case executableNotFound
    case executionFailed(String)
    case invalidOutput
    case decodingFailed(Error)

    var errorDescription: String? {
        switch self {
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

actor HelperClient {
    static let shared = HelperClient()

    private var proxyctlPath: String?
    private var updateProcess: Process?

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

    private func runCommand(_ args: [String]) async -> Result<Data, HelperError> {
        guard let executable = getProxyctlPath() else {
            return .failure(.executableNotFound)
        }

        return await withCheckedContinuation { continuation in
            DispatchQueue.global().async {
                let task = Process()
                task.executableURL = URL(fileURLWithPath: executable)
                task.arguments = args

                let outputPipe = Pipe()
                let errorPipe = Pipe()
                task.standardOutput = outputPipe
                task.standardError = errorPipe

                do {
                    try task.run()
                    task.waitUntilExit()

                    let data = outputPipe.fileHandleForReading.readDataToEndOfFile()
                    let errorData = errorPipe.fileHandleForReading.readDataToEndOfFile()

                    if task.terminationStatus != 0,
                       let stderr = String(data: errorData, encoding: .utf8),
                       !stderr.isEmpty {
                        continuation.resume(returning: .failure(.executionFailed(stderr)))
                        return
                    }

                    continuation.resume(returning: .success(data))
                } catch {
                    continuation.resume(returning: .failure(.executionFailed(error.localizedDescription)))
                }
            }
        }
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
                guard let executable = getProxyctlPath() else {
                    continuation.yield(UpdateEvent(stage: "error", message: "proxyctl not found", progress: nil, newVersion: nil))
                    continuation.finish()
                    return
                }

                let process = Process()
                process.executableURL = URL(fileURLWithPath: executable)
                process.arguments = ["self-update", "--json"]
                self.updateProcess = process

                let outputPipe = Pipe()
                let errorPipe = Pipe()
                process.standardOutput = outputPipe
                process.standardError = errorPipe

                let decoder = JSONDecoder()
                var buffer = Data()
                outputPipe.fileHandleForReading.readabilityHandler = { handle in
                    let data = handle.availableData
                    guard !data.isEmpty else { return }
                    buffer.append(data)
                    while let newline = buffer.firstIndex(of: 10) {
                        let line = buffer[..<newline]
                        buffer.removeSubrange(...newline)
                        if let event = try? decoder.decode(UpdateEvent.self, from: Data(line)) {
                            continuation.yield(event)
                        } else {
                            os_log("self-update: skipping malformed NDJSON line: %{public}@", String(decoding: line, as: UTF8.self))
                        }
                    }
                }

                errorPipe.fileHandleForReading.readabilityHandler = { handle in
                    let data = handle.availableData
                    if let message = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines),
                       !message.isEmpty {
                        continuation.yield(UpdateEvent(stage: "error", message: message, progress: nil, newVersion: nil))
                    }
                }

                process.terminationHandler = { _ in
                    outputPipe.fileHandleForReading.readabilityHandler = nil
                    errorPipe.fileHandleForReading.readabilityHandler = nil
                    let remaining = outputPipe.fileHandleForReading.readDataToEndOfFile()
                    if !remaining.isEmpty {
                        buffer.append(remaining)
                    }
                    while let newline = buffer.firstIndex(of: 10) {
                        let line = buffer[..<newline]
                        buffer.removeSubrange(...newline)
                        if let event = try? decoder.decode(UpdateEvent.self, from: Data(line)) {
                            continuation.yield(event)
                        }
                    }
                    if !buffer.isEmpty,
                       let event = try? decoder.decode(UpdateEvent.self, from: buffer) {
                        continuation.yield(event)
                    }
                    continuation.finish()
                }

                do {
                    try process.run()
                } catch {
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

    func selectProxy(group: String, proxy: String) async -> Result<Void, HelperError> {
        let result = await runCommand(["groups", "select", group, proxy])
        return result.map { _ in () }
    }

    func bootstrap(subscriptionURL: String) -> AsyncStream<String> {
        AsyncStream { continuation in
            Task {
                guard let executable = getProxyctlPath() else {
                    continuation.yield("proxyctl not found")
                    continuation.finish()
                    return
                }

                let task = Process()
                task.executableURL = URL(fileURLWithPath: executable)
                task.arguments = ["bootstrap", subscriptionURL]

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
                    outputPipe.fileHandleForReading.readabilityHandler = nil
                    errorPipe.fileHandleForReading.readabilityHandler = nil
                    continuation.finish()
                }

                do {
                    try task.run()
                } catch {
                    continuation.yield("Failed to run: \(error.localizedDescription)")
                    continuation.finish()
                }
            }
        }
    }

}
