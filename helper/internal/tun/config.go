// Package tun manages a temporary, authorized Mihomo session on macOS.
package tun

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func configuration(content []byte, token, mode string, selections map[string]string) ([]byte, error) {
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("empty configuration")
	}
	cfg["external-controller"] = "127.0.0.1:9090"
	delete(cfg, "external-controller-unix")
	delete(cfg, "external-controller-pipe")
	delete(cfg, "external-controller-tls")
	cfg["secret"], cfg["allow-lan"], cfg["mixed-port"] = token, false, 7890
	if mode != "" {
		cfg["mode"] = mode
	}
	cfg["tun"] = map[string]interface{}{
		"enable": true, "stack": "system", "auto-route": true,
		"auto-detect-interface": true, "dns-hijack": []string{"any:53"},
	}
	dns, _ := cfg["dns"].(map[string]interface{})
	if dns == nil {
		dns = map[string]interface{}{}
	}
	dns["enable"], dns["listen"] = true, "127.0.0.1:0"
	if dns["nameserver"] == nil {
		dns["nameserver"] = []string{"https://dns.alidns.com/dns-query", "https://doh.pub/dns-query"}
	}
	if dns["default-nameserver"] == nil {
		dns["default-nameserver"] = []string{"223.5.5.5", "119.29.29.29"}
	}
	cfg["dns"] = dns
	// Preserve live selector choices without sharing a root-owned cache with the user core.
	cfg["profile"] = map[string]interface{}{"store-selected": false}
	groups, _ := cfg["proxy-groups"].([]interface{})
	for _, entry := range groups {
		group, _ := entry.(map[string]interface{})
		name, _ := group["name"].(string)
		if group["type"] != "select" || selections[name] == "" {
			continue
		}
		members, _ := group["proxies"].([]interface{})
		for i, member := range members {
			if member == selections[name] {
				group["proxies"] = append([]interface{}{member}, append(members[:i], members[i+1:]...)...)
				break
			}
		}
	}
	return yaml.Marshal(cfg)
}
