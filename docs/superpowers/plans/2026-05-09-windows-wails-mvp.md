# Windows Wails MVP Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Windows desktop client for ProxyCat using Wails (Go + Web frontend) that matches macOS MVP functionality: paste subscription, generate config, start mihomo, toggle system proxy, select nodes, switch modes, test connection, run diagnostics.

**Architecture:** Reuse the existing Go helper packages (subscription, config, controller, redact, diagnose) with targeted Windows adaptations for paths, core management, system proxy, and mihomo installation. Add a Wails v2 project for the desktop UI with system tray. Go backend serves as the Wails binding layer, calling existing helper logic.

**Tech Stack:** Go 1.21+, Wails v2, vanilla HTML/CSS/JS (keep frontend simple for MVP), existing helper packages.

---

## File Structure

### New files (Wails project)

```
app/ProxyCatWin/
├── wails.json                    # Wails project config
├── main.go                       # Wails app entry point
├── app.go                        # Wails bindings (exposes Go methods to frontend)
├── systray.go                    # System tray setup
├── frontend/
│   ├── index.html                # Main UI
│   ├── style.css                 # Styles
│   └── main.js                   # Frontend logic, calls Wails bindings
└── build/
    └── windows/                  # Windows build config, icon
```

### Modified files (cross-platform adaptation)

```
helper/internal/paths/paths.go         # Add Windows data directory
helper/internal/paths/paths_test.go    # Add Windows path test
helper/internal/core/core.go           # Replace pgrep with cross-platform process lookup
helper/internal/core/core_test.go      # Cross-platform test process
helper/internal/core/install.go        # Add Windows mihomo download (zip instead of gz)
helper/internal/core/install_test.go   # Add Windows asset name test
helper/internal/sysproxy/sysproxy.go   # Add build tag split: sysproxy_darwin.go + sysproxy_windows.go
helper/internal/sysproxy/sysproxy_test.go  # Platform-specific test split
```

### Unchanged files (directly reusable)

```
helper/internal/subscription/*     # All files - download, storage
helper/internal/config/*           # All files - format, convert, validate, backup
helper/internal/controller/*       # All files - client, test
helper/internal/redact/*           # All files
helper/internal/diagnose/*         # All files
```

---

## Chunk 1: Cross-Platform Go Helper

Adapt existing helper packages to compile and work on Windows. Each task produces a compilable, testable change.

### Task 1: Cross-platform paths

**Files:**
- Modify: `helper/internal/paths/paths.go`
- Modify: `helper/internal/paths/paths_test.go`

- [ ] **Step 1: Write the failing test**

Add to `paths_test.go`:

```go
func TestForHomeWindows(t *testing.T) {
    base := ForHome("windows")
    expected := filepath.Join(os.Getenv("LOCALAPPDATA"), "ProxyCat")
    if os.Getenv("LOCALAPPDATA") == "" {
        home, _ := os.UserHomeDir()
        expected = filepath.Join(home, "AppData", "Local", "ProxyCat")
    }
    if base != expected {
        t.Fatalf("ForHome(windows) = %q, want %q", base, expected)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd helper && go test ./internal/paths/ -run TestForHomeWindows -v`
Expected: FAIL - `ForHome` does not accept platform argument

- [ ] **Step 3: Implement**

Refactor `paths.go`:

```go
func ForHome(goos string) string {
    if goos == "windows" {
        local := os.Getenv("LOCALAPPDATA")
        if local == "" {
            home, _ := os.UserHomeDir()
            local = filepath.Join(home, "AppData", "Local")
        }
        return filepath.Join(local, "ProxyCat")
    }
    home, _ := os.UserHomeDir()
    return filepath.Join(home, "Library", "Application Support", "ProxyCat")
}

func Default() (*RuntimePaths, error) {
    return fromBase(ForHome(runtime.GOOS))
}

func fromBase(base string) (*RuntimePaths, error) {
    // existing logic unchanged, just uses base parameter
}
```

Update `Default()` and `ForHome()` so existing macOS callers still work. Extract the body into `fromBase()`.

