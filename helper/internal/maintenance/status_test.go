package maintenance

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStatusPersistenceAndSubscriptionFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "maintenance.json")
	when := time.Date(2026, 11, 12, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	status := MarkSuccess(Status{}, when)
	status = MarkAttempt(status, when.Add(time.Hour))
	status = MarkFailure(status, "timeout")
	if status.SubscriptionLastSuccess != "2026-11-12T02:00:00Z" || status.SubscriptionLastError != "timeout" {
		t.Fatalf("unexpected status after failure: %+v", status)
	}
	if err := Save(path, status); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.SubscriptionLastSuccess != status.SubscriptionLastSuccess || loaded.SubscriptionLastError != "timeout" {
		t.Fatalf("load = %+v, %v", loaded, err)
	}
}

func TestStatusDropsUnverifiedLegacyGeoSuccessAndTracksUpdateProgress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "maintenance.json")
	if err := os.WriteFile(path, []byte(`{"geoLastSuccess":"2026-11-12T02:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := Load(path)
	if err != nil || status.GeoLastObservedChange != "" {
		t.Fatalf("unverified legacy geo timestamp was exposed: %+v, %v", status, err)
	}
	attempted := MarkAttempt(status, time.Date(2026, 11, 12, 3, 0, 0, 0, time.UTC))
	if !attempted.SubscriptionInProgress {
		t.Fatal("subscription attempt was not marked in progress")
	}
	failed := MarkFailure(attempted, "network timeout")
	if failed.SubscriptionInProgress || failed.SubscriptionLastError != "network timeout" {
		t.Fatalf("failed update state is inconsistent: %+v", failed)
	}
}
