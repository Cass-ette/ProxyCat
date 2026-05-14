import SwiftUI
import Combine
import AppKit

@MainActor
class StatusViewModel: ObservableObject {
    @Published var coreRunning = false
    @Published var corePID = 0
    @Published var systemProxyEnabled = false
    @Published var currentMode = "rule"
    @Published var currentGroup = ""
    @Published var currentNode = ""
    @Published var proxyGroups: [ProxyGroup] = []
    @Published var lastError: String?
    @Published var isRefreshing = false
    @Published var testResult: TestResult?
    @Published var diagnoseReport: DiagnoseReport?
    @Published var proxyDelays: [String: ProxyDelayResult] = [:]
    @Published var isTestingDelays = false
    @Published var bootstrapStatus: String = ""
    @Published var updateStatus: String = ""
    @Published var updateProgress: Int?
    @Published var availableUpdateVersion: String?
    @Published var isCheckingForUpdate = false
    @Published var isInstallingUpdate = false
    @Published var showRestartAlert = false
    @Published var profiles: [Profile] = []
    @Published var activeProfileID: String?
    @Published var updateFailed = false

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

        let modeResult = await helper.getMode()
        switch modeResult {
        case .success(let mode):
            currentMode = mode
        case .failure:
            currentMode = "rule"
        }

        // Proxy groups for current selection
        let groupsResult = await helper.getProxyGroups()
        switch groupsResult {
        case .success(let groups):
            proxyGroups = groups
            let selectedGroup = groups.first { $0.name == currentGroup } ?? groups.first
            currentGroup = selectedGroup?.name ?? ""
            currentNode = selectedGroup?.now ?? ""
        case .failure:
            proxyGroups = []
            currentGroup = ""
            currentNode = ""
        }

        await loadProfiles()
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

    func setMode(_ mode: String) async {
        let result = await helper.setMode(mode)
        if case .failure(let error) = result {
            lastError = "Set mode failed: \(error.localizedDescription)"
        }
        await refreshStatus()
    }

    func selectProxy(group: String, proxy: String) async {
        let result = await helper.selectProxy(group: group, proxy: proxy)
        if case .failure(let error) = result {
            lastError = "Select proxy failed: \(error.localizedDescription)"
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

    func testProxyDelays() async {
        isTestingDelays = true
        defer { isTestingDelays = false }

        let result = await helper.testProxyDelays()
        switch result {
        case .success(let delays):
            proxyDelays = delays
            lastError = nil
        case .failure(let error):
            lastError = "Delay test failed: \(error.localizedDescription)"
        }
    }

    func updateSubscription() async {
        let result = await helper.updateSubscription()
        if case .failure(let error) = result {
            lastError = "Update failed: \(error.localizedDescription)"
        }
    }

    func checkForUpdate() async {
        isCheckingForUpdate = true
        updateStatus = "正在检查更新..."
        updateProgress = nil
        availableUpdateVersion = nil
        defer { isCheckingForUpdate = false }

        let result = await helper.checkForUpdate()
        switch result {
        case .success(let event):
            updateStatus = event.message
            updateProgress = event.progress
            availableUpdateVersion = event.newVersion
            lastError = event.stage == "error" ? event.message : nil
        case .failure(let error):
            updateStatus = ""
            lastError = "检查更新失败：\(error.localizedDescription)"
        }
    }

    func installUpdate() async {
        isInstallingUpdate = true
        updateFailed = false
        updateStatus = "正在准备更新..."
        updateProgress = nil
        lastError = nil
        defer { isInstallingUpdate = false }

        let stream = await helper.installUpdate()
        for await event in stream {
            updateStatus = event.message
            updateProgress = event.progress
            availableUpdateVersion = event.stage == "done" ? nil : (event.newVersion ?? availableUpdateVersion)
            if event.stage == "done" {
                showRestartAlert = true
            }
            if event.stage == "error" {
                updateFailed = true
            }
        }
    }

    func cancelUpdate() async {
        await helper.cancelUpdate()
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

    func loadProfiles() async {
        let result = await helper.getProfiles()
        switch result {
        case .success(let p):
            profiles = p
            activeProfileID = p.first(where: { $0.active })?.id
        case .failure:
            profiles = []
        }
    }

    func activateProfile(id: String) async {
        let result = await helper.activateProfile(id: id)
        if case .failure(let error) = result {
            lastError = "Activate profile failed: \(error.localizedDescription)"
            return
        }
        activeProfileID = id
        let restartResult = await helper.restartCore()
        if case .failure(let error) = restartResult {
            lastError = "Restart core failed: \(error.localizedDescription)"
        }
        await refreshStatus()
    }

    func bootstrap(subscriptionURL: String) async {
        bootstrapStatus = "正在启动..."
        lastError = nil
        testResult = nil
        diagnoseReport = nil

        let stream = await helper.bootstrap(subscriptionURL: subscriptionURL)

        for await line in stream {
            bootstrapStatus = line
        }

        await refreshStatus()
    }

    func openConfigFolder() {
        openDirectory(applicationSupportDirectory.appendingPathComponent("config", isDirectory: true), label: "config folder")
    }

    func openLogs() {
        openDirectory(applicationSupportDirectory.appendingPathComponent("logs", isDirectory: true), label: "logs folder")
    }

    func restartApp() {
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/bin/bash")
        task.arguments = ["-c", "sleep 0.5; open \"\(Bundle.main.bundlePath)\""]
        try? task.run()
        NSApplication.shared.terminate(nil)
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
