package subscription

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeSelectsLargestValidClashYAMLAcrossUserAgents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("User-Agent") {
		case "clash-verge/v1.7.7":
			_, _ = w.Write([]byte(validClashYAML("Node A", "Node B")))
		case "Clash.Meta/1.18.0":
			_, _ = w.Write([]byte(validClashYAML("Node A")))
		default:
			_, _ = w.Write([]byte("<html>not a subscription</html>"))
		}
	}))
	defer server.Close()

	result := Probe(server.Client(), server.URL)

	if result.Selected == nil {
		t.Fatalf("Selected is nil: %+v", result)
	}
	if result.Selected.UserAgent != "clash-verge/v1.7.7" {
		t.Fatalf("selected UA = %q", result.Selected.UserAgent)
	}
	if result.Selected.Format != "clash-yaml" {
		t.Fatalf("selected format = %q", result.Selected.Format)
	}
	if result.Selected.ProxyCount != 2 {
		t.Fatalf("proxy count = %d, want 2", result.Selected.ProxyCount)
	}
	if string(result.SelectedContent) == "" {
		t.Fatal("SelectedContent is empty")
	}
}

func TestProbeReportsHTMLSubscriptionPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!doctype html><html><body>login</body></html>"))
	}))
	defer server.Close()

	result := Probe(server.Client(), server.URL+"?token=secret")

	if result.Selected != nil {
		t.Fatalf("Selected = %+v, want nil", result.Selected)
	}
	if strings.Contains(result.URL, "secret") {
		t.Fatalf("result URL leaked token: %s", result.URL)
	}
	if !strings.Contains(result.Message, "网页") {
		t.Fatalf("message = %q, want webpage explanation", result.Message)
	}
	if result.SuggestedFix == "" {
		t.Fatal("missing suggested fix")
	}
}

func TestProbeFallsBackToSupportedURIList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("trojan://secret@example.com:443?sni=example.com#Test%20Node\n"))
	}))
	defer server.Close()

	result := Probe(server.Client(), server.URL)

	if result.Selected == nil {
		t.Fatalf("Selected is nil: %+v", result)
	}
	if result.Selected.Format != "plain-uri-list" {
		t.Fatalf("selected format = %q", result.Selected.Format)
	}
	if result.Selected.NodeCount != 1 {
		t.Fatalf("node count = %d, want 1", result.Selected.NodeCount)
	}
}

func TestProbeReportsUnsupportedURIProtocols(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hysteria2://password@example.com:443#HY2\nhy2://password@example.com:443#HY2Alias\n"))
	}))
	defer server.Close()

	result := Probe(server.Client(), server.URL)

	if result.Selected != nil {
		t.Fatalf("Selected = %+v, want nil", result.Selected)
	}
	if !strings.Contains(result.Message, "暂不支持") {
		t.Fatalf("message = %q, want unsupported protocol explanation", result.Message)
	}
	if result.Attempts[0].UnsupportedSchemes["hysteria2"] != 1 {
		t.Fatalf("hysteria2 count = %d", result.Attempts[0].UnsupportedSchemes["hysteria2"])
	}
	if result.Attempts[0].UnsupportedSchemes["hy2"] != 1 {
		t.Fatalf("hy2 count = %d", result.Attempts[0].UnsupportedSchemes["hy2"])
	}
}

func TestProbeCountsOnlySupportedURIProtocolsAsUsable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("trojan://secret@example.com:443?sni=example.com#Trojan\nhysteria2://password@example.com:443#HY2\n"))
	}))
	defer server.Close()

	result := Probe(server.Client(), server.URL)

	if result.Selected == nil {
		t.Fatalf("Selected is nil: %+v", result)
	}
	if result.Selected.NodeCount != 1 {
		t.Fatalf("node count = %d, want 1 supported node", result.Selected.NodeCount)
	}
	if result.Selected.UnsupportedSchemes["hysteria2"] != 1 {
		t.Fatalf("hysteria2 count = %d", result.Selected.UnsupportedSchemes["hysteria2"])
	}
}

func TestProbeReportsEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// return 200 with empty body
	}))
	defer server.Close()

	result := Probe(server.Client(), server.URL)

	if result.Selected != nil {
		t.Fatalf("Selected = %+v, want nil", result.Selected)
	}
	if !strings.Contains(result.Message, "空内容") {
		t.Fatalf("message = %q, want empty content explanation", result.Message)
	}
}

func TestProbeReportsNetworkError(t *testing.T) {
	// Use a closed server to simulate connection refused
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	result := Probe(server.Client(), server.URL)

	if result.Selected != nil {
		t.Fatalf("Selected = %+v, want nil", result.Selected)
	}
	if !strings.Contains(result.Message, "无法导入") {
		t.Fatalf("message = %q, want generic failure", result.Message)
	}
}

func TestProbeHandlesBase64URIList(t *testing.T) {
	plain := "trojan://secret@example.com:443?sni=example.com#Node1\ntrojan://secret@example.com:444?sni=example.com#Node2\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(plain))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(encoded))
	}))
	defer server.Close()

	result := Probe(server.Client(), server.URL)

	if result.Selected == nil {
		t.Fatalf("Selected is nil: %+v", result)
	}
	if result.Selected.Format != "base64-uri-list" {
		t.Fatalf("selected format = %q, want base64-uri-list", result.Selected.Format)
	}
	if result.Selected.NodeCount < 1 {
		t.Fatalf("node count = %d, want >= 1", result.Selected.NodeCount)
	}
}

func validClashYAML(names ...string) string {
	var b strings.Builder
	b.WriteString("proxies:\n")
	for _, name := range names {
		b.WriteString("  - name: ")
		b.WriteString(name)
		b.WriteString("\n    type: trojan\n    server: example.com\n    port: 443\n    password: secret\n")
	}
	b.WriteString("proxy-groups:\n")
	b.WriteString("  - name: Proxy\n    type: select\n    proxies:\n")
	for _, name := range names {
		b.WriteString("      - ")
		b.WriteString(name)
		b.WriteString("\n")
	}
	b.WriteString("rules:\n  - MATCH,Proxy\n")
	return b.String()
}
