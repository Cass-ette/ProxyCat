package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

var sha256Pattern = regexp.MustCompile(`(?i)\b([a-f0-9]{64})\b`)

type progressFunc func(percent int)

func parseSHA256Sidecar(content []byte) (string, error) {
	match := sha256Pattern.FindStringSubmatch(string(content))
	if match == nil {
		return "", fmt.Errorf("更新包校验失败，请截图发给我")
	}
	return strings.ToLower(match[1]), nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func verifyFileSHA256(path string, expected string) error {
	actual, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if actual != strings.ToLower(expected) {
		return fmt.Errorf("更新包校验失败，请截图发给我")
	}
	return nil
}

func downloadFile(client *http.Client, url string, dest string, progress progressFunc) error {
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("下载失败，请检查网络后重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败，请检查网络后重试")
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}
	if progress != nil {
		progress(100)
	}
	return nil
}
