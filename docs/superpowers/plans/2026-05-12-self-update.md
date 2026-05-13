# Self-Update Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a safe self-update path so a non-technical user can update ProxyCat from the app menu or by running `proxycat update`.

**Architecture:** Implement update logic in a focused Go package under `helper/internal/selfupdate`, expose it through `proxyctl self-update`, then add a Swift menu action that streams NDJSON progress from the helper. Reuse the existing unsigned installer packaging model and add SHA256 sidecars plus a `/usr/local/bin/proxycat` wrapper for CLI convenience.

**Tech Stack:** Go 1.24 helper, macOS shell commands (`osascript`, `killall`, `xattr`, `open`, `ditto`/archive extraction), SwiftUI menu app, GitHub Releases API, existing bash packaging scripts.

---

## File Structure

### Go helper

- Create: `helper/internal/selfupdate/semver.go`
  - Strict `major.minor.patch` parser and comparator.
- Create: `helper/internal/selfupdate/release.go`
  - GitHub latest release fetching, asset filtering, cached check timestamp support.
- Create: `helper/internal/selfupdate/download.go`
  - Asset download, SHA256 parsing/verifying, temp directory lifecycle.
- Create: `helper/internal/selfupdate/install.go`
  - Bundle validation, app backup, app quit/kill fallback, replace, rollback, quarantine clearing, relaunch.
- Create: `helper/internal/selfupdate/update.go`
  - High-level orchestration, progress event model, JSON/human output writer.
- Create: `helper/internal/selfupdate/*_test.go`
  - Unit tests for each isolated behavior, using temp dirs and fake HTTP servers.
- Modify: `helper/cmd/proxyctl/main.go`
  - Add `self-update` command and `update` alias for `proxycat update` compatibility if invoked via wrapper.
- Modify: `helper/cmd/proxyctl/main_test.go`
  - CLI command recognition and JSON output tests.

### Packaging scripts

- Modify: `scripts/package-unsigned.sh`
  - Generate SHA256 sidecar for installer zip.
  - Add `/usr/local/bin/proxycat` wrapper creation to `安装 ProxyCat.command`.
- Modify/Test: `scripts/package-unsigned_test.sh`
  - Validate script emits/install instructions include wrapper and SHA256 path if current test structure supports it.

### Swift app

- Modify: `app/ProxyCat/ProxyCat/Models.swift`
  - Add `UpdateProgress` model for NDJSON events.
- Modify: `app/ProxyCat/ProxyCat/HelperClient.swift`
  - Add `selfUpdate() -> AsyncStream<UpdateProgress>` that runs `proxyctl self-update --json` and decodes line-delimited JSON.
- Modify: `app/ProxyCat/ProxyCat/StatusViewModel.swift`
  - Add `isUpdating`, `updateStatus`, and `checkForUpdate()`.
- Modify: `app/ProxyCat/ProxyCat/MenuContentView.swift`
  - Add "检查更新" button in actions section and show status in feedback/inline UI.

### Build artifacts

- Modify after successful build: `app/ProxyCat/ProxyCat/Resources/proxyctl`
  - Rebuild bundled universal helper so the app contains `self-update`.

---

## Chunk 1: Go selfupdate package foundations

### Task 1: Strict version parsing and comparison

**Files:**
- Create: `helper/internal/selfupdate/semver.go`
- Create: `helper/internal/selfupdate/semver_test.go`

- [ ] **Step 1: Write failing tests for strict semver parsing**

Create `helper/internal/selfupdate/semver_test.go`:

```go
package selfupdate

import "testing"

func TestParseVersionAcceptsStrictMajorMinorPatch(t *testing.T) {
	v, err := parseVersion("0.12.3")
	if err != nil {
		t.Fatalf("parseVersion returned error: %v", err)
	}
	if v.major != 0 || v.minor != 12 || v.patch != 3 {
		t.Fatalf("version = %+v", v)
	}
}

func TestParseVersionRejectsNonStrictVersions(t *testing.T) {
	for _, input := range []string{"v0.1.0", "0.1", "0.1.0-beta.1", "latest", ""} {
		if _, err := parseVersion(input); err == nil {
			t.Fatalf("parseVersion(%q) returned nil error", input)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		current string
		latest  string
		want    int
	}{
		{"0.1.0", "0.1.1", -1},
		{"0.2.0", "0.1.99", 1},
		{"1.0.0", "1.0.0", 0},
	}
	for _, tc := range cases {
		current, _ := parseVersion(tc.current)
		latest, _ := parseVersion(tc.latest)
		if got := current.compare(latest); got != tc.want {
			t.Fatalf("%s compare %s = %d, want %d", tc.current, tc.latest, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/selfupdate -run TestParseVersion -v
```

Expected: FAIL because package/file does not exist or functions are undefined.

- [ ] **Step 3: Implement minimal semver parser**

Create `helper/internal/selfupdate/semver.go`:

