package tun

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/controller"
)

// Serve runs only after an interactive macOS authorization. The executable and
// config used by the privileged child live in a private root-owned directory.
func Serve(directory string) (result error) {
	if os.Geteuid() != 0 {
		return fmt.Errorf("TUN supervisor requires administrator authorization")
	}
	var s session
	b, err := os.ReadFile(filepath.Join(directory, "session.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s.Directory != directory || s.UID <= 0 || len(s.Token) != 64 || processStart(s.ParentPID) != s.ParentStart || s.ParentStart == "" {
		return fmt.Errorf("invalid TUN owner")
	}
	return serveAuthorizedSession(s)
}

func serveAuthorizedSession(s session) (result error) {
	directory := s.Directory
	defer func() {
		if result != nil {
			if file, err := openOwnedFile(filepath.Join(directory, "error"), s.UID, os.O_WRONLY); err == nil {
				_ = file.Truncate(0)
				_, _ = file.WriteString(result.Error())
				file.Close()
			}
		}
	}()
	rootDir, err := os.MkdirTemp("/private/tmp", "proxycat-tun-root-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(rootDir)
	defer func() { result = errors.Join(result, syncGeoData(rootDir, s.DataDirectory, s.UID, s.GID)) }()
	// Users may traverse this directory to reach the control socket but cannot
	// replace it, its executable, or its configuration.
	if err := os.Chmod(rootDir, 0711); err != nil {
		return err
	}
	s.SocketPath = filepath.Join(rootDir, "control.sock")
	address, err := openOwnedFile(filepath.Join(directory, "socket-path"), s.UID, os.O_WRONLY)
	if err != nil {
		return err
	}
	_, writeErr := address.WriteString(s.SocketPath)
	if err := address.Close(); writeErr != nil || err != nil {
		return fmt.Errorf("could not publish TUN control socket")
	}
	if err := copyFile(s.Binary, filepath.Join(rootDir, "mihomo"), 0700); err != nil {
		return err
	}
	if err := copyUserFile(filepath.Join(directory, "config.yaml"), filepath.Join(rootDir, "config.yaml"), s.UID, 0600); err != nil {
		return err
	}
	private, release, err := preparePrivateListeners(filepath.Join(rootDir, "config.yaml"))
	if err != nil {
		return err
	}
	defer release()
	for _, name := range []string{"GeoSite.dat", "GeoIP.dat", "geoip.metadb", "Country.mmdb", "ASN.mmdb"} {
		if err := copyUserFile(filepath.Join(s.DataDirectory, name), filepath.Join(rootDir, name), s.UID, 0600); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	log, err := openOwnedFile(s.LogPath, s.UID, os.O_APPEND|os.O_WRONLY)
	if err != nil {
		return err
	}
	defer log.Close()
	ownerAlive := func() bool {
		_, cancelled := os.Stat(filepath.Join(directory, "cancel"))
		_, exists := os.Stat(directory)
		return exists == nil && os.IsNotExist(cancelled) && processStart(s.ParentPID) == s.ParentStart
	}
	var relay *relayProcess
	relayFailed := make(chan error, 1)
	launch := func() (*exec.Cmd, error) {
		var err error
		relay, err = startUserRelay(s, private, log, relayFailed)
		if err != nil {
			return nil, err
		}
		release()
		cmd := exec.Command(filepath.Join(rootDir, "mihomo"), "-d", rootDir, "-f", filepath.Join(rootDir, "config.yaml"))
		cmd.Stdout, cmd.Stderr = log, log
		return cmd, cmd.Start()
	}
	client := controller.NewClientWithSecret("http://"+loopback(private.ControllerPort), s.Token)
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client.SetClient(&http.Client{Transport: transport, Timeout: time.Second})
	ready := func() bool {
		// Public listeners have already been bound by the user relay. Require
		// the private mixed listener as well as the real TUN interface.
		defer transport.CloseIdleConnections()
		cfg, err := client.GetConfig()
		return err == nil && Ready(cfg) && cfg.MixedPort == private.ProxyPort
	}
	return superviseWithCompanion(s, nil, launch, ready, ownerAlive, func() { relay.stop() }, relayFailed)
}

var geoDataFiles = []string{"GeoSite.dat", "GeoIP.dat", "geoip.metadb", "Country.mmdb", "ASN.mmdb"}

func syncGeoData(sourceDir, destinationDir string, uid, gid int) error {
	if destinationDir == "" || uid <= 0 || gid < 0 {
		return nil
	}
	parentDir := filepath.Dir(destinationDir)
	if err := ensureUserDirectory(parentDir, uid, gid); err != nil {
		return err
	}
	info, err := os.Lstat(destinationDir)
	if os.IsNotExist(err) {
		if err := os.Mkdir(destinationDir, 0700); err != nil {
			return err
		}
		if err := os.Chown(destinationDir, uid, gid); err != nil {
			return err
		}
		info, err = os.Lstat(destinationDir)
	}
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || int(stat.Uid) != uid {
		return fmt.Errorf("拒绝写入不属于当前用户的地理库目录")
	}
	root, err := os.OpenRoot(destinationDir)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, name := range geoDataFiles {
		source, err := os.Open(filepath.Join(sourceDir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		sourceInfo, statErr := source.Stat()
		if statErr != nil || !sourceInfo.Mode().IsRegular() {
			source.Close()
			return fmt.Errorf("invalid geodata file")
		}
		unchanged, compareErr := sameGeoData(source, root, name)
		if compareErr != nil {
			source.Close()
			return compareErr
		}
		if unchanged {
			if err := source.Close(); err != nil {
				return err
			}
			continue
		}
		var nonce [12]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			source.Close()
			return err
		}
		tmp := ".proxycat-geo-" + hex.EncodeToString(nonce[:])
		output, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			source.Close()
			return err
		}
		_, copyErr := io.Copy(output, source)
		sourceCloseErr := source.Close()
		if copyErr == nil {
			copyErr = output.Sync()
		}
		if copyErr == nil {
			copyErr = output.Chown(uid, gid)
		}
		if copyErr == nil {
			copyErr = output.Chmod(0644)
		}
		copyErr = errors.Join(copyErr, output.Close(), sourceCloseErr)
		if copyErr != nil {
			_ = root.Remove(tmp)
			return copyErr
		}
		if err := root.Rename(tmp, name); err != nil {
			_ = root.Remove(tmp)
			return err
		}
	}
	return nil
}

func sameGeoData(source *os.File, destination *os.Root, name string) (bool, error) {
	info, err := destination.Lstat(name)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return false, nil
	}
	current, err := destination.Open(name)
	if err != nil {
		return false, err
	}
	defer current.Close()
	sourceHash, currentHash := sha256.New(), sha256.New()
	if _, err := io.Copy(sourceHash, source); err != nil {
		return false, err
	}
	if _, err := io.Copy(currentHash, current); err != nil {
		return false, err
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	return bytes.Equal(sourceHash.Sum(nil), currentHash.Sum(nil)), nil
}

func ensureUserDirectory(path string, uid, gid int) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.Mkdir(path, 0700); err != nil {
			return err
		}
		if err := os.Chown(path, uid, gid); err != nil {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || int(stat.Uid) != uid {
		return fmt.Errorf("拒绝写入不属于当前用户的地理库目录")
	}
	return nil
}

func openOwnedFile(path string, uid, flags int) (*os.File, error) {
	file, err := os.OpenFile(path, flags|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err == nil {
		owner, ok := info.Sys().(*syscall.Stat_t)
		if ok && int(owner.Uid) == uid && info.Mode().IsRegular() {
			return file, nil
		}
	}
	file.Close()
	return nil, fmt.Errorf("invalid TUN file owner")
}

// Ready verifies that the configured Darwin tunnel has a live network interface.
func Ready(cfg *controller.Config) bool {
	if cfg == nil || cfg.Tun["enable"] != true {
		return false
	}
	device, _ := cfg.Tun["device"].(string)
	iface, err := net.InterfaceByName(device)
	return err == nil && strings.HasPrefix(device, "utun") && iface.Flags&net.FlagUp != 0
}

func supervise(s session, cmd *exec.Cmd, ready, ownerAlive func() bool) error {
	return superviseSession(s, cmd, nil, ready, ownerAlive)
}

// Publish the control socket before launching the privileged core. The caller
// can prove that preparation succeeded while its ordinary core is still alive.
func superviseSession(s session, cmd *exec.Cmd, launch func() (*exec.Cmd, error), ready, ownerAlive func() bool) error {
	return superviseWithCompanion(s, cmd, launch, ready, ownerAlive, nil, nil)
}

func superviseWithCompanion(s session, cmd *exec.Cmd, launch func() (*exec.Cmd, error), ready, ownerAlive func() bool, stopCompanion func(), companionFailed <-chan error) error {
	var exited chan struct{}
	var exitError error
	var started atomic.Bool
	startedAt := time.Now()
	watch := func(child *exec.Cmd) {
		cmd = child
		exited = make(chan struct{})
		startedAt = time.Now()
		started.Store(true)
		go func() { exitError = child.Wait(); close(exited) }()
	}
	if cmd != nil {
		watch(cmd)
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			started.Store(false)
			if stopCompanion != nil {
				stopCompanion()
			}
			if cmd == nil {
				return
			}
			_ = cmd.Process.Signal(syscall.SIGTERM)
			select {
			case <-exited:
			case <-time.After(3 * time.Second):
				_ = cmd.Process.Kill()
				<-exited
			}
		})
	}
	defer stop()
	socketPath := s.SocketPath
	if socketPath == "" {
		socketPath = filepath.Join(s.Directory, "control.sock")
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(listener.Addr().String(), 0600); err != nil {
		return err
	}
	if err := os.Chown(listener.Addr().String(), s.UID, s.GID); err != nil {
		return err
	}
	type control struct {
		action string
		result chan error
	}
	commands := make(chan control)
	done := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+s.Token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/prepared":
			if !ownerAlive() {
				http.Error(w, "owner exited", http.StatusServiceUnavailable)
				return
			}
		case r.Method == http.MethodGet && r.URL.Path == "/status":
			if !started.Load() || !ready() {
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
		case r.Method == http.MethodPost && (r.URL.Path == "/start" || r.URL.Path == "/stop"):
			command := control{action: r.URL.Path, result: make(chan error, 1)}
			select {
			case commands <- command:
			case <-done:
				http.Error(w, "session stopped", http.StatusServiceUnavailable)
				return
			case <-r.Context().Done():
				return
			}
			select {
			case err := <-command.result:
				if err != nil {
					http.Error(w, "core startup failed", http.StatusServiceUnavailable)
					return
				}
			case <-r.Context().Done():
				return
			}
		default:
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go server.Serve(listener)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		_ = server.Close()
	}()
	defer close(done)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, os.Interrupt)
	defer signal.Stop(signals)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastReady := time.Now()
	for {
		select {
		case err := <-companionFailed:
			return err
		case <-exited:
			return fmt.Errorf("代理内核已退出：%v", exitError)
		case command := <-commands:
			if command.action == "/stop" {
				stop()
				command.result <- nil
				return nil
			}
			if !ownerAlive() {
				err := fmt.Errorf("TUN 启动已取消")
				command.result <- err
				return err
			}
			if !started.Load() {
				child, err := launch()
				if err != nil {
					command.result <- err
					return err
				}
				watch(child)
				lastReady = time.Now()
			}
			command.result <- nil
		case <-signals:
			return nil
		case <-ticker.C:
			if !ownerAlive() {
				return nil
			}
			if !started.Load() {
				if time.Since(startedAt) > 30*time.Second {
					return fmt.Errorf("TUN 启动等待超时")
				}
				continue
			}
			if ready() {
				lastReady = time.Now()
			} else if time.Since(lastReady) > 18*time.Second {
				return fmt.Errorf("未能建立 TUN 网络接口")
			}
		}
	}
}