- [ ] **Step 4: Run all paths tests**

Run: `cd helper && go test ./internal/paths/ -v`
Expected: ALL PASS (macOS + new Windows test)

- [ ] **Step 5: Commit**

```bash
git add helper/internal/paths/
git commit -m "feat(paths): add Windows data directory support"
```

### Task 2: Cross-platform process status

**Files:**
- Modify: `helper/internal/core/core.go`
- Modify: `helper/internal/core/core_test.go`

- [ ] **Step 1: Write the failing test**

Add a test that verifies `Status()` works on the current platform:

```go
func TestStatusNoProcess(t *testing.T) {
    running, pid, err := Status()
    if err != nil {
        t.Fatalf("Status() error: %v", err)
    }
    // No mihomo process should be running in test
    // Just verify it doesn't crash on any platform
    _ = running
    _ = pid
}
```

- [ ] **Step 2: Run test**

Run: `cd helper && go test ./internal/core/ -run TestStatusNoProcess -v`

- [ ] **Step 3: Replace pgrep with cross-platform approach**

Replace `Status()` in `core.go`:

```go
func Status() (bool, int, error) {
    if runtime.GOOS == "windows" {
        return windowsStatus()
    }
    return unixStatus()
}

func unixStatus() (bool, int, error) {
    output, err := exec.Command("pgrep", "-x", "mihomo").Output()
    if err != nil {
        return false, 0, nil
    }
    // ... existing parsing logic
}

func windowsStatus() (bool, int, error) {
    output, err := exec.Command("tasklist", "/FI", "IMAGENAME eq mihomo.exe", "/FO", "CSV", "/NH").Output()
    if err != nil {
        return false, 0, nil
    }
    line := strings.TrimSpace(string(output))
    if strings.Contains(line, "mihomo.exe") {
        // Parse PID from CSV: "mihomo.exe","12345","Console","1","5,632 K"
        parts := strings.Split(line, ",")
        if len(parts) >= 2 {
            pidStr := strings.Trim(parts[1], "\"")
            pid, err := strconv.Atoi(pidStr)
            if err == nil {
                return true, pid, nil
            }
        }
        return true, 0, nil
    }
    return false, 0, nil
}
```

Also update `Start()` to use `mihomo.exe` on Windows and `mihomo` elsewhere.

- [ ] **Step 4: Run all core tests**

Run: `cd helper && go test ./internal/core/ -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add helper/internal/core/core.go helper/internal/core/core_test.go
git commit -m "feat(core): cross-platform process status (pgrep + tasklist)"
```

### Task 3: Windows mihomo download

**Files:**
- Modify: `helper/internal/core/install.go`
- Modify: `helper/internal/core/install_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestWindowsAssetName(t *testing.T) {
    asset := windowsAssetName("amd64", "v1.19.24")
    want := "mihomo-windows-amd64-v1.19.24.zip"
    if asset != want {
        t.Fatalf("got %q, want %q", asset, want)
    }
}
```

- [ ] **Step 2: Run test**

Run: `cd helper && go test ./internal/core/ -run TestWindowsAssetName -v`
Expected: FAIL

- [ ] **Step 3: Implement**

Add to `install.go`:

```go
func windowsAssetName(arch, version string) string {
    return fmt.Sprintf("mihomo-windows-%s-%s.zip", arch, version)
}

func latestReleaseURL(goos, arch string) (string, string, error) {
    version, err := fetchLatestVersion()
    if err != nil {
        return "", "", err
    }
    var asset string
    switch goos {
    case "darwin":
        asset = darwinMihomoAssetName(arch, version)
    case "windows":
        asset = windowsAssetName(arch, version)
    default:
        return "", "", fmt.Errorf("unsupported OS: %s", goos)
    }
    url := fmt.Sprintf("https://github.com/MetaCubeX/mihomo/releases/download/%s/%s", version, asset)
    return url, version, nil
}

func Installed(binPath string) bool {
    info, err := os.Stat(binPath)
    if err != nil {
        return false
    }
    if runtime.GOOS == "windows" {
        return info.Size() > 0
    }
    return info.Mode()&0o111 != 0
}
```

