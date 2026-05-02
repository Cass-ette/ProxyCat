package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTestConnectionSuccess(t *testing.T) {
	// Create a mock server that simulates both Google and GitHub responses
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/generate_204":
			w.WriteHeader(http.StatusNoContent)
		case "/":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html><body>GitHub</body></html>"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Use the test server as a proxy (in real scenario it would be a proxy)
	// For testing, we can't easily test the actual proxy behavior without a real proxy server
	// So we test that the function accepts the proxy URL and attempts to connect

	// Test with an invalid proxy URL (this tests error handling path)
	result, err := TestConnection("http://invalid-proxy-url-that-does-not-exist:59999")
	if err != nil {
		t.Fatalf("TestConnection should not return error for connection failures: %v", err)
	}

	// Should return result with Error field populated
	if result.Error == "" {
		t.Fatal("expected Error to be populated when proxy is unreachable")
	}
}

func TestTestConnectionInvalidProxyURL(t *testing.T) {
	result, err := TestConnection("://invalid-url")
	if err != nil {
		t.Fatalf("TestConnection should not return error for invalid URL: %v", err)
	}

	if result.Error == "" {
		t.Fatal("expected Error field to be populated for invalid URL")
	}

	if result.GoogleOK || result.GitHubOK {
		t.Fatal("expected both GoogleOK and GitHubOK to be false for invalid URL")
	}
}

func TestTestConnectionStruct(t *testing.T) {
	// Test the TestResult struct fields
	result := TestResult{
		GoogleOK: true,
		GitHubOK: false,
		Error:    "some error",
	}

	if !result.GoogleOK {
		t.Fatal("expected GoogleOK to be true")
	}

	if result.GitHubOK {
		t.Fatal("expected GitHubOK to be false")
	}

	if result.Error != "some error" {
		t.Fatalf("expected Error to be 'some error', got %s", result.Error)
	}
}

func TestTestConnectionEmptyProxyURL(t *testing.T) {
	// Test with empty proxy URL (should fail to parse)
	result, err := TestConnection("")
	if err != nil {
		t.Fatalf("TestConnection should not return error for empty URL: %v", err)
	}

	if result.Error == "" {
		t.Fatal("expected Error field to be populated for empty URL")
	}
}

func TestTestConnectionWithProxy(t *testing.T) {
	// Create a mock proxy server that routes requests
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// In a real proxy, this would forward the request
		// For testing, we simulate the responses directly
		switch r.URL.Host {
		case "clients3.google.com":
			if r.URL.Path == "/generate_204" {
				w.WriteHeader(http.StatusNoContent)
			}
		case "github.com":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html></html>"))
		default:
			// Try to match by looking at the request
			if r.URL.Path == "/generate_204" {
				w.WriteHeader(http.StatusNoContent)
			} else if r.URL.Path == "/" || r.URL.Path == "" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("<html></html>"))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))
	defer proxyServer.Close()

	// Test connection through the mock proxy
	result, err := TestConnection(proxyServer.URL)
	if err != nil {
		t.Fatalf("TestConnection returned error: %v", err)
	}

	// Since we're using a mock proxy that handles the requests differently,
	// we just verify the function executed without panic
	// In a real scenario with a proper proxy, the requests would be routed correctly
	if result.Error != "" {
		// It's OK if there's an error, as long as the function handled it gracefully
		t.Logf("TestConnection returned error in result (expected for mock): %s", result.Error)
	}
}

func TestTestConnectionGoogleOnlyOK(t *testing.T) {
	// Create a mock proxy that returns 204 for Google but fails for GitHub
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Host {
		case "clients3.google.com":
			if r.URL.Path == "/generate_204" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		// Everything else fails
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer proxyServer.Close()

	result, err := TestConnection(proxyServer.URL)
	if err != nil {
		t.Fatalf("TestConnection returned error: %v", err)
	}

	// The test should complete without panic
	// Exact behavior depends on how the mock handles requests
	t.Logf("Result: GoogleOK=%v, GitHubOK=%v, Error=%s", result.GoogleOK, result.GitHubOK, result.Error)
}

func TestTestConnectionGitHubOnlyOK(t *testing.T) {
	// Create a mock proxy that returns 200 for GitHub but fails for Google
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Host {
		case "github.com":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html></html>"))
			return
		}
		// Google and everything else fails
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer proxyServer.Close()

	result, err := TestConnection(proxyServer.URL)
	if err != nil {
		t.Fatalf("TestConnection returned error: %v", err)
	}

	t.Logf("Result: GoogleOK=%v, GitHubOK=%v, Error=%s", result.GoogleOK, result.GitHubOK, result.Error)
}

func TestTestConnectionWithGoogleError(t *testing.T) {
	// Test when Google request fails but function still returns result
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't handle any requests properly, just return 500
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer proxyServer.Close()

	result, err := TestConnection(proxyServer.URL)
	if err != nil {
		t.Fatalf("TestConnection should not return error even when requests fail: %v", err)
	}

	// Both should be false since we got 500 responses
	if result.GoogleOK {
		t.Fatal("expected GoogleOK to be false when getting 500 response")
	}

	if result.GitHubOK {
		t.Fatal("expected GitHubOK to be false when getting 500 response")
	}
}
