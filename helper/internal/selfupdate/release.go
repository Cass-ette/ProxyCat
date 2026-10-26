package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Release struct {
	Version      string
	InstallerURL string
	SHA256URL    string
	Size         int64
	Format       string
	Notes        string
	URL          string
	PublishedAt  string
}

type githubRelease struct {
	TagName     string `json:"tag_name"`
	Body        string `json:"body"`
	URL         string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

func fetchLatestRelease(client *http.Client, endpoint string) (Release, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Release{}, fmt.Errorf("更新地址无效")
	}
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("无法连接更新服务，请检查网络后重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0") {
		return Release{}, fmt.Errorf("GitHub 更新请求已达限额，请稍后重试或查看所有版本")
	}
	if resp.StatusCode == http.StatusForbidden {
		return Release{}, fmt.Errorf("更新服务拒绝访问（HTTP 403），请稍后重试")
	}
	if resp.StatusCode == http.StatusNotFound {
		return Release{}, fmt.Errorf("还没有可用更新")
	}
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("更新服务暂时不可用（HTTP %d），请稍后重试", resp.StatusCode)
	}

	var decoded githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return Release{}, fmt.Errorf("更新说明格式不正确，请稍后重试")
	}

	versionText := strings.TrimPrefix(decoded.TagName, "v")
	if _, err := parseVersion(versionText); err != nil {
		return Release{}, err
	}

	// Prefer the same DMG offered for manual installation. Keep ZIP support
	// for releases published before DMG updates were available, and never mix
	// an installer's checksum with a different format or version.
	for _, candidate := range []struct{ suffix, format string }{{".dmg", "dmg"}, {"-installer.zip", "zip"}} {
		release := Release{Version: versionText, Format: candidate.format, Notes: decoded.Body, URL: decoded.URL, PublishedAt: decoded.PublishedAt}
		name := "ProxyCat-" + versionText + candidate.suffix
		for _, asset := range decoded.Assets {
			if asset.Name == name {
				release.InstallerURL = asset.URL
				release.Size = asset.Size
			}
			if asset.Name == name+".sha256" {
				release.SHA256URL = asset.URL
			}
		}
		if release.InstallerURL != "" && release.SHA256URL != "" {
			return release, nil
		}
	}
	return Release{}, fmt.Errorf("更新失败：安装包格式不正确")
}

// The release asset is the primary source and does not consume GitHub's
// shared anonymous API quota. The API remains available for older releases
// without a manifest and when the asset download service is unavailable.
func fetchReleaseWithFallback(client *http.Client, endpoint, fallback string) (Release, error) {
	release, err := fetchLatestRelease(client, endpoint)
	if err == nil || fallback == "" || fallback == endpoint {
		return release, err
	}
	return fetchLatestRelease(client, fallback)
}
