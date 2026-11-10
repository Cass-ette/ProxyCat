package tun

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
)

// Keep the version-2 wire protocol so already-installed apps remain compatible.
// The relay capability distinguishes helpers that can preserve public ports.
const serviceProtocol = 2
const serviceCapabilityUserOwnedRelay = "user-owned-local-relay"

type serviceHealth struct {
	Protocol     int      `json:"protocol"`
	Capabilities []string `json:"capabilities"`
}

func currentServiceHealth() serviceHealth {
	return serviceHealth{Protocol: serviceProtocol, Capabilities: []string{serviceCapabilityUserOwnedRelay}}
}

type serviceSettings struct {
	UID  int    `json:"uid"`
	GID  int    `json:"gid"`
	Home string `json:"home"`
}

func serviceLabel(uid int) string { return "com.cassette.proxycat.tun." + strconv.Itoa(uid) }
func serviceDirectory(uid int) string {
	return filepath.Join("/Library/PrivilegedHelperTools", serviceLabel(uid))
}
func servicePlist(uid int) string {
	return filepath.Join("/Library/LaunchDaemons", serviceLabel(uid)+".plist")
}
func serviceSocket(uid int) string {
	return filepath.Join("/var/run", serviceLabel(uid), "control.sock")
}

func serviceClient(socket string) *http.Client {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}
}

func serviceCall(client *http.Client, method, endpoint string, input, output interface{}) error {
	defer client.CloseIdleConnections()
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, "http://localhost"+endpoint, body)
	if err != nil {
		return err
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("TUN 助手：%s", strings.TrimSpace(string(message)))
	}
	if output != nil {
		return json.NewDecoder(response.Body).Decode(output)
	}
	return nil
}

func serviceAvailable(client *http.Client) bool {
	var health serviceHealth
	if serviceCall(client, http.MethodGet, "/health", nil, &health) != nil || health.Protocol != serviceProtocol {
		return false
	}
	for _, capability := range health.Capabilities {
		if capability == serviceCapabilityUserOwnedRelay {
			return true
		}
	}
	return false
}

func authorizeService(executable, action string, uid int) error {
	command := shellQuote(executable) + " tun " + action + " " + strconv.Itoa(uid)
	script := "do shell script " + strconv.Quote(command) + " with administrator privileges with prompt \"ProxyCat 需要安装或移除 TUN 系统助手。安装后开启 TUN 无需重复输入密码。\""
	output, err := exec.Command("/usr/bin/osascript", "-e", script).CombinedOutput()
	if err == nil {
		return nil
	}
	if strings.Contains(string(output), "-128") {
		return fmt.Errorf("已取消管理员授权，未改变 TUN 状态")
	}
	return fmt.Errorf("TUN 助手操作失败：%s", strings.TrimSpace(string(output)))
}

func ensureService(client *http.Client, install func() error) error {
	if serviceAvailable(client) {
		return nil
	}
	if err := install(); err != nil {
		return err
	}
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
		if serviceAvailable(client) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("TUN 系统助手未能启动，请查看系统助手日志")
}

// PrepareService starts only a session supervisor. The caller still has to
// complete /prepared, stop its normal core, and explicitly send /start.
func PrepareService(s session) error {
	return prepareServiceWithProgress(s, nil)
}

func prepareServiceWithProgress(s session, report ProgressFunc) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	client := serviceClient(serviceSocket(s.UID))
	if err := ensureService(client, func() error {
		report.send("authorize", "首次安装或更新 TUN 助手，请完成系统授权")
		return authorizeService(executable, "install-service", s.UID)
	}); err != nil {
		return err
	}
	return serviceCall(client, http.MethodPost, "/prepare", map[string]string{"directory": s.Directory}, nil)
}

// InstallHelper prepares the persistent service without preparing a session or
// changing the currently running core, routes or system proxy.
func InstallHelper() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	uid := os.Getuid()
	return ensureService(serviceClient(serviceSocket(uid)), func() error {
		return authorizeService(executable, "install-service", uid)
	})
}

func RemoveService() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return authorizeService(executable, "remove-service", os.Getuid())
}

