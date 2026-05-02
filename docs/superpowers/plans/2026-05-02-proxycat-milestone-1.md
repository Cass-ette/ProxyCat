# ProxyCat Milestone 1 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first testable `proxyctl` helper milestone: Go module skeleton, runtime path definitions, redaction utilities, and a static `diagnose --json` command.

**Architecture:** The Go helper lives under `helper/` and keeps operational behavior testable without SwiftUI. Milestone 1 creates only the files needed now: a small CLI entrypoint, a `paths` package for ProxyCat runtime locations, a `redact` package for safe display/logging, and a `diagnose` package that returns static local checks without network, system proxy mutation, or Mihomo process control.

**Tech Stack:** Go 1.24 standard library only, `go test ./...`, JSON output via `encoding/json`, file/path checks via `os` and `path/filepath`.

---

## Chunk 1: Helper Module, Paths, Redaction, Diagnose Skeleton

### File Structure

Create these files only:

- `helper/go.mod`
  - Declares module `github.com/Cass-ette/ProxyCat/helper` and Go version.
- `helper/internal/paths/paths.go`
  - Owns ProxyCat runtime path construction under `~/Library/Application Support/ProxyCat`.
  - Provides explicit fields for `bin`, `mihomo`, `proxyctl`, `config`, `config.yaml`, `subscriptions.json`, `backups`, `logs`, `proxycat.log`, `mihomo.log`, `reports`, and `diagnose-latest.json`.
- `helper/internal/paths/paths_test.go`
  - Verifies path construction from a fake home directory.
- `helper/internal/redact/redact.go`
  - Owns redaction for URLs and node URI strings.
  - No logging, no network, no subscription parsing.
- `helper/internal/redact/redact_test.go`
  - Verifies query token redaction, URL userinfo redaction, node URI userinfo redaction, VMess base64 JSON redaction, and idempotent handling of non-secret text.
- `helper/internal/diagnose/diagnose.go`
  - Owns the static diagnostic report shape and local file existence checks.
  - Does not download subscriptions, start Mihomo, mutate system proxy, or test external connectivity in Milestone 1.
- `helper/internal/diagnose/diagnose_test.go`
  - Verifies result aggregation and JSON-safe report shape.
- `helper/cmd/proxyctl/main.go`
  - Minimal CLI entrypoint supporting `diagnose --json`, `diagnose`, and `help`.
  - Unknown commands return a concise error without exposing input secrets.
- `helper/cmd/proxyctl/main_test.go`
  - Verifies `diagnose --json` output parses as JSON and does not include injected secret-like home path data.

Do not create SwiftUI files, subscription packages, config generation packages, core/system proxy/controller packages, scripts, README, or GitHub Actions in this milestone.

### Task 1: Create Go Module and Runtime Paths

**Files:**
- Create: `helper/go.mod`
- Create: `helper/internal/paths/paths.go`
- Create: `helper/internal/paths/paths_test.go`

- [ ] **Step 1: Create Go module file**

Create `helper/go.mod`:

```go
module github.com/Cass-ette/ProxyCat/helper

go 1.24
```

- [ ] **Step 2: Write failing path tests**

Create `helper/internal/paths/paths_test.go`:

