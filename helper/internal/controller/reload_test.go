package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReloadConfigUsesPayloadAndRejectsUnconfirmedResponses(t *testing.T) {
	for _, code := range []int{200, 204, 400, 500} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.URL.Path != "/configs" || r.Header.Get("Authorization") != "Bearer session" {
					t.Errorf("unexpected reload request: %s %s", r.Method, r.URL)
				}
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["payload"] != "mode: rule\n" || body["path"] != "" {
					t.Errorf("bad reload content: %v %v", body, err)
				}
				w.WriteHeader(code)
				if code >= 400 {
					_, _ = w.Write([]byte("password: secret"))
				}
			}))
			defer server.Close()
			err := NewClientWithSecret(server.URL, "session").ReloadConfig([]byte("mode: rule\n"))
			if (err == nil) != (code < 400) || (err != nil && strings.Contains(err.Error(), "secret")) {
				t.Fatalf("unexpected reload result: %v", err)
			}
		})
	}
}
