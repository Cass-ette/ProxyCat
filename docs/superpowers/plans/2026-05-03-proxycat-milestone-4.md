# ProxyCat Milestone 4 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create SwiftUI macOS menu bar app that bundles proxyctl and provides native UI for proxy management.

**Architecture:**
- SwiftUI menu bar app with `NSMenu` via `MenuBarController`
- `HelperClient` executes proxyctl commands and parses JSON
- `StatusViewModel` bridges UI and data layer
- Menu items: Status display, Actions, Diagnostics, Open, Quit

**Tech Stack:** Swift, SwiftUI, Foundation, AppKit (for NSStatusBar)

---

## File Structure

```
app/
└── ProxyCat/
    ├── ProxyCat.xcodeproj/          # Xcode project
    ├── ProxyCat/
    │   ├── ProxyCatApp.swift      # App entry point
    │   ├── MenuBarController.swift  # NSStatusBar menu management
    │   ├── HelperClient.swift       # proxyctl command wrapper
    │   ├── StatusViewModel.swift    # Observable state management
    │   ├── MenuContentView.swift    # SwiftUI menu content
    │   └── Info.plist
    └── ProxyCatUITests/
```

---

## Task 1: Create Xcode Project Structure

**Files:**
- Create: `app/ProxyCat/ProxyCat.xcodeproj/project.pbxproj`
- Create: `app/ProxyCat/ProxyCat/Info.plist`
- Create: `app/ProxyCat/ProxyCat/ProxyCat.entitlements`

**Design:**
- macOS app bundle identifier: `com.cassette.proxycat`
- Minimum macOS version: 11.0 (Big Sur)
- Capabilities: None needed for MVP (no sandbox for helper execution)
- Bundle proxyctl binary in Resources

- [ ] **Step 1: Create project structure**

```bash
mkdir -p app/ProxyCat/ProxyCat
mkdir -p app/ProxyCat/ProxyCatUITests
```

- [ ] **Step 2: Create Info.plist**

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" ...>
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>en</string>
    <key>CFBundleExecutable</key>
    <string>ProxyCat</string>
    <key>CFBundleIdentifier</key>
    <string>com.cassette.proxycat</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>ProxyCat</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>0.1.0</string>
    <key>LSMinimumSystemVersion</key>
    <string>11.0</string>
    <key>LSUIElement</key>
    <true/>  <!-- Menu bar only, no dock icon -->
</dict>
</plist>
```

- [ ] **Step 3: Create entitlements**

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" ...>
<plist version="1.0">
<dict>
    <key>com.apple.security.app-sandbox</key>
    <false/>
</dict>
</plist>
```

- [ ] **Step 4: Commit**

```bash
git add app/ProxyCat/
git commit -m "chore: create Xcode project structure"
```

---

## Task 2: HelperClient

**Files:**
- Create: `app/ProxyCat/ProxyCat/HelperClient.swift`
- Create: `app/ProxyCat/ProxyCat/Models.swift`

**Design:**
- Execute bundled proxyctl from Resources
- Async methods returning Result<T, Error>
- Parse JSON output for all status commands
- Handle proxyctl not found / errors gracefully

- [ ] **Step 1: Define data models**

```swift
// Models.swift
struct CoreStatus: Codable {
    let running: Bool
    let pid: Int
}

struct SystemProxyStatus: Codable {
    let httpEnabled: Bool
    let httpHost: String
    let httpPort: Int
    let httpsEnabled: Bool
    let httpsHost: String
    let httpsPort: Int
    let socksEnabled: Bool
    let socksHost: String
    let socksPort: Int
}

struct TestResult: Codable {
    let googleOK: Bool
    let githubOK: Bool
    let error: String?
}

struct ProxyGroup: Codable {
    let name: String
    let type: String
    let now: String
    let all: [String]
}

struct DiagnoseReport: Codable {
    let app: String
    let milestone: String
    let checks: [CheckResult]
}

struct CheckResult: Codable {
    let name: String
    let status: String
    let message: String
    let suggestedFix: String?
}
```

