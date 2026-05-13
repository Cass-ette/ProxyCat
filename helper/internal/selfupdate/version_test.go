package selfupdate

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestReadBundleVersionFromInfoPlist(t *testing.T) {
	app := fakeAppBundle(t, "com.cassette.proxycat")
	got, err := ReadBundleVersion(app)
	if err != nil {
		t.Fatalf("ReadBundleVersion returned error: %v", err)
	}
	if got != "0.1.0" {
		t.Fatalf("version = %q, want %q", got, "0.1.0")
	}
}

func TestReadBundleVersionMissingPlist(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadBundleVersion(filepath.Join(dir, "NoSuch.app"))
	if err == nil {
		t.Fatal("expected error for missing plist")
	}
}

func TestExtractInstallerZipReturnsAppPath(t *testing.T) {
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

	destDir := t.TempDir()
	appPath, err := extractInstallerZip(zipPath, destDir)
	if err != nil {
		t.Fatalf("extractInstallerZip returned error: %v", err)
	}

	expected := filepath.Join(destDir, "ProxyCat.app")
	if appPath != expected {
		t.Fatalf("appPath = %q, want %q", appPath, expected)
	}
	if _, err := os.Stat(filepath.Join(appPath, "Contents", "Info.plist")); err != nil {
		t.Fatalf("extracted plist missing: %v", err)
	}
}

func TestExtractInstallerZipReturnsNestedInstallerAppPath(t *testing.T) {
	zipDir := t.TempDir()
	appDir := filepath.Join(zipDir, "ProxyCat 安装包", "ProxyCat.app", "Contents", "Resources")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>com.cassette.proxycat</string><key>CFBundleShortVersionString</key><string>0.3.0</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(zipDir, "ProxyCat 安装包", "ProxyCat.app", "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "proxyctl"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(t.TempDir(), "update.zip")
	createZip(t, zipDir, zipPath)

	destDir := t.TempDir()
	appPath, err := extractInstallerZip(zipPath, destDir)
	if err != nil {
		t.Fatalf("extractInstallerZip returned error: %v", err)
	}
	expected := filepath.Join(destDir, "ProxyCat 安装包", "ProxyCat.app")
	if appPath != expected {
		t.Fatalf("appPath = %q, want %q", appPath, expected)
	}
}

func TestExtractInstallerZipRejectsMissingApp(t *testing.T) {
	zipDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(zipDir, "readme.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(t.TempDir(), "update.zip")
	createZip(t, zipDir, zipPath)

	destDir := t.TempDir()
	_, err := extractInstallerZip(zipPath, destDir)
	if err == nil {
		t.Fatal("expected error when zip has no ProxyCat.app")
	}
}

func TestExtractInstallerZipRejectsPathTraversal(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "evil.zip")
	w, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(w)
	if _, err := zw.Create("ProxyCat.app/"); err != nil {
		t.Fatal(err)
	}
	fw, err := zw.Create("../outside.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("owned")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	destDir := t.TempDir()
	_, err = extractInstallerZip(zipPath, destDir)
	if err == nil {
		t.Fatal("expected error for path traversal entry")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(destDir), "outside.txt")); !os.IsNotExist(err) {
		t.Fatalf("path traversal wrote outside extraction dir: %v", err)
	}
}

func createZip(t *testing.T, srcDir string, zipPath string) {
	t.Helper()
	w, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	zw := zip.NewWriter(w)
	defer zw.Close()

	filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			t.Fatal(walkErr)
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			t.Fatal(err)
		}
		if rel == "." {
			return nil
		}
		if info.IsDir() {
			h := &zip.FileHeader{Name: rel + "/"}
			h.SetMode(info.Mode())
			if _, err := zw.CreateHeader(h); err != nil {
				t.Fatal(err)
			}
			return nil
		}
		h := &zip.FileHeader{Name: rel}
		h.SetMode(info.Mode())
		fw, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := io.Copy(fw, f); err != nil {
			t.Fatal(err)
		}
		return nil
	})
}
