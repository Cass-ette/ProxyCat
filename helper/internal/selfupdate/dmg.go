package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
)

// Copy only ProxyCat.app from a verified installer into a writable staging
// directory. Never execute anything on the image or move from a read-only volume.
func extractInstallerDMG(dmgPath, destDir string) (appPath string, err error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	// Keep the mount outside the download directory: its caller cleans that
	// directory recursively even when unmounting fails.
	mountPath, err := os.MkdirTemp("", "proxycat-mount-*")
	if err != nil {
		return "", err
	}
	if _, err := commandOutputFunc("/usr/bin/hdiutil", "attach", "-readonly", "-nobrowse", "-noautoopen", "-mountpoint", mountPath, dmgPath); err != nil {
		_ = os.Remove(mountPath)
		return "", fmt.Errorf("更新失败：无法打开 DMG 安装包：%w", err)
	}
	defer func() {
		if _, detachErr := commandOutputFunc("/usr/bin/hdiutil", "detach", mountPath); detachErr != nil {
			// Do not remove a directory while a filesystem is still mounted there.
			err = fmt.Errorf("更新失败：无法卸载 DMG 安装盘：%w", detachErr)
			return
		}
		_ = os.Remove(mountPath)
	}()

	source := filepath.Join(mountPath, "ProxyCat.app")
	info, err := os.Lstat(source)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("更新失败：安装包中没有有效的 ProxyCat.app")
	}
	if err := validateBundle(source); err != nil {
		return "", err
	}
	if _, err := commandOutputFunc("/usr/bin/codesign", "--verify", "--deep", "--strict", source); err != nil {
		return "", fmt.Errorf("更新失败：应用签名校验未通过：%w", err)
	}
	appPath = filepath.Join(destDir, "ProxyCat.app")
	if _, err := commandOutputFunc("/usr/bin/ditto", source, appPath); err != nil {
		return "", fmt.Errorf("更新失败：无法复制 DMG 中的应用：%w", err)
	}
	return appPath, nil
}
