package controller

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// WebsiteResult describes exactly one GET through the configured local HTTP
// proxy. Route fields are only populated from a connection owning our socket.
type WebsiteResult struct {
	Host        string   `json:"host"`
	Scheme      string   `json:"scheme"`
	Outcome     string   `json:"outcome"`
	StatusCode  int      `json:"statusCode"`
	ElapsedMS   int64    `json:"elapsedMS"`
	Route       string   `json:"route"`
	Rule        string   `json:"rule"`
	RulePayload string   `json:"rulePayload"`
	Chains      []string `json:"chains"`
	Node        string   `json:"node"`
	Group       string   `json:"group"`
	Mode        string   `json:"mode"`
	ObservedAt  string   `json:"observedAt"`
}

type connectionPort string

func (p *connectionPort) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*p = connectionPort(s)
		return nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*p = connectionPort(strconv.Itoa(n))
	return nil
}

type trackedConnection struct {
	ID       string `json:"id"`
	Start    string `json:"start"`
	Metadata struct {
		SourceIP        string         `json:"sourceIP"`
		SourcePort      connectionPort `json:"sourcePort"`
		DestinationPort connectionPort `json:"destinationPort"`
		Host            string         `json:"host"`
		DestinationIP   string         `json:"destinationIP"`
		Process         string         `json:"process"`
		ProcessPath     string         `json:"processPath"`
		Network         string         `json:"network"`
		Type            string         `json:"type"`
	} `json:"metadata"`
	Chains      []string `json:"chains"`
	Rule        string   `json:"rule"`
	RulePayload string   `json:"rulePayload"`
}

func (c *Client) connections(ctx context.Context) ([]trackedConnection, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/connections", nil)
	if err != nil {
		return nil, err
	}
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("connections: HTTP %d", resp.StatusCode)
	}
	var body struct {
		Connections []trackedConnection `json:"connections"`
	}
	err = json.NewDecoder(resp.Body).Decode(&body)
	return body.Connections, err
}

func websiteURL(input string) (*url.URL, error) {
	input = strings.TrimSpace(input)
	if !strings.Contains(input, "://") {
		input = "https://" + input
	}
	u, err := url.Parse(input)
	if err != nil || u == nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return nil, errors.New("请输入 HTTP 或 HTTPS 网址，不含用户名和密码")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, errors.New("网址端口无效")
		}
	}
	u.Fragment = ""
	return u, nil
}

func (c *Client) CheckWebsite(ctx context.Context, input string) (*WebsiteResult, error) {
	u, err := websiteURL(input)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	started := time.Now()
	result := &WebsiteResult{Host: u.Host, Scheme: u.Scheme, Route: "unknown", Chains: []string{}, ObservedAt: started.UTC().Format(time.RFC3339)}
	defer func() { result.ElapsedMS = time.Since(started).Milliseconds() }()
	configCtx, configCancel := context.WithTimeout(ctx, 2*time.Second)
	req, _ := http.NewRequestWithContext(configCtx, http.MethodGet, c.baseURL+"/configs", nil)
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	resp, configErr := c.client.Do(req)
	var cfg Config
	if configErr == nil {
		if resp.StatusCode != http.StatusOK {
			configErr = errors.New("controller unavailable")
		} else {
			configErr = json.NewDecoder(resp.Body).Decode(&cfg)
		}
		resp.Body.Close()
	}
	configCancel()
	if configErr != nil {
		result.Outcome = "engineUnavailable"
		return result, nil
	}
	result.Mode = cfg.Mode
	port := cfg.MixedPort
	if port == 0 {
		port = cfg.Port
	}
	if port < 1 || port > 65535 {
		result.Outcome = "proxyUnavailable"
		return result, nil
	}
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		result.Outcome = "proxyUnavailable"
		return result, nil
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	conn.SetDeadline(deadline)
	// Cancel closes the socket even during TLS or response-header reads.
	stopClose := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopClose()
	sourcePort := strconv.Itoa(conn.LocalAddr().(*net.TCPAddr).Port)
	destinationPort := u.Port()
	if destinationPort == "" {
		if u.Scheme == "https" {
			destinationPort = "443"
		} else {
			destinationPort = "80"
		}
	}
	observations := make(chan *trackedConnection, 1)
	observeCtx, stopObserving := context.WithCancel(ctx)
	defer stopObserving()
	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			pollCtx, done := context.WithTimeout(observeCtx, 400*time.Millisecond)
			connections, _ := c.connections(pollCtx)
			done()
			for _, candidate := range connections {
				ip := net.ParseIP(candidate.Metadata.SourceIP)
				if string(candidate.Metadata.SourcePort) == sourcePort && string(candidate.Metadata.DestinationPort) == destinationPort && ip != nil && ip.IsLoopback() &&
					(candidate.Metadata.Host == "" || strings.EqualFold(strings.TrimSuffix(candidate.Metadata.Host, "."), strings.TrimSuffix(u.Hostname(), "."))) {
					observations <- &candidate
					return
				}
			}
			select {
			case <-observeCtx.Done():
				observations <- nil
				return
			case <-ticker.C:
			}
		}
	}()

	result.Outcome, result.StatusCode = requestWebsite(ctx, conn, u, func() {
		// CONNECT keeps the exact route alive before a fast page can finish.
		select {
		case observation := <-observations:
			applyObservation(result, observation)
		case <-time.After(650 * time.Millisecond):
		case <-ctx.Done():
		}
	})
	// Keep our socket open briefly so a fast successful response can still be
	// observed. Never substitute another request to the same host or a rule guess.
	if result.Route == "unknown" {
		select {
		case observation := <-observations:
			applyObservation(result, observation)
		case <-time.After(200 * time.Millisecond):
		case <-ctx.Done():
		}
	}
	stopObserving()
	return result, nil
}

