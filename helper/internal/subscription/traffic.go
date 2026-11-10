package subscription

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Traffic is the provider's account-wide subscription usage, in bytes.
// It is not a counter of traffic observed by this application.
type Traffic struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
	Expire   int64 `json:"expire,omitempty"`
}

func ParseTraffic(value string) *Traffic {
	fields := make(map[string]int64)
	for _, part := range strings.Split(value, ";") {
		key, raw, ok := strings.Cut(part, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		if !ok || (key != "upload" && key != "download" && key != "total" && key != "expire") {
			continue
		}
		number, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || number < 0 {
			return nil
		}
		if _, duplicate := fields[key]; duplicate {
			return nil
		}
		fields[key] = number
	}
	for _, required := range []string{"upload", "download", "total"} {
		if _, ok := fields[required]; !ok {
			return nil
		}
	}
	return &Traffic{Upload: fields["upload"], Download: fields["download"], Total: fields["total"], Expire: fields["expire"]}
}

// FetchTraffic reads metadata only. It does not regenerate configuration,
// reload a core, or change any network setting.
func FetchTraffic(client HTTPClient, rawURL string) (*Traffic, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid subscription URL")
	}
	req.Header.Set("User-Agent", ProbeUserAgents[0])
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接订阅服务，请稍后刷新流量")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("订阅服务返回 HTTP %d，请稍后刷新流量", response.StatusCode)
	}
	return ParseTraffic(response.Header.Get("Subscription-Userinfo")), nil
}
