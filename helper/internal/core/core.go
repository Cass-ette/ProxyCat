// Package core provides process lifecycle management for Mihomo proxy.
package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Start launches a process with the given binary path, config path, and log path.
// It opens the log file for append and redirects stdout/stderr to it.
// Additional args can be passed after the log path.
// Returns the process PID or an error.
func Start(binPath, configPath, logPath string, extraArgs ...string) (int, error) {
	// Open log file for append (create if doesn't exist)
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	// Build command arguments: -f configPath followed by any extra args
	args := append([]string{"-f", configPath}, extraArgs...)

	// Create command
	cmd := exec.Command(binPath, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Start the process
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start process: %w", err)
	}
	// A long-lived guard can start and later replace this child. Reap it rather
	// than leaving a zombie that process-name lookup mistakes for a live core.
	go func() { _ = cmd.Wait() }()

	return cmd.Process.Pid, nil
}

// Stop terminates the process with the given PID.
// Returns an error if the process cannot be found or killed.
func Stop(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	if err := process.Kill(); err != nil {
		return fmt.Errorf("failed to kill process %d: %w", pid, err)
	}

	return nil
}

// StopGraceful asks the process to exit and only escalates to SIGKILL when it
// ignores the request. Always wait for process exit before starting a successor;
// a successfully sent signal alone does not establish listener availability.
func StopGraceful(pid int, timeout time.Duration) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return fmt.Errorf("signal process %d: %w", pid, err)
	}
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); {
		if processStopped(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("kill process %d: %w", pid, err)
	}
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if processStopped(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("process %d is still running", pid)
}

// ProbePort reports whether something accepts TCP connections on a local port.
// This is the only honest readiness signal: a mihomo process that failed to
// bind its port stays alive and still answers pgrep.
func ProbePort(port int, timeout time.Duration) bool {
	if port <= 0 {
		return false
	}
	if timeout <= 0 {
		timeout = 400 * time.Millisecond
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// WaitForPort polls until the port accepts connections or the deadline passes.
func WaitForPort(port int, total, perAttempt time.Duration) bool {
	if total <= 0 {
		total = 8 * time.Second
	}
	for deadline := time.Now().Add(total); ; {
		if ProbePort(port, perAttempt) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Status checks if the Mihomo process is currently running.
// Uses pgrep -x mihomo to find the process.
// If multiple processes exist, returns the first one (oldest).
// Returns (running bool, pid int, err error).
func Status() (bool, int, error) {
	return StatusContext(context.Background())
}

// StatusContext allows a diagnostic to bound process inspection.
func StatusContext(ctx context.Context) (bool, int, error) {
	// Run pgrep -x mihomo to find the process
	output, err := exec.CommandContext(ctx, "pgrep", "-x", "mihomo").Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return false, 0, nil
		}
		return false, 0, fmt.Errorf("check running mihomo: %w", err)
	}

	// Parse PID from output (handle multiple lines - take first)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return false, 0, nil
	}

	pidStr := strings.TrimSpace(lines[0])
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return false, 0, fmt.Errorf("failed to parse pid: %w", err)
	}

	return true, pid, nil
}
