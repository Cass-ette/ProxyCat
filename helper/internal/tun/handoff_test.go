package tun

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/controller"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
)

func TestHandoffFailureKeepsOrRestoresTheOriginalService(t *testing.T) {
	for _, test := range []struct {
		name, failAt, restoreError, disableError string
		want                                     []string
		message                                  string
	}{
		{"supervisor never prepared", "prepare", "", "", []string{"prepare", "cancel"}, "原普通代理未停止"},
		{"normal core cannot stop", "stop", "", "", []string{"prepare", "stop", "cancel"}, "原普通代理未停止"},
		{"privileged launch fails", "start", "", "", []string{"prepare", "stop", "start", "cancel", "restore"}, "已确认普通代理恢复"},
		{"tunnel does not become ready", "confirm", "", "", []string{"prepare", "stop", "start", "confirm", "cancel", "restore"}, "已确认普通代理恢复"},
		{"rollback cannot restore listener", "confirm", "listener unavailable", "", []string{"prepare", "stop", "start", "confirm", "cancel", "restore", "disable"}, "已关闭系统代理"},
		{"fallback also fails", "confirm", "listener unavailable", "permission denied", []string{"prepare", "stop", "start", "confirm", "cancel", "restore", "disable"}, "关闭系统代理失败"},
		{"success", "", "", "", []string{"prepare", "stop", "start", "confirm"}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			var events []string
			step := func(name string) error {
				events = append(events, name)
				if test.failAt == name {
					return errors.New(name + " failed")
				}
				return nil
			}
			err := handoff(true, handoffActions{
				prepare:       func() error { return step("prepare") },
				stopNormal:    func() (bool, error) { err := step("stop"); return err == nil, err },
				startTunnel:   func() error { return step("start") },
				confirmTunnel: func() error { return step("confirm") },
				cancelTunnel:  func() error { return step("cancel") },
				restoreNormal: func() error {
					_ = step("restore")
					if test.restoreError != "" {
						return errors.New(test.restoreError)
					}
					return nil
				},
				disableProxy: func() error {
					_ = step("disable")
					if test.disableError != "" {
						return errors.New(test.disableError)
					}
					return nil
				},
			})
			if !reflect.DeepEqual(events, test.want) {
				t.Fatalf("sequence: %v, want %v", events, test.want)
			}
			if test.message == "" && err != nil {
				t.Fatal(err)
			}
			if test.message != "" && (err == nil || !strings.Contains(err.Error(), test.message)) {
				t.Fatalf("result: %v", err)
			}
		})
	}
}

func TestHandoffRestoresAfterStopAcknowledgementFails(t *testing.T) {
	restored := false
	err := handoff(true, handoffActions{
		prepare:       func() error { return nil },
		stopNormal:    func() (bool, error) { return true, errors.New("process has not finished exiting") },
		cancelTunnel:  func() error { return nil },
		restoreNormal: func() error { restored = true; return nil },
	})
	if !restored || err == nil || !strings.Contains(err.Error(), "已确认普通代理恢复") {
		t.Fatalf("rollback skipped: %v", err)
	}
}

func TestHandoffDoesNotStartCompetingCoreWhenCleanupFails(t *testing.T) {
	restored := false
	err := handoff(true, handoffActions{
		prepare:       func() error { return nil },
		stopNormal:    func() (bool, error) { return true, nil },
		startTunnel:   func() error { return errors.New("launch failed") },
		cancelTunnel:  func() error { return errors.New("child still exiting") },
		restoreNormal: func() error { restored = true; return nil },
	})
	if restored || err == nil || !strings.Contains(err.Error(), "TUN 清理失败") || strings.Contains(err.Error(), "已确认普通代理恢复") {
		t.Fatalf("unconfirmed cleanup: restored=%v error=%v", restored, err)
	}
}

func TestHandoffWithoutOriginalCoreDoesNotClaimOneWasPreserved(t *testing.T) {
	err := handoff(false, handoffActions{
		prepare:      func() error { return errors.New("preparation failed") },
		cancelTunnel: func() error { return nil },
	})
	if err == nil || strings.Contains(err.Error(), "原普通代理") {
		t.Fatalf("reported a nonexistent original core: %v", err)
	}
}

