import SwiftUI
import Combine
import AppKit
import CoreWLAN
import CoreLocation

@MainActor
class StatusViewModel: ObservableObject {
    @Published var connectionLog = ConnectionLog()
    @Published var connectionLogError: String?
    @Published var connectionLookupNotice: String?
    @Published private(set) var isLoadingConnections = false
    private var connectionObservationTask: Task<Void, Never>?
    private var connectionObservationID: UUID?
    @Published var websiteResult: WebsiteCheckResult?
    @Published var websiteCheckError: String?
    @Published var isCheckingWebsite = false
    @Published var isRecoveringWebsite = false
    @Published var nodeChangeNotice: NodeChangeNotice?
    @Published var websitePreviousMode: String?
    private var nodeBaseline: (context: String, groups: [ProxyGroup])?
    private var selectionInProgress = false
    private var websiteContext = ""
    private var lastWebsiteInput = ""

    @Published var coreRunning = false
    @Published var corePID = 0
    @Published var systemProxyEnabled = false
    @Published var currentMode = "rule"
    @Published var tunEnabled = false
    @Published var tunDevice = ""
    @Published var isChangingTUN = false
    @Published private(set) var tunChangeTarget: Bool?
    @Published private(set) var tunProgress: TUNProgress?
    @Published private(set) var tunChangeStartedAt: Date?
    private var tunOperationID: UUID?
    @Published var currentGroup = ""
    @Published var currentNode = ""
    @Published var proxyGroups: [ProxyGroup] = []
    @Published var proxyGroupsError: String?
    @Published var lastError: String?
    @Published var isRefreshing = false
    @Published var testResult: TestResult?
    @Published var diagnoseReport: DiagnoseReport?
    @Published private(set) var diagnosisNeedsRefresh = false
    @Published private(set) var isDiagnosing = false
    private var networkFeedbackRevision: UInt64 = 0
    @Published var proxyDelays: [String: ProxyDelayResult] = [:]
    @Published var isTestingDelays = false
    @Published var toggleTestResult: ProxyDelayResult?
    @Published var isToggleTesting = false
    @Published var isChangingMode = false
    @Published var bootstrapStatus: String = ""
    @Published private(set) var isBootstrapping = false
    @Published var updateStatus: String = ""
    @Published var updateErrorDetails: String?
    @Published var updateProgress: Int?
    @Published var availableUpdateVersion: String?
    @Published var isCheckingForUpdate = false
    @Published private(set) var isCheckingForUpdateAutomatically = false
    @Published var isInstallingUpdate = false
    @Published var showRestartAlert = false
    @Published var updateReleaseVersion: String?
    @Published var updateReleaseNotes = ""
    @Published var updateReleaseURL: String?
    @Published var updatePublishedAt: String?
    @Published var showingCurrentReleaseNotes = true
    @Published var profiles: [Profile] = []
    @Published private(set) var isRefreshingTraffic = false
    @Published private(set) var trafficRefreshError: String?
    private var trafficAttempts: [String: Date] = [:]
    @Published var activeProfileID: String? {
        didSet {
            // First loading saved profile metadata does not change the route.
            if let oldValue, oldValue != activeProfileID { invalidateNetworkFeedback() }
        }
    }
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
    private let appVersion: String
    private let releaseNotesOverride: String?
    private let dateProvider: () -> Date
    private let wifiSSIDProvider: () -> String?
    private let campusPrompt: (String) -> Bool
    private var refreshTimer: Timer?
    private var automaticUpdateCheckTask: Task<Void, Never>?
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
         }, campusPrompt: ((String) -> Bool)? = nil, currentVersion: String? = nil,
         currentReleaseNotes: String? = nil, dateProvider: @escaping () -> Date = { Date() }) {
        self.helper = helper
        self.defaults = defaults
        self.appVersion = currentVersion ?? Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "未知版本"
        self.releaseNotesOverride = currentReleaseNotes
        self.dateProvider = dateProvider
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
        if let data = defaults.data(forKey: "availableUpdate.\(appVersion)"),
           let event = try? JSONDecoder().decode(UpdateEvent.self, from: data),
           event.stage == "done", event.newVersion != nil {
            availableUpdateVersion = event.newVersion
            updateStatus = event.message
            updateReleaseVersion = event.releaseVersion ?? event.newVersion
            updateReleaseNotes = event.releaseNotes ?? ""
            updateReleaseURL = event.releaseURL
            updatePublishedAt = event.publishedAt
        }
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
        automaticUpdateCheckTask?.cancel()
        stopObservingConnections()
    }

    func startObservingConnections() {
        guard connectionObservationTask == nil else { return }
        let observationID = UUID()
        connectionObservationID = observationID
        connectionObservationTask = Task { [weak self] in
            let preparation = await self?.helper.prepareConnectionObservation()
            guard let self, !Task.isCancelled, self.connectionObservationID == observationID else { return }
            if case .failure = preparation {
                self.connectionLookupNotice = "应用识别暂不可用，仍显示已捕获的连接。重新打开此窗口可重试。"
            } else {
                self.connectionLookupNotice = nil
            }
            while !Task.isCancelled {
                await self.refreshConnections()
                do { try await Task.sleep(nanoseconds: 2_000_000_000) }
                catch { return }
            }
        }
    }

    func stopObservingConnections() {
        connectionObservationID = nil
        connectionObservationTask?.cancel()
        connectionObservationTask = nil
    }

    func refreshConnections() async {
        guard !isLoadingConnections else { return }
        let observationID = connectionObservationID
        isLoadingConnections = true
        defer { isLoadingConnections = false }
        let result = await helper.getConnections()
        guard !Task.isCancelled, connectionObservationID == observationID else { return }
        switch result {
        case .success(let snapshot):
            connectionLog.observe(snapshot, at: dateProvider())
            connectionLogError = nil
        case .failure:
            connectionLogError = "暂时无法读取连接，请确认代理引擎正在运行。已有记录保留为上次采样结果。"
        }
    }

    func clearEndedConnections() { connectionLog.clearEnded() }

    func refreshStatus() async {
        guard !isRefreshing else { return }
        isRefreshing = true
        defer { isRefreshing = false }
        var previousContext = networkFeedbackContext
        func invalidateChangedFeedback() {
            let context = networkFeedbackContext
            if previousContext != context {
                invalidateNetworkFeedback()
                previousContext = context
            }
        }

        currentWiFiSSID = wifiSSIDProvider()
        invalidateChangedFeedback()

        // Core status
        let coreResult = await helper.getCoreStatus()
        switch coreResult {
        case .success(let status):
            coreRunning = status.running
            corePID = status.pid
        case .failure(let error):
            lastError = "Core: \(error.localizedDescription)"
            invalidateNetworkFeedback()
        }
        invalidateChangedFeedback()

        // System proxy status
        let proxyResult = await helper.getSystemProxyStatus()
        switch proxyResult {
        case .success(let status):
            systemProxyEnabled = status.socksEnabled || status.httpEnabled || status.httpsEnabled
        case .failure:
            systemProxyEnabled = false
            invalidateNetworkFeedback()
        }
        invalidateChangedFeedback()

        await enforceCampusNetworkPolicy()
        invalidateChangedFeedback()

        let modeResult = await helper.getModeStatus()
        switch modeResult {
        case .success(let status):
            currentMode = status.mode
            tunEnabled = coreRunning && status.tunEnabled == true
            tunDevice = status.tunDevice ?? ""
        case .failure:
            invalidateNetworkFeedback()
            if !coreRunning { currentMode = "rule"; tunEnabled = false; tunDevice = "" }
        }
        invalidateChangedFeedback()

        // Proxy groups for current selection
        let groupsResult = await helper.getProxyGroups()
        switch groupsResult {
        case .success(let groups) where !groups.isEmpty || !coreRunning:
            updateProxyGroups(coreRunning ? groups : [])
            proxyGroupsError = nil
        case .success:
            proxyGroupsError = "节点组暂未就绪，已保留上次的节点列表。"
            invalidateNetworkFeedback()
        case .failure(let error):
            invalidateNetworkFeedback()
            // A controller request can fail while switching networks or loading
            // a config. Keep the last selection until the core actually stops.
            if coreRunning {
                proxyGroupsError = "暂时无法读取节点：\(error.localizedDescription)"
            } else {
                updateProxyGroups([])
                proxyGroupsError = nil
            }
        }
        invalidateChangedFeedback()

        await loadProfiles()
        invalidateChangedFeedback()
        if websitePreviousMode != nil && currentMode != "global" { websitePreviousMode = nil }
        recordNodeChanges()
        scheduleAutomaticUpdateCheck()
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

        guard systemProxyEnabled || coreRunning || tunEnabled || isBootstrapping else {
            lastCampusPromptSSID = nil
            return
        }

        campusActionInProgress = true
        invalidateNetworkFeedback()
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
            tunEnabled = false
            tunDevice = ""
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
        guard !isToggleTesting, !isChangingMode, !isChangingTUN, canStartProxyOnCurrentNetwork() else { return }
        invalidateNetworkFeedback()
        isToggleTesting = true
        toggleTestResult = nil
        lastError = nil
        defer { isToggleTesting = false }

        // Reuse the config on disk instead of downloading the subscription again.
        let coreResult = await helper.getCoreStatus()
        switch coreResult {
        case .success(let status):
            coreRunning = status.running
            corePID = status.pid
            if !status.running {
                guard canStartProxyOnCurrentNetwork() else { return }
                if case .failure(let error) = await helper.startCore() {
                    lastError = "无法启动已保存的配置：\(error.localizedDescription)"
                    return
                }
                coreRunning = true
            }
        case .failure(let error):
            lastError = "无法检查代理引擎：\(error.localizedDescription)"
            return
        }

        // Only fresh controller data can establish readiness; cached groups are
        // for display and must not let us enable a proxy with no working core.
        guard let target = await waitForProxyGroups() else { return }
        guard canStartProxyOnCurrentNetwork() else {
            await enforceCampusNetworkPolicy()
            return
        }
        let result = await helper.enableSystemProxy()
        if case .failure(let error) = result {
            lastError = "Enable proxy failed: \(error.localizedDescription)"
            await refreshStatus()
            return
        }

        // A periodic refresh may already be in flight. Read this result directly
        // instead of relying on refreshStatus(), which skips overlapping work.
        switch await helper.getSystemProxyStatus() {
        case .success(let status):
            systemProxyEnabled = status.socksEnabled || status.httpEnabled || status.httpsEnabled
        case .failure(let error):
            lastError = "无法确认系统代理状态：\(error.localizedDescription)"
            return
        }
        await enforceCampusNetworkPolicy()
        guard systemProxyEnabled else { return }

        // Test the group returned by the readiness check, even if a periodic
        // refresh updates the displayed selection while enabling the proxy.
        let testRevision = networkFeedbackRevision
        let testContext = networkFeedbackContext
        let delayResult = await helper.proxyDelay(name: target)
        guard testRevision == networkFeedbackRevision, testContext == networkFeedbackContext else { return }
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

    private func updateProxyGroups(_ groups: [ProxyGroup]) {
        proxyGroups = groups
        if currentMode == "direct" {
            currentGroup = groups.isEmpty ? "" : "DIRECT"
            currentNode = groups.isEmpty ? "" : "DIRECT"
            return
        }
        let selectedGroup: ProxyGroup?
        if currentMode == "global" {
            selectedGroup = groups.first { $0.name == "GLOBAL" }
        } else {
            let ruleGroups = groups.filter { $0.name != "GLOBAL" }
            selectedGroup = ruleGroups.first { $0.name == currentGroup }
                ?? ruleGroups.first { $0.type == "Selector" }
                ?? ruleGroups.first
        }
        currentGroup = selectedGroup?.name ?? ""
        currentNode = selectedGroup?.now ?? ""
    }

    private func waitForProxyGroups() async -> String? {
        var failure = "代理引擎尚未提供可用的节点组。"
        for attempt in 0..<10 {
            guard !Task.isCancelled else { return nil }
            guard canStartProxyOnCurrentNetwork() else {
                await enforceCampusNetworkPolicy()
                return nil
            }
            switch await helper.getProxyGroups() {
            case .success(let groups) where !groups.isEmpty:
                updateProxyGroups(groups)
                if !currentGroup.isEmpty {
                    proxyGroupsError = nil
                    return currentGroup
                }
                failure = "当前模式的节点组尚未就绪。"
            case .success:
                failure = "代理引擎尚未提供可用的节点组。"
            case .failure(let error):
                failure = error.localizedDescription
            }
            if attempt < 9 {
                do {
                    try await Task.sleep(nanoseconds: 300_000_000)
                } catch {
                    return nil
                }
            }
        }
        if case .success(let status) = await helper.getCoreStatus() {
            coreRunning = status.running
            corePID = status.pid
            if !status.running { updateProxyGroups([]) }
        }
        proxyGroupsError = "暂时无法读取节点：\(failure)"
        lastError = "代理引擎尚未就绪，未开启系统代理。请稍后重试，无需重新粘贴订阅。\n\(failure)"
        return nil
    }

    func disableSystemProxy() async {
        invalidateNetworkFeedback()
        let result = await helper.disableSystemProxy()
        if case .failure(let error) = result {
            lastError = "Disable proxy failed: \(error.localizedDescription)"
        }
        await refreshStatus()
    }

    func startCore() async {
        guard canStartProxyOnCurrentNetwork() else { return }
        invalidateNetworkFeedback()
        let result = await helper.startCore()
        if case .failure(let error) = result {
            lastError = "Start core failed: \(error.localizedDescription)"
        }
        await refreshStatus()
    }

    func stopCore() async {
        invalidateNetworkFeedback()
        let result = await helper.stopCore()
        if case .failure(let error) = result {
            lastError = "Stop core failed: \(error.localizedDescription)"
        }
        await refreshStatus()
    }

    func restartCore() async {
        guard !isChangingTUN, canStartProxyOnCurrentNetwork() else { return }
        invalidateNetworkFeedback()
        let result = await helper.restartCore()
        if case .failure(let error) = result {
            lastError = "Restart core failed: \(error.localizedDescription)"
        }
        await refreshStatus()
    }

    func setMode(_ mode: String) async {
        guard !isChangingMode, !isToggleTesting, !isChangingTUN else { return }
        invalidateNetworkFeedback()
        isChangingMode = true
        defer { isChangingMode = false }
        let result = await helper.setMode(mode)
        switch result {
        case .success:
            currentMode = mode
            if mode != "global" { websitePreviousMode = nil }
            toggleTestResult = nil
            testResult = nil
            lastError = nil
            updateProxyGroups(proxyGroups)
        case .failure(let error):
            lastError = "Set mode failed: \(error.localizedDescription)"
        }
        await refreshStatus()
    }

    func uninstallTUNHelper() async {
        guard !tunEnabled, !isChangingTUN, !isBootstrapping else { return }
        isChangingTUN = true
        lastError = nil
        defer { isChangingTUN = false }
        if case .failure(let error) = await helper.uninstallTUNHelper() {
            lastError = error.localizedDescription
        }
    }

    func setTUNEnabled(_ enabled: Bool) async {
        guard !isChangingTUN, !isChangingMode, !isToggleTesting, !isBootstrapping else { return }
        if enabled && !canStartProxyOnCurrentNetwork() { return }
        invalidateNetworkFeedback()
        isChangingTUN = true
        tunChangeTarget = enabled
        let operationID = UUID()
        tunOperationID = operationID
        tunChangeStartedAt = Date()
        tunProgress = TUNProgress(stage: "prepare", message: enabled ? "准备开启 TUN" : "准备关闭 TUN")
        lastError = nil
        switch await helper.setTUNEnabled(enabled, onProgress: { [weak self] progress in
            guard let self, self.tunOperationID == operationID, self.isChangingTUN else { return }
            self.tunProgress = progress
        }) {
        case .success:
            tunProgress = TUNProgress(stage: "verify", message: "确认最终连接状态")
            // Confirm the kernel's result before showing an active tunnel.
            if case .success(let status) = await helper.getModeStatus() {
                tunEnabled = status.tunEnabled == true
                tunDevice = status.tunDevice ?? ""
                currentMode = status.mode
                if enabled && !tunEnabled { lastError = "内核尚未确认 TUN 已开启，请查看日志。" }
            } else {
                lastError = "无法确认 TUN 状态，请查看日志。"
            }
        case .failure(let error):
            lastError = error.localizedDescription
        }
        // The lifecycle command and confirmation are complete. A slow node or
        // profile refresh must not keep the TUN switch locked afterwards.
        isChangingTUN = false
        tunChangeTarget = nil
        tunProgress = nil
        tunChangeStartedAt = nil
        tunOperationID = nil
        await refreshStatus()
        await enforceCampusNetworkPolicy()
    }

    func selectProxy(group: String, proxy: String) async {
        invalidateNetworkFeedback()
        selectionInProgress = true
        nodeBaseline = nil
        nodeChangeNotice = nil
        defer { selectionInProgress = false }
        lastError = nil
        let result = await helper.selectProxy(group: group, proxy: proxy)
        if case .failure(let error) = result {
            lastError = "Select proxy failed: \(error.localizedDescription)"
        }
        await refreshStatus()
    }

    // Resolve nested automatic groups for display without inventing a leaf.
    var selectedLeafNode: String {
        var name = currentNode
        var visited = Set<String>()
        while let group = proxyGroups.first(where: { $0.name == name }) {
            guard visited.insert(name).inserted, !group.now.isEmpty else { return currentNode }
            name = group.now
        }
        return name
    }

    private func recordNodeChanges() {
        guard coreRunning, !isChangingMode, !isChangingTUN, !isBootstrapping, !selectionInProgress, proxyGroupsError == nil else {
            nodeBaseline = nil
            return
        }
        let context = "\(corePID)|\(activeProfileID ?? "")|\(currentMode)"
        defer { nodeBaseline = (context, proxyGroups) }
        guard let baseline = nodeBaseline, baseline.context == context else {
            nodeChangeNotice = nil
            return
        }
        for group in proxyGroups where ["URLTest", "Fallback"].contains(group.type) {
            guard let previous = baseline.groups.first(where: { $0.name == group.name && $0.type == group.type }),
                  !previous.now.isEmpty, !group.now.isEmpty, previous.now != group.now else { continue }
            nodeChangeNotice = NodeChangeNotice(message: "「\(ProxyDisplay.name(group.name))」已从 \(ProxyDisplay.name(previous.now)) 切换到 \(ProxyDisplay.name(group.now))", observedAt: Date())
            break
        }
    }

    func checkWebsite(_ input: String) async {
        guard !isCheckingWebsite else { return }
        let trimmed = input.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        lastWebsiteInput = trimmed
        websiteContext = "\(activeProfileID ?? "")|\(currentMode)|\(corePID)"
        isCheckingWebsite = true
        websiteResult = nil
        websiteCheckError = nil
        defer { isCheckingWebsite = false }
        switch await helper.checkWebsite(trimmed) {
        case .success(let result): websiteResult = result
        case .failure(let error): websiteCheckError = error.localizedDescription
        }
    }

    var websiteResultIsCurrent: Bool {
        websiteContext == "\(activeProfileID ?? "")|\(currentMode)|\(corePID)"
    }

    var websiteRecoveryGroups: [ProxyGroup] {
        guard websiteResultIsCurrent, let result = websiteResult, result.route == "proxy" else { return [] }
        return proxyGroups.filter { $0.type == "Selector" && result.chains.contains($0.name) }
    }

    func changeNodeAndRecheck(group: String, proxy: String) async {
        guard !isRecoveringWebsite, !isCheckingWebsite, !isChangingTUN, !isChangingMode,
              let result = websiteResult, result.route == "proxy", result.connectionFailed,
              websiteRecoveryGroups.contains(where: { $0.name == group && $0.all.contains(proxy) }) else { return }
        isRecoveringWebsite = true
        defer { isRecoveringWebsite = false }
        await selectProxy(group: group, proxy: proxy)
        guard lastError == nil else { websiteCheckError = lastError; return }
        await checkWebsite(lastWebsiteInput)
    }

    func checkWebsiteUsingGlobal() async {
        guard !isRecoveringWebsite, !isCheckingWebsite, !isChangingTUN, !isChangingMode, !isToggleTesting,
              websiteResultIsCurrent, let result = websiteResult, result.route == "direct", result.connectionFailed,
              currentMode == result.mode, currentMode != "global" else { return }
        isRecoveringWebsite = true
        defer { isRecoveringWebsite = false }
        let previous = currentMode
        await setMode("global")
        guard currentMode == "global", lastError == nil else { websiteCheckError = lastError; return }
        websitePreviousMode = previous
        await checkWebsite(lastWebsiteInput)
    }

    func restoreWebsiteMode() async {
        guard let previous = websitePreviousMode, !isRecoveringWebsite, !isCheckingWebsite, !isChangingMode else { return }
        isRecoveringWebsite = true
        defer { isRecoveringWebsite = false }
        await setMode(previous)
        if currentMode == previous { websitePreviousMode = nil }
        else { websiteCheckError = lastError }
    }

    func testConnection() async {
        guard !isChangingTUN, !isChangingMode, !isBootstrapping else { return }
        let revision = networkFeedbackRevision
        let context = networkFeedbackContext
        let result = await helper.testConnection()
        guard revision == networkFeedbackRevision, context == networkFeedbackContext else { return }
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
        guard !isTestingDelays, !isChangingTUN, !isChangingMode, !isBootstrapping else { return }
        isTestingDelays = true
        defer { isTestingDelays = false }
        let revision = networkFeedbackRevision
        let context = networkFeedbackContext

        let result = await helper.testProxyDelays()
        guard revision == networkFeedbackRevision, context == networkFeedbackContext else { return }
        switch result {
        case .success(let delays):
            proxyDelays = delays
            lastError = nil
        case .failure(let error):
            lastError = "Delay test failed: \(error.localizedDescription)"
        }
    }

    func updateSubscription() async {
        invalidateNetworkFeedback()
        let result = await helper.updateSubscription()
        if case .failure(let error) = result {
            lastError = "Update failed: \(error.localizedDescription)"
        }
    }

    private var canAutomaticallyCheckForUpdate: Bool {
        guard coreRunning, systemProxyEnabled || tunEnabled,
              !isBootstrapping, !isChangingTUN, !isChangingMode, !isToggleTesting,
              availableUpdateVersion == nil else { return false }
        if let history = defaults.dictionary(forKey: "updateCheckHistory.\(appVersion)"),
           let checkedAt = history["checkedAt"] as? Date,
           let succeeded = history["succeeded"] as? Bool {
            let elapsed = dateProvider().timeIntervalSince(checkedAt)
            let interval: TimeInterval = succeeded ? 24 * 60 * 60 : 30 * 60
            if elapsed >= 0 && elapsed < interval { return false }
        }
        return true
    }

    private func scheduleAutomaticUpdateCheck() {
        guard automaticUpdateCheckTask == nil, canAutomaticallyCheckForUpdate,
              !isCheckingForUpdate, !isInstallingUpdate, !showRestartAlert else { return }
        // Do not hold up status refreshes or opening the menu while GitHub responds.
        automaticUpdateCheckTask = Task { [weak self] in
            guard let self else { return }
            defer { self.automaticUpdateCheckTask = nil }
            await self.checkForUpdate(automatically: true)
        }
    }

    func checkForUpdate(automatically: Bool = false) async {
        guard !isInstallingUpdate, !showRestartAlert else { return }
        if isCheckingForUpdate {
            // A user who opens "Check for updates" can review the result of
            // the existing request, including errors, without another request.
            if !automatically {
                isCheckingForUpdateAutomatically = false
                showingCurrentReleaseNotes = false
            }
            return
        }
        guard !Task.isCancelled, !automatically || canAutomaticallyCheckForUpdate else { return }
        isCheckingForUpdate = true
        isCheckingForUpdateAutomatically = automatically
        if !automatically {
            showingCurrentReleaseNotes = false
            updateFailed = false
            updateStatus = "正在检查更新..."
            updateErrorDetails = nil
            updateProgress = nil
            availableUpdateVersion = nil
            updateReleaseVersion = nil
            updateReleaseNotes = ""
            updateReleaseURL = nil
            updatePublishedAt = nil
        }
        defer {
            isCheckingForUpdate = false
            isCheckingForUpdateAutomatically = false
        }

        let result = await helper.checkForUpdate()
        guard !Task.isCancelled else { return }
        let succeeded: Bool
        if case .success(let event) = result { succeeded = event.stage == "done" }
        else { succeeded = false }
        defaults.set(["checkedAt": dateProvider(), "succeeded": succeeded],
                     forKey: "updateCheckHistory.\(appVersion)")
        if succeeded, case .success(let event) = result {
            if event.newVersion != nil, let data = try? JSONEncoder().encode(event) {
                defaults.set(data, forKey: "availableUpdate.\(appVersion)")
            } else {
                defaults.removeObject(forKey: "availableUpdate.\(appVersion)")
            }
        }
        let background = isCheckingForUpdateAutomatically
        if background {
            // Background failures and "already current" results stay silent.
            // Preserve unrelated errors and any changelog the user is reading.
            guard case .success(let event) = result, succeeded, event.newVersion != nil else { return }
            updateFailed = false
            updateErrorDetails = nil
        }
        switch result {
        case .success(let event):
            updateStatus = event.message
            updateProgress = event.progress
            availableUpdateVersion = event.newVersion
            updateReleaseVersion = event.releaseVersion ?? event.newVersion
            updateReleaseNotes = event.releaseNotes ?? ""
            updateReleaseURL = event.releaseURL
            updatePublishedAt = event.publishedAt
            updateFailed = event.stage == "error"
            updateErrorDetails = event.stage == "error" ? event.details : nil
            if !background { lastError = event.stage == "error" ? event.message : nil }
        case .failure(let error):
            updateStatus = "检查更新失败：\(error.localizedDescription)"
            updateFailed = true
            lastError = "检查更新失败：\(error.localizedDescription)"
        }
        if updateFailed {
            // A failed remote check must not hide the changelog shipped with
            // this app or imply that the installed version is up to date.
            showingCurrentReleaseNotes = true
            availableUpdateVersion = nil
        }
    }

    func installUpdate() async {
        guard !isCheckingForUpdate, !isInstallingUpdate, !showRestartAlert,
              let version = availableUpdateVersion else { return }
        showingCurrentReleaseNotes = false
        isInstallingUpdate = true
        updateFailed = false
        updateStatus = "正在准备更新..."
        updateErrorDetails = nil
        updateProgress = nil
        lastError = nil
        defer { isInstallingUpdate = false }

        let stream = await helper.installUpdate(expectedVersion: version)
        var finished = false
        for await event in stream {
            updateStatus = event.message
            updateProgress = event.progress
            availableUpdateVersion = event.stage == "done" ? nil : (event.newVersion ?? availableUpdateVersion)
            if event.stage == "done" {
                finished = true
                showRestartAlert = event.progress == 100
                defaults.removeObject(forKey: "availableUpdate.\(appVersion)")
            }
            if event.stage == "error" {
                finished = true
                updateFailed = true
                updateErrorDetails = event.details
                if event.recheck == true {
                    availableUpdateVersion = nil
                    defaults.removeObject(forKey: "availableUpdate.\(appVersion)")
                }
            }
        }
        if !finished {
            updateFailed = true
            updateStatus = "更新已中止，可以重试。"
        }
    }

    var currentAppVersion: String {
        appVersion
    }

    var currentReleaseNotes: String {
        if let releaseNotesOverride { return releaseNotesOverride }
        guard let url = Bundle.main.url(forResource: "v\(currentAppVersion)", withExtension: "md", subdirectory: "releases"),
              let text = try? String(contentsOf: url, encoding: .utf8) else { return "" }
        return text
    }

    func cancelUpdate() async {
        await helper.cancelUpdate()
    }

    private var networkFeedbackContext: String {
        let selections = proxyGroups.sorted { $0.name < $1.name }.map { "\($0.name)=\($0.now)" }
        return ([currentWiFiSSID ?? "", String(coreRunning), String(corePID), String(systemProxyEnabled),
                 String(tunEnabled), currentMode] + selections).joined(separator: "\n")
    }

    private func invalidateNetworkFeedback() {
        networkFeedbackRevision &+= 1
        if diagnoseReport != nil || isDiagnosing { diagnosisNeedsRefresh = true }
        diagnoseReport = nil
        testResult = nil
        toggleTestResult = nil
    }

    func runDiagnose() async {
        guard !isDiagnosing, !isChangingTUN, !isChangingMode, !isBootstrapping else { return }
        isDiagnosing = true
        defer { isDiagnosing = false }
        let revision = networkFeedbackRevision
        let context = networkFeedbackContext
        let result = await helper.runDiagnose()
        // A result from before a switch must not erase its failure or certify
        // the new route. Refreshes also invalidate results on observed changes.
        guard revision == networkFeedbackRevision, context == networkFeedbackContext else {
            diagnosisNeedsRefresh = true
            return
        }
        switch result {
        case .success(let report):
            diagnoseReport = report
            diagnosisNeedsRefresh = false
            lastError = nil
        case .failure(let error):
            diagnoseReport = nil
            lastError = "Diagnose failed: \(error.localizedDescription)"
        }
    }

    func loadProfiles() async {
        let result = await helper.getProfiles()
        switch result {
        case .success(let p):
            let nextID = p.first(where: { $0.active })?.id
            if activeProfileID != nextID { trafficRefreshError = nil }
            profiles = p
            activeProfileID = nextID
        case .failure:
            // Listing saved profiles does not depend on network availability.
            // A transient helper error must not erase the visible saved list.
            break
        }
    }

    var activeProfile: Profile? { profiles.first { $0.id == activeProfileID } }

    func refreshSubscriptionTraffic(force: Bool = false) async {
        guard !isRefreshingTraffic, let profile = activeProfile else { return }
        let now = dateProvider()
        if !force {
            if let attempt = trafficAttempts[profile.id], now.timeIntervalSince(attempt) < 5 * 60 { return }
            if let checkedAt = TrafficPresentation(profile: profile).checkedAt,
               now.timeIntervalSince(checkedAt) >= 0, now.timeIntervalSince(checkedAt) < 30 * 60 { return }
        }
        isRefreshingTraffic = true
        trafficAttempts[profile.id] = now
        defer {
            isRefreshingTraffic = false
            if activeProfileID != profile.id {
                Task { await refreshSubscriptionTraffic() }
            }
        }
        let result = await helper.refreshSubscriptionTraffic(profileID: profile.id)
        guard activeProfileID == profile.id else { return }
        switch result {
        case .success(let snapshot):
            guard snapshot.profileID == profile.id,
                  let index = profiles.firstIndex(where: { $0.id == profile.id }) else { return }
            profiles[index].traffic = snapshot.traffic
            profiles[index].trafficCheckedAt = snapshot.checkedAt
            trafficRefreshError = nil
        case .failure:
            trafficRefreshError = "流量刷新失败，可稍后重试；已保留上次数据。"
        }
    }

    func activateProfile(id: String) async {
        guard !isChangingTUN, canStartProxyOnCurrentNetwork() else { return }
        invalidateNetworkFeedback()
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
        guard !isChangingTUN else { return }
        guard canStartProxyOnCurrentNetwork() else {
            bootstrapStatus = "校园网保护已阻止启动代理。"
            return
        }
        invalidateNetworkFeedback()
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