```go
package selfupdate

import (
	"fmt"
	"regexp"
	"strconv"
)

var strictVersionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

type version struct {
	major int
	minor int
	patch int
}

func parseVersion(raw string) (version, error) {
	match := strictVersionPattern.FindStringSubmatch(raw)
	if match == nil {
		return version{}, fmt.Errorf("invalid version: %s", raw)
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	return version{major: major, minor: minor, patch: patch}, nil
}

func (v version) compare(other version) int {
	if v.major != other.major {
		return compareInt(v.major, other.major)
	}
	if v.minor != other.minor {
		return compareInt(v.minor, other.minor)
	}
	return compareInt(v.patch, other.patch)
}

func compareInt(a int, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
```

- [ ] **Step 4: Run tests to verify GREEN**

Run:

```bash
go test ./internal/selfupdate -run 'TestParseVersion|TestVersionCompare' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add helper/internal/selfupdate/semver.go helper/internal/selfupdate/semver_test.go
git commit -m "test: add self-update version parsing"
```

### Task 2: Release metadata fetching and asset filtering

**Files:**
- Create: `helper/internal/selfupdate/release.go`
- Create: `helper/internal/selfupdate/release_test.go`

- [ ] **Step 1: Write failing tests for latest release parsing and strict asset filter**

Create `helper/internal/selfupdate/release_test.go`:

```go
package selfupdate

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchLatestReleaseFindsStrictInstallerAndSHA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name": "v0.2.0",
			"assets": [
				{"name":"ProxyCat-dev-installer.zip","browser_download_url":"https://example.com/dev.zip","size":1},
				{"name":"ProxyCat-0.2.0-installer.zip","browser_download_url":"https://example.com/app.zip","size":100},
				{"name":"ProxyCat-0.2.0-installer.zip.sha256","browser_download_url":"https://example.com/app.zip.sha256","size":64}
			]
		}`))
	}))
	defer server.Close()

	client := server.Client()
	release, err := fetchLatestRelease(client, server.URL)
	if err != nil {
		t.Fatalf("fetchLatestRelease returned error: %v", err)
	}
	if release.Version != "0.2.0" {
		t.Fatalf("version = %q", release.Version)
	}
	if release.InstallerURL != "https://example.com/app.zip" {
		t.Fatalf("installer URL = %q", release.InstallerURL)
	}
	if release.SHA256URL != "https://example.com/app.zip.sha256" {
		t.Fatalf("sha URL = %q", release.SHA256URL)
	}
}

func TestFetchLatestReleaseRejectsMissingSHA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","assets":[{"name":"ProxyCat-0.2.0-installer.zip","browser_download_url":"https://example.com/app.zip","size":100}]}`))
	}))
	defer server.Close()

	_, err := fetchLatestRelease(server.Client(), server.URL)
	if err == nil {
		t.Fatal("expected missing sha error")
	}
}
```

- [ ] **Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/selfupdate -run TestFetchLatestRelease -v
```

Expected: FAIL because `fetchLatestRelease` is undefined.

- [ ] **Step 3: Implement minimal release fetching**

Create `helper/internal/selfupdate/release.go` with:

```go
package selfupdate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var installerAssetPattern = regexp.MustCompile(`^ProxyCat-(\d+\.\d+\.\d+)-installer\.zip$`)

type Release struct {
	Version      string
	InstallerURL string
	SHA256URL    string
	Size         int64
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

func fetchLatestRelease(client *http.Client, endpoint string) (Release, error) {
	resp, err := client.Get(endpoint)
	if err != nil {
		return Release{}, fmt.Errorf("下载失败，请检查网络后重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return Release{}, fmt.Errorf("更新检查暂时不可用，请稍后重试")
	}
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("下载失败，请检查网络后重试")
	}

	var decoded githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return Release{}, fmt.Errorf("更新失败：安装包格式不正确")
	}

	versionText := strings.TrimPrefix(decoded.TagName, "v")
	if _, err := parseVersion(versionText); err != nil {
		return Release{}, err
	}

	var release Release
	release.Version = versionText
	for _, asset := range decoded.Assets {
		match := installerAssetPattern.FindStringSubmatch(asset.Name)
		if match != nil && match[1] == versionText {
			release.InstallerURL = asset.URL
			release.Size = asset.Size
		}
		if asset.Name == "ProxyCat-"+versionText+"-installer.zip.sha256" {
			release.SHA256URL = asset.URL
		}
	}
	if release.InstallerURL == "" || release.SHA256URL == "" {
		return Release{}, fmt.Errorf("更新失败：安装包格式不正确")
	}
	return release, nil
}
```

- [ ] **Step 4: Run tests to verify GREEN**

Run:

```bash
go test ./internal/selfupdate -run TestFetchLatestRelease -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add helper/internal/selfupdate/release.go helper/internal/selfupdate/release_test.go
git commit -m "feat: fetch self-update release metadata"
```

---

## Chunk 2: Download, verify, and installer extraction

### Task 3: SHA256 parsing and download verification

**Files:**
- Create: `helper/internal/selfupdate/download.go`
- Create: `helper/internal/selfupdate/download_test.go`

