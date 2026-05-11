package subscription

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/Cass-ette/ProxyCat/helper/internal/config"
	"github.com/Cass-ette/ProxyCat/helper/internal/redact"
)

var ProbeUserAgents = []string{
	"clash-verge/v1.7.7",
	"Clash.Meta/1.18.0",
	"mihomo/1.18.0",
	"ClashforWindows/0.20.39",
	"ProxyCat/0.1.0",
}

var supportedURISchemes = map[string]bool{
	"ss":     true,
	"trojan": true,
	"vmess":  true,
	"vless":  true,
}

var uriSchemePattern = regexp.MustCompile(`(?i)(^|\s)([a-z][a-z0-9+.-]*)://`)

type ProbeResult struct {
	URL             string         `json:"url"`
	Attempts        []ProbeAttempt `json:"attempts"`
	Selected        *ProbeAttempt  `json:"selected,omitempty"`
	SelectedContent []byte         `json:"-"`
	Message         string         `json:"message,omitempty"`
	SuggestedFix    string         `json:"suggestedFix,omitempty"`
}

type ProbeAttempt struct {
	UserAgent          string         `json:"userAgent"`
	Status             string         `json:"status"`
	Format             string         `json:"format"`
	HTTPStatus         int            `json:"httpStatus,omitempty"`
	ProxyCount         int            `json:"proxyCount,omitempty"`
	GroupCount         int            `json:"groupCount,omitempty"`
	RuleCount          int            `json:"ruleCount,omitempty"`
	NodeCount          int            `json:"nodeCount,omitempty"`
	UnsupportedSchemes map[string]int `json:"unsupportedSchemes,omitempty"`
	Message            string         `json:"message,omitempty"`
}

func Probe(client HTTPClient, rawURL string) ProbeResult {
	result := ProbeResult{URL: redact.URL(rawURL)}

	for _, userAgent := range ProbeUserAgents {
		content, httpStatus, err := downloadForProbe(client, rawURL, userAgent)
		attempt := classifyAttempt(userAgent, httpStatus, content, err)
		result.Attempts = append(result.Attempts, attempt)
		if isBetterAttempt(attempt, result.Selected) {
			copyAttempt := attempt
			result.Selected = &copyAttempt
			result.SelectedContent = content
		}
	}

	if result.Selected == nil {
		result.Message, result.SuggestedFix = summarizeProbeFailure(result.Attempts)
	}
	return result
}

func downloadForProbe(client HTTPClient, rawURL string, userAgent string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("download: status %d", resp.StatusCode)
	}
	content, err := readLimitedBody(resp.Body)
	return content, resp.StatusCode, err
}

func readLimitedBody(body io.Reader) ([]byte, error) {
	return readAllLimited(body, maxDownloadSize)
}

func classifyAttempt(userAgent string, httpStatus int, content []byte, err error) ProbeAttempt {
	attempt := ProbeAttempt{UserAgent: userAgent, HTTPStatus: httpStatus}
	if err != nil {
		attempt.Status = "fail"
		attempt.Format = "download-error"
		attempt.Message = redact.String(err.Error())
		return attempt
	}

	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		attempt.Status = "fail"
		attempt.Format = "empty-response"
		attempt.Message = "订阅链接返回了空内容。"
		return attempt
	}
	if looksLikeHTML(trimmed) {
		attempt.Status = "fail"
		attempt.Format = "html-page"
		attempt.Message = "订阅链接返回的是网页，不是代理订阅。"
		return attempt
	}

	format, _ := config.DetectFormat(content)
	attempt.Format = string(format)
	switch format {
	case config.FormatClashYAML:
		validation := config.Validate(content)
		attempt.ProxyCount = validation.ProxyCount
		attempt.GroupCount = validation.GroupCount
		attempt.RuleCount = validation.RuleCount
		if validation.Valid {
			attempt.Status = "success"
			return attempt
		}
		attempt.Status = "fail"
		attempt.Message = validation.Message
		return attempt
	case config.FormatBase64List, config.FormatPlainList:
		attempt.NodeCount, attempt.UnsupportedSchemes = usableURINodes(content, format)
		if attempt.NodeCount > 0 {
			attempt.Status = "success"
			return attempt
		}
		attempt.Status = "fail"
		attempt.Message = "订阅下载成功，但节点协议暂不支持。"
		return attempt
	default:
		attempt.Status = "fail"
		attempt.Format = "unknown"
		attempt.UnsupportedSchemes = unsupportedSchemesFromText(string(content))
		if len(attempt.UnsupportedSchemes) > 0 {
			attempt.Message = "订阅下载成功，但节点协议暂不支持。"
			return attempt
		}
		attempt.Message = "无法识别订阅格式。"
		return attempt
	}
}

