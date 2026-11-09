import SwiftUI

struct WebsiteCheckView: View {
    @ObservedObject var viewModel: StatusViewModel
    @State private var address = ""
    @State private var showingEvidence = false
    @State private var confirmGlobal = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                Text("这个网站打不开？").font(.title2.weight(.semibold))
                Text("输入网址，实际检测这次请求的路径和响应。")
                    .foregroundColor(.secondary)
                HStack(spacing: 10) {
                    TextField("https://example.com", text: $address)
                        .textFieldStyle(RoundedBorderTextFieldStyle())
                        .disableAutocorrection(true)
                        .onSubmit { check() }
                    Button(viewModel.isCheckingWebsite ? "检测中…" : "检测", action: check)
                        .disabled(address.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || viewModel.isCheckingWebsite || viewModel.isRecoveringWebsite)
                        .keyboardShortcut(.defaultAction)
                }
                if viewModel.isCheckingWebsite {
                    HStack { ProgressView().scaleEffect(0.7); Text("正在访问，最多约 18 秒…") }
                        .font(.callout).foregroundColor(.secondary)
                }
                if let error = viewModel.websiteCheckError {
                    Text(error).foregroundColor(.orange).textSelection(.enabled)
                }
                if let result = viewModel.websiteResult {
                    VStack(alignment: .leading, spacing: 10) {
                        Text(result.host).font(.headline).textSelection(.enabled)
                        Text(result.summary).fixedSize(horizontal: false, vertical: true).textSelection(.enabled)
                        Text("检测于 \(checkTime(result.observedAt)) · 用时 \(String(format: "%.1f", Double(result.elapsedMS) / 1000)) 秒")
                            .font(.caption).foregroundColor(.secondary)
                    }
                    .padding(14).frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color.secondary.opacity(0.08)).cornerRadius(8)

                    if result.connectionFailed && result.route == "proxy" && viewModel.websiteResultIsCurrent {
                        if !viewModel.websiteRecoveryGroups.isEmpty {
                            Menu {
                                ForEach(viewModel.websiteRecoveryGroups) { group in
                                    Section(group.name) {
                                        ForEach(group.all.filter { $0 != group.now && $0 != "REJECT" && $0 != "DIRECT" }, id: \.self) { proxy in
                                            Button(proxy) { Task { await viewModel.changeNodeAndRecheck(group: group.name, proxy: proxy) } }
                                        }
                                    }
                                }
                            } label: { Label("换个节点并重测", systemImage: "arrow.triangle.2.circlepath") }
                            .disabled(viewModel.isCheckingWebsite || viewModel.isRecoveringWebsite)
                            Text("会改变对应分组后续连接的节点。")
                                .font(.caption).foregroundColor(.secondary)
                        } else {
                            Button("重新检测") { check() }
                            Text("这条路径由自动分组决定，可先测试节点延迟或在菜单中调整分组。")
                                .font(.caption).foregroundColor(.secondary)
                        }
                    } else if result.connectionFailed && result.route == "direct" && viewModel.websiteResultIsCurrent && viewModel.currentMode != "global" {
                        Button("切到全局后重测") { confirmGlobal = true }
                            .disabled(viewModel.isCheckingWebsite || viewModel.isRecoveringWebsite)
                    } else if ["engineUnavailable", "proxyUnavailable"].contains(result.outcome) {
                        Button("启动引擎后重测") {
                            Task { await viewModel.startCore(); await viewModel.checkWebsite(address) }
                        }
                        .disabled(viewModel.isCheckingWebsite || viewModel.isRecoveringWebsite)
                        Text("启动引擎不等于开启系统代理；本次检测会主动经过本地代理端口。")
                            .font(.caption).foregroundColor(.secondary)
                    } else if result.outcome == "certificate" {
                        Text("请检查系统时间、网站证书或网络登录页；检测保留证书验证。")
                            .font(.callout).foregroundColor(.secondary)
                    } else if result.route == "unknown" {
                        Button("重新检测") { check() }.disabled(viewModel.isCheckingWebsite || viewModel.isRecoveringWebsite)
                        Text("短连接可能在采样前结束；没有捕获到证据时，不能据此判断直连或代理。")
                            .font(.caption).foregroundColor(.secondary)
                    }
                    if !viewModel.websiteResultIsCurrent {
                        Text("配置或模式已变化，请重新检测后再采取恢复操作。")
                            .font(.callout).foregroundColor(.orange)
                    }
                    DisclosureGroup("路径证据", isExpanded: $showingEvidence) {
                        VStack(alignment: .leading, spacing: 8) {
                            evidence("请求", "\(result.scheme.uppercased()) GET · \(result.host)")
                            evidence("命中规则", result.rule.isEmpty ? "未捕获" : "\(result.rule) \(result.rulePayload)")
                            evidence("节点链", result.chains.isEmpty ? "未捕获" : result.chains.reversed().joined(separator: " → "))
                            evidence("HTTP", result.statusCode == 0 ? "未收到网页响应" : String(result.statusCode))
                        }.padding(.top, 8)
                    }
                }
                if let previous = viewModel.websitePreviousMode {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("已切到全局，影响所有进入 ProxyCat 的新连接，直到你恢复或另选模式。")
                            .font(.callout)
                        Button("恢复\(previous == "direct" ? "直连" : "规则")模式") {
                            Task { await viewModel.restoreWebsiteMode() }
                        }
                        .disabled(viewModel.isCheckingWebsite || viewModel.isChangingMode)
                    }
                    .padding(12).background(Color.orange.opacity(0.1)).cornerRadius(8)
                }
                Divider()
                Text("检测会经 ProxyCat 访问你输入的地址，不携带浏览器登录状态、不跟随跳转。这只能说明这次检测的路径；应用流量是否进入 ProxyCat、网页其他资源是否可用，需要另行确认。")
                    .font(.caption).foregroundColor(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                if !viewModel.systemProxyEnabled && !viewModel.tunEnabled {
                    Text("当前系统代理和 TUN 都已关闭，只有主动使用本地代理的程序会进入 ProxyCat。")
                        .font(.caption).foregroundColor(.orange)
                }
            }
            .padding(24)
        }
        .frame(minWidth: 500, minHeight: 420)
        .alert("切到全局并检测？", isPresented: $confirmGlobal) {
            Button("取消", role: .cancel) {}
            Button("切换并检测") { Task { await viewModel.checkWebsiteUsingGlobal() } }
        } message: {
            Text("这会改变所有进入 ProxyCat 的新连接，已有连接可能继续原路径。检测后可点击“恢复原模式”；关闭此窗口不会自动恢复。")
        }
    }

    private func check() {
        guard !viewModel.isCheckingWebsite, !viewModel.isRecoveringWebsite else { return }
        Task { await viewModel.checkWebsite(address) }
    }
    private func checkTime(_ raw: String) -> String {
        guard let date = ISO8601DateFormatter().date(from: raw) else { return raw }
        let formatter = DateFormatter(); formatter.dateFormat = "HH:mm:ss"
        return formatter.string(from: date)
    }
    private func evidence(_ label: String, _ value: String) -> some View {
        HStack(alignment: .top, spacing: 12) {
            Text(label).foregroundColor(.secondary).frame(width: 70, alignment: .leading)
            Text(value).textSelection(.enabled)
            Spacer(minLength: 0)
        }.font(.callout)
    }
}