- [ ] **Step 1: Write failing tests for SHA256 sidecar parsing**

Create `helper/internal/selfupdate/download_test.go`:

```go
package selfupdate

import "testing"

func TestParseSHA256Sidecar(t *testing.T) {
	got, err := parseSHA256Sidecar([]byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  ProxyCat-0.2.0-installer.zip\n"))
	if err != nil {
		t.Fatalf("parseSHA256Sidecar returned error: %v", err)
	}
	if got != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("hash = %q", got)
	}
}

func TestParseSHA256SidecarRejectsInvalidHash(t *testing.T) {
	if _, err := parseSHA256Sidecar([]byte("not-a-hash file.zip")); err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/selfupdate -run TestParseSHA256Sidecar -v
```

Expected: FAIL because `parseSHA256Sidecar` is undefined.

- [ ] **Step 3: Implement SHA256 parsing and verification helpers**

Create `helper/internal/selfupdate/download.go`:

```go
package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

var sha256Pattern = regexp.MustCompile(`(?i)\b([a-f0-9]{64})\b`)

func parseSHA256Sidecar(content []byte) (string, error) {
	match := sha256Pattern.FindStringSubmatch(string(content))
	if match == nil {
		return "", fmt.Errorf("更新包校验失败，请截图发给我")
	}
	return strings.ToLower(match[1]), nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func verifyFileSHA256(path string, expected string) error {
	actual, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if actual != strings.ToLower(expected) {
		return fmt.Errorf("更新包校验失败，请截图发给我")
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify GREEN**

Run:

```bash
go test ./internal/selfupdate -run TestParseSHA256Sidecar -v
```

Expected: PASS.

- [ ] **Step 5: Add download test with fake server**

Append to `download_test.go`:

```go
func TestDownloadFileWritesContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "file.zip")
	if err := downloadFile(server.Client(), server.URL, dest, nil); err != nil {
		t.Fatalf("downloadFile returned error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("content = %q", got)
	}
}
```

Add imports in `download_test.go`:

```go
import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)
```

- [ ] **Step 6: Run test to verify RED**

Run:

```bash
go test ./internal/selfupdate -run TestDownloadFileWritesContent -v
```

Expected: FAIL because `downloadFile` is undefined.

- [ ] **Step 7: Implement minimal downloadFile**

Append to `download.go`:

```go
type progressFunc func(percent int)

func downloadFile(client *http.Client, url string, dest string, progress progressFunc) error {
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("下载失败，请检查网络后重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败，请检查网络后重试")
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}
	if progress != nil {
		progress(100)
	}
	return nil
}
```

Add `net/http` to imports.

- [ ] **Step 8: Run tests to verify GREEN**

Run:

```bash
go test ./internal/selfupdate -run 'TestParseSHA256Sidecar|TestDownloadFileWritesContent' -v
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add helper/internal/selfupdate/download.go helper/internal/selfupdate/download_test.go
git commit -m "feat: verify self-update downloads"
```

### Task 4: Installer zip extraction and bundle validation

**Files:**
- Create: `helper/internal/selfupdate/install.go`
- Create: `helper/internal/selfupdate/install_test.go`

- [ ] **Step 1: Write failing bundle validation tests**

Create `helper/internal/selfupdate/install_test.go`:

```go
package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateBundleAcceptsProxyCatApp(t *testing.T) {
	app := fakeAppBundle(t, "com.cassette.proxycat")
	if err := validateBundle(app); err != nil {
		t.Fatalf("validateBundle returned error: %v", err)
	}
}

func TestValidateBundleRejectsWrongBundleID(t *testing.T) {
	app := fakeAppBundle(t, "com.example.other")
	if err := validateBundle(app); err == nil {
		t.Fatal("expected error")
	}
}

