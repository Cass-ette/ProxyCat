import SwiftUI

struct ConnectionsView: View {
    @ObservedObject var viewModel: StatusViewModel
    @State private var query = ""

    private var groups: [ConnectionAppGroup] { viewModel.connectionLog.groups(matching: query) }
    private var activeCount: Int { viewModel.connectionLog.entries.filter(\.isActive).count }
    private var activeLabel: String { viewModel.connectionLogError == nil ? "连接中" : "上次连接中" }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(alignment: .firstTextBaseline) {
                Text("连接记录").font(.title2.weight(.semibold))
                Spacer()
                Text("\(activeCount) 条\(activeLabel) · \(viewModel.connectionLog.entries.count) 条记录")
                    .font(.caption).foregroundColor(.secondary)
            }
            HStack(spacing: 10) {
                HStack(spacing: 6) {
                    Image(systemName: "magnifyingglass").foregroundColor(.secondary)
                    TextField("搜索应用、网站或节点", text: $query)
                        .textFieldStyle(PlainTextFieldStyle())
                        .disableAutocorrection(true)
                }
                .padding(8).background(Color.secondary.opacity(0.08)).cornerRadius(6)
                Button("清除已结束") { viewModel.clearEndedConnections() }
                    .disabled(!viewModel.connectionLog.entries.contains { !$0.isActive })
            }
            if let error = viewModel.connectionLogError {
                Text(error).font(.callout).foregroundColor(.orange)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if let notice = viewModel.connectionLookupNotice {
                Text(notice).font(.caption).foregroundColor(.secondary)
            }
            if groups.isEmpty {
                VStack(spacing: 10) {
                    Image(systemName: "list.bullet.rectangle").font(.system(size: 28)).foregroundColor(.secondary)
                    Text(emptyTitle).font(.headline)
                    Text(query.isEmpty ? "打开一个网页或使用应用，经过 ProxyCat 的连接会显示在这里。" : "试试应用名、域名或节点名。")
                        .font(.callout).foregroundColor(.secondary).multilineTextAlignment(.center)
                    if viewModel.connectionLogError != nil {
                        Button("重试") { Task { await viewModel.refreshConnections() } }
                            .disabled(viewModel.isLoadingConnections)
                    }
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 18) {
                        ForEach(groups) { group in
                            VStack(alignment: .leading, spacing: 6) {
                                HStack(spacing: 7) {
                                    Image(systemName: "app").foregroundColor(.secondary)
                                    Text(group.name).fontWeight(.semibold)
                                        .help(group.name == "未识别应用" ? "应用识别对新连接生效；已有连接或部分系统流量可能没有应用名称。" : group.name)
                                    Spacer()
                                    Text("\(group.activeCount) 条\(activeLabel) / \(group.entries.count) 条")
                                        .font(.caption).foregroundColor(.secondary)
                                }.padding(.horizontal, 4)
                                VStack(spacing: 0) {
                                    ForEach(group.entries) { entry in
                                        ConnectionRow(entry: entry, isStale: viewModel.connectionLogError != nil, viewModel: viewModel)
                                        if entry.id != group.entries.last?.id { Divider().padding(.horizontal, 12) }
                                    }
                                }
                                .background(Color.secondary.opacity(0.06)).cornerRadius(8)
                            }
                        }
                    }.padding(.vertical, 3)
                }
            }
            Divider()
            VStack(alignment: .leading, spacing: 5) {
                HStack {
                    Text(viewModel.tunEnabled ? "TUN 已开启" : "没看到某个应用？可在菜单开启 TUN 后重试。")
                    Spacer(minLength: 8)
                    if let updatedAt = viewModel.connectionLog.updatedAt {
                        Text("采样于 \(updatedAt.formatted(date: .omitted, time: .standard))")
                    }
                }
                Text("窗口打开时每 2 秒采样，最多保留 \(ConnectionLog.capacity) 条，退出后清空。短连接可能漏采；没有记录不代表应用未联网，连接中也不代表网站可用。")
                    .fixedSize(horizontal: false, vertical: true)
            }.font(.caption).foregroundColor(.secondary)
        }
        .padding(22)
        .frame(minWidth: 580, minHeight: 420)
    }

    private var emptyTitle: String {
        if !query.isEmpty { return "没有匹配的记录" }
        if viewModel.connectionLogError != nil { return "连接记录暂不可用" }
        return viewModel.isLoadingConnections ? "正在读取连接…" : "还没有捕获到连接"
    }
}

private struct ConnectionRow: View {
    let entry: ConnectionLogEntry
    let isStale: Bool
    @ObservedObject var viewModel: StatusViewModel
    @State private var showingDetails = false
    private var connection: ConnectionRecord { entry.connection }

    var body: some View {
        DisclosureGroup(isExpanded: $showingDetails) {
            VStack(alignment: .leading, spacing: 5) {
                Text("协议：\(connection.network.isEmpty ? "未知" : connection.network.uppercased()) · 入口：\(connection.inbound.isEmpty ? "未知" : connection.inbound) · 端口：\(connection.destinationPort.isEmpty ? "未知" : connection.destinationPort)")
                Text("规则：\(connection.rule.isEmpty ? "未提供" : connection.rule) \(connection.rulePayload)")
                Text("节点链：\(connection.chains.isEmpty ? "未提供" : connection.chains.reversed().map(ProxyDisplay.name).joined(separator: " → "))")
                Text("最近观察：\(entry.lastSeen.formatted(date: .omitted, time: .standard))")
                if !["", "未知目标"].contains(connection.host) {
                    Menu {
                        Button("此网站总是走代理") { Task { await viewModel.setWebsitePreference(host: connection.host, route: "proxy") } }
                        Button("此网站总是直连") { Task { await viewModel.setWebsitePreference(host: connection.host, route: "direct") } }
                        if viewModel.websitePreference(for: connection.host) != nil {
                            Divider()
                            Button("恢复订阅默认") { Task { await viewModel.removeWebsitePreference(host: connection.host) } }
                        }
                    } label: { Label("网站分流偏好", systemImage: "line.3.horizontal.decrease.circle") }
                    .disabled(viewModel.websitePreferenceBusy)
                    Text("域名包含子域名；IP 仅匹配自身。仅规则模式生效。")
                        .font(.caption).foregroundColor(.secondary)
                    if let preference = viewModel.websitePreference(for: connection.host) {
                        Text("已设为总是\(preference.route == "proxy" ? "走代理" : "直连")")
                    }
                    WebsitePreferenceFeedbackView(viewModel: viewModel, host: connection.host)
                }
            }
            .font(.caption).foregroundColor(.secondary).textSelection(.enabled)
            .frame(maxWidth: .infinity, alignment: .leading).padding(.top, 6)
        } label: {
            VStack(alignment: .leading, spacing: 4) {
                HStack(alignment: .firstTextBaseline, spacing: 10) {
                    Text("\(connection.destination) → \(connection.routeLabel)")
                        .font(.system(size: 13)).lineLimit(2)
                        .help("\(connection.destination) → \(connection.routeLabel)")
                    Spacer(minLength: 0)
                    Text(entry.isActive ? (isStale ? "上次连接中" : "连接中") : "已结束")
                        .font(.system(size: 10)).foregroundColor(.secondary).fixedSize()
                }
                Text(connection.reason).font(.caption).foregroundColor(.secondary)
                    .lineLimit(1).help(connection.reason)
            }
            .padding(.leading, 3)
        }
        .padding(12)
    }
}
