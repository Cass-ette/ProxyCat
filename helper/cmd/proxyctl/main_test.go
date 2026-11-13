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
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/controller"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
	"github.com/Cass-ette/ProxyCat/helper/internal/preferences"
	"github.com/Cass-ette/ProxyCat/helper/internal/profile"
)

func installAcceptingMihomo(t *testing.T) {
	t.Helper()
	p, err := paths.Default()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p.Mihomo), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n[ \"$1\" = \"-t\" ] && [ \"$2\" = \"-d\" ] && [ \"$4\" = \"-f\" ] && [ -s \"$5\" ]\n"
	if err := os.WriteFile(p.Mihomo, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

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

func TestBootstrapInstallsMihomoBeforeValidatingConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("proxies:\n  - name: Node\n    type: direct\nproxy-groups:\n  - name: Proxy\n    type: select\n    proxies: [Node]\nrules:\n  - MATCH,Proxy\n"))
	}))
	defer server.Close()

	profileID, err := saveProfileSubscription(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	p, err := paths.Default()
	if err != nil {
		t.Fatal(err)
	}
	previousInstaller := installMihomoBinary
	installMihomoBinary = func(path string) (string, error) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\n[ \"$1\" = \"-t\" ] && [ \"$2\" = \"-d\" ] && [ \"$4\" = \"-f\" ] && [ -s \"$5\" ]\n"), 0o755); err != nil {
			return "", err
		}
		return path, nil
	}
	t.Cleanup(func() { installMihomoBinary = previousInstaller })

	var stdout, stderr bytes.Buffer
	if code := prepareBootstrapProfile(profileID, &stdout, &stderr); code != 0 {
		t.Fatalf("bootstrap preparation exit = %d, stderr=%s, stdout=%s", code, stderr.String(), stdout.String())
	}
	if _, err := os.Stat(p.Mihomo); err != nil {
		t.Fatalf("Mihomo was not installed before config generation: %v", err)
	}
	activeConfig, err := os.ReadFile(p.ConfigYAML)
	if err != nil || !strings.Contains(string(activeConfig), "MATCH,Proxy") {
		t.Fatalf("bootstrap did not activate generated config: %q, %v", activeConfig, err)
	}
}

func TestWebsitePreferenceWithoutActiveConfigDoesNotClaimRestart(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	result, err := commitConfiguration(p, map[string][]byte{p.PreferencesJSON: []byte(`{"preferences":[]}`)})
	if err != nil || result.State != "inactive" || strings.Contains(result.Message, "已应用") {
		t.Fatalf("unexpected website preference status: %+v %v", result, err)
	}
}