Update `InstallMihomo` to handle `.zip` on Windows (use `archive/zip` instead of `compress/gzip`).

- [ ] **Step 4: Run all core tests**

Run: `cd helper && go test ./internal/core/ -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add helper/internal/core/install.go helper/internal/core/install_test.go
git commit -m "feat(core): Windows mihomo download (zip asset + exe check)"
```

### Task 4: Windows system proxy (registry)

**Files:**
- Rename: `helper/internal/sysproxy/sysproxy.go` → `helper/internal/sysproxy/sysproxy_darwin.go`
- Create: `helper/internal/sysproxy/sysproxy_windows.go`
- Rename: `helper/internal/sysproxy/sysproxy_test.go` → `helper/internal/sysproxy/sysproxy_darwin_test.go`
- Create: `helper/internal/sysproxy/sysproxy_windows_test.go`

- [ ] **Step 1: Split darwin file**

Add build tag to existing `sysproxy.go` (rename to `sysproxy_darwin.go`):

```go
//go:build darwin

package sysproxy
// ... existing code unchanged
```

- [ ] **Step 2: Write the failing Windows test**

Create `sysproxy_windows_test.go`:

```go
//go:build windows

package sysproxy

import "testing"

func TestWindowsProxyRegistryRoundTrip(t *testing.T) {
    // Enable proxy
    if err := Enable(7890); err != nil {
        t.Fatalf("Enable: %v", err)
    }
    status, err := GetStatus()
    if err != nil {
        t.Fatalf("GetStatus: %v", err)
    }
    if !status.HTTPEnabled {
        t.Fatal("expected HTTP proxy enabled")
    }
    if status.HTTPPort != 7890 {
        t.Fatalf("expected port 7890, got %d", status.HTTPPort)
    }
    // Disable proxy
    if err := Disable(); err != nil {
        t.Fatalf("Disable: %v", err)
    }
    status2, err := GetStatus()
    if err != nil {
        t.Fatalf("GetStatus after disable: %v", err)
    }
    if status2.HTTPEnabled {
        t.Fatal("expected HTTP proxy disabled")
    }
}
```

- [ ] **Step 3: Implement Windows sysproxy**

Create `sysproxy_windows.go`:

```go
//go:build windows

package sysproxy

import (
    "fmt"
    "golang.org/x/sys/windows/registry"
    "strconv"
)

const internetSettingsPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Internet Settings`

func Enable(port int) error {
    k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.SET_VALUE)
    if err != nil {
        return fmt.Errorf("open registry: %w", err)
    }
    defer k.Close()

    proxyAddr := "127.0.0.1:" + strconv.Itoa(port)
    if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
        return fmt.Errorf("set ProxyEnable: %w", err)
    }
    if err := k.SetStringValue("ProxyServer", proxyAddr); err != nil {
        return fmt.Errorf("set ProxyServer: %w", err)
    }
    return refreshProxy()
}

func Disable() error {
    k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.SET_VALUE)
    if err != nil {
        return fmt.Errorf("open registry: %w", err)
    }
    defer k.Close()

    if err := k.SetDWordValue("ProxyEnable", 0); err != nil {
        return fmt.Errorf("set ProxyEnable: %w", err)
    }
    return refreshProxy()
}

func GetStatus() (*Status, error) {
    k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.READ)
    if err != nil {
        return nil, fmt.Errorf("open registry: %w", err)
    }
    defer k.Close()

    enabled, _, err := k.GetIntegerValue("ProxyEnable")
    if err != nil {
        return nil, fmt.Errorf("read ProxyEnable: %w", err)
    }

    proxyServer, _, err := k.GetStringValue("ProxyServer")
    if err != nil {
        proxyServer = ""
    }

    status := &Status{}
    isOn := enabled == 1
    status.HTTPEnabled = isOn
    status.HTTPSEnabled = isOn
    status.SOCKSEnabled = false

    if isOn && proxyServer != "" {
        host, portStr, _ := net.SplitHostPort(proxyServer)
        port, _ := strconv.Atoi(portStr)
        status.HTTPHost = host
        status.HTTPPort = port
        status.HTTPSHost = host
        status.HTTPSPort = port
    }

    return status, nil
}

