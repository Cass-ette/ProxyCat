package core

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestHandoffWaitsForBothListenerEndpoints(t *testing.T) {
	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	api, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- waitForListenerRelease(ctx, []string{proxy.Addr().String(), api.Addr().String()}) }()
	proxy.Close()
	select {
	case err := <-done:
		t.Fatalf("accepted the still-occupied controller: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	api.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("did not continue after both endpoints were released")
	}
}

func TestHandoffDoesNotClaimOccupiedPortWasReleased(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	err = waitForListenerRelease(ctx, []string{listener.Addr().String()})
	if err == nil || !strings.Contains(err.Error(), listener.Addr().String()) {
		t.Fatalf("missing occupied endpoint in timeout: %v", err)
	}
}
