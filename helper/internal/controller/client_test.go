package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	client := NewClient("http://127.0.0.1:9090")
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.baseURL != "http://127.0.0.1:9090" {
		t.Fatalf("expected baseURL to be http://127.0.0.1:9090, got %s", client.baseURL)
	}
	if client.client == nil {
		t.Fatal("expected http.Client to be initialized")
	}
}

func TestGetConfig(t *testing.T) {
	// Create test server
	config := Config{
		Port:   7890,
		Mode:   "rule",
		LogLevel: "info",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/configs" {
			t.Fatalf("expected /configs, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	result, err := client.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	if result.Port != 7890 {
		t.Fatalf("expected port 7890, got %d", result.Port)
	}
	if result.Mode != "rule" {
		t.Fatalf("expected mode 'rule', got %s", result.Mode)
	}
}

func TestGetConfigError(t *testing.T) {
	// Create server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal error"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetConfig()
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestGetProxies(t *testing.T) {
	proxies := map[string]Proxy{
		"Direct": {
			Name:   "Direct",
			Type:   "Direct",
			Server: "",
			Port:   0,
		},
		"Proxy1": {
			Name:   "Proxy1",
			Type:   "ss",
			Server: "1.2.3.4",
			Port:   8388,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/proxies" {
			t.Fatalf("expected /proxies, got %s", r.URL.Path)
		}

		resp := struct {
			Proxies map[string]Proxy `json:"proxies"`
		}{
			Proxies: proxies,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	result, err := client.GetProxies()
	if err != nil {
		t.Fatalf("GetProxies failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 proxies, got %d", len(result))
	}

	if result["Proxy1"].Server != "1.2.3.4" {
		t.Fatalf("expected server 1.2.3.4, got %s", result["Proxy1"].Server)
	}
}

func TestGetProxyGroups(t *testing.T) {
	// Proxy groups are returned as part of the proxies endpoint
	allProxies := map[string]Proxy{
		"Auto": {
			Name:    "Auto",
			Type:    "url-test",
			Now:     "Proxy1",
			All:     []string{"Proxy1", "Proxy2", "Proxy3"},
		},
		"Manual": {
			Name:    "Manual",
			Type:    "select",
			Now:     "Direct",
			All:     []string{"Direct", "Proxy1", "Proxy2"},
		},
		"Proxy1": {
			Name:   "Proxy1",
			Type:   "ss",
			Server: "1.2.3.4",
			Port:   8388,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxies" {
			t.Fatalf("expected /proxies, got %s", r.URL.Path)
		}

		resp := struct {
			Proxies map[string]Proxy `json:"proxies"`
		}{
			Proxies: allProxies,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	result, err := client.GetProxyGroups()
	if err != nil {
		t.Fatalf("GetProxyGroups failed: %v", err)
	}

	// Should only return groups (url-test, select, etc.), not regular proxies
	if len(result) != 2 {
		t.Fatalf("expected 2 proxy groups, got %d", len(result))
	}

	auto, ok := result["Auto"]
	if !ok {
		t.Fatal("expected 'Auto' group")
	}
	if auto.Now != "Proxy1" {
		t.Fatalf("expected current proxy 'Proxy1', got %s", auto.Now)
	}
	if len(auto.All) != 3 {
		t.Fatalf("expected 3 proxies in group, got %d", len(auto.All))
	}
}

func TestSelectProxy(t *testing.T) {
	var capturedBody map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/proxies/Auto"
		if r.URL.Path != expectedPath {
			t.Fatalf("expected %s, got %s", expectedPath, r.URL.Path)
		}
		if r.Method != "PUT" {
			t.Fatalf("expected PUT, got %s", r.Method)
		}

		// Parse request body
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&capturedBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.SelectProxy("Auto", "Proxy2")
	if err != nil {
		t.Fatalf("SelectProxy failed: %v", err)
	}

	if capturedBody == nil {
		t.Fatal("no request body captured")
	}
	if capturedBody["name"] != "Proxy2" {
		t.Fatalf("expected proxy name 'Proxy2', got %s", capturedBody["name"])
	}
}

func TestSelectProxyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "proxy not found"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.SelectProxy("Auto", "InvalidProxy")
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
}

func TestGetProxiesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetProxies()
	if err == nil {
		t.Fatal("expected error for 503 response")
	}
}

func TestGetProxyGroupsFilterTypes(t *testing.T) {
	// Test that only selector-like types are returned
	allProxies := map[string]Proxy{
		"Auto":      {Name: "Auto", Type: "url-test", Now: "Proxy1", All: []string{"Proxy1"}},
		"Manual":    {Name: "Manual", Type: "select", Now: "Direct", All: []string{"Direct"}},
		"Fallback":  {Name: "Fallback", Type: "fallback", Now: "Proxy1", All: []string{"Proxy1"}},
		"LoadBalance": {Name: "LoadBalance", Type: "load-balance", Now: "Proxy1", All: []string{"Proxy1"}},
		"Relay":     {Name: "Relay", Type: "relay", Now: "Proxy1", All: []string{"Proxy1"}},
		"Proxy1":    {Name: "Proxy1", Type: "ss", Server: "1.2.3.4", Port: 8388},
		"Proxy2":    {Name: "Proxy2", Type: "vmess", Server: "5.6.7.8", Port: 443},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			Proxies map[string]Proxy `json:"proxies"`
		}{
			Proxies: allProxies,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	result, err := client.GetProxyGroups()
	if err != nil {
		t.Fatalf("GetProxyGroups failed: %v", err)
	}

	// Should only return selector types, not regular proxies (ss, vmess, etc.)
	expectedGroups := 5 // Auto, Manual, Fallback, LoadBalance, Relay
	if len(result) != expectedGroups {
		t.Fatalf("expected %d proxy groups, got %d", expectedGroups, len(result))
	}

	// Regular proxies should not be included
	if _, ok := result["Proxy1"]; ok {
		t.Fatal("regular proxy 'Proxy1' should not be in groups")
	}
	if _, ok := result["Proxy2"]; ok {
		t.Fatal("regular proxy 'Proxy2' should not be in groups")
	}
}

func TestClientWithCustomHTTPClient(t *testing.T) {
	customClient := &http.Client{
		Timeout: 0,
	}

	client := &Client{
		baseURL: "http://127.0.0.1:9090",
		client:  customClient,
	}

	if client.client != customClient {
		t.Fatal("expected custom http.Client to be used")
	}
}

func TestGetConfigJSONError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetConfig()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSelectProxyWithSpecialCharacters(t *testing.T) {
	var capturedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.SelectProxy("Auto Group", "Proxy-US-West")
	if err != nil {
		t.Fatalf("SelectProxy failed: %v", err)
	}

	// Go HTTP server auto-decodes URL paths, so we verify the decoded path was received
	// The encoding happens in the request; the server sees it decoded
	expectedPath := "/proxies/Auto Group"
	if capturedPath != expectedPath {
		t.Fatalf("expected path %s, got %s", expectedPath, capturedPath)
	}
}

func TestNewClientWithSecret(t *testing.T) {
	secret := "test-secret-key"
	client := NewClientWithSecret("http://127.0.0.1:9090", secret)

	if client == nil {
		t.Fatal("NewClientWithSecret returned nil")
	}
	if client.secret != secret {
		t.Fatalf("expected secret to be set, got %s", client.secret)
	}
}

func TestClientWithSecretHeader(t *testing.T) {
	var authHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Config{Port: 7890})
	}))
	defer server.Close()

	client := NewClientWithSecret(server.URL, "my-secret")
	_, err := client.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	expectedAuth := "Bearer my-secret"
	if authHeader != expectedAuth {
		t.Fatalf("expected Authorization header %s, got %s", expectedAuth, authHeader)
	}
}

func TestClientRequestError(t *testing.T) {
	// Use an invalid URL that will cause connection errors
	client := NewClient("http://invalid.host.that.does.not.exist:9090")
	_, err := client.GetConfig()
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestGetProxiesEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	result, err := client.GetProxies()
	if err != nil {
		t.Fatalf("GetProxies failed: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 proxies, got %d", len(result))
	}
}

func TestGetProxyGroupsEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"proxies": {}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	result, err := client.GetProxyGroups()
	if err != nil {
		t.Fatalf("GetProxyGroups failed: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(result))
	}
}
