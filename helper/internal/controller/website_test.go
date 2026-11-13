package controller

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWebsiteURL(t *testing.T) {
	for _, input := range []string{"example.com/path?q=1", " https://example.com/#x ", "http://[::1]:8080/path"} {
		u, err := websiteURL(input)
		if err != nil || u.Fragment != "" {
			t.Fatalf("%q: %v", input, err)
		}
	}
	for _, input := range []string{"", "https://", "ftp://example.com", "https://name:secret@example.com", "https://example.com:99999", "https://example.com:bad", "https://exa mple.com"} {
		if _, err := websiteURL(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestWebsiteDiagnosticRejectsChangedProxyPort(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/configs" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"mixed-port":12345,"mode":"rule"}`)
	}))
	defer api.Close()
	result, err := NewClient(api.URL).CheckWebsiteAtPort(context.Background(), "https://example.invalid", 7890)
	if err != nil || result.Outcome != "proxyUnavailable" {
		t.Fatalf("changed listener was accepted: %+v %v", result, err)
	}
}

// The controller exposes unrelated same-host traffic. Only our socket may
// supply evidence; credentials, query strings and paths are absent from output.
func TestWebsiteSocketCorrelation(t *testing.T) {
	var mu sync.Mutex
	port := ""
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, source, _ := net.SplitHostPort(r.RemoteAddr)
		mu.Lock()
		port = source
		mu.Unlock()
		local, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer local.Close()
		io.WriteString(local, "HTTP/1.1 200 Connection Established\r\n\r\n")
		reader := bufio.NewReader(local)
		if _, err := http.ReadRequest(reader); err != nil {
			return
		}
		io.WriteString(local, "HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\ntest")
		io.Copy(io.Discard, reader)
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	for _, observe := range []bool{true, false} {
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer fixture-secret" {
				w.WriteHeader(401)
				return
			}
			if r.URL.Path == "/configs" {
				fmt.Fprintf(w, `{"mixed-port":%s,"mode":"rule"}`, proxyURL.Port())
				return
			}
			mu.Lock()
			ownPort := port
			mu.Unlock()
			chain := `["Wrong Node","Wrong Group"]`
			fmt.Fprint(w, `{"connections":[{"metadata":{"sourceIP":"127.0.0.1","sourcePort":"1","destinationPort":"80","host":"example.com"},"chains":`+chain+`,"rule":"GeoIP","rulePayload":"CN"}`)
			if observe && ownPort != "" {
				fmt.Fprintf(w, `,{"metadata":{"sourceIP":"127.0.0.1","sourcePort":%s,"destinationPort":"80","host":"example.com"},"chains":["Actual Node","Proxy"],"rule":"Match","rulePayload":""}`, ownPort)
			}
			fmt.Fprint(w, ` ]}`)
		}))
		c := NewClientWithSecret(api.URL, "fixture-secret")
		result, err := c.CheckWebsite(context.Background(), "http://example.com/private?token=secret")
		api.Close()
		if err != nil || result.StatusCode != 200 || result.Outcome != "responded" {
			t.Fatalf("%+v %v", result, err)
		}
		if observe && (result.Node != "Actual Node" || result.Group != "Proxy" || result.Rule != "Match" || result.Route != "proxy") {
			t.Fatalf("wrong route %+v", result)
		}
		if !observe && (result.Route != "unknown" || result.Node != "") {
			t.Fatalf("guessed from unrelated traffic %+v", result)
		}
		data, _ := json.Marshal(result)
		if strings.Contains(string(data), "secret") || strings.Contains(string(data), "private") {
			t.Fatal("URL credentials leaked")
		}
	}
}

func TestWebsiteResponseSemantics(t *testing.T) {
	for _, test := range []struct {
		code    int
		outcome string
	}{{204, "responded"}, {301, "redirect"}, {401, "denied"}, {403, "denied"}, {429, "denied"}, {404, "httpError"}, {503, "httpError"}} {
		t.Run(fmt.Sprint(test.code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				local, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer local.Close()
				io.WriteString(local, "HTTP/1.1 200 Connection Established\r\n\r\n")
				if _, err := http.ReadRequest(bufio.NewReader(local)); err != nil {
					return
				}
				fmt.Fprintf(local, "HTTP/1.1 %d Test\r\nLocation: https://unvisited.invalid\r\nContent-Length: 0\r\n\r\n", test.code)
			}))
			defer server.Close()
			u, _ := url.Parse(server.URL)
			conn, err := net.Dial("tcp", u.Host)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(time.Second))
			outcome, code := requestWebsite(context.Background(), conn, u, nil)
			if code != test.code || outcome != test.outcome {
				t.Fatalf("%s %d", outcome, code)
			}
		})
	}
}

func TestWebsiteCertificateAndTunnelFailure(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("request sent despite untrusted certificate") }))
	defer target.Close()
	u, _ := url.Parse(target.URL)
	for _, denied := range []bool{false, true} {
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "CONNECT" {
				t.Errorf("method %s", r.Method)
				return
			}
			if denied {
				w.WriteHeader(502)
				return
			}
			upstream, err := net.Dial("tcp", u.Host)
			if err != nil {
				t.Error(err)
				return
			}
			defer upstream.Close()
			local, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			defer local.Close()
			io.WriteString(local, "HTTP/1.1 200 Connection Established\r\n\r\n")
			go io.Copy(upstream, local)
			io.Copy(local, upstream)
		}))
		proxyURL, _ := url.Parse(proxy.URL)
		conn, err := net.Dial("tcp", proxyURL.Host)
		if err != nil {
			t.Fatal(err)
		}
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		outcome, code := requestWebsite(context.Background(), conn, u, nil)
		conn.Close()
		proxy.Close()
		want := "certificate"
		if denied {
			want = "tunnelFailed"
		}
		if code != 0 || outcome != want {
			t.Fatalf("%s %d", outcome, code)
		}
	}
}

func TestWebsiteTimeoutAndControllerUnavailable(t *testing.T) {
	client := NewClient("http://127.0.0.1:1")
	result, err := client.CheckWebsite(context.Background(), "example.com")
	if err != nil || result.Outcome != "engineUnavailable" || result.Route != "unknown" {
		t.Fatalf("%+v %v", result, err)
	}
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	clientConn.SetDeadline(time.Now().Add(25 * time.Millisecond))
	u, _ := url.Parse("http://example.com")
	outcome, _ := requestWebsite(context.Background(), clientConn, u, nil)
	if outcome != "timeout" {
		t.Fatal(outcome)
	}
}
