package selfupdate

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const validReleaseManifest = `{"tag_name":"v0.12.4","body":"## 修复\n- 更新检查","assets":[{"name":"ProxyCat-0.12.4.dmg","browser_download_url":"/app.dmg"},{"name":"ProxyCat-0.12.4.dmg.sha256","browser_download_url":"/app.dmg.sha256"}]}`

func TestManifestCheckDoesNotConsumeAPIQuota(t *testing.T) {
	apiCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/update.json" {
			fmt.Fprint(w, validReleaseManifest)
			return
		}
		apiCalls++
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	var out bytes.Buffer
	runner := Runner{CurrentVersion: "0.12.3", CheckOnly: true, Client: server.Client(),
		Endpoint: server.URL + "/update.json", FallbackEndpoint: server.URL + "/api/latest"}
	if code := runner.Run(&out, true); code != 0 || !strings.Contains(out.String(), `"newVersion":"0.12.4"`) || !strings.Contains(out.String(), "更新检查") {
		t.Fatalf("manifest check failed: %s", &out)
	}
	if apiCalls != 0 {
		t.Fatalf("used anonymous API quota %d times", apiCalls)
	}
}

func TestReleaseSourceFallback(t *testing.T) {
	for _, status := range []int{403, 404, 429, 500, 200} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.URL.Path)
				if r.URL.Path == "/primary" {
					w.WriteHeader(status)
					fmt.Fprint(w, "invalid metadata")
					return
				}
				fmt.Fprint(w, validReleaseManifest)
			}))
			defer server.Close()
			release, err := fetchReleaseWithFallback(server.Client(), server.URL+"/primary", server.URL+"/fallback")
			if err != nil || release.Version != "0.12.4" || strings.Join(calls, ",") != "/primary,/fallback" {
				t.Fatalf("release = %+v, error = %v, calls = %v", release, err, calls)
			}
		})
	}
}

func TestUnavailableSourcesReportRateLimit(t *testing.T) {
	for _, status := range []int{403, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/manifest" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.WriteHeader(status)
			}))
			defer server.Close()
			_, err := fetchReleaseWithFallback(server.Client(), server.URL+"/manifest", server.URL+"/api")
			if err == nil || !strings.Contains(err.Error(), "限额") {
				t.Fatalf("expected actionable rate limit, got %v", err)
			}
		})
	}
}

func TestReleaseSourceFallbackAfterNetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, validReleaseManifest)
	}))
	defer server.Close()
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	release, err := fetchReleaseWithFallback(server.Client(), closed.URL, server.URL)
	if err != nil || release.Version != "0.12.4" {
		t.Fatalf("release = %+v, error = %v", release, err)
	}
}

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
			"body": "## 修复\n- 连接测试",
			"html_url": "https://github.com/Cass-ette/ProxyCat/releases/tag/v0.2.0",
			"published_at": "2026-10-26T08:00:00Z",
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
	if release.Notes != "## 修复\n- 连接测试" || release.URL != "https://github.com/Cass-ette/ProxyCat/releases/tag/v0.2.0" || release.PublishedAt != "2026-10-26T08:00:00Z" {
		t.Fatalf("release notes metadata lost: %+v", release)
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
