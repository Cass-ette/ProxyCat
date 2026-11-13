import AppKit
import SwiftUI

@main
enum ProxyCatInstaller {
    @MainActor
    static func main() {
        let arguments = Array(CommandLine.arguments.dropFirst())
        if arguments.count == 3, arguments[0] == "--install-files",
           let ownerUID = UInt32(arguments[1]), ownerUID > 0,
           let ownerGID = UInt32(arguments[2]), geteuid() == 0 {
            // The administrator-authorized worker has fixed source/destination
            // paths and never terminates processes or changes network settings.
            do {
                let source = Bundle.main.bundleURL.deletingLastPathComponent().appendingPathComponent("ProxyCat.app")
                try validatePayload(source)
                var transaction = InstallerTransaction()
                transaction.prepareOwnership = { candidate in
                    // Preserve the caller's ability to use in-app updates after
                    // an administrator-authorized install. Never follow links.
                    try InstallerCommand.run("/usr/sbin/chown", ["-R", "-P", "\(ownerUID):\(ownerGID)", candidate.path])
                }
                try transaction.install(source: source, destination: URL(fileURLWithPath: "/Applications/ProxyCat.app"))
                exit(0)
            } catch { fputs(error.localizedDescription + "\n", stderr); exit(1) }
        }
        guard CommandLine.arguments.count == 1 else { exit(2) }
        let application = NSApplication.shared
        let delegate = InstallerDelegate()
        application.delegate = delegate
        application.setActivationPolicy(.regular)
        application.run()
        withExtendedLifetime(delegate) {}
    }

    static func validatePayload(_ source: URL) throws {
        let payload = try InstallerBundle.inspect(source)
        guard payload.version == Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String,
              payload.build == Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String else {
            throw InstallerFailure("安装程序与 ProxyCat 版本不一致。请重新获取完整安装包。")
        }
        try InstallerCommand.verifySignature(source)
    }
}

@MainActor
final class InstallerDelegate: NSObject, NSApplicationDelegate, NSWindowDelegate, ObservableObject {
    private let destination = URL(fileURLWithPath: "/Applications/ProxyCat.app")
    private var window: NSWindow?
    @Published private(set) var busy = true
    @Published private(set) var ready = false
    @Published private(set) var status = "正在检查安装包…"
    @Published private(set) var errorMessage: String?
    @Published private(set) var errorDetails: String?
    @Published private(set) var isDowngrade = false
    @Published private(set) var versionLabel = "准备安装"
    @Published private(set) var buildLabel: String?
    @Published private(set) var actionTitle = "安装并打开"
    private var source: URL { Bundle.main.bundleURL.deletingLastPathComponent().appendingPathComponent("ProxyCat.app") }
    var applicationIcon: NSImage { NSImage(named: "AppIcon") ?? NSWorkspace.shared.icon(forFile: source.path) }

