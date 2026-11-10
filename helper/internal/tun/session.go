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
	"github.com/Cass-ette/ProxyCat/helper/internal/sysproxy"
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
	command := exec.Command("/bin/ps", "-p", strconv.Itoa(pid), "-o", "lstart=")
	// Authorization runs in a different environment. The same process must
	// compare equal across user/root locale and time-zone settings.
	command.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC")
	b, err := command.Output()
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
	return captureController(Client(p), s)
}

func captureController(c *controller.Client, s snapshot) snapshot {
	// A local, unhealthy controller must not add two 30-second waits before
	// we even start a handoff. Release our own idle connections before stop.
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	c.SetClient(&http.Client{Transport: transport, Timeout: time.Second})
	if cfg, err := c.GetConfig(); err == nil {
		s.mode = cfg.Mode
	}
	if proxies, err := c.GetProxies(); err == nil {
		for name, proxy := range proxies {
			if proxy.Type == "Selector" {
				s.selections[name] = proxy.Now
			}
		}
	}
	return s
}

func restoreSelections(p paths.RuntimePaths, state snapshot, expectTUN bool) error {
	c := Client(p)
	return restoreController(c, state, expectTUN)
}

func restoreController(c *controller.Client, state snapshot, expectTUN bool) error {
	c.SetClient(&http.Client{Timeout: time.Second})
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
		if cfg, err := c.GetConfig(); err == nil && (cfg.Tun["enable"] == true) == expectTUN {
			for group, name := range state.selections {
				if err := c.SelectProxy(group, name); err != nil {
					return fmt.Errorf("恢复节点选择失败：%w", err)
				}
			}
			if state.mode != "" {
				if err := c.SetMode(state.mode); err != nil {
					return err
				}
			}
			if err := proxyListenerReady(cfg.MixedPort); err == nil {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("代理内核未能恢复，请查看日志")
}

func startNormal(p paths.RuntimePaths, state snapshot) error {
	return startNormalWithProgress(p, state, nil)
}

func startNormalWithProgress(p paths.RuntimePaths, state snapshot, report ProgressFunc) error {
	running, _, err := core.Status()
	if err != nil {
		return err
	}
	if !running {
		report.send("release", "等待系统释放本地代理端口")
		// Root-owned TUN sockets may remain unavailable to an ordinary user
		// after the child has exited (including TCP TIME_WAIT). Confirm that
		// both endpoints can bind before launching, instead of starting and
		// replacing multiple cores that all fail the same bind.
		if err := core.WaitForListenerRelease(p.ConfigYAML, 45*time.Second); err != nil {
			return err
		}
	}
	report.send("restore", "启动普通代理并验证连接入口")
	if _, err := core.EnsureProxyReady(p.Mihomo, p.ConfigYAML, p.MihomoLog); err != nil {
		return err
	}
	return restoreSelections(p, state, false)
}

func proxyListenerReady(port int) error {
	if port <= 0 {
		return fmt.Errorf("代理内核未提供可用的本地端口")
	}
	connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		return fmt.Errorf("代理端口尚不可用：%w", err)
	}
	return connection.Close()
}

func waitForSession(s session, endpoint string, timeout time.Duration) error {
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); {
		if err := request(s, http.MethodGet, endpoint); err == nil {
			return nil
		}
		if b, err := os.ReadFile(filepath.Join(s.Directory, "error")); err == nil && len(b) > 0 {
			return fmt.Errorf("TUN 启动失败：%s", strings.TrimSpace(string(b)))
		}
		time.Sleep(100 * time.Millisecond)
	}
	if endpoint == "/prepared" {
		return fmt.Errorf("TUN 后台服务未能及时就绪，普通代理未停止")
	}
	return fmt.Errorf("TUN 内核未能及时就绪")
}

// Enable asks macOS for authorization. A supervisor owns the privileged child;
// it waits for an explicit handoff and exits when the owning application exits.
func Enable(p paths.RuntimePaths, parentPID int) (result error) {
	return EnableWithProgress(p, parentPID, nil)
}