```go
package paths

import "testing"

func TestForHomeBuildsProxyCatRuntimePaths(t *testing.T) {
	p := ForHome("/Users/example")

	wantBase := "/Users/example/Library/Application Support/ProxyCat"
	if p.Base != wantBase {
		t.Fatalf("Base = %q, want %q", p.Base, wantBase)
	}
	if p.Bin != wantBase+"/bin" {
		t.Fatalf("Bin = %q", p.Bin)
	}
	if p.Proxyctl != wantBase+"/bin/proxyctl" {
		t.Fatalf("Proxyctl = %q", p.Proxyctl)
	}
	if p.Mihomo != wantBase+"/bin/mihomo" {
		t.Fatalf("Mihomo = %q", p.Mihomo)
	}
	if p.Config != wantBase+"/config" {
		t.Fatalf("Config = %q", p.Config)
	}
	if p.ConfigYAML != wantBase+"/config/config.yaml" {
		t.Fatalf("ConfigYAML = %q", p.ConfigYAML)
	}
	if p.SubscriptionsJSON != wantBase+"/config/subscriptions.json" {
		t.Fatalf("SubscriptionsJSON = %q", p.SubscriptionsJSON)
	}
	if p.Backups != wantBase+"/config/backups" {
		t.Fatalf("Backups = %q", p.Backups)
	}
	if p.Logs != wantBase+"/logs" {
		t.Fatalf("Logs = %q", p.Logs)
	}
	if p.ProxyCatLog != wantBase+"/logs/proxycat.log" {
		t.Fatalf("ProxyCatLog = %q", p.ProxyCatLog)
	}
	if p.MihomoLog != wantBase+"/logs/mihomo.log" {
		t.Fatalf("MihomoLog = %q", p.MihomoLog)
	}
	if p.Reports != wantBase+"/reports" {
		t.Fatalf("Reports = %q", p.Reports)
	}
	if p.DiagnoseLatest != wantBase+"/reports/diagnose-latest.json" {
		t.Fatalf("DiagnoseLatest = %q", p.DiagnoseLatest)
	}
}

func TestDefaultUsesUserHomeDir(t *testing.T) {
	t.Setenv("HOME", "/tmp/proxycat-home")

	p, err := Default()
	if err != nil {
		t.Fatalf("Default returned error: %v", err)
	}
	if p.Base != "/tmp/proxycat-home/Library/Application Support/ProxyCat" {
		t.Fatalf("Base = %q", p.Base)
	}
}
```

- [ ] **Step 3: Run path tests to verify they fail**

Run:

```bash
go test ./internal/paths -run Test -v
```

from `helper/`.

Expected: FAIL because `ForHome`, `Default`, and `RuntimePaths` do not exist yet.

- [ ] **Step 4: Implement runtime paths**

Create `helper/internal/paths/paths.go`:

```go
package paths

import (
	"os"
	"path/filepath"
)

const appName = "ProxyCat"

type RuntimePaths struct {
	Base              string `json:"base"`
	Bin               string `json:"bin"`
	Proxyctl          string `json:"proxyctl"`
	Mihomo            string `json:"mihomo"`
	Config            string `json:"config"`
	ConfigYAML        string `json:"configYaml"`
	SubscriptionsJSON string `json:"subscriptionsJson"`
	Backups           string `json:"backups"`
	Logs              string `json:"logs"`
	ProxyCatLog       string `json:"proxycatLog"`
	MihomoLog         string `json:"mihomoLog"`
	Reports           string `json:"reports"`
	DiagnoseLatest    string `json:"diagnoseLatest"`
}

func Default() (RuntimePaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return RuntimePaths{}, err
	}
	return ForHome(home), nil
}

func ForHome(home string) RuntimePaths {
	base := filepath.Join(home, "Library", "Application Support", appName)
	bin := filepath.Join(base, "bin")
	config := filepath.Join(base, "config")
	logs := filepath.Join(base, "logs")
	reports := filepath.Join(base, "reports")

	return RuntimePaths{
		Base:              base,
		Bin:               bin,
		Proxyctl:          filepath.Join(bin, "proxyctl"),
		Mihomo:            filepath.Join(bin, "mihomo"),
		Config:            config,
		ConfigYAML:        filepath.Join(config, "config.yaml"),
		SubscriptionsJSON: filepath.Join(config, "subscriptions.json"),
		Backups:           filepath.Join(config, "backups"),
		Logs:              logs,
		ProxyCatLog:       filepath.Join(logs, "proxycat.log"),
		MihomoLog:         filepath.Join(logs, "mihomo.log"),
		Reports:           reports,
		DiagnoseLatest:    filepath.Join(reports, "diagnose-latest.json"),
	}
}
```

- [ ] **Step 5: Run path tests to verify they pass**

Run:

```bash
go test ./internal/paths -run Test -v
```

Expected: PASS.

- [ ] **Step 6: Commit paths task**

Run:

```bash
git add helper/go.mod helper/internal/paths/paths.go helper/internal/paths/paths_test.go
git commit -m "feat: add proxyctl runtime paths"
```

Do not include `Co-Authored-By`.

### Task 2: Implement Redaction Package

**Files:**
- Create: `helper/internal/redact/redact.go`
- Create: `helper/internal/redact/redact_test.go`

- [ ] **Step 1: Write failing redaction tests**

Create `helper/internal/redact/redact_test.go`:

```go
package redact

import (
	"strings"
	"testing"
)

func TestURLRedactsSensitiveQueryValues(t *testing.T) {
	input := "https://example.com/api/v1/client/subscribe?token=abc123&key=def456&name=cat"
	got := URL(input)

	if strings.Contains(got, "abc123") || strings.Contains(got, "def456") {
		t.Fatalf("redacted URL leaked sensitive query values: %s", got)
	}
	if !strings.Contains(got, "token=%3Credacted%3E") || !strings.Contains(got, "key=%3Credacted%3E") {
		t.Fatalf("redacted URL missing redacted query markers: %s", got)
	}
	if !strings.Contains(got, "name=cat") {
		t.Fatalf("redacted URL removed non-sensitive query value: %s", got)
	}
}

func TestURLRedactsUserinfo(t *testing.T) {
	input := "https://user:secret@example.com/path?name=cat"
	got := URL(input)

	if strings.Contains(got, "user") || strings.Contains(got, "secret") {
		t.Fatalf("redacted URL leaked userinfo: %s", got)
	}
	if !strings.Contains(got, "https://%3Credacted%3E@example.com/path") {
		t.Fatalf("redacted URL missing redacted userinfo: %s", got)
	}
}

func TestStringRedactsRawNodeURIs(t *testing.T) {
	cases := []string{
		"ss://YWVzLTEyOC1nY206cGFzc3dvcmQ@example.com:443#node",
		"trojan://password@example.com:443#node",
		"vmess://eyJ2IjoiMiIsInBzIjoibm9kZSIsImlkIjoiMTIzZS00NTYifQ==",
		"vless://123e4567-e89b-12d3-a456-426614174000@example.com:443#node",
	}

	for _, input := range cases {
		got := String(input)
		if got != "<redacted-node-uri>" {
			t.Fatalf("String(%q) = %q, want <redacted-node-uri>", input, got)
		}
	}
}

func TestStringRedactsSecretQueryAndUserinfoInsideText(t *testing.T) {
	input := "download https://user:secret@example.com/sub?token=abc123&name=cat now"
	got := String(input)

	if strings.Contains(got, "user") || strings.Contains(got, "secret") || strings.Contains(got, "abc123") {
		t.Fatalf("redacted text leaked secret: %s", got)
	}
	if !strings.Contains(got, "name=cat") {
		t.Fatalf("redacted text removed non-sensitive query: %s", got)
	}
}

func TestStringKeepsNonSecretText(t *testing.T) {
	input := "system proxy is off"
	if got := String(input); got != input {
		t.Fatalf("String(%q) = %q", input, got)
	}
}
```

- [ ] **Step 2: Run redaction tests to verify they fail**

Run:

```bash
go test ./internal/redact -run Test -v
```

Expected: FAIL because the package does not exist yet.

- [ ] **Step 3: Implement redaction package**

Create `helper/internal/redact/redact.go`:

```go
package redact

import (
	"net/url"
	"regexp"
	"strings"
)

const (
	redactedValue   = "<redacted>"
	redactedNodeURI = "<redacted-node-uri>"
)

var sensitiveQueryKeys = map[string]struct{}{
	"token":    {},
	"key":      {},
	"password": {},
	"pass":     {},
	"uuid":     {},
	"secret":   {},
}

var rawNodeURIPattern = regexp.MustCompile(`(?i)\b(ss|ssr|trojan|vmess|vless|hysteria|hysteria2|hy2)://\S+`)
var httpURLPattern = regexp.MustCompile(`https?://\S+`)

func URL(input string) string {
	parsed, err := url.Parse(input)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return String(input)
	}
	return redactParsedURL(parsed)
}

func String(input string) string {
	if isRawNodeURI(input) {
		return redactedNodeURI
	}

	redacted := rawNodeURIPattern.ReplaceAllString(input, redactedNodeURI)
	return httpURLPattern.ReplaceAllStringFunc(redacted, func(match string) string {
		parsed, err := url.Parse(match)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return match
		}
		return redactParsedURL(parsed)
	})
}

func redactParsedURL(parsed *url.URL) string {
	copyValue := *parsed
	if copyValue.User != nil {
		copyValue.User = url.User(redactedValue)
	}

	query := copyValue.Query()
	for key := range query {
		if _, ok := sensitiveQueryKeys[strings.ToLower(key)]; ok {
			query.Set(key, redactedValue)
		}
	}
	copyValue.RawQuery = query.Encode()
	return copyValue.String()
}

