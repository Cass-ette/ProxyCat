import SwiftUI

struct UpdateView: View {
    @ObservedObject var viewModel: StatusViewModel
    let onClose: () -> Void
    @State private var collapsedSections: Set<Int> = []

    private var isCurrent: Bool { viewModel.showingCurrentReleaseNotes }
    private var version: String {
        isCurrent ? viewModel.currentAppVersion : (viewModel.updateReleaseVersion ?? viewModel.currentAppVersion)
    }
    private var notes: String {
        isCurrent ? viewModel.currentReleaseNotes : viewModel.updateReleaseNotes
    }
    private var busy: Bool { viewModel.isCheckingForUpdate || viewModel.isInstallingUpdate }

    var body: some View {
        VStack(spacing: 0) {
            ScrollView {
                VStack(spacing: 22) {
                    header
                    if viewModel.isCheckingForUpdate {
                        ProgressView("正在检查新版本…")
                            .padding(.vertical, 32)
                    } else if notes.isEmpty {
                        VStack(spacing: 12) {
                            Image(systemName: viewModel.updateFailed ? "wifi.exclamationmark" : "doc.text")
                                .font(.system(size: 26))
                            Text(viewModel.updateFailed && !isCurrent ? "暂时无法获取更新说明" : "此版本暂无更新说明")
                                .font(.headline)
                            Text("可以稍后重试，或查看 GitHub 上的所有版本。")
                                .font(.callout)
                        }
                        .foregroundColor(.secondary)
                        .padding(.vertical, 28)
                    } else {
                        ForEach(ReleaseNotesSection.parse(notes)) { section in
                            notesCard(section)
                        }
                    }
                }
                .padding(28)
            }
            networkNotice
            if !viewModel.updateStatus.isEmpty && (!isCurrent || viewModel.updateFailed) {
                VStack(alignment: .leading, spacing: 8) {
                    HStack(spacing: 8) {
                        if viewModel.isInstallingUpdate { ProgressView().controlSize(.small) }
                        Text(viewModel.updateStatus)
                            .font(.callout)
                            .foregroundColor(viewModel.updateFailed ? .red : .secondary)
                            .fixedSize(horizontal: false, vertical: true)
                        Spacer(minLength: 0)
                    }
                    if viewModel.isInstallingUpdate, let progress = viewModel.updateProgress {
                        ProgressView(value: Double(progress), total: 100)
                    }
                    if viewModel.updateFailed, let details = viewModel.updateErrorDetails, !details.isEmpty {
                        DisclosureGroup("技术详情") {
                            ScrollView {
                                Text(details)
                                    .font(.system(size: 11, design: .monospaced))
                                    .foregroundColor(.secondary)
                                    .textSelection(.enabled)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                            }
                            .frame(maxHeight: 90)
                        }
                        .font(.caption)
                    }
                }
                .padding(.horizontal, 24)
                .padding(.vertical, 14)
            }
            Divider()
            footer
        }
        .frame(minWidth: 520, idealWidth: 600, minHeight: 520, idealHeight: 680)
        .background(Color(NSColor.windowBackgroundColor))
        .onChange(of: version) { _ in collapsedSections = [] }
    }

    private var networkNotice: some View {
        Label {
            Text("检查和下载更新需要访问 GitHub，部分网络可能需要先开启代理或 VPN。连接失败时，请开启后重试。")
                .font(.callout)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
        } icon: {
            Image(systemName: "network")
                .foregroundColor(.accentColor)
        }
        .padding(.horizontal, 24)
        .padding(.vertical, 12)
        .background(Color.accentColor.opacity(0.06))
    }

