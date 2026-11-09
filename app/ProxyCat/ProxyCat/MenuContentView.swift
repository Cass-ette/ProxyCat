import SwiftUI

struct MenuContentView: View {
    @ObservedObject var viewModel: StatusViewModel
    let onShowUpdates: (Bool) -> Void
    @State private var subscriptionURL: String = ""
    @State private var isBootstrapping: Bool = false
    @State private var campusSSIDInput = ""
    @State private var showingModeHelp = false
    @State private var showingDiagnosticDetails = false
    @AppStorage("nodesExpanded") private var nodesExpanded = false
    @AppStorage("connectionDetailsExpanded") private var connectionDetailsExpanded = false
    @AppStorage("profilesExpanded") private var profilesExpanded = false
    @AppStorage("campusSettingsExpanded") private var campusSettingsExpanded = false
    @AppStorage("moreExpanded") private var moreExpanded = false
    @AppStorage("nodeGroupExpansion") private var nodeGroupExpansion = "{}"
    @State private var defaultExpandedGroups: [String: String] = [:]

    var body: some View {
        VStack(spacing: 0) {
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    headerSection
                    controlsSection

                    if hasFeedback {
                        feedbackSection
                    }
                    campusNotices
                    updateActivitySection

                    Divider()
                    nodesSection
                    Divider()
                    configurationSection
                    Divider()
                    campusSection
                    Divider()
                    moreSection
                }
            }
            Divider()
            quitButton
        }
        .background(VisualEffectView(material: .menu, blendingMode: .behindWindow))
        .cornerRadius(8)
        .shadow(radius: 4)
        .frame(width: 320)
        .frame(maxHeight: 640)
        .onAppear(perform: captureDefaultExpandedGroup)
        .onChange(of: viewModel.currentGroup) { _ in captureDefaultExpandedGroup() }
        .onChange(of: viewModel.activeProfileID) { _ in captureDefaultExpandedGroup() }
        .onChange(of: viewModel.proxyGroups.map(\.name)) { _ in captureDefaultExpandedGroup() }
    }

    private var headerSection: some View {
        let state = MenuBarProxyState(running: viewModel.coreRunning, enabled: viewModel.systemProxyEnabled, mode: viewModel.currentMode, tunEnabled: viewModel.tunEnabled)
        return HStack {
            Image(systemName: state.symbol)
                .foregroundColor(state.color.map { Color(nsColor: $0) } ?? .secondary)
            Text(state.title)
                .font(.headline)
            Spacer()
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
        .help(state.description)
    }

    private var profileSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            ForEach(viewModel.profiles) { p in
                HStack {
                    Image(systemName: viewModel.activeProfileID == p.id ? "checkmark.circle.fill" : "circle")
                        .frame(width: 20)
                        .foregroundColor(viewModel.activeProfileID == p.id ? .accentColor : .secondary)
                    Button(action: {
                        Task { await viewModel.activateProfile(id: p.id) }
                    }) {
                        Text(p.name)
                            .lineLimit(1)
                    }
                    .buttonStyle(PlainButtonStyle())
                    Spacer()
                    Button(action: {
                        showDeleteConfirmation(for: p)
                    }) {
                        Image(systemName: "xmark")
                            .font(.system(size: 10, weight: .bold))
                            .foregroundColor(.secondary)
                    }
                    .buttonStyle(PlainButtonStyle())
                    .disabled(p.active)
                    .help(p.active ? "无法删除正在使用的配置" : "")
                }
                .font(.system(size: 12))
                .padding(.horizontal, 16)
                .padding(.vertical, 3)
                .contentShape(Rectangle())
            }
        }
        .padding(.bottom, 8)
    }

    private var configurationSection: some View {
        let importing = isBootstrapping || viewModel.isBootstrapping
        let needsConfiguration = viewModel.profiles.isEmpty
        let expanded = profilesExpanded || needsConfiguration || importing
        let profileName = viewModel.profiles.first { $0.id == viewModel.activeProfileID }?.name ?? "未选择配置"
        return VStack(alignment: .leading, spacing: 0) {
            MenuDisclosureRow(
                "配置", summary: needsConfiguration ? "添加订阅以开始" : profileName,
                isExpanded: expanded, canCollapse: !needsConfiguration && !importing
            ) { profilesExpanded.toggle() }
            if expanded {
                if !needsConfiguration { profileSection }
                quickStartSection
                if !needsConfiguration {
                    MenuButton("更新当前订阅", icon: "arrow.clockwise") {
                        Task { await viewModel.updateSubscription() }
                    }
                    .disabled(importing)
                }
            }
        }
        .padding(.bottom, expanded ? 8 : 0)
    }

    private func showDeleteConfirmation(for profile: Profile) {
        let alert = NSAlert()
        alert.messageText = "删除配置"
        alert.informativeText = "确定要删除「\(profile.name)」吗？此操作无法撤销。"
        alert.addButton(withTitle: "取消")
        alert.addButton(withTitle: "删除")
        alert.alertStyle = .critical
        if alert.runModal() == .alertSecondButtonReturn {
            Task { await viewModel.deleteProfile(id: profile.id) }
        }
    }

    private var quickStartSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(viewModel.profiles.isEmpty ? "快速开始" : "添加订阅")
                .font(.caption)
                .foregroundColor(.secondary)
                .padding(.horizontal, 16)
                .padding(.top, 8)

            // 订阅链接输入框
            TextField("粘贴订阅链接", text: $subscriptionURL)
                .textFieldStyle(RoundedBorderTextFieldStyle())
                .font(.system(size: 12))
                .padding(.horizontal, 16)
                .disableAutocorrection(true)

            // 一键启动按钮
            Button(action: {
                guard !subscriptionURL.isEmpty else { return }
                isBootstrapping = true
                Task {
                    await viewModel.bootstrap(subscriptionURL: subscriptionURL)
                    isBootstrapping = false
                }
            }) {
                HStack {
                    if isBootstrapping {
                        ProgressView()
                            .scaleEffect(0.8)
                            .frame(width: 20)
                    } else {
                        Image(systemName: "play.fill")
                            .frame(width: 20)
                    }
                    Text(isBootstrapping ? "启动中..." : "导入并启动")
                    Spacer()
                }
                .frame(maxWidth: .infinity)
                .contentShape(Rectangle())
            }
            .buttonStyle(PlainButtonStyle())
            .padding(.horizontal, 16)
            .padding(.vertical, 8)
            .background(Color.accentColor.opacity(0.15))
            .cornerRadius(6)
            .padding(.horizontal, 16)
            .disabled(isBootstrapping || subscriptionURL.isEmpty)

            if isBootstrapping {
                Text(viewModel.bootstrapStatus)
                    .font(.system(size: 11))
                    .foregroundColor(.secondary)
                    .padding(.horizontal, 16)
                    .lineLimit(1)
            }
        }
        .padding(.bottom, 8)
    }

    private var statusSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            StatusRow(label: "代理引擎", value: viewModel.coreRunning ? "运行中" : "已停止")
            if viewModel.coreRunning && viewModel.corePID > 0 {
                StatusRow(label: "进程", value: String(viewModel.corePID))
            }
            StatusRow(label: "系统代理", value: viewModel.systemProxyEnabled ? "已开启" : "已关闭")
            StatusRow(label: "TUN", value: viewModel.tunEnabled ? "已开启" : "已关闭")
            StatusRow(label: "代理模式", value: modeLabel(viewModel.currentMode))
            if !viewModel.tunDevice.isEmpty {
                StatusRow(label: "TUN 设备", value: viewModel.tunDevice)
            }
        }
        .padding(.bottom, 8)
    }

    private var controlsSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("系统代理")
                Spacer()
                Toggle("系统代理", isOn: Binding(
                    get: { viewModel.systemProxyEnabled },
                    set: { enabled in
                        Task {
                            if enabled { await viewModel.enableSystemProxy() }
                            else { await viewModel.disableSystemProxy() }
                        }
                    }
                ))
                .labelsHidden()
                .toggleStyle(SwitchToggleStyle())
                .disabled(!viewModel.systemProxyEnabled && (viewModel.isToggleTesting || viewModel.isChangingMode || viewModel.isChangingTUN || viewModel.isBootstrapping))
            }
            .font(.system(size: 12))
            .padding(.horizontal, 16)

            modeControls

            HStack {
                Text("TUN")
                Spacer()
                Toggle("TUN", isOn: Binding(
                    get: { viewModel.tunEnabled },
                    set: { enabled in Task { await viewModel.setTUNEnabled(enabled) } }
                ))
                .labelsHidden()
                .toggleStyle(SwitchToggleStyle())
                .disabled(viewModel.isChangingTUN || viewModel.isChangingMode || viewModel.isToggleTesting || viewModel.isBootstrapping)
            }
            .font(.system(size: 12))
            .padding(.horizontal, 16)
            .help("接管应用流量，仍按所选模式分流；开启需管理员授权。")

            if viewModel.isChangingTUN {
                activityLabel("正在切换 TUN，请完成系统授权…")
            }
            if viewModel.isChangingMode {
                activityLabel("正在切换代理模式…")
            }
            if viewModel.isToggleTesting {
                activityLabel("正在检查连接…")
            } else if viewModel.systemProxyEnabled, let result = viewModel.toggleTestResult {
                HStack {
                    Text(viewModel.currentMode == "direct" ? "直连测试" : "连接测试")
                        .foregroundColor(.secondary)
                    Spacer()
                    Text(toggleTestLabel(result))
                        .foregroundColor(toggleTestColor(result))
                        .lineLimit(2)
                }
                .font(.system(size: 11))
                .padding(.horizontal, 16)
                if result.error != nil { toggleFailureBanner }
            }

            MenuDisclosureRow("连接详情", summary: viewModel.coreRunning ? "引擎运行中" : "引擎已停止", isExpanded: connectionDetailsExpanded) {
                connectionDetailsExpanded.toggle()
            }
            if connectionDetailsExpanded { statusSection }
        }
        .padding(.bottom, 4)
    }

    private var nodesSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            MenuDisclosureRow("节点", summary: currentNodeSummary, isExpanded: nodesExpanded) {
                nodesExpanded.toggle()
            }
            if viewModel.isTestingDelays { activityLabel("正在测试节点延迟…") }
            if nodesExpanded {
                if !viewModel.proxyGroups.isEmpty {
                    delayControls
                    proxyGroupControls
                } else {
                    Text(viewModel.coreRunning
                         ? "节点尚未就绪，正在等待代理引擎。"
                         : "代理引擎已停止，开启系统代理将使用已保存的配置启动。")
                        .font(.system(size: 12))
                        .foregroundColor(.secondary)
                        .padding(.horizontal, 16)
                }
            }

            if let error = viewModel.proxyGroupsError {
                Text(error)
                    .font(.system(size: 12))
                    .foregroundColor(.orange)
                    .padding(.horizontal, 16)
            }
        }
        .padding(.bottom, nodesExpanded || viewModel.proxyGroupsError != nil || viewModel.isTestingDelays ? 8 : 0)
    }

    private var currentNodeSummary: String {
        if viewModel.currentMode == "direct" { return "直连模式" }
        guard !viewModel.currentNode.isEmpty else { return "未选择" }
        let name = displayName(for: viewModel.currentNode)
        let delay = delayLabel(for: viewModel.currentNode)
        return delay.isEmpty ? name : "\(name) · \(delay)"
    }

    private func activityLabel(_ title: String) -> some View {
        HStack(spacing: 6) {
            ProgressView().scaleEffect(0.6).frame(width: 14, height: 14)
            Text(title).fixedSize(horizontal: false, vertical: true)
        }
        .font(.system(size: 11))
        .foregroundColor(.secondary)
        .padding(.horizontal, 16)
    }

    private var campusSection: some View {
        let summary = !viewModel.campusProtectionEnabled ? "已关闭"
            : viewModel.protectedWiFiSSIDs.isEmpty ? "已开启 · 未设置 Wi-Fi"
            : "已开启 · \(viewModel.campusNetworkAction.title)"
        return VStack(alignment: .leading, spacing: 0) {
            MenuDisclosureRow("校园网保护", summary: summary, isExpanded: campusSettingsExpanded) {
                campusSettingsExpanded.toggle()
            }
            if campusSettingsExpanded { campusNetworkSection }
        }
    }

    private var campusNotices: some View {
        VStack(alignment: .leading, spacing: 6) {
            if let notice = viewModel.campusProtectionNotice {
                Text(notice)
                    .font(.system(size: 11))
                    .foregroundColor(.orange)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.horizontal, 16)
                    .padding(.bottom, 6)
            }
            if viewModel.currentWiFiSSID == nil && viewModel.campusProtectionEnabled && !viewModel.protectedWiFiSSIDs.isEmpty {
                Text("无法识别当前 Wi-Fi，校园网保护尚未生效。")
                    .font(.system(size: 11))
                    .foregroundColor(.orange)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.horizontal, 16)
                wifiAccessButton
            }
        }
    }

    private var wifiAccessButton: some View {
        MenuButton("允许读取 Wi-Fi 名称", icon: "location.circle") {
            viewModel.requestWiFiAccess()
        }
        .font(.system(size: 11))
    }

    private var campusNetworkSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Toggle("连接校园网时保护", isOn: $viewModel.campusProtectionEnabled)
                .font(.system(size: 12))
                .toggleStyle(SwitchToggleStyle())
                .padding(.horizontal, 16)

            StatusRow(
                label: "当前 Wi-Fi",
                value: viewModel.currentWiFiSSID ?? "未连接或无权读取"
            )

            Button(action: viewModel.addCurrentWiFiToCampusNetworks) {
                HStack {
                    Image(systemName: "plus.circle")
                        .frame(width: 20)
                    Text("将当前 Wi-Fi 标记为校园网")
                    Spacer()
                }
                .font(.system(size: 12))
                .contentShape(Rectangle())
            }
            .buttonStyle(PlainButtonStyle())
            .padding(.horizontal, 16)
            .disabled(viewModel.currentWiFiSSID == nil || viewModel.protectedWiFiSSIDs.contains(viewModel.currentWiFiSSID ?? ""))

            HStack(spacing: 6) {
                TextField("手动输入校园网 SSID", text: $campusSSIDInput)
                    .textFieldStyle(RoundedBorderTextFieldStyle())
                    .disableAutocorrection(true)
                Button(action: {
                    viewModel.addCampusNetwork(campusSSIDInput)
                    campusSSIDInput = ""
                }) {
                    Image(systemName: "plus")
                        .frame(width: 24, height: 24)
                }
                .buttonStyle(PlainButtonStyle())
                .disabled(campusSSIDInput.isEmpty)
                .help("添加校园网 SSID")
            }
            .font(.system(size: 12))
            .padding(.horizontal, 16)

            if !viewModel.protectedWiFiSSIDs.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    ForEach(viewModel.protectedWiFiSSIDs, id: \.self) { ssid in
                        HStack(spacing: 6) {
                            Text(ssid)
                                .font(.system(size: 11))
                                .lineLimit(1)
                            Spacer(minLength: 4)
                            Button(action: { viewModel.removeCampusNetwork(ssid) }) {
                                Image(systemName: "xmark.circle")
                                    .foregroundColor(.secondary)
                            }
                            .buttonStyle(PlainButtonStyle())
                            .help("移除校园网")
                        }
                        .padding(.horizontal, 16)
                    }
                }
            }

            HStack(spacing: 8) {
                Text("匹配时")
                    .font(.system(size: 12))
                    .foregroundColor(.secondary)
                Picker("校园网处理方式", selection: $viewModel.campusNetworkAction) {
                    ForEach(CampusNetworkAction.allCases) { action in
                        Text(action.title).tag(action)
                    }
                }
                .labelsHidden()
                .pickerStyle(SegmentedPickerStyle())
            }
            .padding(.horizontal, 16)
            .disabled(!viewModel.campusProtectionEnabled)

            if viewModel.currentWiFiSSID == nil && (!viewModel.campusProtectionEnabled || viewModel.protectedWiFiSSIDs.isEmpty) {
                wifiAccessButton
            }
        }
        .padding(.bottom, 8)
    }

    private var modeControls: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                ForEach([("rule", "规则"), ("global", "全局"), ("direct", "直连")], id: \.0) { mode, title in
                    Button(action: {
                        Task { await viewModel.setMode(mode) }
                    }) {
                        Text(title)
                            .font(.system(size: 12, weight: viewModel.currentMode == mode ? .semibold : .regular))
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 6)
                            .background(viewModel.currentMode == mode ? Color.accentColor.opacity(0.2) : Color.secondary.opacity(0.08))
                            .cornerRadius(6)
                    }
                    .buttonStyle(PlainButtonStyle())
                    .disabled(viewModel.isChangingMode || viewModel.isToggleTesting || viewModel.isChangingTUN)
                }

                Button(action: { showingModeHelp.toggle() }) {
                    Image(systemName: "questionmark.circle")
                        .font(.system(size: 14))
                        .foregroundColor(.secondary)
                        .frame(width: 28, height: 28)
                        .contentShape(Rectangle())
                }
                .buttonStyle(PlainButtonStyle())
                .help(showingModeHelp ? "收起模式说明" : "查看模式区别")
                .accessibilityLabel(showingModeHelp ? "收起模式说明" : "查看模式区别")
                .accessibilityValue(showingModeHelp ? "已展开" : "已收起")
            }

            if showingModeHelp {
                VStack(alignment: .leading, spacing: 5) {
                    Text("规则：按配置规则选择直连或代理。")
                    Text("全局：统一使用全局分组选定的出口。")
                    Text("直连：直接访问，不经过代理节点。")
                    Text("仅影响进入 ProxyCat 的流量。")
                        .foregroundColor(.secondary)
                    Text("TUN：接管应用流量，可搭配以上任一模式；开启需管理员授权。")
                }
                .font(.system(size: 11))
                .fixedSize(horizontal: false, vertical: true)
                .padding(10)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(Color.secondary.opacity(0.08))
                .cornerRadius(6)
            }
        }
        .padding(.horizontal, 16)
    }

    private var delayControls: some View {
        Button(action: {
            Task { await viewModel.testProxyDelays() }
        }) {
            HStack {
                if viewModel.isTestingDelays {
                    ProgressView()
                        .scaleEffect(0.7)
                        .frame(width: 20)
                } else {
                    Image(systemName: "speedometer")
                        .frame(width: 20)
                }
                Text(viewModel.isTestingDelays ? "测延迟中..." : "测延迟")
                Spacer()
            }
            .font(.system(size: 12, weight: .medium))
        }
        .buttonStyle(PlainButtonStyle())
        .padding(.horizontal, 16)
        .padding(.vertical, 6)
        .background(Color.secondary.opacity(0.08))
        .cornerRadius(6)
        .padding(.horizontal, 16)
        .disabled(viewModel.isTestingDelays)
    }

    private var proxyGroupControls: some View {
        VStack(alignment: .leading, spacing: 6) {
            ForEach(viewModel.proxyGroups) { group in
                VStack(alignment: .leading, spacing: 4) {
                    MenuDisclosureRow(group.name, summary: group.now.isEmpty ? "未选择" : displayName(for: group.now), isExpanded: isGroupExpanded(group)) {
                        setGroupExpanded(group, expanded: !isGroupExpanded(group))
                    }

                    if isGroupExpanded(group) {
                        ForEach(group.all, id: \.self) { proxy in
                            Button(action: {
                                Task { await viewModel.selectProxy(group: group.name, proxy: proxy) }
                            }) {
                                HStack {
                                    Image(systemName: group.now == proxy ? "checkmark.circle.fill" : "circle")
                                        .frame(width: 20)
                                        .foregroundColor(group.now == proxy ? .accentColor : .secondary)
                                    Text(displayName(for: proxy))
                                        .lineLimit(1)
                                    Spacer()
                                    Text(delayLabel(for: proxy))
                                        .font(.system(size: 11, design: .monospaced))
                                        .foregroundColor(delayColor(for: proxy))
                                }
                                .font(.system(size: 12))
                                .contentShape(Rectangle())
                            }
                            .buttonStyle(PlainButtonStyle())
                            .padding(.horizontal, 16)
                            .padding(.vertical, 3)
                        }
                    }
                }
                .padding(.vertical, 4)
            }
        }
    }

    private var profileExpansionKey: String { viewModel.activeProfileID ?? "_default" }

    private func captureDefaultExpandedGroup() {
        guard defaultExpandedGroups[profileExpansionKey] == nil,
              viewModel.proxyGroups.contains(where: { $0.name == viewModel.currentGroup }) else { return }
        // Snapshot once per profile/menu opening; periodic status refreshes only update summaries.
        defaultExpandedGroups[profileExpansionKey] = viewModel.currentGroup
    }

    private var savedGroupExpansion: [String: [String: Bool]] {
        (try? JSONDecoder().decode([String: [String: Bool]].self, from: Data(nodeGroupExpansion.utf8))) ?? [:]
    }

    private func isGroupExpanded(_ group: ProxyGroup) -> Bool {
        if let saved = savedGroupExpansion[profileExpansionKey]?[group.name] { return saved }
        return group.name == (defaultExpandedGroups[profileExpansionKey] ?? viewModel.currentGroup)
    }

    private func setGroupExpanded(_ group: ProxyGroup, expanded: Bool) {
        var saved = savedGroupExpansion
        saved[profileExpansionKey, default: [:]][group.name] = expanded
        if let data = try? JSONEncoder().encode(saved), let value = String(data: data, encoding: .utf8) {
            nodeGroupExpansion = value
        }
    }

    private func displayName(for proxy: String) -> String {
        proxy.replacingOccurrences(of: "\u{1F1F9}\u{1F1FC}", with: "\u{1F1E8}\u{1F1F3}")
    }

    private func delayLabel(for proxy: String) -> String {
        guard let result = viewModel.proxyDelays[proxy] else {
            return ""
        }
        if result.error != nil {
            return "超时"
        }
        return "\(result.delay)ms"
    }

    private func delayColor(for proxy: String) -> Color {
        guard let result = viewModel.proxyDelays[proxy], result.error == nil else {
            return .secondary
        }
        if result.delay <= 100 {
            return .green
        }
        if result.delay <= 250 {
            return .orange
        }
        return .red
    }

    private var hasFeedback: Bool {
        viewModel.lastError != nil || viewModel.testResult != nil || viewModel.diagnoseReport != nil
    }

    private var feedbackSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("结果")
                .font(.caption)
                .foregroundColor(.secondary)
                .padding(.horizontal, 16)
                .padding(.top, 8)

            if let testResult = viewModel.testResult {
                StatusRow(
                    label: "连接测试",
                    value: "Google: \(testResult.googleOK ? "✓" : "✗"), GitHub: \(testResult.githubOK ? "✓" : "✗")"
                )
                if let error = testResult.error, !error.isEmpty {
                    Text(error)
                        .font(.system(size: 12))
                        .foregroundColor(.red)
                        .lineLimit(3)
                        .padding(.horizontal, 16)
                }
            }

            if let report = viewModel.diagnoseReport {
                if report.checks.contains(where: { $0.status.lowercased() != "pass" }) {
                    MenuDisclosureRow("诊断", summary: diagnoseSummary(report), isExpanded: showingDiagnosticDetails) {
                        showingDiagnosticDetails.toggle()
                    }
                    if showingDiagnosticDetails { diagnoseDetails(report) }
                } else {
                    StatusRow(label: "诊断", value: diagnoseSummary(report))
                }
            }

            if let lastError = viewModel.lastError {
                Text(lastError)
                    .font(.system(size: 12))
                    .foregroundColor(.red)
                    .lineLimit(3)
                    .padding(.horizontal, 16)
            }
        }
        .padding(.bottom, 8)
    }

    private func modeLabel(_ mode: String) -> String {
        switch mode {
        case "global":
            return "全局"
        case "direct":
            return "直连"
        default:
            return "规则"
        }
    }

    private func diagnoseSummary(_ report: DiagnoseReport) -> String {
        let failCount = report.checks.filter { $0.status.lowercased() == "fail" }.count
        let warnCount = report.checks.filter { $0.status.lowercased() == "warn" }.count

        if failCount > 0 {
            return "\(failCount) 失败, \(warnCount) 警告"
        }
        if warnCount > 0 {
            return "\(warnCount) 警告"
        }
        return "全部通过"
    }

    private func diagnoseDetails(_ report: DiagnoseReport) -> some View {
        let issues = report.checks.filter { $0.status.lowercased() != "pass" }
        return VStack(alignment: .leading, spacing: 6) {
            ForEach(issues, id: \.name) { check in
                VStack(alignment: .leading, spacing: 3) {
                    HStack(spacing: 6) {
                        Text(diagnoseStatusLabel(check.status))
                            .font(.system(size: 11, weight: .semibold))
                            .foregroundColor(diagnoseStatusColor(check.status))
                        Text(check.name)
                            .font(.system(size: 11, weight: .semibold))
                        Spacer()
                    }
                    Text(check.message)
                        .font(.system(size: 11))
                        .foregroundColor(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                    if let suggestedFix = check.suggestedFix, !suggestedFix.isEmpty {
                        Text("建议：\(suggestedFix)")
                            .font(.system(size: 11))
                            .foregroundColor(.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
                .padding(8)
                .background(diagnoseStatusColor(check.status).opacity(0.12))
                .cornerRadius(6)
            }
        }
        .padding(.horizontal, 16)
    }

    private func diagnoseStatusLabel(_ status: String) -> String {
        switch status.lowercased() {
        case "fail":
            return "失败"
        case "warn":
            return "警告"
        default:
            return status
        }
    }

    private func diagnoseStatusColor(_ status: String) -> Color {
        switch status.lowercased() {
        case "fail":
            return .red
        case "warn":
            return .orange
        default:
            return .secondary
        }
    }

    private var toggleFailureBanner: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(viewModel.currentMode == "direct"
                 ? "直连模式使用当前网络访问测试地址。测试未通过，请检查当前网络能否访问该地址。"
                 : "连接测试暂未通过。切换网络后请稍候重试，也可更换节点或运行诊断。")
                .font(.system(size: 12))
                .foregroundColor(.red)
                .fixedSize(horizontal: false, vertical: true)
            Button(action: {
                Task { await viewModel.runDiagnose() }
            }) {
                HStack {
                    Image(systemName: "stethoscope").frame(width: 16)
                    Text("运行诊断").font(.system(size: 12, weight: .medium))
                    Spacer()
                }
                .padding(.horizontal, 10)
                .padding(.vertical, 6)
                .background(Color.red.opacity(0.12))
                .cornerRadius(6)
            }
            .buttonStyle(PlainButtonStyle())
        }
        .padding(.horizontal, 16)
    }

    private func toggleTestLabel(_ r: ProxyDelayResult) -> String {
        if let err = r.error, !err.isEmpty { return "✕ \(err)" }
        return "\(r.delay)ms"
    }

    private func toggleTestColor(_ r: ProxyDelayResult) -> Color {
        if r.error != nil { return .red }
        if r.delay <= 100 { return .green }
        if r.delay <= 250 { return .orange }
        return .red
    }

    private var moreSection: some View {
        VStack(alignment: .leading, spacing: 0) {
            MenuDisclosureRow("更多", summary: "工具与设置", isExpanded: moreExpanded) { moreExpanded.toggle() }
            if moreExpanded {
                actionsSection
                updateSection
                diagnosticsSection
                openSection
            }
        }
    }

    private var actionsSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            MenuButton("测试连接", icon: "checkmark.shield") {
                Task { await viewModel.testConnection() }
            }

            MenuButton("重启代理引擎", icon: "arrow.counterclockwise") {
                Task { await viewModel.restartCore() }
            }
        }
        .padding(.bottom, 8)
    }

    private var updateSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            MenuButton(viewModel.isCheckingForUpdate ? "检查中..." : "检查更新", icon: "arrow.down.circle") {
                onShowUpdates(true)
            }
            .disabled(viewModel.isCheckingForUpdate || viewModel.isInstallingUpdate)

            MenuButton("更新日志", icon: "clock.arrow.circlepath") {
                onShowUpdates(false)
            }
        }
        .padding(.bottom, 8)
    }

    private var updateActivitySection: some View {
        VStack(alignment: .leading, spacing: 4) {
            if !viewModel.updateStatus.isEmpty {
                VStack(alignment: .leading, spacing: 6) {
                    Text(viewModel.updateStatus)
                        .font(.system(size: 12))
                        .foregroundColor(viewModel.updateFailed ? .red : .secondary)
                    if let progress = viewModel.updateProgress {
                        ProgressView(value: Double(progress), total: 100)
                    }
                }
                .padding(.horizontal, 16)
                .padding(.bottom, 6)
            }
            if viewModel.availableUpdateVersion != nil || viewModel.isInstallingUpdate || viewModel.showRestartAlert || viewModel.updateFailed {
                MenuButton(viewModel.showRestartAlert ? "更新完成 · 查看详情" : "查看更新详情", icon: "doc.text") {
                    onShowUpdates(true)
                }
            }
        }
    }

    private var diagnosticsSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            MenuButton("运行诊断", icon: "stethoscope") {
                Task { await viewModel.runDiagnose() }
            }

            Link(destination: URL(string: "https://github.com/Cass-ette/ProxyCat/issues/new/choose")!) {
                HStack {
                    Image(systemName: "bubble.left.and.bubble.right")
                        .frame(width: 20)
                    Text("问题反馈（Issue）")
                    Spacer()
                    Image(systemName: "arrow.up.right")
                        .font(.system(size: 10))
                        .foregroundColor(.secondary)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(PlainButtonStyle())
            .padding(.horizontal, 16)
            .padding(.vertical, 6)
            .help("在 GitHub 报告问题或提出功能建议")

        }
        .padding(.bottom, 8)
    }

    private var openSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            MenuButton("配置文件夹", icon: "folder") {
                viewModel.openConfigFolder()
            }

            MenuButton("日志", icon: "doc.text.magnifyingglass") {
                viewModel.openLogs()
            }
        }
        .padding(.vertical, 8)
    }

    private var quitButton: some View {
        MenuButton("退出", icon: "xmark.circle") {
            NSApplication.shared.terminate(nil)
        }
        .padding(.vertical, 6)
    }
}

