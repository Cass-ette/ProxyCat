# ProxyCat Milestone 3 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Mihomo core management, controller API client, macOS system proxy control, and connectivity test command.

**Architecture:** Core management handles process lifecycle (start/stop/status). Controller client communicates with Mihomo's REST API (127.0.0.1:9090). System proxy uses `networksetup` commands. Test command performs connectivity checks through the proxy.

**Tech Stack:** Go, os/exec for process/networksetup, net/http for controller API.

---

## File Structure

```
helper/internal/
├── core/
│   ├── core.go          # Process lifecycle management
│   └── core_test.go     # Tests with mocked exec
├── controller/
│   ├── client.go        # Mihomo REST API client
│   └── client_test.go   # Tests with httptest
└── sysproxy/
    ├── sysproxy.go      # macOS system proxy control
    └── sysproxy_test.go # Tests with mocked exec
```

---

## Task 1: Core Process Management

**Files:**
- Create: `helper/internal/core/core.go`
- Create: `helper/internal/core/core_test.go`

**Design:**
- `Start(binPath, configPath, logPath string) (pid int, err error)` - Start Mihomo process with config
- `Stop(pid int) error` - Stop process by PID
- `Status() (running bool, pid int, err error)` - Check if Mihomo is running
- Use `os/exec` to spawn process, `pgrep` or pid file for status checking

- [ ] **Step 1: Write failing test**

```go
func TestStartStop(t *testing.T) {
    // Test with fake binary that just sleeps
    pid, err := Start("/bin/sleep", "config.yaml", "/tmp/test.log")
    if err != nil {
        t.Fatalf("start: %v", err)
    }
    if pid == 0 {
        t.Fatal("expected non-zero pid")
    }

    running, _, err := Status()
    if err != nil {
        t.Fatalf("status: %v", err)
    }
    if !running {
        t.Fatal("expected process to be running")
    }

    if err := Stop(pid); err != nil {
        t.Fatalf("stop: %v", err)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core -v`
Expected: FAIL with "core.Start not defined"

- [ ] **Step 3: Write minimal implementation**

```go
package core

import (
    "fmt"
    "os"
    "os/exec"
    "strconv"
    "strings"
)

func Start(binPath, configPath, logPath string) (int, error) {
    logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    if err != nil {
        return 0, fmt.Errorf("open log: %w", err)
    }
    defer logFile.Close()

    cmd := exec.Command(binPath, "-f", configPath)
    cmd.Stdout = logFile
    cmd.Stderr = logFile

    if err := cmd.Start(); err != nil {
        return 0, fmt.Errorf("start mihomo: %w", err)
    }

    return cmd.Process.Pid, nil
}

func Stop(pid int) error {
    process, err := os.FindProcess(pid)
    if err != nil {
        return fmt.Errorf("find process: %w", err)
    }
    return process.Kill()
}

func Status() (bool, int, error) {
    // Check for mihomo process using pgrep
    cmd := exec.Command("pgrep", "-x", "mihomo")
    out, _ := cmd.Output()
    if len(out) == 0 {
        return false, 0, nil
    }

    pidStr := strings.TrimSpace(string(out))
    pid, err := strconv.Atoi(pidStr)
    if err != nil {
        return false, 0, nil
    }

    return true, pid, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add helper/internal/core/
git commit -m "feat: add core process management"
```

---

## Task 2: Controller API Client

**Files:**
- Create: `helper/internal/controller/client.go`
- Create: `helper/internal/controller/client_test.go`