- [ ] **Step 2: Implement HelperClient**

```swift
// HelperClient.swift
import Foundation

enum HelperError: Error {
    case executableNotFound
    case executionFailed(String)
    case invalidOutput
    case decodingFailed(Error)
}

actor HelperClient {
    static let shared = HelperClient()

    private var proxyctlPath: String?

    private func getProxyctlPath() -> String? {
        if let path = proxyctlPath { return path }

        // Check bundled in Resources
        if let bundled = Bundle.main.path(forResource: "proxyctl", ofType: nil) {
            proxyctlPath = bundled
            return bundled
        }

        // Check PATH
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/usr/bin/which")
        task.arguments = ["proxyctl"]
        let pipe = Pipe()
        task.standardOutput = pipe
        try? task.run()
        task.waitUntilExit()

        let data = pipe.fileHandleForReading.readDataToEndOfFile()
        if let path = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines),
           !path.isEmpty {
            proxyctlPath = path
            return path
        }

        return nil
    }

    private func runCommand(_ args: [String]) async -> Result<Data, HelperError> {
        guard let executable = getProxyctlPath() else {
            return .failure(.executableNotFound)
        }

        return await withCheckedContinuation { continuation in
            DispatchQueue.global().async {
                let task = Process()
                task.executableURL = URL(fileURLWithPath: executable)
                task.arguments = args

                let outputPipe = Pipe()
                let errorPipe = Pipe()
                task.standardOutput = outputPipe
                task.standardError = errorPipe

                do {
                    try task.run()
                    task.waitUntilExit()

                    let data = outputPipe.fileHandleForReading.readDataToEndOfFile()
                    let errorData = errorPipe.fileHandleForReading.readDataToEndOfFile()

                    if task.terminationStatus != 0,
                       let stderr = String(data: errorData, encoding: .utf8),
                       !stderr.isEmpty {
                        continuation.resume(returning: .failure(.executionFailed(stderr)))
                        return
                    }

                    continuation.resume(returning: .success(data))
                } catch {
                    continuation.resume(returning: .failure(.executionFailed(error.localizedDescription)))
                }
            }
        }
    }

    func getCoreStatus() async -> Result<CoreStatus, HelperError> {
        let result = await runCommand(["core", "status", "--json"])
        return result.flatMap { data in
            do {
                let status = try JSONDecoder().decode(CoreStatus.self, from: data)
                return .success(status)
            } catch {
                return .failure(.decodingFailed(error))
            }
        }
    }

    func getSystemProxyStatus() async -> Result<SystemProxyStatus, HelperError> {
        let result = await runCommand(["system-proxy", "status", "--json"])
        return result.flatMap { data in
            do {
                let status = try JSONDecoder().decode(SystemProxyStatus.self, from: data)
                return .success(status)
            } catch {
                return .failure(.decodingFailed(error))
            }
        }
    }

    func enableSystemProxy() async -> Result<Void, HelperError> {
        let result = await runCommand(["system-proxy", "on"])
        return result.map { _ in () }
    }

    func disableSystemProxy() async -> Result<Void, HelperError> {
        let result = await runCommand(["system-proxy", "off"])
        return result.map { _ in () }
    }

    func startCore() async -> Result<Void, HelperError> {
        let result = await runCommand(["core", "start"])
        return result.map { _ in () }
    }

    func stopCore() async -> Result<Void, HelperError> {
        let result = await runCommand(["core", "stop"])
        return result.map { _ in () }
    }

    func restartCore() async -> Result<Void, HelperError> {
        let result = await runCommand(["core", "restart"])
        return result.map { _ in () }
    }

    func testConnection() async -> Result<TestResult, HelperError> {
        let result = await runCommand(["test", "--json"])
        return result.flatMap { data in
            do {
                let testResult = try JSONDecoder().decode(TestResult.self, from: data)
                return .success(testResult)
            } catch {
                return .failure(.decodingFailed(error))
            }
        }
    }

    func updateSubscription() async -> Result<Void, HelperError> {
        let result = await runCommand(["subscription", "update"])
        return result.map { _ in () }
    }

    func runDiagnose() async -> Result<DiagnoseReport, HelperError> {
        let result = await runCommand(["diagnose", "--json"])
        return result.flatMap { data in
            do {
                let report = try JSONDecoder().decode(DiagnoseReport.self, from: data)
                return .success(report)
            } catch {
                return .failure(.decodingFailed(error))
            }
        }
    }

    func getProxyGroups() async -> Result<[ProxyGroup], HelperError> {
        let result = await runCommand(["groups", "--json"])
        return result.flatMap { data in
            do {
                // Groups come as dictionary, convert to array
                let dict = try JSONDecoder().decode([String: ProxyGroup].self, from: data)
                let groups = dict.map { name, group in
                    ProxyGroup(name: name, type: group.type, now: group.now, all: group.all)
                }.sorted { $0.name < $1.name }
                return .success(groups)
            } catch {
                return .failure(.decodingFailed(error))
            }
        }
    }

    func selectProxy(group: String, proxy: String) async -> Result<Void, HelperError> {
        let result = await runCommand(["select", group, proxy])
        return result.map { _ in () }
    }

    func openConfigFolder() async {
        _ = await runCommand(["config", "open"])
    }

    func openLogs() async {
        _ = await runCommand(["logs", "open"])
    }
}
```

