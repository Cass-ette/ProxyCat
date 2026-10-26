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
    private var updateWindow: NSWindow?

    override init() {
        super.init()
        setupStatusItem()
        viewModel = StatusViewModel()
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
        let windowWidth: CGFloat = 320
        let windowHeight: CGFloat = 640
        let hostingController = NSHostingController(
            rootView: MenuContentView(viewModel: viewModel, onShowUpdates: { [weak self] check in
                self?.showUpdates(check: check)
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
        if let button = statusItem.button {
            let buttonRect = button.convert(button.bounds, to: nil)
            let screenRect = button.window?.convertToScreen(buttonRect) ?? buttonRect
            let visibleFrame = button.window?.screen?.visibleFrame ?? NSScreen.main?.visibleFrame ?? screenRect
            let margin: CGFloat = 8

            let rawX = screenRect.midX - windowWidth / 2
            let minX = visibleFrame.minX + margin
            let maxX = visibleFrame.maxX - windowWidth - margin
            let x = min(max(rawX, minX), maxX)

            let rawY = screenRect.minY - windowHeight - margin
            let minY = visibleFrame.minY + margin
            let y = max(rawY, minY)

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
            image.isTemplate = true
            button.image = image
        } else {
            button.image = nil
        }
        button.title = ""
        button.toolTip = accessibilityDescription
    }
}