func fakeAppBundle(t *testing.T, bundleID string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "ProxyCat.app")
	contents := filepath.Join(root, "Contents")
	resources := filepath.Join(contents, "Resources")
	if err := os.MkdirAll(resources, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>` + bundleID + `</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	proxyctl := filepath.Join(resources, "proxyctl")
	if err := os.WriteFile(proxyctl, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}
```

- [ ] **Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/selfupdate -run TestValidateBundle -v
```

Expected: FAIL because `validateBundle` is undefined.

- [ ] **Step 3: Implement minimal bundle validation**

Create `helper/internal/selfupdate/install.go`:

```go
package selfupdate

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const proxyCatBundleID = "com.cassette.proxycat"

func validateBundle(appPath string) error {
	infoPlist := filepath.Join(appPath, "Contents", "Info.plist")
	out, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIdentifier", infoPlist).Output()
	if err != nil {
		return fmt.Errorf("更新失败：安装包格式不正确")
	}
	if strings.TrimSpace(string(out)) != proxyCatBundleID {
		return fmt.Errorf("更新失败：安装包格式不正确")
	}
	proxyctl := filepath.Join(appPath, "Contents", "Resources", "proxyctl")
	info, err := os.Stat(proxyctl)
	if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return fmt.Errorf("更新失败：安装包格式不正确")
	}
	return nil
}

func commandOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify GREEN**

Run:

```bash
go test ./internal/selfupdate -run TestValidateBundle -v
```

Expected: PASS.

- [ ] **Step 5: Add replace/rollback tests using temp directories**

Append to `install_test.go`:

```go
func TestReplaceAppBacksUpOldApp(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "Applications", "ProxyCat.app")
	backupDir := filepath.Join(root, "backups")
	newApp := fakeAppBundle(t, "com.cassette.proxycat")
	if err := os.MkdirAll(filepath.Dir(current), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(current, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(current, "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := replaceApp(current, newApp, backupDir, "0.1.0"); err != nil {
		t.Fatalf("replaceApp returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(backupDir, "ProxyCat-0.1.0.app", "old.txt")); err != nil {
		t.Fatalf("backup missing old file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(current, "Contents", "Info.plist")); err != nil {
		t.Fatalf("new app missing: %v", err)
	}
}
```

- [ ] **Step 6: Run test to verify RED**

Run:

```bash
go test ./internal/selfupdate -run TestReplaceAppBacksUpOldApp -v
```

Expected: FAIL because `replaceApp` is undefined.

- [ ] **Step 7: Implement replaceApp with backup requirement**

Append to `install.go`:

```go
func replaceApp(currentApp string, newApp string, backupDir string, oldVersion string) error {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	backupPath := filepath.Join(backupDir, "ProxyCat-"+oldVersion+".app")
	_ = os.RemoveAll(backupPath)
	if err := os.Rename(currentApp, backupPath); err != nil {
		return err
	}
	if err := os.Rename(newApp, currentApp); err != nil {
		_ = os.RemoveAll(currentApp)
		_ = os.Rename(backupPath, currentApp)
		return fmt.Errorf("更新失败，已恢复旧版本。请截图发给我。")
	}
	return nil
}
```

- [ ] **Step 8: Run tests to verify GREEN**

Run:

```bash
go test ./internal/selfupdate -run 'TestValidateBundle|TestReplaceAppBacksUpOldApp' -v
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add helper/internal/selfupdate/install.go helper/internal/selfupdate/install_test.go
git commit -m "feat: validate and replace self-update app bundles"
```

---

## Chunk 3: Orchestration and CLI command

### Task 5: Progress events and no-update behavior

**Files:**
- Create: `helper/internal/selfupdate/update.go`
- Create: `helper/internal/selfupdate/update_test.go`

- [ ] **Step 1: Write failing test for no-update flow**

Create `helper/internal/selfupdate/update_test.go`:

```go
package selfupdate

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunReportsAlreadyLatest(t *testing.T) {
	var out bytes.Buffer
	runner := Runner{
		CurrentVersion: "0.2.0",
		Latest: Release{Version: "0.2.0"},
	}
	code := runner.Run(&out, false)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out.String(), "已经是最新版") {
		t.Fatalf("output = %q", out.String())
	}
}
```

- [ ] **Step 2: Run test to verify RED**

Run:

```bash
go test ./internal/selfupdate -run TestRunReportsAlreadyLatest -v
```

Expected: FAIL because `Runner` is undefined.

- [ ] **Step 3: Implement minimal Runner no-update logic**

Create `helper/internal/selfupdate/update.go`:

```go
package selfupdate

import (
	"encoding/json"
	"fmt"
	"io"
)

type Runner struct {
	CurrentVersion string
	Latest         Release
}

type Event struct {
	Stage      string `json:"stage"`
	Message    string `json:"message"`
	Progress   int    `json:"progress,omitempty"`
	NewVersion string `json:"newVersion,omitempty"`
}

func (r Runner) Run(stdout io.Writer, jsonOutput bool) int {
	current, err := parseVersion(r.CurrentVersion)
	if err != nil {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: "更新失败：安装包格式不正确"})
		return 1
	}
	latest, err := parseVersion(r.Latest.Version)
	if err != nil {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: "更新失败：安装包格式不正确"})
		return 1
	}
	if current.compare(latest) >= 0 {
		emit(stdout, jsonOutput, Event{Stage: "done", Message: "已经是最新版"})
		return 0
	}
	return 1
}