    private var header: some View {
        VStack(spacing: 12) {
            Image(nsImage: NSApplication.shared.applicationIconImage)
                .resizable()
                .frame(width: 78, height: 78)
                .accessibilityHidden(true)
            Text(!isCurrent && viewModel.availableUpdateVersion != nil ? "发现新版本" : "更新日志")
                .font(.system(size: 26, weight: .semibold))
            if isCurrent && viewModel.updateFailed {
                Text("当前版本更新日志 · 尚未确认是否有新版本")
                    .font(.callout)
                    .foregroundColor(.secondary)
            }
            Text("v\(version)")
                .font(.system(size: 15, weight: .semibold))
                .padding(.horizontal, 16)
                .padding(.vertical, 6)
                .background(Capsule().fill(Color.primary.opacity(0.05)))
                .overlay(Capsule().strokeBorder(Color.primary.opacity(0.08)))
            if !isCurrent, viewModel.availableUpdateVersion != nil {
                Text("当前版本 v\(viewModel.currentAppVersion) · 查看更新内容后选择安装")
                    .font(.callout)
                    .foregroundColor(.secondary)
            }
            if !isCurrent, let timestamp = viewModel.updatePublishedAt,
               let date = ISO8601DateFormatter().date(from: timestamp) {
                Text(date, style: .date)
                    .font(.caption)
                    .foregroundColor(.secondary)
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.top, 6)
        .padding(.bottom, 4)
    }

    private func notesCard(_ section: ReleaseNotesSection) -> some View {
        let style = sectionStyle(section.title)
        return VStack(alignment: .leading, spacing: 0) {
            Button {
                if collapsedSections.contains(section.id) { collapsedSections.remove(section.id) }
                else { collapsedSections.insert(section.id) }
            } label: {
                HStack(spacing: 12) {
                    Image(systemName: style.icon)
                        .font(.system(size: 15, weight: .semibold))
                        .foregroundColor(.white)
                        .frame(width: 30, height: 30)
                        .background(RoundedRectangle(cornerRadius: 8).fill(style.color))
                    Text(section.title)
                        .font(.system(size: 16, weight: .semibold))
                        .foregroundColor(.primary)
                    Spacer()
                    Image(systemName: collapsedSections.contains(section.id) ? "chevron.right" : "chevron.down")
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundColor(.secondary)
                }
                .padding(16)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel("\(section.title)，\(collapsedSections.contains(section.id) ? "已折叠" : "已展开")")
            if !collapsedSections.contains(section.id) {
                Divider()
                VStack(alignment: .leading, spacing: 12) {
                    ForEach(Array(section.items.enumerated()), id: \.offset) { _, item in
                        HStack(alignment: .top, spacing: 10) {
                            Text("•").foregroundColor(.secondary)
                            Text((try? AttributedString(markdown: item, options: .init(interpretedSyntax: .inlineOnlyPreservingWhitespace))) ?? AttributedString(item))
                                .textSelection(.enabled)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                    }
                }
                .font(.system(size: 14))
                .lineSpacing(4)
                .padding(16)
            }
        }
        .background(RoundedRectangle(cornerRadius: 14).fill(Color.primary.opacity(0.025)))
        .overlay(RoundedRectangle(cornerRadius: 14).strokeBorder(Color.primary.opacity(0.09)))
    }

    private var footer: some View {
        HStack(spacing: 12) {
            Link(destination: URL(string: "https://github.com/Cass-ette/ProxyCat/releases")!) {
                Label("查看所有版本", systemImage: "arrow.up.right")
            }
            .buttonStyle(.bordered)
            Spacer()
            if viewModel.showRestartAlert {
                Button("稍后", action: onClose)
                Button("重启 ProxyCat") { viewModel.restartApp() }
                    .buttonStyle(.borderedProminent)
            } else if viewModel.isInstallingUpdate {
                Button("取消更新") { Task { await viewModel.cancelUpdate() } }
            } else if !isCurrent, viewModel.availableUpdateVersion != nil {
                Button("稍后", action: onClose)
                Button(viewModel.updateFailed ? "重试安装" : "安装更新") {
                    Task { await viewModel.installUpdate() }
                }
                .buttonStyle(.borderedProminent)
                .disabled(busy)
            } else {
                Button("完成", action: onClose)
                Button(viewModel.isCheckingForUpdate ? "检查中…" : "检查更新") {
                    Task { await viewModel.checkForUpdate() }
                }
                .buttonStyle(.borderedProminent)
                .disabled(busy)
            }
        }
        .controlSize(.large)
        .padding(.horizontal, 24)
        .padding(.vertical, 16)
    }

    private func sectionStyle(_ title: String) -> (icon: String, color: Color) {
        if title.contains("亮点") { return ("star.fill", .orange) }
        if title.contains("新功能") { return ("sparkles", .purple) }
        if title.contains("修复") { return ("wrench.and.screwdriver.fill", .blue) }
        if title.contains("改进") || title.contains("优化") { return ("slider.horizontal.3", .green) }
        return ("doc.text.fill", .secondary)
    }
}
