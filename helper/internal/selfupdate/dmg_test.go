package selfupdate

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type dmgFixture struct {
	app, mount, fail string
	commands         []string
}

func mockDMG(t *testing.T, failure string) *dmgFixture {
	t.Helper()
	f := &dmgFixture{app: fakeAppBundle(t, proxyCatBundleID), fail: failure}
	plist := filepath.Join(f.app, "Contents", "Info.plist")
	data, err := os.ReadFile(plist)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plist, bytes.ReplaceAll(data, []byte("0.1.0"), []byte("0.3.0")), 0o644); err != nil {
		t.Fatal(err)
	}
	old := commandOutputFunc
	commandOutputFunc = func(name string, args ...string) ([]byte, error) {
		f.commands = append(f.commands, name+" "+strings.Join(args, " "))
		if name == "/usr/bin/hdiutil" {
			switch args[0] {
			case "attach":
				if f.fail == "attach" {
					return nil, fmt.Errorf("unreadable image")
				}
				for i, arg := range args {
					if arg == "-mountpoint" {
						f.mount = args[i+1]
					}
				}
				if !strings.Contains(strings.Join(args, " "), "-readonly -nobrowse -noautoopen") {
					t.Fatal("must mount read-only without opening Finder or the app")
				}
				if f.fail == "missing app" {
					return nil, nil
				}
				if f.fail == "symlink app" {
					return nil, os.Symlink(f.app, filepath.Join(f.mount, "ProxyCat.app"))
				}
				return commandOutput("/usr/bin/ditto", f.app, filepath.Join(f.mount, "ProxyCat.app"))
			case "detach":
				if f.fail == "detach" {
					return nil, fmt.Errorf("busy image")
				}
				return nil, os.RemoveAll(f.mount)
			}
		}
		if name == "/usr/bin/codesign" {
			if f.fail == "signature" {
				return nil, fmt.Errorf("invalid signature")
			}
			return nil, nil
		}
		if name == "/usr/bin/ditto" && f.fail == "copy" {
			return nil, os.ErrPermission
		}
		return commandOutput(name, args...)
	}
	t.Cleanup(func() {
		commandOutputFunc = old
		if f.mount != "" {
			_ = os.RemoveAll(f.mount)
		}
	})
	return f
}

func TestExtractDMGCopiesAppAndUnmountsOnFailures(t *testing.T) {
	for _, failure := range []string{"", "attach", "missing app", "symlink app", "signature", "copy", "detach"} {
		t.Run("failure="+failure, func(t *testing.T) {
			f := mockDMG(t, failure)
			dest := filepath.Join(t.TempDir(), "extracted")
			app, err := extractInstallerDMG("update.dmg", dest)
			if (err != nil) != (failure != "") {
				t.Fatalf("app=%q error=%v", app, err)
			}
			if failure == "" {
				if _, err := os.Stat(filepath.Join(app, "Contents", "Resources", "proxyctl")); err != nil {
					t.Fatal(err)
				}
			}
			if f.mount != "" && failure != "detach" {
				if _, err := os.Stat(f.mount); !os.IsNotExist(err) {
					t.Fatalf("mount not cleaned up: %v", err)
				}
			}
			if failure == "detach" {
				if _, err := os.Stat(filepath.Join(f.mount, "ProxyCat.app")); err != nil {
					t.Fatal("must not remove files from a still-mounted volume")
				}
			}
		})
	}
}

