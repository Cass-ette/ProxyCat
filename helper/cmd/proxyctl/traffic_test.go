package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
	"github.com/Cass-ette/ProxyCat/helper/internal/profile"
)

func TestTrafficCommandUpdatesOnlyMetadataAndRetainsCacheOnFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "unavailable", 503)
			return
		}
		w.Header().Set("Subscription-Userinfo", "upload=10;download=20;total=100")
		w.Write([]byte("not a config"))
	}))
	defer server.Close()
	p, _ := paths.Default()
	entry := profile.Profile{ID: "plan", URL: server.URL + "?token=xzgs-traffic-fixture", Active: true}
	if _, err := profile.EnsureProfileDir(p.ProfilesDir, entry.ID); err != nil {
		t.Fatal(err)
	}
	if err := profile.SaveAll(p.ProfilesDir, []profile.Profile{entry}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ConfigYAML, []byte("unchanged config"), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"profile", "traffic", "plan", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("metadata request failed: %s", &stderr)
	}
	var result profile.TrafficSnapshot
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Traffic == nil || result.Traffic.Download != 20 {
		t.Fatalf("missing balance: %s", &stdout)
	}
	if bytes.Contains(stdout.Bytes(), []byte("xzgs-traffic-fixture")) {
		t.Fatal("metadata output leaked subscription credential")
	}
	content, _ := os.ReadFile(p.ConfigYAML)
	if string(content) != "unchanged config" {
		t.Fatal("quota request changed active config")
	}
	fail.Store(true)
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"profile", "traffic", "plan", "--json"}, &stdout, &stderr); code == 0 {
		t.Fatal("failed refresh reported success")
	}
	loaded, err := profile.LoadAll(p.ProfilesDir)
	if err != nil || loaded[0].Traffic == nil || loaded[0].Traffic.Total != 100 {
		t.Fatal("a failed refresh erased the previous usage")
	}
}
