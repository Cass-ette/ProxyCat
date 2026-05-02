package controller

import (
	"fmt"
	"net/http"
	"net/url"
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
	result := &TestResult{}

	// Parse proxy URL
	proxy, err := url.Parse(proxyURL)
	if err != nil {
		result.Error = fmt.Sprintf("invalid proxy URL: %v", err)
		return result, nil
	}

	// Create HTTP client with proxy transport
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxy),
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	// Test Google endpoint
	googleReq, err := http.NewRequest("GET", "http://clients3.google.com/generate_204", nil)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create Google request: %v", err)
		return result, nil
	}

	googleResp, err := client.Do(googleReq)
	if err != nil {
		result.Error = fmt.Sprintf("Google request failed: %v", err)
	} else {
		googleResp.Body.Close()
		result.GoogleOK = googleResp.StatusCode == http.StatusNoContent
	}

	// Test GitHub endpoint
	githubReq, err := http.NewRequest("GET", "https://github.com", nil)
	if err != nil {
		if result.Error != "" {
			result.Error += "; "
		}
		result.Error += fmt.Sprintf("failed to create GitHub request: %v", err)
		return result, nil
	}

	githubResp, err := client.Do(githubReq)
	if err != nil {
		if result.Error != "" {
			result.Error += "; "
		}
		result.Error += fmt.Sprintf("GitHub request failed: %v", err)
	} else {
		githubResp.Body.Close()
		result.GitHubOK = githubResp.StatusCode == http.StatusOK
	}

	return result, nil
}