    func applicationDidFinishLaunching(_ notification: Notification) {
        let content = NSHostingView(rootView: InstallerView(model: self))
        let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 480, height: 305),
                              styleMask: [.titled, .closable], backing: .buffered, defer: false)
        window.title = "安装 ProxyCat"
        window.isReleasedWhenClosed = false
        window.delegate = self
        window.contentView = content
        window.center()
        window.makeKeyAndOrderFront(nil)
        self.window = window
        NSApplication.shared.activate(ignoringOtherApps: true)
        Task { await prepare() }
    }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply { busy ? .terminateCancel : .terminateNow }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
    func windowShouldClose(_ sender: NSWindow) -> Bool { !busy }
    func close() { if !busy { NSApp.terminate(nil) } }

    func resize(height: CGFloat) {
        guard let window, height > 0 else { return }
        let content = NSSize(width: 480, height: max(280, ceil(height)))
        let frame = window.frameRect(forContentRect: NSRect(origin: .zero, size: content))
        guard abs(window.frame.height - frame.height) > 1 else { return }
        window.setFrame(NSRect(x: window.frame.minX, y: window.frame.maxY - frame.height,
                               width: frame.width, height: frame.height), display: true)
    }

    private func prepare() async {
        do {
            let payload = source
            let target = destination
            let (next, previous) = try await Task.detached {
                try ProxyCatInstaller.validatePayload(payload)
                let next = try InstallerBundle.inspect(payload)
                let exists = (try? FileManager.default.attributesOfItem(atPath: target.path)) != nil
                return (next, exists ? try InstallerBundle.inspect(target) : nil)
            }.value
            isDowngrade = previous.map {
                next.version.compare($0.version, options: .numeric) == .orderedAscending
                    || (next.version == $0.version && next.build.compare($0.build, options: .numeric) == .orderedAscending)
            } ?? false
            versionLabel = previous.map { "\($0.version) → \(next.version)" } ?? "版本 \(next.version)"
            buildLabel = "构建 \(next.build)"
            actionTitle = previous == nil ? "安装并打开" : "更新并打开"
            if isDowngrade { actionTitle = "安装较早版本" }
            ready = true
        } catch { show(error) }
        busy = false
    }

    func install() async {
        guard ready, !busy else { return }
        if isDowngrade {
            let alert = NSAlert()
            alert.alertStyle = .warning
            alert.messageText = "安装较早版本？"
            alert.informativeText = "此安装包早于当前版本，继续会替换现有应用。订阅和设置保留。"
            alert.addButton(withTitle: "继续安装")
            alert.addButton(withTitle: "取消")
            guard alert.runModal() == .alertFirstButtonReturn else { return }
        }
        busy = true
        status = "正在检查安装包…"
        do {
            // Validate again before requesting any change to the running app.
            let payload = source
            try await Task.detached { try ProxyCatInstaller.validatePayload(payload) }.value
            let flow = InstallerFlow(
                validate: {},
                quitAndWait: { try await self.quitAndWait() },
                replace: { try await self.replace() },
                launch: { try await self.launch() }
            )
            try await flow.run()
            busy = false
            NSApp.terminate(nil)
        } catch {
            busy = false
            ready = false
            show(error)
        }
    }

    private func show(_ error: Error) {
        errorMessage = error.localizedDescription
        errorDetails = (error as? InstallerFailure)?.details
    }

    private func quitAndWait() async throws {
        status = "正在等待旧版安全退出…"
        let running = NSRunningApplication.runningApplications(withBundleIdentifier: "com.cassette.proxycat")
        for application in running where !application.isTerminated {
            guard application.terminate() else {
                throw InstallerFailure("旧版尚未接受退出请求，未替换应用。请检查旧版的退出提示后重试。")
            }
        }
        let start = Date()
        while running.contains(where: { !$0.isTerminated }) {
            let elapsed = Int(Date().timeIntervalSince(start))
            guard elapsed < 45 else {
                throw InstallerFailure("旧版尚未安全退出，未替换应用。请处理 ProxyCat 中的退出提示后重试；安装程序不会强制结束代理。")
            }
            status = "正在等待旧版安全退出… \(elapsed) 秒"
            try await Task.sleep(nanoseconds: 200_000_000)
        }
    }

    private func replace() async throws {
        status = "正在安装并验证…"
        let payload = source
        let target = destination
        let parent = destination.deletingLastPathComponent()
        if FileManager.default.isWritableFile(atPath: parent.path) {
            try await Task.detached { try InstallerTransaction().install(source: payload, destination: target) }.value
        } else {
            status = "等待 macOS 安装授权…"
            // Paths are quoted for the shell and then separately for AppleScript.
            let executable = Bundle.main.executableURL!.path
            let command = InstallerCommand.shellQuote(executable) + " --install-files \(getuid()) \(getgid())"
            let script = "do shell script " + InstallerCommand.appleScriptQuote(command)
                + " with administrator privileges with prompt \"ProxyCat 需要安装到应用程序文件夹。\""
            _ = try await Task.detached { try InstallerCommand.run("/usr/bin/osascript", ["-e", script]) }.value
        }
    }

    private func launch() async throws {
        status = "正在打开新版…"
        do {
            try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
                NSWorkspace.shared.openApplication(at: destination, configuration: .init()) { application, error in
                    if let error { continuation.resume(throwing: error) }
                    else if application == nil { continuation.resume(throwing: InstallerFailure("未收到应用启动确认。")) }
                    else { continuation.resume() }
                }
            }
        } catch {
            throw InstallerFailure("新版已安装，但未能自动打开。请从“应用程序”打开 ProxyCat。\n\(error.localizedDescription)")
        }
    }
}
