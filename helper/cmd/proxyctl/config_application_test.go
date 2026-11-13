package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cass-ette/ProxyCat/helper/internal/controller"
	"github.com/Cass-ette/ProxyCat/helper/internal/maintenance"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
	"github.com/Cass-ette/ProxyCat/helper/internal/preferences"
	"github.com/Cass-ette/ProxyCat/helper/internal/profile"
)

const applicationConfig = "mixed-port: 7890\nmode: rule\nproxies: [{name: Node, type: direct}]\nproxy-groups: [{name: Proxy, type: select, proxies: [Node]}]\nrules: [\"MATCH,Proxy\"]\n"

func applicationFixture() (applicationActions, *[]string) {
	calls := &[]string{}
	now := "Node"
	a := applicationActions{
		tunSession: func() bool { return false }, running: func() (bool, int, error) { return true, 123, nil },
		owned:  func(int) error { return nil },
		config: func() (*controller.Config, error) { return &controller.Config{Mode: "global", MixedPort: 7890}, nil },
		proxies: func() (map[string]controller.Proxy, error) {
			return map[string]controller.Proxy{"Proxy": {Type: "Selector", Now: now, All: []string{"Node", "Other"}}}, nil
		},
		reload: func(content []byte) error {
			*calls = append(*calls, string(content))
			if !bytes.Contains(content, []byte("mode: global")) {
				return errors.New("mode was lost")
			}
			now = "Other"
			return nil
		},
		selectProxy: func(group, name string) error { now = name; return nil },
		ready: func(mode string) error {
			if mode != "global" {
				return errors.New("wrong mode")
			}
			return nil
		},
	}
	return a, calls
}

func TestConfigApplicationKeepsModeAndSelection(t *testing.T) {
	a, calls := applicationFixture()
	result, err := applyConfiguration([]byte(applicationConfig), []byte(applicationConfig+"ipv6: true\n"), a)
	if err != nil || result.State != "applied" || len(*calls) != 1 {
		t.Fatalf("result=%+v err=%v calls=%v", result, err, *calls)
	}
	proxies, _ := a.proxies()
	if proxies["Proxy"].Now != "Node" {
		t.Fatal("application lost the selected node")
	}
}

func TestConfigApplicationDefersTUNEvenWithUnreadyController(t *testing.T) {
	a, calls := applicationFixture()
	a.tunSession = func() bool { return true }
	a.running = func() (bool, int, error) { t.Fatal("inspected core during TUN handoff"); return false, 0, nil }
	result, err := applyConfiguration(nil, nil, a)
	if err != nil || result.State != "pending" || len(*calls) != 0 {
		t.Fatalf("unsafe TUN application: %+v %v", result, err)
	}
	a, calls = applicationFixture()
	a.config = func() (*controller.Config, error) {
		return &controller.Config{Tun: map[string]interface{}{"enable": true}}, nil
	}
	result, err = applyConfiguration(nil, nil, a)
	if err != nil || result.State != "pending" || len(*calls) != 0 {
		t.Fatalf("unmarked TUN was reloaded: %+v %v", result, err)
	}
}

func TestConfigApplicationDoesNotStartStoppedOrForeignCore(t *testing.T) {
	a, calls := applicationFixture()
	a.running = func() (bool, int, error) { return false, 0, nil }
	result, err := applyConfiguration(nil, nil, a)
	if err != nil || result.State != "stopped" || len(*calls) != 0 {
		t.Fatalf("unexpected stopped state: %+v %v", result, err)
	}
	a.running = func() (bool, int, error) { return true, 456, nil }
	a.owned = func(int) error { return errors.New("foreign core") }
	if _, err := applyConfiguration(nil, nil, a); err == nil || len(*calls) != 0 {
		t.Fatal("foreign core was touched")
	}
}

