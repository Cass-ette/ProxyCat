package redact

import (
	"net/url"
	"regexp"
	"strings"
)

const (
	redactedValue   = "<redacted>"
	redactedNodeURI = "<redacted-node-uri>"
)

var sensitiveQueryKeys = map[string]struct{}{
	"token":    {},
	"key":      {},
	"password": {},
	"pass":     {},
	"uuid":     {},
	"secret":   {},
}

var rawNodeURIPattern = regexp.MustCompile(`(?i)\b(ss|ssr|trojan|vmess|vless|hysteria|hysteria2|hy2)://\S+`)
var httpURLPattern = regexp.MustCompile(`(?i)\bhttps?://\S+`)

func URL(input string) string {
	if isRawNodeURI(input) {
		return redactedNodeURI
	}

	parsed, err := url.Parse(input)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return String(input)
	}
	return redactParsedURL(parsed)
}

func String(input string) string {
	if isRawNodeURI(input) {
		return redactedNodeURI
	}

	redacted := rawNodeURIPattern.ReplaceAllString(input, redactedNodeURI)
	return httpURLPattern.ReplaceAllStringFunc(redacted, func(match string) string {
		parsed, err := url.Parse(match)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return match
		}
		return redactParsedURL(parsed)
	})
}

func redactParsedURL(parsed *url.URL) string {
	copyValue := *parsed
	if copyValue.User != nil {
		copyValue.User = url.User(redactedValue)
	}

	query := copyValue.Query()
	for key := range query {
		if _, ok := sensitiveQueryKeys[strings.ToLower(key)]; ok {
			query.Set(key, redactedValue)
		}
	}
	copyValue.RawQuery = query.Encode()
	return copyValue.String()
}

func isRawNodeURI(input string) bool {
	trimmed := strings.TrimSpace(input)
	return rawNodeURIPattern.MatchString(trimmed) && rawNodeURIPattern.FindString(trimmed) == trimmed
}