struct MenuDisclosureRow: View {
    let title: String
    let summary: String
    let isExpanded: Bool
    var canCollapse = true
    let action: () -> Void

    init(_ title: String, summary: String, isExpanded: Bool, canCollapse: Bool = true, action: @escaping () -> Void) {
        self.title = title
        self.summary = summary
        self.isExpanded = isExpanded
        self.canCollapse = canCollapse
        self.action = action
    }

    var body: some View {
        Button(action: action) {
            HStack(spacing: 8) {
                Text(title)
                    .font(.system(size: 12, weight: .medium))
                    .lineLimit(1)
                    .layoutPriority(1)
                Spacer(minLength: 4)
                Text(summary)
                    .font(.system(size: 11))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
                Image(systemName: isExpanded ? "chevron.down" : "chevron.right")
                    .font(.system(size: 9, weight: .semibold))
                    .foregroundColor(.secondary)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 10)
            .frame(maxWidth: .infinity)
            .contentShape(Rectangle())
        }
        .buttonStyle(PlainButtonStyle())
        .disabled(!canCollapse)
        .accessibilityLabel("\(title)，\(summary)")
        .accessibilityValue(isExpanded ? "已展开" : "已收起")
        .help(canCollapse ? (isExpanded ? "收起\(title)" : "展开\(title)") : "请先完成配置")
    }
}

