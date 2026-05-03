package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestConvertPlainURIListToMihomoYAML(t *testing.T) {
	content := []byte("trojan://secret@example.com:443?sni=example.com#HK%201\n")

	yamlText, err := ConvertURIToYAML(content, FormatPlainList)
	if err != nil {
		t.Fatalf("ConvertURIToYAML returned error: %v", err)
	}

	result := Validate([]byte(yamlText))
	if !result.Valid {
		t.Fatalf("generated config invalid: %s\n%s", result.Message, yamlText)
	}
	if !strings.Contains(yamlText, "mixed-port: 7890") {
		t.Fatalf("generated config missing mixed-port: %s", yamlText)
	}
	if !strings.Contains(yamlText, "external-controller: 127.0.0.1:9090") {
		t.Fatalf("generated config missing external-controller: %s", yamlText)
	}
	if !strings.Contains(yamlText, "HK 1") {
		t.Fatalf("generated config missing decoded node name: %s", yamlText)
	}
	if strings.Contains(yamlText, "trojan://") {
		t.Fatalf("generated config leaked raw node URI: %s", yamlText)
	}
}

func TestConvertBase64URIListToMihomoYAML(t *testing.T) {
	raw := "vless://00000000-0000-0000-0000-000000000000@example.com:443?encryption=none&security=tls&type=ws&host=example.com&path=%2Fws#US%201\n"
	content := []byte(base64.StdEncoding.EncodeToString([]byte(raw)))

	yamlText, err := ConvertURIToYAML(content, FormatBase64List)
	if err != nil {
		t.Fatalf("ConvertURIToYAML returned error: %v", err)
	}

	result := Validate([]byte(yamlText))
	if !result.Valid {
		t.Fatalf("generated config invalid: %s\n%s", result.Message, yamlText)
	}
	if !strings.Contains(yamlText, "US 1") {
		t.Fatalf("generated config missing decoded node name: %s", yamlText)
	}
	if !strings.Contains(yamlText, "ws-opts") {
		t.Fatalf("generated config missing websocket options: %s", yamlText)
	}
}

func TestNormalizeClashYAMLFillsProxyCatPorts(t *testing.T) {
	content := []byte(`proxies:
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
`)

	yamlText, err := NormalizeClashYAML(content)
	if err != nil {
		t.Fatalf("NormalizeClashYAML returned error: %v", err)
	}

	if !strings.Contains(yamlText, "mixed-port: 7890") {
		t.Fatalf("normalized config missing mixed-port: %s", yamlText)
	}
	if !strings.Contains(yamlText, "external-controller: 127.0.0.1:9090") {
		t.Fatalf("normalized config missing external-controller: %s", yamlText)
	}
}
