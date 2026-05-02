package sysproxy

import (
	"runtime"
	"testing"
)

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
  HTTPHost : 127.0.0.1
  HTTPPort : 8080
  HTTPSEnable : 1
  HTTPSHost : 127.0.0.1
  HTTPSPort : 8080
  SOCKSEnable : 1
  SOCKSHost : 127.0.0.1
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
  HTTPHost : (null)
  HTTPPort : 0
  HTTPSEnable : 0
  HTTPSHost : (null)
  HTTPSPort : 0
  SOCKSEnable : 0
  SOCKSHost : (null)
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
  HTTPHost : 192.168.1.1
  HTTPPort : 3128
  HTTPSEnable : 0
  HTTPSHost : (null)
  HTTPSPort : 0
  SOCKSEnable : 1
  SOCKSHost : 10.0.0.1
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
  HTTPHost : 127.0.0.1
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
