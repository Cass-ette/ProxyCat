package profile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/subscription"
)

func TestTrafficCacheCannotOverwriteOrCrossProfiles(t *testing.T) {
	dir := t.TempDir()
	p := Profile{ID: "first", Name: "Plan", URL: "https://example.invalid/subscribe?token=private-value", Active: true}
	if _, err := EnsureProfileDir(dir, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := SaveAll(dir, []Profile{p}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, indexFile))
	if _, err := SaveTraffic(dir, p, &subscription.Traffic{Upload: 1, Download: 2, Total: 100}, time.Now()); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, indexFile))
	if !bytes.Equal(before, after) {
		t.Fatal("background metadata wrote the profile index")
	}
	cache, _ := os.ReadFile(filepath.Join(dir, p.ID, "traffic.json"))
	if bytes.Contains(cache, []byte("private-value")) {
		t.Fatal("traffic cache exposed the subscription credential")
	}
	loaded, err := LoadAll(dir)
	if err != nil || loaded[0].Traffic == nil || loaded[0].Traffic.Total != 100 || loaded[0].TrafficCheckedAt == nil {
		t.Fatalf("missing cached metadata: %+v %v", loaded, err)
	}
	p.URL = "https://example.invalid/a-different-plan"
	if err := SaveAll(dir, []Profile{p}); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadAll(dir)
	if err != nil || loaded[0].Traffic != nil {
		t.Fatal("a different subscription inherited the old balance")
	}
}

func TestUnsupportedMetadataClearsOldBalanceAndKeepsCheckTime(t *testing.T) {
	dir := t.TempDir()
	p := Profile{ID: "first", URL: "https://example.invalid/sub"}
	if _, err := EnsureProfileDir(dir, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := SaveAll(dir, []Profile{p}); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTraffic(dir, p, &subscription.Traffic{Total: 100}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTraffic(dir, p, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAll(dir)
	if err != nil || loaded[0].Traffic != nil || loaded[0].TrafficCheckedAt == nil {
		t.Fatalf("unsupported metadata displayed a stale known balance: %+v %v", loaded, err)
	}
}
