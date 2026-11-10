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
    private var connectionsWindow: NSWindow?
    private var updateWindow: NSWindow?
    private var statusObserver: MenuBarStatusObserver?
    private var outsideClickMonitor: Any?
    private var localClickMonitor: Any?
    private var spaceObserver: NSObjectProtocol?
    private var inactiveObserver: NSObjectProtocol?

    override convenience init() {
        self.init(viewModel: StatusViewModel(), autoRefresh: true)
    }

    init(viewModel: StatusViewModel, autoRefresh: Bool) {
        super.init()
        self.viewModel = viewModel
        setupStatusItem()
        statusObserver = MenuBarStatusObserver(viewModel: viewModel) { [weak self] state in
            self?.updateIcon(state)
        }
        if autoRefresh { viewModel.startAutoRefresh() }
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
            updateIcon(MenuBarIndicator(state: .off))
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
            }, onShowAvailableUpdate: { [weak self] in
                self?.showUpdates(check: false, showAvailable: true)
            }, onShowWebsiteCheck: { [weak self] in self?.showWebsiteCheck() },
            onShowConnections: { [weak self] in self?.showConnections() },
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
        window.collectionBehavior = [.moveToActiveSpace, .transient, .ignoresCycle]
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
        startMenuDismissal()
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
        stopMenuDismissal()
        menuWindow?.orderOut(nil)
        menuWindow = nil
    }

    private func startMenuDismissal() {
        stopMenuDismissal()
        outsideClickMonitor = NSEvent.addGlobalMonitorForEvents(matching: [.leftMouseDown, .rightMouseDown, .otherMouseDown]) { [weak self] _ in
            Task { @MainActor in self?.hideMenu() }
        }
        localClickMonitor = NSEvent.addLocalMonitorForEvents(matching: [.leftMouseDown, .rightMouseDown, .otherMouseDown, .keyDown]) { [weak self] event in
            guard let self, let menu = self.menuWindow else { return event }
            if event.type == .keyDown {
                if event.keyCode == 53, event.window === menu { self.hideMenu(); return nil }
                return event
            }
            // Let the cat button's own action toggle the menu once. Child
            // popovers and sheets remain interactive while the menu is open.
            if event.window === self.statusItem.button?.window { return event }
            var window = event.window
            while let candidate = window {
                if candidate === menu { return event }
                window = candidate.parent
            }
            if NSApplication.shared.modalWindow != nil { return event }
            self.hideMenu()
            return event
        }
        spaceObserver = NSWorkspace.shared.notificationCenter.addObserver(forName: NSWorkspace.activeSpaceDidChangeNotification, object: nil, queue: .main) { [weak self] _ in
            Task { @MainActor in self?.hideMenu() }
        }
        inactiveObserver = NotificationCenter.default.addObserver(forName: NSApplication.didResignActiveNotification, object: nil, queue: .main) { [weak self] _ in
            Task { @MainActor in self?.hideMenu() }
        }
    }

    private func stopMenuDismissal() {
        if let monitor = outsideClickMonitor { NSEvent.removeMonitor(monitor) }
        if let monitor = localClickMonitor { NSEvent.removeMonitor(monitor) }
        if let observer = spaceObserver { NSWorkspace.shared.notificationCenter.removeObserver(observer) }
        if let observer = inactiveObserver { NotificationCenter.default.removeObserver(observer) }
        outsideClickMonitor = nil
        localClickMonitor = nil
        spaceObserver = nil
        inactiveObserver = nil
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

    private func showConnections() {
        hideMenu()
        if connectionsWindow == nil {
            let controller = NSHostingController(rootView: ConnectionsView(viewModel: viewModel))
            let window = NSWindow(contentViewController: controller)
            window.title = "ProxyCat · 连接记录"
            window.styleMask = [.titled, .closable, .miniaturizable, .resizable]
            window.isReleasedWhenClosed = false
            window.delegate = self
            window.setContentSize(NSSize(width: 680, height: 620))
            window.contentMinSize = NSSize(width: 580, height: 420)
            window.center()
            connectionsWindow = window
        }
        NSApplication.shared.activate(ignoringOtherApps: true)
        connectionsWindow?.makeKeyAndOrderFront(nil)
        viewModel.startObservingConnections()
    }

    func windowWillClose(_ notification: Notification) {
        if let window = notification.object as? NSWindow, window === connectionsWindow {
            viewModel.stopObservingConnections()
        }
    }

    private func showUpdates(check: Bool, showAvailable: Bool = false) {
        hideMenu()
        if !viewModel.isInstallingUpdate && !viewModel.showRestartAlert {
            viewModel.showingCurrentReleaseNotes = !check && !showAvailable
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

    private func updateIcon(_ state: MenuBarIndicator) {
        guard let button = statusItem.button else { return }
        button.image = state.image()
        button.title = ""
        button.toolTip = state.description
        button.setAccessibilityLabel(state.description)
    }
}
