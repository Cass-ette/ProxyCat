import SwiftUI

struct MenuContentView: View {
    @ObservedObject var viewModel: StatusViewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Header - ProxyCat On/Off
            headerSection

            Divider()

            // Status section
            statusSection

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
        .background(VisualEffectView(material: .menu, blendingMode: .behindWindow))
        .cornerRadius(8)
        .shadow(radius: 4)
    }

    private var headerSection: some View {
        HStack {
            Image(systemName: viewModel.systemProxyEnabled ? "cat.fill" : "cat")
                .foregroundColor(viewModel.systemProxyEnabled ? .green : .secondary)
            Text("ProxyCat: \(viewModel.systemProxyEnabled ? "On" : "Off")")
                .font(.headline)
            Spacer()
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
    }

    private var statusSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Status")
                .font(.caption)
                .foregroundColor(.secondary)
                .padding(.horizontal, 16)
                .padding(.top, 8)

            StatusRow(label: "Core", value: viewModel.coreRunning ? "Running" : "Stopped")
            StatusRow(label: "System Proxy", value: viewModel.systemProxyEnabled ? "On" : "Off")
            StatusRow(label: "Mode", value: viewModel.currentMode)
            StatusRow(label: "Current", value: "\(viewModel.currentGroup) → \(viewModel.currentNode)")
        }
        .padding(.bottom, 8)
    }

    private var hasFeedback: Bool {
        viewModel.lastError != nil || viewModel.testResult != nil || viewModel.diagnoseReport != nil
    }

    private var feedbackSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Results")
                .font(.caption)
                .foregroundColor(.secondary)
                .padding(.horizontal, 16)
                .padding(.top, 8)

            if let testResult = viewModel.testResult {
                StatusRow(
                    label: "Test",
                    value: "Google \(statusText(testResult.googleOK)), GitHub \(statusText(testResult.githubOK))"
                )
            }

            if let report = viewModel.diagnoseReport {
                StatusRow(label: "Diagnose", value: diagnoseSummary(report))
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
        ok ? "OK" : "Fail"
    }

    private func diagnoseSummary(_ report: DiagnoseReport) -> String {
        let failCount = report.checks.filter { $0.status.lowercased() == "fail" }.count
        let warnCount = report.checks.filter { $0.status.lowercased() == "warn" }.count

        if failCount > 0 {
            return "\(failCount) failed, \(warnCount) warnings"
        }
        if warnCount > 0 {
            return "\(warnCount) warnings"
        }
        return "\(report.checks.count) checks passed"
    }

    private var actionsSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Actions")
                .font(.caption)
                .foregroundColor(.secondary)
                .padding(.horizontal, 16)
                .padding(.top, 8)

            if viewModel.systemProxyEnabled {
                MenuButton("Turn System Proxy Off", icon: "network.slash") {
                    Task { await viewModel.disableSystemProxy() }
                }
            } else {
                MenuButton("Turn System Proxy On", icon: "network") {
                    Task { await viewModel.enableSystemProxy() }
                }
            }

            MenuButton("Update Subscription", icon: "arrow.clockwise") {
                Task { await viewModel.updateSubscription() }
            }

            MenuButton("Test Connection", icon: "checkmark.shield") {
                Task { await viewModel.testConnection() }
            }

            MenuButton("Restart Core", icon: "arrow.counterclockwise") {
                Task { await viewModel.restartCore() }
            }
        }
        .padding(.bottom, 8)
    }

    private var diagnosticsSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Diagnostics")
                .font(.caption)
                .foregroundColor(.secondary)
                .padding(.horizontal, 16)
                .padding(.top, 8)

            MenuButton("Run Diagnose", icon: "stethoscope") {
                Task { await viewModel.runDiagnose() }
            }
        }
        .padding(.bottom, 8)
    }

    private var openSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            MenuButton("Config Folder", icon: "folder") {
                viewModel.openConfigFolder()
            }

            MenuButton("Logs", icon: "doc.text.magnifyingglass") {
                viewModel.openLogs()
            }
        }
        .padding(.vertical, 8)
    }

    private var quitButton: some View {
        MenuButton("Quit", icon: "xmark.circle") {
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
