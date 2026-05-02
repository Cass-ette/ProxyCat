package config

import (
	"bytes"
	"encoding/base64"
	"regexp"
	"strings"
)

// Format identifies the detected subscription/config format.
type Format string

const (
	FormatUnknown     Format = "unknown"
	FormatClashYAML   Format = "clash-yaml"
	FormatBase64List  Format = "base64-uri-list"
	FormatPlainList   Format = "plain-uri-list"
)

// Confidence indicates how certain the format detection is.
type Confidence string

const (
	ConfidenceNone Confidence = "none"
	ConfidenceLow  Confidence = "low"
	ConfidenceHigh Confidence = "high"
)

var nodeURIPattern = regexp.MustCompile(`(?i)^(ss|ssr|trojan|vmess|vless|hysteria|hysteria2|hy2)://`)

// DetectFormat analyzes content to identify its subscription format.
// Supports Clash YAML, base64-encoded URI lists, and plain URI lists.
// Returns the detected format and confidence level.
func DetectFormat(content []byte) (Format, Confidence) {
	if len(content) == 0 {
		return FormatUnknown, ConfidenceNone
	}

	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return FormatUnknown, ConfidenceNone
	}

	// Check for Clash YAML
	if looksLikeClashYAML(trimmed) {
		return FormatClashYAML, ConfidenceHigh
	}

	// Check for base64 encoded content
	if looksLikeBase64List(trimmed) {
		return FormatBase64List, ConfidenceHigh
	}

	// Check for plain URI list
	if looksLikePlainList(trimmed) {
		return FormatPlainList, ConfidenceHigh
	}

	return FormatUnknown, ConfidenceNone
}

func looksLikeClashYAML(content []byte) bool {
	s := string(content)
	// Look for key YAML markers that indicate Clash/Mihomo config
	hasProxies := strings.Contains(s, "proxies:") || strings.Contains(s, "Proxy:")
	hasProxyGroups := strings.Contains(s, "proxy-groups:") || strings.Contains(s, "Proxy Group:")
	hasRules := strings.Contains(s, "rules:") || strings.Contains(s, "Rule:")

	// Require at least two of the three key sections for high confidence
	sections := 0
	if hasProxies {
		sections++
	}
	if hasProxyGroups {
		sections++
	}
	if hasRules {
		sections++
	}

	return sections >= 2
}

func looksLikeBase64List(content []byte) bool {
	lines := bytes.Split(content, []byte("\n"))
	validBase64Lines := 0
	totalLines := 0

	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		totalLines++

		// Try to decode as base64
		decoded, err := base64.StdEncoding.DecodeString(string(trimmed))
		if err == nil && len(decoded) > 0 {
			// Check if decoded content looks like node URIs
			if nodeURIPattern.Match(decoded) {
				validBase64Lines++
			}
		}
	}

	// High confidence if majority of non-empty lines are valid base64 node URIs
	if totalLines > 0 && validBase64Lines > totalLines/2 {
		return true
	}
	return false
}

func looksLikePlainList(content []byte) bool {
	lines := bytes.Split(content, []byte("\n"))
	uriLines := 0
	totalLines := 0

	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		totalLines++

		if nodeURIPattern.Match(trimmed) {
			uriLines++
		}
	}

	// High confidence if majority of non-empty lines are node URIs
	if totalLines > 0 && uriLines > totalLines/2 {
		return true
	}
	return false
}
