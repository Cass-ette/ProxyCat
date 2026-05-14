import SwiftUI

struct MenuContentView: View {
    @ObservedObject var viewModel: StatusViewModel
    @State private var subscriptionURL: String = ""
    @State private var isBootstrapping: Bool = false
    @AppStorage("nodesExpanded") private var nodesExpanded: Bool = true

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                // Header - ProxyCat On/Off
                headerSection

                if !viewModel.profiles.isEmpty {
                    Divider()
                    profileSection
                }

                Divider()

                // Quick Start section - 一键启动
                quickStartSection

                Divider()

                // Status section
                statusSection

                Divider()

                // Controls section
                controlsSection

                if hasFeedback {
                    Divider()
                    feedbackSection
                }

                Divider()

                // Actions section
                actionsSection

                Divider()

                // Updates
                updateSection

                Divider()

                // Diagnostics
                diagnosticsSection

                Divider()

                // Open section
                openSection

                Divider()

                // Quit
                quitButton
            }
        }
        .background(VisualEffectView(material: .menu, blendingMode: .behindWindow))
        .cornerRadius(8)
        .shadow(radius: 4)
        .frame(width: 320)
        .frame(maxHeight: 640)
        .onChange(of: viewModel.showRestartAlert) { show in
            if show {
                viewModel.showRestartAlert = false
                let alert = NSAlert()
                alert.messageText = "更新完成"
                alert.informativeText = "需要重启 ProxyCat 才能使用新版本。"
                alert.addButton(withTitle: "重启")
                alert.addButton(withTitle: "稍后")
                if alert.runModal() == .alertFirstButtonReturn {
                    viewModel.restartApp()
                }
            }
        }
    }

    private var headerSection: some View {
        HStack {
            Image(systemName: viewModel.systemProxyEnabled ? "cat.fill" : "cat")
                .foregroundColor(viewModel.systemProxyEnabled ? .green : .secondary)
            Text(viewModel.systemProxyEnabled ? "ProxyCat: 已连接" : "ProxyCat: 未连接")
                .font(.headline)
            Spacer()
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
    }

    private var profileSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("配置")
                .font(.caption)
                .foregroundColor(.secondary)
                .padding(.horizontal, 16)
                .padding(.top, 8)

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
            Text("快速开始")
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
                    Text(isBootstrapping ? "启动中..." : "一键启动")
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
            Text("状态")
                .font(.caption)
                .foregroundColor(.secondary)
                .padding(.horizontal, 16)
                .padding(.top, 8)

            StatusRow(label: "代理引擎", value: viewModel.coreRunning ? "运行中" : "已停止")
            StatusRow(label: "系统代理", value: viewModel.systemProxyEnabled ? "已开启" : "已关闭")
            StatusRow(label: "代理模式", value: modeLabel(viewModel.currentMode))
            StatusRow(label: "当前节点", value: viewModel.currentNode.isEmpty ? "-" : viewModel.currentNode)
        }
        .padding(.bottom, 8)
    }

    private var controlsSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("控制")
                .font(.caption)
                .foregroundColor(.secondary)
                .padding(.horizontal, 16)
                .padding(.top, 8)

            modeControls

            if !viewModel.proxyGroups.isEmpty {
                delayControls
            }

            if viewModel.proxyGroups.isEmpty {
                Text("暂无节点，请先一键启动或更新订阅")
                    .font(.system(size: 12))
                    .foregroundColor(.secondary)
                    .padding(.horizontal, 16)
            } else {
                HStack {
                    Text("节点")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(.secondary)
                    Spacer()
                    Button(action: {
                        withAnimation(.easeInOut(duration: 0.2)) {
                            nodesExpanded.toggle()
                        }
                    }) {
                        Image(systemName: nodesExpanded ? "chevron.up" : "chevron.down")
                            .font(.system(size: 10, weight: .semibold))
                            .foregroundColor(.secondary)
                            .frame(width: 20, height: 20)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(PlainButtonStyle())
                }
                .padding(.horizontal, 16)

                if nodesExpanded {
                    ScrollView {
                        proxyGroupControls
                    }
                    .frame(maxHeight: 320)
                    .transition(.opacity)
                }
            }
        }
        .padding(.bottom, 8)
    }

    private var modeControls: some View {
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
                    HStack {
                        Text(group.name)
                            .font(.system(size: 12, weight: .semibold))
                        Spacer()
                        Text(group.now.isEmpty ? "-" : group.now)
                            .font(.system(size: 11))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                    }
                    .padding(.horizontal, 16)

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
                        }
                        .buttonStyle(PlainButtonStyle())
                        .padding(.horizontal, 16)
                        .padding(.vertical, 3)
                        .contentShape(Rectangle())
                    }
                }
                .padding(.vertical, 4)
            }
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
            }

            if let report = viewModel.diagnoseReport {
                StatusRow(label: "诊断", value: diagnoseSummary(report))
                diagnoseDetails(report)
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

    private func statusText(_ ok: Bool) -> String {
        ok ? "✓" : "✗"
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

    private var actionsSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("操作")
                .font(.caption)
                .foregroundColor(.secondary)
                .padding(.horizontal, 16)
                .padding(.top, 8)

            if viewModel.systemProxyEnabled {
                MenuButton("关闭系统代理", icon: "network.slash") {
                    Task { await viewModel.disableSystemProxy() }
                }
            } else {
                MenuButton("开启系统代理", icon: "network") {
                    Task { await viewModel.enableSystemProxy() }
                }
            }

            MenuButton("更新订阅", icon: "arrow.clockwise") {
                Task { await viewModel.updateSubscription() }
            }

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
            Text("更新")
                .font(.caption)
                .foregroundColor(.secondary)
                .padding(.horizontal, 16)
                .padding(.top, 8)

            MenuButton(viewModel.isCheckingForUpdate ? "检查中..." : "检查更新", icon: "arrow.down.circle") {
                Task { await viewModel.checkForUpdate() }
            }
            .disabled(viewModel.isCheckingForUpdate || viewModel.isInstallingUpdate)

            if viewModel.isInstallingUpdate {
                MenuButton("取消更新", icon: "xmark.circle") {
                    Task { await viewModel.cancelUpdate() }
                }
            } else if viewModel.updateFailed {
                MenuButton("重试更新", icon: "arrow.clockwise") {
                    Task { await viewModel.installUpdate() }
                }
            } else {
                MenuButton(viewModel.isInstallingUpdate ? "更新中..." : "安装更新", icon: "square.and.arrow.down") {
                    Task { await viewModel.installUpdate() }
                }
                .disabled(viewModel.availableUpdateVersion == nil)
            }

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
        }
        .padding(.bottom, 8)
    }

    private var diagnosticsSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("诊断")
                .font(.caption)
                .foregroundColor(.secondary)
                .padding(.horizontal, 16)
                .padding(.top, 8)

            MenuButton("运行诊断", icon: "stethoscope") {
                Task { await viewModel.runDiagnose() }
            }
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
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
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
