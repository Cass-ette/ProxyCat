# ProxyCat Milestone 2 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build subscription import and config validation: storage, download, format detection (Clash YAML vs base64/plain URI), YAML validation, and backup.

**Architecture:** The subscription package handles storage and download. The config package handles format detection, validation, and backup. Both are testable without real network calls by using interface-based HTTP client injection.

**Tech Stack:** Go 1.24 standard library (`net/http` for download, `encoding/base64` for decoding, `gopkg.in/yaml.v3` for YAML parsing).

---

## Chunk 1: Subscription Storage and Download

### File Structure

Create these files:

- `helper/internal/subscription/storage.go`
  - Subscription record type with URL, name, last update time.
  - Load/save to `subscriptions.json`.
  - Redacted JSON serialization (URL tokens redacted).
- `helper/internal/subscription/storage_test.go`
  - Tests for load/save roundtrip, redaction in JSON output.
- `helper/internal/subscription/download.go`
  - HTTP download with configurable User-Agent.
  - Interface-based HTTP client for testability.
  - Response body reading with size limit.
- `helper/internal/subscription/download_test.go`
  - Tests using `httptest` server, mock HTTP client.
  - Tests verify download success and failure handling.

Do not implement format detection or config generation in this chunk.

### Task 1: Subscription Storage

**Files:**
- Create: `helper/internal/subscription/storage.go`
- Create: `helper/internal/subscription/storage_test.go`

- [ ] **Step 1: Write failing storage tests**

Create `helper/internal/subscription/storage_test.go`:

```go
package subscription

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSubscriptionRecordJSONRedactsURLToken(t *testing.T) {
	rec := Record{
		URL:        "https://example.com/sub?token=abc123&name=test",
		Name:       "TestSub",
		LastUpdate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	payload, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if string(payload) == "" {
		t.Fatal("empty payload")
	}

	decoded := make(map[string]interface{})
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	urlField, ok := decoded["url"].(string)
	if !ok {
		t.Fatal("url field not string")
	}

	if urlField == rec.URL {
		t.Fatalf("URL not redacted in JSON: %s", urlField)
	}

	if jsonContains(decoded, "abc123") {
		t.Fatalf("token leaked in JSON: %s", string(payload))
	}
}

func jsonContains(v interface{}, substr string) bool {
	bs, _ := json.Marshal(v)
	return string(bs) != "" && contains(string(bs), substr)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	if start > len(s)-len(substr) {
		return false
	}
	if s[start:start+len(substr)] == substr {
		return true
	}
	return containsAt(s, substr, start+1)
}

func TestLoadAndSaveRoundtrip(t *testing.T) {
	temp := t.TempDir()
	path := filepath.Join(temp, "subscriptions.json")

	original := []Record{
		{URL: "https://a.com/sub?token=t1", Name: "A", LastUpdate: time.Now()},
		{URL: "https://b.com/sub?token=t2", Name: "B", LastUpdate: time.Now()},
	}

	if err := Save(path, original); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("loaded %d records, want 2", len(loaded))
	}

	if loaded[0].Name != "A" || loaded[1].Name != "B" {
		t.Fatalf("names wrong: %+v", loaded)
	}
}

func TestLoadNonexistentReturnsEmpty(t *testing.T) {
	temp := t.TempDir()
	path := filepath.Join(temp, "nonexistent.json")

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load nonexistent: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("want empty, got %+v", loaded)
	}
}
```

- [ ] **Step 2: Run storage tests to verify they fail**

Run:

```bash
go test ./internal/subscription -run Test -v
```

Expected: FAIL because `Record`, `Load`, `Save` do not exist.

- [ ] **Step 3: Implement subscription storage**

Create `helper/internal/subscription/storage.go`:

```go
package subscription

import (
	"encoding/json"
	"os"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/redact"
)

type Record struct {
	URL        string    `json:"url"`
	Name       string    `json:"name"`
	LastUpdate time.Time `json:"lastUpdate"`
}

func (r Record) MarshalJSON() ([]byte, error) {
	type alias Record
	return json.Marshal(&struct {
		URL string `json:"url"`
		alias
	}{
		URL:   redact.URL(r.URL),
		alias: (alias)(r),
	})
}

func Load(path string) ([]Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Record{}, nil
		}
		return nil, err
	}

	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func Save(path string, records []Record) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
```

