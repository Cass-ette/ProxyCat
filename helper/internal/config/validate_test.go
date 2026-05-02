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
