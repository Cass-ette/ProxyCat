package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
	"github.com/Cass-ette/ProxyCat/helper/internal/profile"
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

func TestGroupsDelayCommandRecognized(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"groups", "delay", "--json"}, stdout, stderr)

	if strings.Contains(stderr.String(), "unknown groups subcommand") {
		t.Fatalf("groups delay should be recognized: %s", stderr.String())
	}
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

func TestAppPathFromProxyctlExecutable(t *testing.T) {
	executable := filepath.Join("/Applications", "ProxyCat.app", "Contents", "Resources", "proxyctl")
	got, err := appPathFromProxyctlExecutable(executable)
	if err != nil {
		t.Fatalf("appPathFromProxyctlExecutable returned error: %v", err)
	}
	want := filepath.Join("/Applications", "ProxyCat.app")
	if got != want {
		t.Fatalf("app path = %q, want %q", got, want)
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
		"profile",
	}

	for _, cmd := range commands {
		if !strings.Contains(output, cmd) {
			t.Fatalf("help output missing command %q: %s", cmd, output)
		}
	}
}

func TestProfileListCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"profile", "list", "--json"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}
	output := stdout.String()
	if output != "[]\n" {
		t.Fatalf("expected empty JSON array, got: %s", output)
	}
}

func TestProfileMissingSubcommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"profile"}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2", exitCode)
	}
	if !strings.Contains(stderr.String(), "subcommand required") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestProfileActivateMissingID(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"profile", "activate"}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2", exitCode)
	}
}

func TestSaveProfileSubscriptionGeneratesDistinctDefaultNames(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	firstID, err := saveProfileSubscription("https://alpha.example.com/sub?token=a")
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := saveProfileSubscription("https://alpha.example.com/other?token=b")
	if err != nil {
		t.Fatal(err)
	}
	if firstID == secondID {
		t.Fatalf("expected distinct IDs")
	}
	runtimePaths, err := paths.Default()
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := profile.LoadAll(runtimePaths.ProfilesDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("got %d profiles, want 2", len(profiles))
	}
	if profiles[0].Name == profiles[1].Name {
		t.Fatalf("profile names should be distinct: %+v", profiles)
	}
}

func TestProfileListJSONRedactsSubscriptionURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	runtimePaths, err := paths.Default()
	if err != nil {
		t.Fatal(err)
	}
	profiles := []profile.Profile{
		{
			ID:        "p1",
			Name:      "Example",
			URL:       "https://example.com/sub?token=secret-token",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	if err := profile.SaveAll(runtimePaths.ProfilesDir, profiles); err != nil {
		t.Fatal(err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"profile", "list", "--json"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}
	if strings.Contains(stdout.String(), "secret-token") {
		t.Fatalf("profile list leaked token: %s", stdout.String())
	}
	var decoded []profile.Profile
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if len(decoded) != 1 || !strings.Contains(decoded[0].URL, "redacted") {
		t.Fatalf("URL was not redacted: %+v", decoded)
	}
}

func TestProfileDeleteCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	runtimePaths, err := paths.Default()
	if err != nil {
		t.Fatal(err)
	}

	profileDir := filepath.Join(runtimePaths.ProfilesDir, "a1b2c3d4")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "config.yaml"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profilesData, _ := json.Marshal([]profile.Profile{
		{ID: "a1b2c3d4", Name: "ToDelete", URL: "https://x.com/sub"},
	})
	if err := os.WriteFile(filepath.Join(runtimePaths.ProfilesDir, "profiles.json"), profilesData, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"profile", "delete", "a1b2c3d4"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", exitCode, stderr.String())
	}
	if !strings.Contains(strings.ToLower(stdout.String()), "deleted") {
		t.Fatalf("stdout=%q", stdout.String())
	}

	data, err := os.ReadFile(filepath.Join(runtimePaths.ProfilesDir, "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	var remaining []profile.Profile
	if err := json.Unmarshal(data, &remaining); err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected 0 profiles, got %d", len(remaining))
	}
}