func TestMaintenanceGeoChangeUsesObservationTimeInsteadOfFileMtime(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	geoDir := filepath.Join(home, ".config", "mihomo")
	if err := os.MkdirAll(geoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	geoFile := filepath.Join(geoDir, "geoip.metadb")
	if err := os.WriteFile(geoFile, []byte("initial"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldMtime := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(geoFile, oldMtime, oldMtime); err != nil {
		t.Fatal(err)
	}
	var first bytes.Buffer
	if code := run([]string{"maintenance", "status", "--json"}, &first, io.Discard); code != 0 {
		t.Fatalf("initial status exit = %d", code)
	}
	changedMtime := time.Now().Add(-12 * time.Hour)
	if err := os.Chtimes(geoFile, changedMtime, changedMtime); err != nil {
		t.Fatal(err)
	}
	observedAfter := time.Now().UTC()
	var second bytes.Buffer
	if code := run([]string{"maintenance", "status", "--json"}, &second, io.Discard); code != 0 {
		t.Fatalf("updated status exit = %d", code)
	}
	var status struct {
		GeoLastObservedChange string `json:"geoLastObservedChange"`
	}
	if err := json.Unmarshal(second.Bytes(), &status); err != nil {
		t.Fatalf("status is not JSON: %v\n%s", err, second.String())
	}
	changedAt, err := time.Parse(time.RFC3339, status.GeoLastObservedChange)
	if err != nil || changedAt.Before(observedAfter.Add(-time.Second)) {
		t.Fatalf("geo observation time = %q, err=%v, expected a recent observation", status.GeoLastObservedChange, err)
	}
}

func TestConfigGenerateUsesProbeSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	installAcceptingMihomo(t)
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
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "建议：") {
		t.Fatalf("update output missing suggested fix: %s", stdout.String())
	}
}

func TestSubscriptionUpdateDoesNotReportLegacyProbeAsSuccess(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	installAcceptingMihomo(t)

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
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "selected clash-yaml") {
		t.Fatalf("update output missing probe selection: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "仅完成连通性检查") || !strings.Contains(stdout.String(), "未生成或应用配置") {
		t.Fatalf("update output reported no clear legacy limitation: %s", stdout.String())
	}
}

func TestSubscriptionUpdateKeepsOldConfigWhenMihomoRejectsCandidate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("proxies:\n  - name: NewNode\n    type: direct\nproxy-groups:\n  - name: Proxy\n    type: select\n    proxies: [NewNode]\nrules:\n  - MATCH,Proxy\n"))
	}))
	defer server.Close()

	profileID, err := saveProfileSubscription(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	p, err := paths.Default()
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := profile.LoadAll(p.ProfilesDir)
	if err != nil || len(profiles) != 1 {
		t.Fatalf("load test profile: %v, %v", profiles, err)
	}
	profiles[0].Active = true
	if err := profile.SaveAll(p.ProfilesDir, profiles); err != nil {
		t.Fatal(err)
	}
	oldProfileConfig := []byte("old profile config")
	oldActiveConfig := []byte("old active config")
	for path, content := range map[string][]byte{
		profile.ProfileConfigPath(p.ProfilesDir, profileID): oldProfileConfig,
		p.ConfigYAML: oldActiveConfig,
	} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(p.Mihomo), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Mihomo, []byte("#!/bin/sh\necho 'bad URL https://user:pass@example.com/sub?token=secret' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"subscription", "update"}, &stdout, &stderr); code != 1 {
		t.Fatalf("update exit = %d, stderr=%s", code, stderr.String())
	}
	for path, want := range map[string][]byte{
		profile.ProfileConfigPath(p.ProfilesDir, profileID): oldProfileConfig,
		p.ConfigYAML: oldActiveConfig,
	} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(want) {
			t.Fatalf("failed update changed %s: %q, %v", path, got, err)
		}
	}
	if strings.Contains(stdout.String(), "user:pass") || strings.Contains(stdout.String(), "token=secret") {
		t.Fatalf("validation output leaked credentials: %s", stdout.String())
	}
}