func refreshProxy() error {
    // Notify WinINet of proxy change
    // Using InternetSetOption via syscall or just skip for MVP
    return nil
}
```

Add `golang.org/x/sys` dependency:

Run: `cd helper && go get golang.org/x/sys/windows`

- [ ] **Step 4: Run tests on current platform**

Run: `cd helper && go test ./internal/sysproxy/ -v`

On macOS: darwin tests pass, windows file not compiled.
On Windows: windows tests pass, darwin file not compiled.

- [ ] **Step 5: Commit**

```bash
git add helper/internal/sysproxy/ helper/go.mod helper/go.sum
git commit -m "feat(sysproxy): Windows registry-based system proxy control"
```

### Task 5: Cross-platform helper compiles on Windows

**Files:**
- Modify: `helper/internal/core/core.go` (mihomo binary name)
- Modify: `helper/cmd/proxyctl/main.go` (minor platform guards if needed)

- [ ] **Step 1: Verify full build on current platform**

Run: `cd helper && go build ./... && go test ./...`
Expected: ALL PASS on macOS

- [ ] **Step 2: Verify Windows cross-compilation**

Run: `cd helper && GOOS=windows GOARCH=amd64 go build ./cmd/proxyctl`
Expected: builds without errors, produces `proxyctl.exe`

- [ ] **Step 3: Fix any compilation errors**

Common issues:
- `exec.Command("pgrep")` not guarded → already fixed in Task 2
- `networksetup` references not guarded → already fixed in Task 4
- Unix-specific test helpers → add build tags

- [ ] **Step 4: Run all tests**

Run: `cd helper && go test ./...`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add -A helper/
git commit -m "chore: verify helper cross-compiles for Windows"
```

---

## Chunk 2: Wails Project Setup

### Task 6: Initialize Wails project

**Files:**
- Create: `app/ProxyCatWin/` (entire Wails scaffold)

- [ ] **Step 1: Install Wails CLI**

Run: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

- [ ] **Step 2: Initialize project**

Run: `wails init -n ProxyCatWin -d app/ -t vanilla`

This creates `app/ProxyCatWin/` with the default vanilla template.

- [ ] **Step 3: Verify template builds**

Run: `cd app/ProxyCatWin && wails build`
Expected: produces `build/bin/ProxyCatWin.exe` (or `.app` on macOS)

- [ ] **Step 4: Commit**

```bash
git add app/ProxyCatWin/
git commit -m "chore: initialize Wails project for Windows client"
```

### Task 7: Wire Go backend to Wails bindings

**Files:**
- Create: `app/ProxyCatWin/app.go`
- Modify: `app/ProxyCatWin/main.go`

- [ ] **Step 1: Create App struct with Wails bindings**

Create `app.go`:

```go
package main

import (
    "context"
    "fmt"
    "github.com/Cass-ette/ProxyCat/helper/internal/config"
    "github.com/Cass-ette/ProxyCat/helper/internal/controller"
    "github.com/Cass-ette/ProxyCat/helper/internal/core"
    "github.com/Cass-ette/ProxyCat/helper/internal/paths"
    "github.com/Cass-ette/ProxyCat/helper/internal/subscription"
    "github.com/Cass-ette/ProxyCat/helper/internal/sysproxy"
    "os"
    "os/exec"
    "path/filepath"
    "time"
)

type App struct {
    ctx context.Context
}

func NewApp() *App {
    return &App{}
}

func (a *App) startup(ctx context.Context) {
    a.ctx = ctx
}

// Bootstrap runs the full setup: save sub, generate config, install core, start, enable proxy, test
func (a *App) Bootstrap(subscriptionURL string) string {
    runtimePaths, err := paths.Default()
    if err != nil {
        return fmt.Sprintf("Error: %v", err)
    }

    // Save subscription
    records := []subscription.Record{{URL: subscriptionURL, Name: "Subscription", LastUpdate: time.Now()}}
    if err := os.MkdirAll(filepath.Dir(runtimePaths.SubscriptionsJSON), 0755); err != nil {
        return fmt.Sprintf("Error: %v", err)
    }
    if err := subscription.Save(runtimePaths.SubscriptionsJSON, records); err != nil {
        return fmt.Sprintf("Error: %v", err)
    }

    // Generate config
    client := &http.Client{}
    content, err := subscription.Download(client, subscriptionURL, "Clash.Meta/1.18.0")
    if err != nil {
        return fmt.Sprintf("Error downloading subscription: %v", err)
    }

    format, _ := config.DetectFormat(content)
    var configYAML string
    switch format {
    case config.FormatClashYAML:
        configYAML, err = config.NormalizeClashYAML(content)
    case config.FormatBase64List, config.FormatPlainList:
        configYAML, err = config.ConvertURIToYAML(content, format)
    default:
        return "Error: unknown subscription format"
    }
    if err != nil {
        return fmt.Sprintf("Error generating config: %v", err)
    }

    if err := os.MkdirAll(filepath.Dir(runtimePaths.ConfigYAML), 0755); err != nil {
        return fmt.Sprintf("Error: %v", err)
    }
    if err := os.WriteFile(runtimePaths.ConfigYAML, []byte(configYAML), 0644); err != nil {
        return fmt.Sprintf("Error writing config: %v", err)
    }

    // Install & start core
    if _, err := core.InstallMihomo(runtimePaths.Mihomo); err != nil {
        return fmt.Sprintf("Error installing mihomo: %v", err)
    }
    if err := os.MkdirAll(runtimePaths.Logs, 0755); err != nil {
        return fmt.Sprintf("Error: %v", err)
    }
    if _, err := core.Start(runtimePaths.Mihomo, runtimePaths.ConfigYAML, runtimePaths.MihomoLog); err != nil {
        return fmt.Sprintf("Error starting mihomo: %v", err)
    }

    return "ProxyCat connected"
}

// GetStatus returns current proxy status
func (a *App) GetStatus() map[string]interface{} {
    runtimePaths, _ := paths.Default()
    running, pid, _ := core.Status()
    proxyStatus, _ := sysproxy.GetStatus()
    return map[string]interface{}{
        "coreRunning":      running,
        "corePID":          pid,
        "systemProxyOn":    proxyStatus != nil && proxyStatus.HTTPEnabled,
        "configExists":     fileExists(runtimePaths.ConfigYAML),
    }
}

func fileExists(path string) bool {
    _, err := os.Stat(path)
    return err == nil
}
```

Note: This is a simplified version. Real implementation will need more methods (EnableProxy, DisableProxy, SelectProxy, SetMode, TestConnection, etc.) matching the macOS StatusViewModel API.

- [ ] **Step 2: Update main.go to wire App**

```go
package main

import (
    "embed"
    "github.com/wailsapp/wails/v2"
    "github.com/wailsapp/wails/v2/pkg/options"
    "github.com/wailsapp/wails/v2/pkg/options/assetserver"
    "github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
    app := NewApp()

    err := wails.Run(&options.App{
        Title:  "ProxyCat",
        Width:  360,
        Height: 600,
        AssetServer: &assetserver.Options{
            Assets: assets,
        },
        OnStartup:  app.startup,
        Bind: []interface{}{
            app,
        },
        Windows: &windows.Options{
            WebviewIsTransparent: false,
            WindowIsTranslucent:  false,
        },
    })

    if err != nil {
        println("Error:", err.Error())
    }
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd app/ProxyCatWin && go build .`
Expected: compiles without errors

- [ ] **Step 4: Commit**

```bash
git add app/ProxyCatWin/
git commit -m "feat(wails): wire Go backend bindings to Wails app"
```

---

## Chunk 3: Frontend UI

### Task 8: Build frontend matching macOS MVP