func isRawNodeURI(input string) bool {
	trimmed := strings.TrimSpace(input)
	return rawNodeURIPattern.MatchString(trimmed) && rawNodeURIPattern.FindString(trimmed) == trimmed
}
```

- [ ] **Step 4: Run redaction tests to verify they pass**

Run:

```bash
go test ./internal/redact -run Test -v
```

Expected: PASS.

- [ ] **Step 5: Run all tests so far**

Run:

```bash
go test ./...
```

from `helper/`.

Expected: PASS.

- [ ] **Step 6: Commit redaction task**

Run:

```bash
git add helper/internal/redact/redact.go helper/internal/redact/redact_test.go
git commit -m "feat: add proxy secret redaction"
```

Do not include `Co-Authored-By`.

### Task 3: Implement Diagnose Skeleton

**Files:**
- Create: `helper/internal/diagnose/diagnose.go`
- Create: `helper/internal/diagnose/diagnose_test.go`

- [ ] **Step 1: Write failing diagnose tests**

Create `helper/internal/diagnose/diagnose_test.go`:

```go
package diagnose

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
)

func TestRunReportsStaticLocalChecks(t *testing.T) {
	temp := t.TempDir()
	p := paths.ForHome(temp)
	if err := os.MkdirAll(p.Config, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(p.SubscriptionsJSON, []byte(`[]`), 0o644); err != nil {
		t.Fatalf("write subscriptions: %v", err)
	}

	report := Run(p)
	if report.App != "ProxyCat" {
		t.Fatalf("App = %q", report.App)
	}
	if report.Milestone != "milestone-1" {
		t.Fatalf("Milestone = %q", report.Milestone)
	}
	if len(report.Checks) != 5 {
		t.Fatalf("len(Checks) = %d, want 5", len(report.Checks))
	}

	checks := checksByName(report.Checks)
	if checks["runtime-paths"].Status != StatusPass {
		t.Fatalf("runtime-paths status = %q", checks["runtime-paths"].Status)
	}
	if checks["subscription-storage"].Status != StatusPass {
		t.Fatalf("subscription-storage status = %q", checks["subscription-storage"].Status)
	}
	if checks["generated-config"].Status != StatusWarn {
		t.Fatalf("generated-config status = %q", checks["generated-config"].Status)
	}
	if checks["mihomo-binary"].Status != StatusWarn {
		t.Fatalf("mihomo-binary status = %q", checks["mihomo-binary"].Status)
	}
	if checks["network-checks"].Status != StatusWarn {
		t.Fatalf("network-checks status = %q", checks["network-checks"].Status)
	}
}

func TestRunReportJSONShape(t *testing.T) {
	report := Run(paths.ForHome(t.TempDir()))
	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}

	var decoded struct {
		App       string `json:"app"`
		Milestone string `json:"milestone"`
		Checks    []struct {
			Name         string `json:"name"`
			Status       string `json:"status"`
			Message      string `json:"message"`
			SuggestedFix string `json:"suggestedFix,omitempty"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if decoded.App != "ProxyCat" || decoded.Milestone != "milestone-1" || len(decoded.Checks) == 0 {
		t.Fatalf("unexpected decoded report: %+v", decoded)
	}
}

func checksByName(checks []Check) map[string]Check {
	byName := make(map[string]Check, len(checks))
	for _, check := range checks {
		byName[check.Name] = check
	}
	return byName
}
```

- [ ] **Step 2: Run diagnose tests to verify they fail**

Run:

```bash
go test ./internal/diagnose -run Test -v
```

Expected: FAIL because the package does not exist yet.

- [ ] **Step 3: Implement diagnose skeleton**

Create `helper/internal/diagnose/diagnose.go`:

```go
package diagnose

import (
	"os"

	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
)

type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

type Report struct {
	App       string  `json:"app"`
	Milestone string  `json:"milestone"`
	Checks    []Check `json:"checks"`
}

type Check struct {
	Name         string `json:"name"`
	Status       Status `json:"status"`
	Message      string `json:"message"`
	SuggestedFix string `json:"suggestedFix,omitempty"`
}

func Run(p paths.RuntimePaths) Report {
	return Report{
		App:       "ProxyCat",
		Milestone: "milestone-1",
		Checks: []Check{
			checkRuntimePaths(p),
			checkSubscriptionStorage(p),
			checkGeneratedConfig(p),
			checkMihomoBinary(p),
			{
				Name:         "network-checks",
				Status:       StatusWarn,
				Message:      "Network, controller, system proxy, and outbound connectivity checks are not implemented in milestone 1.",
				SuggestedFix: "Continue with Milestone 2 and Milestone 3.",
			},
		},
	}
}

func checkRuntimePaths(p paths.RuntimePaths) Check {
	if p.Base == "" || p.Config == "" || p.ConfigYAML == "" || p.SubscriptionsJSON == "" || p.Mihomo == "" || p.DiagnoseLatest == "" {
		return Check{Name: "runtime-paths", Status: StatusFail, Message: "Runtime paths are incomplete.", SuggestedFix: "Check ProxyCat path configuration."}
	}
	return Check{Name: "runtime-paths", Status: StatusPass, Message: "Runtime paths resolved."}
}

func checkSubscriptionStorage(p paths.RuntimePaths) Check {
	if fileExists(p.SubscriptionsJSON) {
		return Check{Name: "subscription-storage", Status: StatusPass, Message: "Subscription storage exists."}
	}
	return Check{Name: "subscription-storage", Status: StatusWarn, Message: "No subscription storage found.", SuggestedFix: "Run subscription add in Milestone 2."}
}

func checkGeneratedConfig(p paths.RuntimePaths) Check {
	if fileExists(p.ConfigYAML) {
		return Check{Name: "generated-config", Status: StatusPass, Message: "Generated config exists."}
	}
	return Check{Name: "generated-config", Status: StatusWarn, Message: "No generated config found.", SuggestedFix: "Run config generate after adding a subscription."}
}

func checkMihomoBinary(p paths.RuntimePaths) Check {
	if fileExists(p.Mihomo) {
		return Check{Name: "mihomo-binary", Status: StatusPass, Message: "Mihomo binary exists."}
	}
	return Check{Name: "mihomo-binary", Status: StatusWarn, Message: "Mihomo binary not found.", SuggestedFix: "Choose a Mihomo binary distribution mode before core management."}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
```

- [ ] **Step 4: Run diagnose tests to verify they pass**

Run:

```bash
go test ./internal/diagnose -run Test -v
```

Expected: PASS.

- [ ] **Step 5: Run all tests so far**

Run:

```bash
go test ./...
```

from `helper/`.

Expected: PASS.

- [ ] **Step 6: Commit diagnose task**

Run:

```bash
git add helper/internal/diagnose/diagnose.go helper/internal/diagnose/diagnose_test.go
git commit -m "feat: add diagnose skeleton"
```

Do not include `Co-Authored-By`.

### Task 4: Implement `proxyctl diagnose` CLI

**Files:**
- Create: `helper/cmd/proxyctl/main.go`
- Create: `helper/cmd/proxyctl/main_test.go`

- [ ] **Step 1: Write failing CLI tests**

Create `helper/cmd/proxyctl/main_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnoseJSONCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"diagnose", "--json"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var decoded struct {
		App       string `json:"app"`
		Milestone string `json:"milestone"`
		Checks    []struct {
			Name         string `json:"name"`
			Status       string `json:"status"`
			Message      string `json:"message"`
			SuggestedFix string `json:"suggestedFix,omitempty"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if decoded.App != "ProxyCat" || decoded.Milestone != "milestone-1" || len(decoded.Checks) != 5 {
		t.Fatalf("unexpected diagnose output: %+v", decoded)
	}
}

func TestDiagnoseHumanCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"diagnose"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "ProxyCat Diagnose") {
		t.Fatalf("human output missing heading: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "runtime-paths") {
		t.Fatalf("human output missing check name: %s", stdout.String())
	}
}

func TestUnknownCommandRedactsSecrets(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"https://user:secret@example.com/sub?token=abc123"}, stdout, stderr)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	got := stderr.String()
	if strings.Contains(got, "user") || strings.Contains(got, "secret") || strings.Contains(got, "abc123") {
		t.Fatalf("stderr leaked secret: %s", got)
	}
	if !strings.Contains(got, "unknown command") {
		t.Fatalf("stderr missing unknown command message: %s", got)
	}
}
```

- [ ] **Step 2: Run CLI tests to verify they fail**

Run:

```bash
go test ./cmd/proxyctl -run Test -v
```

Expected: FAIL because the command entrypoint does not exist yet.

- [ ] **Step 3: Implement CLI entrypoint**

Create `helper/cmd/proxyctl/main.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/Cass-ette/ProxyCat/helper/internal/diagnose"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
	"github.com/Cass-ette/ProxyCat/helper/internal/redact"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printHelp(stdout)
		return 0
	}

	switch args[0] {
	case "diagnose":
		return runDiagnose(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", redact.String(args[0]))
		printHelp(stderr)
		return 2
	}
}

