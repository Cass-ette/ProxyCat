import SwiftUI
import AppKit

@MainActor enum AppDelegate { static var relaunchOnExit = false }

@main enum InteractionTest {
    @MainActor static func main() throws {
        print("Starting isolated native menu interaction checks.")
        fflush(stdout)
        let app = NSApplication.shared
        app.setActivationPolicy(.accessory)
        app.finishLaunching()
        app.activate(ignoringOtherApps: true)
        // Reproduce a menu-bar agent without SwiftUI-generated Edit commands.
        app.mainMenu = nil
        let suite = "proxycat-menu-interaction-\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suite)!
        defer { defaults.removePersistentDomain(forName: suite) }
        let vm = StatusViewModel(helper: HelperClient(proxyctlPath: "/nonexistent-proxycat-test-helper"), defaults: defaults, wifiSSIDProvider: { nil }, currentVersion: "0.16.0", currentReleaseNotes: "")
        let controller = MenuBarController(viewModel: vm, autoRefresh: false, menuDefaults: defaults)
        func pump(_ interval: TimeInterval = 0.2) {
            let deadline = Date().addingTimeInterval(interval)
            while Date() < deadline {
                if let event = app.nextEvent(matching: .any, until: Date().addingTimeInterval(0.01), inMode: .default, dequeue: true) { app.sendEvent(event) }
                RunLoop.main.run(until: Date().addingTimeInterval(0.01))
            }
        }
        func open() -> NSWindow {
            controller.perform(NSSelectorFromString("toggleMenu"))
            pump()
            guard let popover = controller.menuPopover, popover.isShown,
                  let menu = popover.contentViewController?.view.window, menu.isVisible else { fatalError("Native menu popover did not open") }
            return menu
        }
        pump()
        func click(_ window: NSWindow) {
            let event = NSEvent.mouseEvent(with: .leftMouseDown, location: NSPoint(x: 1, y: 1), modifierFlags: [], timestamp: 0, windowNumber: window.windowNumber, context: nil, eventNumber: 1, clickCount: 1, pressure: 1)!
            app.postEvent(event, atStart: false)
            pump()
        }
        var menu = open()
        func descendants(_ view: NSView) -> [NSView] {
            [view] + view.subviews.flatMap(descendants)
        }
        guard let content = controller.menuPopover?.contentViewController?.view,
              let input = descendants(content).compactMap({ $0 as? NSTextField }).first(where: { $0.isEditable }) else {
            fatalError("The subscription input must remain available in the native popover")
        }
        precondition(menu.makeFirstResponder(input), "The subscription input must accept keyboard focus")
        input.currentEditor()?.insertText("https://example.com/subscription")
        precondition(input.stringValue == "https://example.com/subscription", "Typing in the subscription field must work without activating a proxy")
        do {
            func require(_ condition: Bool, _ message: String) throws {
                if !condition { throw NSError(domain: "MenuInteractionTests", code: 1, userInfo: [NSLocalizedDescriptionKey: message]) }
            }
            let pasteboard = NSPasteboard.general
            let savedClipboard = (pasteboard.pasteboardItems ?? []).map { item in
                item.types.compactMap { type in item.data(forType: type).map { (type, $0) } }
            }
            defer {
                pasteboard.clearContents()
                let items = savedClipboard.map { values -> NSPasteboardItem in
                    let item = NSPasteboardItem()
                    for (type, data) in values { item.setData(data, forType: type) }
                    return item
                }
                pasteboard.writeObjects(items)
            }
            func shortcut(_ key: String, code: UInt16) {
                let event = NSEvent.keyEvent(with: .keyDown, location: .zero, modifierFlags: .command, timestamp: 0, windowNumber: menu.windowNumber, context: nil, characters: key, charactersIgnoringModifiers: key, isARepeat: false, keyCode: code)!
                app.sendEvent(event)
                pump()
            }
            pasteboard.clearContents()
            pasteboard.setString("https://example.com/pasted-subscription", forType: .string)
            shortcut("a", code: 0)
            shortcut("v", code: 9)
            try require(input.stringValue == "https://example.com/pasted-subscription", "Command-A and Command-V must replace the subscription through normal key events")
            shortcut("a", code: 0)
            shortcut("c", code: 8)
            try require(pasteboard.string(forType: .string) == input.stringValue, "Command-C must copy the subscription")
            shortcut("x", code: 7)
            try require(input.stringValue.isEmpty, "Command-X must cut the selected subscription")
            shortcut("v", code: 9)
            try require(input.stringValue == "https://example.com/pasted-subscription", "Command-V must restore the cut subscription")
            try require(app.isActive && menu.isVisible, "Editing shortcuts must keep the application active and its native menu open")
        }
        let collapsedHeight = menu.frame.height
        defaults.set(true, forKey: "moreExpanded")
        pump(0.6)
        precondition(menu.frame.height > collapsedHeight + 20, "Expanding tools must grow the native popover")
        defaults.set(false, forKey: "moreExpanded")
        pump(0.6)
        precondition(abs(menu.frame.height - collapsedHeight) < 1, "Collapsing tools must remove unused popover height")
        precondition(menu.isVisible, "Typing and changing menu height must keep the popover open")
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
        print("Passed isolated native menu subscription-input, Command editing shortcuts, sizing, child-window, nested-popover, outside-click, Space-change and Escape interaction checks; no network helper was launched.")
        fflush(stdout)
    }
}