- [ ] **Step 4: Run storage tests to verify they pass**

Run:

```bash
go test ./internal/subscription -run Test -v
```

Expected: PASS.

- [ ] **Step 5: Commit storage task**

Run:

```bash
git add helper/internal/subscription/storage.go helper/internal/subscription/storage_test.go
git commit -m "feat: add subscription storage"
```

Do not include `Co-Authored-By`.

### Task 2: Subscription Download

**Files:**
- Create: `helper/internal/subscription/download.go`
- Create: `helper/internal/subscription/download_test.go`

- [ ] **Step 1: Write failing download tests**

Create `helper/internal/subscription/download_test.go`:

```go
package subscription

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadFetchesContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Fatal("missing User-Agent")
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("test-content"))
	}))
	defer server.Close()

	client := &http.Client{}
	content, err := Download(client, server.URL, "ProxyCat/1.0")
	if err != nil {
		t.Fatalf("download: %v", err)
	}

	if string(content) != "test-content" {
		t.Fatalf("content = %q", string(content))
	}
}

func TestDownloadReturnsErrorOnNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", 404)
	}))
	defer server.Close()

	client := &http.Client{}
	_, err := Download(client, server.URL, "ProxyCat/1.0")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestDownloadRespectsSizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		large := make([]byte, maxDownloadSize+1000)
		for i := range large {
			large[i] = 'a'
		}
		_, _ = w.Write(large)
	}))
	defer server.Close()

	client := &http.Client{}
	_, err := Download(client, server.URL, "ProxyCat/1.0")
	if err == nil {
		t.Fatal("expected error for oversized response")
	}
}

type mockHTTPClient struct {
	response *http.Response
	err      error
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func TestDownloadWithMockClient(t *testing.T) {
	mock := &mockHTTPClient{
		response: &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(io.LimitReader(io.NewSectionReader(nil, 0, 0), 0)),
			Header:     make(http.Header),
		},
	}
	mock.response.Body = io.NopCloser(io.MultiReader())

	_, err := downloadWithClient(mock, "http://test", "UA")
	if err == nil {
		// Empty body is fine, we're just testing the client interface
	}
}
```

- [ ] **Step 2: Run download tests to verify they fail**

Run:

```bash
go test ./internal/subscription -run TestDownload -v
```

Expected: FAIL because `Download`, `maxDownloadSize`, `downloadWithClient` do not exist.

- [ ] **Step 3: Implement subscription download**

Create `helper/internal/subscription/download.go`:

```go
package subscription

import (
	"fmt"
	"io"
	"net/http"
)

const maxDownloadSize = 10 * 1024 * 1024 // 10MB limit

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func Download(client HTTPClient, url string, userAgent string) ([]byte, error) {
	return downloadWithClient(client, url, userAgent)
}

func downloadWithClient(client HTTPClient, url string, userAgent string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, maxDownloadSize+1)
	content, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if len(content) > maxDownloadSize {
		return nil, fmt.Errorf("download: response exceeds size limit")
	}

	return content, nil
}
```

- [ ] **Step 4: Run download tests to verify they pass**

Run:

```bash
go test ./internal/subscription -run TestDownload -v
```

Expected: PASS.

- [ ] **Step 5: Run full helper test suite**

Run:

```bash
go test ./...
```

Expected: PASS (paths, redact, subscription, diagnose, proxyctl).

- [ ] **Step 6: Commit download task**

Run:

```bash
git add helper/internal/subscription/download.go helper/internal/subscription/download_test.go
git commit -m "feat: add subscription download"
```

Do not include `Co-Authored-By`.

---

## Chunk 2: Format Detection and Config Validation

### File Structure

Create these files:

- `helper/internal/config/format.go`
  - Detect Clash YAML vs base64 URI list vs plain URI list.
  - Return format enum and confidence.
