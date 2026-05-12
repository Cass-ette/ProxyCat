package selfupdate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var installerAssetPattern = regexp.MustCompile(`^ProxyCat-(\d+\.\d+\.\d+)-installer\.zip$`)

type Release struct {
	Version      string
	InstallerURL string
	SHA256URL    string
	Size         int64
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

	var release Release
	release.Version = versionText
	for _, asset := range decoded.Assets {
		match := installerAssetPattern.FindStringSubmatch(asset.Name)
		if match != nil && match[1] == versionText {
			release.InstallerURL = asset.URL
			release.Size = asset.Size
		}
		if asset.Name == "ProxyCat-"+versionText+"-installer.zip.sha256" {
			release.SHA256URL = asset.URL
		}
	}
	if release.InstallerURL == "" || release.SHA256URL == "" {
		return Release{}, fmt.Errorf("更新失败：安装包格式不正确")
	}
	return release, nil
}
