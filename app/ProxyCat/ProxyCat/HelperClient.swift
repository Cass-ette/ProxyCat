import Foundation

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

    func selectProxy(group: String, proxy: String) async -> Result<Void, HelperError> {
        let result = await runCommand(["select", group, proxy])
        return result.map { _ in () }
    }

}
