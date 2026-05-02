package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTestConnection(t *testing.T) {
	// Expected test result
	expectedResult := TestResult{
		Delay:   150,
		MeanDelay: 145,
		Success: true,
		Message: "",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Fatalf("expected GET, got %s", r.Method)
		}

		// Parse URL to extract proxy name from path
		// Path format: /proxies/{name}/delay
		expectedPathPrefix := "/proxies/"
		expectedPathSuffix := "/delay"
		path := r.URL.Path

		if len(path) <= len(expectedPathPrefix)+len(expectedPathSuffix) {
			t.Fatalf("unexpected path length: %s", path)
		}

		if path[:len(expectedPathPrefix)] != expectedPathPrefix {
			t.Fatalf("expected path to start with %s, got %s", expectedPathPrefix, path)
		}

		if path[len(path)-len(expectedPathSuffix):] != expectedPathSuffix {
			t.Fatalf("expected path to end with %s, got %s", expectedPathSuffix, path)
		}

		// Check query parameters
		timeout := r.URL.Query().Get("timeout")
		if timeout == "" {
			t.Fatal("expected timeout query parameter")
		}

		url := r.URL.Query().Get("url")
		if url != "http://www.gstatic.com/generate_204" {
			t.Fatalf("expected default test URL, got %s", url)
		}

		// Return delay result
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedResult)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	result, err := client.TestConnection("Proxy1", 5000)
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}

	if !result.Success {
		t.Fatal("expected success to be true")
	}
	if result.Delay != 150 {
		t.Fatalf("expected delay 150, got %d", result.Delay)
	}
}

func TestTestConnectionWithCustomURL(t *testing.T) {
	capturedURL := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.Query().Get("url")

		result := TestResult{
			Delay:   100,
			Success: true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	result, err := client.TestConnectionWithURL("Proxy1", 5000, "https://www.google.com")
	if err != nil {
		t.Fatalf("TestConnectionWithURL failed: %v", err)
	}

	if !result.Success {
		t.Fatal("expected success")
	}
	if capturedURL != "https://www.google.com" {
		t.Fatalf("expected URL https://www.google.com, got %s", capturedURL)
	}
}

func TestTestConnectionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := TestResult{
			Delay:   0,
			Success: false,
			Message: "connection timeout",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	result, err := client.TestConnection("Proxy1", 5000)
	if err != nil {
		t.Fatalf("TestConnection failed with error: %v", err)
	}

	if result.Success {
		t.Fatal("expected success to be false")
	}
	if result.Message != "connection timeout" {
		t.Fatalf("expected message 'connection timeout', got %s", result.Message)
	}
}

func TestTestConnectionErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "proxy not found"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.TestConnection("NonExistentProxy", 5000)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestTestConnectionWithSpecialCharacters(t *testing.T) {
	var capturedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path

		result := TestResult{Delay: 50, Success: true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.TestConnection("Auto Group", 5000)
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}

	// URL encoding for "Auto Group" -> "Auto%20Group"
	// But httptest server auto-decodes it
	if capturedPath != "/proxies/Auto Group/delay" {
		t.Fatalf("expected path /proxies/Auto Group/delay, got %s", capturedPath)
	}
}

func TestTestConnectionServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.TestConnection("Proxy1", 5000)
	if err == nil {
		t.Fatal("expected error for 503 response")
	}
}

func TestTestConnectionJSONError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.TestConnection("Proxy1", 5000)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestTestConnectionRequestError(t *testing.T) {
	// Use an invalid URL that will cause connection errors
	client := NewClient("http://invalid.host.that.does.not.exist:9090")
	_, err := client.TestConnection("Proxy1", 5000)
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestTestConnectionWithSecret(t *testing.T) {
	var authHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")

		result := TestResult{Delay: 75, Success: true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	client := NewClientWithSecret(server.URL, "my-secret")
	_, err := client.TestConnection("Proxy1", 5000)
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}

	expectedAuth := "Bearer my-secret"
	if authHeader != expectedAuth {
		t.Fatalf("expected Authorization header %s, got %s", expectedAuth, authHeader)
	}
}