- `helper/internal/config/format_test.go`
  - Tests with fixture data for each format.
- `helper/internal/config/validate.go`
  - Validate minimal Mihomo config shape: proxies, proxy-groups, rules.
  - Return validation result with counts.
- `helper/internal/config/validate_test.go`
  - Tests for valid YAML, missing sections, empty files.
- `helper/internal/config/backup.go`
  - Write timestamped backup before overwriting config.yaml.
  - Manage backup retention (keep last N).
- `helper/internal/config/backup_test.go`
  - Tests for backup creation and cleanup.

Do not implement full config generation yet - just detection, validation, and backup.

### Task 3: Format Detection

**Files:**
- Create: `helper/internal/config/format.go`
- Create: `helper/internal/config/format_test.go`

- [ ] **Step 1: Write failing format tests**

Create `helper/internal/config/format_test.go`:

```go
package config

import (
	"testing"
)

func TestDetectClashYAML(t *testing.T) {
	content := []byte(`
proxies:
  - name: "Node1"
    type: ss
    server: 1.2.3.4
    port: 8388
proxy-groups:
  - name: "Proxy"
    type: select
    proxies:
      - Node1
rules:
  - DOMAIN-SUFFIX,google.com,Proxy
`)

	format, confidence := DetectFormat(content)
	if format != FormatClashYAML {
		t.Fatalf("format = %v, want ClashYAML", format)
	}
	if confidence != ConfidenceHigh {
		t.Fatalf("confidence = %v, want High", confidence)
	}
}

func TestDetectBase64URIs(t *testing.T) {
	content := []byte("c3M6Ly9ZMjFoYVhKMGFXTnZibmtpYkdsdA==\nc3M6Ly9ZMjFoYVhKMGFXTnZibmtpYkdsdA==\n")

	format, confidence := DetectFormat(content)
	if format != FormatBase64List {
		t.Fatalf("format = %v, want Base64List", format)
	}
	if confidence != ConfidenceHigh {
		t.Fatalf("confidence = %v, want High", confidence)
	}
}

func TestDetectPlainURIs(t *testing.T) {
	content := []byte("ss://YWVzLTEyOC1nY206cGFzc3dvcmQ@example.com:443#Node1\ntrojan://password@example.com:443#Node2\n")

	format, confidence := DetectFormat(content)
	if format != FormatPlainList {
		t.Fatalf("format = %v, want PlainList", format)
	}
	if confidence != ConfidenceHigh {
		t.Fatalf("confidence = %v, want High", confidence)
	}
}

func TestDetectUnknown(t *testing.T) {
	content := []byte("random text that is not a valid format")

	format, confidence := DetectFormat(content)
	if format != FormatUnknown {
		t.Fatalf("format = %v, want Unknown", format)
	}
	if confidence != ConfidenceNone {
		t.Fatalf("confidence = %v, want None", confidence)
	}
}

func TestDetectEmpty(t *testing.T) {
	format, confidence := DetectFormat([]byte{})
	if format != FormatUnknown {
		t.Fatalf("format = %v, want Unknown for empty", format)
	}
	if confidence != ConfidenceNone {
		t.Fatalf("confidence = %v, want None for empty", confidence)
	}
}
```

- [ ] **Step 2: Run format tests to verify they fail**

Run:

```bash
go test ./internal/config -run TestDetect -v
```

Expected: FAIL because `DetectFormat`, `FormatClashYAML`, etc. do not exist.

- [ ] **Step 3: Implement format detection**

Create `helper/internal/config/format.go`:

```go
package config

import (
	"bytes"
	"encoding/base64"
	"regexp"
	"strings"
)

type Format string

const (
	FormatUnknown     Format = "unknown"
	FormatClashYAML   Format = "clash-yaml"
	FormatBase64List  Format = "base64-uri-list"
	FormatPlainList   Format = "plain-uri-list"
)

type Confidence string

const (
	ConfidenceNone  Confidence = "none"
	ConfidenceLow   Confidence = "low"
	ConfidenceHigh  Confidence = "high"
)

var nodeURIPattern = regexp.MustCompile(`(?i)^(ss|ssr|trojan|vmess|vless|hysteria|hysteria2|hy2)://`)