struct StatusRow: View {
    let label: String
    let value: String

    var body: some View {
        HStack {
            Text(label + ":")
                .foregroundColor(.secondary)
            Text(value)
                .fontWeight(.medium)
            Spacer()
        }
        .padding(.horizontal, 16)
        .font(.system(size: 13))
    }
}

struct MenuButton: View {
    let title: String
    let icon: String
    let action: () -> Void

    init(_ title: String, icon: String, action: @escaping () -> Void) {
        self.title = title
        self.icon = icon
        self.action = action
    }

    var body: some View {
        Button(action: action) {
            HStack {
                Image(systemName: icon)
                    .frame(width: 20)
                Text(title)
                Spacer()
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(PlainButtonStyle())
        .padding(.horizontal, 16)
        .padding(.vertical, 6)
        .contentShape(Rectangle())
    }
}

// Helper for menu background
struct VisualEffectView: NSViewRepresentable {
    let material: NSVisualEffectView.Material
    let blendingMode: NSVisualEffectView.BlendingMode

    func makeNSView(context: Context) -> NSVisualEffectView {
        let view = NSVisualEffectView()
        view.material = material
        view.blendingMode = blendingMode
        view.state = .active
        view.wantsLayer = true
        view.layer?.cornerRadius = 8
        view.layer?.masksToBounds = true
        return view
    }

    func updateNSView(_ nsView: NSVisualEffectView, context: Context) {
        nsView.material = material
        nsView.blendingMode = blendingMode
    }
}