func preparedSupervisor(t *testing.T, owner func() bool, launches *atomic.Int32) (session, <-chan error) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "pcprep-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	s := session{Directory: dir, Token: strings.Repeat("b", 64), UID: os.Getuid(), GID: os.Getgid()}
	done := make(chan error, 1)
	launch := func() (*exec.Cmd, error) {
		launches.Add(1)
		cmd := exec.Command("/bin/sleep", "30")
		return cmd, cmd.Start()
	}
	go func() { done <- superviseSession(s, nil, launch, func() bool { return true }, owner) }()
	t.Cleanup(func() { _ = request(s, http.MethodPost, "/stop") })
	if err := waitForSession(s, "/prepared", 2*time.Second); err != nil {
		t.Fatal(err)
	}
	return s, done
}

func TestPreparedSupervisorDoesNotStartCoreBeforeExplicitHandoff(t *testing.T) {
	var launches atomic.Int32
	s, done := preparedSupervisor(t, func() bool { return true }, &launches)
	if launches.Load() != 0 {
		t.Fatal("preparation interrupted the old core")
	}
	if request(s, http.MethodGet, "/status") == nil {
		t.Fatal("prepared supervisor claimed a live tunnel")
	}
	if err := request(s, http.MethodPost, "/start"); err != nil {
		t.Fatal(err)
	}
	if err := request(s, http.MethodPost, "/start"); err != nil {
		t.Fatal(err)
	}
	if launches.Load() != 1 {
		t.Fatal("duplicate start created a second child")
	}
	if err := request(s, http.MethodGet, "/status"); err != nil {
		t.Fatal(err)
	}
	if err := request(s, http.MethodPost, "/stop"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("supervisor did not stop")
	}
}

func TestCancelledPreparedSessionCannotStartCore(t *testing.T) {
	var launches atomic.Int32
	var owner atomic.Bool
	owner.Store(true)
	s, done := preparedSupervisor(t, owner.Load, &launches)
	owner.Store(false)
	if err := request(s, http.MethodPost, "/start"); err == nil {
		t.Fatal("cancelled handoff accepted")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled supervisor did not stop")
	}
	if launches.Load() != 0 {
		t.Fatal("cancelled preparation launched a core")
	}
}

func TestPreparationFailureHasNoCoreToStop(t *testing.T) {
	var launches atomic.Int32
	s, done := preparedSupervisor(t, func() bool { return true }, &launches)
	if err := request(s, http.MethodPost, "/stop"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("prepared supervisor did not stop")
	}
	if launches.Load() != 0 {
		t.Fatal("abandoning preparation launched a core")
	}
}

func TestProxyReadinessRequiresAListeningPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := proxyListenerReady(port); err != nil {
		t.Fatal(err)
	}
	listener.Close()
	if proxyListenerReady(port) == nil {
		t.Fatal("closed listener reported restored")
	}
	if proxyListenerReady(0) == nil {
		t.Fatal("missing listener reported restored")
	}
}

func TestProcessIdentitySurvivesDifferentTimezones(t *testing.T) {
	t.Setenv("TZ", "Asia/Shanghai")
	first := processStart(os.Getpid())
	t.Setenv("TZ", "America/Los_Angeles")
	second := processStart(os.Getpid())
	if first == "" || first != second {
		t.Fatalf("process identity changed: %q %q", first, second)
	}
}

func TestPreparationReportsFailureWithoutAListener(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "error"), []byte("root preparation failed"), 0600); err != nil {
		t.Fatal(err)
	}
	err := waitForSession(session{Directory: dir}, "/prepared", time.Second)
	if err == nil || !strings.Contains(err.Error(), "root preparation failed") {
		t.Fatalf("lost startup failure: %v", err)
	}
}