- [ ] **Step 3: Commit**

```bash
git add app/ProxyCat/ProxyCat/HelperClient.swift app/ProxyCat/ProxyCat/Models.swift
git commit -m "feat: add HelperClient and data models"
```

---

## Task 3: StatusViewModel

**Files:**
- Create: `app/ProxyCat/ProxyCat/StatusViewModel.swift`

**Design:**
- ObservableObject for SwiftUI binding
- @Published properties for all UI state
- Async refresh methods
- Timer-based auto-refresh

- [ ] **Step 1: Implement StatusViewModel**

```swift
// StatusViewModel.swift
import SwiftUI
import Combine

@MainActor
class StatusViewModel: ObservableObject {
    @Published var coreRunning = false
    @Published var corePID = 0
    @Published var systemProxyEnabled = false
    @Published var currentMode = "Rule"
    @Published var currentGroup = ""
    @Published var currentNode = ""
    @Published var lastError: String?
    @Published var isRefreshing = false
    @Published var testResult: TestResult?

    private let helper = HelperClient.shared
    private var refreshTimer: Timer?

    func startAutoRefresh(interval: TimeInterval = 5.0) {
        refreshTimer?.invalidate()
        refreshTimer = Timer.scheduledTimer(withTimeInterval: interval, repeats: true) { _ in
            Task { await self.refreshStatus() }
        }
        Task { await refreshStatus() }
    }

    func stopAutoRefresh() {
        refreshTimer?.invalidate()
        refreshTimer = nil
    }

    func refreshStatus() async {
        isRefreshing = true
        defer { isRefreshing = false }

        // Core status
        let coreResult = await helper.getCoreStatus()
        switch coreResult {
        case .success(let status):
            coreRunning = status.running
            corePID = status.pid
        case .failure(let error):
            lastError = "Core: \(error)"
        }

        // System proxy status
        let proxyResult = await helper.getSystemProxyStatus()
        switch proxyResult {
        case .success(let status):
            systemProxyEnabled = status.socksEnabled || status.httpEnabled
        case .failure:
            systemProxyEnabled = false
        }

        // Proxy groups for current selection
        let groupsResult = await helper.getProxyGroups()
        switch groupsResult {
        case .success(let groups):
            if let firstGroup = groups.first {
                currentGroup = firstGroup.name
                currentNode = firstGroup.now
                // Mode detection could be enhanced via GetConfig
            }
        case .failure:
            currentGroup = ""
            currentNode = ""
        }
    }

    func enableSystemProxy() async {
        let result = await helper.enableSystemProxy()
        if case .failure(let error) = result {
            lastError = "Enable proxy failed: \(error)"
        }
        await refreshStatus()
    }

    func disableSystemProxy() async {
        let result = await helper.disableSystemProxy()
        if case .failure(let error) = result {
            lastError = "Disable proxy failed: \(error)"
        }
        await refreshStatus()
    }

    func startCore() async {
        let result = await helper.startCore()
        if case .failure(let error) = result {
            lastError = "Start core failed: \(error)"
        }
        await refreshStatus()
    }

    func stopCore() async {
        let result = await helper.stopCore()
        if case .failure(let error) = result {
            lastError = "Stop core failed: \(error)"
        }
        await refreshStatus()
    }

    func restartCore() async {
        let result = await helper.restartCore()
        if case .failure(let error) = result {
            lastError = "Restart core failed: \(error)"
        }
        await refreshStatus()
    }

    func testConnection() async {
        let result = await helper.testConnection()
        switch result {
        case .success(let test):
            testResult = test
        case .failure(let error):
            lastError = "Test failed: \(error)"
            testResult = nil
        }
    }

    func updateSubscription() async {
        let result = await helper.updateSubscription()
        if case .failure(let error) = result {
            lastError = "Update failed: \(error)"
        }
    }

    func runDiagnose() async {
        let result = await helper.runDiagnose()
        switch result {
        case .success(let report):
            // Could show report in a window
            print("Diagnose: \(report.checks.count) checks")
        case .failure(let error):
            lastError = "Diagnose failed: \(error)"
        }
    }

    func openConfigFolder() {
        Task { await helper.openConfigFolder() }
    }

    func openLogs() {
        Task { await helper.openLogs() }
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add app/ProxyCat/ProxyCat/StatusViewModel.swift
git commit -m "feat: add StatusViewModel"
```