**Files:**
- Create: `app/ProxyCatWin/frontend/index.html`
- Create: `app/ProxyCatWin/frontend/style.css`
- Create: `app/ProxyCatWin/frontend/main.js`

- [ ] **Step 1: Create index.html**

Minimal structure matching macOS MenuContentView sections:
- Header: ProxyCat status (connected/disconnected)
- Quick start: subscription URL input + bootstrap button
- Status: core running, system proxy, mode, current node
- Controls: mode buttons (rule/global/direct) + node selection
- Actions: toggle proxy, update subscription, test connection, restart core
- Diagnostics: run diagnose button

- [ ] **Step 2: Create style.css**

Clean, minimal dark theme. Keep it simple - single CSS file, no framework. Match macOS menu bar compact layout in a 360px wide window.

- [ ] **Step 3: Create main.js**

```javascript
// Wails runtime imports
import { Bootstrap, GetStatus, EnableProxy, DisableProxy, UpdateSubscription,
         TestConnection, RestartCore, RunDiagnose, SetMode, SelectProxy, GetProxyGroups } from '../wailsjs/go/main/App.js';
import { EventsOn } from '../wailsjs/runtime/runtime.js';

// State
let status = {};

// UI update functions
async function refreshStatus() {
    status = await GetStatus();
    renderStatus(status);
}

// Bootstrap flow
async function doBootstrap() {
    const url = document.getElementById('subscriptionURL').value;
    if (!url) return;
    showLoading(true);
    const result = await Bootstrap(url);
    showLoading(false);
    showResult(result);
    refreshStatus();
}

// ... event listeners for all buttons
```

- [ ] **Step 4: Verify frontend loads**

Run: `cd app/ProxyCatWin && wails dev`
Expected: window opens with UI visible

- [ ] **Step 5: Commit**

```bash
git add app/ProxyCatWin/frontend/
git commit -m "feat(wails): frontend UI matching macOS MVP layout"
```

---

## Chunk 4: System Tray + Packaging

### Task 9: Add system tray

**Files:**
- Create: `app/ProxyCatWin/systray.go`
- Modify: `app/ProxyCatWin/main.go`

- [ ] **Step 1: Add tray support**

Use `github.com/wailsapp/wails/v2/pkg/menu` and `wails/v2/pkg/menu/tray` for Windows system tray. Show ProxyCat icon in tray, right-click menu with: Show Window, Enable/Disable Proxy, Quit.

- [ ] **Step 2: Verify tray appears on Windows**

Test on Windows machine: tray icon visible, right-click menu works.

- [ ] **Step 3: Commit**

```bash
git add app/ProxyCatWin/
git commit -m "feat(wails): system tray with proxy toggle"
```

### Task 10: Windows build and packaging script

**Files:**
- Create: `scripts/package-windows.sh` (or `.bat` for Windows)

- [ ] **Step 1: Create packaging script**

```bash
#!/bin/bash
set -euo pipefail
# Cross-compile from macOS
cd app/ProxyCatWin
wails build -platform windows/amd64
# Package
cp -r build/bin/ProxyCatWin.exe ../dist/ProxyCat.exe
cd ../../
zip -j dist/ProxyCat-Windows-0.1.0.zip dist/ProxyCat.exe
```

- [ ] **Step 2: Test build**

Run: `cd app/ProxyCatWin && wails build -platform windows/amd64`
Expected: produces `ProxyCatWin.exe`

- [ ] **Step 3: Commit**

```bash
git add scripts/package-windows.sh
git commit -m "chore: add Windows packaging script"
```

---

## Summary

| Chunk | Tasks | What it delivers |
|-------|-------|------------------|
| 1 | 1-5 | Go helper cross-compiles for Windows, all tests pass |
| 2 | 6-7 | Wails project scaffolding + Go backend bindings wired |
| 3 | 8 | Frontend UI matching macOS MVP |
| 4 | 9-10 | System tray + Windows packaging |

**Chunk 1** is the highest priority and can be done entirely on macOS with cross-compilation verification. Chunks 2-4 require testing on the actual Windows machine.
