// Package controller provides a client for the Mihomo/Clash controller API.
package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "http://127.0.0.1:9090"

// Client provides methods to interact with the Mihomo controller API.
type Client struct {
	baseURL string
	secret  string
	client  *http.Client
}

// Config represents the Mihomo configuration.
type Config struct {
	Port        int               `json:"port"`
	SocksPort   int               `json:"socks-port"`
	RedirPort   int               `json:"redir-port"`
	MixedPort   int               `json:"mixed-port"`
	Mode        string            `json:"mode"`
	LogLevel    string            `json:"log-level"`
	AllowLan    bool              `json:"allow-lan"`
	BindAddress string            `json:"bind-address"`
	IPv6        bool              `json:"ipv6"`
	ExternalUI  string            `json:"external-ui"`
	Secret      string            `json:"secret"`
	Tun         map[string]interface{} `json:"tun,omitempty"`
	DNS         map[string]interface{}   `json:"dns,omitempty"`
}

// Proxy represents a proxy or proxy group.
type Proxy struct {
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	Server     string                 `json:"server,omitempty"`
	Port       int                    `json:"port,omitempty"`
	Now        string                 `json:"now,omitempty"`
	All        []string               `json:"all,omitempty"`
	History    []ProxyHistory         `json:"history,omitempty"`
	Extra      map[string]interface{} `json:"-,omitempty"`
}

// ProxyHistory represents the latency history of a proxy.
type ProxyHistory struct {
	Time    string  `json:"time"`
	Delay   int     `json:"delay"`
	MeanDelay int   `json:"meanDelay,omitempty"`
}

// proxiesResponse is the wrapper for the /proxies endpoint response.
type proxiesResponse struct {
	Proxies map[string]Proxy `json:"proxies"`
}

// selectProxyRequest is the request body for selecting a proxy.
type selectProxyRequest struct {
	Name string `json:"name"`
}

// NewClient creates a new controller client with the given base URL.
// The base URL should include the protocol and host (e.g., "http://127.0.0.1:9090").
// If baseURL is empty, it defaults to http://127.0.0.1:9090.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewClientWithSecret creates a new controller client with authentication secret.
func NewClientWithSecret(baseURL, secret string) *Client {
	return &Client{
		baseURL: baseURL,
		secret:  secret,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetClient allows replacing the underlying HTTP client.
func (c *Client) SetClient(client *http.Client) {
	c.client = client
}

// get performs an HTTP GET request and returns the response body.
func (c *Client) get(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// GetConfig retrieves the current configuration from the controller.
func (c *Client) GetConfig() (*Config, error) {
	resp, err := c.get("/configs")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var config Config
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &config, nil
}

// GetProxies retrieves all proxies from the controller.
func (c *Client) GetProxies() (map[string]Proxy, error) {
	resp, err := c.get("/proxies")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result proxiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Proxies, nil
}

// isProxyGroupType returns true if the proxy type is a group type (selectable).
func isProxyGroupType(proxyType string) bool {
	switch proxyType {
	case "select", "url-test", "fallback", "load-balance", "relay":
		return true
	default:
		return false
	}
}

// GetProxyGroups retrieves all proxy groups from the controller.
// Proxy groups are proxies with types: select, url-test, fallback, load-balance, or relay.
func (c *Client) GetProxyGroups() (map[string]Proxy, error) {
	proxies, err := c.GetProxies()
	if err != nil {
		return nil, err
	}

	groups := make(map[string]Proxy)
	for name, proxy := range proxies {
		if isProxyGroupType(proxy.Type) {
			groups[name] = proxy
		}
	}

	return groups, nil
}

// SelectProxy selects a proxy for a given proxy group.
func (c *Client) SelectProxy(groupName, proxyName string) error {
	// URL encode the group name to handle special characters
	encodedGroup := url.PathEscape(groupName)
	path := "/proxies/" + encodedGroup

	reqBody := selectProxyRequest{Name: proxyName}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("PUT", c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