---

## Task 4: MenuBarController

**Files:**
- Create: `app/ProxyCat/ProxyCat/MenuBarController.swift`

**Design:**
- NSStatusBar integration
- NSMenu with SwiftUI hosting
- Status icon management
- Menu item management

- [ ] **Step 1: Implement MenuBarController**

```swift
// MenuBarController.swift
import SwiftUI
import AppKit

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

    deinit {
        viewModel.stopAutoRefresh()
    }

    private func setupStatusItem() {
        statusItem = NSStatusBar.shared.statusItem(withLength: NSStatusItem.variableLength)

        if let button = statusItem.button {
            button.image = NSImage(systemSymbolName: "cat", accessibilityDescription: "ProxyCat")
            button.image?.size = NSSize(width: 18, height: 18)
            button.target = self
            button.action = #selector(toggleMenu)
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
        if let button = statusItem.button,
           let screen = button.window?.screen {
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

        // Update icon to active
        statusItem.button?.image = NSImage(systemSymbolName: "cat.fill", accessibilityDescription: "ProxyCat Active")
    }

    private func hideMenu() {
        menuWindow?.orderOut(nil)
        menuWindow = nil

        // Update icon to inactive
        statusItem.button?.image = NSImage(systemSymbolName: "cat", accessibilityDescription: "ProxyCat")
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
        statusItem.button?.image = NSImage(systemSymbolName: iconName, accessibilityDescription: "ProxyCat")
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add app/ProxyCat/ProxyCat/MenuBarController.swift
git commit -m "feat: add MenuBarController"
```

---

## Task 5: MenuContentView (SwiftUI)

**Files:**
- Create: `app/ProxyCat/ProxyCat/MenuContentView.swift`

**Design:**
- SwiftUI view matching design spec menu layout
- Status section, Actions section, Diagnostics, Open, Quit
- Real-time status updates via ViewModel

- [ ] **Step 1: Implement MenuContentView**

```swift
// MenuContentView.swift
import SwiftUI

struct MenuContentView: View {
    @ObservedObject var viewModel: StatusViewModel
    @State private var showingProxyMenu = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Header - ProxyCat On/Off
            headerSection

            Divider()

            // Status section
            statusSection

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

            MenuButton("Open Last Report", icon: "doc.text") {
                // Open diagnose-latest.json
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
        .background(Color.accentColor.opacity(0))
        .onHover { isHovered in
            // Hover effect would go here
        }
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
```