func TestConfigApplicationRollsBackWhenReadinessOrSelectionFails(t *testing.T) {
	for _, failure := range []string{"reload", "ready", "selection"} {
		t.Run(failure, func(t *testing.T) {
			a, calls := applicationFixture()
			if failure == "reload" {
				original := a.reload
				a.reload = func(b []byte) error {
					err := original(b)
					if len(*calls) == 1 {
						return errors.New("rejected")
					}
					return err
				}
			}
			if failure == "ready" {
				a.ready = func(string) error {
					if len(*calls) == 1 {
						return errors.New("port closed")
					}
					return nil
				}
			}
			if failure == "selection" {
				selectOriginal := a.selectProxy
				a.selectProxy = func(group, name string) error {
					if len(*calls) == 1 {
						return errors.New("cannot restore selection")
					}
					return selectOriginal(group, name)
				}
			}
			result, err := applyConfiguration([]byte(applicationConfig), []byte(applicationConfig+"ipv6: true\n"), a)
			if err == nil || result.State != "rolledBack" || !strings.Contains(err.Error(), "已恢复并确认旧配置") || len(*calls) != 2 || strings.Contains((*calls)[1], "ipv6: true") {
				t.Fatalf("rollback missing: result=%+v err=%v calls=%v", result, err, *calls)
			}
		})
	}
}

func TestConfigApplicationDoesNotCertifyFailedRollback(t *testing.T) {
	a, calls := applicationFixture()
	a.ready = func(string) error { return errors.New("port remains closed") }
	result, err := applyConfiguration([]byte(applicationConfig), []byte(applicationConfig), a)
	if err == nil || result.State != "unknown" || !strings.Contains(err.Error(), "旧配置恢复也未能确认") || len(*calls) != 2 {
		t.Fatalf("false recovery: %+v %v", result, err)
	}
}

