import SwiftUI
import Combine
import AppKit
import CoreWLAN
import CoreLocation

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
    @Published var toggleTestResult: ProxyDelayResult?
    @Published var isToggleTesting = false
    @Published var bootstrapStatus: String = ""
    @Published private(set) var isBootstrapping = false
    @Published var updateStatus: String = ""
    @Published var updateProgress: Int?
    @Published var availableUpdateVersion: String?
    @Published var isCheckingForUpdate = false
    @Published var isInstallingUpdate = false
    @Published var showRestartAlert = false
    @Published var profiles: [Profile] = []
    @Published var activeProfileID: String?
    @Published var updateFailed = false
    @Published var currentWiFiSSID: String?
    @Published var campusProtectionNotice: String?
    @Published var protectedWiFiSSIDs: [String] {
        didSet {
            defaults.set(protectedWiFiSSIDs, forKey: "protectedWiFiSSIDs")
            campusSettingsChanged()
        }
    }
    @Published var campusProtectionEnabled: Bool {
        didSet {
            defaults.set(campusProtectionEnabled, forKey: "campusProtectionEnabled")
            campusSettingsChanged()
        }
    }
    @Published var campusNetworkAction: CampusNetworkAction {
        didSet {
            defaults.set(campusNetworkAction.rawValue, forKey: "campusNetworkAction")
            campusSettingsChanged()
        }
    }

    private let helper: HelperClient
    private let defaults: UserDefaults
    private let wifiSSIDProvider: () -> String?
    private let campusPrompt: (String) -> Bool
    private var refreshTimer: Timer?
    private var campusActionInProgress = false
    private var lastCampusPromptSSID: String?
    private lazy var locationDelegate = WiFiLocationDelegate { [weak self] in
        Task { @MainActor [weak self] in await self?.refreshStatus() }
    }
    private lazy var locationManager: CLLocationManager = {
        let manager = CLLocationManager()
        manager.delegate = locationDelegate
        return manager
    }()

    init(helper: HelperClient = .shared, defaults: UserDefaults = .standard,
         wifiSSIDProvider: @escaping () -> String? = {
             CWWiFiClient.shared().interfaces()?.compactMap { $0.ssid() }.first
         }, campusPrompt: ((String) -> Bool)? = nil) {
        self.helper = helper
        self.defaults = defaults
        self.wifiSSIDProvider = wifiSSIDProvider
        self.campusPrompt = campusPrompt ?? { ssid in
            let alert = NSAlert()
            alert.messageText = "检测到校园网 Wi-Fi"
            alert.informativeText = "当前网络「\(ssid)」已标记为校园网，ProxyCat 代理仍在运行。是否关闭系统代理并停止代理引擎？"
            alert.addButton(withTitle: "关闭代理")
            alert.addButton(withTitle: "暂时保持")
            alert.alertStyle = .warning
            NSApplication.shared.activate(ignoringOtherApps: true)
            return alert.runModal() == .alertFirstButtonReturn
        }
        protectedWiFiSSIDs = defaults.stringArray(forKey: "protectedWiFiSSIDs") ?? []
        campusProtectionEnabled = defaults.object(forKey: "campusProtectionEnabled") as? Bool ?? true
        campusNetworkAction = CampusNetworkAction(rawValue: defaults.string(forKey: "campusNetworkAction") ?? "prompt") ?? .prompt
    }

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
        guard !isRefreshing else { return }
        isRefreshing = true
        defer { isRefreshing = false }

        currentWiFiSSID = wifiSSIDProvider()

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
            systemProxyEnabled = status.socksEnabled || status.httpEnabled || status.httpsEnabled
        case .failure:
            systemProxyEnabled = false
        }

        await enforceCampusNetworkPolicy()

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

    func addCurrentWiFiToCampusNetworks() {
        guard let ssid = currentWiFiSSID else { return }
        addCampusNetwork(ssid)
    }

    func requestWiFiAccess() {
        guard CLLocationManager.locationServicesEnabled() else {
            lastError = "请在系统设置中启用定位服务后读取 Wi-Fi 名称。"
            return
        }

        switch locationManager.authorizationStatus {
        case .notDetermined:
            NSApplication.shared.activate(ignoringOtherApps: true)
            locationManager.requestWhenInUseAuthorization()
        case .denied, .restricted:
            lastError = "请在系统设置的隐私与安全性中允许 ProxyCat 使用定位服务。"
        default:
            break
        }
    }

    func addCampusNetwork(_ ssid: String) {
        guard !ssid.isEmpty, !protectedWiFiSSIDs.contains(ssid) else { return }
        protectedWiFiSSIDs.append(ssid)
    }

    func removeCampusNetwork(_ ssid: String) {
        protectedWiFiSSIDs.removeAll { $0 == ssid }
    }

    private func enforceCampusNetworkPolicy() async {
        guard !campusActionInProgress else { return }
        currentWiFiSSID = wifiSSIDProvider()
        guard campusProtectionEnabled,
              let ssid = currentWiFiSSID,
              protectedWiFiSSIDs.contains(ssid) else {
            lastCampusPromptSSID = nil
            campusProtectionNotice = nil
            return
        }

        guard systemProxyEnabled || coreRunning || isBootstrapping else {
            lastCampusPromptSSID = nil
            return
        }

        campusActionInProgress = true
        defer { campusActionInProgress = false }
        if campusNetworkAction == .prompt {
            guard lastCampusPromptSSID != ssid else { return }
            lastCampusPromptSSID = ssid
            campusProtectionNotice = "校园网「\(ssid)」：ProxyCat 代理仍在运行。"
            guard campusPrompt(ssid) else { return }
        }

        let result = await helper.disconnectForCampusNetwork()
        switch result {
        case .success:
            systemProxyEnabled = false
            coreRunning = false
            corePID = 0
            toggleTestResult = nil
            campusProtectionNotice = "校园网「\(ssid)」：已关闭系统代理并停止代理引擎。"
        case .failure(let error):
            lastError = "校园网断开代理失败：\(error.localizedDescription)"
            campusProtectionNotice = "校园网「\(ssid)」：未能完全关闭代理，请检查错误信息。"
        }
    }

    private func campusSettingsChanged() {
        lastCampusPromptSSID = nil
        Task { await refreshStatus() }
    }

    private func canStartProxyOnCurrentNetwork() -> Bool {
        currentWiFiSSID = wifiSSIDProvider()
        guard campusProtectionEnabled, campusNetworkAction == .disconnect,
              let ssid = currentWiFiSSID, protectedWiFiSSIDs.contains(ssid) else {
            lastCampusPromptSSID = nil
            return true
        }
        campusProtectionNotice = "校园网「\(ssid)」：自动断开模式已阻止开启 ProxyCat 代理。"
        return false
    }

    func enableSystemProxy() async {
        guard canStartProxyOnCurrentNetwork() else { return }
        isToggleTesting = true
        toggleTestResult = nil
        defer { isToggleTesting = false }

        let result = await helper.enableSystemProxy()
        if case .failure(let error) = result {
            lastError = "Enable proxy failed: \(error.localizedDescription)"
            await refreshStatus()
            return
        }

        await refreshStatus()
        guard systemProxyEnabled else { return }

        // Test the currently selected group (Mihomo tests its selected proxy).
        // Fall back to the first group's name if currentGroup is empty (e.g. cold start).
        let target = currentGroup.isEmpty ? (proxyGroups.first?.name ?? "") : currentGroup
        guard !target.isEmpty else {
            lastError = "Toggle test skipped: no proxy group available"
            await refreshStatus()
            return
        }

        let delayResult = await helper.proxyDelay(name: target)
        switch delayResult {
        case .success(let r):
            toggleTestResult = r
            if r.error != nil {
                // Failure path: auto-diagnose so the user sees which layer failed.
                await runDiagnose()
            } else {
                lastError = nil
            }
        case .failure(let error):
            lastError = "Toggle test failed: \(error.localizedDescription)"
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
        guard canStartProxyOnCurrentNetwork() else { return }
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
        guard canStartProxyOnCurrentNetwork() else { return }
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
        guard canStartProxyOnCurrentNetwork() else { return }
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

    func deleteProfile(id: String) async {
        let result = await helper.deleteProfile(id: id)
        switch result {
        case .success:
            await loadProfiles()
        case .failure(let error):
            lastError = "Delete profile failed: \(error.localizedDescription)"
        }
    }

    func bootstrap(subscriptionURL: String) async {
        guard canStartProxyOnCurrentNetwork() else {
            bootstrapStatus = "校园网保护已阻止启动代理。"
            return
        }
        isBootstrapping = true
        defer { isBootstrapping = false }
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
        AppDelegate.relaunchOnExit = true
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

private final class WiFiLocationDelegate: NSObject, CLLocationManagerDelegate {
    private let onAuthorizationChange: @Sendable () -> Void

    init(onAuthorizationChange: @escaping @Sendable () -> Void) {
        self.onAuthorizationChange = onAuthorizationChange
    }

    func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        onAuthorizationChange()
    }
}