func TestRestoreWaitsForExpectedCoreAndRestoresRouting(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var configs, selections, modes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/configs":
			n := configs.Add(1)
			json.NewEncoder(w).Encode(controller.Config{MixedPort: listener.Addr().(*net.TCPAddr).Port, Tun: map[string]interface{}{"enable": n == 1}})
		case r.Method == http.MethodGet && r.URL.Path == "/proxies":
			w.Write([]byte(`{"proxies":{"GLOBAL":{"type":"Selector","now":"Node B","all":["Node B","DIRECT"]}}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/proxies/Main":
			var value struct {
				Name string `json:"name"`
			}
			json.NewDecoder(r.Body).Decode(&value)
			if value.Name != "Node B" {
				t.Errorf("lost node selection: %q", value.Name)
			}
			selections.Add(1)
		case r.Method == http.MethodPatch && r.URL.Path == "/configs":
			var value struct {
				Mode string `json:"mode"`
			}
			json.NewDecoder(r.Body).Decode(&value)
			if value.Mode != "global" {
				t.Errorf("lost routing mode: %q", value.Mode)
			}
			modes.Add(1)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	err = restoreController(controller.NewClient(server.URL), snapshot{mode: "global", selections: map[string]string{"Main": "Node B"}}, false)
	if err != nil || configs.Load() < 2 || selections.Load() != 1 || modes.Load() != 1 {
		t.Fatalf("restoration not verified: error=%v configs=%d selections=%d modes=%d", err, configs.Load(), selections.Load(), modes.Load())
	}
}

func TestRestoreDoesNotIgnoreFailedNodeSelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Write([]byte(`{"mixed-port":1,"tun":{"enable":false}}`))
			return
		}
		http.Error(w, "node unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	err := restoreController(controller.NewClient(server.URL), snapshot{selections: map[string]string{"Main": "Node B"}}, false)
	if err == nil || !strings.Contains(err.Error(), "恢复节点选择失败") {
		t.Fatalf("selection error ignored: %v", err)
	}
}

func TestUnresponsiveTunnelRetainsCancellationUntilExitIsConfirmed(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	if err := os.MkdirAll(p.Config, 0700); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	s := session{Directory: dir, Token: strings.Repeat("c", 64)}
	b, _ := json.Marshal(s)
	if err := os.WriteFile(marker(p), b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cancel"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	commands := t.TempDir()
	if err := os.WriteFile(filepath.Join(commands, "pgrep"), []byte(fmt.Sprintf("#!/bin/sh\nprintf '%%d\\n' %d\n", os.Getpid())), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", commands+string(os.PathListSeparator)+os.Getenv("PATH"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	_, err := stopSession(p, s, controller.NewClient(server.URL))
	if err == nil || !strings.Contains(err.Error(), "尚未确认 TUN 内核退出") {
		t.Fatalf("unresponsive child treated as stopped: %v", err)
	}
	for _, path := range []string{marker(p), filepath.Join(dir, "cancel")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("discarded cancellation before exit: %s %v", path, err)
		}
	}
}

func TestSuccessfulStopAcknowledgementStillWaitsForTunnelExit(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	if err := os.MkdirAll(p.Config, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("/tmp", "pcack-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	s := session{Directory: dir, Token: strings.Repeat("d", 64)}
	data, _ := json.Marshal(s)
	if err := os.WriteFile(marker(p), data, 0600); err != nil {
		t.Fatal(err)
	}
	child := exec.Command("/bin/sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { _ = child.Wait(); close(exited) }()
	t.Cleanup(func() { _ = child.Process.Kill(); <-exited })
	commands := t.TempDir()
	if err := os.WriteFile(filepath.Join(commands, "pgrep"), []byte(fmt.Sprintf("#!/bin/sh\nprintf '%%d\\n' %d\n", child.Process.Pid)), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", commands+string(os.PathListSeparator)+os.Getenv("PATH"))
	listener, err := net.Listen("unix", filepath.Join(dir, "control.sock"))
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/stop" {
			t.Error("unexpected stop request")
		}
		go func() { time.Sleep(250 * time.Millisecond); _ = child.Process.Kill() }()
		w.WriteHeader(http.StatusOK)
	})}
	go server.Serve(listener)
	defer server.Close()
	controllerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"tun":{"enable":true}}`) }))
	defer controllerServer.Close()
	start := time.Now()
	managed, err := stopSession(p, s, controller.NewClient(controllerServer.URL))
	if err != nil || !managed || time.Since(start) < 200*time.Millisecond {
		t.Fatalf("stop returned before child exit: %t %v %s", managed, err, time.Since(start))
	}
	select {
	case <-exited:
	default:
		t.Fatal("stop acknowledged while child still running")
	}
}