func TestFailedApplicationRestoresAllSavedFiles(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	for _, dir := range []string{p.Config, p.ProfilesDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	profilePath := filepath.Join(p.ProfilesDir, "existing.yaml")
	for _, path := range []string{p.ConfigYAML, profilePath} {
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	original := applyConfigurationRuntime
	t.Cleanup(func() { applyConfigurationRuntime = original })
	applyConfigurationRuntime = func(paths.RuntimePaths, []byte, []byte) (configApplication, error) {
		return configApplication{}, errors.New("cannot apply")
	}
	_, err := commitConfiguration(p, map[string][]byte{p.ConfigYAML: []byte("new"), profilePath: []byte("new"), p.PreferencesJSON: []byte("new")})
	if err == nil {
		t.Fatal("application failure was ignored")
	}
	for _, path := range []string{p.ConfigYAML, profilePath} {
		b, err := os.ReadFile(path)
		if err != nil || string(b) != "old" {
			t.Fatalf("old file not restored: %q %v", b, err)
		}
	}
	if _, err := os.Stat(p.PreferencesJSON); !os.IsNotExist(err) {
		t.Fatalf("new preference was left after failure: %v", err)
	}
}

func TestSubscriptionApplicationStatusAndRollbackPreservePreferences(t *testing.T) {
	for _, state := range []string{"applied", "pending", "failed", "rolledBack", "unknown"} {
		t.Run(state, func(t *testing.T) {
			failed := state == "failed" || state == "rolledBack" || state == "unknown"
			t.Setenv("HOME", t.TempDir())
			installAcceptingMihomo(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, applicationConfig) }))
			defer server.Close()
			id, err := saveProfileSubscription(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			p, _ := paths.Default()
			profiles, _ := profile.LoadAll(p.ProfilesDir)
			profiles[0].Active = true
			if err := profile.SaveAll(p.ProfilesDir, profiles); err != nil {
				t.Fatal(err)
			}
			profilePath := profile.ProfileConfigPath(p.ProfilesDir, id)
			for _, path := range []string{p.ConfigYAML, profilePath} {
				if err := os.WriteFile(path, []byte("old config"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := recordConfigApplication(p, configApplication{"applied", "old status"}); err != nil {
				t.Fatal(err)
			}
			prefs := preferences.Store{Preferences: []preferences.Preference{{Host: "example.com", Route: "direct"}}}
			if err := preferences.Save(p.PreferencesJSON, prefs); err != nil {
				t.Fatal(err)
			}
			previousApply := applyConfigurationRuntime
			t.Cleanup(func() { applyConfigurationRuntime = previousApply })
			calls := 0
			applyConfigurationRuntime = func(_ paths.RuntimePaths, previous, next []byte) (configApplication, error) {
				calls++
				if string(previous) != "old config" || !strings.Contains(string(next), "DOMAIN-SUFFIX,example.com,DIRECT") {
					t.Errorf("subscription update lost the old config or user preference: %s / %s", previous, next)
				}
				if state == "failed" {
					return configApplication{}, errors.New("application rejected; old runtime restored")
				}
				if failed {
					return configApplication{state, "application: " + state}, errors.New("application failed")
				}
				return configApplication{state, "application: " + state}, nil
			}
			var stdout, stderr bytes.Buffer
			code := run([]string{"subscription", "update", "--json"}, &stdout, &stderr)
			if calls != 1 || (code == 0) == failed {
				t.Fatalf("bad update result: calls=%d code=%d out=%s err=%s", calls, code, &stdout, &stderr)
			}
			status, err := maintenance.Load(p.MaintenanceJSON)
			if err != nil {
				t.Fatal(err)
			}
			if !failed {
				if status.ConfigApplicationState != state || status.SubscriptionLastSuccess == "" {
					t.Fatalf("saved/applied status lost: %+v", status)
				}
			} else {
				if state != "failed" && (status.ConfigApplicationState != state || status.ConfigSHA256 != configurationHash([]byte("old config"))) {
					t.Fatalf("failed application retained a stale success status: %+v", status)
				}
				if status.SubscriptionLastSuccess != "" || status.SubscriptionLastError == "" {
					t.Fatalf("failed application was marked successful: %+v", status)
				}
				for _, path := range []string{p.ConfigYAML, profilePath} {
					content, _ := os.ReadFile(path)
					if string(content) != "old config" {
						t.Fatalf("failed application changed %s: %s", path, content)
					}
				}
			}
			loaded, err := preferences.Load(p.PreferencesJSON)
			if err != nil || len(loaded.Preferences) != 1 || loaded.Preferences[0].Route != "direct" {
				t.Fatalf("subscription update discarded website preference: %+v %v", loaded, err)
			}
		})
	}
}

func TestChangedConfigRetiresPreviousApplicationStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p, _ := paths.Default()
	if err := os.MkdirAll(p.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ConfigYAML, []byte(applicationConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := recordConfigApplication(p, configApplication{"applied", "old application passed"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ConfigYAML, []byte(applicationConfig+"ipv6: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runMaintenanceStatus(io.Discard, io.Discard, true); code != 0 {
		t.Fatalf("maintenance exit = %d", code)
	}
	status, err := maintenance.Load(p.MaintenanceJSON)
	if err != nil || status.ConfigApplicationState != "unknown" || strings.Contains(status.ConfigApplicationMessage, "passed") {
		t.Fatalf("stale application remained current: %+v %v", status, err)
	}
}

func TestSubscriptionHTTPFailurePreservesSavedStateAndReportsReason(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	installAcceptingMihomo(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	id, err := saveProfileSubscription(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := paths.Default()
	profiles, _ := profile.LoadAll(p.ProfilesDir)
	profiles[0].Active = true
	if err := profile.SaveAll(p.ProfilesDir, profiles); err != nil {
		t.Fatal(err)
	}
	profilePath := profile.ProfileConfigPath(p.ProfilesDir, id)
	if err := os.MkdirAll(filepath.Dir(profilePath), 0o700); err != nil {
		t.Fatal(err)
	}
	saved := map[string][]byte{
		p.ConfigYAML: []byte(applicationConfig), profilePath: []byte(applicationConfig),
		subscriptionBasePath(p.ConfigYAML): []byte(applicationConfig), subscriptionBasePath(profilePath): []byte(applicationConfig),
		p.PreferencesJSON: []byte(`{"preferences":[{"host":"example.com","route":"direct"}]}`),
	}
	for path, content := range saved {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const previousSuccess = "2026-11-01T00:00:00Z"
	if err := maintenance.Save(p.MaintenanceJSON, maintenance.Status{SubscriptionLastSuccess: previousSuccess}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"subscription", "update", "--json"}, &stdout, &stderr); code == 0 || !strings.Contains(stdout.String(), "HTTP 503") || !strings.Contains(stderr.String(), "HTTP 503") {
		t.Fatalf("download failure was not explained: code=%d out=%s err=%s", code, &stdout, &stderr)
	}
	status, err := maintenance.Load(p.MaintenanceJSON)
	if err != nil || status.SubscriptionLastSuccess != previousSuccess || !strings.Contains(status.SubscriptionLastError, "HTTP 503") || status.SubscriptionInProgress {
		t.Fatalf("failed subscription lost its previous success or reason: %+v %v", status, err)
	}
	for path, want := range saved {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(want, got) {
			t.Fatalf("download failure modified %s: %q %v", path, got, err)
		}
	}
}
