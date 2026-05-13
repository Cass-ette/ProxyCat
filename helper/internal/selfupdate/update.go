package selfupdate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type Runner struct {
	CurrentVersion string
	Latest         Release
	CheckOnly      bool
	Client         *http.Client
	Endpoint       string
	AppPath        string
	BackupDir      string
}

type Event struct {
	Stage      string `json:"stage"`
	Message    string `json:"message"`
	Progress   int    `json:"progress,omitempty"`
	NewVersion string `json:"newVersion,omitempty"`
}

func (r Runner) Run(stdout io.Writer, jsonOutput bool) int {
	current, err := parseVersion(r.CurrentVersion)
	if err != nil {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: "更新失败：安装包格式不正确"})
		return 1
	}

	var latest Release
	if r.Latest.Version != "" {
		latest = r.Latest
	} else if r.Endpoint != "" && r.Client != nil {
		emit(stdout, jsonOutput, Event{Stage: "checking", Message: "正在检查更新..."})
		fetched, err := fetchLatestRelease(r.Client, r.Endpoint)
		if err != nil {
			emit(stdout, jsonOutput, Event{Stage: "error", Message: err.Error()})
			return 1
		}
		latest = fetched
	} else {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: "更新失败：安装包格式不正确"})
		return 1
	}

	latestVer, err := parseVersion(latest.Version)
	if err != nil {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: "更新失败：安装包格式不正确"})
		return 1
	}
	if current.compare(latestVer) >= 0 {
		emit(stdout, jsonOutput, Event{Stage: "done", Message: "已经是最新版"})
		return 0
	}
	if r.CheckOnly {
		emit(stdout, jsonOutput, Event{Stage: "done", Message: "发现新版本 " + latest.Version, NewVersion: latest.Version})
		return 0
	}

	if r.Client == nil || r.AppPath == "" || r.BackupDir == "" {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: "更新失败：安装尚未完成"})
		return 1
	}

	tmpDir, err := os.MkdirTemp("", "proxycat-update-*")
	if err != nil {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: "更新失败，请截图发给我"})
		return 1
	}
	defer os.RemoveAll(tmpDir)

	// Download installer zip
	emit(stdout, jsonOutput, Event{Stage: "downloading", Message: "正在下载更新...", Progress: 20})
	zipPath := filepath.Join(tmpDir, "update.zip")
	if err := downloadFile(r.Client, latest.InstallerURL, zipPath, nil); err != nil {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: err.Error()})
		return 1
	}

	// Download and verify SHA256
	emit(stdout, jsonOutput, Event{Stage: "verifying", Message: "正在验证更新包...", Progress: 50})
	sha256Path := filepath.Join(tmpDir, "update.zip.sha256")
	if err := downloadFile(r.Client, latest.SHA256URL, sha256Path, nil); err != nil {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: err.Error()})
		return 1
	}
	sha256Content, err := os.ReadFile(sha256Path)
	if err != nil {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: "更新失败：安装包格式不正确"})
		return 1
	}
	expectedHash, err := parseSHA256Sidecar(sha256Content)
	if err != nil {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: err.Error()})
		return 1
	}
	if err := verifyFileSHA256(zipPath, expectedHash); err != nil {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: err.Error()})
		return 1
	}

	// Extract
	emit(stdout, jsonOutput, Event{Stage: "installing", Message: "正在安装更新...", Progress: 70})
	extractDir := filepath.Join(tmpDir, "extracted")
	newApp, err := extractInstallerZip(zipPath, extractDir)
	if err != nil {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: err.Error()})
		return 1
	}

	// Validate bundle
	if err := validateBundle(newApp); err != nil {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: err.Error()})
		return 1
	}

	// Replace
	if err := replaceApp(r.AppPath, newApp, r.BackupDir, r.CurrentVersion); err != nil {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: err.Error()})
		return 1
	}

	emit(stdout, jsonOutput, Event{Stage: "done", Message: "更新完成 " + latest.Version, NewVersion: latest.Version, Progress: 100})
	return 0
}

func emit(w io.Writer, jsonOutput bool, event Event) {
	if jsonOutput {
		_ = json.NewEncoder(w).Encode(event)
		return
	}
	fmt.Fprintln(w, event.Message)
}
