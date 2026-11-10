package tun

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func freeRelayAddresses(t *testing.T) (string, string) {
	t.Helper()
	first, err := net.Listen("tcp4", loopback(0))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := net.Listen("tcp4", loopback(0))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	return first.Addr().String(), second.Addr().String()
}

func portNumber(address string) int {
	_, port, _ := net.SplitHostPort(address)
	n, _ := strconv.Atoi(port)
	return n
}

func TestUserRelayForwardsTrafficAndReleasesActiveSocketsOnStop(t *testing.T) {
	backend, err := net.Listen("tcp4", loopback(0))
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	udp, err := net.ListenPacket("udp4", backend.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	go func() {
		buffer := make([]byte, 1024)
		for {
			n, address, err := udp.ReadFrom(buffer)
			if err != nil {
				return
			}
			_, _ = udp.WriteTo(buffer[:n], address)
		}
	}()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := backend.Accept()
		if err == nil {
			accepted <- conn
			_, _ = io.Copy(conn, conn)
			conn.Close()
		}
	}()
	var patchedPort atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/configs" {
			if r.Method == http.MethodPatch {
				var patch map[string]int
				_ = json.NewDecoder(r.Body).Decode(&patch)
				patchedPort.Store(int32(patch["mixed-port"]))
				w.WriteHeader(http.StatusNoContent)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"mixed-port": portNumber(backend.Addr().String()), "tun": map[string]bool{"enable": true}})
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer api.Close()
	proxyAddress, apiAddress := freeRelayAddresses(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready, done := make(chan struct{}), make(chan error, 1)
	go func() {
		done <- relayLocal(ctx, proxyAddress, apiAddress, relaySettings{portNumber(backend.Addr().String()), portNumber(api.Listener.Addr().String())}, func() error { close(ready); return nil })
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("relay did not prepare")
	}
	client, err := net.DialTimeout("tcp4", proxyAddress, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = client.Write([]byte("HTTP and SOCKS bytes stay unchanged"))
	response := make([]byte, len("HTTP and SOCKS bytes stay unchanged"))
	if _, err := io.ReadFull(client, response); err != nil || string(response) != "HTTP and SOCKS bytes stay unchanged" {
		t.Fatalf("TCP relay failed: %q %v", response, err)
	}
	backendConnection := <-accepted
	defer backendConnection.Close()
	udpClient, err := net.Dial("udp4", proxyAddress)
	if err != nil {
		t.Fatal(err)
	}
	defer udpClient.Close()
	_ = udpClient.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = udpClient.Write([]byte("udp packet"))
	packet := make([]byte, 64)
	if n, err := udpClient.Read(packet); err != nil || string(packet[:n]) != "udp packet" {
		t.Fatalf("UDP relay failed: %q %v", packet[:n], err)
	}
	httpClient := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Second}
	defer httpClient.CloseIdleConnections()
	request, _ := http.NewRequest(http.MethodGet, "http://"+apiAddress+"/configs", nil)
	unauthorized, err := httpClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatal("controller authentication was bypassed")
	}
	request.Header.Set("Authorization", "Bearer test-token")
	config, err := httpClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	_ = json.NewDecoder(config.Body).Decode(&cfg)
	config.Body.Close()
	if cfg["mixed-port"] != float64(portNumber(proxyAddress)) || cfg["tun"].(map[string]interface{})["enable"] != true {
		t.Fatalf("public readiness/config changed: %v", cfg)
	}
	patchBody, _ := json.Marshal(map[string]int{"mixed-port": portNumber(proxyAddress)})
	patchRequest, _ := http.NewRequest(http.MethodPatch, "http://"+apiAddress+"/configs", bytes.NewReader(patchBody))
	patchRequest.Header.Set("Authorization", "Bearer test-token")
	patchRequest.Header.Set("Content-Type", "application/json")
	patched, err := httpClient.Do(patchRequest)
	if err != nil {
		t.Fatal(err)
	}
	patched.Body.Close()
	if patched.StatusCode != http.StatusNoContent || int(patchedPort.Load()) != portNumber(backend.Addr().String()) {
		t.Fatalf("listener recovery patch was not mapped to the private port: status=%d port=%d", patched.StatusCode, patchedPort.Load())
	}
	// Keep both the proxy tunnel and HTTP keep-alive connection open at stop.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("active traffic prevented relay shutdown")
	}
	for _, address := range []string{proxyAddress, apiAddress} {
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			t.Fatalf("ordinary core cannot immediately reclaim %s: %v", address, err)
		}
		listener.Close()
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := client.Read(packet); err == nil {
		t.Fatal("active client socket survived TUN stop")
	}
}

