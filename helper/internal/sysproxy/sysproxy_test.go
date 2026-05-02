package sysproxy

import (
	"runtime"
	"testing"
)

func TestStatusNotDarwin(t *testing.T) {
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
