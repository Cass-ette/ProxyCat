package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TestResult represents the result of a connection test through a proxy.
type TestResult struct {
	GoogleOK bool   `json:"googleOK"`
	GitHubOK bool   `json:"githubOK"`
	Error    string `json:"error,omitempty"`
}

// TestConnection tests connectivity through a proxy by testing Google and GitHub endpoints.
// The proxyURL should be in the format http://127.0.0.1:7890 (or with credentials).
// Returns a TestResult indicating whether Google and GitHub are accessible through the proxy.
func TestConnection(proxyURL string) (*TestResult, error) {
	// Parse proxy URL
	proxy, err := url.Parse(proxyURL)
	if err != nil {
		return &TestResult{Error: fmt.Sprintf("invalid proxy URL: %v", err)}, nil
	}

	// Create HTTP client with proxy transport
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxy),
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
	return testConnection(client), nil
}

func testConnection(client *http.Client) *TestResult {
	result := &TestResult{}
	var failures []string
	for _, endpoint := range []struct {
		name string
		url  string
		want int
		ok   *bool
	}{
		{"Google", "http://clients3.google.com/generate_204", http.StatusNoContent, &result.GoogleOK},
		{"GitHub", "https://github.com", http.StatusOK, &result.GitHubOK},
	} {
		resp, err := client.Get(endpoint.url)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s request failed: %v", endpoint.name, err))
			continue
		}
		resp.Body.Close()
		*endpoint.ok = resp.StatusCode == endpoint.want
		if !*endpoint.ok {
			failures = append(failures, fmt.Sprintf("%s returned HTTP %d (expected %d)", endpoint.name, resp.StatusCode, endpoint.want))
		}
	}
	result.Error = strings.Join(failures, "; ")
	return result
}
