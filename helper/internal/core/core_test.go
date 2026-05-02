package core

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestStartStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create a dummy config file
	if err := os.WriteFile(configPath, []byte("test: true\n"), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	// Test Start with /bin/sleep as a fake binary
	pid, err := Start("/bin/sleep", configPath, logPath, "10")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("expected positive PID, got %d", pid)
	}

	// Give process a moment to start
	time.Sleep(100 * time.Millisecond)

	// Verify log file was created
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Fatalf("log file was not created")
	}

	// Test Stop
	if err := Stop(pid); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Give process time to exit
	time.Sleep(100 * time.Millisecond)

	// Verify process is gone
	proc, err := os.FindProcess(pid)
	if err != nil {
		return // Process not found is OK
	}
	// Try to send signal 0 to check if process exists
	if proc != nil {
		err = proc.Signal(os.Signal(nil))
		if err == nil {
			t.Fatalf("process %d should have been stopped", pid)
		}
	}
}

func TestStartInvalidBinary(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte("test: true\n"), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	_, err := Start("/nonexistent/binary", configPath, logPath)
	if err == nil {
		t.Fatal("expected error for invalid binary path")
	}
}

func TestStopInvalidPID(t *testing.T) {
	// Try to stop a non-existent process (PID 99999)
	err := Stop(99999)
	if err == nil {
		t.Fatal("expected error for non-existent PID")
	}
}

func TestStatusNotRunning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	// Test when no mihomo process is running
	running, pid, err := Status()
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if running {
		t.Fatalf("expected not running when no mihomo process")
	}
	if pid != 0 {
		t.Fatalf("expected PID 0 when not running, got %d", pid)
	}
}

func TestStartWithLogFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte("test: true\n"), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	// Start a process that will write to log
	pid, err := Start("/bin/sleep", configPath, logPath, "0.1")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for process to complete
	time.Sleep(200 * time.Millisecond)

	// Verify log file was created
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Fatalf("log file was not created")
	}

	// Clean up (may already be dead)
	Stop(pid)
}