func requestWebsite(ctx context.Context, conn net.Conn, u *url.URL, onTunnel func()) (string, int) {
	var stream net.Conn = conn
	{
		authority := u.Host
		if u.Port() == "" {
			port := "80"
			if u.Scheme == "https" {
				port = "443"
			}
			authority = net.JoinHostPort(u.Hostname(), port)
		}
		connect := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: authority}, Host: authority, Header: make(http.Header)}
		if err := connect.Write(conn); err != nil {
			return websiteError(err, "connection"), 0
		}
		response, err := http.ReadResponse(bufio.NewReader(io.LimitReader(conn, 64<<10)), connect)
		if err != nil {
			return websiteError(err, "connection"), 0
		}
		if response.StatusCode != http.StatusOK {
			return "tunnelFailed", 0
		}
		if onTunnel != nil {
			onTunnel()
		}
	}
	if u.Scheme == "https" {
		secure := tls.Client(conn, &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
		if err := secure.HandshakeContext(ctx); err != nil {
			return websiteError(err, "tls"), 0
		}
		stream = secure
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	request.Header.Set("User-Agent", "ProxyCat-WebsiteCheck/1.0")
	if err := request.Write(stream); err != nil {
		return websiteError(err, "connection"), 0
	}
	response, err := http.ReadResponse(bufio.NewReader(io.LimitReader(stream, 1<<20)), request)
	if err != nil {
		return websiteError(err, "connection"), 0
	}
	// Do not follow redirects or download the body: one URL, one GET, no cookies.
	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return "responded", response.StatusCode
	case response.StatusCode >= 300 && response.StatusCode < 400:
		return "redirect", response.StatusCode
	case response.StatusCode == 401 || response.StatusCode == 403 || response.StatusCode == 429:
		return "denied", response.StatusCode
	default:
		return "httpError", response.StatusCode
	}
}

func websiteError(err error, stage string) string {
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		return "timeout"
	}
	var certificate *tls.CertificateVerificationError
	var invalid x509.CertificateInvalidError
	if errors.As(err, &certificate) || errors.As(err, &invalid) {
		return "certificate"
	}
	if stage == "tls" {
		return "tlsFailed"
	}
	return "connectionFailed"
}

func applyObservation(result *WebsiteResult, conn *trackedConnection) {
	if conn == nil {
		return
	}
	result.Rule, result.RulePayload, result.Chains = conn.Rule, conn.RulePayload, conn.Chains
	if len(conn.Chains) == 0 {
		return
	}
	// Mihomo serializes chains from the outbound leaf towards enclosing groups.
	result.Node = conn.Chains[0]
	if len(conn.Chains) > 1 {
		result.Group = conn.Chains[len(conn.Chains)-1]
	}
	switch strings.ToUpper(result.Node) {
	case "DIRECT":
		result.Route = "direct"
	case "REJECT", "REJECT-DROP":
		result.Route = "reject"
	case "PASS", "COMPATIBLE":
		result.Route = "unknown"
	default:
		result.Route = "proxy"
	}
}
