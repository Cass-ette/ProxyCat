package tun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Only this unprivileged process binds the public endpoints. The root core
// uses private, session-specific ports, so its closing TCP connections cannot
// prevent the ordinary user core from reclaiming 7890/9090 on macOS.
type relaySettings struct {
	ProxyPort      int `json:"proxyPort"`
	ControllerPort int `json:"controllerPort"`
}

func loopback(port int) string { return net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) }

func RunRelay(input io.Reader, output io.Writer) error {
	if os.Geteuid() == 0 || !rootSupervisorParent(os.Getppid()) {
		return fmt.Errorf("TUN local relay must be started by the authorized system helper")
	}
	var settings relaySettings
	if err := json.NewDecoder(io.LimitReader(input, 4096)).Decode(&settings); err != nil {
		return err
	}
	for _, port := range []int{settings.ProxyPort, settings.ControllerPort} {
		if port < 49152 || port > 65535 || port == 7890 || port == 9090 {
			return fmt.Errorf("invalid private TUN port")
		}
	}
	if settings.ProxyPort == settings.ControllerPort {
		return fmt.Errorf("duplicate private TUN port")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer cancel()
	return relayLocal(ctx, loopback(7890), loopback(9090), settings, func() error {
		_, err := io.WriteString(output, "ready\n")
		return err
	})
}

func rootSupervisorParent(pid int) bool {
	if pid < 2 {
		return false
	}
	output, err := exec.Command("/bin/ps", "-p", strconv.Itoa(pid), "-o", "uid=").Output()
	return err == nil && strings.TrimSpace(string(output)) == "0"
}

type sourceMap struct {
	sync.RWMutex
	addresses map[string]string
}

func (m *sourceMap) add(network, backend, original string) func() {
	backend = network + "|" + backend
	m.Lock()
	m.addresses[backend] = original
	m.Unlock()
	return func() { m.Lock(); delete(m.addresses, backend); m.Unlock() }
}

func relayLocal(ctx context.Context, proxyAddress, apiAddress string, settings relaySettings, ready func() error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	proxy, err := net.Listen("tcp4", proxyAddress)
	if err != nil {
		return fmt.Errorf("代理入口：%w", err)
	}
	defer proxy.Close()
	udp, err := net.ListenPacket("udp4", proxyAddress)
	if err != nil {
		return fmt.Errorf("UDP 代理入口：%w", err)
	}
	defer udp.Close()
	api, err := net.Listen("tcp4", apiAddress)
	if err != nil {
		return fmt.Errorf("控制入口：%w", err)
	}
	defer api.Close()
	var workers sync.WaitGroup
	defer workers.Wait()
	// Closing listeners alone leaves active proxy tunnels behind. Cancel all
	// accepted sockets as well, before acknowledging a completed TUN stop.
	defer cancel()
	sources := &sourceMap{addresses: map[string]string{}}
	target, _ := url.Parse("http://" + loopback(settings.ControllerPort))
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext}
	defer transport.CloseIdleConnections()
	reverse := httputil.NewSingleHostReverseProxy(target)
	reverse.Transport = transport
	_, port, _ := net.SplitHostPort(proxy.Addr().String())
	publicPort, _ := strconv.Atoi(port)
	direct := reverse.Director
	reverse.Director = func(request *http.Request) {
		direct(request)
		// Keep PATCH semantics consistent with the public GET /configs view.
		// Mode/process-discovery patches pass through unchanged.
		if request.Method == http.MethodPatch && request.URL.Path == "/configs" && request.Body != nil {
			data, err := io.ReadAll(request.Body)
			request.Body.Close()
			var patch map[string]json.RawMessage
			if err == nil && json.Unmarshal(data, &patch) == nil {
				var requested int
				if json.Unmarshal(patch["mixed-port"], &requested) == nil && requested == publicPort {
					patch["mixed-port"], _ = json.Marshal(settings.ProxyPort)
					data, _ = json.Marshal(patch)
				}
			}
			request.Body = io.NopCloser(strings.NewReader(string(data)))
			request.ContentLength = int64(len(data))
		}
	}
	reverse.ModifyResponse = func(response *http.Response) error {
		return sources.translate(response, publicPort)
	}
	server := &http.Server{Handler: reverse, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	failures := make(chan error, 3)
	workers.Add(3)
	go func() {
		defer workers.Done()
		failures <- server.Serve(api)
	}()
	go func() {
		defer workers.Done()
		failures <- relayUDP(ctx, udp, loopback(settings.ProxyPort), sources)
	}()
	go func() {
		defer workers.Done()
		for {
			incoming, err := proxy.Accept()
			if err != nil {
				failures <- err
				return
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				relayTCP(ctx, incoming, loopback(settings.ProxyPort), sources)
			}()
		}
	}()
	// A separate closer unblocks Accept, UDP reads and active HTTP requests.
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		<-ctx.Done()
		proxy.Close()
		udp.Close()
		server.Close()
	}()
	defer func() { cancel(); <-closed }()
	if err := ready(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return nil
	case err := <-failures:
		return err
	}
}

