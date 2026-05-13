package selfupdate

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSHA256Sidecar(t *testing.T) {
	got, err := parseSHA256Sidecar([]byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  ProxyCat-0.2.0-installer.zip\n"))
	if err != nil {
		t.Fatalf("parseSHA256Sidecar returned error: %v", err)
	}
	if got != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("hash = %q", got)
	}
}

func TestParseSHA256SidecarRejectsInvalidHash(t *testing.T) {
	if _, err := parseSHA256Sidecar([]byte("not-a-hash file.zip")); err == nil {
		t.Fatal("expected error")
	}
}

func TestDownloadFileWritesContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "file.zip")
	if err := downloadFile(server.Client(), server.URL, dest, nil); err != nil {
		t.Fatalf("downloadFile returned error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("content = %q", got)
	}
}

func TestDownloadFileReportsHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer server.Close()

	err := downloadFile(server.Client(), server.URL, filepath.Join(t.TempDir(), "file.zip"), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("error = %q, want HTTP status", err.Error())
	}
}

func TestDownloadFileReportsNetworkError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("lookup github.com: no such host")
	})}

	err := downloadFile(client, "https://github.com/Cass-ette/ProxyCat/releases/download/v0.6.0/ProxyCat.zip", filepath.Join(t.TempDir(), "file.zip"), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "lookup github.com") {
		t.Fatalf("error = %q, want network detail", err.Error())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
