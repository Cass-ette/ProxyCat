package selfupdate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Release struct {
	Version      string
	InstallerURL string
	SHA256URL    string
	Size         int64
	Format       string
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

func fetchLatestRelease(client *http.Client, endpoint string) (Release, error) {
	resp, err := client.Get(endpoint)
	if err != nil {
		return Release{}, fmt.Errorf("下载失败，请检查网络后重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return Release{}, fmt.Errorf("更新检查暂时不可用，请稍后重试")
	}
	if resp.StatusCode == http.StatusNotFound {
		return Release{}, fmt.Errorf("还没有可用更新")
	}
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("下载失败，请检查网络后重试")
	}

	var decoded githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return Release{}, fmt.Errorf("更新失败：安装包格式不正确")
	}

	versionText := strings.TrimPrefix(decoded.TagName, "v")
	if _, err := parseVersion(versionText); err != nil {
		return Release{}, err
	}

	// Prefer the same DMG offered for manual installation. Keep ZIP support
	// for releases published before DMG updates were available, and never mix
	// an installer's checksum with a different format or version.
	for _, candidate := range []struct{ suffix, format string }{{".dmg", "dmg"}, {"-installer.zip", "zip"}} {
		release := Release{Version: versionText, Format: candidate.format}
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
