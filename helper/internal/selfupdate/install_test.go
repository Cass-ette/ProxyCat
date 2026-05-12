package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateBundleAcceptsProxyCatApp(t *testing.T) {
	app := fakeAppBundle(t, "com.cassette.proxycat")
	if err := validateBundle(app); err != nil {
		t.Fatalf("validateBundle returned error: %v", err)
	}
}

func TestValidateBundleRejectsWrongBundleID(t *testing.T) {
	app := fakeAppBundle(t, "com.example.other")
	if err := validateBundle(app); err == nil {
		t.Fatal("expected error")
	}
}

func TestReplaceAppBacksUpOldApp(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "Applications", "ProxyCat.app")
	backupDir := filepath.Join(root, "backups")
	newApp := fakeAppBundle(t, "com.cassette.proxycat")
	if err := os.MkdirAll(filepath.Dir(current), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(current, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(current, "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := replaceApp(current, newApp, backupDir, "0.1.0"); err != nil {
		t.Fatalf("replaceApp returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(backupDir, "ProxyCat-0.1.0.app", "old.txt")); err != nil {
		t.Fatalf("backup missing old file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(current, "Contents", "Info.plist")); err != nil {
		t.Fatalf("new app missing: %v", err)
	}
}

func fakeAppBundle(t *testing.T, bundleID string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "ProxyCat.app")
	contents := filepath.Join(root, "Contents")
	resources := filepath.Join(contents, "Resources")
	if err := os.MkdirAll(resources, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>` + bundleID + `</string><key>CFBundleShortVersionString</key><string>0.1.0</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	proxyctl := filepath.Join(resources, "proxyctl")
	if err := os.WriteFile(proxyctl, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}
