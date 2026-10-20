import SwiftUI

@main
struct ProxyCatApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var appDelegate

    var body: some Scene {
        // No window scenes - menu bar only
        Settings {
            EmptyView()
        }
    }
}

@MainActor
class AppDelegate: NSObject, NSApplicationDelegate {
    static var relaunchOnExit = false
    var menuBarController: MenuBarController?
    private var terminationInProgress = false

    func applicationDidFinishLaunching(_ notification: Notification) {
        menuBarController = MenuBarController()
        NSWorkspace.shared.notificationCenter.addObserver(
            self,
            selector: #selector(systemWillPowerOff(_:)),
            name: NSWorkspace.willPowerOffNotification,
            object: nil
        )
    }

    @objc private func systemWillPowerOff(_ notification: Notification) {
        Self.relaunchOnExit = false
        NSApplication.shared.terminate(nil)
    }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        guard !terminationInProgress else { return .terminateLater }
        terminationInProgress = true
        menuBarController?.stopAutoRefresh()

        Task {
            let result = await HelperClient.shared.shutdown()
            switch result {
            case .success:
                if Self.relaunchOnExit {
                    relaunchAfterExit()
                }
                sender.reply(toApplicationShouldTerminate: true)
            case .failure(let error):
                await HelperClient.shared.resumeAfterCancelledShutdown()
                terminationInProgress = false
                Self.relaunchOnExit = false
                sender.reply(toApplicationShouldTerminate: false)
                menuBarController?.startAutoRefresh()

                let alert = NSAlert()
                alert.messageText = "无法安全退出 ProxyCat"
                alert.informativeText = "关闭系统代理失败，已保留代理内核以维持网络连接。请关闭系统代理后重试。\n\n\(error.localizedDescription)"
                alert.alertStyle = .warning
                alert.runModal()
            }
        }
        // macOS must wait for proxy settings to be persisted before ending the session.
        return .terminateLater
    }

    private func relaunchAfterExit() {
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/bin/sh")
        task.arguments = [
            "-c",
            "while /bin/kill -0 \"$1\" 2>/dev/null; do sleep 0.2; done; /usr/bin/open \"$2\"",
            "proxycat-relaunch",
            String(ProcessInfo.processInfo.processIdentifier),
            Bundle.main.bundlePath
        ]
        try? task.run()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        return false  // Keep running in menu bar
    }
}