func TestPrivateListenersDoNotRebindPublicPortsOrChangeRouting(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(config, []byte("mixed-port: 7890\nexternal-controller: 127.0.0.1:9090\nmode: global\nrules: [MATCH,DIRECT]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	settings, release, err := preparePrivateListeners(config)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if settings.ProxyPort < 49152 || settings.ControllerPort < 49152 || settings.ProxyPort == settings.ControllerPort || settings.ProxyPort == 7890 || settings.ControllerPort == 9090 {
		t.Fatalf("private/public ports overlap: %+v", settings)
	}
	for _, port := range []int{settings.ProxyPort, settings.ControllerPort} {
		if listener, err := net.Listen("tcp4", loopback(port)); err == nil {
			listener.Close()
			t.Fatal("private port not reserved during preparation")
		}
	}
	data, _ := os.ReadFile(config)
	var cfg map[string]interface{}
	_ = yaml.Unmarshal(data, &cfg)
	if cfg["mixed-port"] != settings.ProxyPort || cfg["external-controller"] != loopback(settings.ControllerPort) || cfg["mode"] != "global" {
		t.Fatalf("incorrect private configuration: %v", cfg)
	}
	release()
	for _, port := range []int{settings.ProxyPort, settings.ControllerPort} {
		listener, err := net.Listen("tcp4", loopback(port))
		if err != nil {
			t.Fatal(err)
		}
		listener.Close()
	}
}

func TestRelayConnectionRecordsKeepOriginalApplicationSourcesAndNodeChain(t *testing.T) {
	mapping := &sourceMap{addresses: map[string]string{}}
	forget := mapping.add("tcp", "127.0.0.1:54321", "127.0.0.1:60001")
	defer forget()
	body := `{"downloadTotal":123,"connections":[{"id":"keep","rule":"Match","chains":["Node A","Main"],"metadata":{"network":"tcp","sourceIP":"127.0.0.1","sourcePort":"54321","process":"proxyctl","processPath":"/helper/proxyctl","host":"example.com"}},{"id":"tun","metadata":{"network":"udp","sourceIP":"127.0.0.1","sourcePort":"54321","process":"Browser"}}]}`
	request, _ := http.NewRequest(http.MethodGet, "http://localhost/connections", nil)
	response := &http.Response{StatusCode: 200, Request: request, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
	if err := mapping.translate(response, 7890); err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(response.Body)
	for _, expected := range []string{`"sourcePort":"60001"`, `"chains":["Node A","Main"]`, `"rule":"Match"`, `"process":"Browser"`, `"downloadTotal":123`} {
		if !bytes.Contains(data, []byte(expected)) {
			t.Fatalf("connection information lost: %s", data)
		}
	}
	if bytes.Contains(data, []byte("proxyctl")) {
		t.Fatalf("reported relay as the original application: %s", data)
	}
}

func TestRelayOccupiedPortFailureReleasesOtherPreparedListeners(t *testing.T) {
	proxy, api := freeRelayAddresses(t)
	occupied, err := net.Listen("tcp4", api)
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	err = relayLocal(context.Background(), proxy, api, relaySettings{40000, 40001}, func() error { t.Error("reported readiness before binding all ports"); return nil })
	if err == nil {
		t.Fatal("occupied controller port was accepted")
	}
	listener, err := net.Listen("tcp4", proxy)
	if err != nil {
		t.Fatalf("failed setup leaked its proxy port: %v", err)
	}
	listener.Close()
}
