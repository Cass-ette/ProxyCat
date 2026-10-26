package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Mihomo's GLOBAL selector starts at DIRECT independently of the rule groups.
// Select a proxy before changing modes so switching to global does not silently
// send all traffic directly. Keep an existing non-default global selection.
func (c *Client) prepareGlobalProxy() error {
	proxies, err := c.GetProxies()
	if err != nil {
		return fmt.Errorf("read global proxy: %w", err)
	}
	global, ok := proxies["GLOBAL"]
	if !ok {
		return fmt.Errorf("GLOBAL group is unavailable; mode unchanged")
	}
	if global.Now != "" && global.Now != "DIRECT" {
		return nil
	}

	resp, err := c.get("/rules")
	if err != nil {
		return fmt.Errorf("read rule selection: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("read rule selection: unexpected status code: %d", resp.StatusCode)
	}
	var rules struct {
		Rules []struct {
			Type  string `json:"type"`
			Proxy string `json:"proxy"`
		} `json:"rules"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rules); err != nil {
		return fmt.Errorf("read rule selection: %w", err)
	}

	allowed := make(map[string]bool, len(global.All))
	for _, name := range global.All {
		allowed[name] = true
	}
	selectable := func(name string) bool {
		return allowed[name] && usesProxy(name, proxies, make(map[string]bool))
	}
	selectTarget := func(name string) error {
		if err := c.SelectProxy("GLOBAL", name); err != nil {
			return fmt.Errorf("select global proxy: %w", err)
		}
		return nil
	}
	// Prefer the rule configuration's default route, preserving its current
	// node or automatic selector rather than selecting an arbitrary first node.
	for _, rule := range rules.Rules {
		if rule.Type == "Match" && selectable(rule.Proxy) {
			return selectTarget(rule.Proxy)
		}
	}
	for _, name := range global.All {
		if isProxyGroupType(proxies[name].Type) && selectable(name) {
			return selectTarget(name)
		}
	}
	for _, name := range global.All {
		if selectable(name) {
			return selectTarget(name)
		}
	}
	return fmt.Errorf("no proxy node available for GLOBAL; mode unchanged")
}

func usesProxy(name string, proxies map[string]Proxy, visiting map[string]bool) bool {
	if name == "" || name == "GLOBAL" || name == "DIRECT" || name == "REJECT" || visiting[name] {
		return false
	}
	proxy, ok := proxies[name]
	if !ok {
		return false
	}
	switch proxy.Type {
	case "Direct", "Reject", "RejectDrop", "Pass":
		return false
	}
	if !isProxyGroupType(proxy.Type) {
		return true
	}
	visiting[name] = true
	defer delete(visiting, name)
	if proxy.Now != "" {
		return usesProxy(proxy.Now, proxies, visiting)
	}
	for _, member := range proxy.All {
		if usesProxy(member, proxies, visiting) {
			return true
		}
	}
	return false
}
