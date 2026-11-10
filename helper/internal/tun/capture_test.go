package tun

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/controller"
)

func TestCaptureReleasesItsOwnControllerConnectionsBeforeHandoff(t *testing.T) {
	closed := make(chan struct{}, 4)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/configs":
			_, _ = w.Write([]byte(`{"mode":"rule"}`))
		case "/proxies":
			_, _ = w.Write([]byte(`{"proxies":{"Proxy":{"type":"Selector","now":"Node A"}}}`))
		}
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			select {
			case closed <- struct{}{}:
			default:
			}
		}
	}
	server.Start()
	defer server.Close()
	state := captureController(controller.NewClient(server.URL), snapshot{selections: map[string]string{}})
	if state.mode != "rule" || state.selections["Proxy"] != "Node A" {
		t.Fatalf("lost live routing state: %+v", state)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("capture left an idle connection for core shutdown to close")
	}
}

func TestUnresponsiveControllerCannotAddMinuteLongCaptureDelay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	started := time.Now()
	state := captureController(controller.NewClient(server.URL), snapshot{selections: map[string]string{}})
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("local state capture blocked the switch for %v", elapsed)
	}
	if state.mode != "" || len(state.selections) != 0 {
		t.Fatalf("invented live state after timeout: %+v", state)
	}
}
