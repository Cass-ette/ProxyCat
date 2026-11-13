import SwiftUI

struct InstallerView: View {
    @ObservedObject var model: InstallerDelegate

    var body: some View {
        VStack(alignment: .leading, spacing: 22) {
            HStack(spacing: 14) {
                Image(nsImage: model.applicationIcon).resizable().frame(width: 56, height: 56)
                VStack(alignment: .leading, spacing: 4) {
                    Text("ProxyCat").font(.system(size: 23, weight: .semibold))
                    Text(model.versionLabel).font(.callout).foregroundColor(.secondary)
                }
                Spacer()
            }
            VStack(alignment: .leading, spacing: 8) {
                Label("安装到应用程序", systemImage: "folder")
                Text("订阅和设置保留。安装时会自动退出旧版，代理连接暂时中断。")
                    .font(.callout).foregroundColor(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if model.busy {
                HStack(spacing: 10) {
                    ProgressView().controlSize(.small)
                    Text(model.status).font(.callout)
                }.accessibilityElement(children: .combine)
            } else if let error = model.errorMessage {
                VStack(alignment: .leading, spacing: 8) {
                    Label("安装未完成", systemImage: "exclamationmark.circle")
                        .font(.callout.weight(.medium)).foregroundColor(.orange)
                    Text(error).font(.callout).fixedSize(horizontal: false, vertical: true).textSelection(.enabled)
                    if let details = model.errorDetails {
                        DisclosureGroup("查看详情") {
                            ScrollView {
                                Text(details).font(.system(size: 11, design: .monospaced))
                                    .textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading)
                            }.frame(height: 72)
                        }.font(.caption).foregroundColor(.secondary)
                    }
                }
            } else if model.isDowngrade {
                Label("这是较早的版本，继续安装将降级。", systemImage: "arrow.down.circle")
                    .font(.callout).foregroundColor(.orange)
            }
            Divider()
            HStack {
                if let build = model.buildLabel { Text(build).font(.caption).foregroundColor(.secondary) }
                Spacer()
                Button(model.errorMessage == nil ? "取消" : "关闭") { model.close() }
                    .keyboardShortcut(.cancelAction).disabled(model.busy)
                if model.errorMessage == nil {
                    Button(model.actionTitle) { Task { await model.install() } }
                        .buttonStyle(.borderedProminent).keyboardShortcut(.defaultAction)
                        .disabled(!model.ready || model.busy)
                }
            }
        }
        .padding(26).frame(width: 480)
        .background(Color(nsColor: .windowBackgroundColor))
        .background(GeometryReader { geometry in
            Color.clear.preference(key: InstallerHeight.self, value: geometry.size.height)
        })
        .onPreferenceChange(InstallerHeight.self) { model.resize(height: $0) }
    }
}

private struct InstallerHeight: PreferenceKey {
    static var defaultValue: CGFloat = 0
    static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) { value = nextValue() }
}
