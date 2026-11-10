package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// SetMixedPort retries listener creation after a startup bind failure. Mihomo
// can keep its controller alive while setting a failed mixed listener to zero.
// Patch only this field; never reload routing, DNS, TUN or node selections.
func (c *Client) SetMixedPort(ctx context.Context, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid mixed port")
	}
	body, _ := json.Marshal(map[string]int{"mixed-port": port})
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+"/configs", bytes.NewReader(body))
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
		return fmt.Errorf("restore mixed listener: HTTP %d", resp.StatusCode)
	}
	return nil
}
