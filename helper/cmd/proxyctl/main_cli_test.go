package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCoreCommands tests the core subcommands
func TestCoreStatusCommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	// Test core status - should work even if mihomo isn't running
	exitCode := run([]string{"core", "status"}, stdout, stderr)

	// Should return 0 whether running or not (status is informational)
	if exitCode != 0 && exitCode != 1 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}

	output := stdout.String() + stderr.String()
	// Should mention either "running" or "not running"
	if !strings.Contains(output, "running") && !strings.Contains(output, "not running") {
		t.Fatalf("expected status output about mihomo running state, got: %s", output)
	}
}

func TestCoreStartStopRestartCommands(t *testing.T) {
	// These tests would need actual mihomo binary to work properly
	// For now, just test that the commands are recognized
	tests := []struct {
		name   string
		args   []string
		expect string
	}{
		{
			name:   "start requires binary to exist",
			args:   []string{"core", "start"},
			expect: "start",
		},
		{
			name:   "stop handles not running",
			args:   []string{"core", "stop"},
			expect: "stop",
		},
		{
			name:   "restart handles not running",
			args:   []string{"core", "restart"},
			expect: "restart",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			exitCode := run(tt.args, stdout, stderr)

			// These will likely fail since there's no mihomo binary
			// Just verify the command is recognized (not "unknown command")
			output := stderr.String()
			if strings.Contains(output, "unknown core subcommand") {
				t.Fatalf("command should be recognized: %s", output)
			}
			_ = exitCode // Allow failure since mihomo isn't available in tests
		})
	}
}

func TestCoreMissingSubcommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"core"}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for missing subcommand, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "subcommand required") {
		t.Fatalf("expected 'subcommand required' in stderr, got: %s", stderr.String())
	}
}

func TestCoreUnknownSubcommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"core", "unknown"}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for unknown subcommand, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "unknown core subcommand") {
		t.Fatalf("expected 'unknown core subcommand' in stderr, got: %s", stderr.String())
	}
}

// TestSystemProxyCommands tests the system-proxy subcommands
func TestSystemProxyStatusCommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"system-proxy", "status"}, stdout, stderr)

	// On non-Darwin platforms, this will return error
	// On Darwin, it will work
	output := stdout.String() + stderr.String()

	// Should contain some status information or error
	if !strings.Contains(output, "proxy") && !strings.Contains(output, "macOS") {
		t.Fatalf("expected proxy-related output, got: %s", output)
	}

	_ = exitCode // Allow different exit codes based on platform
}

func TestSystemProxyOnOffCommands(t *testing.T) {
	// Test that on/off commands are recognized
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "on",
			args: []string{"system-proxy", "on"},
		},
		{
			name: "off",
			args: []string{"system-proxy", "off"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			exitCode := run(tt.args, stdout, stderr)

			// Should not be "unknown subcommand" error
			if strings.Contains(stderr.String(), "unknown system-proxy subcommand") {
				t.Fatalf("command should be recognized: %s", stderr.String())
			}

			// On non-macOS, expect error about platform support
			// On macOS, command might succeed or fail based on system state
			_ = exitCode
		})
	}
}

func TestSystemProxyMissingSubcommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"system-proxy"}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for missing subcommand, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "subcommand required") {
		t.Fatalf("expected 'subcommand required' in stderr, got: %s", stderr.String())
	}
}

func TestSystemProxyUnknownSubcommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"system-proxy", "unknown"}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for unknown subcommand, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "unknown system-proxy subcommand") {
		t.Fatalf("expected 'unknown system-proxy subcommand' in stderr, got: %s", stderr.String())
	}
}

func TestModeMissingSubcommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"mode"}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for missing subcommand, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "mode subcommand required") {
		t.Fatalf("expected mode subcommand error, got: %s", stderr.String())
	}
}

func TestModeSetMissingArgument(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"mode", "set"}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for missing mode, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "mode set requires") {
		t.Fatalf("expected mode set requires error, got: %s", stderr.String())
	}
}

