package selfupdate

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const proxyCatBundleID = "com.cassette.proxycat"

func validateBundle(appPath string) error {
	infoPlist := filepath.Join(appPath, "Contents", "Info.plist")
	out, err := commandOutput("/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIdentifier", infoPlist)
	if err != nil {
		return fmt.Errorf("更新失败：安装包格式不正确")
	}
	if strings.TrimSpace(string(out)) != proxyCatBundleID {
		return fmt.Errorf("更新失败：安装包格式不正确")
	}
	proxyctl := filepath.Join(appPath, "Contents", "Resources", "proxyctl")
	info, err := os.Stat(proxyctl)
	if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return fmt.Errorf("更新失败：安装包格式不正确")
	}
	return nil
}

func replaceApp(currentApp string, newApp string, backupDir string, oldVersion string) error {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	backupPath := filepath.Join(backupDir, "ProxyCat-"+oldVersion+".app")
	_ = os.RemoveAll(backupPath)
	if err := os.Rename(currentApp, backupPath); err != nil {
		return err
	}
	if err := os.Rename(newApp, currentApp); err != nil {
		_ = os.RemoveAll(currentApp)
		_ = os.Rename(backupPath, currentApp)
		return fmt.Errorf("更新失败，已恢复旧版本。请截图发给我。")
	}
	return nil
}

func commandOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}
	return out, nil
}
