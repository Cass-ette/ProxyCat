package profile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	profiles := []Profile{
		{ID: "a1", Name: "Airport A", URL: "https://a.com/sub", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "b2", Name: "Airport B", URL: "https://b.com/sub", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	if err := SaveAll(dir, profiles); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}
	loaded, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("got %d profiles, want 2", len(loaded))
	}
	if loaded[0].ID != "a1" || loaded[1].Name != "Airport B" {
		t.Fatalf("unexpected: %+v", loaded)
	}
}

func TestLoadAllEmpty(t *testing.T) {
	dir := t.TempDir()
	loaded, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("got %d, want 0", len(loaded))
	}
}

func TestFindByURL(t *testing.T) {
	profiles := []Profile{
		{ID: "a1", URL: "https://a.com/sub"},
		{ID: "b2", URL: "https://b.com/sub"},
	}
	found := FindByURL(profiles, "https://b.com/sub")
	if found == nil || found.ID != "b2" {
		t.Fatalf("FindByURL = %+v", found)
	}
	if FindByURL(profiles, "https://c.com/sub") != nil {
		t.Fatalf("FindByURL should return nil for unknown URL")
	}
}

func TestActivateWritesConfig(t *testing.T) {
	dir := t.TempDir()
	profilesDir := filepath.Join(dir, "profiles")
	activeConfig := filepath.Join(dir, "config.yaml")

	p := Profile{ID: "a1", Name: "Test", URL: "https://a.com/sub"}
	profileDir := filepath.Join(profilesDir, p.ID)
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	profileConfig := filepath.Join(profileDir, "config.yaml")
	if err := os.WriteFile(profileConfig, []byte("mixed-port: 7890\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveAll(profilesDir, []Profile{p}); err != nil {
		t.Fatal(err)
	}

	if err := Activate(profilesDir, p.ID, activeConfig); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	data, err := os.ReadFile(activeConfig)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "mixed-port: 7890\n" {
		t.Fatalf("active config = %q", string(data))
	}
	loaded, err := LoadAll(profilesDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || !loaded[0].Active {
		t.Fatalf("active profile not marked: %+v", loaded)
	}
}

func TestNextID(t *testing.T) {
	dir := t.TempDir()
	id1 := NextID(dir)
	if id1 == "" {
		t.Fatalf("NextID returned empty")
	}
	profiles := []Profile{{ID: id1, Name: "First", URL: "https://a.com"}}
	if err := SaveAll(dir, profiles); err != nil {
		t.Fatal(err)
	}
	id2 := NextID(dir)
	if id2 == id1 {
		t.Fatalf("NextID returned same ID %q", id2)
	}
}
