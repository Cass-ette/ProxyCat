package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConnectionProcessLookupOnlyChangesAttribution(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPatch || r.URL.Path != "/configs" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected preparation request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body) != 1 || body["find-process-mode"] != "always" {
			t.Errorf("preparation changed more than attribution: %+v", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if err := NewClientWithSecret(server.URL, "test-token").EnableConnectionProcessLookup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("unexpected extra requests: %d", calls)
	}
	client := NewClient(server.URL)
	client.client.Transport = connectionTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	if err := client.EnableConnectionProcessLookup(context.Background()); err == nil {
		t.Fatal("ignored rejected preparation")
	}
}

func TestConnectionSnapshotUsesObservedRoutesAndAppBundles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/connections" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected connection request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(401)
			return
		}
		fmt.Fprint(w, `{"connections":[
			{"id":"a","start":"2026-11-10T01:00:00Z","metadata":{"process":"Browser Helper","processPath":"/Users/private-user/Applications/Browser.app/Contents/Frameworks/Helper.app/Contents/MacOS/Helper","host":"example.com","destinationPort":443,"network":"tcp","type":"Tun"},"chains":["Singapore A","Auto","Proxy"],"rule":"Match","rulePayload":""},
			{"id":"b","metadata":{"process":"curl","destinationIP":"203.0.113.1","destinationPort":"80","network":"tcp"},"chains":["DIRECT"],"rule":"GeoIP","rulePayload":"CN"},
			{"id":"c","metadata":{"host":"blocked.invalid"},"chains":["REJECT"],"rule":"DomainSuffix","rulePayload":"blocked.invalid"},
			{"id":"d","metadata":{},"chains":[]},
			{"id":"e","metadata":{},"chains":["PASS"]}
		]}`)
	}))
	defer server.Close()
	result, err := NewClientWithSecret(server.URL, "test-token").GetConnections(context.Background())
	if err != nil || len(result.Connections) != 5 {
		t.Fatalf("%+v %v", result, err)
	}
	if _, err := time.Parse(time.RFC3339Nano, result.ObservedAt); err != nil {
		t.Fatal(err)
	}
	proxy, direct, rejected, unknown, pass := result.Connections[0], result.Connections[1], result.Connections[2], result.Connections[3], result.Connections[4]
	if proxy.App != "Browser" || proxy.Route != "proxy" || proxy.Node != "Singapore A" || proxy.DestinationPort != "443" || proxy.Rule != "Match" {
		t.Fatalf("incorrect proxy record: %+v", proxy)
	}
	if direct.App != "curl" || direct.Host != "203.0.113.1" || direct.Route != "direct" || direct.RulePayload != "CN" {
		t.Fatalf("incorrect direct record: %+v", direct)
	}
	if rejected.Route != "reject" || unknown.Route != "unknown" || pass.Route != "unknown" || unknown.App != "" {
		t.Fatal("guessed a route or an application")
	}
	encoded, _ := json.Marshal(result)
	for _, private := range []string{"private-user", "/Applications/", "processPath", "sourceIP", "test-token"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("unnecessary private field exported: %s", private)
		}
	}
}

func TestConnectionSnapshotKeepsSocketsSeparateWithoutIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"connections":[
			{"metadata":{"host":"example.com","sourceIP":"127.0.0.1","sourcePort":"1","destinationPort":"443"}},
			{"metadata":{"host":"example.com","sourceIP":"127.0.0.1","sourcePort":"2","destinationPort":"443"}}
		]}`)
	}))
	defer server.Close()
	client := NewClient(server.URL)
	first, err := client.GetConnections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.GetConnections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Connections[0].ID == first.Connections[1].ID || first.Connections[0].ID != second.Connections[0].ID {
		t.Fatal("fallback identity lost socket separation or stability")
	}
}

func TestConnectionSnapshotFailuresAreNotEmptySuccess(t *testing.T) {
	for _, body := range []string{"", `{"connections":`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		_, err := NewClient(server.URL).GetConnections(context.Background())
		server.Close()
		if err == nil {
			t.Fatal("accepted malformed controller response")
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	client := NewClient(server.URL)
	if _, err := client.GetConnections(context.Background()); err == nil {
		t.Fatal("accepted unavailable controller")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.GetConnections(ctx); err == nil {
		t.Fatal("ignored cancelled observation")
	}
}

func TestConnectionAppFallbacks(t *testing.T) {
	for _, test := range []struct{ path, process, want string }{
		{"/usr/bin/curl", "", "curl"}, {"", "/usr/bin/curl", "curl"}, {"", "", ""},
		{"/Applications/ProxyCat.app/Contents/MacOS/ProxyCat", "ProxyCat", "ProxyCat"},
	} {
		if got := connectionApp(test.path, test.process); got != test.want {
			t.Errorf("%q: got %q", test.path, got)
		}
	}
}
