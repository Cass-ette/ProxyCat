package core

import (
	"runtime"
	"testing"
)

func TestDarwinMihomoAssetName(t *testing.T) {
	asset := DarwinMihomoAssetName("v1.19.24", "arm64")
	if asset != "mihomo-darwin-arm64-v1.19.24.gz" {
		t.Fatalf("asset = %q", asset)
	}

	asset = DarwinMihomoAssetName("v1.19.24", "amd64")
	if asset != "mihomo-darwin-amd64-v1.19.24.gz" {
		t.Fatalf("asset = %q", asset)
	}
}

func TestLatestReleaseURL(t *testing.T) {
	url := LatestReleaseURL("v1.19.24", "arm64")
	want := "https://github.com/MetaCubeX/mihomo/releases/download/v1.19.24/mihomo-darwin-arm64-v1.19.24.gz"
	if url != want {
		t.Fatalf("url = %q, want %q", url, want)
	}
}

func TestCurrentDarwinArch(t *testing.T) {
	arch := CurrentDarwinArch()
	if runtime.GOARCH == "arm64" && arch != "arm64" {
		t.Fatalf("arch = %q", arch)
	}
	if runtime.GOARCH == "amd64" && arch != "amd64" {
		t.Fatalf("arch = %q", arch)
	}
}
