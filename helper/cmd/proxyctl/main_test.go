package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
			SuggestedFix string `json:"suggestedFix"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if decoded.App != "ProxyCat" || decoded.Milestone != "milestone-1" || len(decoded.Checks) != 5 {
		t.Fatalf("unexpected diagnose output: %+v", decoded)
	}
	for _, check := range decoded.Checks {
		if check.Name == "network-checks" {
			t.Fatal("diagnose output should not include obsolete network-checks placeholder")
		}
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

func TestConfigGenerateUsesProbeSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var firstUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if firstUserAgent == "" {
			firstUserAgent = r.Header.Get("User-Agent")
		}
		_, _ = w.Write([]byte(`proxies:
  - name: Node1
    type: trojan
    server: example.com
    port: 443
    password: secret
proxy-groups:
  - name: Proxy
    type: select
    proxies:
      - Node1
rules:
  - MATCH,Proxy
`))
	}))
	defer server.Close()

	run([]string{"subscription", "add", server.URL}, io.Discard, io.Discard)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"config", "generate"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}
	if firstUserAgent != "clash-verge/v1.7.7" {
		t.Fatalf("first User-Agent = %q, want clash-verge/v1.7.7", firstUserAgent)
	}
	if !strings.Contains(stdout.String(), "Selected subscription format: clash-yaml via clash-verge/v1.7.7") {
		t.Fatalf("config generate output missing probe selection: %s", stdout.String())
	}
}

func TestSubscriptionProbeJSONCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`proxies:
  - name: Node1
    type: trojan
    server: example.com
    port: 443
    password: secret
proxy-groups:
  - name: Proxy
    type: select
    proxies:
      - Node1
rules:
  - MATCH,Proxy
`))
	}))
	defer server.Close()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"subscription", "probe", server.URL + "?token=secret", "--json"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}
	if strings.Contains(stdout.String(), "secret") {
		t.Fatalf("probe output leaked token: %s", stdout.String())
	}
	var decoded struct {
		Selected *struct {
			Format string `json:"format"`
		} `json:"selected"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if decoded.Selected == nil || decoded.Selected.Format != "clash-yaml" {
		t.Fatalf("unexpected probe output: %s", stdout.String())
	}
}

func TestSubscriptionUpdateShowsSuggestedFixForInvalidProbe(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<!doctype html><html><body>login</body></html>"))
	}))
	defer server.Close()

	run([]string{"subscription", "add", server.URL}, io.Discard, io.Discard)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"subscription", "update"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "建议：") {
		t.Fatalf("update output missing suggested fix: %s", stdout.String())
	}
}

func TestSubscriptionUpdateUsesProbe(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`proxies:
  - name: Node1
    type: trojan
    server: example.com
    port: 443
    password: secret
proxy-groups:
  - name: Proxy
    type: select
    proxies:
      - Node1
rules:
  - MATCH,Proxy
`))
	}))
	defer server.Close()

	// Add subscription pointing to test server
	run([]string{"subscription", "add", server.URL}, io.Discard, io.Discard)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"subscription", "update"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "selected clash-yaml") {
		t.Fatalf("update output missing probe selection: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Valid: 1 proxies") {
		t.Fatalf("update output missing proxy count: %s", stdout.String())
	}
}

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

func TestCoreStatusJSONCommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"core", "status", "--json"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var decoded struct {
		Running bool `json:"running"`
		PID     int  `json:"pid"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
}

func TestTestJSONCommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"test", "--json"}, stdout, stderr)
	if exitCode != 0 && exitCode != 1 {
		t.Fatalf("unexpected exitCode = %d, stderr = %s", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want JSON-only output", stderr.String())
	}

	var decoded struct {
		GoogleOK bool   `json:"googleOK"`
		GitHubOK bool   `json:"githubOK"`
		Error    string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
}
