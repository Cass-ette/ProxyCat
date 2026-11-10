package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMixedListenerRecoveryOnlyPatchesItsPort(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPatch || r.URL.Path != "/configs" || r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("unexpected recovery request")
		}
		var body map[string]int
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) != 1 || body["mixed-port"] != 12345 {
			t.Errorf("recovery changed unrelated configuration: %+v %v", body, err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	c := NewClientWithSecret(server.URL, "fixture-token")
	if err := c.SetMixedPort(context.Background(), 12345); err != nil {
		t.Fatal(err)
	}
	for _, port := range []int{0, -1, 65536} {
		if c.SetMixedPort(context.Background(), port) == nil {
			t.Fatal("accepted invalid port")
		}
	}
	if calls != 1 {
		t.Fatal("invalid port made a request")
	}
}