**Design:**
- `Client` struct with base URL (default: http://127.0.0.1:9090)
- `GetConfig() (*Config, error)` - Get current config from controller
- `GetProxies() (map[string]Proxy, error)` - Get proxy list
- `GetProxyGroups() ([]ProxyGroup, error)` - Get proxy groups
- `SelectProxy(group, proxy string) error` - Select proxy for group

- [ ] **Step 1: Write failing test**

```go
func TestGetConfig(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/configs" {
            t.Errorf("path = %s, want /configs", r.URL.Path)
        }
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"port":7890,"mode":"rule"}`))
    }))
    defer server.Close()

    client := NewClient(server.URL)
    cfg, err := client.GetConfig()
    if err != nil {
        t.Fatalf("get config: %v", err)
    }
    if cfg.Mode != "rule" {
        t.Errorf("mode = %s, want rule", cfg.Mode)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/controller -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
package controller

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

const defaultBaseURL = "http://127.0.0.1:9090"

type Client struct {
    baseURL string
    client  *http.Client
}

type Config struct {
    Port int    `json:"port"`
    Mode string `json:"mode"`
}

type Proxy struct {
    Name   string `json:"name"`
    Type   string `json:"type"`
    Server string `json:"server"`
}

type ProxyGroup struct {
    Name    string   `json:"name"`
    Type    string   `json:"type"`
    Proxies []string `json:"proxies"`
    Now     string   `json:"now"`
}

func NewClient(baseURL string) *Client {
    if baseURL == "" {
        baseURL = defaultBaseURL
    }
    return &Client{
        baseURL: baseURL,
        client:  &http.Client{Timeout: 5 * time.Second},
    }
}

func (c *Client) GetConfig() (*Config, error) {
    resp, err := c.client.Get(c.baseURL + "/configs")
    if err != nil {
        return nil, fmt.Errorf("get config: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("get config: status %d", resp.StatusCode)
    }

    var cfg Config
    if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
        return nil, fmt.Errorf("decode config: %w", err)
    }
    return &cfg, nil
}

func (c *Client) GetProxies() (map[string]Proxy, error) {
    resp, err := c.client.Get(c.baseURL + "/proxies")
    if err != nil {
        return nil, fmt.Errorf("get proxies: %w", err)
    }
    defer resp.Body.Close()

    var result struct {
        Proxies map[string]Proxy `json:"proxies"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("decode proxies: %w", err)
    }
    return result.Proxies, nil
}

func (c *Client) GetProxyGroups() ([]ProxyGroup, error) {
    proxies, err := c.GetProxies()
    if err != nil {
        return nil, err
    }

    var groups []ProxyGroup
    for name, p := range proxies {
        if p.Type == "Selector" || p.Type == "URLTest" || p.Type == "Fallback" {
            groups = append(groups, ProxyGroup{
                Name: name,
                Type: p.Type,
            })
        }
    }
    return groups, nil
}

func (c *Client) SelectProxy(group, proxy string) error {
    payload := map[string]string{"name": proxy}
    body, _ := json.Marshal(payload)

    resp, err := c.client.Put(c.baseURL+"/proxies/"+group, "application/json", bytes.NewReader(body))
    if err != nil {
        return fmt.Errorf("select proxy: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusNoContent {
        return fmt.Errorf("select proxy: status %d", resp.StatusCode)
    }
    return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/controller -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add helper/internal/controller/
git commit -m "feat: add controller API client"
```

---

## Task 3: System Proxy Control

**Files:**
- Create: `helper/internal/sysproxy/sysproxy.go`
- Create: `helper/internal/sysproxy/sysproxy_test.go`

**Design:**
- `Enable(port int) error` - Enable system proxy with given mixed-port
- `Disable() error` - Disable system proxy
- `Status() (*Status, error)` - Get current system proxy state using `scutil --proxy`
- Use `networksetup` to set proxy for all network services

- [ ] **Step 1: Write failing test**

```go
func TestStatusParsing(t *testing.T) {
    // Test parsing of scutil --proxy output
    output := `<dictionary> {
  HTTPProxy : 127.0.0.1
  HTTPPort : 7890
  HTTPSEnable : 1
}`
    status, err := parseStatus(output)
    if err != nil {
        t.Fatalf("parse: %v", err)
    }
    if !status.HTTPSEnabled {
        t.Error("expected HTTPS enabled")
    }
    if status.HTTPPort != 7890 {
        t.Errorf("port = %d, want 7890", status.HTTPPort)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/sysproxy -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
package sysproxy

import (
    "bufio"
    "fmt"
    "os/exec"
    "strconv"
    "strings"
)

type Status struct {
    Service      string `json:"service"`
    HTTPEnabled  bool   `json:"httpEnabled"`
    HTTPHost     string `json:"httpHost"`
    HTTPPort     int    `json:"httpPort"`
    HTTPSEnabled bool   `json:"httpsEnabled"`
    HTTPSHost    string `json:"httpsHost"`
    HTTPSPort    int    `json:"httpsPort"`
    SOCKSEnabled bool   `json:"socksEnabled"`
    SOCKSHost    string `json:"socksHost"`
    SOCKSPort    int    `json:"socksPort"`
}

func Enable(port int) error {
    services, err := getNetworkServices()
    if err != nil {
        return fmt.Errorf("get services: %w", err)
    }

    host := "127.0.0.1"
    for _, svc := range services {
        // Set HTTP proxy
        if err := exec.Command("networksetup", "-setwebproxy", svc, host, strconv.Itoa(port)).Run(); err != nil {
            return fmt.Errorf("set http proxy for %s: %w", svc, err)
        }
        // Set HTTPS proxy
        if err := exec.Command("networksetup", "-setsecurewebproxy", svc, host, strconv.Itoa(port)).Run(); err != nil {
            return fmt.Errorf("set https proxy for %s: %w", svc, err)
        }
        // Set SOCKS proxy
        if err := exec.Command("networksetup", "-setsocksfirewallproxy", svc, host, strconv.Itoa(port)).Run(); err != nil {
            return fmt.Errorf("set socks proxy for %s: %w", svc, err)
        }
    }
    return nil
}

func Disable() error {
    services, err := getNetworkServices()
    if err != nil {
        return fmt.Errorf("get services: %w", err)
    }

    for _, svc := range services {
        if err := exec.Command("networksetup", "-setwebproxystate", svc, "off").Run(); err != nil {
            return fmt.Errorf("disable http proxy for %s: %w", svc, err)
        }
        if err := exec.Command("networksetup", "-setsecurewebproxystate", svc, "off").Run(); err != nil {
            return fmt.Errorf("disable https proxy for %s: %w", svc, err)
        }
        if err := exec.Command("networksetup", "-setsocksfirewallproxystate", svc, "off").Run(); err != nil {
            return fmt.Errorf("disable socks proxy for %s: %w", svc, err)
        }
    }
    return nil
}

func Status() (*Status, error) {
    output, err := exec.Command("scutil", "--proxy").Output()
    if err != nil {
        return nil, fmt.Errorf("get proxy status: %w", err)
    }
    return parseStatus(string(output))
}

func getNetworkServices() ([]string, error) {
    output, err := exec.Command("networksetup", "-listallnetworkservices").Output()
    if err != nil {
        return nil, err
+    }

    var services []string
    lines := strings.Split(string(output), "\n")
    for i, line := range lines {
        // Skip header line
        if i == 0 || strings.HasPrefix(line, "An asterisk") {
            continue
        }
        line = strings.TrimSpace(line)
        if line != "" && !strings.HasPrefix(line, "*") {
            services = append(services, line)
        }
    }
    return services, nil
}

func parseStatus(output string) (*Status, error) {
    status := &Status{}
    scanner := bufio.NewScanner(strings.NewReader(output))

    for scanner.Scan() {
        line := scanner.Text()
        line = strings.TrimSpace(line)

        if strings.HasPrefix(line, "HTTPEnable") {
            status.HTTPEnabled = strings.Contains(line, ": 1")
        } else if strings.HasPrefix(line, "HTTPProxy") && !strings.HasPrefix(line, "HTTPEnable") {
            parts := strings.Split(line, ":")
            if len(parts) >= 2 {
                status.HTTPHost = strings.TrimSpace(parts[1])
            }
        } else if strings.HasPrefix(line, "HTTPPort") {
            parts := strings.Split(line, ":")
            if len(parts) >= 2 {
                portStr := strings.TrimSpace(parts[1])
                status.HTTPPort, _ = strconv.Atoi(portStr)
            }
        } else if strings.HasPrefix(line, "HTTPSEnable") {
            status.HTTPSEnabled = strings.Contains(line, ": 1")
        } else if strings.HasPrefix(line, "SOCKSEnable") {
            status.SOCKSEnabled = strings.Contains(line, ": 1")
        } else if strings.HasPrefix(line, "SOCKSPort") {
            parts := strings.Split(line, ":")
            if len(parts) >= 2 {
                portStr := strings.TrimSpace(parts[1])
                status.SOCKSPort, _ = strconv.Atoi(portStr)
            }
        }
    }

    return status, scanner.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/sysproxy -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add helper/internal/sysproxy/
git commit -m "feat: add system proxy control"
```

---

## Task 4: Test Command

**Files:**
- Modify: `helper/internal/controller/client.go` (add test method)
- Create: `helper/internal/controller/test.go`

**Design:**
- `TestConnection(url string) (*TestResult, error)` - Test connectivity through proxy
- Use http.Client with proxy URL set to http://127.0.0.1:7890
- Test Google (generate_204) and GitHub connectivity

- [ ] **Step 1: Write failing test**

```go
func TestTestConnection(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusNoContent)
    }))
    defer server.Close()

    result, err := TestConnection(server.URL, server.URL) // Use same URL for proxy
    if err != nil {
        t.Fatalf("test connection: %v", err)
    }
    if !result.GoogleOK {
        t.Error("expected google OK")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/controller -v -run TestTest`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
// test.go in controller package
package controller

import (
    "fmt"
    "net/http"
    "net/url"
    "time"
)

type TestResult struct {
    GoogleOK   bool   `json:"googleOK"`
    GitHubOK   bool   `json:"githubOK"`
    ProxyIP    string `json:"proxyIP,omitempty"`
    Error      string `json:"error,omitempty"`
}

func TestConnection(proxyURL string) (*TestResult, error) {
    result := &TestResult{}

    // Configure HTTP client with proxy
    proxy, _ := url.Parse(proxyURL)
    client := &http.Client{
        Timeout: 10 * time.Second,
        Transport: &http.Transport{
            Proxy: http.ProxyURL(proxy),
        },
    }

    // Test Google generate_204
    resp, err := client.Get("http://clients3.google.com/generate_204")
    if err != nil {
        result.Error = fmt.Sprintf("google test: %v", err)
    } else {
        resp.Body.Close()
        result.GoogleOK = resp.StatusCode == http.StatusNoContent
    }

    // Test GitHub
    resp, err = client.Get("https://github.com")
    if err != nil {
        if result.Error != "" {
            result.Error += "; "
        }
        result.Error += fmt.Sprintf("github test: %v", err)
    } else {
        resp.Body.Close()
        result.GitHubOK = resp.StatusCode == http.StatusOK
    }

    return result, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/controller -v -run TestTest`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add helper/internal/controller/test.go
git commit -m "feat: add connection test"
```

---

## Task 5: CLI Integration

**Files:**
- Modify: `helper/cmd/proxyctl/main.go` - Add core, system-proxy, groups, test commands

- [ ] **Step 1: Write failing test**

Add test in `main_test.go`:
```go
func TestCoreCommands(t *testing.T) {
    tests := []struct {
        name string
        args []string
        wantExit int
    }{
        {"core status", []string{"core", "status"}, 0},
        {"system-proxy status", []string{"system-proxy", "status"}, 0},
        {"test", []string{"test"}, 0},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            exit := run(tt.args, io.Discard, io.Discard)
            if exit != tt.wantExit {
                t.Errorf("exit = %d, want %d", exit, tt.wantExit)
            }
        })
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/proxyctl -v -run TestCore`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

Add in main.go:
```go
case "core":
    if len(args) < 2 {
        fmt.Fprintf(stderr, "core subcommand required: start, stop, restart, status\n")
        return 2
    }
    switch args[1] {
    case "start":
        return runCoreStart(stdout, stderr)
    case "stop":
        return runCoreStop(stdout, stderr)
    case "restart":
        return runCoreRestart(stdout, stderr)
    case "status":
        return runCoreStatus(stdout, stderr)
    }

case "system-proxy":
    if len(args) < 2 {
        fmt.Fprintf(stderr, "system-proxy subcommand required: on, off, status\n")
        return 2
    }
    switch args[1] {
    case "on":
        return runSystemProxyOn(stdout, stderr)
    case "off":
        return runSystemProxyOff(stdout, stderr)
    case "status":
        return runSystemProxyStatus(stdout, stderr)
    }

case "test":
    return runTest(stdout, stderr)
```

Implement helper functions:
```go
func runCoreStatus(stdout, stderr io.Writer) int {
    running, pid, err := core.Status()
    if err != nil {
        fmt.Fprintf(stderr, "check status: %v\n", err)
        return 1
    }
    if running {
        fmt.Fprintf(stdout, "Mihomo is running (pid: %d)\n", pid)
    } else {
        fmt.Fprintln(stdout, "Mihomo is not running")
    }
    return 0
}

func runSystemProxyStatus(stdout, stderr io.Writer) int {
    status, err := sysproxy.Status()
    if err != nil {
        fmt.Fprintf(stderr, "get status: %v\n", err)
        return 1
    }

    if jsonOutput {
        data, _ := json.Marshal(status)
        fmt.Fprintln(stdout, string(data))
        return 0
    }

    if status.SOCKSEnabled {
        fmt.Fprintf(stdout, "System proxy: ON (socks5://%s:%d)\n", status.SOCKSHost, status.SOCKSPort)
    } else {
        fmt.Fprintln(stdout, "System proxy: OFF")
    }
    return 0
}

func runTest(stdout, stderr io.Writer) int {
    result, err := controller.TestConnection("http://127.0.0.1:7890")
    if err != nil {
        fmt.Fprintf(stderr, "test: %v\n", err)
        return 1
    }

    if jsonOutput {
        data, _ := json.Marshal(result)
        fmt.Fprintln(stdout, string(data))
        return 0
    }

    if result.GoogleOK {
        fmt.Fprintln(stdout, "Google: OK")
    } else {
        fmt.Fprintln(stdout, "Google: FAIL")
    }
    if result.GitHubOK {
        fmt.Fprintln(stdout, "GitHub: OK")
    } else {
        fmt.Fprintln(stdout, "GitHub: FAIL")
    }
    return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/proxyctl -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add helper/cmd/proxyctl/main.go helper/cmd/proxyctl/main_test.go
git commit -m "feat: add core, system-proxy, and test CLI commands"
```

---

## Task 6: Final Milestone 3 Verification

- [ ] **Step 1: Verify branch diff scope**

Run: `git status` and `git diff --stat origin/main...HEAD`
Expected: Only Milestone 3 related files

- [ ] **Step 2: Run full tests**

Run: `go test ./...`
Expected: All tests pass

- [ ] **Step 3: Build helper**

Run: `go build ./cmd/proxyctl`
Expected: Binary created

- [ ] **Step 4: Manual CLI verification**

Run these commands and verify output:
```bash
./proxyctl core status
./proxyctl system-proxy status --json
./proxyctl test --json
```

- [ ] **Step 5: Commit plan document**

```bash
git add docs/superpowers/plans/2026-05-03-proxycat-milestone-3.md
git commit -m "docs: add milestone 3 implementation plan"
```

---

## Task 7: Push and Create PR

- [ ] **Push branch and create PR**

```bash
git push -u origin feat/milestone-3
```

Create PR with title "Add core management, system proxy, and connectivity test" and description covering:
- Core process start/stop/status
- Controller API client for proxies/groups
- System proxy enable/disable on macOS
- Connectivity test command
- CLI integration for all new features

---

**Note:** Some commands (core start, system-proxy on) may fail on machines without Mihomo binary or when not on macOS. This is expected - the tests mock these dependencies.
