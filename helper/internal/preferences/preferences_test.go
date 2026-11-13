package preferences

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsInvalidOrDuplicateStoredPreferences(t *testing.T) {
	for name, content := range map[string]string{
		"invalid route":  `{"preferences":[{"host":"example.com","route":"automatic"}]}`,
		"invalid host":   `{"preferences":[{"host":"bad_domain.com","route":"proxy"}]}`,
		"duplicate host": `{"preferences":[{"host":"EXAMPLE.com.","route":"proxy"},{"host":"example.com","route":"direct"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "preferences.json")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("Load accepted invalid stored preferences")
			}
		})
	}
}

func TestLoadNormalizesStoredHosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	if err := os.WriteFile(path, []byte(`{"preferences":[{"host":"EXAMPLE.com.","route":"proxy"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Load(path)
	if err != nil || len(store.Preferences) != 1 || store.Preferences[0].Host != "example.com" {
		t.Fatalf("Load = %+v, %v", store, err)
	}
}

func TestNormalizeHost(t *testing.T) {
	for raw, want := range map[string]string{"HTTPS://Example.COM/path": "example.com", "1.2.3.4": "1.2.3.4", "[2001:db8::1]": "2001:db8::1", "2001:db8::2": "2001:db8::2"} {
		got, err := NormalizeHost(raw)
		if err != nil || got != want {
			t.Fatalf("NormalizeHost(%q) = %q, %v", raw, got, err)
		}
	}
}

func TestNormalizeHostRejectsInvalidDomain(t *testing.T) {
	for _, raw := range []string{"bad host.com", "bad_domain.com", "-bad.example", "https://user:pass@example.com"} {
		if _, err := NormalizeHost(raw); err == nil {
			t.Errorf("NormalizeHost(%q) accepted invalid input", raw)
		}
	}
}

func TestApplyToYAMLPrependsPreferences(t *testing.T) {
	store := Store{Preferences: []Preference{{Host: "example.com", Route: "proxy"}, {Host: "baidu.com", Route: "direct"}}}
	content := []byte("proxy-groups:\n  - name: Proxy\n    type: select\n    proxies: [DIRECT]\nrules:\n  - MATCH,Proxy\n")
	got, err := ApplyToYAML(content, store)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "DOMAIN-SUFFIX,example.com,Proxy") || !strings.Contains(got, "DOMAIN-SUFFIX,baidu.com,DIRECT") {
		t.Fatalf("missing preference rules: %s", got)
	}
	if strings.Index(got, "DOMAIN-SUFFIX,example.com,Proxy") > strings.Index(got, "MATCH,Proxy") {
		t.Fatal("preference rule was not prepended")
	}
}

func TestApplyToYAMLRejectsMissingProxyGroup(t *testing.T) {
	_, err := ApplyToYAML([]byte("rules: [MATCH,DIRECT]\n"), Store{Preferences: []Preference{{Host: "example.com", Route: "proxy"}}})
	if err == nil {
		t.Fatal("expected missing proxy group error")
	}
}

func TestRebuildYAMLRemovesOldRouteBeforeAddingNewPreference(t *testing.T) {
	base := []byte("proxy-groups:\n  - name: Proxy\n    type: select\n    proxies: [DIRECT]\nrules:\n  - MATCH,Proxy\n")
	previous := Store{Preferences: []Preference{{Host: "example.com", Route: "direct"}}}
	effective, err := ApplyToYAML(base, previous)
	if err != nil {
		t.Fatal(err)
	}
	next := Store{Preferences: []Preference{{Host: "example.com", Route: "proxy"}}}
	rebuilt, err := RebuildYAML([]byte(effective), previous, next)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rebuilt, "DOMAIN-SUFFIX,example.com,Proxy") || strings.Contains(rebuilt, "DOMAIN-SUFFIX,example.com,DIRECT") {
		t.Fatalf("old route remained after preference change: %s", rebuilt)
	}
}

func TestApplyToYAMLUsesExactHostMatchForIP(t *testing.T) {
	for host, expected := range map[string]string{"192.0.2.8": "IP-CIDR,192.0.2.8/32,DIRECT", "2001:db8::1": "IP-CIDR6,2001:db8::1/128,DIRECT"} {
		text, err := ApplyToYAML([]byte("rules: [MATCH,DIRECT]\n"), Store{Preferences: []Preference{{Host: host, Route: "direct"}}})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, expected) {
			t.Fatalf("missing exact IP route %q in %s", expected, text)
		}
	}
}

func TestChildDomainPreferencePrecedesParentWithoutChangingStore(t *testing.T) {
	store := Store{Preferences: []Preference{{Host: "example.com", Route: "direct"}, {Host: "app.example.com", Route: "proxy"}}}
	text, err := ApplyToYAML([]byte("proxy-groups: [{name: Proxy, type: select, proxies: [DIRECT]}]\nrules: [\"MATCH,DIRECT\"]\n"), store)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(text, "DOMAIN-SUFFIX,app.example.com,Proxy") > strings.Index(text, "DOMAIN-SUFFIX,example.com,DIRECT") || store.Preferences[0].Host != "example.com" {
		t.Fatalf("child preference was shadowed, or persistence order mutated: %s %+v", text, store)
	}
}
