package selfupdate

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchLatestReleaseFindsStrictInstallerAndSHA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name": "v0.2.0",
			"assets": [
				{"name":"ProxyCat-dev-installer.zip","browser_download_url":"https://example.com/dev.zip","size":1},
				{"name":"ProxyCat-0.2.0-installer.zip","browser_download_url":"https://example.com/app.zip","size":100},
				{"name":"ProxyCat-0.2.0-installer.zip.sha256","browser_download_url":"https://example.com/app.zip.sha256","size":64}
			]
		}`))
	}))
	defer server.Close()

	release, err := fetchLatestRelease(server.Client(), server.URL)
	if err != nil {
		t.Fatalf("fetchLatestRelease returned error: %v", err)
	}
	if release.Version != "0.2.0" {
		t.Fatalf("version = %q", release.Version)
	}
	if release.InstallerURL != "https://example.com/app.zip" {
		t.Fatalf("installer URL = %q", release.InstallerURL)
	}
	if release.SHA256URL != "https://example.com/app.zip.sha256" {
		t.Fatalf("sha URL = %q", release.SHA256URL)
	}
}

func TestFetchLatestReleaseRejectsMissingSHA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","assets":[{"name":"ProxyCat-0.2.0-installer.zip","browser_download_url":"https://example.com/app.zip","size":100}]}`))
	}))
	defer server.Close()

	_, err := fetchLatestRelease(server.Client(), server.URL)
	if err == nil {
		t.Fatal("expected missing sha error")
	}
}
