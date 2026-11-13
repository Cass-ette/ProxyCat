package controller

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/preferences"
)

// Opt in with an installed engine path. A renamed copy, random listeners and
// private data keep the developer's ordinary/TUN core and routes untouched.
func TestReloadConfigWithIsolatedMihomo(t *testing.T) {
	binary := os.Getenv("PROXYCAT_TEST_MIHOMO")
	if binary == "" {
		t.Skip("set PROXYCAT_TEST_MIHOMO for the isolated engine integration")
	}
	dir := t.TempDir()
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	isolated := filepath.Join(dir, "proxycat-test-core")
	if err := os.WriteFile(isolated, data, 0o700); err != nil {
		t.Fatal(err)
	}
	listeners := []net.Listener{}
	for range 2 {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		listeners = append(listeners, listener)
		defer listener.Close()
	}
	proxyPort := listeners[0].Addr().(*net.TCPAddr).Port
	api := "http://" + listeners[1].Addr().String()
	configuration := func(node string) []byte {
		return []byte(fmt.Sprintf("mixed-port: %d\nexternal-controller: %s\nmode: rule\nlog-level: silent\nallow-lan: false\ngeo-auto-update: false\ntun: {enable: false}\ndns: {enable: false}\nhosts: {acceptance.proxycat.test: 127.0.0.1}\nproxies: [{name: %s, type: direct}]\nproxy-groups: [{name: Proxy, type: select, proxies: [%s]}]\nrules: [\"MATCH,Proxy\"]\n", proxyPort, listeners[1].Addr().String(), node, node))
	}
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, configuration("Before"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, listener := range listeners {
		listener.Close()
	}
	log, err := os.Create(filepath.Join(dir, "engine.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	command := exec.Command(isolated, "-d", dir, "-f", configPath)
	command.Stdout, command.Stderr = log, log
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	c := NewClient(api)
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	c.SetClient(&http.Client{Transport: transport, Timeout: time.Second})
	for deadline := time.Now().Add(8 * time.Second); ; {
		if cfg, err := c.GetConfig(); err == nil && cfg.MixedPort == proxyPort {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("isolated engine did not become ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := c.ReloadConfig(configuration("After")); err != nil {
		t.Fatal(err)
	}
	proxies, err := c.GetProxies()
	if err != nil || proxies["Proxy"].Now != "After" {
		t.Fatalf("new subscription not applied: %+v %v", proxies, err)
	}
	if _, exists := proxies["Before"]; exists {
		t.Fatal("old subscription node survived reload")
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "isolated OK") }))
	defer target.Close()
	proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
	proxyTransport := &http.Transport{Proxy: http.ProxyURL(proxy)}
	defer proxyTransport.CloseIdleConnections()
	response, err := (&http.Client{Transport: proxyTransport, Timeout: 2 * time.Second}).Get(target.URL)
	if err != nil {
		t.Fatalf("reloaded mixed listener failed: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "isolated OK" {
		t.Fatalf("new config did not forward local request: %q %v", body, err)
	}

	// Exercise persisted website preferences with real connection evidence. All
	// traffic stays on loopback; neither the live app nor 7890/9090 is touched.
	targetURL, _ := url.Parse(target.URL)
	website := "http://acceptance.proxycat.test:" + targetURL.Port()
	storePath := filepath.Join(dir, "preferences.json")
	store := preferences.Store{}
	if err := preferences.Upsert(&store, website, "direct"); err != nil {
		t.Fatal(err)
	}
	if err := preferences.Save(storePath, store); err != nil {
		t.Fatal(err)
	}
	store, err = preferences.Load(storePath) // Same disk read after a relaunch.
	if err != nil || len(store.Preferences) != 1 {
		t.Fatalf("saved preference did not survive reload: %+v %v", store, err)
	}
	applyAndCheck := func(base []byte, wantRule, wantRoute string) {
		t.Helper()
		effective, err := preferences.ApplyToYAML(base, store)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.ReloadConfig([]byte(effective)); err != nil {
			t.Fatal(err)
		}
		result, err := c.CheckWebsite(context.Background(), website)
		if err != nil || result == nil || result.Outcome != "responded" || result.StatusCode != 200 || result.Rule != wantRule || result.Route != wantRoute {
			t.Fatalf("preference route not verified: result=%+v err=%v want=%s/%s", result, err, wantRule, wantRoute)
		}
		t.Logf("HTTP %d; rule=%s payload=%s route=%s chains=%v", result.StatusCode, result.Rule, result.RulePayload, result.Route, result.Chains)
	}
	applyAndCheck(configuration("After"), "DomainSuffix", "direct")
	// Refreshing subscription rules must retain the independently saved choice.
	applyAndCheck(configuration("Refreshed"), "DomainSuffix", "direct")
	if err := preferences.Upsert(&store, website, "proxy"); err != nil {
		t.Fatal(err)
	}
	applyAndCheck(configuration("Refreshed"), "DomainSuffix", "proxy")
	if _, err := preferences.Remove(&store, website); err != nil {
		t.Fatal(err)
	}
	if err := preferences.Save(storePath, store); err != nil {
		t.Fatal(err)
	}
	store, err = preferences.Load(storePath)
	if err != nil || len(store.Preferences) != 0 {
		t.Fatalf("removed preference returned on reload: %+v %v", store, err)
	}
	applyAndCheck(configuration("Refreshed"), "Match", "proxy")
}