func emit(w io.Writer, jsonOutput bool, event Event) {
	if jsonOutput {
		_ = json.NewEncoder(w).Encode(event)
		return
	}
	fmt.Fprintln(w, event.Message)
}
```

- [ ] **Step 4: Run test to verify GREEN**

Run:

```bash
go test ./internal/selfupdate -run TestRunReportsAlreadyLatest -v
```

Expected: PASS.

- [ ] **Step 5: Add JSON output test**

Append to `update_test.go`:

```go
func TestRunReportsAlreadyLatestJSON(t *testing.T) {
	var out bytes.Buffer
	runner := Runner{CurrentVersion: "0.2.0", Latest: Release{Version: "0.2.0"}}
	code := runner.Run(&out, true)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out.String(), `"stage":"done"`) || !strings.Contains(out.String(), "已经是最新版") {
		t.Fatalf("json output = %q", out.String())
	}
}
```

- [ ] **Step 6: Run tests to verify GREEN**

Run:

```bash
go test ./internal/selfupdate -run TestRunReportsAlreadyLatest -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add helper/internal/selfupdate/update.go helper/internal/selfupdate/update_test.go
git commit -m "feat: add self-update progress events"
```

### Task 6: CLI wiring for `proxyctl self-update`

**Files:**
- Modify: `helper/cmd/proxyctl/main.go`
- Modify: `helper/cmd/proxyctl/main_test.go`

- [ ] **Step 1: Write failing CLI recognition test**

Add to `helper/cmd/proxyctl/main_test.go`:

```go
func TestSelfUpdateCommandIsRecognized(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"self-update", "--check-only"}, stdout, stderr)
	if strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("self-update command should be recognized: %s", stderr.String())
	}
	if exitCode != 0 && exitCode != 1 {
		t.Fatalf("unexpected exitCode = %d, stderr = %s", exitCode, stderr.String())
	}
}
```

- [ ] **Step 2: Run test to verify RED**

Run:

```bash
go test ./cmd/proxyctl -run TestSelfUpdateCommandIsRecognized -v
```

Expected: FAIL with unknown command.

- [ ] **Step 3: Add command dispatch and help text**

Modify `helper/cmd/proxyctl/main.go`:

- Add import: `github.com/Cass-ette/ProxyCat/helper/internal/selfupdate`
- Add top-level case:

```go
case "self-update":
	return runSelfUpdate(stdout, stderr, jsonOutput)
case "update":
	return runSelfUpdate(stdout, stderr, jsonOutput)
```

- Add help line:

```go
fmt.Fprintln(w, "  proxyctl self-update [--json]")
fmt.Fprintln(w, "  proxyctl update [--json]")
```

- Add minimal function:

```go
func runSelfUpdate(stdout io.Writer, stderr io.Writer, jsonOutput bool) int {
	fmt.Fprintln(stderr, "self-update is not configured in this build")
	return 1
}
```

This is temporary; later task replaces it with real runner wiring.

- [ ] **Step 4: Run test to verify GREEN**

Run:

```bash
go test ./cmd/proxyctl -run TestSelfUpdateCommandIsRecognized -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add helper/cmd/proxyctl/main.go helper/cmd/proxyctl/main_test.go
git commit -m "feat: add self-update command entrypoint"
```

### Task 7: Wire real self-update runner behind CLI

**Files:**
- Modify: `helper/internal/selfupdate/update.go`
- Modify: `helper/cmd/proxyctl/main.go`
- Modify: `helper/internal/selfupdate/update_test.go`

- [ ] **Step 1: Extend tests for configurable endpoints and check-only mode**

Add to `update_test.go` a fake release server test that checks a newer version emits checking/downloading stages but uses a `CheckOnly` flag to avoid replacing apps:

```go
func TestRunCheckOnlyReportsAvailableUpdate(t *testing.T) {
	var out bytes.Buffer
	runner := Runner{
		CurrentVersion: "0.1.0",
		Latest: Release{Version: "0.2.0"},
		CheckOnly: true,
	}
	code := runner.Run(&out, true)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out.String(), `"stage":"done"`) || !strings.Contains(out.String(), "发现新版本 0.2.0") {
		t.Fatalf("json output = %q", out.String())
	}
}
```

- [ ] **Step 2: Run test to verify RED**

Run:

```bash
go test ./internal/selfupdate -run TestRunCheckOnlyReportsAvailableUpdate -v
```

Expected: FAIL because `CheckOnly` behavior is missing.

- [ ] **Step 3: Implement CheckOnly branch**

Modify `Runner`:

```go
type Runner struct {
	CurrentVersion string
	Latest         Release
	CheckOnly      bool
}
```

In `Run`, after version comparison:

```go
if r.CheckOnly {
	emit(stdout, jsonOutput, Event{Stage: "done", Message: "发现新版本 " + r.Latest.Version, NewVersion: r.Latest.Version})
	return 0
}
```

- [ ] **Step 4: Run tests to verify GREEN**

Run:

```bash
go test ./internal/selfupdate -run 'TestRunReportsAlreadyLatest|TestRunCheckOnlyReportsAvailableUpdate' -v
```

Expected: PASS.

- [ ] **Step 5: Wire CLI flags**

Modify `helper/cmd/proxyctl/main.go` command dispatch to parse `--check-only` and pass it to `runSelfUpdate`:

```go
case "self-update":
	return runSelfUpdate(stdout, stderr, jsonOutput, containsArg(args[1:], "--check-only"))
case "update":
	return runSelfUpdate(stdout, stderr, jsonOutput, containsArg(args[1:], "--check-only"))
