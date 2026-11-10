package core

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
	"strings"
	"testing"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/controller"
)

func TestListenerRecoveryDoesNotAcceptControllerOnlyOrAlterTUN(t *testing.T) {
	for _, activeTUN := range []bool{false, true} {
		t.Run(fmt.Sprint(activeTUN), func(t *testing.T) {
			patches := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPatch {
					patches++
					w.WriteHeader(http.StatusNoContent)
					return
				}
				fmt.Fprintf(w, `{"mixed-port":0,"tun":{"enable":%t}}`, activeTUN)
			}))
			defer server.Close()
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			port := listener.Addr().(*net.TCPAddr).Port
			listener.Close()
			client := controller.NewClient(server.URL)
			ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			defer cancel()
			err = waitForOrdinaryListener(ctx, client, port)
			if err == nil || (activeTUN && patches != 0) || (!activeTUN && patches == 0) {
				t.Fatalf("controller-only/TUN accepted: patches %d, error %v", patches, err)
			}
		})
	}
}

// Reproduce the actual failure with a real core, without TUN, system settings,
// live app ports or public network requests. A TCP bind collision can leave
// the process alive but mixed-port zero; releasing the blocker alone does not
// make Mihomo bind again.
func TestMihomoRecoversMixedListenerAfterBindFailure(t *testing.T) {
	binary := os.Getenv("PROXYCAT_TEST_MIHOMO")
	if binary == "" {
		t.Skip("set PROXYCAT_TEST_MIHOMO for isolated real-kernel recovery")
	}
	t.Run("tcp", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		blocker, port := listener, listener.Addr().(*net.TCPAddr).Port
		defer blocker.Close()
		reserved, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address := reserved.Addr().String()
		reserved.Close()
		dir := t.TempDir()
		configPath, logPath := filepath.Join(dir, "config.yaml"), filepath.Join(dir, "mihomo.log")
		config := fmt.Sprintf("mixed-port: %d\nexternal-controller: %s\nbind-address: 127.0.0.1\nallow-lan: false\nmode: direct\nlog-level: warning\nproxies: []\nproxy-groups: []\nrules: ['MATCH,DIRECT']\n", port, address)
		if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
			t.Fatal(err)
		}
		log, err := os.Create(logPath)
		if err != nil {
			t.Fatal(err)
		}
		defer log.Close()
		cmd := exec.Command(binary, "-d", dir, "-f", configPath)
		cmd.Stdout, cmd.Stderr = log, log
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		client := controller.NewClient("http://" + address)
		client.SetClient(&http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Second})
		ready := false
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
			if cfg, err := client.GetConfig(); err == nil && cfg.MixedPort == 0 {
				ready = true
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		if !ready {
			t.Fatal("failed to reproduce alive core with mixed-port zero")
		}
		if err := os.WriteFile(filepath.Join(dir, "pgrep"), []byte(fmt.Sprintf("#!/bin/sh\nprintf '%%d\\n' %d\n", cmd.Process.Pid)), 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
		blocker.Close()
		pid, err := EnsureProxyReady(binary, configPath, logPath)
		if err != nil || pid != cmd.Process.Pid {
			data, _ := os.ReadFile(logPath)
			t.Fatalf("could not repair owned core in place: pid %d error %v\n%s", pid, err, data)
		}
		cfg, err := client.GetConfig()
		if err != nil || cfg.MixedPort != port || cfg.Mode != "direct" || cfg.Tun["enable"] == true {
			t.Fatalf("unexpected repaired config: %+v %v", cfg, err)
		}
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "local recovery works") }))
		defer origin.Close()
		proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
		transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		defer transport.CloseIdleConnections()
		response, err := (&http.Client{Transport: transport, Timeout: time.Second}).Get(origin.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if strings.TrimSpace(string(body)) != "local recovery works" {
			t.Fatalf("recovered port did not serve proxy traffic: %s", body)
		}
	})
}

func TestMihomoReplacesOwnedCoreWhenControllerCannotRepairListener(t *testing.T) {
	binary := os.Getenv("PROXYCAT_TEST_MIHOMO")
	if binary == "" {
		t.Skip("set PROXYCAT_TEST_MIHOMO for isolated core replacement")
	}
	reserve := func() net.Listener {
		t.Helper()
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		return ln
	}
	proxy, api := reserve(), reserve()
	port, address := proxy.Addr().(*net.TCPAddr).Port, api.Addr().String()
	proxy.Close()
	api.Close()
	dir := t.TempDir()
	configPath, oldConfig, logPath := filepath.Join(dir, "config.yaml"), filepath.Join(dir, "old.yaml"), filepath.Join(dir, "mihomo.log")
	// The old normal core's controller rejects the current credential. This
	// exercises the restart fallback without arbitrary command injection or a
	// real system proxy; the new core reads the desired config and credential.
	makeConfig := func(port int, token string) string {
		return fmt.Sprintf("mixed-port: %d\nexternal-controller: %s\nsecret: %s\nbind-address: 127.0.0.1\nallow-lan: false\nmode: direct\nlog-level: warning\nrules: ['MATCH,DIRECT']\n", port, address, token)
	}
	for path, body := range map[string]string{oldConfig: makeConfig(0, "old-fixture-token"), configPath: makeConfig(port, "current-fixture-token")} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	old := exec.Command(binary, "-d", dir, "-f", oldConfig)
	old.Stdout, old.Stderr = log, log
	if err := old.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = old.Wait(); close(done) }()
	t.Cleanup(func() { _ = old.Process.Kill(); <-done })
	oldClient := controller.NewClientWithSecret("http://"+address, "old-fixture-token")
	oldClient.SetClient(&http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Second})
	ready := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if cfg, err := oldClient.GetConfig(); err == nil && cfg.MixedPort == 0 {
			ready = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ready {
		t.Fatal("fixture core did not expose its controller")
	}
	if err := os.WriteFile(filepath.Join(dir, "pgrep"), []byte(fmt.Sprintf("#!/bin/sh\nprintf '%%d\\n' %d\n", old.Process.Pid)), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	newPID, err := EnsureProxyReady(binary, configPath, logPath)
	if newPID > 0 && newPID != old.Process.Pid {
		t.Cleanup(func() { _ = StopGraceful(newPID, time.Second) })
	}
	if err != nil || newPID == old.Process.Pid {
		t.Fatalf("owned core was not replaced: pid %d error %v", newPID, err)
	}
	client := controller.NewClientWithSecret("http://"+address, "current-fixture-token")
	client.SetClient(&http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Second})
	cfg, err := client.GetConfig()
	if err != nil || cfg.MixedPort != port || !ProbePort(port, time.Second) {
		t.Fatalf("replacement has no mixed listener: %+v %v", cfg, err)
	}
	select {
	case <-done:
	default:
		t.Fatal("new core started before old core exit")
	}
}