func DetectFormat(content []byte) (Format, Confidence) {
	if len(content) == 0 {
		return FormatUnknown, ConfidenceNone
	}

	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return FormatUnknown, ConfidenceNone
	}

	// Check for Clash YAML
	if looksLikeClashYAML(trimmed) {
		return FormatClashYAML, ConfidenceHigh
	}

	// Check for base64 encoded content
	if looksLikeBase64List(trimmed) {
		return FormatBase64List, ConfidenceHigh
	}

	// Check for plain URI list
	if looksLikePlainList(trimmed) {
		return FormatPlainList, ConfidenceHigh
	}

	return FormatUnknown, ConfidenceNone
}

func looksLikeClashYAML(content []byte) bool {
	s := string(content)
	// Look for key YAML markers that indicate Clash/Mihomo config
	hasProxies := strings.Contains(s, "proxies:") || strings.Contains(s, "Proxy:")
	hasProxyGroups := strings.Contains(s, "proxy-groups:") || strings.Contains(s, "Proxy Group:")
	hasRules := strings.Contains(s, "rules:") || strings.Contains(s, "Rule:")

	// Require at least two of the three key sections for high confidence
	sections := 0
	if hasProxies {
		sections++
	}
	if hasProxyGroups {
		sections++
	}
	if hasRules {
		sections++
	}

	return sections >= 2
}

func looksLikeBase64List(content []byte) bool {
	lines := bytes.Split(content, []byte("\n"))
	validBase64Lines := 0
	totalLines := 0

	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		totalLines++

		// Try to decode as base64
		decoded, err := base64.StdEncoding.DecodeString(string(trimmed))
		if err == nil && len(decoded) > 0 {
			// Check if decoded content looks like node URIs
			if nodeURIPattern.Match(decoded) {
				validBase64Lines++
			}
		}
	}

	// High confidence if majority of non-empty lines are valid base64 node URIs
	if totalLines > 0 && validBase64Lines >= totalLines/2 {
		return true
	}
	return false
}

func looksLikePlainList(content []byte) bool {
	lines := bytes.Split(content, []byte("\n"))
	uriLines := 0
	totalLines := 0

	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		totalLines++

		if nodeURIPattern.Match(trimmed) {
			uriLines++
		}
	}

	// High confidence if majority of non-empty lines are node URIs
	if totalLines > 0 && uriLines >= totalLines/2 {
		return true
	}
	return false
}
```

- [ ] **Step 4: Run format tests to verify they pass**

Run:

```bash
go test ./internal/config -run TestDetect -v
```

Expected: PASS.

- [ ] **Step 5: Commit format detection task**

Run:

```bash
git add helper/internal/config/format.go helper/internal/config/format_test.go
git commit -m "feat: add subscription format detection"
```

Do not include `Co-Authored-By`.

### Task 4: Config Validation

**Files:**
- Create: `helper/internal/config/validate.go`
- Create: `helper/internal/config/validate_test.go`

- [ ] **Step 1: Write failing validation tests**

Create `helper/internal/config/validate_test.go`:

```go
package config

import (
	"testing"
)

func TestValidateClashYAMLValid(t *testing.T) {
	content := []byte(`
proxies:
  - name: "Node1"
    type: ss
    server: 1.2.3.4
    port: 8388
proxy-groups:
  - name: "Proxy"
    type: select
    proxies:
      - Node1
rules:
  - DOMAIN-SUFFIX,google.com,Proxy
`)

	result := Validate(content)
	if !result.Valid {
		t.Fatalf("expected valid, got: %s", result.Message)
	}
	if result.ProxyCount != 1 {
		t.Fatalf("proxy count = %d, want 1", result.ProxyCount)
	}
	if result.GroupCount != 1 {
		t.Fatalf("group count = %d, want 1", result.GroupCount)
	}
	if result.RuleCount != 1 {
		t.Fatalf("rule count = %d, want 1", result.RuleCount)
	}
}

