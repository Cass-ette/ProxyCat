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

func TestURLRedactsRawNodeURIs(t *testing.T) {
	input := "vmess://eyJ2IjoiMiIsInBzIjoibm9kZSIsImlkIjoiMTIzZS00NTYifQ=="
	if got := URL(input); got != "<redacted-node-uri>" {
		t.Fatalf("URL(%q) = %q, want <redacted-node-uri>", input, got)
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

func TestStringRedactsMixedCaseHTTPURL(t *testing.T) {
	input := "download HTTPS://user:secret@example.com/sub?token=abc123 now"
	got := String(input)

	if strings.Contains(got, "user") || strings.Contains(got, "secret") || strings.Contains(got, "abc123") {
		t.Fatalf("redacted text leaked secret: %s", got)
	}
}

func TestStringKeepsNonSecretText(t *testing.T) {
	input := "system proxy is off"
	if got := String(input); got != input {
		t.Fatalf("String(%q) = %q", input, got)
	}
}
