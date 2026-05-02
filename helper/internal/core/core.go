// Package core provides process lifecycle management for Mihomo proxy.
package core

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
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

// Status checks if the Mihomo process is currently running.
// Uses pgrep -x mihomo to find the process.
// If multiple processes exist, returns the first one (oldest).
// Returns (running bool, pid int, err error).
func Status() (bool, int, error) {
	// Run pgrep -x mihomo to find the process
	output, err := exec.Command("pgrep", "-x", "mihomo").Output()
	if err != nil {
		// No process found
		return false, 0, nil
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
