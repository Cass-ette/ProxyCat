import SwiftUI
import Combine
import AppKit

@MainActor
class StatusViewModel: ObservableObject {
    @Published var coreRunning = false
    @Published var corePID = 0
    @Published var systemProxyEnabled = false
    @Published var currentMode = "Rule"
    @Published var currentGroup = ""
    @Published var currentNode = ""
    @Published var lastError: String?
    @Published var isRefreshing = false
    @Published var testResult: TestResult?
    @Published var diagnoseReport: DiagnoseReport?

    private let helper = HelperClient.shared
    private var refreshTimer: Timer?

    func startAutoRefresh(interval: TimeInterval = 5.0) {
        refreshTimer?.invalidate()
        refreshTimer = Timer.scheduledTimer(withTimeInterval: interval, repeats: true) { _ in
            Task { await self.refreshStatus() }
        }
        Task { await refreshStatus() }
    }

    func stopAutoRefresh() {
        refreshTimer?.invalidate()
        refreshTimer = nil
    }

    func refreshStatus() async {
        isRefreshing = true
        defer { isRefreshing = false }

        // Core status
        let coreResult = await helper.getCoreStatus()
        switch coreResult {
        case .success(let status):
            coreRunning = status.running
            corePID = status.pid
        case .failure(let error):
            lastError = "Core: \(error.localizedDescription)"
        }

        // System proxy status
        let proxyResult = await helper.getSystemProxyStatus()
        switch proxyResult {
        case .success(let status):
            systemProxyEnabled = status.socksEnabled || status.httpEnabled
        case .failure:
            systemProxyEnabled = false
        }

        // Proxy groups for current selection
        let groupsResult = await helper.getProxyGroups()
        switch groupsResult {
        case .success(let groups):
            if let firstGroup = groups.first {
                currentGroup = firstGroup.name
                currentNode = firstGroup.now
            }
        case .failure:
            currentGroup = ""
            currentNode = ""
        }
    }

    func enableSystemProxy() async {
        let result = await helper.enableSystemProxy()
        if case .failure(let error) = result {
            lastError = "Enable proxy failed: \(error.localizedDescription)"
        }
        await refreshStatus()
    }

    func disableSystemProxy() async {
        let result = await helper.disableSystemProxy()
        if case .failure(let error) = result {
            lastError = "Disable proxy failed: \(error.localizedDescription)"
        }
        await refreshStatus()
    }

    func startCore() async {
        let result = await helper.startCore()
        if case .failure(let error) = result {
            lastError = "Start core failed: \(error.localizedDescription)"
        }
        await refreshStatus()
    }

    func stopCore() async {
        let result = await helper.stopCore()
        if case .failure(let error) = result {
            lastError = "Stop core failed: \(error.localizedDescription)"
        }
        await refreshStatus()
    }

    func restartCore() async {
        let result = await helper.restartCore()
        if case .failure(let error) = result {
            lastError = "Restart core failed: \(error.localizedDescription)"
        }
        await refreshStatus()
    }

    func testConnection() async {
        let result = await helper.testConnection()
        switch result {
        case .success(let test):
            testResult = test
            lastError = nil
        case .failure(let error):
            lastError = "Test failed: \(error.localizedDescription)"
            testResult = nil
        }
    }

    func updateSubscription() async {
        let result = await helper.updateSubscription()
        if case .failure(let error) = result {
            lastError = "Update failed: \(error.localizedDescription)"
        }
    }

    func runDiagnose() async {
        let result = await helper.runDiagnose()
        switch result {
        case .success(let report):
            diagnoseReport = report
            lastError = nil
        case .failure(let error):
            lastError = "Diagnose failed: \(error.localizedDescription)"
        }
    }

    func openConfigFolder() {
        openDirectory(applicationSupportDirectory.appendingPathComponent("config", isDirectory: true), label: "config folder")
    }

    func openLogs() {
        openDirectory(applicationSupportDirectory.appendingPathComponent("logs", isDirectory: true), label: "logs folder")
    }

    private var applicationSupportDirectory: URL {
        FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library", isDirectory: true)
            .appendingPathComponent("Application Support", isDirectory: true)
            .appendingPathComponent("ProxyCat", isDirectory: true)
    }

    private func openDirectory(_ url: URL, label: String) {
        do {
            try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
            if !NSWorkspace.shared.open(url) {
                lastError = "Open \(label) failed"
            }
        } catch {
            lastError = "Open \(label) failed: \(error.localizedDescription)"
        }
    }
}