func xmlText(s string) string {
	var out bytes.Buffer
	_ = xml.EscapeText(&out, []byte(s))
	return out.String()
}
func launchDaemonPlist(settings serviceSettings, executable string) []byte {
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>tun</string><string>service</string><string>%d</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
<key>ThrottleInterval</key><integer>5</integer>
<key>ExitTimeOut</key><integer>8</integer>
<key>StandardOutPath</key><string>%s</string>
<key>StandardErrorPath</key><string>%s</string>
</dict></plist>
`, serviceLabel(settings.UID), xmlText(executable), settings.UID, xmlText(filepath.Join(filepath.Dir(executable), "service.log")), xmlText(filepath.Join(filepath.Dir(executable), "service.log"))))
}

func rootDirectory(path string, mode os.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || !info.IsDir() || info.Mode()&0022 != 0 {
		return fmt.Errorf("invalid system helper directory: %s", path)
	}
	return nil
}

func InstallService(uid int) error {
	if os.Geteuid() != 0 || uid <= 0 {
		return fmt.Errorf("安装 TUN 助手需要管理员授权")
	}
	account, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return err
	}
	settings := serviceSettings{UID: uid, GID: gid, Home: account.HomeDir}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	p := paths.ForHome(settings.Home)
	for _, directory := range []string{"/Library/PrivilegedHelperTools", "/Library/LaunchDaemons"} {
		if err := rootDirectory(directory, 0755); err != nil {
			return err
		}
	}
	stage, err := os.MkdirTemp("/Library/PrivilegedHelperTools", ".proxycat-tun-install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := os.Chmod(stage, 0755); err != nil {
		return err
	}
	if err := copyFile(executable, filepath.Join(stage, "proxyctl"), 0755); err != nil {
		return err
	}
	if err := copyUserFile(p.Mihomo, filepath.Join(stage, "mihomo"), uid, 0755); err != nil {
		return fmt.Errorf("请先安装普通代理内核：%w", err)
	}
	b, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(stage, "settings.json"), b, 0600); err != nil {
		return err
	}
	destination := serviceDirectory(uid)
	return replaceServiceBundle(stage, destination, servicePlist(uid), launchDaemonPlist(settings, filepath.Join(destination, "proxyctl")),
		func() error {
			output, err := exec.Command("/bin/launchctl", "bootout", "system/"+serviceLabel(uid)).CombinedOutput()
			if err != nil {
				return fmt.Errorf("停止旧 TUN 助手失败：%s", strings.TrimSpace(string(output)))
			}
			return nil
		},
		func() error {
			output, err := exec.Command("/bin/launchctl", "bootstrap", "system", servicePlist(uid)).CombinedOutput()
			if err != nil {
				return fmt.Errorf("注册 TUN 系统助手失败：%s", strings.TrimSpace(string(output)))
			}
			return nil
		})
}

// Installation is transactional; all filesystem paths and launchd actions are
// injected here so rollback can be tested without installing a real service.
func replaceServiceBundle(stage, destination, plistPath string, plist []byte, stop, start func() error) (result error) {
	backup := destination + ".previous"
	if _, err := os.Stat(backup); err == nil {
		return fmt.Errorf("旧助手备份尚未清理：%s", backup)
	}
	oldPlist, plistErr := os.ReadFile(plistPath)
	if plistErr != nil && !os.IsNotExist(plistErr) {
		return plistErr
	}
	hadOld := false
	if _, err := os.Stat(destination); err == nil {
		hadOld = true
	}
	if hadOld && plistErr == nil {
		if err := stop(); err != nil {
			return err
		}
	}
	if hadOld {
		if err := os.Rename(destination, backup); err != nil {
			_ = start()
			return err
		}
	}
	defer func() {
		if result == nil {
			if hadOld {
				_ = os.RemoveAll(backup)
			}
			return
		}
		_ = os.RemoveAll(destination)
		if hadOld {
			if err := os.Rename(backup, destination); err != nil {
				result = errors.Join(result, err)
				return
			}
		}
		if plistErr == nil {
			_ = os.WriteFile(plistPath, oldPlist, 0644)
		} else {
			_ = os.Remove(plistPath)
		}
		if hadOld && plistErr == nil {
			result = errors.Join(result, start())
		}
	}()
	if err := os.Rename(stage, destination); err != nil {
		return err
	}
	if err := os.WriteFile(plistPath, plist, 0644); err != nil {
		return err
	}
	return start()
}

func UninstallService(uid int) error {
	if os.Geteuid() != 0 || uid <= 0 {
		return fmt.Errorf("移除 TUN 助手需要管理员授权")
	}
	if _, err := os.Stat(servicePlist(uid)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(servicePlist(uid)); err == nil {
		if output, err := exec.Command("/bin/launchctl", "bootout", "system/"+serviceLabel(uid)).CombinedOutput(); err != nil {
			return fmt.Errorf("停止 TUN 系统助手失败：%s", strings.TrimSpace(string(output)))
		}
	}
	for _, path := range []string{servicePlist(uid), serviceDirectory(uid), filepath.Dir(serviceSocket(uid))} {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

func loadServiceSettings(uid int) (serviceSettings, error) {
	var settings serviceSettings
	file, err := openOwnedFile(filepath.Join(serviceDirectory(uid), "settings.json"), 0, os.O_RDONLY)
	if err != nil {
		return settings, err
	}
	defer file.Close()
	if err := json.NewDecoder(file).Decode(&settings); err != nil {
		return settings, err
	}
	if settings.UID != uid || uid <= 0 || !filepath.IsAbs(settings.Home) {
		return settings, fmt.Errorf("invalid service settings")
	}
	return settings, nil
}

func copyUserFile(source, destination string, uid int, mode os.FileMode) error {
	input, err := openOwnedFile(source, uid, os.O_RDONLY)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	closeErr := output.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func readServiceSession(directory string, settings serviceSettings) (session, error) {
	var s session
	if filepath.Dir(directory) != "/private/tmp" || !strings.HasPrefix(filepath.Base(directory), "proxycat-tun-") {
		return s, fmt.Errorf("invalid session directory")
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return s, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != settings.UID || !info.IsDir() || info.Mode()&0077 != 0 {
		return s, fmt.Errorf("invalid session owner")
	}
	file, err := openOwnedFile(filepath.Join(directory, "session.json"), settings.UID, os.O_RDONLY)
	if err != nil {
		return s, err
	}
	defer file.Close()
	if err := json.NewDecoder(io.LimitReader(file, 16384)).Decode(&s); err != nil {
		return s, err
	}
	if s.Directory != directory || s.UID != settings.UID || s.GID != settings.GID || len(s.Token) != 64 || s.ParentStart == "" || s.ParentStart != processStart(s.ParentPID) {
		return s, fmt.Errorf("invalid TUN owner")
	}
	if _, err := hex.DecodeString(s.Token); err != nil {
		return s, fmt.Errorf("invalid session token")
	}
	owner, err := exec.Command("/bin/ps", "-p", strconv.Itoa(s.ParentPID), "-o", "uid=").Output()
	if err != nil || strings.TrimSpace(string(owner)) != strconv.Itoa(settings.UID) {
		return s, fmt.Errorf("invalid parent process owner")
	}
	// Requests never select an executable or a privileged filesystem path.
	p := paths.ForHome(settings.Home)
	s.Binary = filepath.Join(serviceDirectory(settings.UID), "mihomo")
	s.DataDirectory = filepath.Join(settings.Home, ".config", "mihomo")
	s.LogPath = p.MihomoLog
	return s, nil
}

func ServeServiceSession(directory string, uid int) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("TUN helper requires administrator authorization")
	}
	settings, err := loadServiceSettings(uid)
	if err != nil {
		return err
	}
	s, err := readServiceSession(directory, settings)
	if err != nil {
		return err
	}
	return serveAuthorizedSession(s)
}

type serviceSessions struct {
	mu         sync.Mutex
	command    *exec.Cmd
	directory  string
	done       chan struct{}
	executable string
	settings   serviceSettings
}

func (s *serviceSessions) prepare(directory string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.command != nil {
		if s.directory == directory {
			return nil
		}
		return fmt.Errorf("上一次 TUN 会话尚未退出，请稍后重试")
	}
	if _, err := readServiceSession(directory, s.settings); err != nil {
		return err
	}
	command := exec.Command(s.executable, "tun", "service-session", directory, strconv.Itoa(s.settings.UID))
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Start(); err != nil {
		return err
	}
	s.command, s.directory, s.done = command, directory, make(chan struct{})
	done := s.done
	go func() {
		_ = command.Wait()
		s.mu.Lock()
		s.command = nil
		s.directory = ""
		close(done)
		s.mu.Unlock()
	}()
	return nil
}
func (s *serviceSessions) stop() {
	s.mu.Lock()
	command, done := s.command, s.done
	s.mu.Unlock()
	if command == nil {
		return
	}
	_ = command.Process.Signal(syscall.SIGTERM)
	// The session supervisor reaps its kernel before it exits.
	<-done
}

func serviceHandler(prepare func(string) error) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewEncoder(w).Encode(currentServiceHealth())
	})
	mux.HandleFunc("/prepare", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var input struct {
			Directory string `json:"directory"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "invalid session request", http.StatusBadRequest)
			return
		}
		if err := prepare(input.Directory); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func RunService(uid int) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("TUN service requires administrator authorization")
	}
	settings, err := loadServiceSettings(uid)
	if err != nil {
		return err
	}
	socket := serviceSocket(uid)
	if err := rootDirectory(filepath.Dir(socket), 0711); err != nil {
		return err
	}
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(socket, 0600); err != nil {
		return err
	}
	if err := os.Chown(socket, settings.UID, settings.GID); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	sessions := &serviceSessions{executable: executable, settings: settings}
	defer sessions.stop()
	server := &http.Server{Handler: serviceHandler(sessions.prepare), ReadHeaderTimeout: time.Second}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		_ = server.Close()
		return nil
	case err := <-stopped:
		return err
	}
}
