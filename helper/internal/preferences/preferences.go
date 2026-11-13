package preferences

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Preference struct {
	Host      string    `json:"host"`
	Route     string    `json:"route"`
	CreatedAt time.Time `json:"createdAt"`
}

type Store struct {
	Preferences []Preference `json:"preferences"`
}

func NormalizeHost(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return "", errors.New("网站地址不能为空")
	}
	if ip := net.ParseIP(strings.Trim(value, "[]")); ip != nil {
		return ip.String(), nil
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("请输入有效的网站或 IP 地址")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if ip := net.ParseIP(host); ip != nil {
		return ip.String(), nil
	}
	if len(host) > 253 || strings.Contains(host, "..") {
		return "", errors.New("网站域名无效")
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", errors.New("网站域名无效")
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return "", errors.New("网站域名无效")
			}
		}
	}
	return host, nil
}

func Load(path string) (Store, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Store{Preferences: []Preference{}}, nil
	}
	if err != nil {
		return Store{}, err
	}
	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return Store{}, err
	}
	if store.Preferences == nil {
		store.Preferences = []Preference{}
	}
	seen := make(map[string]struct{}, len(store.Preferences))
	for i := range store.Preferences {
		item := &store.Preferences[i]
		if item.Route != "proxy" && item.Route != "direct" {
			return Store{}, fmt.Errorf("website preference %d has an invalid route", i)
		}
		host, err := NormalizeHost(item.Host)
		if err != nil {
			return Store{}, fmt.Errorf("website preference %d has an invalid host: %w", i, err)
		}
		if _, exists := seen[host]; exists {
			return Store{}, fmt.Errorf("website preferences contain duplicate host %q", host)
		}
		seen[host] = struct{}{}
		item.Host = host
	}
	return store, nil
}

func Save(path string, store Store) error {
	sort.Slice(store.Preferences, func(i, j int) bool { return store.Preferences[i].Host < store.Preferences[j].Host })
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".preferences-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func Upsert(store *Store, host, route string) error {
	if route != "proxy" && route != "direct" {
		return fmt.Errorf("网站偏好必须是 proxy 或 direct")
	}
	host, err := NormalizeHost(host)
	if err != nil {
		return err
	}
	for i := range store.Preferences {
		if store.Preferences[i].Host == host {
			store.Preferences[i].Route = route
			return nil
		}
	}
	store.Preferences = append(store.Preferences, Preference{Host: host, Route: route, CreatedAt: time.Now().UTC()})
	return nil
}

func Remove(store *Store, host string) (Store, error) {
	normalized, err := NormalizeHost(host)
	if err != nil {
		return Store{}, err
	}
	filtered := store.Preferences[:0]
	for _, item := range store.Preferences {
		if item.Host != normalized {
			filtered = append(filtered, item)
		}
	}
	store.Preferences = filtered
	return *store, nil
}

// ApplyToYAML inserts user preferences before subscription rules. It returns an
// error when a proxy preference cannot resolve to a proxy-capable group, so a
// failed refresh never replaces a previously working config.
func ApplyToYAML(content []byte, store Store) (string, error) {
	if len(store.Preferences) == 0 {
		return string(content), nil
	}
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return "", err
	}
	target := ""
	for _, preference := range store.Preferences {
		if preference.Route == "proxy" {
			target = proxyTarget(cfg)
			if target == "" {
				return "", errors.New("当前配置没有可用的代理分组，未应用网站偏好")
			}
			break
		}
	}
	rules := make([]string, 0, len(store.Preferences))
	// More specific domains must win over a saved parent-domain preference.
	ordered := append([]Preference(nil), store.Preferences...)
	sort.SliceStable(ordered, func(i, j int) bool { return len(ordered[i].Host) > len(ordered[j].Host) })
	for _, preference := range ordered {
		if ip := net.ParseIP(preference.Host); ip != nil {
			kind := "IP-CIDR"
			if ip.To4() == nil {
				kind = "IP-CIDR6"
			}
			prefix := "/32"
			if kind == "IP-CIDR6" {
				prefix = "/128"
			}
			rules = append(rules, fmt.Sprintf("%s,%s%s,%s", kind, preference.Host, prefix, routeTarget(preference.Route, target)))
		} else {
			rules = append(rules, fmt.Sprintf("DOMAIN-SUFFIX,%s,%s", preference.Host, routeTarget(preference.Route, target)))
		}
	}
	preferenceSet := make(map[string]bool, len(rules))
	for _, rule := range rules {
		preferenceSet[rule] = true
	}
	merged := make([]interface{}, 0, len(rules))
	for _, rule := range rules {
		merged = append(merged, rule)
	}
	switch existing := cfg["rules"].(type) {
	case []interface{}:
		for _, item := range existing {
			if rule, ok := item.(string); ok && preferenceSet[rule] {
				continue
			}
			merged = append(merged, item)
		}
	case []string:
		for _, rule := range existing {
			if preferenceSet[rule] {
				continue
			}
			merged = append(merged, rule)
		}
	}
	cfg["rules"] = merged
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func RebuildYAML(content []byte, previous, next Store) (string, error) {
	if len(previous.Preferences) == 0 {
		return ApplyToYAML(content, next)
	}
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return "", err
	}
	oldHosts := make(map[string]bool, len(previous.Preferences))
	for _, item := range previous.Preferences {
		oldHosts[item.Host] = true
	}
	rules := make([]interface{}, 0)
	switch existing := cfg["rules"].(type) {
	case []interface{}:
		for _, item := range existing {
			if !isPreferenceRule(item, oldHosts) {
				rules = append(rules, item)
			}
		}
	case []string:
		for _, item := range existing {
			if !isPreferenceRule(item, oldHosts) {
				rules = append(rules, item)
			}
		}
	}
	cfg["rules"] = rules
	base, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return ApplyToYAML(base, next)
}

func isPreferenceRule(raw interface{}, hosts map[string]bool) bool {
	rule, ok := raw.(string)
	if !ok {
		return false
	}
	parts := strings.Split(rule, ",")
	if len(parts) != 3 {
		return false
	}
	kind, host := strings.ToUpper(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1])
	if strings.HasPrefix(kind, "IP-CIDR") {
		host = strings.SplitN(host, "/", 2)[0]
	}
	if hosts[host] && (kind == "DOMAIN-SUFFIX" || kind == "IP-CIDR" || kind == "IP-CIDR6") {
		return true
	}
	return false
}

func routeTarget(route, proxy string) string {
	if route == "direct" {
		return "DIRECT"
	}
	return proxy
}

func proxyTarget(cfg map[string]interface{}) string {
	groups, ok := cfg["proxy-groups"].([]interface{})
	if !ok {
		return ""
	}
	preferred := ""
	for _, raw := range groups {
		group, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := group["name"].(string)
		typ, _ := group["type"].(string)
		if name == "" || strings.EqualFold(name, "GLOBAL") || strings.EqualFold(name, "DIRECT") || strings.EqualFold(name, "REJECT") {
			continue
		}
		if strings.EqualFold(name, "Proxy") && isProxyGroup(typ) {
			return name
		}
		if preferred == "" && isProxyGroup(typ) {
			preferred = name
		}
	}
	return preferred
}

func isProxyGroup(kind string) bool {
	switch strings.ToLower(kind) {
	case "select", "url-test", "fallback", "load-balance", "relay":
		return true
	default:
		return false
	}
}
