package tun

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/controller"
	"gopkg.in/yaml.v3"
)

func TestConfigurationPreservesRoutingAndDNS(t *testing.T) {
	input := []byte(`mode: rule
external-controller: 0.0.0.0:9090
external-controller-unix: /tmp/unsafe.sock
secret: old
dns:
  nameserver: [https://example.com/dns-query]
proxy-groups:
  - name: Main
    type: select
    proxies: [A, B, DIRECT]
rules: ["DOMAIN,example.cn,DIRECT", "MATCH,Main"]
`)
	out, err := configuration(input, strings.Repeat("a", 64), "global", map[string]string{"Main": "B"})
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["mode"] != "global" || cfg["secret"] != strings.Repeat("a", 64) || cfg["external-controller"] != "127.0.0.1:9090" || cfg["external-controller-unix"] != nil {
		t.Fatalf("unsafe controller or mode: %v", cfg)
	}
	dns := cfg["dns"].(map[string]interface{})
	if dns["enable"] != true || dns["nameserver"].([]interface{})[0] != "https://example.com/dns-query" {
		t.Fatalf("lost DNS settings: %v", dns)
	}
	if cfg["rules"].([]interface{})[0] != "DOMAIN,example.cn,DIRECT" {
		t.Fatal("routing rules were changed")
	}
	group := cfg["proxy-groups"].([]interface{})[0].(map[string]interface{})
	if group["proxies"].([]interface{})[0] != "B" {
		t.Fatal("live selection was lost")
	}
	if cfg["tun"].(map[string]interface{})["auto-route"] != true {
		t.Fatal("TUN routing is not enabled")
	}
	if _, err := configuration([]byte("[]"), "token", "", nil); err == nil {
		t.Fatal("invalid configuration accepted")
	}
}

func startSupervisor(t *testing.T, ready, owner func() bool) (session, <-chan error, *exec.Cmd) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "pctun-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	s := session{Directory: dir, Token: strings.Repeat("a", 64), UID: os.Getuid(), GID: os.Getgid()}
	cmd := exec.Command("/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- supervise(s, cmd, ready, owner) }()
	t.Cleanup(func() { _ = request(s, http.MethodPost, "/stop"); _ = cmd.Process.Kill() })
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if _, err := os.Stat(filepath.Join(dir, "control.sock")); err == nil {
			return s, done, cmd
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("supervisor socket did not appear")
	return s, done, cmd
}

func TestSupervisorRequiresTokenAndWaitsForChildOnStop(t *testing.T) {
	s, done, cmd := startSupervisor(t, func() bool { return true }, func() bool { return true })
	wrong := s
	wrong.Token = "wrong"
	if request(wrong, http.MethodPost, "/stop") == nil {
		t.Fatal("unauthorized stop accepted")
	}
	if err := request(s, http.MethodGet, "/status"); err != nil {
		t.Fatal(err)
	}
	if err := request(s, http.MethodPost, "/stop"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor failed to stop")
	}
	if cmd.ProcessState == nil {
		t.Fatal("child was not reaped")
	}
}

func TestSupervisorDoesNotReportUnreadyTunnel(t *testing.T) {
	s, _, _ := startSupervisor(t, func() bool { return false }, func() bool { return true })
	if request(s, http.MethodGet, "/status") == nil {
		t.Fatal("unready tunnel reported active")
	}
	if err := request(s, http.MethodPost, "/stop"); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorStopsWhenOwnerDisappears(t *testing.T) {
	var owner atomic.Bool
	owner.Store(true)
	_, done, cmd := startSupervisor(t, func() bool { return true }, owner.Load)
	owner.Store(false)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("owner exit left child running")
	}
	if cmd.ProcessState == nil {
		t.Fatal("child was not reaped")
	}
}

func TestShellQuotingDoesNotExecutePaths(t *testing.T) {
	value := "space ' quote $(printf unsafe) `printf unsafe`"
	out, err := exec.Command("/bin/sh", "-c", "printf '%s' "+shellQuote(value)).Output()
	if err != nil || string(out) != value {
		t.Fatalf("quote failed: %q %v", out, err)
	}
}

func TestMissingInterfaceNeverReportsReady(t *testing.T) {
	if Ready(nil) || Ready(&controller.Config{Tun: map[string]interface{}{"enable": true, "device": "utun999999"}}) {
		t.Fatal("missing interface was reported as ready")
	}
}

func TestCancelledAuthorizationCannotStartPrivilegedCore(t *testing.T) {
	if err := awaitStart(t.TempDir(), func() bool { return false }); err == nil {
		t.Fatal("cancelled session was allowed to launch")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "start"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := awaitStart(dir, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
}

func TestPrivilegedOutputRejectsSymlinksAndOtherOwners(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output")
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if file, err := openOwnedFile(link, os.Getuid(), os.O_WRONLY); err == nil {
		file.Close()
		t.Fatal("symlink accepted")
	}
	if file, err := openOwnedFile(path, os.Getuid()+1, os.O_WRONLY); err == nil {
		file.Close()
		t.Fatal("different owner accepted")
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "keep" {
		t.Fatal("rejected output changed file contents")
	}
}

func TestConfigurationAcceptedByMihomo(t *testing.T) {
	binary := os.Getenv("PROXYCAT_TEST_MIHOMO")
	if binary == "" {
		t.Skip("set PROXYCAT_TEST_MIHOMO for kernel configuration validation")
	}
	dir := t.TempDir()
	input := []byte("proxies: [{name: Node, type: http, server: 127.0.0.1, port: 9}]\nproxy-groups: [{name: Main, type: select, proxies: [Node]}]\nrules: ['MATCH,Main']\n")
	b, err := configuration(input, strings.Repeat("a", 64), "rule", nil)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(binary, "-t", "-d", dir, "-f", p).CombinedOutput(); err != nil {
		t.Fatalf("kernel rejected TUN config: %s %v", output, err)
	}
}
