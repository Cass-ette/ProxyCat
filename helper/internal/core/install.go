package core

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

const LatestMihomoVersion = "v1.19.24"

func CurrentDarwinArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "arm64"
	case "amd64":
		return "amd64"
	default:
		return runtime.GOARCH
	}
}

func DarwinMihomoAssetName(version string, arch string) string {
	return fmt.Sprintf("mihomo-darwin-%s-%s.gz", arch, version)
}

func LatestReleaseURL(version string, arch string) string {
	asset := DarwinMihomoAssetName(version, arch)
	return fmt.Sprintf("https://github.com/MetaCubeX/mihomo/releases/download/%s/%s", version, asset)
}

func Installed(binPath string) bool {
	info, err := os.Stat(binPath)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

func InstallMihomo(binPath string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("automatic Mihomo install currently supports macOS only")
	}
	if Installed(binPath) {
		return binPath, nil
	}

	url := LatestReleaseURL(LatestMihomoVersion, CurrentDarwinArch())
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		return "", fmt.Errorf("create bin directory: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("download Mihomo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download Mihomo: HTTP %d from %s", resp.StatusCode, url)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("open Mihomo gzip: %w", err)
	}
	defer gz.Close()

	tmpPath := binPath + ".tmp"
	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", fmt.Errorf("create Mihomo binary: %w", err)
	}
	_, copyErr := io.Copy(out, gz)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("write Mihomo binary: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("close Mihomo binary: %w", closeErr)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("chmod Mihomo binary: %w", err)
	}
	if err := os.Rename(tmpPath, binPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("install Mihomo binary: %w", err)
	}
	return binPath, nil
}