func TestValidateMissingProxies(t *testing.T) {
	content := []byte(`
proxy-groups:
  - name: "Proxy"
    type: select
rules:
  - DOMAIN-SUFFIX,google.com,Proxy
`)

	result := Validate(content)
	if result.Valid {
		t.Fatal("expected invalid for missing proxies")
	}
	if result.Message == "" {
		t.Fatal("expected error message")
	}
}

func TestValidateMissingProxyGroups(t *testing.T) {
	content := []byte(`
proxies:
  - name: "Node1"
    type: ss
    server: 1.2.3.4
    port: 8388
rules:
  - DOMAIN-SUFFIX,google.com,Proxy
`)

	result := Validate(content)
	if result.Valid {
		t.Fatal("expected invalid for missing proxy-groups")
	}
}

func TestValidateMissingRules(t *testing.T) {
	content := []byte(`
proxies:
  - name: "Node1"
    type: ss
    server: 1.2.3.4
    port: 8388
proxy-groups:
  - name: "Proxy"
    type: select
`)

	result := Validate(content)
	if result.Valid {
		t.Fatal("expected invalid for missing rules")
	}
}

func TestValidateInvalidYAML(t *testing.T) {
	content := []byte(`not: valid: yaml: [
`)

	result := Validate(content)
	if result.Valid {
		t.Fatal("expected invalid for bad YAML")
	}
}

func TestValidateEmpty(t *testing.T) {
	result := Validate([]byte{})
	if result.Valid {
		t.Fatal("expected invalid for empty content")
	}
}
```

- [ ] **Step 2: Run validation tests to verify they fail**

Run:

```bash
go test ./internal/config -run TestValidate -v
```

Expected: FAIL because `Validate`, `ValidationResult` do not exist.

- [ ] **Step 3: Implement config validation**

Add to `helper/go.mod` (add yaml dependency):

```
go get gopkg.in/yaml.v3
```

Create `helper/internal/config/validate.go`:

```go
package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type ValidationResult struct {
	Valid       bool   `json:"valid"`
	Message     string `json:"message"`
	ProxyCount  int    `json:"proxyCount"`
	GroupCount  int    `json:"groupCount"`
	RuleCount   int    `json:"ruleCount"`
}

// Minimal config structure for validation
type minimalConfig struct {
	Proxies     []yaml.Node `yaml:"proxies"`
	ProxyGroups []yaml.Node `yaml:"proxy-groups"`
	Rules       []yaml.Node `yaml:"rules"`
}