func runDiagnose(args []string, stdout io.Writer, stderr io.Writer) int {
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			fmt.Fprintf(stderr, "unknown diagnose flag: %s\n", redact.String(arg))
			return 2
		}
	}

	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve runtime paths: %v\n", err)
		return 1
	}
	report := diagnose.Run(runtimePaths)
	if jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "encode diagnose report: %v\n", err)
			return 1
		}
		return 0
	}

	fmt.Fprintln(stdout, "ProxyCat Diagnose")
	for _, check := range report.Checks {
		fmt.Fprintf(stdout, "- %s: %s - %s\n", check.Name, check.Status, check.Message)
		if check.SuggestedFix != "" {
			fmt.Fprintf(stdout, "  Suggested fix: %s\n", check.SuggestedFix)
		}
	}
	return 0
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "ProxyCat proxyctl")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  proxyctl diagnose [--json]")
}
```

- [ ] **Step 4: Run CLI tests to verify they pass**

Run:

```bash
go test ./cmd/proxyctl -run Test -v
```

Expected: PASS.

- [ ] **Step 5: Run full helper test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 6: Build `proxyctl`**

Run:

```bash
go build ./cmd/proxyctl
```

Expected: PASS and produce a local `proxyctl` binary in `helper/`. Remove the generated binary before committing:

```bash
rm -f proxyctl
```

- [ ] **Step 7: Manually verify JSON command**

Run:

```bash
go run ./cmd/proxyctl diagnose --json
```

Expected: JSON object with `app`, `milestone`, and five `checks`. It may contain `warn` statuses because Milestone 1 does not implement subscription import, Mihomo binary setup, network checks, or generated config yet.

- [ ] **Step 8: Commit CLI task**

Run:

```bash
git add helper/cmd/proxyctl/main.go helper/cmd/proxyctl/main_test.go
git commit -m "feat: add proxyctl diagnose command"
```

Do not include `Co-Authored-By`.

### Task 5: Final Milestone 1 Verification

**Files:**
- Modify: none unless verification exposes a defect.

- [ ] **Step 1: Verify branch diff scope**

Run from the worktree root:

```bash
git status --short --branch --untracked-files=normal
git diff --stat origin/main...HEAD
```

Expected: only Milestone 1 files and this plan document are changed relative to `origin/main`.

- [ ] **Step 2: Run full tests**

Run from `helper/`:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Build helper**

Run from `helper/`:

```bash
go build ./cmd/proxyctl
rm -f proxyctl
```

Expected: build exits 0, generated binary removed before commit or final status.

- [ ] **Step 4: Verify CLI JSON output**

Run from `helper/`:

```bash
go run ./cmd/proxyctl diagnose --json
```

Expected: valid JSON, redacted-safe by construction, with five checks.

- [ ] **Step 5: Verify no generated binary remains**

Run from the worktree root:

```bash
git status --short --untracked-files=normal
```

Expected: no untracked `helper/proxyctl` binary. The plan file may be uncommitted unless committed in Step 6.

- [ ] **Step 6: Commit implementation plan if not already committed**

Run from the worktree root:

```bash
git add docs/superpowers/plans/2026-05-02-proxycat-milestone-1.md
git commit -m "docs: add ProxyCat milestone 1 plan"
```

Do not include `Co-Authored-By`.

- [ ] **Step 7: Prepare for review**

Run:

```bash
git status --short --branch --untracked-files=normal
git log --oneline --decorate origin/main..HEAD
```

Expected: clean working tree and commits for the plan plus each implementation task.
