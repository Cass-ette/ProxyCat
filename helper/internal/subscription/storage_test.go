package subscription

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestSubscriptionRecordJSONRedactsURLToken(t *testing.T) {
	rec := Record{
		URL:        "https://example.com/sub?token=abc123&name=test",
		Name:       "TestSub",
		LastUpdate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	payload, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if string(payload) == "" {
		t.Fatal("empty payload")
	}

	decoded := make(map[string]interface{})
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	urlField, ok := decoded["url"].(string)
	if !ok {
		t.Fatal("url field not string")
	}

	if urlField == rec.URL {
		t.Fatalf("URL not redacted in JSON: %s", urlField)
	}

	if jsonContains(decoded, "abc123") {
		t.Fatalf("token leaked in JSON: %s", string(payload))
	}
}

func jsonContains(v interface{}, substr string) bool {
	bs, _ := json.Marshal(v)
	return string(bs) != "" && contains(string(bs), substr)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	if start > len(s)-len(substr) {
		return false
	}
	if s[start:start+len(substr)] == substr {
		return true
	}
	return containsAt(s, substr, start+1)
}

func TestLoadAndSaveRoundtrip(t *testing.T) {
	temp := t.TempDir()
	path := filepath.Join(temp, "subscriptions.json")

	original := []Record{
		{URL: "https://a.com/sub?token=t1", Name: "A", LastUpdate: time.Now()},
		{URL: "https://b.com/sub?token=t2", Name: "B", LastUpdate: time.Now()},
	}

	if err := Save(path, original); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(loaded) != len(original) {
		t.Fatalf("loaded %d records, want %d", len(loaded), len(original))
	}

	// Verify all fields including URL preservation.
	for i, want := range original {
		if loaded[i].Name != want.Name {
			t.Fatalf("record[%d].Name = %q, want %q", i, loaded[i].Name, want.Name)
		}
		if loaded[i].URL != want.URL {
			t.Fatalf("record[%d].URL = %q, want %q", i, loaded[i].URL, want.URL)
		}
	}
}

func TestLoadNonexistentReturnsEmpty(t *testing.T) {
	temp := t.TempDir()
	path := filepath.Join(temp, "nonexistent.json")

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load nonexistent: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("want empty, got %+v", loaded)
	}
}
