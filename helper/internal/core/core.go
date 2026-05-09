// Package core provides process lifecycle management for Mihomo proxy.
package core

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Start launches a process with the given binary path, config path, and log path.
// It opens the log file for append and redirects stdout/stderr to it.
// Additional args can be passed after the log path.
// Returns the process PID or an error.
func Start(binPath, configPath, logPath string, extraArgs ...string) (int, error) {
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	args := append([]string{"-f", configPath}, extraArgs...)
	cmd := exec.Command(binPath, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start process: %w", err)
	}

	return cmd.Process.Pid, nil
}

// Stop terminates the process with the given PID.
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
// Returns (running bool, pid int, err error).
func Status() (bool, int, error) {
	if runtime.GOOS == "windows" {
		return windowsStatus()
	}
	return unixStatus()
}

func unixStatus() (bool, int, error) {
	output, err := exec.Command("pgrep", "-x", "mihomo").Output()
	if err != nil {
		return false, 0, nil
	}

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

func windowsStatus() (bool, int, error) {
	output, err := exec.Command("tasklist", "/FI", "IMAGENAME eq mihomo.exe", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false, 0, nil
	}

	line := strings.TrimSpace(string(output))
	if !strings.Contains(line, "mihomo.exe") {
		return false, 0, nil
	}

	// CSV format: "mihomo.exe","12345","Console","1","5,632 K"
	parts := strings.Split(line, ",")
	if len(parts) >= 2 {
		pidStr := strings.Trim(parts[1], "\"")
		pid, err := strconv.Atoi(pidStr)
		if err == nil {
			return true, pid, nil
		}
	}

	return true, 0, nil
}
