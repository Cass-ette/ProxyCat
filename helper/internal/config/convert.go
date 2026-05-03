package config

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type mihomoConfig map[string]interface{}

// NormalizeClashYAML fills the ports and controller settings ProxyCat expects.
func NormalizeClashYAML(content []byte) (string, error) {
	var cfg mihomoConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return "", err
	}
	applyProxyCatDefaults(cfg)
	return marshalConfig(cfg)
}

// ConvertURIToYAML converts a URI-list subscription into a minimal Mihomo config.
func ConvertURIToYAML(content []byte, format Format) (string, error) {
	uris, err := extractURIs(content, format)
	if err != nil {
		return "", err
	}
	if len(uris) == 0 {
		return "", fmt.Errorf("no supported proxy nodes found")
	}

	proxies := make([]map[string]interface{}, 0, len(uris))
	proxyNames := make([]string, 0, len(uris)+1)
	for i, uri := range uris {
		proxy, err := parseNodeURI(uri, i+1)
		if err != nil {
			continue
		}
		proxies = append(proxies, proxy)
		proxyNames = append(proxyNames, proxy["name"].(string))
	}
	if len(proxies) == 0 {
		return "", fmt.Errorf("no supported proxy nodes found")
	}
	proxyNames = append(proxyNames, "DIRECT")

	cfg := mihomoConfig{
		"proxies": proxies,
		"proxy-groups": []map[string]interface{}{
			{
				"name":    "Proxy",
				"type":    "select",
				"proxies": proxyNames,
			},
		},
		"rules": []string{"MATCH,Proxy"},
	}
	applyProxyCatDefaults(cfg)
	return marshalConfig(cfg)
}

func applyProxyCatDefaults(cfg mihomoConfig) {
	cfg["mixed-port"] = 7890
	cfg["allow-lan"] = false
	cfg["mode"] = "rule"
	cfg["log-level"] = "info"
	cfg["external-controller"] = "127.0.0.1:9090"
}

func marshalConfig(cfg mihomoConfig) (string, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func extractURIs(content []byte, format Format) ([]string, error) {
	switch format {
	case FormatPlainList:
		return plainURIs(content), nil
	case FormatBase64List:
		decoded, err := decodeBase64Subscription(content)
		if err != nil {
			return nil, err
		}
		return plainURIs(decoded), nil
	default:
		return nil, fmt.Errorf("unsupported URI list format: %s", format)
	}
}

func decodeBase64Subscription(content []byte) ([]byte, error) {
	compact := bytes.TrimSpace(content)
	compact = bytes.ReplaceAll(compact, []byte("\n"), nil)
	compact = bytes.ReplaceAll(compact, []byte("\r"), nil)
	decoded, err := base64.StdEncoding.DecodeString(string(compact))
	if err == nil {
		return decoded, nil
	}
	decoded, rawErr := base64.RawStdEncoding.DecodeString(string(compact))
	if rawErr == nil {
		return decoded, nil
	}
	return nil, err
}

func plainURIs(content []byte) []string {
	lines := strings.Fields(string(content))
	uris := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if nodeURIPattern.MatchString(trimmed) {
			uris = append(uris, trimmed)
		}
	}
	return uris
}

func parseNodeURI(raw string, fallbackIndex int) (map[string]interface{}, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}

	name := u.Fragment
	if name == "" {
		name = fmt.Sprintf("node-%d", fallbackIndex)
	}

	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil, fmt.Errorf("invalid port")
	}

	switch strings.ToLower(u.Scheme) {
	case "trojan":
		return parseTrojan(u, name, port), nil
	case "vless":
		return parseVLESS(u, name, port), nil
	case "vmess":
		return parseVMess(u, name, fallbackIndex)
	case "ss":
		return parseShadowsocks(u, name, port)
	default:
		return nil, fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
}

func parseTrojan(u *url.URL, name string, port int) map[string]interface{} {
	q := u.Query()
	proxy := map[string]interface{}{
		"name":     name,
		"type":     "trojan",
		"server":   u.Hostname(),
		"port":     port,
		"password": u.User.Username(),
		"udp":      true,
		"tls":      true,
	}
	proxy["sni"] = firstNonEmpty(q.Get("sni"), q.Get("peer"), u.Hostname())
	if q.Get("allowInsecure") == "1" || q.Get("skip-cert-verify") == "true" {
		proxy["skip-cert-verify"] = true
	}
	addTransport(proxy, q)
	return proxy
}

