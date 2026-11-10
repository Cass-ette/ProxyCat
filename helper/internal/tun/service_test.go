package tun

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSystemHelperAuthorizationPersistsAcrossClients(t *testing.T) {
	var installed atomic.Bool
	var authorizations atomic.Int32
	var prepared atomic.Int32
	handler := serviceHandler(func(directory string) error {
		if directory != "/private/tmp/proxycat-tun-test" {
			t.Errorf("session changed: %q", directory)
		}
		prepared.Add(1)
		return nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !installed.Load() {
			http.Error(w, "not installed", http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	// Fresh client instances model separate app runs; installation is performed once.
	for i := 0; i < 3; i++ {
		client := server.Client()
		client.Transport = roundTripToServer{server.URL, http.DefaultTransport}
		err := ensureService(client, func() error { authorizations.Add(1); installed.Store(true); return nil })
		if err != nil {
			t.Fatal(err)
		}
		if err := serviceCall(client, http.MethodPost, "/prepare", map[string]string{"directory": "/private/tmp/proxycat-tun-test"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if authorizations.Load() != 1 || prepared.Load() != 3 {
		t.Fatalf("authorizations=%d sessions=%d", authorizations.Load(), prepared.Load())
	}
}

type roundTripToServer struct {
	url      string
	delegate http.RoundTripper
}

func (s roundTripToServer) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	target, _ := http.NewRequest(request.Method, s.url+request.URL.Path, nil)
	cloned.URL = target.URL
	return s.delegate.RoundTrip(cloned)
}

func TestCancelledHelperInstallationDoesNotPrepareSession(t *testing.T) {
	var preparations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/prepare" {
			preparations.Add(1)
		}
		http.Error(w, "not installed", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client := server.Client()
	client.Transport = roundTripToServer{server.URL, http.DefaultTransport}
	cancelled := errors.New("cancelled authorization")
	if err := ensureService(client, func() error { return cancelled }); !errors.Is(err, cancelled) {
		t.Fatalf("lost cancellation: %v", err)
	}
	if preparations.Load() != 0 {
		t.Fatal("cancelled installation launched a session")
	}
}

func TestOutdatedSupervisorIsUpgradedOnceBeforeReuse(t *testing.T) {
	var upgrades atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if upgrades.Load() == 0 {
			_ = json.NewEncoder(w).Encode(map[string]int{"protocol": serviceProtocol})
			return
		}
		_ = json.NewEncoder(w).Encode(currentServiceHealth())
	}))
	defer server.Close()
	client := server.Client()
	client.Transport = roundTripToServer{server.URL, http.DefaultTransport}
	for i := 0; i < 2; i++ {
		if err := ensureService(client, func() error { upgrades.Add(1); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if upgrades.Load() != 1 {
		t.Fatalf("helper update authorizations: %d", upgrades.Load())
	}
}

func TestRelayServiceKeepsLegacyProtocolAndAdvertisesCapability(t *testing.T) {
	var health serviceHealth
	server := httptest.NewServer(serviceHandler(func(string) error { return nil }))
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	if health.Protocol != 2 || len(health.Capabilities) != 1 || health.Capabilities[0] != serviceCapabilityUserOwnedRelay {
		t.Fatalf("legacy protocol or relay capability missing: %+v", health)
	}
}

func TestDaemonPlistUsesLaunchdWithoutBackgroundShell(t *testing.T) {
	settings := serviceSettings{UID: 501, GID: 20, Home: "/Users/test"}
	path := filepath.Join(t.TempDir(), "helper.plist")
	data := launchDaemonPlist(settings, "/Library/PrivilegedHelperTools/Proxy Cat & Test/proxyctl")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "nohup") || strings.Contains(text, "/bin/sh") || !strings.Contains(text, "KeepAlive") || !strings.Contains(text, "&amp;") {
		t.Fatalf("wrong launch configuration: %s", text)
	}
	// Parsing is local; launchctl is never called from unit tests.
	if !strings.Contains(text, "<string>service</string><string>501</string>") {
		t.Fatal("missing daemon command")
	}
}

func TestSystemServicePreparationDoesNotLaunchTheKernel(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "pcservice-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "daemon.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	var preparations atomic.Int32
	server := &http.Server{Handler: serviceHandler(func(string) error { preparations.Add(1); return nil })}
	go server.Serve(listener)
	defer server.Close()
	client := serviceClient(socket)
	if !serviceAvailable(client) {
		t.Fatal("service health unavailable")
	}
	if err := serviceCall(client, http.MethodPost, "/prepare", map[string]string{"directory": "/private/tmp/proxycat-tun-test"}, nil); err != nil {
		t.Fatal(err)
	}
	if preparations.Load() != 1 {
		t.Fatal("supervisor was not prepared")
	}
	if err := serviceCall(client, http.MethodPost, "/start", nil, nil); err == nil {
		t.Fatal("daemon directly accepted kernel start")
	}
}

func TestServiceSessionUsesInstalledKernelAndFixedUserPaths(t *testing.T) {
	dir, err := os.MkdirTemp("/private/tmp", "proxycat-tun-service-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	home := t.TempDir()
	settings := serviceSettings{UID: os.Getuid(), GID: os.Getgid(), Home: home}
	s := session{Directory: dir, Token: strings.Repeat("a", 64), UID: os.Getuid(), GID: os.Getgid(), ParentPID: os.Getpid(), ParentStart: processStart(os.Getpid()), Binary: "ignored", LogPath: "ignored", DataDirectory: "ignored"}
	b, _ := json.Marshal(s)
	if err := os.WriteFile(filepath.Join(dir, "session.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	actual, err := readServiceSession(dir, settings)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Binary != filepath.Join(serviceDirectory(os.Getuid()), "mihomo") || actual.LogPath != filepath.Join(home, "Library/Application Support/ProxyCat/logs/mihomo.log") || actual.DataDirectory != filepath.Join(home, ".config/mihomo") {
		t.Fatalf("session did not use installed settings: %+v", actual)
	}
}

func TestPreparedSystemSessionExitsWithoutStart(t *testing.T) {
	var launches atomic.Int32
	s, done := preparedSupervisor(t, func() bool { return true }, &launches)
	if err := request(s, http.MethodPost, "/stop"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("session did not exit")
	}
	if launches.Load() != 0 {
		t.Fatal("preparing a session launched a core")
	}
}

func TestHelperRegistrationFailureRestoresExistingInstall(t *testing.T) {
	dir := t.TempDir()
	destination, stage, plist := filepath.Join(dir, "helper"), filepath.Join(dir, "stage"), filepath.Join(dir, "job.plist")
	for _, p := range []string{destination, stage} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(destination, "identity"), []byte("old"), 0600)
	os.WriteFile(filepath.Join(stage, "identity"), []byte("new"), 0600)
	os.WriteFile(plist, []byte("old plist"), 0600)
	starts, stops := 0, 0
	err := replaceServiceBundle(stage, destination, plist, []byte("new plist"), func() error { stops++; return nil }, func() error {
		starts++
		if starts == 1 {
			return errors.New("bootstrap failed")
		}
		return nil
	})
	identity, _ := os.ReadFile(filepath.Join(destination, "identity"))
	content, _ := os.ReadFile(plist)
	if err == nil || string(identity) != "old" || string(content) != "old plist" || starts != 2 || stops != 1 {
		t.Fatalf("rollback failed: error=%v helper=%q plist=%q starts=%d stops=%d", err, identity, content, starts, stops)
	}
}

func TestFirstHelperRegistrationFailureRemovesIncompleteInstall(t *testing.T) {
	dir := t.TempDir()
	destination, stage, plist := filepath.Join(dir, "helper"), filepath.Join(dir, "stage"), filepath.Join(dir, "job.plist")
	os.Mkdir(stage, 0700)
	err := replaceServiceBundle(stage, destination, plist, []byte("new plist"), func() error { t.Fatal("stopped a nonexistent helper"); return nil }, func() error { return errors.New("bootstrap failed") })
	if err == nil {
		t.Fatal("registration failure ignored")
	}
	for _, p := range []string{destination, plist} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("incomplete install retained: %s %v", p, err)
		}
	}
}
