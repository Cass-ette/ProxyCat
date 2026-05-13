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

func TestReplaceAppClearsQuarantine(t *testing.T) {
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

	var gotName string
	var gotArgs []string
	oldCommandOutput := commandOutputFunc
	commandOutputFunc = func(name string, args ...string) ([]byte, error) {
		gotName = name
		gotArgs = args
		return []byte{}, nil
	}
	defer func() { commandOutputFunc = oldCommandOutput }()

	if err := replaceApp(current, newApp, backupDir, "0.1.0"); err != nil {
		t.Fatalf("replaceApp returned error: %v", err)
	}
	if gotName != "xattr" || len(gotArgs) != 2 || gotArgs[0] != "-cr" || gotArgs[1] != current {
		t.Fatalf("clear quarantine command = %s %v", gotName, gotArgs)
	}
}

func TestReplaceAppKeepsOnlyLatestBackup(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "Applications", "ProxyCat.app")
	backupDir := filepath.Join(root, "backups")
	oldBackup := filepath.Join(backupDir, "ProxyCat-0.0.9.app")
	newApp := fakeAppBundle(t, "com.cassette.proxycat")
	if err := os.MkdirAll(filepath.Dir(current), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(current, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(oldBackup, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceApp(current, newApp, backupDir, "0.1.0"); err != nil {
		t.Fatalf("replaceApp returned error: %v", err)
	}
	if _, err := os.Stat(oldBackup); !os.IsNotExist(err) {
		t.Fatalf("old backup still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(backupDir, "ProxyCat-0.1.0.app")); err != nil {
		t.Fatalf("latest backup missing: %v", err)
	}
}

func TestReplaceAppRollsBackWhenNewAppMoveFails(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "Applications", "ProxyCat.app")
	backupDir := filepath.Join(root, "backups")
	missingNewApp := filepath.Join(root, "missing", "ProxyCat.app")
	if err := os.MkdirAll(filepath.Dir(current), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(current, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(current, "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := replaceApp(current, missingNewApp, backupDir, "0.1.0")
	if err == nil {
		t.Fatal("expected replaceApp error")
	}
	if _, statErr := os.Stat(filepath.Join(current, "old.txt")); statErr != nil {
		t.Fatalf("old app was not restored: %v", statErr)
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