```

Add helper if not already present:

```go
func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}
```

- [ ] **Step 6: Keep real destructive install behind non-test path**

For the first implementation, wire `runSelfUpdate` to only support `--check-only` until end-to-end release assets are available:

```go
func runSelfUpdate(stdout io.Writer, stderr io.Writer, jsonOutput bool, checkOnly bool) int {
	if !checkOnly {
		fmt.Fprintln(stderr, "self-update install is not available until a GitHub Release asset exists")
		return 1
	}
	// TODO in Task 8: construct real Runner from Info.plist + GitHub release.
}
```

- [ ] **Step 7: Run CLI tests**

Run:

```bash
go test ./cmd/proxyctl -run TestSelfUpdateCommandIsRecognized -v
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add helper/internal/selfupdate/update.go helper/internal/selfupdate/update_test.go helper/cmd/proxyctl/main.go helper/cmd/proxyctl/main_test.go
git commit -m "feat: wire self-update check mode"
```

---

## Chunk 4: Full install path and packaging integration

### Task 8: End-to-end self-update install orchestration

**Files:**
- Modify: `helper/internal/selfupdate/update.go`
- Modify: `helper/internal/selfupdate/release.go`
- Modify: `helper/internal/selfupdate/download.go`
- Modify: `helper/internal/selfupdate/install.go`
- Modify: `helper/cmd/proxyctl/main.go`

- [ ] **Step 1: Define Options for real runner**

Add to `update.go`:

```go
type Options struct {
	CurrentAppPath string
	Endpoint       string
	HTTPClient     *http.Client
	CheckOnly      bool
	JSONOutput     bool
}
```

- [ ] **Step 2: Write tests for Options validation**

Add tests ensuring missing app path and missing endpoint fail with Chinese messages.

- [ ] **Step 3: Implement `Run(opts Options, stdout io.Writer) int`**

This should:

1. Read current version from `CurrentAppPath/Contents/Info.plist`
2. Fetch latest release
3. Compare versions
4. If check-only or no update, exit before download
5. Download zip and sha
6. Verify sha
7. Extract zip using `/usr/bin/ditto -x -k`
8. Find extracted `ProxyCat.app`
9. Validate bundle
10. Backup and replace
11. Clear quarantine
12. Relaunch

- [ ] **Step 4: Add tests for failure paths using fake HTTP and temp app dirs**

Tests to add:

- SHA mismatch returns `更新包校验失败`
- Wrong bundle id returns `更新失败：安装包格式不正确`
- Backup failure aborts before removing current app

- [ ] **Step 5: Run package tests**

Run:

```bash
go test ./internal/selfupdate -v
```

Expected: PASS.

- [ ] **Step 6: Wire `proxyctl self-update` to real runner**

In `main.go`, use:

```go
appPath := "/Applications/ProxyCat.app"
endpoint := "https://api.github.com/repos/Cass-ette/ProxyCat/releases/latest"
return selfupdate.Run(selfupdate.Options{
	CurrentAppPath: appPath,
	Endpoint: endpoint,
	HTTPClient: &http.Client{Timeout: 60 * time.Second},
	CheckOnly: checkOnly,
	JSONOutput: jsonOutput,
}, stdout)
```

- [ ] **Step 7: Run focused CLI tests**

```bash
go test ./cmd/proxyctl ./internal/selfupdate -run 'SelfUpdate|Run|Validate|Replace|Download|Fetch|Parse' -v
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add helper/internal/selfupdate helper/cmd/proxyctl/main.go helper/cmd/proxyctl/main_test.go
git commit -m "feat: implement self-update installation"
```

### Task 9: Packaging script creates SHA sidecar and wrapper

**Files:**
- Modify: `scripts/package-unsigned.sh`
- Modify: `scripts/package-unsigned_test.sh` if current test supports script-content assertions

- [ ] **Step 1: Write failing script test or shell assertion**

If `scripts/package-unsigned_test.sh` has assertions, add checks for:

- `ProxyCat-<version>-installer.zip.sha256` creation
- `/usr/local/bin/proxycat` wrapper creation in installer script

If no suitable test harness exists, add a bash dry-run helper test with temp `DIST_DIR` if feasible.

- [ ] **Step 2: Run test to verify RED**

Run:

```bash
bash scripts/package-unsigned_test.sh
```

Expected: FAIL before script changes.

- [ ] **Step 3: Generate SHA256 sidecar in package script**

After installer zip creation in `scripts/package-unsigned.sh`:

```bash
shasum -a 256 "$INSTALLER_ZIP_PATH" > "$INSTALLER_ZIP_PATH.sha256"
```

- [ ] **Step 4: Add wrapper creation to installer script**

Inside generated `安装 ProxyCat.command`, after copying app and clearing xattr:

```bash
WRAPPER_DEST="/usr/local/bin/proxycat"
WRAPPER_CONTENT='#!/bin/bash
exec "/Applications/ProxyCat.app/Contents/Resources/proxyctl" "$@"
'
mkdir -p /usr/local/bin 2>/dev/null || sudo mkdir -p /usr/local/bin
printf "%s" "$WRAPPER_CONTENT" > "$WRAPPER_DEST" 2>/dev/null || echo "$WRAPPER_CONTENT" | sudo tee "$WRAPPER_DEST" >/dev/null
chmod +x "$WRAPPER_DEST" 2>/dev/null || sudo chmod +x "$WRAPPER_DEST"
```

- [ ] **Step 5: Update Chinese installer text**

Add line:

```text
以后可以在终端输入 proxycat update 更新 ProxyCat。
```

- [ ] **Step 6: Run script test**

Run:

```bash
bash scripts/package-unsigned_test.sh
```

Expected: PASS.

- [ ] **Step 7: Run package script once locally**

Run:

```bash
./scripts/package-unsigned.sh
```

Expected:

- `dist/ProxyCat-<version>-installer.zip`
- `dist/ProxyCat-<version>-installer.zip.sha256`

- [ ] **Step 8: Commit**

```bash
git add scripts/package-unsigned.sh scripts/package-unsigned_test.sh
git commit -m "feat: package self-update assets"
```

---

## Chunk 5: Swift UI integration

### Task 10: Add update progress model and helper stream

**Files:**
- Modify: `app/ProxyCat/ProxyCat/Models.swift`
- Modify: `app/ProxyCat/ProxyCat/HelperClient.swift`

- [ ] **Step 1: Add `UpdateProgress` model**

In `Models.swift`:

```swift
struct UpdateProgress: Codable {
    let stage: String
    let message: String
    let progress: Int?
    let newVersion: String?
}
```

- [ ] **Step 2: Add `selfUpdate()` stream to HelperClient**

In `HelperClient.swift`, add:

```swift
func selfUpdate() -> AsyncStream<UpdateProgress> {
    AsyncStream { continuation in
        Task {
            guard let executable = getProxyctlPath() else {
                continuation.yield(UpdateProgress(stage: "error", message: "更新失败：找不到 proxyctl", progress: nil, newVersion: nil))
                continuation.finish()
                return
            }
            let task = Process()
            task.executableURL = URL(fileURLWithPath: executable)
            task.arguments = ["self-update", "--json"]
            let outputPipe = Pipe()
            let errorPipe = Pipe()
            task.standardOutput = outputPipe
            task.standardError = errorPipe
            let decoder = JSONDecoder()
            outputPipe.fileHandleForReading.readabilityHandler = { handle in
                let data = handle.availableData
                guard !data.isEmpty else { return }
                for line in String(data: data, encoding: .utf8)?.split(separator: "\n") ?? [] {
                    if let eventData = String(line).data(using: .utf8),
                       let event = try? decoder.decode(UpdateProgress.self, from: eventData) {
                        continuation.yield(event)
                    }
                }
            }
            errorPipe.fileHandleForReading.readabilityHandler = { handle in
                let data = handle.availableData
                if let message = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines), !message.isEmpty {
                    continuation.yield(UpdateProgress(stage: "error", message: message, progress: nil, newVersion: nil))
                }
            }
            task.terminationHandler = { _ in
                outputPipe.fileHandleForReading.readabilityHandler = nil
                errorPipe.fileHandleForReading.readabilityHandler = nil
                continuation.finish()
            }
            do {
                try task.run()
            } catch {
                continuation.yield(UpdateProgress(stage: "error", message: "更新失败：\(error.localizedDescription)", progress: nil, newVersion: nil))
                continuation.finish()
            }
        }
    }
}
```

- [ ] **Step 3: Build Swift target**

Run:

```bash
xcodebuild -project app/ProxyCat/ProxyCat.xcodeproj -scheme ProxyCat -configuration Release -derivedDataPath app/ProxyCat/build CODE_SIGNING_ALLOWED=NO
```

Expected: BUILD SUCCEEDED.

- [ ] **Step 4: Commit**

```bash
git add app/ProxyCat/ProxyCat/Models.swift app/ProxyCat/ProxyCat/HelperClient.swift
git commit -m "feat: stream self-update progress to app"
```

### Task 11: Add menu button and view model state

**Files:**
- Modify: `app/ProxyCat/ProxyCat/StatusViewModel.swift`
- Modify: `app/ProxyCat/ProxyCat/MenuContentView.swift`

- [ ] **Step 1: Add view model state**

In `StatusViewModel.swift`:

```swift
@Published var isUpdating = false
@Published var updateStatus: String?
```

- [ ] **Step 2: Add update action**

In `StatusViewModel.swift`:

```swift
func checkForUpdate() async {
    isUpdating = true
    updateStatus = "正在检查更新..."
    lastError = nil
    defer { isUpdating = false }

    let stream = await helper.selfUpdate()
    for await event in stream {
        updateStatus = event.message
        if event.stage == "error" {
            lastError = event.message
        }
    }
}
```

- [ ] **Step 3: Show update status in `hasFeedback`**

In `MenuContentView.swift`:

```swift
private var hasFeedback: Bool {
    viewModel.lastError != nil || viewModel.testResult != nil || viewModel.diagnoseReport != nil || viewModel.updateStatus != nil
}
```

- [ ] **Step 4: Render update status**

In `feedbackSection`, before `lastError`:

```swift
if let updateStatus = viewModel.updateStatus {
    Text(updateStatus)
        .font(.system(size: 12))
        .foregroundColor(.secondary)
        .lineLimit(3)
        .padding(.horizontal, 16)
}
```

- [ ] **Step 5: Add action button**

In `actionsSection`, after "更新订阅":

```swift
MenuButton(viewModel.isUpdating ? "更新检查中..." : "检查更新", icon: "arrow.down.circle") {
    Task { await viewModel.checkForUpdate() }
}
.disabled(viewModel.isUpdating)
```

If `MenuButton` does not support `.disabled` cleanly due to return type, wrap it in a conditional or add a `disabled` parameter in the component.

- [ ] **Step 6: Build Swift target**

Run:

```bash
xcodebuild -project app/ProxyCat/ProxyCat.xcodeproj -scheme ProxyCat -configuration Release -derivedDataPath app/ProxyCat/build CODE_SIGNING_ALLOWED=NO
```

Expected: BUILD SUCCEEDED.

- [ ] **Step 7: Commit**

```bash
git add app/ProxyCat/ProxyCat/StatusViewModel.swift app/ProxyCat/ProxyCat/MenuContentView.swift
git commit -m "feat: add update action to menu"
```

---

## Chunk 6: Validation, bundled helper rebuild, and PR readiness

### Task 12: Full validation

**Files:**
- Modify: `app/ProxyCat/ProxyCat/Resources/proxyctl`

- [ ] **Step 1: Run Go formatting**

Run:

```bash
gofmt -w helper/cmd/proxyctl/main.go helper/cmd/proxyctl/main_test.go helper/internal/selfupdate/*.go
```

Expected: no output.

- [ ] **Step 2: Run Go tests**

Run:

```bash
go -C helper test ./... -timeout 10m
```

Expected: all packages PASS.

- [ ] **Step 3: Build app and bundled helper**

Run:

```bash
./scripts/build-app.sh
```

Expected: BUILD SUCCEEDED.

- [ ] **Step 4: Ad-hoc sign app**

Run:

```bash
./scripts/adhoc-sign.sh app/ProxyCat/build/Build/Products/Release/ProxyCat.app
```

Expected: `valid on disk`, `satisfies its Designated Requirement`.

- [ ] **Step 5: Verify bundled proxyctl commands**

Run:

```bash
app/ProxyCat/build/Build/Products/Release/ProxyCat.app/Contents/Resources/proxyctl self-update --check-only
app/ProxyCat/build/Build/Products/Release/ProxyCat.app/Contents/Resources/proxyctl self-update --check-only --json
```

Expected: Chinese no-update or update-available message; JSON emits NDJSON event lines.

- [ ] **Step 6: Run package script**

Run:

```bash
./scripts/package-unsigned.sh
```

Expected: installer zip and `.sha256` sidecar created.

- [ ] **Step 7: Verify source bundled helper changed**

Run:

```bash
app/ProxyCat/ProxyCat/Resources/proxyctl self-update --check-only --json
```

Expected: command recognized and emits JSON event.

- [ ] **Step 8: Commit rebuilt helper**

```bash
git add app/ProxyCat/ProxyCat/Resources/proxyctl
git commit -m "chore: rebuild bundled proxyctl"
```

### Task 13: Code review and PR

**Files:** all changed files.

- [ ] **Step 1: Request code review**

Use `superpowers:requesting-code-review` with context:

- What was implemented: self-update MVP with CLI/app entry points and packaging SHA sidecar
- Requirements: spec at `docs/superpowers/specs/2026-05-12-self-update-design.md`
- Base SHA: branch point from `main`
- Head SHA: current HEAD

- [ ] **Step 2: Fix Critical/Important review findings**

Follow `superpowers:receiving-code-review`.

- [ ] **Step 3: Run final verification**

Run:

```bash
go -C helper test ./... -timeout 10m
./scripts/build-app.sh
./scripts/adhoc-sign.sh app/ProxyCat/build/Build/Products/Release/ProxyCat.app
./scripts/package-unsigned.sh
```

Expected: all pass.

- [ ] **Step 4: Create PR**

Open a PR from the self-update branch to `main` with summary:

- Adds `proxyctl self-update` and `proxycat update` path
- Adds App menu update button
- Adds SHA256 sidecar packaging
- Explains unsigned-app trust limitations

---

## Notes for implementation

- Start this work from `main`, not from `feat/subscription-probe`, after PR #7 is merged or by creating a new branch from latest `main`.
- Follow TDD for each Go behavior: write failing tests first, watch them fail, then implement.
- Keep destructive replacement logic testable by parameterizing paths; never run real `/Applications` mutation in unit tests.
- For manual local install/update testing, use a temp fake app path first; only test `/Applications/ProxyCat.app` after unit tests and code review pass.
- Do not implement Sparkle, notarization, Developer ID signing, or auto-check-on-launch in this MVP.
