package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// TestResult represents the result of a connection test to a proxy.
type TestResult struct {
	Delay     int    `json:"delay"`
	MeanDelay int    `json:"meanDelay,omitempty"`
	Success   bool   `json:"success"`
	Message   string `json:"message,omitempty"`
}

// defaultTestURL is the default URL used for connection testing.
const defaultTestURL = "http://www.gstatic.com/generate_204"

// TestConnection tests the connection to a proxy by measuring its delay.
// It uses the controller's /proxies/{name}/delay endpoint to test the proxy.
// The timeout parameter specifies the maximum time in milliseconds to wait.
// Returns a TestResult containing the delay and success status.
func (c *Client) TestConnection(proxyName string, timeout int) (*TestResult, error) {
	return c.TestConnectionWithURL(proxyName, timeout, defaultTestURL)
}

// TestConnectionWithURL tests the connection to a proxy using a specific test URL.
// The timeout parameter specifies the maximum time in milliseconds to wait.
// The testURL parameter specifies the URL to use for testing.
// Returns a TestResult containing the delay and success status.
func (c *Client) TestConnectionWithURL(proxyName string, timeout int, testURL string) (*TestResult, error) {
	// URL encode the proxy name to handle special characters
	encodedName := url.PathEscape(proxyName)
	path := "/proxies/" + encodedName + "/delay"

	// Build query parameters
	query := url.Values{}
	query.Set("timeout", strconv.Itoa(timeout))
	query.Set("url", testURL)

	fullPath := path + "?" + query.Encode()

	resp, err := c.get(fullPath)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result TestResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Set success based on delay if not explicitly set
	if !result.Success && result.Delay > 0 {
		result.Success = true
	}

	return &result, nil
}
