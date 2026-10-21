package selfupdate

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchLatestReleasePrefersMatchedDMGAndSupportsLegacyZIP(t *testing.T) {
	for _, tc := range []struct {
		name, assets, format, url string
		wantError                 bool
	}{
		{"dmg only", `{"name":"ProxyCat-0.3.0.dmg","browser_download_url":"/app.dmg"},{"name":"ProxyCat-0.3.0.dmg.sha256","browser_download_url":"/app.dmg.sha256"}`, "dmg", "/app.dmg", false},
		{"both formats", `{"name":"ProxyCat-0.3.0-installer.zip","browser_download_url":"/app.zip"},{"name":"ProxyCat-0.3.0-installer.zip.sha256","browser_download_url":"/app.zip.sha256"},{"name":"ProxyCat-0.3.0.dmg","browser_download_url":"/app.dmg"},{"name":"ProxyCat-0.3.0.dmg.sha256","browser_download_url":"/app.dmg.sha256"}`, "dmg", "/app.dmg", false},
		{"legacy zip", `{"name":"ProxyCat-0.3.0-installer.zip","browser_download_url":"/app.zip"},{"name":"ProxyCat-0.3.0-installer.zip.sha256","browser_download_url":"/app.zip.sha256"}`, "zip", "/app.zip", false},
		{"incomplete dmg falls back", `{"name":"ProxyCat-0.3.0.dmg","browser_download_url":"/app.dmg"},{"name":"ProxyCat-0.3.0-installer.zip","browser_download_url":"/app.zip"},{"name":"ProxyCat-0.3.0-installer.zip.sha256","browser_download_url":"/app.zip.sha256"}`, "zip", "/app.zip", false},
		{"mixed checksums", `{"name":"ProxyCat-0.3.0.dmg","browser_download_url":"/app.dmg"},{"name":"ProxyCat-0.3.0-installer.zip.sha256","browser_download_url":"/app.zip.sha256"}`, "", "", true},
		{"wrong version", `{"name":"ProxyCat-0.2.0.dmg","browser_download_url":"/app.dmg"},{"name":"ProxyCat-0.2.0.dmg.sha256","browser_download_url":"/app.dmg.sha256"}`, "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"tag_name":"v0.3.0","assets":[%s]}`, tc.assets)
			}))
			defer server.Close()
			release, err := fetchLatestRelease(server.Client(), server.URL)
			if tc.wantError {
				if err == nil {
					t.Fatal("expected an unmatched installer/checksum error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if release.Format != tc.format || release.InstallerURL != tc.url || release.SHA256URL != tc.url+".sha256" {
				t.Fatalf("selected release = %+v", release)
			}
		})
	}
}

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

func TestFetchLatestReleaseReportsNoRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, err := fetchLatestRelease(server.Client(), server.URL)
	if err == nil {
		t.Fatal("expected no release error")
	}
	if err.Error() != "还没有可用更新" {
		t.Fatalf("error = %q", err.Error())
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
