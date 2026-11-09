import SwiftUI
import AppKit

class KeyEventWindow: NSWindow {
    override var canBecomeKey: Bool { true }
}

@MainActor
class MenuBarController: NSObject, NSWindowDelegate {
    private var statusItem: NSStatusItem!
    private var viewModel: StatusViewModel!
    private var menuWindow: KeyEventWindow?
    private var websiteWindow: NSWindow?
    private var updateWindow: NSWindow?
    private var statusObserver: MenuBarStatusObserver?

    override init() {
        super.init()
        setupStatusItem()
        viewModel = StatusViewModel()
        statusObserver = MenuBarStatusObserver(viewModel: viewModel) { [weak self] state in
            self?.updateIcon(state)
        }
        viewModel.startAutoRefresh()
    }

    func startAutoRefresh() {
        viewModel.startAutoRefresh()
    }

    func stopAutoRefresh() {
        viewModel.stopAutoRefresh()
    }


    private func setupStatusItem() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)

        if let button = statusItem.button {
            button.target = self
            button.action = #selector(toggleMenu)
            button.imagePosition = .imageLeading
            updateIcon(.off)
        }
    }

    @objc private func toggleMenu() {
        if menuWindow == nil {
            showMenu()
        } else {
            hideMenu()
        }
    }

    private func showMenu() {
        let windowWidth: CGFloat = 320
        guard let button = statusItem.button else { return }
        let buttonRect = button.convert(button.bounds, to: nil)
        let screenRect = button.window?.convertToScreen(buttonRect) ?? buttonRect
        let visibleFrame = button.window?.screen?.visibleFrame ?? NSScreen.main?.visibleFrame ?? screenRect
        let margin: CGFloat = 8
        let windowHeight = min(640, max(160, screenRect.minY - visibleFrame.minY - margin * 2))
        let hostingController = NSHostingController(
            rootView: MenuContentView(viewModel: viewModel, onShowUpdates: { [weak self] check in
                self?.showUpdates(check: check)
            }, onShowWebsiteCheck: { [weak self] in self?.showWebsiteCheck() },
            maximumHeight: windowHeight, onHeightChange: { [weak self] height in
                // Apply geometry after SwiftUI finishes its layout pass.
                DispatchQueue.main.async { self?.resizeMenu(to: height) }
            })
                .frame(width: windowWidth)
        )

        let window = KeyEventWindow(contentViewController: hostingController)
        window.styleMask = [.borderless]
        window.isOpaque = false
        window.backgroundColor = .clear
        window.hasShadow = false
        window.level = .statusBar
        window.delegate = self

        // Position the whole window below the menu bar item, clamped to the visible screen.
        let rawX = screenRect.midX - windowWidth / 2
        let minX = visibleFrame.minX + margin
        let maxX = visibleFrame.maxX - windowWidth - margin
        let x = min(max(rawX, minX), maxX)

        let rawY = screenRect.minY - windowHeight - margin
        let minY = visibleFrame.minY + margin
        let y = max(rawY, minY)

        window.setFrame(NSRect(x: x, y: y, width: windowWidth, height: windowHeight), display: true)

        menuWindow = window
        window.makeKeyAndOrderFront(nil)
    }

    private func resizeMenu(to height: CGFloat) {
        guard let window = menuWindow, abs(window.frame.height - height) > 0.5 else { return }
        var frame = window.frame
        // Keep the top edge below the menu bar when sections open or close.
        frame.origin.y = frame.maxY - height
        frame.size.height = height
        window.setFrame(frame, display: true)
    }

    private func hideMenu() {
        menuWindow?.orderOut(nil)
        menuWindow = nil
    }

    private func showWebsiteCheck() {
        hideMenu()
        if websiteWindow == nil {
            let controller = NSHostingController(rootView: WebsiteCheckView(viewModel: viewModel))
            let window = NSWindow(contentViewController: controller)
            window.title = "ProxyCat · 网站检查"
            window.styleMask = [.titled, .closable, .miniaturizable, .resizable]
            window.isReleasedWhenClosed = false
            window.setContentSize(NSSize(width: 560, height: 640))
            window.contentMinSize = NSSize(width: 500, height: 420)
            window.center()
            websiteWindow = window
        }
        NSApplication.shared.activate(ignoringOtherApps: true)
        websiteWindow?.makeKeyAndOrderFront(nil)
    }

    private func showUpdates(check: Bool) {
        hideMenu()
        if !viewModel.isInstallingUpdate && !viewModel.showRestartAlert {
            viewModel.showingCurrentReleaseNotes = !check
        }
        if updateWindow == nil {
            let controller = NSHostingController(rootView: UpdateView(viewModel: viewModel, onClose: { [weak self] in
                self?.updateWindow?.orderOut(nil)
            }))
            let window = NSWindow(contentViewController: controller)
            window.title = "ProxyCat · 更新日志"
            window.styleMask = [.titled, .closable, .miniaturizable, .resizable]
            window.isReleasedWhenClosed = false
            window.setContentSize(NSSize(width: 600, height: 680))
            window.contentMinSize = NSSize(width: 520, height: 520)
            window.center()
            updateWindow = window
        }
        NSApplication.shared.activate(ignoringOtherApps: true)
        updateWindow?.makeKeyAndOrderFront(nil)
        if check { Task { await viewModel.checkForUpdate() } }
    }

    func windowDidResignKey(_ notification: Notification) {
        hideMenu()
    }

    private func updateIcon(_ state: MenuBarProxyState) {
        guard let button = statusItem.button else { return }
        button.image = state.image()
        button.title = ""
        button.toolTip = state.description
        button.setAccessibilityLabel(state.description)
    }
}
