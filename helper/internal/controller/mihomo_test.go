package controller

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Optional integration check with a real, isolated Mihomo. All traffic and
// controller requests stay on loopback; this never changes the system proxy.
func TestMihomoRoutingModes(t *testing.T) {
	binary := os.Getenv("PROXYCAT_TEST_MIHOMO")
	if binary == "" {
		t.Skip("set PROXYCAT_TEST_MIHOMO to run isolated kernel integration test")
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "direct route")
	}))
	defer origin.Close()
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "proxy route")
	}))
	defer node.Close()
	nodeURL, _ := url.Parse(node.URL)
	reserve := func() net.Listener {
		t.Helper()
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		return listener
	}
	controllerListener, proxyListener := reserve(), reserve()
	controllerAddress := controllerListener.Addr().String()
	proxyPort := proxyListener.Addr().(*net.TCPAddr).Port
	controllerListener.Close()
	proxyListener.Close()
	dir := t.TempDir()
	config := fmt.Sprintf(`mixed-port: %d
external-controller: %s
bind-address: 127.0.0.1
allow-lan: false
mode: rule
log-level: silent
profile:
  store-selected: false
proxies:
  - name: Test Node
    type: http
    server: 127.0.0.1
    port: %s
proxy-groups:
  - name: Rule Proxy
    type: select
    proxies: [Test Node, DIRECT]
rules:
  - MATCH,Rule Proxy
`, proxyPort, controllerAddress, nodeURL.Port())
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	command := exec.Command(binary, "-d", dir, "-f", configPath)
	command.Stdout, command.Stderr = &log, &log
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Process.Kill(); command.Wait() })
	client := NewClient("http://" + controllerAddress)
	client.SetClient(&http.Client{Timeout: time.Second})
	ready := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if _, err := client.GetConfig(); err == nil {
			ready = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ready {
		t.Fatal("isolated Mihomo did not become ready")
	}
	localProxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
	transport := &http.Transport{Proxy: http.ProxyURL(localProxy), DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	for _, mode := range []string{"rule", "global", "direct", "rule", "global"} {
		if err := client.SetMode(mode); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		resp, err := httpClient.Get(origin.URL + "/route")
		if err != nil {
			t.Fatalf("%s route: %v", mode, err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		want := "proxy route"
		if mode == "direct" {
			want = "direct route"
		}
		if err != nil || string(body) != want {
			t.Fatalf("%s route = %q, want %q (error %v)", mode, body, want, err)
		}
		proxies, err := client.GetProxies()
		if err != nil || proxies["Rule Proxy"].Now != "Test Node" {
			t.Fatalf("%s changed rule selection: %+v (error %v)", mode, proxies["Rule Proxy"], err)
		}
	}
}
