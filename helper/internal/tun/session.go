package tun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/controller"
	"github.com/Cass-ette/ProxyCat/helper/internal/core"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
)

type session struct {
	Directory     string `json:"directory"`
	Token         string `json:"token"`
	Binary        string `json:"binary"`
	DataDirectory string `json:"dataDirectory"`
	LogPath       string `json:"logPath"`
	SocketPath    string `json:"-"`
	ParentPID     int    `json:"parentPID"`
	ParentStart   string `json:"parentStart"`
	UID           int    `json:"uid"`
	GID           int    `json:"gid"`
}

func marker(p paths.RuntimePaths) string { return filepath.Join(p.Config, "tun-session.json") }
func readSession(p paths.RuntimePaths) (session, error) {
	var s session
	b, err := os.ReadFile(marker(p))
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(b, &s)
	if err == nil && (s.Directory == "" || s.Token == "") {
		err = fmt.Errorf("invalid TUN session")
	}
	return s, err
}

// Client authenticates controller requests while an authorized session is running.
func Client(p paths.RuntimePaths) *controller.Client {
	if s, err := readSession(p); err == nil {
		return controller.NewClientWithSecret("http://127.0.0.1:9090", s.Token)
	}
	return controller.NewClient("")
}

func request(s session, method, endpoint string) error {
	socketPath := filepath.Join(s.Directory, "control.sock")
	if b, err := os.ReadFile(filepath.Join(s.Directory, "socket-path")); err == nil && len(b) > 0 {
		socketPath = strings.TrimSpace(string(b))
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second}
	req, _ := http.NewRequest(method, "http://localhost"+endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+s.Token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("TUN control returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func Active(p paths.RuntimePaths) bool {
	s, err := readSession(p)
	return err == nil && request(s, http.MethodGet, "/status") == nil
}

func processStart(pid int) string {
	if pid < 2 {
		return ""
	}
	b, err := exec.Command("/bin/ps", "-p", strconv.Itoa(pid), "-o", "lstart=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

type snapshot struct {
	running    bool
	mode       string
	selections map[string]string
}

func capture(p paths.RuntimePaths) snapshot {
	s := snapshot{selections: map[string]string{}}
	s.running, _, _ = core.Status()
	if cfg, err := Client(p).GetConfig(); err == nil {
		s.mode = cfg.Mode
	}
	if proxies, err := Client(p).GetProxies(); err == nil {
		for name, proxy := range proxies {
			if proxy.Type == "Selector" {
				s.selections[name] = proxy.Now
			}
		}
	}
	return s
}

func restoreSelections(p paths.RuntimePaths, state snapshot) error {
	c := Client(p)
	c.SetClient(&http.Client{Timeout: time.Second})
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
		if _, err := c.GetConfig(); err == nil {
			for group, name := range state.selections {
				_ = c.SelectProxy(group, name)
			}
			if state.mode != "" {
				if err := c.SetMode(state.mode); err != nil {
					return err
				}
			}
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("代理内核未能恢复，请查看日志")
}

func startNormal(p paths.RuntimePaths, state snapshot) error {
	if _, err := core.Start(p.Mihomo, p.ConfigYAML, p.MihomoLog); err != nil {
		return err
	}
	return restoreSelections(p, state)
}

// Enable asks macOS for authorization. A supervisor owns the privileged child;
// it accepts only a stop request and exits when the owning application exits.
func Enable(p paths.RuntimePaths, parentPID int) (result error) {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("TUN requires macOS")
	}
	if Active(p) {
		return nil
	}
	if _, err := os.Stat(marker(p)); err == nil {
		if _, err := Stop(p); err != nil {
			return err
		}
	}
	state := capture(p)
	content, err := os.ReadFile(p.ConfigYAML)
	if err != nil {
		return fmt.Errorf("请先导入订阅：%w", err)
	}
	if !core.Installed(p.Mihomo) {
		return fmt.Errorf("请先使用“一键启动”安装代理内核，再开启 TUN")
	}
	parentStart := processStart(parentPID)
	if parentStart == "" {
		return fmt.Errorf("无法确认应用进程，未开启 TUN")
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("/private/tmp", "proxycat-tun-")
	if err != nil {
		return err
	}
	s := session{Directory: dir, Token: hex.EncodeToString(token), Binary: p.Mihomo,
		DataDirectory: filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(p.Base))), ".config", "mihomo"),
		LogPath:       p.MihomoLog,
		ParentPID:     parentPID, ParentStart: parentStart, UID: os.Getuid(), GID: os.Getgid()}
	// p.Base is <home>/Library/Application Support/ProxyCat.
	data, err := configuration(content, s.Token, state.mode, state.selections)
	if err != nil {
		os.RemoveAll(dir)
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0600); err != nil {
		os.RemoveAll(dir)
		return err
	}
	b, _ := json.Marshal(s)
	if err = os.WriteFile(filepath.Join(dir, "session.json"), b, 0600); err != nil {
		os.RemoveAll(dir)
		return err
	}
	for _, name := range []string{"error", "socket-path"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			os.RemoveAll(dir)
			return err
		}
	}
	if err = os.MkdirAll(p.Config, 0755); err != nil {
		os.RemoveAll(dir)
		return err
	}
	if err = os.WriteFile(marker(p), b, 0600); err != nil {
		os.RemoveAll(dir)
		return err
	}
	if err = os.MkdirAll(p.Logs, 0755); err != nil {
		os.Remove(marker(p))
		os.RemoveAll(dir)
		return err
	}
	log, err := os.OpenFile(s.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Remove(marker(p))
		os.RemoveAll(dir)
		return err
	}
	log.Close()
	stoppedNormal := false
	defer func() {
		if result == nil {
			return
		}
		_ = os.WriteFile(filepath.Join(dir, "cancel"), nil, 0600)
		// If authorization succeeded but startup failed, stop the supervisor before rollback.
		if err := request(s, http.MethodPost, "/stop"); err != nil && Active(p) {
			result = errors.Join(result, err)
			return
		}
		os.Remove(marker(p))
		os.RemoveAll(dir)
		if stoppedNormal {
			result = errors.Join(result, startNormal(p, state))
		}
	}()
	// Validate before asking for authorization or interrupting the current core.
	check := exec.Command(p.Mihomo, "-t", "-d", s.DataDirectory, "-f", filepath.Join(dir, "config.yaml"))
	if err = check.Run(); err != nil {
		return fmt.Errorf("TUN 配置校验失败，原连接未改变")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	command := "/usr/bin/nohup " + shellQuote(executable) + " tun serve " + shellQuote(dir) + " </dev/null >/dev/null 2>&1 &"
	script := "do shell script " + strconv.Quote(command) + " with administrator privileges with prompt \"ProxyCat 需要管理员权限来创建 TUN 隧道。\""
	if output, err := exec.Command("/usr/bin/osascript", "-e", script).CombinedOutput(); err != nil {
		if strings.Contains(string(output), "-128") {
			return fmt.Errorf("已取消管理员授权，未开启 TUN")
		}
		return fmt.Errorf("TUN 管理员授权失败：%s", strings.TrimSpace(string(output)))
	}
	// Keep the ordinary core available throughout the authorization dialog.
	if state.running {
		_, pid, _ := core.Status()
		actual, err := exec.Command("/bin/ps", "-ww", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
		if err != nil || strings.TrimSpace(string(actual)) != p.Mihomo {
			return fmt.Errorf("运行中的内核不属于 ProxyCat，请先关闭其他代理内核")
		}
		if err = core.Stop(pid); err != nil {
			return err
		}
		stoppedNormal = true
		time.Sleep(200 * time.Millisecond)
	}
	if err := os.WriteFile(filepath.Join(dir, "start"), nil, 0600); err != nil {
		return err
	}
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		if request(s, http.MethodGet, "/status") == nil {
			return restoreSelections(p, state)
		}
		if b, err := os.ReadFile(filepath.Join(dir, "error")); err == nil && len(b) > 0 {
			return fmt.Errorf("TUN 启动失败：%s", strings.TrimSpace(string(b)))
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("TUN 启动超时，已恢复普通代理")
}

// Stop does not relaunch a normal core, so quit and campus cleanup remain ordered.
func Stop(p paths.RuntimePaths) (bool, error) {
	s, err := readSession(p)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if err := request(s, http.MethodPost, "/stop"); err != nil {
		c := Client(p)
		c.SetClient(&http.Client{Timeout: time.Second})
		if _, alive := c.GetConfig(); alive == nil {
			return true, fmt.Errorf("无法关闭 TUN：%w", err)
		}
	}
	os.Remove(marker(p))
	os.RemoveAll(s.Directory)
	return true, nil
}

func Disable(p paths.RuntimePaths) error {
	state := capture(p)
	stopped, err := Stop(p)
	if err != nil || !stopped {
		return err
	}
	return startNormal(p, state)
}

func Restart(p paths.RuntimePaths, parentPID int) error {
	state := capture(p)
	if _, err := Stop(p); err != nil {
		return err
	}
	if err := startNormal(p, state); err != nil {
		return err
	}
	return Enable(p, parentPID)
}

func copyFile(source, destination string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}
