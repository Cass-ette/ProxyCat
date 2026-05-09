//go:build windows

package sysproxy

import "testing"

func TestWindowsProxyRegistryRoundTrip(t *testing.T) {
	if err := Enable(7890); err != nil {
		t.Fatalf("Enable: %v", err)
	}

	status, err := GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if !status.HTTPEnabled {
		t.Fatal("expected HTTP proxy enabled")
	}
	if status.HTTPPort != 7890 {
		t.Fatalf("expected port 7890, got %d", status.HTTPPort)
	}

	if err := Disable(); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	status2, err := GetStatus()
	if err != nil {
		t.Fatalf("GetStatus after disable: %v", err)
	}
	if status2.HTTPEnabled {
		t.Fatal("expected HTTP proxy disabled")
	}
}
