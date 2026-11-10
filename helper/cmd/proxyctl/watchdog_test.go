package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/core"
	"github.com/Cass-ette/ProxyCat/helper/internal/guard"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
)

func waitTestCondition(t *testing.T, condition func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for isolated watchdog fixture")
}

func startTestWatch(t *testing.T, base string, reconcile func() (guard.Result, error)) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- watchGuard(base, io.Discard, io.Discard, false, 25*time.Millisecond, reconcile) }()
	t.Cleanup(func() {
		stopGuardWatch(base)
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("isolated watcher did not stop")
		}
	})
	waitTestCondition(t, func() bool { return guardWatchRequest(base, "/status") == nil })
}

func TestWatchUsesKernelLockAndReadinessHandshake(t *testing.T) {
	base := t.TempDir()
	// A stale PID, including a live unrelated PID, is never treated as readiness.
	legacy := filepath.Join(base, "guard-watch.pid")
	if err := os.WriteFile(legacy, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		t.Fatal(err)
	}
	startTestWatch(t, base, func() (guard.Result, error) { return guard.Result{Action: guard.ActionNoop}, nil })
	if release, err := acquireGuardLock(base); !errors.Is(err, errGuardRunning) {
		if release != nil {
			release()
		}
		t.Fatalf("duplicate watcher entered: %v", err)
	}
	t.Setenv("PROXYCAT_NO_WATCHDOG", "")
	original := guardCommand
	guardCommand = func() (*exec.Cmd, error) { t.Fatal("spawned duplicate watcher despite readiness"); return nil, nil }
	t.Cleanup(func() { guardCommand = original })
	if err := spawnGuardWatch(base); err != nil {
		t.Fatal(err)
	}
	socket, err := guardSocket(base)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(socket)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("socket is not private: %v %v", info, err)
	}
}

func TestStopWatchNeverSignalsPIDFile(t *testing.T) {
	base := t.TempDir()
	cmd := exec.Command("/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	if err := os.WriteFile(filepath.Join(base, "guard-watch.pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0600); err != nil {
		t.Fatal(err)
	}
	stopGuardWatch(base)
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("signalled an unrelated process: %v", err)
	}
}

func TestWatchReadinessDoesNotWaitForLifecycleLock(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	unlock, err := core.LockLifecycle(p.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	// This real reconcile can only take its busy branch: no OS commands run.
	startTestWatch(t, p.Base, func() (guard.Result, error) { return guard.Reconcile(guard.Options{Paths: p}) })
	if err := guardWatchRequest(p.Base, "/status"); err != nil {
		t.Fatal(err)
	}
	// Stop before releasing the lock, so no live system proxy can be inspected.
	stopGuardWatch(p.Base)
}

func TestWatchRecoversStaleSocket(t *testing.T) {
	base := t.TempDir()
	socket, err := guardSocket(base)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	stale.Close()
	startTestWatch(t, base, func() (guard.Result, error) { return guard.Result{Action: guard.ActionNoop}, nil })
}

func TestSpawnGuardWatchHonoursOptOut(t *testing.T) {
	t.Setenv("PROXYCAT_NO_WATCHDOG", "1")
	base := t.TempDir()
	if err := spawnGuardWatch(base); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(base)
	if len(entries) != 0 {
		t.Fatal("opt-out spawn touched runtime state")
	}
}

func TestSpawnRequiresReadinessAfterStartingChild(t *testing.T) {
	t.Setenv("PROXYCAT_NO_WATCHDOG", "")
	original := guardCommand
	t.Cleanup(func() { guardCommand = original })
	guardCommand = func() (*exec.Cmd, error) { return exec.Command("/usr/bin/false"), nil }
	if err := spawnGuardWatch(t.TempDir()); err == nil {
		t.Fatal("accepted child exit as readiness")
	}
}

// This subprocess fixture never runs Reconcile or any OS proxy command. It
// verifies detachment using a fake heartbeat, even if the entire launching
// process group is force-killed. The decision logic is tested in internal/guard.
func TestGuardWatchSubprocess(t *testing.T) {
	role := os.Getenv("PROXYCAT_TEST_WATCH_ROLE")
	if role == "" {
		return
	}
	base := os.Getenv("PROXYCAT_TEST_WATCH_BASE")
	if role == "watch" {
		err := watchGuard(base, io.Discard, io.Discard, false, 25*time.Millisecond, func() (guard.Result, error) {
			err := os.WriteFile(filepath.Join(base, "heartbeat"), []byte(time.Now().Format(time.RFC3339Nano)), 0600)
			return guard.Result{Action: guard.ActionNoop}, err
		})
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	if role != "parent" {
		t.Fatal("invalid fixture role")
	}
	os.Setenv("PROXYCAT_NO_WATCHDOG", "")
	guardCommand = func() (*exec.Cmd, error) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestGuardWatchSubprocess$")
		cmd.Env = append(os.Environ(), "PROXYCAT_TEST_WATCH_ROLE=watch")
		return cmd, nil
	}
	if err := spawnGuardWatch(base); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "parent-ready"), []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestDetachedWatchSurvivesForcedParentGroupExit(t *testing.T) {
	base := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestGuardWatchSubprocess$")
	cmd.Env = append(os.Environ(), "PROXYCAT_TEST_WATCH_ROLE=parent", "PROXYCAT_TEST_WATCH_BASE="+base)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		stopGuardWatch(base)
	}()
	waitTestCondition(t, func() bool { _, err := os.Stat(filepath.Join(base, "parent-ready")); return err == nil })
	// The process group is created exclusively for this test, not looked up.
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	waited = true
	before, err := os.ReadFile(filepath.Join(base, "heartbeat"))
	if err != nil {
		t.Fatal(err)
	}
	waitTestCondition(t, func() bool {
		after, err := os.ReadFile(filepath.Join(base, "heartbeat"))
		return err == nil && len(after) > 0 && !bytes.Equal(before, after)
	})
	if err := guardWatchRequest(base, "/status"); err != nil {
		t.Fatalf("watcher died with parent: %v; %s", err, output.String())
	}
}