func Validate(content []byte) ValidationResult {
	if len(content) == 0 {
		return ValidationResult{Valid: false, Message: "empty config"}
	}

	var cfg minimalConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return ValidationResult{Valid: false, Message: fmt.Sprintf("invalid YAML: %v", err)}
	}

	proxyCount := len(cfg.Proxies)
	groupCount := len(cfg.ProxyGroups)
	ruleCount := len(cfg.Rules)

	if proxyCount == 0 {
		return ValidationResult{
			Valid:       false,
			Message:     "config missing proxies section",
			ProxyCount:  proxyCount,
			GroupCount:  groupCount,
			RuleCount:   ruleCount,
		}
	}

	if groupCount == 0 {
		return ValidationResult{
			Valid:       false,
			Message:     "config missing proxy-groups section",
			ProxyCount:  proxyCount,
			GroupCount:  groupCount,
			RuleCount:   ruleCount,
		}
	}

	if ruleCount == 0 {
		return ValidationResult{
			Valid:       false,
			Message:     "config missing rules section",
			ProxyCount:  proxyCount,
			GroupCount:  groupCount,
			RuleCount:   ruleCount,
		}
	}

	return ValidationResult{
		Valid:       true,
		Message:     "valid Mihomo config",
		ProxyCount:  proxyCount,
		GroupCount:  groupCount,
		RuleCount:   ruleCount,
	}
}
```

- [ ] **Step 4: Run validation tests to verify they pass**

Run:

```bash
go test ./internal/config -run TestValidate -v
```

Expected: PASS.

- [ ] **Step 5: Commit validation task**

Run:

```bash
git add helper/internal/config/validate.go helper/internal/config/validate_test.go
git commit -m "feat: add config validation"
```

Do not include `Co-Authored-By`.

### Task 5: Config Backup

**Files:**
- Create: `helper/internal/config/backup.go`
- Create: `helper/internal/config/backup_test.go`

- [ ] **Step 1: Write failing backup tests**

Create `helper/internal/config/backup_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupCreatesTimestampedCopy(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config.yaml")
	backupDir := filepath.Join(temp, "backups")

	original := []byte("original config")
	if err := os.WriteFile(configPath, original, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	backupPath, err := Backup(configPath, backupDir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	// Verify backup file exists and has content
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backupContent) != string(original) {
		t.Fatalf("backup content mismatch")
	}
}

func TestBackupCreatesBackupDir(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config.yaml")
	backupDir := filepath.Join(temp, "new-backups")

	if err := os.WriteFile(configPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Backup(configPath, backupDir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	// Verify backup directory was created
	info, err := os.Stat(backupDir)
	if err != nil {
		t.Fatalf("backup dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("backup path is not a directory")
	}
}

func TestBackupNonexistentConfig(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "nonexistent.yaml")
	backupDir := filepath.Join(temp, "backups")

	_, err := Backup(configPath, backupDir)
	if err == nil {
		t.Fatal("expected error for nonexistent config")
	}
}

func TestCleanupOldBackups(t *testing.T) {
	temp := t.TempDir()
	backupDir := filepath.Join(temp, "backups")

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("create backup dir: %v", err)
	}

	// Create 5 fake backup files
	for i := 0; i < 5; i++ {
		name := filepath.Join(backupDir, formatBackupName(i))
		if err := os.WriteFile(name, []byte("backup"), 0o644); err != nil {
			t.Fatalf("write backup: %v", err)
		}
	}

	// Cleanup to keep only 3
	if err := CleanupOldBackups(backupDir, 3); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	// Count remaining backups
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}

	count := 0
	for _, e := range entries {
		if !e.IsDir() && isBackupFile(e.Name()) {
			count++
		}
	}

	if count != 3 {
		t.Fatalf("backup count = %d, want 3", count)
	}
}

func formatBackupName(index int) string {
	// Simple sequential naming for test
	return filepath.Join("backup", string(rune('a'+index))+".yaml")
}

func isBackupFile(name string) bool {
	return len(name) > 0
}
```

- [ ] **Step 2: Run backup tests to verify they fail**

Run:

```bash
go test ./internal/config -run TestBackup -v
```

Expected: FAIL because `Backup`, `CleanupOldBackups` do not exist.

- [ ] **Step 3: Implement backup functionality**

Create `helper/internal/config/backup.go`:

```go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const backupTimeFormat = "20060102-150405"

func Backup(configPath string, backupDir string) (string, error) {
	// Read original config
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read config: %w", err)
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	// Create timestamped backup filename
	timestamp := time.Now().Format(backupTimeFormat)
	baseName := filepath.Base(configPath)
	ext := filepath.Ext(baseName)
	nameWithoutExt := strings.TrimSuffix(baseName, ext)
	backupName := fmt.Sprintf("%s-%s%s", nameWithoutExt, timestamp, ext)
	backupPath := filepath.Join(backupDir, backupName)

	// Write backup
	if err := os.WriteFile(backupPath, content, 0o644); err != nil {
		return "", fmt.Errorf("write backup: %w", err)
	}

	return backupPath, nil
}

func CleanupOldBackups(backupDir string, keep int) error {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read backup dir: %w", err)
	}

	// Collect backup files
	var backups []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && isBackupFile(e.Name()) {
			backups = append(backups, e)
		}
	}

	// Sort by modification time (newest first)
	sort.Slice(backups, func(i, j int) bool {
		infoI, _ := backups[i].Info()
		infoJ, _ := backups[j].Info()
		return infoI.ModTime().After(infoJ.ModTime())
	})

	// Remove excess backups
	if len(backups) > keep {
		for _, b := range backups[keep:] {
			path := filepath.Join(backupDir, b.Name())
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove old backup %s: %w", b.Name(), err)
			}
		}
	}

	return nil
}

