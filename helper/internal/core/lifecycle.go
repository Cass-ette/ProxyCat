package core

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var ErrLifecycleBusy = errors.New("代理正在切换")

// TryLockLifecycle lets background repairs stand down during a user operation.
// Use the same kernel lock as start/stop and TUN handoffs, without waiting for
// them or holding the lock for the lifetime of the watcher.
func TryLockLifecycle(directory string) (func(), error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(directory, ".lifecycle.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, ErrLifecycleBusy
		}
		return nil, err
	}
	return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close() }, nil
}

// LockLifecycle serializes helper processes, including a TUN handoff and a
// second start/stop requested while that handoff is still in progress.
func LockLifecycle(directory string) (func(), error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(directory, ".lifecycle.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for deadline := time.Now().Add(90 * time.Second); ; {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close() }, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			file.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			file.Close()
			return nil, fmt.Errorf("代理正在切换，请稍后重试")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// EnsureRunning is called under LockLifecycle. Reuse the owned core instead of
// creating another process that can stay alive after failing to bind its ports.
func EnsureRunning(binary, config, log string) (int, error) {
	running, pid, err := Status()
	if err != nil {
		return 0, err
	}
	if running {
		if err := CheckOwner(pid, binary); err != nil {
			return 0, err
		}
		return pid, nil
	}
	return Start(binary, config, log)
}

func CheckOwner(pid int, binary string) error {
	output, err := exec.Command("/bin/ps", "-ww", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil || strings.TrimSpace(string(output)) != binary {
		return fmt.Errorf("运行中的内核不属于 ProxyCat，请先关闭其他代理内核")
	}
	return nil
}

// WaitStopped waits for the actual process to exit, rather than treating a
// successful signal as proof that its listeners have already been released.
func WaitStopped(pid int) error {
	for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); {
		if processStopped(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("代理内核尚未退出，未启动重复进程")
}

func processStopped(pid int) bool {
	output, err := exec.Command("/bin/ps", "-p", strconv.Itoa(pid), "-o", "stat=").Output()
	return (err == nil && strings.HasPrefix(strings.TrimSpace(string(output)), "Z")) ||
		(err != nil && syscall.Kill(pid, 0) == syscall.ESRCH)
}
