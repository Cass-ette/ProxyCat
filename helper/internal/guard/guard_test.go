package guard

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/core"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
	"github.com/Cass-ette/ProxyCat/helper/internal/sysproxy"
)

// stub holds the fake world a test presents to Reconcile. Every side-effecting
// hook is replaced so a case never touches the real system proxy, never spawns
// a core, and never depends on what happens to be running on the machine.
type stub struct {
	proxyOn     bool
	tunActive   bool
	coreRunning bool
	portOpen    bool
	waitOpen    bool
	startErr    error
	disableErr  error

	startCalls   int
	disableCalls int
	probedPort   int
}

func withStub(t *testing.T, s *stub) {
	t.Helper()
	origStatus, origDisable, origCore, origTun := getProxyStatus, disableProxy, coreStatus, tunIsActive
	origStart, origProbe, origWait := startCore, probePort, waitPort
	t.Cleanup(func() {
		getProxyStatus, disableProxy, coreStatus, tunIsActive = origStatus, origDisable, origCore, origTun
		startCore, probePort, waitPort = origStart, origProbe, origWait
	})

	getProxyStatus = func() (*sysproxy.Status, error) {
		st := &sysproxy.Status{HTTPPort: 7890}
		if s.proxyOn {
			st.HTTPEnabled = true
			st.HTTPHost = "127.0.0.1"
			st.HTTPSEnabled = true
			st.HTTPSPort = 7890
			st.SOCKSEnabled = true
			st.SOCKSPort = 7890
		}
		return st, nil
	}
	disableProxy = func(port int) error {
		if port != 7890 {
			t.Fatalf("cleanup targeted unexpected port: %d", port)
		}
		s.disableCalls++
		return s.disableErr
	}
	coreStatus = func() (bool, int, error) {
		if s.coreRunning {
			return true, 4242, nil
		}
		return false, 0, nil
	}
	tunIsActive = func(paths.RuntimePaths) bool { return s.tunActive }
	startCore = func(paths.RuntimePaths) error {
		s.startCalls++
		return s.startErr
	}
	probePort = func(port int, timeout time.Duration) bool {
		s.probedPort = port
		return s.portOpen
	}
	waitPort = func(port int, total, perAttempt time.Duration) bool {
		return s.portOpen || s.waitOpen
	}
}

func newOpts(t *testing.T) Options {
	t.Helper()
	dir := t.TempDir()
	return Options{
		Paths:            paths.ForHome(dir),
		StatePath:        filepath.Join(dir, "guard-state.json"),
		FailureThreshold: 3,
		DialTimeout:      time.Millisecond,
	}
}

