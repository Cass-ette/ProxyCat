package selfupdate

import (
	"fmt"
	"path/filepath"
)

func ReadBundleVersion(appPath string) (string, error) {
	infoPlist := filepath.Join(appPath, "Contents", "Info.plist")
	out, err := commandOutput("/usr/libexec/PlistBuddy", "-c", "Print :CFBundleShortVersionString", infoPlist)
	if err != nil {
		return "", fmt.Errorf("读取版本失败")
	}
	return trimNewline(string(out)), nil
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
