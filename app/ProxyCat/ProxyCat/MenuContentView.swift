import SwiftUI

struct MenuContentView: View {
    @ObservedObject var viewModel: StatusViewModel
    @State private var subscriptionURL: String = ""
    @State private var isBootstrapping: Bool = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                // Header - ProxyCat On/Off
                headerSection

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

            if viewModel.proxyGroups.isEmpty {
                Text("暂无节点，请先一键启动或更新订阅")
                    .font(.system(size: 12))
                    .foregroundColor(.secondary)
                    .padding(.horizontal, 16)
            } else {
                proxyGroupControls
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
                                Text(proxy)
                                    .lineLimit(1)
                                Spacer()
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
        return view
    }

    func updateNSView(_ nsView: NSVisualEffectView, context: Context) {
        nsView.material = material
        nsView.blendingMode = blendingMode
    }
}