func TestNoopWhenSystemProxyDisabled(t *testing.T) {
	s := &stub{proxyOn: false, coreRunning: false, portOpen: false}
	withStub(t, s)

	res, err := Reconcile(newOpts(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Action != ActionNoop {
		t.Errorf("action = %q, want %q", res.Action, ActionNoop)
	}
	if s.startCalls != 0 || s.disableCalls != 0 {
		t.Errorf("side effects on a disabled proxy: start=%d disable=%d", s.startCalls, s.disableCalls)
	}
}

func TestTunWithDeadMixedPortIsNotHealthy(t *testing.T) {
	// Explicit system proxies still need the mixed listener, even with TUN.
	s := &stub{proxyOn: true, tunActive: true, coreRunning: false, portOpen: false}
	withStub(t, s)

	opts := newOpts(t)
	res, err := Reconcile(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Action != ActionPending || res.PortListening {
		t.Errorf("dead mixed port treated as healthy: %+v", res)
	}
	if s.startCalls != 0 || s.disableCalls != 0 {
		t.Errorf("premature cleanup or competing core: start=%d disable=%d", s.startCalls, s.disableCalls)
	}
	for range 2 {
		res, err = Reconcile(opts)
		if err != nil {
			t.Fatal(err)
		}
	}
	if res.Action != ActionDisabledProxy || s.startCalls != 0 || s.disableCalls != 1 {
		t.Fatalf("failed to clear a dead TUN mixed proxy safely: %+v", res)
	}
}

func TestHealthyWhenCoreServesPort(t *testing.T) {
	s := &stub{proxyOn: true, coreRunning: true, portOpen: true}
	withStub(t, s)

	res, err := Reconcile(newOpts(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Action != ActionHealthy {
		t.Errorf("action = %q, want %q", res.Action, ActionHealthy)
	}
	if s.disableCalls != 0 {
		t.Error("disabled a working proxy")
	}
}

func TestRestartsCoreWhenProcessMissing(t *testing.T) {
	s := &stub{proxyOn: true, coreRunning: false, waitOpen: true}
	withStub(t, s)

	res, err := Reconcile(newOpts(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Action != ActionStartedCore {
		t.Errorf("action = %q, want %q", res.Action, ActionStartedCore)
	}
	if !res.Repaired {
		t.Error("Repaired = false after restarting the core")
	}
	if s.startCalls != 1 {
		t.Errorf("startCalls = %d, want 1", s.startCalls)
	}
	if s.disableCalls != 0 {
		t.Error("disabled the proxy even though the core came back")
	}
}

func TestPendingBelowThreshold(t *testing.T) {
	s := &stub{proxyOn: true, coreRunning: false, portOpen: false, startErr: errors.New("boom")}
	opts := newOpts(t)
	withStub(t, s)

	res, err := Reconcile(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Action != ActionPending {
		t.Errorf("action = %q, want %q", res.Action, ActionPending)
	}
	if res.Failures != 1 {
		t.Errorf("Failures = %d, want 1", res.Failures)
	}
	if s.disableCalls != 0 {
		t.Error("disabled the proxy before the failure streak proved this is not a race")
	}
}

func TestDisablesProxyAfterRepeatedFailures(t *testing.T) {
	s := &stub{proxyOn: true, coreRunning: false, portOpen: false, startErr: errors.New("boom")}
	opts := newOpts(t)
	withStub(t, s)

	// Two failures already on record; the third must trip the breaker and
	// restore a working direct connection.
	writeState(opts.StatePath, state{Failures: 2})

	res, err := Reconcile(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Action != ActionDisabledProxy {
		t.Errorf("action = %q, want %q", res.Action, ActionDisabledProxy)
	}
	if s.disableCalls != 1 {
		t.Errorf("disableCalls = %d, want 1", s.disableCalls)
	}
	if res.ProxyEnabled {
		t.Error("ProxyEnabled = true after disabling")
	}
	if res.Failures != 0 {
		t.Errorf("Failures = %d, want 0 after the breaker tripped", res.Failures)
	}
}

func TestFailureStreakSurvivesAcrossRuns(t *testing.T) {
	s := &stub{proxyOn: true, coreRunning: false, portOpen: false, startErr: errors.New("boom")}
	opts := newOpts(t)
	withStub(t, s)

	for i := 1; i <= 3; i++ {
		res, err := Reconcile(opts)
		if err != nil {
			t.Fatalf("run %d: unexpected error: %v", i, err)
		}
		if i < 3 && res.Action != ActionPending {
			t.Fatalf("run %d: action = %q, want %q", i, res.Action, ActionPending)
		}
		if i == 3 && res.Action != ActionDisabledProxy {
			t.Fatalf("run %d: action = %q, want %q", i, res.Action, ActionDisabledProxy)
		}
	}
}

func TestHealthyRunResetsStreak(t *testing.T) {
	s := &stub{proxyOn: true, coreRunning: true, portOpen: true}
	opts := newOpts(t)
	withStub(t, s)

	writeState(opts.StatePath, state{Failures: 2})
	if _, err := Reconcile(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := readState(opts.StatePath).Failures; got != 0 {
		t.Errorf("Failures = %d after a healthy run, want 0", got)
	}
}

func TestPortOpenDetectsListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	if !PortOpen(port, 200*time.Millisecond) {
		t.Errorf("PortOpen(%d) = false for a live listener", port)
	}

	// A port nobody listens on: bind, read the number, release it.
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	deadPort := dead.Addr().(*net.TCPAddr).Port
	dead.Close()
	if PortOpen(deadPort, 200*time.Millisecond) {
		t.Errorf("PortOpen(%d) = true for a closed port", deadPort)
	}
}

func TestForeignProxiesAreIgnored(t *testing.T) {
	for _, status := range []*sysproxy.Status{
		{HTTPEnabled: true, HTTPHost: "company.example", HTTPPort: 7890},
		{SOCKSEnabled: true, SOCKSHost: "127.0.0.1", SOCKSPort: 1080},
		{HTTPSEnabled: true, HTTPSPort: 7890},
	} {
		s := &stub{}
		withStub(t, s)
		getProxyStatus = func() (*sysproxy.Status, error) { return status, nil }
		res, err := Reconcile(newOpts(t))
		if err != nil || res.Action != ActionNoop || s.startCalls != 0 || s.disableCalls != 0 || s.probedPort != 0 {
			t.Fatalf("foreign proxy was touched: %+v %+v %v", status, res, err)
		}
	}
}

func TestGuardStandsDownDuringHandoffAndStop(t *testing.T) {
	s := &stub{proxyOn: true, startErr: errors.New("missing")}
	withStub(t, s)
	opts := newOpts(t)
	unlock, err := core.LockLifecycle(opts.Paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	writeState(opts.StatePath, state{Failures: 2})
	getProxyStatus = func() (*sysproxy.Status, error) {
		t.Fatal("read state in the middle of a lifecycle operation")
		return nil, nil
	}
	res, err := Reconcile(opts)
	if err != nil || res.Action != ActionPending || s.startCalls != 0 || s.disableCalls != 0 {
		t.Fatalf("guard interfered with handoff: %+v %v", res, err)
	}
	if readState(opts.StatePath).Failures != 2 {
		t.Fatal("handoff changed the failure streak")
	}
}

func TestWorkingTunStillChecksMixedPort(t *testing.T) {
	s := &stub{proxyOn: true, tunActive: true, portOpen: true}
	withStub(t, s)
	res, err := Reconcile(newOpts(t))
	if err != nil || res.Action != ActionHealthy || s.probedPort != 7890 || s.startCalls != 0 || s.disableCalls != 0 {
		t.Fatalf("healthy TUN: %+v %v", res, err)
	}
}

func TestLookupFailureDoesNotPreventDeadProxyCleanup(t *testing.T) {
	s := &stub{proxyOn: true}
	withStub(t, s)
	coreStatus = func() (bool, int, error) { return false, 0, errors.New("lookup unavailable") }
	opts := newOpts(t)
	opts.FailureThreshold = 1
	res, err := Reconcile(opts)
	if err != nil || res.Action != ActionDisabledProxy || s.startCalls != 0 || s.disableCalls != 1 {
		t.Fatalf("left a dead proxy after process lookup failed: %+v %v", res, err)
	}
}

func TestMixedForeignHTTPAndOwnedHTTPSUsesOnlyOwnedPort(t *testing.T) {
	s := &stub{proxyOn: true, startErr: errors.New("missing core")}
	withStub(t, s)
	getProxyStatus = func() (*sysproxy.Status, error) {
		return &sysproxy.Status{
			HTTPEnabled: true, HTTPHost: "company.example", HTTPPort: 3128,
			HTTPSEnabled: true, HTTPSHost: "127.0.0.1", HTTPSPort: 7890,
		}, nil
	}
	opts := newOpts(t)
	opts.FailureThreshold = 1
	res, err := Reconcile(opts)
	if err != nil || res.Action != ActionDisabledProxy || s.probedPort != 7890 || s.disableCalls != 1 {
		t.Fatalf("foreign HTTP port was mistaken for ProxyCat: %+v %v", res, err)
	}
}

func TestGuardRecoversKilledIsolatedCoreWithoutDisablingProxy(t *testing.T) {
	binary := os.Getenv("PROXYCAT_TEST_MIHOMO")
	if binary == "" {
		t.Skip("set PROXYCAT_TEST_MIHOMO for isolated guard recovery")
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
	opts := newOpts(t)
	opts.Port = port
	if err := os.MkdirAll(opts.Paths.Config, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(opts.Paths.Logs, 0700); err != nil {
		t.Fatal(err)
	}
	opts.Paths.Mihomo = binary
	config := fmt.Sprintf("mixed-port: %d\nexternal-controller: %s\nbind-address: 127.0.0.1\nallow-lan: false\nmode: direct\nlog-level: silent\nrules: ['MATCH,DIRECT']\n", port, address)
	if err := os.WriteFile(opts.Paths.ConfigYAML, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	old := exec.Command(binary, "-d", opts.Paths.Config, "-f", opts.Paths.ConfigYAML)
	if err := old.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = old.Process.Kill()
			_ = old.Wait()
		}
	}()
	if !WaitForPort(port, 5*time.Second, 50*time.Millisecond) {
		t.Fatal("isolated core did not start")
	}
	if err := old.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = old.Wait()
	waited = true
	commands := t.TempDir()
	if err := os.WriteFile(filepath.Join(commands, "pgrep"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", commands+string(os.PathListSeparator)+os.Getenv("PATH"))
	s := &stub{proxyOn: true}
	withStub(t, s)
	getProxyStatus = func() (*sysproxy.Status, error) {
		return &sysproxy.Status{HTTPEnabled: true, HTTPHost: "127.0.0.1", HTTPPort: port}, nil
	}
	probePort, waitPort = PortOpen, WaitForPort
	var recoveredPID int
	startCore = func(p paths.RuntimePaths) error {
		var err error
		recoveredPID, err = core.EnsureProxyReady(p.Mihomo, p.ConfigYAML, p.MihomoLog)
		return err
	}
	t.Cleanup(func() {
		if recoveredPID > 0 {
			_ = core.StopGraceful(recoveredPID, time.Second)
		}
	})
	res, err := Reconcile(opts)
	if err != nil || res.Action != ActionStartedCore || !res.PortListening || !res.ProxyEnabled || s.disableCalls != 0 {
		t.Fatalf("guard disabled proxy instead of recovery: %+v %v", res, err)
	}
	// The restored endpoint also answers its controller; it is not merely a
	// living PID returned by a process-name query.
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Second}
	defer client.CloseIdleConnections()
	response, err := client.Get("http://" + address + "/configs")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal("restored controller unavailable")
	}
}
