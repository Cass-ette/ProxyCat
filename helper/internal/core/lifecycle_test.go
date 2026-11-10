package core

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestTryLifecycleLockSkipsActiveOperation(t *testing.T) {
	directory := t.TempDir()
	unlock, err := LockLifecycle(directory)
	if err != nil {
		t.Fatal(err)
	}
	second, err := TryLockLifecycle(directory)
	if second != nil {
		second()
	}
	if !errors.Is(err, ErrLifecycleBusy) {
		unlock()
		t.Fatalf("background operation entered the active lifecycle: %v", err)
	}
	unlock()
	second, err = TryLockLifecycle(directory)
	if err != nil {
		t.Fatal(err)
	}
	second()
}

func TestLifecycleLockSerializesIndependentHandles(t *testing.T) {
	directory := t.TempDir()
	unlock, err := LockLifecycle(directory)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			unlock()
		}
	}()
	acquired := make(chan func(), 1)
	go func() {
		second, err := LockLifecycle(directory)
		if err != nil {
			acquired <- nil
			return
		}
		acquired <- second
	}()
	select {
	case second := <-acquired:
		if second != nil {
			second()
		}
		t.Fatal("a second operation entered an active handoff")
	case <-time.After(100 * time.Millisecond):
	}
	unlock()
	released = true
	select {
	case second := <-acquired:
		if second == nil {
			t.Fatal("second operation failed")
		}
		second()
	case <-time.After(2 * time.Second):
		t.Fatal("lock remained held after completion")
	}
}

func TestEnsureRunningReusesTheExistingOwnedProcess(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "pgrep"), []byte(fmt.Sprintf("#!/bin/sh\nprintf '%%d\\n' %d\n", cmd.Process.Pid)), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	pid, err := EnsureRunning("/bin/sleep", "/unused-config", "/unused-log")
	if err != nil || pid != cmd.Process.Pid {
		t.Fatalf("existing process not reused: %d %v", pid, err)
	}
}

func TestEnsureRunningDoesNotLaunchWhenProcessLookupFails(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "pgrep"), []byte("#!/bin/sh\nexit 2\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	log := filepath.Join(directory, "should-not-exist.log")
	if _, err := EnsureRunning("/bin/sleep", "/unused-config", log); err == nil {
		t.Fatal("process lookup failure was treated as no existing core")
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("attempted to launch another core after failed process lookup")
	}
}
