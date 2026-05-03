import SwiftUI
import AppKit

@MainActor
class MenuBarController: NSObject, NSWindowDelegate {
    private var statusItem: NSStatusItem!
    private var viewModel: StatusViewModel!
    private var menuWindow: NSWindow?

    override init() {
        super.init()
        setupStatusItem()
        viewModel = StatusViewModel()
        viewModel.startAutoRefresh()
    }


    private func setupStatusItem() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)

        if let button = statusItem.button {
            button.target = self
            button.action = #selector(toggleMenu)
            setIcon("cat", accessibilityDescription: "ProxyCat")
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
        let hostingController = NSHostingController(
            rootView: MenuContentView(viewModel: viewModel)
                .frame(width: 280)
        )

        let window = NSWindow(contentViewController: hostingController)
        window.styleMask = [.borderless]
        window.isOpaque = false
        window.backgroundColor = .clear
        window.level = .statusBar
        window.delegate = self

        // Position window below status bar icon
        if let button = statusItem.button {
            let buttonRect = button.convert(button.bounds, to: nil)
            let screenRect = button.window?.convertToScreen(buttonRect) ?? buttonRect
            let windowWidth: CGFloat = 280
            let windowHeight: CGFloat = 400
            let x = screenRect.midX - windowWidth / 2
            let y = screenRect.minY - 5
            window.setFrame(NSRect(x: x, y: y, width: windowWidth, height: windowHeight), display: true)
        }

        window.makeKeyAndOrderFront(nil)
        menuWindow = window

        setIcon("cat.fill", accessibilityDescription: "ProxyCat Active")
    }

    private func hideMenu() {
        menuWindow?.orderOut(nil)
        menuWindow = nil

        setIcon("cat", accessibilityDescription: "ProxyCat")
    }

    func windowDidResignKey(_ notification: Notification) {
        hideMenu()
    }

    func updateIcon(running: Bool, enabled: Bool) {
        let iconName: String
        if running && enabled {
            iconName = "cat.fill"
        } else if running {
            iconName = "cat.circle"
        } else {
            iconName = "cat"
        }
        setIcon(iconName, accessibilityDescription: "ProxyCat")
    }

    private func setIcon(_ systemName: String, accessibilityDescription: String) {
        guard let button = statusItem.button else { return }
        if let image = NSImage(systemSymbolName: systemName, accessibilityDescription: accessibilityDescription) {
            image.size = NSSize(width: 18, height: 18)
            button.image = image
            button.title = ""
        } else {
            button.image = nil
            button.title = "PC"
        }
    }
}