- [ ] **Step 2: Commit**

```bash
git add app/ProxyCat/ProxyCat/MenuContentView.swift
git commit -m "feat: add MenuContentView"
```

---

## Task 6: App Entry Point

**Files:**
- Create: `app/ProxyCat/ProxyCat/ProxyCatApp.swift`

**Design:**
- SwiftUI App protocol
- MenuBarController as app delegate
- No main window (menu bar only)

- [ ] **Step 1: Implement ProxyCatApp**

```swift
// ProxyCatApp.swift
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

class AppDelegate: NSObject, NSApplicationDelegate {
    var menuBarController: MenuBarController?

    func applicationDidFinishLaunching(_ notification: Notification) {
        menuBarController = MenuBarController()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        return false  // Keep running in menu bar
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add app/ProxyCat/ProxyCat/ProxyCatApp.swift
git commit -m "feat: add app entry point"
```

---

## Task 7: Build Script for proxyctl

**Files:**
- Modify: `helper/` - Add build script
- Create: `scripts/build-app.sh`

**Design:**
- Build Go helper binary
- Copy to app Resources
- Build Xcode project

- [ ] **Step 1: Create build script**

```bash
#!/bin/bash
set -e

# scripts/build-app.sh

APP_DIR="app/ProxyCat"
HELPER_DIR="helper"
RESOURCES_DIR="$APP_DIR/ProxyCat/Resources"

# Build proxyctl for macOS native
echo "Building proxyctl..."
cd "$HELPER_DIR"
GOOS=darwin GOARCH=amd64 go build -o ../"$RESOURCES_DIR/proxyctl-amd64" ./cmd/proxyctl
GOOS=darwin GOARCH=arm64 go build -o ../"$RESOURCES_DIR/proxyctl-arm64" ./cmd/proxyctl
cd ..

# Create universal binary
echo "Creating universal binary..."
lipo -create \
    "$RESOURCES_DIR/proxyctl-amd64" \
    "$RESOURCES_DIR/proxyctl-arm64" \
    -output "$RESOURCES_DIR/proxyctl"

rm "$RESOURCES_DIR/proxyctl-amd64" "$RESOURCES_DIR/proxyctl-arm64"

echo "Build complete: $RESOURCES_DIR/proxyctl"
```

- [ ] **Step 2: Commit**

```bash
git add scripts/build-app.sh
git commit -m "chore: add app build script"
```

---

## Task 8: Final Milestone 4 Verification

- [ ] **Step 1: Build proxyctl and copy to Resources**

```bash
mkdir -p app/ProxyCat/ProxyCat/Resources
cd helper
GOOS=darwin GOARCH=arm64 go build -o ../app/ProxyCat/ProxyCat/Resources/proxyctl ./cmd/proxyctl
cd ..
```

- [ ] **Step 2: Build Xcode project**

```bash
cd app/ProxyCat
xcodebuild -project ProxyCat.xcodeproj -scheme ProxyCat -configuration Release -derivedDataPath build
```

- [ ] **Step 3: Test run app**

```bash
open build/Build/Products/Release/ProxyCat.app
```

- [ ] **Step 4: Verify menu items work**
- Check status display
- Toggle system proxy
- Test connection
- Open config/logs

- [ ] **Step 5: Commit plan document**

```bash
git add docs/superpowers/plans/2026-05-03-proxycat-milestone-4.md
git commit -m "docs: add milestone 4 implementation plan"
```

---

## Task 9: Push and Create PR

```bash
git push -u origin feat/milestone-4
```

Create PR with title "Add SwiftUI menu bar app" and description.

---

**Implementation Notes:**

1. **No sandbox**: App needs to execute proxyctl helper
2. **Menu bar only**: LSUIElement=true, no dock icon
3. **Universal binary**: Support both Intel and Apple Silicon
4. **Auto-refresh**: 5-second timer for status updates
5. **Error handling**: Display errors in menu via alert or status text