func EnableWithProgress(p paths.RuntimePaths, parentPID int, report ProgressFunc) (result error) {
	report.send("prepare", "检查配置与 TUN 系统助手")
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
	handoffStarted := false
	defer func() {
		if result == nil || handoffStarted {
			return
		}
		_ = os.WriteFile(filepath.Join(dir, "cancel"), nil, 0600)
		// If authorization succeeded but startup failed, stop the supervisor before rollback.
		if _, err := stopSession(p, s, Client(p)); err != nil {
			result = errors.Join(result, err)
		}
	}()
	// Validate before asking for authorization or interrupting the current core.
	check := exec.Command(p.Mihomo, "-t", "-d", s.DataDirectory, "-f", filepath.Join(dir, "config.yaml"))
	if err = check.Run(); err != nil {
		return fmt.Errorf("TUN 配置校验失败，原连接未改变")
	}
	if err := prepareServiceWithProgress(s, report); err != nil {
		return err
	}
	handoffStarted = true
	return handoff(state.running, handoffActions{
		prepare: func() error { return waitForSession(s, "/prepared", 8*time.Second) },
		stopNormal: func() (bool, error) {
			report.send("stop", "正在交接普通代理")
			running, pid, err := core.Status()
			if err != nil {
				return false, err
			}
			if !running {
				return true, nil
			}
			if err := core.CheckOwner(pid, p.Mihomo); err != nil {
				return false, err
			}
			if err := core.StopGraceful(pid, 3*time.Second); err != nil {
				return false, err
			}
			return true, core.WaitStopped(pid)
		},
		startTunnel: func() error {
			report.send("start", "建立 TUN 网络接口")
			return request(s, http.MethodPost, "/start")
		},
		confirmTunnel: func() error {
			report.send("verify", "确认隧道与原节点已就绪")
			if err := waitForSession(s, "/status", 8*time.Second); err != nil {
				return err
			}
			return restoreSelections(p, state, true)
		},
		cancelTunnel: func() error {
			report.send("cleanup", "清理未完成的隧道")
			_ = os.WriteFile(filepath.Join(dir, "cancel"), nil, 0600)
			_, err := stopSession(p, s, Client(p))
			return err
		},
		restoreNormal: func() error { return startNormalWithProgress(p, state, report) },
		disableProxy:  sysproxy.Disable,
	})
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
	return stopSession(p, s, Client(p))
}

func stopSession(p paths.RuntimePaths, s session, c *controller.Client) (bool, error) {
	managed := true
	// Cancellation is also observed when the control socket cannot be reached.
	_ = os.WriteFile(filepath.Join(s.Directory, "cancel"), nil, 0600)
	stopErr := request(s, http.MethodPost, "/stop")
	c.SetClient(&http.Client{Timeout: time.Second})
	cfg, configErr := c.GetConfig()
	if configErr == nil && cfg.Tun["enable"] != true {
		// An ordinary core may have been started after an application crash.
		// Discard the stale TUN marker without claiming that this core stopped.
		managed = false
	} else if running, pid, statusErr := core.Status(); statusErr != nil {
		return true, statusErr
	} else if running {
		if ownerErr := core.CheckOwner(pid, p.Mihomo); ownerErr != nil || (configErr == nil && cfg.Tun["enable"] == true) {
			// Verify process exit even after a successful stop acknowledgement.
			// An older installed supervisor may acknowledge before its child
			// finishes releasing listeners. Keep cancellation until then.
			if waitErr := core.WaitStopped(pid); waitErr != nil {
				return true, fmt.Errorf("尚未确认 TUN 内核退出：%w", errors.Join(stopErr, waitErr))
			}
		} else {
			managed = false
		}
	}
	os.Remove(marker(p))
	os.RemoveAll(s.Directory)
	return managed, nil
}

func Disable(p paths.RuntimePaths) error {
	return DisableWithProgress(p, nil)
}

func DisableWithProgress(p paths.RuntimePaths, report ProgressFunc) error {
	report.send("prepare", "保存当前模式与节点")
	state := capture(p)
	report.send("stop", "关闭 TUN 并撤回隧道路由")
	stopped, err := Stop(p)
	if err != nil || !stopped {
		return err
	}
	return startNormalWithProgress(p, state, report)
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
