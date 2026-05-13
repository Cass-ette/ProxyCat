package selfupdate

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func extractInstallerZip(zipPath string, destDir string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("更新失败：安装包格式不正确")
	}
	defer r.Close()

	appEntry := ""
	for _, f := range r.File {
		if strings.HasSuffix(strings.TrimSuffix(f.Name, "/"), "ProxyCat.app") {
			appEntry = strings.TrimSuffix(f.Name, "/")
			break
		}
	}
	if appEntry == "" {
		return "", fmt.Errorf("更新失败：安装包格式不正确")
	}

	for _, f := range r.File {
		destPath, err := safeExtractPath(destDir, f.Name)
		if err != nil {
			return "", err
		}
		if strings.HasSuffix(f.Name, "/") {
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return "", err
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		w, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return "", err
		}
		_, err = io.Copy(w, rc)
		w.Close()
		rc.Close()
		if err != nil {
			return "", err
		}
	}

	appPath, err := safeExtractPath(destDir, appEntry)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(appPath); err != nil {
		return "", fmt.Errorf("更新失败：安装包格式不正确")
	}
	return appPath, nil
}

func safeExtractPath(destDir string, name string) (string, error) {
	cleanName := filepath.Clean(name)
	if cleanName == "." || filepath.IsAbs(cleanName) || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) || cleanName == ".." {
		return "", fmt.Errorf("更新失败：安装包格式不正确")
	}
	return filepath.Join(destDir, cleanName), nil
}