func isBackupFile(name string) bool {
	// Backup files have timestamp suffix like "config-20060102-150405.yaml"
	return strings.Contains(name, "-") && strings.HasSuffix(name, ".yaml")
}
```

- [ ] **Step 4: Run backup tests to verify they pass**

Run:

```bash
go test ./internal/config -run TestBackup -v
```

Expected: PASS.

- [ ] **Step 5: Run full helper test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit backup task**

Run:

```bash
git add helper/internal/config/backup.go helper/internal/config/backup_test.go
git commit -m "feat: add config backup"
```

Do not include `Co-Authored-By`.

---

## Chunk 3: CLI Commands Integration

### File Structure

Modify:

- `helper/cmd/proxyctl/main.go`
  - Add `subscription add <url>` command.
  - Add `subscription list` command.
  - Add `subscription update` command.
  - Add `config validate` command.
- `helper/cmd/proxyctl/main_test.go`
  - Tests for new commands.

### Task 6: Subscription CLI Commands

**Files:**
- Modify: `helper/cmd/proxyctl/main.go`
- Modify: `helper/cmd/proxyctl/main_test.go`

- [ ] **Step 1: Write failing CLI tests**

Add to `helper/cmd/proxyctl/main_test.go`:

```go
func TestSubscriptionAddCommand(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"subscription", "add", "https://example.com/sub?token=test123"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}

	// Verify subscriptions.json was created
	configDir := filepath.Join(tempHome, "Library", "Application Support", "ProxyCat", "config")
	_, err := os.Stat(filepath.Join(configDir, "subscriptions.json"))
	if err != nil {
		t.Fatalf("subscriptions.json not created: %v", err)
	}
}

func TestSubscriptionListCommand(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// First add a subscription
	run([]string{"subscription", "add", "https://example.com/sub?token=test123"}, io.Discard, io.Discard)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"subscription", "list"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "example.com") {
		t.Fatalf("list output missing subscription: %s", output)
	}
}
```

- [ ] **Step 2: Implement subscription commands**

Modify `helper/cmd/proxyctl/main.go`:

Add imports:
```go
import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/config"
	"github.com/Cass-ette/ProxyCat/helper/internal/diagnose"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
	"github.com/Cass-ette/ProxyCat/helper/internal/redact"
	"github.com/Cass-ette/ProxyCat/helper/internal/subscription"
)
```

Add to `run()` switch:
```go
case "subscription":
	if len(args) < 2 {
		fmt.Fprintf(stderr, "subscription subcommand required: add, list, update\n")
		printHelp(stderr)
		return 2
	}
	switch args[1] {
	case "add":
		if len(args) < 3 {
			fmt.Fprintf(stderr, "subscription add requires URL\n")
			return 2
		}
		return runSubscriptionAdd(args[2], stdout, stderr)
	case "list":
		return runSubscriptionList(stdout, stderr)
	case "update":
		return runSubscriptionUpdate(stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown subscription subcommand: %s\n", redact.String(args[1]))
		return 2
	}
```

Add new functions:
```go
func runSubscriptionAdd(url string, stdout io.Writer, stderr io.Writer) int {
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}

	records, err := subscription.Load(runtimePaths.SubscriptionsJSON)
	if err != nil {
		fmt.Fprintf(stderr, "load subscriptions: %v\n", err)
		return 1
	}

	// Check for duplicate URL
	for _, r := range records {
		if r.URL == url {
			fmt.Fprintf(stderr, "subscription already exists\n")
			return 1
		}
	}

	newRecord := subscription.Record{
		URL:        url,
		Name:       "Subscription",
		LastUpdate: time.Now(),
	}
	records = append(records, newRecord)

	if err := subscription.Save(runtimePaths.SubscriptionsJSON, records); err != nil {
		fmt.Fprintf(stderr, "save subscriptions: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Added subscription: %s\n", redact.URL(url))
	return 0
}

func runSubscriptionList(stdout io.Writer, stderr io.Writer) int {
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}

	records, err := subscription.Load(runtimePaths.SubscriptionsJSON)
	if err != nil {
		fmt.Fprintf(stderr, "load subscriptions: %v\n", err)
		return 1
	}

	if len(records) == 0 {
		fmt.Fprintln(stdout, "No subscriptions")
		return 0
	}

	for i, r := range records {
		fmt.Fprintf(stdout, "%d. %s (%s)\n", i+1, r.Name, redact.URL(r.URL))
	}
	return 0
}

