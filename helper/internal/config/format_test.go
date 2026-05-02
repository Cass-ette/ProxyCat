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