func TestEnableSystemProxyOrdersReadinessAndLimitsCleanup(t *testing.T) {
	for _, scenario := range []string{"normal", "tun-closed", "listener-closed", "watch-failed", "enable-partial"} {
		t.Run(scenario, func(t *testing.T) {
			var calls []string
			ops := systemProxyEnableOps{
				portOpen: func(int, time.Duration) bool { return false },
				waitPort: func(int, time.Duration, time.Duration) bool {
					calls = append(calls, "listener")
					return scenario != "listener-closed"
				},
				tunActive: func(paths.RuntimePaths) bool { return scenario == "tun-closed" },
				startCore: func(paths.RuntimePaths) error { calls = append(calls, "start"); return nil },
				startWatch: func(string) error {
					calls = append(calls, "watch")
					if scenario == "watch-failed" {
						return errors.New("no handshake")
					}
					return nil
				},
				enable: func(int) error {
					calls = append(calls, "enable")
					if scenario == "enable-partial" {
						return errors.New("partial enable")
					}
					return nil
				},
				disableOwned: func(port int) error { calls = append(calls, fmt.Sprintf("cleanup:%d", port)); return nil },
			}
			err := enableSystemProxy(paths.ForHome(t.TempDir()), 7890, ops)
			expected := map[string]string{
				"normal":          "[start listener watch enable]",
				"tun-closed":      "[]",
				"listener-closed": "[start listener]",
				"watch-failed":    "[start listener watch]",
				"enable-partial":  "[start listener watch enable cleanup:7890]",
			}[scenario]
			if fmt.Sprint(calls) != expected || ((err == nil) != (scenario == "normal")) {
				t.Fatalf("enable ordering: %v; %v", calls, err)
			}
		})
	}
}

func TestExplicitCoreStopDisablesProxyBeforeInspectingCore(t *testing.T) {
	for _, failDisable := range []bool{false, true} {
		t.Run(fmt.Sprint(failDisable), func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "commands.log")
			t.Setenv("PROXYCAT_TEST_STOP_LOG", log)
			t.Setenv("PROXYCAT_TEST_FAIL_DISABLE", strconv.FormatBool(failDisable))
			for name, body := range map[string]string{
				"networksetup": `
if [ "$1" = '-listallnetworkservices' ]; then
    printf 'An asterisk denotes a disabled service.\nTest Wi-Fi\n'
    exit 0
fi
printf '%s\n' "$1" >> "$PROXYCAT_TEST_STOP_LOG"
if [ "$PROXYCAT_TEST_FAIL_DISABLE" = true ]; then exit 7; fi
`,
				"pgrep": "echo core-status >> \"$PROXYCAT_TEST_STOP_LOG\"\nexit 1\n",
			} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			var out, diagnostic bytes.Buffer
			exit := run([]string{"core", "stop"}, &out, &diagnostic)
			commands, _ := os.ReadFile(log)
			want := "-setwebproxystate\n-setsecurewebproxystate\n-setsocksfirewallproxystate\n"
			if !failDisable {
				want += "core-status\n"
			}
			if string(commands) != want || (exit == 0) == failDisable {
				t.Fatalf("unsafe stop order: %q exit %d %s", strings.TrimSpace(string(commands)), exit, diagnostic.String())
			}
		})
	}
}
