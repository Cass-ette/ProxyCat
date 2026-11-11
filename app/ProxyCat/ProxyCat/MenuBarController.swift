import SwiftUI
import AppKit

@MainActor
class MenuBarController: NSObject, NSWindowDelegate, NSPopoverDelegate {
    private var statusItem: NSStatusItem!
    private var viewModel: StatusViewModel!
    private(set) var menuPopover: NSPopover?
    private var websiteWindow: NSWindow?
    private var connectionsWindow: NSWindow?
    private var updateWindow: NSWindow?
    private var statusObserver: MenuBarStatusObserver?
    private var spaceObserver: NSObjectProtocol?
    private let menuDefaults: UserDefaults

    override convenience init() {
        self.init(viewModel: StatusViewModel(), autoRefresh: true)
    }

    init(viewModel: StatusViewModel, autoRefresh: Bool, menuDefaults: UserDefaults = .standard) {
        self.menuDefaults = menuDefaults
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
        if menuPopover?.isShown == true {
            hideMenu()
        } else {
            showMenu()
        }
    }

    private func showMenu() {
        let contentWidth: CGFloat = 320
        guard let button = statusItem.button else { return }
        let buttonRect = button.convert(button.bounds, to: nil)
        let screenRect = button.window?.convertToScreen(buttonRect) ?? buttonRect
        let visibleFrame = button.window?.screen?.visibleFrame ?? NSScreen.main?.visibleFrame ?? screenRect
        // Reserve room for AppKit's popover frame and arrow. AppKit owns the
        // anchor and screen clamping; only the scrollable content is bounded.
        let maximumHeight = min(640, max(160, screenRect.minY - visibleFrame.minY - 32))
        let popover = NSPopover()
        popover.behavior = .transient
        popover.animates = false
        popover.delegate = self
        let hostingController = NSHostingController(
            rootView: MenuContentView(viewModel: viewModel, onShowUpdates: { [weak self] check in
                self?.showUpdates(check: check)
            }, onShowAvailableUpdate: { [weak self] in
                self?.showUpdates(check: false, showAvailable: true)
            }, onShowWebsiteCheck: { [weak self] in self?.showWebsiteCheck() },
            onShowConnections: { [weak self] in self?.showConnections() },
            maximumHeight: maximumHeight, onHeightChange: { [weak self, weak popover] height in
                // Apply geometry after SwiftUI finishes its layout pass.
                DispatchQueue.main.async {
                    guard let self, let popover, self.menuPopover === popover,
                          popover.isShown else { return }
                    self.resizeMenu(to: height)
                }
            }).defaultAppStorage(menuDefaults)
        )
        popover.contentViewController = hostingController
        hostingController.view.layoutSubtreeIfNeeded()
        let fittingHeight = hostingController.view.fittingSize.height
        popover.contentSize = NSSize(width: contentWidth, height: min(maximumHeight, max(1, fittingHeight)))
        menuPopover = popover
        popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
        popover.contentViewController?.view.window?.makeKey()
        startSpaceDismissal()
    }

    private func resizeMenu(to height: CGFloat) {
        guard let popover = menuPopover, popover.isShown,
              abs(popover.contentSize.height - height) > 0.5 else { return }
        popover.contentSize = NSSize(width: 320, height: height)
    }

    private func hideMenu() {
        menuPopover?.performClose(nil)
    }

    private func startSpaceDismissal() {
        spaceObserver = NSWorkspace.shared.notificationCenter.addObserver(forName: NSWorkspace.activeSpaceDidChangeNotification, object: nil, queue: .main) { [weak self] _ in
            Task { @MainActor in self?.hideMenu() }
        }
    }

    func popoverDidClose(_ notification: Notification) {
        guard let popover = notification.object as? NSPopover, popover === menuPopover else { return }
        if let observer = spaceObserver { NSWorkspace.shared.notificationCenter.removeObserver(observer) }
        spaceObserver = nil
        menuPopover = nil
    }

    func popoverShouldDetach(_ popover: NSPopover) -> Bool {
        false
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