func looksLikeHTML(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype html")
}

func usableURINodes(content []byte, format config.Format) (int, map[string]int) {
	text := string(content)
	if format == config.FormatBase64List {
		decoded, err := decodeProbeBase64(content)
		if err == nil {
			text = string(decoded)
		}
	}

	count := 0
	unsupported := map[string]int{}
	for _, match := range uriSchemePattern.FindAllStringSubmatch(text, -1) {
		scheme := strings.ToLower(match[2])
		if supportedURISchemes[scheme] {
			count++
		} else {
			unsupported[scheme]++
		}
	}
	if len(unsupported) == 0 {
		return count, nil
	}
	return count, unsupported
}

func unsupportedSchemesFromText(text string) map[string]int {
	counts := map[string]int{}
	for _, match := range uriSchemePattern.FindAllStringSubmatch(text, -1) {
		scheme := strings.ToLower(match[2])
		if !supportedURISchemes[scheme] {
			counts[scheme]++
		}
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}

func decodeProbeBase64(content []byte) ([]byte, error) {
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

func isBetterAttempt(candidate ProbeAttempt, selected *ProbeAttempt) bool {
	if candidate.Status != "success" {
		return false
	}
	if selected == nil {
		return true
	}
	if candidate.Format == string(config.FormatClashYAML) && selected.Format != string(config.FormatClashYAML) {
		return true
	}
	if candidate.Format != string(config.FormatClashYAML) && selected.Format == string(config.FormatClashYAML) {
		return false
	}
	return candidate.ProxyCount+candidate.NodeCount > selected.ProxyCount+selected.NodeCount
}

func summarizeProbeFailure(attempts []ProbeAttempt) (string, string) {
	for _, attempt := range attempts {
		if attempt.Format == "html-page" {
			return "订阅链接返回的是网页，不是代理订阅。", "请重新复制机场后台的 Clash/Mihomo/Clash Verge 订阅链接。"
		}
	}
	for _, attempt := range attempts {
		if attempt.Format == "empty-response" {
			return "订阅链接返回了空内容。", "请检查链接是否过期，或重新复制机场后台的 Clash/Mihomo 订阅链接。"
		}
	}
	unsupported := map[string]int{}
	for _, attempt := range attempts {
		for scheme, count := range attempt.UnsupportedSchemes {
			unsupported[scheme] += count
		}
	}
	if len(unsupported) > 0 {
		return "订阅下载成功，但包含 ProxyCat 暂不支持的节点协议：" + formatSchemeCounts(unsupported), "请优先复制 Clash/Mihomo/Clash Verge 订阅链接。"
	}
	return "无法导入这个订阅链接。", "请检查链接是否过期，或重新复制机场后台的 Clash/Mihomo 订阅链接。"
}

func formatSchemeCounts(counts map[string]int) string {
	schemes := make([]string, 0, len(counts))
	for scheme := range counts {
		schemes = append(schemes, scheme)
	}
	sort.Strings(schemes)
	parts := make([]string, 0, len(schemes))
	for _, scheme := range schemes {
		parts = append(parts, fmt.Sprintf("%s %d 个", scheme, counts[scheme]))
	}
	return strings.Join(parts, "，")
}