func TestWebsitePreferenceValidationFailurePreservesStoredFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p, err := paths.Default()
	if err != nil {
		t.Fatal(err)
	}
	oldConfig := []byte("proxies: [{name: Node, type: direct}]\nproxy-groups: [{name: Proxy, type: select, proxies: [Node]}]\nrules: [\"MATCH,Proxy\"]\n")
	basePath := subscriptionBasePath(p.ConfigYAML)
	if err := os.MkdirAll(filepath.Dir(p.ConfigYAML), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{p.ConfigYAML, basePath} {
		if err := os.WriteFile(path, oldConfig, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(p.Mihomo), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Mihomo, []byte("#!/bin/sh\necho invalid config >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"preferences", "set", "proxy", "example.com", "--json"}, &stdout, &stderr); code != 1 {
		t.Fatalf("preference exit = %d, stderr=%s", code, stderr.String())
	}
	for _, path := range []string{p.ConfigYAML, basePath} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(oldConfig) {
			t.Fatalf("failed preference changed %s: %q, %v", path, got, err)
		}
	}
	if _, err := os.Stat(p.PreferencesJSON); !os.IsNotExist(err) {
		t.Fatalf("failed preference was persisted: %v", err)
	}
}

func TestActivatingProfileSwitchesItsSubscriptionBaseAtomically(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p, err := paths.Default()
	if err != nil {
		t.Fatal(err)
	}
	const profileID = "selected"
	profileDir, err := profile.EnsureProfileDir(p.ProfilesDir, profileID)
	if err != nil {
		t.Fatal(err)
	}
	base := []byte("proxies: [{name: Node, type: direct}]\nproxy-groups: [{name: Proxy, type: select, proxies: [Node]}]\nrules: [\"MATCH,Proxy\"]\n")
	effective, err := preferences.ApplyToYAML(base, preferences.Store{Preferences: []preferences.Preference{{Host: "example.com", Route: "proxy"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "config.yaml"), []byte(effective), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(subscriptionBasePath(filepath.Join(profileDir, "config.yaml")), base, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := profile.SaveAll(p.ProfilesDir, []profile.Profile{{ID: profileID, Name: "Selected"}}); err != nil {
		t.Fatal(err)
	}
	if err := preferences.Save(p.PreferencesJSON, preferences.Store{Preferences: []preferences.Preference{{Host: "example.com", Route: "proxy"}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p.ConfigYAML), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ConfigYAML, []byte("old active config"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(subscriptionBasePath(p.ConfigYAML), []byte("old active base"), 0o644); err != nil {
		t.Fatal(err)
	}
	installAcceptingMihomo(t)
	if err := activateProfileWithPreferences(p, profileID); err != nil {
		t.Fatal(err)
	}
	activeConfig, err := os.ReadFile(p.ConfigYAML)
	if err != nil || !strings.Contains(string(activeConfig), "DOMAIN-SUFFIX,example.com,Proxy") {
		t.Fatalf("active config did not apply website preference: %s, %v", activeConfig, err)
	}
	activeBase, err := os.ReadFile(subscriptionBasePath(p.ConfigYAML))
	if err != nil || strings.Contains(string(activeBase), "DOMAIN-SUFFIX,example.com") || !strings.Contains(string(activeBase), "MATCH,Proxy") {
		t.Fatalf("active subscription base is stale or contains preference rules: %s, %v", activeBase, err)
	}
	profiles, err := profile.LoadAll(p.ProfilesDir)
	if err != nil || len(profiles) != 1 || !profiles[0].Active {
		t.Fatalf("profile index did not activate selected profile: %+v, %v", profiles, err)
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
	if exitCode != 0 {
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

func TestFailedConnectionResultOutput(t *testing.T) {
	result := &controller.TestResult{GoogleOK: false, GitHubOK: true, Error: "Google returned HTTP 503 (expected 204)"}
	for _, jsonOutput := range []bool{true, false} {
		var stdout, stderr bytes.Buffer
		exitCode := writeTestResult(result, &stdout, &stderr, jsonOutput)
		if jsonOutput {
			if exitCode != 0 || stderr.Len() != 0 {
				t.Fatalf("JSON failure must be returned in-band: exit=%d stderr=%s", exitCode, &stderr)
			}
			var decoded controller.TestResult
			if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil || decoded != *result {
				t.Fatalf("structured failure lost: result=%+v err=%v", decoded, err)
			}
		} else if exitCode != 1 || !strings.Contains(stderr.String(), result.Error) || !strings.Contains(stdout.String(), "Google: FAIL") {
			t.Fatalf("human CLI must retain nonzero status and diagnostic: exit=%d stdout=%s stderr=%s", exitCode, &stdout, &stderr)
		}
	}
}

func TestUpdateClientUsesSystemProxy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "scutil"), []byte("#!/bin/sh\nprintf '<dictionary> {\\n HTTPSProxy : 127.0.0.1\\n HTTPSPort : 17890\\n HTTPSEnable : 1\\n}\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	client := updateHTTPClient()
	for _, endpoint := range []string{"https://api.github.com", "https://github.com/Cass-ette/ProxyCat/releases/download/v0.13.1/ProxyCat-0.13.1.dmg", "https://release-assets.githubusercontent.com/redirected.dmg"} {
		request, _ := http.NewRequest("GET", endpoint, nil)
		proxy, err := client.Transport.(*http.Transport).Proxy(request)
		if err != nil || proxy == nil || proxy.String() != "http://127.0.0.1:17890" || client.Timeout == 0 {
			t.Fatalf("update requests including asset redirects must honor enabled macOS HTTPS proxy: endpoint=%s proxy=%v err=%v", endpoint, proxy, err)
		}
	}
}

func TestWebsiteCheckRejectsInvalidInputWithoutLeakingURL(t *testing.T) {
	for _, args := range [][]string{{"website-check"}, {"website-check", "ftp://example.com"}, {"website-check", "https://user:private-value@example.com"}, {"website-check", "example.com", "extra"}} {
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		if code := run(args, stdout, stderr); code != 2 {
			t.Fatalf("%v: %d", args, code)
		}
		if strings.Contains(stderr.String(), "private-value") {
			t.Fatal("credentials leaked")
		}
	}
}
