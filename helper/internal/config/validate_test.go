package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateWithMihomoUsesPrivateTemporaryConfigAndRemovesIt(t *testing.T) {
	dir := t.TempDir()
	dataDirectory := filepath.Join(dir, "mihomo-data")
	binary := filepath.Join(dir, "mihomo")
	script := "#!/bin/sh\n[ \"$1\" = \"-t\" ] || exit 31\n[ \"$2\" = \"-d\" ] || exit 32\n[ \"$3\" = \"" + dataDirectory + "\" ] || exit 33\n[ \"$4\" = \"-f\" ] || exit 34\n[ -s \"$5\" ] || exit 35\ngrep -q 'candidate-node' \"$5\" || exit 36\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("proxies: [{name: candidate-node, type: direct}]\nproxy-groups: [{name: Proxy, type: select, proxies: [candidate-node]}]\nrules: [\"MATCH,Proxy\"]\n")
	if err := ValidateWithMihomo(binary, dataDirectory, content); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary validation config was not removed: %v", entries)
	}
}

func TestValidateWithMihomoRejectsRuntimeConfigAndRedactsURLs(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "mihomo")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho 'failed URL https://user:pass@example.com/sub?token=secret' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("proxies: [{name: node, type: direct}]\nproxy-groups: [{name: Proxy, type: select, proxies: [node]}]\nrules: [\"MATCH,Proxy\"]\n")
	err := ValidateWithMihomo(binary, filepath.Join(dir, "data"), content)
	if err == nil || !strings.Contains(err.Error(), "配置校验失败") {
		t.Fatalf("expected engine validation failure, got %v", err)
	}
	for _, secret := range []string{"user:pass", "token=secret"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("validation error leaked %q: %v", secret, err)
		}
	}
}

func TestValidateWithMihomoRejectsStructurallyInvalidConfigBeforeExec(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "should-not-run")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\ntouch '"+filepath.Join(dir, "ran")+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	err := ValidateWithMihomo(binary, filepath.Join(dir, "data"), []byte("rules: [\"MATCH,Proxy\"]\n"))
	if err == nil {
		t.Fatal("expected structural validation error")
	}
	if _, err := os.Stat(filepath.Join(dir, "ran")); !os.IsNotExist(err) {
		t.Fatalf("engine ran before structural validation: %v", err)
	}
}

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
