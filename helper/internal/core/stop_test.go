package core

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGracefulStopWaitsForUnreapedChildAndAllowsPortRelease(t *testing.T) {
	child := exec.Command("/bin/sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	// Wait is deliberately deferred: a terminated but unreaped child must not
	// be mistaken for a live owner just because signal zero still succeeds.
	start := time.Now()
	if err := StopGraceful(child.Process.Pid, time.Second); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("waited for timeout after child exited")
	}
}

func TestCoreReplacementRefusesProcessWhoseOwnerChanged(t *testing.T) {
	child := exec.Command("/bin/sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	_, err := replaceCore(filepath.Join(t.TempDir(), "mihomo"), "unused", "unused", child.Process.Pid)
	if err == nil || !strings.Contains(err.Error(), "不属于 ProxyCat") {
		t.Fatalf("unsafe replacement: %v", err)
	}
	if processStopped(child.Process.Pid) {
		t.Fatal("terminated a process without ownership")
	}
}