func TestRunnerInstallsDMGOnlyAfterChecksumVerification(t *testing.T) {
	for _, validChecksum := range []bool{true, false} {
		t.Run(fmt.Sprint(validChecksum), func(t *testing.T) {
			f := mockDMG(t, "")
			image := []byte("test disk image")
			hash := sha256.Sum256(image)
			if !validChecksum {
				hash = sha256.Sum256([]byte("different image"))
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/app.dmg" {
					_, _ = w.Write(image)
					return
				}
				fmt.Fprintf(w, "%x  ProxyCat-0.3.0.dmg\n", hash)
			}))
			defer server.Close()
			current := fakeAppBundle(t, proxyCatBundleID)
			backup := filepath.Join(t.TempDir(), "backups")
			runner := Runner{CurrentVersion: "0.1.0", Client: server.Client(), AppPath: current, BackupDir: backup,
				Latest: Release{Version: "0.3.0", Format: "dmg", InstallerURL: server.URL + "/app.dmg", SHA256URL: server.URL + "/app.dmg.sha256"}}
			var output bytes.Buffer
			code := runner.Run(&output, true)
			if validChecksum {
				if code != 0 {
					t.Fatalf("update failed: %s", &output)
				}
				version, err := ReadBundleVersion(current)
				if err != nil || version != "0.3.0" {
					t.Fatalf("version=%q error=%v", version, err)
				}
				if _, err := os.Stat(filepath.Join(backup, "ProxyCat-0.1.0.app")); err != nil {
					t.Fatal(err)
				}
			} else {
				if code == 0 || f.mount != "" {
					t.Fatal("checksum failure must not mount or install the DMG")
				}
				version, err := ReadBundleVersion(current)
				if err != nil || version != "0.1.0" {
					t.Fatal("checksum failure changed the installed app")
				}
			}
		})
	}
}

func TestRunnerRejectsDMGWithWrongBundleVersion(t *testing.T) {
	f := mockDMG(t, "")
	image := []byte("test disk image")
	hash := sha256.Sum256(image)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app.dmg" {
			_, _ = w.Write(image)
			return
		}
		fmt.Fprintf(w, "%x  ProxyCat-0.4.0.dmg\n", hash)
	}))
	defer server.Close()
	current := fakeAppBundle(t, proxyCatBundleID)
	runner := Runner{CurrentVersion: "0.1.0", Client: server.Client(), AppPath: current, BackupDir: filepath.Join(t.TempDir(), "backups"),
		Latest: Release{Version: "0.4.0", Format: "dmg", InstallerURL: server.URL + "/app.dmg", SHA256URL: server.URL + "/app.dmg.sha256"}}
	var output bytes.Buffer
	if runner.Run(&output, true) == 0 || !strings.Contains(output.String(), "应用版本与发布版本不一致") {
		t.Fatalf("must reject wrong app version: %s", &output)
	}
	if f.mount == "" {
		t.Fatal("test must validate the mounted app")
	}
	version, err := ReadBundleVersion(current)
	if err != nil || version != "0.1.0" {
		t.Fatal("version mismatch changed installed app")
	}
}

func TestInstallRealDMG(t *testing.T) {
	image := os.Getenv("PROXYCAT_TEST_DMG")
	if image == "" {
		t.Skip("set PROXYCAT_TEST_DMG to test a real release image")
	}
	data, err := os.ReadFile(image)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	version := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(image), "ProxyCat-"), ".dmg")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app.dmg" {
			_, _ = w.Write(data)
			return
		}
		fmt.Fprintf(w, "%x  ProxyCat-%s.dmg\n", hash, version)
	}))
	defer server.Close()
	current := fakeAppBundle(t, proxyCatBundleID)
	backup := filepath.Join(t.TempDir(), "backups")
	runner := Runner{CurrentVersion: "0.1.0", Client: server.Client(), AppPath: current, BackupDir: backup,
		Latest: Release{Version: version, Format: "dmg", InstallerURL: server.URL + "/app.dmg", SHA256URL: server.URL + "/app.dmg.sha256"}}
	var output bytes.Buffer
	if code := runner.Run(&output, true); code != 0 {
		t.Fatalf("real DMG update failed: %s", &output)
	}
	installed, err := ReadBundleVersion(current)
	if err != nil || installed != version {
		t.Fatalf("installed version=%q error=%v", installed, err)
	}
	if _, err := commandOutput("/usr/bin/codesign", "--verify", "--deep", "--strict", current); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(backup, "ProxyCat-0.1.0.app")); err != nil {
		t.Fatal(err)
	}
}
