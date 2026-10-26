package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestGlobalModeSelection(t *testing.T) {
	for _, tt := range []struct {
		name       string
		current    string
		match      string
		noNodes    bool
		cycle      bool
		failSelect bool
		failRules  bool
		want       []string
		wantError  bool
	}{
		{name: "default direct follows rule route", current: "DIRECT", match: "🚀 节点选择", want: []string{"select 🚀 节点选择", "mode global"}},
		{name: "empty selection follows rule route", match: "🚀 节点选择", want: []string{"select 🚀 节点选择", "mode global"}},
		{name: "preserve chosen node", current: "Node B", failRules: true, want: []string{"mode global"}},
		{name: "preserve intentional reject", current: "REJECT", want: []string{"mode global"}},
		{name: "fallback prefers automatic group over first node", current: "DIRECT", want: []string{"select Auto", "mode global"}},
		{name: "skip group pointing back to global", current: "DIRECT", match: "🚀 节点选择", cycle: true, want: []string{"select Auto", "mode global"}},
		{name: "no node keeps mode unchanged", current: "DIRECT", noNodes: true, wantError: true},
		{name: "selection failure keeps mode unchanged", current: "DIRECT", match: "🚀 节点选择", failSelect: true, want: []string{"select 🚀 节点选择"}, wantError: true},
		{name: "rule read failure keeps mode unchanged", current: "DIRECT", failRules: true, wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			proxies := map[string]Proxy{
				"GLOBAL": {Type: "Selector", Now: tt.current, All: []string{"DIRECT", "REJECT", "Node A", "Node B", "Auto", "🚀 节点选择"}},
				"DIRECT": {Type: "Direct"},
				"REJECT": {Type: "Reject"},
				"Node A": {Type: "Trojan"},
				"Node B": {Type: "Shadowsocks"},
				"Auto":   {Type: "URLTest", Now: "Node B", All: []string{"Node A", "Node B"}},
				"🚀 节点选择": {Type: "Selector", Now: "Auto", All: []string{"Auto", "Node A", "DIRECT"}},
			}
			if tt.noNodes {
				proxies["GLOBAL"] = Proxy{Type: "Selector", Now: "DIRECT", All: []string{"DIRECT", "REJECT"}}
			}
			if tt.cycle {
				proxies["🚀 节点选择"] = Proxy{Type: "Selector", Now: "GLOBAL", All: []string{"GLOBAL"}}
			}
			var mutations []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "GET /proxies":
					json.NewEncoder(w).Encode(proxiesResponse{Proxies: proxies})
				case "GET /rules":
					if tt.failRules {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"rules": []map[string]string{{"type": "Match", "proxy": tt.match}}})
				case "PUT /proxies/GLOBAL":
					var selection selectProxyRequest
					if err := json.NewDecoder(r.Body).Decode(&selection); err != nil {
						t.Error(err)
					}
					mutations = append(mutations, "select "+selection.Name)
					if tt.failSelect {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				case "PATCH /configs":
					var patch configPatchRequest
					if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
						t.Error(err)
					}
					mutations = append(mutations, "mode "+patch.Mode)
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			err := NewClient(server.URL).SetMode("global")
			if (err != nil) != tt.wantError {
				t.Fatalf("SetMode error = %v, wantError = %v", err, tt.wantError)
			}
			if !reflect.DeepEqual(mutations, tt.want) {
				t.Fatalf("mutations = %v, want %v", mutations, tt.want)
			}
		})
	}
}

func TestGlobalModeWithoutGlobalGroup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/proxies" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(proxiesResponse{Proxies: map[string]Proxy{}})
	}))
	defer server.Close()
	if err := NewClient(server.URL).SetMode("global"); err == nil || !strings.Contains(err.Error(), "mode unchanged") {
		t.Fatalf("expected missing global group error, got %v", err)
	}
}