func TestModeSetRejectsInvalidMode(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"mode", "set", "invalid"}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for invalid mode, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "mode must be one of") {
		t.Fatalf("expected invalid mode message, got: %s", stderr.String())
	}
}

// TestGroupsCommands tests the groups subcommands
func TestGroupsListCommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	// Test groups list (or just groups which defaults to list)
	exitCode := run([]string{"groups"}, stdout, stderr)

	// This will likely fail since no mihomo is running
	// But it should be recognized as a valid command
	if strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("groups command should be recognized: %s", stderr.String())
	}

	// Output might contain proxy groups or error about connection.
	// A real local Mihomo may return non-English group names, so accept the stable
	// listing shape instead of requiring the English word "group".
	output := stdout.String() + stderr.String()
	if !strings.Contains(output, "current=") && !strings.Contains(output, "connection") && !strings.Contains(output, "No proxy") {
		t.Fatalf("unexpected output: %s", output)
	}

	_ = exitCode
}

func TestGroupsListExplicit(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"groups", "list"}, stdout, stderr)

	if strings.Contains(stderr.String(), "unknown groups subcommand") {
		t.Fatalf("groups list should be recognized: %s", stderr.String())
	}

	_ = exitCode
}

func TestGroupsSelectMissingArgs(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"groups", "select"}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for missing args, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "requires") {
		t.Fatalf("expected 'requires' in stderr, got: %s", stderr.String())
	}
}

func TestGroupsSelectCommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	// Test groups select with args - will fail since no mihomo running
	exitCode := run([]string{"groups", "select", "Auto", "Direct"}, stdout, stderr)

	// Should not be unknown subcommand error
	if strings.Contains(stderr.String(), "unknown groups subcommand") {
		t.Fatalf("groups select should be recognized: %s", stderr.String())
	}

	// Might fail due to connection, but command should be recognized
	_ = exitCode
}

func TestGroupsUnknownSubcommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"groups", "unknown"}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for unknown subcommand, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "unknown groups subcommand") {
		t.Fatalf("expected 'unknown groups subcommand' in stderr, got: %s", stderr.String())
	}
}

// TestTestCommand tests the test command
func TestTestCommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"test"}, stdout, stderr)

	// Should be recognized as valid command
	if strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("test command should be recognized: %s", stderr.String())
	}

	// Output might contain test results (OK/FAIL) or error about connection/no proxy
	output := stdout.String() + stderr.String()
	if !strings.Contains(output, "group") && !strings.Contains(output, "connection") &&
		!strings.Contains(output, "No proxy") && !strings.Contains(output, "OK") && !strings.Contains(output, "FAIL") {
		t.Fatalf("unexpected output: %s", output)
	}

	_ = exitCode
}

// TestSelectCommand tests the top-level select command
func TestSelectCommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	// Test select with args - will fail since no mihomo running
	exitCode := run([]string{"select", "Auto", "Direct"}, stdout, stderr)

	// Should not be unknown command error
	if strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("select command should be recognized: %s", stderr.String())
	}

	// Might fail due to connection, but command should be recognized
	_ = exitCode
}

func TestSelectMissingArgs(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"select"}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for missing args, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "requires") {
		t.Fatalf("expected 'requires' in stderr, got: %s", stderr.String())
	}
}

func TestSelectInsufficientArgs(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"select", "Auto"}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 for insufficient args, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "requires") {
		t.Fatalf("expected 'requires' in stderr, got: %s", stderr.String())
	}
}

// TestHelpCommand verifies help output includes all commands
func TestHelpCommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"help"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0 for help, got %d", exitCode)
	}

	output := stdout.String()

	// Verify all new commands are documented
	commands := []string{
		"core",
		"system-proxy",
		"groups",
		"test",
		"select",
	}

	for _, cmd := range commands {
		if !strings.Contains(output, cmd) {
			t.Fatalf("help output missing command %q: %s", cmd, output)
		}
	}
}
