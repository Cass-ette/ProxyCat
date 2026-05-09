package core

import (
	"archive/zip"
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

func WindowsMihomoAssetName(version string, arch string) string {
	return fmt.Sprintf("mihomo-windows-%s-%s.zip", arch, version)
}

func LatestReleaseURL(version string, arch string) string {
	asset := DarwinMihomoAssetName(version, arch)
	return fmt.Sprintf("https://github.com/MetaCubeX/mihomo/releases/download/%s/%s", version, asset)
}

func LatestReleaseURLForOS(goos, version, arch string) string {
	var asset string
	switch goos {
	case "windows":
		asset = WindowsMihomoAssetName(version, arch)
	default:
		asset = DarwinMihomoAssetName(version, arch)
	}
	return fmt.Sprintf("https://github.com/MetaCubeX/mihomo/releases/download/%s/%s", version, asset)
}

func Installed(binPath string) bool {
	info, err := os.Stat(binPath)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return info.Size() > 0
	}
	return info.Mode()&0o111 != 0
}

func InstallMihomo(binPath string) (string, error) {
	if Installed(binPath) {
		return binPath, nil
	}

	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		return "", fmt.Errorf("create bin directory: %w", err)
	}

	if runtime.GOOS == "windows" {
		return installWindows(binPath)
	}
	return installDarwin(binPath)
}

func installDarwin(binPath string) (string, error) {
	url := LatestReleaseURL(LatestMihomoVersion, CurrentDarwinArch())

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

	return writeBinary(binPath, gz)
}

func installWindows(binPath string) (string, error) {
	arch := "amd64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}
	url := LatestReleaseURLForOS("windows", LatestMihomoVersion, arch)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("download Mihomo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download Mihomo: HTTP %d from %s", resp.StatusCode, url)
	}

	tmpDir, err := os.MkdirTemp("", "mihomo-install")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	zipPath := filepath.Join(tmpDir, "mihomo.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		return "", fmt.Errorf("create temp zip: %w", err)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		return "", fmt.Errorf("write temp zip: %w", err)
	}
	out.Close()

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name == "mihomo-windows-amd64-"+LatestMihomoVersion+".exe" || f.Name == "mihomo.exe" || filepath.Base(f.Name) == "mihomo.exe" {
			rc, err := f.Open()
			if err != nil {
				return "", fmt.Errorf("open zip entry: %w", err)
			}
			result, err := writeBinary(binPath, rc)
			rc.Close()
			return result, err
		}
	}

	return "", fmt.Errorf("mihomo executable not found in zip")
}

func writeBinary(binPath string, src io.Reader) (string, error) {
	tmpPath := binPath + ".tmp"
	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", fmt.Errorf("create binary: %w", err)
	}
	_, copyErr := io.Copy(out, src)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("write binary: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("close binary: %w", closeErr)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0o755); err != nil {
			_ = os.Remove(tmpPath)
			return "", fmt.Errorf("chmod binary: %w", err)
		}
	}
	if err := os.Rename(tmpPath, binPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("install binary: %w", err)
	}
	return binPath, nil
}