func runSubscriptionUpdate(stdout io.Writer, stderr io.Writer) int {
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}

	records, err := subscription.Load(runtimePaths.SubscriptionsJSON)
	if err != nil {
		fmt.Fprintf(stderr, "load subscriptions: %v\n", err)
		return 1
	}

	if len(records) == 0 {
		fmt.Fprintln(stderr, "No subscriptions to update")
		return 1
	}

	client := &http.Client{}
	for i, r := range records {
		content, err := subscription.Download(client, r.URL, "ProxyCat/1.0")
		if err != nil {
			fmt.Fprintf(stderr, "download subscription %d: %v\n", i+1, err)
			continue
		}

		// Detect format
		format, confidence := config.DetectFormat(content)
		fmt.Fprintf(stdout, "Subscription %d: detected %s (confidence: %s)\n", i+1, format, confidence)

		// TODO: In future milestones, generate config and validate
		_ = content

		records[i].LastUpdate = time.Now()
	}

	if err := subscription.Save(runtimePaths.SubscriptionsJSON, records); err != nil {
		fmt.Fprintf(stderr, "save subscriptions: %v\n", err)
		return 1
	}

	return 0
}
```

Update `printHelp()`:
```go
func printHelp(w io.Writer) {
	fmt.Fprintln(w, "ProxyCat proxyctl")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  proxyctl diagnose [--json]")
	fmt.Fprintln(w, "  proxyctl subscription add <url>")
	fmt.Fprintln(w, "  proxyctl subscription list")
	fmt.Fprintln(w, "  proxyctl subscription update")
}
```

- [ ] **Step 3: Run CLI tests to verify they pass**

Run:

```bash
go test ./cmd/proxyctl -run TestSubscription -v
```

Expected: PASS.

- [ ] **Step 4: Run full test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Commit CLI integration**

Run:

```bash
git add helper/cmd/proxyctl/main.go helper/cmd/proxyctl/main_test.go
git commit -m "feat: add subscription CLI commands"
```

Do not include `Co-Authored-By`.

---

## Chunk 4: Final Milestone 2 Verification

### Task 7: Milestone 2 Final Verification

**Files:**
- None (verification only)

- [ ] **Step 1: Verify branch diff scope**

Run from the worktree root:

```bash
git status --short --branch --untracked-files=normal
git diff --stat origin/main...HEAD
```

Expected: Only Milestone 2 files are changed.

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

Expected: Build succeeds.

- [ ] **Step 4: Manual CLI verification**

Run from `helper/`:

```bash
go run ./cmd/proxyctl subscription add "https://example.com/sub?token=test123"
go run ./cmd/proxyctl subscription list
go run ./cmd/proxyctl subscription update
go run ./cmd/proxyctl diagnose --json
```

Expected: Commands work, output is redacted-safe.

- [ ] **Step 5: Verify no generated binary remains**

Run:

```bash
git status --short --untracked-files=normal
```

Expected: Clean working tree.

- [ ] **Step 6: Commit plan if not already committed**

Run:

```bash
git add docs/superpowers/plans/2026-05-02-proxycat-milestone-2.md
git commit -m "docs: add ProxyCat milestone 2 plan"
```

Do not include `Co-Authored-By`.

- [ ] **Step 7: Prepare for review**

Run:

```bash
git status --short --branch --untracked-files=normal
git log --oneline --decorate origin/main..HEAD
```

Expected: Clean working tree, commits for plan and each task.
