import SwiftUI
import AppKit

@MainActor enum AppDelegate { static var relaunchOnExit = false }

@main enum InteractionTest {
    @MainActor static func main() throws {
        let app = NSApplication.shared
        app.setActivationPolicy(.prohibited)
        let suite = "proxycat-menu-interaction-\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suite)!
        defer { defaults.removePersistentDomain(forName: suite) }
        let vm = StatusViewModel(helper: HelperClient(proxyctlPath: "/nonexistent-proxycat-test-helper"), defaults: defaults, wifiSSIDProvider: { nil }, currentVersion: "0.16.0", currentReleaseNotes: "")
        let controller = MenuBarController(viewModel: vm, autoRefresh: false)
        func pump() {
            let deadline = Date().addingTimeInterval(0.15)
            while Date() < deadline {
                if let event = app.nextEvent(matching: .any, until: Date().addingTimeInterval(0.01), inMode: .default, dequeue: true) { app.sendEvent(event) }
                RunLoop.main.run(until: Date().addingTimeInterval(0.01))
            }
        }
        func open() -> NSWindow {
            controller.perform(NSSelectorFromString("toggleMenu"))
            pump()
            guard let menu = app.windows.first(where: { $0 is KeyEventWindow && $0.isVisible }) else { fatalError("Menu did not open") }
            return menu
        }
        func click(_ window: NSWindow) {
            let event = NSEvent.mouseEvent(with: .leftMouseDown, location: NSPoint(x: 1, y: 1), modifierFlags: [], timestamp: 0, windowNumber: window.windowNumber, context: nil, eventNumber: 1, clickCount: 1, pressure: 1)!
            app.postEvent(event, atStart: false)
            pump()
        }
        var menu = open()
        let child = NSPanel(contentRect: NSRect(x: -9000, y: -9000, width: 100, height: 100), styleMask: .borderless, backing: .buffered, defer: false)
        menu.addChildWindow(child, ordered: .above)
        child.orderFront(nil)
        click(child)
        precondition(menu.isVisible, "Clicking a child popover must not dismiss the menu")
        menu.removeChildWindow(child); child.orderOut(nil)
        let popover = NSPopover()
        popover.animates = false
        popover.behavior = .applicationDefined
        popover.contentViewController = NSHostingController(rootView: Text("流量详情测试").padding(16))
        let before = Set(app.windows.map(\.windowNumber))
        popover.show(relativeTo: NSRect(x: 20, y: 20, width: 1, height: 1), of: menu.contentView!, preferredEdge: .maxX)
        pump()
        if let popoverWindow = app.windows.first(where: { !before.contains($0.windowNumber) && $0.isVisible }) {
            click(popoverWindow)
            precondition(menu.isVisible && popover.isShown, "Clicking the native popover must keep it and the menu open")
        } else { fatalError("The native popover did not open") }
        popover.close()
        let outside = NSWindow(contentRect: NSRect(x: -9000, y: -9000, width: 100, height: 100), styleMask: .borderless, backing: .buffered, defer: false)
        outside.orderFront(nil)
        click(outside)
        precondition(!menu.isVisible, "Clicking a different window must dismiss the menu")
        outside.orderOut(nil)
        menu = open()
        NSWorkspace.shared.notificationCenter.post(name: NSWorkspace.activeSpaceDidChangeNotification, object: nil)
        pump()
        precondition(!menu.isVisible, "A Space-change notification must dismiss the menu")
        menu = open()
        let escape = NSEvent.keyEvent(with: .keyDown, location: .zero, modifierFlags: [], timestamp: 0, windowNumber: menu.windowNumber, context: nil, characters: "\u{1b}", charactersIgnoringModifiers: "\u{1b}", isARepeat: false, keyCode: 53)!
        app.postEvent(escape, atStart: false); pump()
        precondition(!menu.isVisible, "Escape must dismiss the menu")
        print("Passed isolated AppKit menu child-window, native-popover, outside-click, Space-change and Escape interaction checks; no network helper was launched.")
    }
}
