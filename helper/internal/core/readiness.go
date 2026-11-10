package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/controller"
	"gopkg.in/yaml.v3"
)

// tunActiveError marks a core that belongs to a TUN session. Such a core must
// not be replaced here: the privileged helper owns its lifetime.
type tunActiveError struct{}

func (tunActiveError) Error() string {
	return "TUN 尚未退出，未改动它的监听或启动重复内核"
}

type listenerSettings struct {
	MixedPort  int    `yaml:"mixed-port"`
	Controller string `yaml:"external-controller"`
	Secret     string `yaml:"secret"`
}

// EnsureProxyReady is used under the lifecycle lock for the ordinary core.
// A living process is insufficient: a bind failure leaves Mihomo running with
// mixed-port zero. Once the old listener is gone, retry the listener in that
// owned process instead of starting duplicates or accepting controller-only
// readiness. No system proxy, route or TUN setting is changed here.
//
// If the listener still cannot be recreated in the owned process, the process
// itself is replaced. Mihomo logs a failed bind and keeps running, so without
// that last step every later check sees "running" and reports a healthy core
// that has no port at all — the state that leaves the whole machine offline
// while the menu bar says the proxy is on.
func EnsureProxyReady(binary, config, log string) (int, error) {
	pid, err := EnsureRunning(binary, config, log)
	if err != nil {
		return 0, err
	}
	settings, err := readListenerSettings(config)
	if err != nil {
		return pid, err
	}
	readyErr := recoverListener(settings, 8*time.Second)
	if readyErr == nil {
		return pid, nil
	}
	var tunErr tunActiveError
	if errors.As(readyErr, &tunErr) {
		return pid, readyErr
	}
	newPID, restartErr := replaceCore(binary, config, log, pid)
	if restartErr != nil {
		return pid, fmt.Errorf("%w；替换内核进程失败：%v", readyErr, restartErr)
	}
	if retryErr := recoverListener(settings, 12*time.Second); retryErr != nil {
		return newPID, fmt.Errorf("内核重启后端口仍未恢复：%w", retryErr)
	}
	return newPID, nil
}

func readListenerSettings(config string) (listenerSettings, error) {
	var settings listenerSettings
	data, err := os.ReadFile(config)
	if err != nil {
		return settings, err
	}
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return settings, fmt.Errorf("read local listener configuration: %w", err)
	}
	host, _, err := net.SplitHostPort(settings.Controller)
	if err != nil || host != "127.0.0.1" || settings.MixedPort < 1 || settings.MixedPort > 65535 {
		return settings, fmt.Errorf("普通代理的本地监听配置无效")
	}
	return settings, nil
}

func recoverListener(settings listenerSettings, total time.Duration) error {
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := controller.NewClientWithSecret("http://"+settings.Controller, settings.Secret)
	client.SetClient(&http.Client{Transport: transport, Timeout: 500 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), total)
	defer cancel()
	return waitForOrdinaryListener(ctx, client, settings.MixedPort)
}

func replaceCore(binary, config, log string, pid int) (int, error) {
	if pid > 0 {
		// Readiness retries take time. Recheck ownership immediately before a
		// signal so a PID that exited or changed is never blindly terminated.
		if err := CheckOwner(pid, binary); err != nil {
			return 0, err
		}
		if err := StopGraceful(pid, 3*time.Second); err != nil {
			return 0, err
		}
	}
	return Start(binary, config, log)
}

func waitForOrdinaryListener(ctx context.Context, client *controller.Client, port int) error {
	var lastErr error
	nextPatch := time.Time{}
	for {
		cfg, err := client.GetConfig()
		if err == nil && cfg.Tun["enable"] == true {
			return tunActiveError{}
		}
		lastErr = err
		if err == nil {
			address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
			conn, dialErr := (&net.Dialer{Timeout: 100 * time.Millisecond}).DialContext(ctx, "tcp", address)
			if dialErr == nil {
				conn.Close()
				if cfg.MixedPort == port {
					return nil
				}
				// Someone else is listening. Do not take it as our readiness or
				// repeatedly patch against another process's occupied port.
				lastErr = fmt.Errorf("本地端口仍被占用，内核未确认监听")
			} else if !time.Now().Before(nextPatch) {
				lastErr = client.SetMixedPort(ctx, port)
				nextPatch = time.Now().Add(500 * time.Millisecond)
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("普通代理端口 %d 未能恢复，请稍后重试：%v", port, lastErr)
		case <-time.After(100 * time.Millisecond):
		}
	}
}
