package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/guard"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
)

const guardProtocol = "proxycat-guard-v1"

var errGuardRunning = errors.New("watchdog already running")

// A kernel lock, rather than a PID file, owns the watcher lifetime. The file
// stays in place so a new process can never lock a different inode accidentally.
func acquireGuardLock(base string) (func(), error) {
	if err := os.MkdirAll(base, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(base, "guard-watch.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, errGuardRunning
		}
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}

// Keep the Unix socket below macOS's path length limit even for long HOME
// paths. Only this user can traverse its directory or connect to the socket.
func guardSocket(base string) (string, error) {
	directory := filepath.Join("/tmp", fmt.Sprintf("proxycat-guard-%d", os.Getuid()))
	if err := os.Mkdir(directory, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return "", err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm() != 0700 || int(owner.Uid) != os.Getuid() {
		return "", fmt.Errorf("watchdog socket directory must be private and owned by the current user")
	}
	hash := sha256.Sum256([]byte(filepath.Clean(base)))
	return filepath.Join(directory, fmt.Sprintf("%x.sock", hash[:16])), nil
}

// Requests never read a PID or send a process signal. A stale or reused PID
// therefore cannot impersonate a watcher or cause another process to be killed.
func guardWatchRequest(base, endpoint string) error {
	socket, err := guardSocket(base)
	if err != nil {
		return err
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 300 * time.Millisecond}
	method := http.MethodGet
	if endpoint == "/stop" {
		method = http.MethodPost
	}
	req, _ := http.NewRequest(method, "http://localhost"+endpoint, nil)
	req.Header.Set("X-ProxyCat-Guard", guardProtocol)
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 128))
	if err != nil || response.StatusCode != http.StatusOK || string(data) != guardProtocol+"\n" {
		return fmt.Errorf("watchdog handshake failed")
	}
	return nil
}

var guardCommand = func() (*exec.Cmd, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return exec.Command(self, "guard", "--watch"), nil
}

func spawnGuardWatch(base string) error {
	if os.Getenv("PROXYCAT_NO_WATCHDOG") != "" {
		return nil // CLI tests opt out of detached production processes.
	}
	if err := guardWatchRequest(base, "/status"); err == nil {
		return nil
	}
	cmd, err := guardCommand()
	if err != nil {
		return err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := os.MkdirAll(filepath.Join(base, "logs"), 0700); err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(base, "logs", "watchdog.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer log.Close() // The child has its own descriptors after Start.
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start watchdog: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := guardWatchRequest(base, "/status"); err == nil {
			return nil
		}
		select {
		case err := <-done:
			// Another simultaneous spawn may have won the kernel lock.
			if guardWatchRequest(base, "/status") == nil {
				return nil
			}
			return fmt.Errorf("watchdog exited before readiness: %v", err)
		case <-deadline.C:
			_ = cmd.Process.Kill() // Only the child just created by this call.
			return fmt.Errorf("watchdog did not become ready")
		case <-ticker.C:
		}
	}
}

func stopGuardWatch(base string) {
	if guardWatchRequest(base, "/status") != nil || guardWatchRequest(base, "/stop") != nil {
		return
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if guardWatchRequest(base, "/status") != nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func runGuardWatch(stdout, stderr io.Writer, jsonOutput bool) int {
	p, err := paths.Default()
	if err == nil {
		err = watchGuard(p.Base, stdout, stderr, jsonOutput, 5*time.Second, func() (guard.Result, error) {
			return guard.Reconcile(guard.Options{Paths: p})
		})
	}
	if err != nil && !errors.Is(err, errGuardRunning) {
		fmt.Fprintf(stderr, "watchdog: %v\n", err)
		return 1
	}
	return 0
}

func watchGuard(base string, stdout, stderr io.Writer, jsonOutput bool, interval time.Duration, reconcile func() (guard.Result, error)) error {
	unlock, err := acquireGuardLock(base)
	if err != nil {
		return err
	}
	defer unlock()
	socket, err := guardSocket(base)
	if err != nil {
		return err
	}
	// Only the process holding the lock can remove a stale socket.
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(socket, 0600); err != nil {
		return err
	}
	stopped := make(chan struct{})
	var once sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-stopped:
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		default:
		}
		fmt.Fprintln(w, guardProtocol)
	})
	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("X-ProxyCat-Guard") != guardProtocol {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Fprintln(w, guardProtocol)
		// Flush the acknowledgement before the main loop closes the server.
		w.(http.Flusher).Flush()
		once.Do(func() { close(stopped) })
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second, IdleTimeout: time.Second}
	defer server.Close()
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Serve(listener) }()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stopped:
			return nil
		case <-signals:
			return nil
		default:
		}
		result, err := reconcile()
		switch {
		case err != nil:
			fmt.Fprintf(stderr, "%s guard: %v\n", time.Now().Format("15:04:05"), err)
		case result.Action != guard.ActionHealthy && result.Action != guard.ActionNoop && result.Port != 0:
			if jsonOutput {
				_ = json.NewEncoder(stdout).Encode(result)
			} else {
				fmt.Fprintf(stdout, "%s %s (port %d, failures %d) %s\n", time.Now().Format("15:04:05"), result.Action, result.Port, result.Failures, result.Detail)
			}
		}
		select {
		case <-stopped:
			return nil
		case <-signals:
			return nil
		case err := <-serverErrors:
			return err
		case <-ticker.C:
		}
	}
}
