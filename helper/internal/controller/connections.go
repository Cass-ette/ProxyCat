package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// EnableConnectionProcessLookup enables attribution for new connections without
// restarting the core or changing traffic capture, routing, DNS or listeners.
func (c *Client) EnableConnectionProcessLookup(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+"/configs", bytes.NewBufferString(`{"find-process-mode":"always"}`))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("process lookup: HTTP %d", resp.StatusCode)
	}
	return nil
}

// ConnectionSnapshot contains observed active connections, never URL paths,
// request headers or absolute process paths. It does not prove reachability.
type ConnectionSnapshot struct {
	ObservedAt  string             `json:"observedAt"`
	Connections []ConnectionRecord `json:"connections"`
}

type ConnectionRecord struct {
	ID              string   `json:"id"`
	App             string   `json:"app"`
	Host            string   `json:"host"`
	DestinationPort string   `json:"destinationPort"`
	Network         string   `json:"network"`
	Inbound         string   `json:"inbound"`
	Route           string   `json:"route"`
	Rule            string   `json:"rule"`
	RulePayload     string   `json:"rulePayload"`
	Chains          []string `json:"chains"`
	Node            string   `json:"node"`
	StartedAt       string   `json:"startedAt"`
}

// GetConnections only reads the controller. It never changes capture or mode.
func (c *Client) GetConnections(ctx context.Context) (*ConnectionSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	connections, err := c.connections(ctx)
	if err != nil {
		return nil, err
	}
	populateConnectionApps(ctx, connections)
	result := &ConnectionSnapshot{ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Connections: []ConnectionRecord{}}
	for _, conn := range connections {
		id := conn.ID
		if id == "" {
			// Compatibility with controllers without IDs: retain socket identity,
			// not just host, so simultaneous requests remain separate records.
			id = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s",
				conn.Metadata.Network, conn.Metadata.SourceIP, conn.Metadata.SourcePort,
				conn.Metadata.DestinationIP, conn.Metadata.DestinationPort, conn.Metadata.Host, conn.Start))))
		}
		host := conn.Metadata.Host
		if host == "" {
			host = conn.Metadata.DestinationIP
		}
		route := &WebsiteResult{Route: "unknown"}
		applyObservation(route, &conn)
		chains := append([]string{}, conn.Chains...)
		result.Connections = append(result.Connections, ConnectionRecord{
			ID: id, App: connectionApp(conn.Metadata.ProcessPath, conn.Metadata.Process),
			Host: host, DestinationPort: string(conn.Metadata.DestinationPort),
			Network: conn.Metadata.Network, Inbound: conn.Metadata.Type,
			Route: route.Route, Rule: conn.Rule, RulePayload: conn.RulePayload,
			Chains: chains, Node: route.Node, StartedAt: conn.Start,
		})
	}
	return result, nil
}

func connectionApp(path, process string) string {
	// Helpers within the same macOS bundle belong to the parent application.
	for _, component := range strings.Split(path, "/") {
		if strings.HasSuffix(strings.ToLower(component), ".app") {
			return component[:len(component)-4]
		}
	}
	if process != "" {
		return filepath.Base(process)
	}
	if path != "" {
		return filepath.Base(path)
	}
	return ""
}