func relayTCP(ctx context.Context, incoming net.Conn, target string, sources *sourceMap) {
	defer incoming.Close()
	backend, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp4", target)
	if err != nil {
		return
	}
	defer backend.Close()
	forget := sources.add("tcp", backend.LocalAddr().String(), incoming.RemoteAddr().String())
	defer forget()
	stop := context.AfterFunc(ctx, func() { incoming.Close(); backend.Close() })
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(backend, incoming)
		if tcp, ok := backend.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()
	_, _ = io.Copy(incoming, backend)
	// Preserve a completed response but release the opposite blocked copy.
	incoming.Close()
	backend.Close()
	<-done
}

func relayUDP(ctx context.Context, listener net.PacketConn, target string, sources *sourceMap) error {
	var mu sync.Mutex
	peers := map[string]net.Conn{}
	var readers sync.WaitGroup
	defer readers.Wait()
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		for _, peer := range peers {
			peer.Close()
		}
	}()
	buffer := make([]byte, 65535)
	for {
		n, address, err := listener.ReadFrom(buffer)
		if err != nil {
			return err
		}
		key := address.String()
		mu.Lock()
		peer := peers[key]
		if peer == nil && len(peers) < 1024 {
			peer, err = (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "udp4", target)
			if err == nil {
				peers[key] = peer
				readers.Add(1)
				go func(conn net.Conn, remote net.Addr) {
					defer readers.Done()
					defer conn.Close()
					forget := sources.add("udp", conn.LocalAddr().String(), remote.String())
					defer forget()
					defer func() { mu.Lock(); delete(peers, remote.String()); mu.Unlock() }()
					packet := make([]byte, 65535)
					for {
						_ = conn.SetReadDeadline(time.Now().Add(time.Minute))
						n, err := conn.Read(packet)
						if err != nil {
							return
						}
						_, _ = listener.WriteTo(packet[:n], remote)
					}
				}(peer, address)
			}
		}
		mu.Unlock()
		if peer != nil {
			_, _ = peer.Write(buffer[:n])
		}
	}
}

// Restore source addresses before the connection UI resolves application names.
// Root Mihomo sees the relay's private socket; that must not be reported as the
// user's application. Other metadata, rules and node chains remain untouched.
func (m *sourceMap) translate(response *http.Response, publicPort int) error {
	path := response.Request.URL.Path
	if response.StatusCode != http.StatusOK || (path != "/connections" && path != "/configs") ||
		!strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") {
		return nil
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		return err
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	if path == "/configs" {
		document["mixed-port"], _ = json.Marshal(publicPort)
	} else {
		var connections []map[string]json.RawMessage
		if err := json.Unmarshal(document["connections"], &connections); err != nil {
			return err
		}
		for _, connection := range connections {
			var metadata map[string]json.RawMessage
			if json.Unmarshal(connection["metadata"], &metadata) != nil {
				continue
			}
			var ip string
			var network string
			var port interface{}
			_ = json.Unmarshal(metadata["sourceIP"], &ip)
			_ = json.Unmarshal(metadata["network"], &network)
			_ = json.Unmarshal(metadata["sourcePort"], &port)
			m.RLock()
			original := m.addresses[strings.ToLower(network)+"|"+net.JoinHostPort(ip, fmt.Sprint(port))]
			m.RUnlock()
			if original == "" {
				continue
			}
			host, sourcePort, err := net.SplitHostPort(original)
			if err != nil {
				continue
			}
			metadata["sourceIP"], _ = json.Marshal(host)
			if _, ok := port.(string); ok {
				metadata["sourcePort"], _ = json.Marshal(sourcePort)
			} else {
				number, _ := strconv.Atoi(sourcePort)
				metadata["sourcePort"], _ = json.Marshal(number)
			}
			metadata["process"], metadata["processPath"] = json.RawMessage(`""`), json.RawMessage(`""`)
			connection["metadata"], _ = json.Marshal(metadata)
		}
		document["connections"], _ = json.Marshal(connections)
	}
	data, err = json.Marshal(document)
	if err != nil {
		return err
	}
	response.Body = io.NopCloser(strings.NewReader(string(data)))
	response.ContentLength = int64(len(data))
	response.Header.Set("Content-Length", strconv.Itoa(len(data)))
	return nil
}
