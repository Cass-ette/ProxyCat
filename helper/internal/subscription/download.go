package subscription

import (
	"fmt"
	"io"
	"net/http"
)

const maxDownloadSize = 10 * 1024 * 1024 // 10MB limit

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func Download(client HTTPClient, url string, userAgent string) ([]byte, error) {
	return downloadWithClient(client, url, userAgent)
}

func downloadWithClient(client HTTPClient, url string, userAgent string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, maxDownloadSize+1)
	content, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if len(content) > maxDownloadSize {
		return nil, fmt.Errorf("download: response exceeds size limit")
	}

	return content, nil
}
