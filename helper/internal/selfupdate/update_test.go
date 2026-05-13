package selfupdate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerReportsAlreadyLatest(t *testing.T) {
	var out bytes.Buffer
	runner := Runner{CurrentVersion: "0.2.0", Latest: Release{Version: "0.2.0"}}
	code := runner.Run(&out, false)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out.String(), "已经是最新版") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunnerReportsAlreadyLatestJSON(t *testing.T) {
	var out bytes.Buffer
	runner := Runner{CurrentVersion: "0.2.0", Latest: Release{Version: "0.2.0"}}
	code := runner.Run(&out, true)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out.String(), `"stage":"done"`) || !strings.Contains(out.String(), "已经是最新版") {
		t.Fatalf("json output = %q", out.String())
	}
}

func TestRunnerCheckOnlyReportsAvailableUpdate(t *testing.T) {
	var out bytes.Buffer
	runner := Runner{CurrentVersion: "0.1.0", Latest: Release{Version: "0.2.0"}, CheckOnly: true}
	code := runner.Run(&out, true)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out.String(), `"stage":"done"`) || !strings.Contains(out.String(), "发现新版本 0.2.0") {
		t.Fatalf("json output = %q", out.String())
	}
}

func TestRunnerInstallOrchestration(t *testing.T) {
	// Build a fake installer zip with a valid ProxyCat.app
	zipDir := t.TempDir()
	appDir := filepath.Join(zipDir, "ProxyCat.app", "Contents", "Resources")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>com.cassette.proxycat</string><key>CFBundleShortVersionString</key><string>0.3.0</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(zipDir, "ProxyCat.app", "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "proxyctl"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(t.TempDir(), "update.zip")
	createZip(t, zipDir, zipPath)

	zipBytes, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(zipBytes)
	sha256Hex := hex.EncodeToString(hash[:])

	// Set up test server serving both the installer zip and its sha256 sidecar
	mux := http.NewServeMux()
	mux.HandleFunc("/app.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(zipBytes)
	})
	mux.HandleFunc("/app.zip.sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  ProxyCat-0.3.0-installer.zip", sha256Hex)
	})
	mux.HandleFunc("/api/release", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"tag_name":"v0.3.0","assets":[{"name":"ProxyCat-0.3.0-installer.zip","browser_download_url":"http://%s/app.zip","size":%d},{"name":"ProxyCat-0.3.0-installer.zip.sha256","browser_download_url":"http://%s/app.zip.sha256","size":64}]}`, r.Host, len(zipBytes), r.Host)))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create the "current app" that will be backed up
	currentApp := filepath.Join(t.TempDir(), "ProxyCat.app")
	if err := os.MkdirAll(filepath.Join(currentApp, "Contents", "Resources"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldPlist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>com.cassette.proxycat</string><key>CFBundleShortVersionString</key><string>0.2.0</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(currentApp, "Contents", "Info.plist"), []byte(oldPlist), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(currentApp, "Contents", "Resources", "proxyctl"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	backupDir := filepath.Join(t.TempDir(), "backups")

	var out bytes.Buffer
	runner := Runner{
		CurrentVersion: "0.2.0",
		CheckOnly:      false,
		Client:         server.Client(),
		Endpoint:       server.URL + "/api/release",
		AppPath:        currentApp,
		BackupDir:      backupDir,
	}
	code := runner.Run(&out, true)

	if code != 0 {
		t.Fatalf("exit code = %d, output = %q", code, out.String())
	}

	output := out.String()
	if !strings.Contains(output, `"stage":"done"`) {
		t.Fatalf("missing done stage in output: %q", output)
	}

	// Verify backup was created
	if _, err := os.Stat(filepath.Join(backupDir, "ProxyCat-0.2.0.app")); err != nil {
		t.Fatalf("backup missing: %v", err)
	}

	// Verify the new app is in place
	version, err := ReadBundleVersion(currentApp)
	if err != nil {
		t.Fatalf("ReadBundleVersion on replaced app: %v", err)
	}
	if version != "0.3.0" {
		t.Fatalf("replaced app version = %q, want %q", version, "0.3.0")
	}
}

func TestRunnerInstallEmitsProgressStages(t *testing.T) {
	zipDir := t.TempDir()
	appDir := filepath.Join(zipDir, "ProxyCat.app", "Contents", "Resources")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>com.cassette.proxycat</string><key>CFBundleShortVersionString</key><string>0.3.0</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(zipDir, "ProxyCat.app", "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "proxyctl"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(t.TempDir(), "update.zip")
	createZip(t, zipDir, zipPath)
	zipBytes, _ := os.ReadFile(zipPath)
	hash := sha256.Sum256(zipBytes)
	sha256Hex := hex.EncodeToString(hash[:])

	mux := http.NewServeMux()
	mux.HandleFunc("/app.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipBytes)
	})
	mux.HandleFunc("/app.zip.sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  ProxyCat-0.3.0-installer.zip", sha256Hex)
	})
	mux.HandleFunc("/api/release", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"tag_name":"v0.3.0","assets":[{"name":"ProxyCat-0.3.0-installer.zip","browser_download_url":"http://%s/app.zip","size":%d},{"name":"ProxyCat-0.3.0-installer.zip.sha256","browser_download_url":"http://%s/app.zip.sha256","size":64}]}`, r.Host, len(zipBytes), r.Host)))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	currentApp := filepath.Join(t.TempDir(), "ProxyCat.app")
	os.MkdirAll(filepath.Join(currentApp, "Contents", "Resources"), 0o755)
	os.WriteFile(filepath.Join(currentApp, "Contents", "Info.plist"), []byte(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>com.cassette.proxycat</string><key>CFBundleShortVersionString</key><string>0.2.0</string></dict></plist>`), 0o644)
	os.WriteFile(filepath.Join(currentApp, "Contents", "Resources", "proxyctl"), []byte("#!/bin/sh\n"), 0o755)

	var out bytes.Buffer
	runner := Runner{
		CurrentVersion: "0.2.0",
		Client:         server.Client(),
		Endpoint:       server.URL + "/api/release",
		AppPath:        currentApp,
		BackupDir:      filepath.Join(t.TempDir(), "backups"),
	}
	code := runner.Run(&out, true)
	if code != 0 {
		t.Fatalf("exit code = %d, output = %q", code, out.String())
	}

	// Decode each JSON line and collect stages
	dec := json.NewDecoder(&out)
	var stages []string
	for {
		var evt Event
		if err := dec.Decode(&evt); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("decode event: %v", err)
		}
		stages = append(stages, evt.Stage)
	}

	expected := []string{"checking", "downloading", "verifying", "installing", "done"}
	for i, s := range expected {
		if i >= len(stages) || stages[i] != s {
			t.Fatalf("stage[%d] = %q, want %q; all stages = %v", i, stages, s, stages)
		}
	}
}