func parseVLESS(u *url.URL, name string, port int) map[string]interface{} {
	q := u.Query()
	proxy := map[string]interface{}{
		"name":   name,
		"type":   "vless",
		"server": u.Hostname(),
		"port":   port,
		"uuid":   u.User.Username(),
		"udp":    true,
		"tls":    isTLS(q),
	}
	if q.Get("flow") != "" {
		proxy["flow"] = q.Get("flow")
	}
	if q.Get("sni") != "" || q.Get("security") == "tls" {
		proxy["servername"] = firstNonEmpty(q.Get("sni"), q.Get("peer"), q.Get("host"), u.Hostname())
	}
	addTransport(proxy, q)
	return proxy
}

func parseVMess(u *url.URL, name string, fallbackIndex int) (map[string]interface{}, error) {
	payload := u.Host
	if payload == "" {
		payload = strings.TrimPrefix(u.Opaque, "//")
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(payload)
	}
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := yaml.Unmarshal(decoded, &data); err != nil {
		return nil, err
	}

	port, _ := strconv.Atoi(fmt.Sprint(data["port"]))
	if port == 0 {
		port, _ = strconv.Atoi(fmt.Sprint(data["ps"]))
	}
	vmessName := firstNonEmpty(name, fmt.Sprint(data["ps"]), fmt.Sprintf("node-%d", fallbackIndex))
	proxy := map[string]interface{}{
		"name":    vmessName,
		"type":    "vmess",
		"server":  fmt.Sprint(data["add"]),
		"port":    port,
		"uuid":    fmt.Sprint(data["id"]),
		"alterId": intFromAny(data["aid"]),
		"cipher":  firstNonEmpty(fmt.Sprint(data["scy"]), "auto"),
		"udp":     true,
	}
	if fmt.Sprint(data["tls"]) == "tls" {
		proxy["tls"] = true
		proxy["servername"] = firstNonEmpty(fmt.Sprint(data["sni"]), fmt.Sprint(data["host"]), fmt.Sprint(data["add"]))
	}
	if fmt.Sprint(data["net"]) == "ws" {
		opts := map[string]interface{}{}
		if host := fmt.Sprint(data["host"]); host != "" && host != "<nil>" {
			opts["headers"] = map[string]string{"Host": host}
		}
		if path := fmt.Sprint(data["path"]); path != "" && path != "<nil>" {
			opts["path"] = path
		}
		proxy["network"] = "ws"
		proxy["ws-opts"] = opts
	}
	return proxy, nil
}

func parseShadowsocks(u *url.URL, name string, port int) (map[string]interface{}, error) {
	method := ""
	password := ""
	userinfo := u.User.String()
	if strings.Contains(userinfo, ":") {
		method = u.User.Username()
		password, _ = u.User.Password()
	} else {
		decoded, err := base64.RawStdEncoding.DecodeString(userinfo)
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(userinfo)
		}
		if err != nil {
			return nil, err
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid ss credentials")
		}
		method = parts[0]
		password = parts[1]
	}
	return map[string]interface{}{
		"name":     name,
		"type":     "ss",
		"server":   net.ParseIP(u.Hostname()).String(),
		"port":     port,
		"cipher":   method,
		"password": password,
		"udp":      true,
	}, nil
}

func addTransport(proxy map[string]interface{}, q url.Values) {
	network := firstNonEmpty(q.Get("type"), q.Get("network"))
	if network == "" {
		return
	}
	proxy["network"] = network
	if network == "ws" {
		opts := map[string]interface{}{}
		if host := q.Get("host"); host != "" {
			opts["headers"] = map[string]string{"Host": host}
		}
		if path := q.Get("path"); path != "" {
			opts["path"] = path
		}
		proxy["ws-opts"] = opts
	}
}

func isTLS(q url.Values) bool {
	security := strings.ToLower(q.Get("security"))
	return security == "tls" || security == "reality"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func intFromAny(value interface{}) int {
	i, _ := strconv.Atoi(fmt.Sprint(value))
	return i
}
