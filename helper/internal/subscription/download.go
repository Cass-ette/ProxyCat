package subscription

import (
	"fmt"
	"io"
	"net/http"
)

const maxDownloadSize = 10 * 1024 * 1024 // 10MB limit

// HTTPClient is the interface for performing HTTP requests.
// Implemented by *http.Client in production and test mocks in tests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Download fetches the subscription content from url using the provided HTTP client.
// Returns an error if the response status is not 200 or if the content exceeds 10MB.
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

	content, err := readAllLimited(resp.Body, maxDownloadSize)
	if err != nil {
		return nil, err
	}

	return content, nil
}

func readAllLimited(reader io.Reader, maxSize int64) ([]byte, error) {
	limited := io.LimitReader(reader, maxSize+1)
	content, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(content)) > maxSize {
		return nil, fmt.Errorf("download: response exceeds size limit")
	}
	return content, nil
}
