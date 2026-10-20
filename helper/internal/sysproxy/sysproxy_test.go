package sysproxy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fakeNetworksetup(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "commands.log")
	t.Setenv("PROXYCAT_TEST_COMMAND_LOG", logPath)
	if err := os.WriteFile(filepath.Join(dir, "networksetup"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func TestDisableCleansAllServicesAfterFailure(t *testing.T) {
	logPath := fakeNetworksetup(t, `
if [ "$1" = "-listallnetworkservices" ]; then
    printf 'An asterisk (*) denotes that a network service is disabled.\nWi-Fi\nUSB LAN\n*Thunderbolt Bridge\n'
    exit 0
fi
printf '%s|%s|%s\n' "$1" "$2" "$3" >> "$PROXYCAT_TEST_COMMAND_LOG"
if [ "$1" = "-setwebproxystate" ] && [ "$2" = "Wi-Fi" ]; then
    echo 'Error: configuration unavailable'
fi
if [ "$1" = "-setsecurewebproxystate" ] && [ "$2" = "USB LAN" ]; then
    echo 'permission denied' >&2
    exit 1
fi
`)
	err := disable(context.Background())
	if err == nil || !strings.Contains(err.Error(), "configuration unavailable") || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected both failures, got %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var expected []string
	for _, service := range []string{"Wi-Fi", "USB LAN", "Thunderbolt Bridge"} {
		for _, flag := range []string{"-setwebproxystate", "-setsecurewebproxystate", "-setsocksfirewallproxystate"} {
			expected = append(expected, flag+"|"+service+"|off")
		}
	}
	if got := strings.TrimSpace(string(data)); got != strings.Join(expected, "\n") {
		t.Fatalf("cleanup did not cover all proxy types and services:\n%s", got)
	}
}

func TestDisableSucceedsWhenAlreadyDisabled(t *testing.T) {
	fakeNetworksetup(t, `
if [ "$1" = "-listallnetworkservices" ]; then
    printf 'An asterisk (*) denotes that a network service is disabled.\nWi-Fi\n'
fi
`)
	for range 2 {
		if err := disable(context.Background()); err != nil {
			t.Fatalf("repeated cleanup failed: %v", err)
		}
	}
}

func TestDisableDeadline(t *testing.T) {
	fakeNetworksetup(t, `
if [ "$1" = "-listallnetworkservices" ]; then
    printf 'An asterisk (*) denotes that a network service is disabled.\nWi-Fi\n'
    exit 0
fi
exec /bin/sleep 10
`)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := disable(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("cleanup did not respect its deadline")
	}
}

func TestParseScutilOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected *Status
	}{
		{
			name: "all proxies enabled",
			output: `<dictionary> {
  HTTPEnable : 1
  HTTPProxy : 127.0.0.1
  HTTPPort : 8080
  HTTPSEnable : 1
  HTTPSProxy : 127.0.0.1
  HTTPSPort : 8080
  SOCKSEnable : 1
  SOCKSProxy : 127.0.0.1
  SOCKSPort : 8080
}`,
			expected: &Status{
				HTTPEnabled:  true,
				HTTPHost:     "127.0.0.1",
				HTTPPort:     8080,
				HTTPSEnabled: true,
				HTTPSHost:    "127.0.0.1",
				HTTPSPort:    8080,
				SOCKSEnabled: true,
				SOCKSHost:    "127.0.0.1",
				SOCKSPort:    8080,
			},
		},
		{
			name: "all proxies disabled",
			output: `<dictionary> {
  HTTPEnable : 0
  HTTPProxy : (null)
  HTTPPort : 0
  HTTPSEnable : 0
  HTTPSProxy : (null)
  HTTPSPort : 0
  SOCKSEnable : 0
  SOCKSProxy : (null)
  SOCKSPort : 0
}`,
			expected: &Status{
				HTTPEnabled:  false,
				HTTPHost:     "",
				HTTPPort:     0,
				HTTPSEnabled: false,
				HTTPSHost:    "",
				HTTPSPort:    0,
				SOCKSEnabled: false,
				SOCKSHost:    "",
				SOCKSPort:    0,
			},
		},
		{
			name: "mixed state",
			output: `<dictionary> {
  HTTPEnable : 1
  HTTPProxy : 192.168.1.1
  HTTPPort : 3128
  HTTPSEnable : 0
  HTTPSProxy : (null)
  HTTPSPort : 0
  SOCKSEnable : 1
  SOCKSProxy : 10.0.0.1
  SOCKSPort : 1080
}`,
			expected: &Status{
				HTTPEnabled:  true,
				HTTPHost:     "192.168.1.1",
				HTTPPort:     3128,
				HTTPSEnabled: false,
				HTTPSHost:    "",
				HTTPSPort:    0,
				SOCKSEnabled: true,
				SOCKSHost:    "10.0.0.1",
				SOCKSPort:    1080,
			},
		},
		{
			name: "missing fields",
			output: `<dictionary> {
  HTTPEnable : 1
  HTTPProxy : 127.0.0.1
  HTTPPort : 8080
}`,
			expected: &Status{
				HTTPEnabled:  true,
				HTTPHost:     "127.0.0.1",
				HTTPPort:     8080,
				HTTPSEnabled: false,
				HTTPSHost:    "",
				HTTPSPort:    0,
				SOCKSEnabled: false,
				SOCKSHost:    "",
				SOCKSPort:    0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := parseScutilOutput(tt.output)
			if err != nil {
				t.Fatalf("parseScutilOutput failed: %v", err)
			}

			if status.HTTPEnabled != tt.expected.HTTPEnabled {
				t.Errorf("HTTPEnabled = %v, want %v", status.HTTPEnabled, tt.expected.HTTPEnabled)
			}
			if status.HTTPHost != tt.expected.HTTPHost {
				t.Errorf("HTTPHost = %s, want %s", status.HTTPHost, tt.expected.HTTPHost)
			}
			if status.HTTPPort != tt.expected.HTTPPort {
				t.Errorf("HTTPPort = %d, want %d", status.HTTPPort, tt.expected.HTTPPort)
			}
			if status.HTTPSEnabled != tt.expected.HTTPSEnabled {
				t.Errorf("HTTPSEnabled = %v, want %v", status.HTTPSEnabled, tt.expected.HTTPSEnabled)
			}
			if status.HTTPSHost != tt.expected.HTTPSHost {
				t.Errorf("HTTPSHost = %s, want %s", status.HTTPSHost, tt.expected.HTTPSHost)
			}
			if status.HTTPSPort != tt.expected.HTTPSPort {
				t.Errorf("HTTPSPort = %d, want %d", status.HTTPSPort, tt.expected.HTTPSPort)
			}
			if status.SOCKSEnabled != tt.expected.SOCKSEnabled {
				t.Errorf("SOCKSEnabled = %v, want %v", status.SOCKSEnabled, tt.expected.SOCKSEnabled)
			}
			if status.SOCKSHost != tt.expected.SOCKSHost {
				t.Errorf("SOCKSHost = %s, want %s", status.SOCKSHost, tt.expected.SOCKSHost)
			}
			if status.SOCKSPort != tt.expected.SOCKSPort {
				t.Errorf("SOCKSPort = %d, want %d", status.SOCKSPort, tt.expected.SOCKSPort)
			}
		})
	}
}

func TestParseScutilOutputEmpty(t *testing.T) {
	_, err := parseScutilOutput("")
	if err == nil {
		t.Error("Expected error for empty output, got nil")
	}
}

func TestGetStatusNotDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("skipping on Darwin")
	}

	_, err := GetStatus()
	if err == nil {
		t.Fatal("expected error on non-Darwin platform")
	}
}

func TestEnableNotDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("skipping on Darwin")
	}

	err := Enable(7890)
	if err == nil {
		t.Fatal("expected error on non-Darwin platform")
	}
}

func TestDisableNotDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("skipping on Darwin")
	}

	err := Disable()
	if err == nil {
		t.Fatal("expected error on non-Darwin platform")
	}
}
