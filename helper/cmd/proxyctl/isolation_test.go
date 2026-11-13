package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/diagnose"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
)

// CLI tests must never stop the developer's core or change their system proxy.
// Keep runtime files private, replace OS mutations, and deny the live ports.
func TestMain(m *testing.M) {
	// Runtime diagnosis has its own fully injected tests; CLI shape tests must
	// not dial the live 7890 port or inspect the user's running core.
	diagnoseReport = func(_ context.Context, p paths.RuntimePaths) diagnose.Report { return diagnose.RunStatic(p) }
	// Subprocess watchdog fixtures keep every temporary HOME under the parent
	// test directory, including the one whose parent is intentionally SIGKILLed.
	directory, err := os.MkdirTemp(os.Getenv("PROXYCAT_TEST_WATCH_BASE"), "proxycat-cli-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	commands := map[string]string{
		"pgrep":        "exit 1\n",
		"networksetup": "if [ \"$1\" = -listallnetworkservices ]; then printf 'An asterisk denotes a disabled service.\\nTest Wi-Fi\\n'; fi\n",
		"scutil":       "printf '<dictionary> {\\n HTTPEnable : 0\\n HTTPSEnable : 0\\n SOCKSEnable : 0\\n}\\n'\n",
	}
	for name, script := range commands {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("#!/bin/sh\n"+script), 0700); err != nil {
			os.RemoveAll(directory)
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	os.Setenv("HOME", directory)
	os.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Never leave a detached watchdog behind: it would survive the test binary
	// and keep running against a HOME that gets deleted below.
	os.Setenv("PROXYCAT_NO_WATCHDOG", "1")
	// Direct TCP readiness probes must not depend on the developer's listener.
	systemProxyPortOpen = func(int, time.Duration) bool { return false }
	systemProxyWaitPort = func(int, time.Duration, time.Duration) bool { return false }
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || (host != "127.0.0.1" && host != "::1" && host != "localhost") || port == "7890" || port == "9090" {
			return nil, fmt.Errorf("connection blocked by CLI test isolation: %s", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	http.DefaultTransport = transport
	code := m.Run()
	transport.CloseIdleConnections()
	os.RemoveAll(directory)
	os.Exit(code)
}

func TestLifecycleCommandsShareLockWithoutBlockingSupervisor(t *testing.T) {
	for _, args := range [][]string{{"bootstrap", "https://example.invalid"}, {"subscription", "add", "https://example.invalid"}, {"subscription", "update"}, {"preferences", "set", "proxy", "example.invalid"}, {"mode", "set", "rule"}, {"groups", "select", "Proxy", "Node"}, {"config", "generate"}, {"profile", "activate", "test"}, {"profile", "delete", "test"}, {"tun", "on"}, {"tun", "off"}, {"core", "start"}, {"core", "stop"}, {"core", "restart"}, {"system-proxy", "on"}, {"system-proxy", "off"}} {
		if !needsLifecycleLock(args) {
			t.Fatalf("unserialized lifecycle command: %v", args)
		}
	}
	for _, args := range [][]string{{"tun", "serve", "/tmp/session"}, {"core", "status"}, {"mode", "status"}, {"system-proxy", "status"}} {
		if needsLifecycleLock(args) {
			t.Fatalf("readiness or status command could block handoff: %v", args)
		}
	}
}
