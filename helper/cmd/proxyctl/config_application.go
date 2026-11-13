package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/controller"
	"github.com/Cass-ette/ProxyCat/helper/internal/core"
	"github.com/Cass-ette/ProxyCat/helper/internal/maintenance"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
	"github.com/Cass-ette/ProxyCat/helper/internal/profile"
	"gopkg.in/yaml.v3"
)

type configApplication struct {
	State   string `json:"applicationState"`
	Message string `json:"applicationMessage"`
}

type applicationActions struct {
	// A session marker also protects a TUN handoff whose controller is not ready.
	tunSession  func() bool
	running     func() (bool, int, error)
	owned       func(int) error
	config      func() (*controller.Config, error)
	proxies     func() (map[string]controller.Proxy, error)
	reload      func([]byte) error
	selectProxy func(string, string) error
	ready       func(string) error
}

func normalApplicationActions(p paths.RuntimePaths) applicationActions {
	c := controller.NewClient("")
	transport := &http.Transport{Proxy: nil}
	c.SetClient(&http.Client{Transport: transport, Timeout: 3 * time.Second})
	// Do not keep sockets open across core lifecycle operations.
	transport.DisableKeepAlives = true
	return applicationActions{
		tunSession: func() bool {
			_, err := os.Stat(filepath.Join(p.Config, "tun-session.json"))
			return !os.IsNotExist(err)
		},
		running: core.Status, owned: func(pid int) error { return core.CheckOwner(pid, p.Mihomo) },
		config: c.GetConfig, proxies: c.GetProxies, reload: c.ReloadConfig, selectProxy: c.SelectProxy,
		ready: func(mode string) error {
			for deadline := time.Now().Add(4 * time.Second); ; {
				cfg, err := c.GetConfig()
				if err == nil && cfg.Mode == mode && cfg.Tun["enable"] != true && cfg.MixedPort == 7890 && core.ProbePort(7890, 200*time.Millisecond) {
					return nil
				}
				if time.Now().After(deadline) {
					return fmt.Errorf("应用后控制接口或本地代理端口未就绪")
				}
				time.Sleep(100 * time.Millisecond)
			}
		},
	}
}

func withLiveMode(content []byte, mode string) ([]byte, error) {
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("没有可应用的配置")
	}
	if mode != "rule" && mode != "global" && mode != "direct" {
		return nil, fmt.Errorf("无法确认当前代理模式")
	}
	cfg["mode"] = mode
	return yaml.Marshal(cfg)
}

func restoreApplicationSelections(a applicationActions, previous map[string]controller.Proxy) error {
	current, err := a.proxies()
	if err != nil {
		return err
	}
	for name, old := range previous {
		group, exists := current[name]
		if old.Type != "Selector" || old.Now == "" || !exists || group.Type != "Selector" || !slices.Contains(group.All, old.Now) {
			continue
		}
		if group.Now != old.Now {
			if err := a.selectProxy(name, old.Now); err != nil {
				return fmt.Errorf("恢复节点选择失败：%w", err)
			}
		}
	}
	confirmed, err := a.proxies()
	if err != nil {
		return err
	}
	for name, old := range previous {
		if group, ok := confirmed[name]; ok && old.Type == "Selector" && old.Now != "" && group.Type == "Selector" && slices.Contains(group.All, old.Now) && group.Now != old.Now {
			return fmt.Errorf("节点选择尚未确认")
		}
	}
	return nil
}

func applyConfiguration(previous, next []byte, a applicationActions) (configApplication, error) {
	if a.tunSession() {
		return configApplication{"pending", "新配置已保存；当前 TUN 沿用原配置，下次关闭或重新开启 TUN 时应用。"}, nil
	}
	running, pid, err := a.running()
	if err != nil {
		return configApplication{}, err
	}
	if !running {
		return configApplication{"stopped", "新配置已保存，下次启动代理引擎时应用。"}, nil
	}
	if err := a.owned(pid); err != nil {
		return configApplication{}, err
	}
	live, err := a.config()
	if err != nil || live == nil {
		return configApplication{}, fmt.Errorf("无法确认当前内核配置，未尝试应用")
	}
	if live.Tun["enable"] == true {
		return configApplication{"pending", "新配置已保存；TUN 当前会话未改动，请完成 TUN 切换后再检查。"}, nil
	}
	old, err := withLiveMode(previous, live.Mode)
	if err != nil {
		return configApplication{}, fmt.Errorf("无法准备旧配置回退：%w", err)
	}
	candidate, err := withLiveMode(next, live.Mode)
	if err != nil {
		return configApplication{}, err
	}
	selected, err := a.proxies()
	if err != nil {
		return configApplication{}, fmt.Errorf("无法保存当前节点选择，未尝试应用")
	}
	apply := func(content []byte) error {
		if err := a.reload(content); err != nil {
			return err
		}
		if err := a.ready(live.Mode); err != nil {
			return err
		}
		return restoreApplicationSelections(a, selected)
	}
	if err := apply(candidate); err != nil {
		if rollbackErr := apply(old); rollbackErr != nil {
			return configApplication{"unknown", "新配置应用失败，旧配置恢复未能确认；请重新诊断。"}, fmt.Errorf("新配置应用失败，旧配置恢复也未能确认：%w", errors.Join(err, rollbackErr))
		}
		return configApplication{"rolledBack", "新配置未生效，已恢复并确认旧配置。"}, fmt.Errorf("新配置应用失败，已恢复并确认旧配置：%w", err)
	}
	return configApplication{"applied", "新配置已应用，本地代理端口、控制接口与原模式已确认。"}, nil
}

// Replaceable only in CLI tests, which must never contact the developer's core.
var applyConfigurationRuntime = func(p paths.RuntimePaths, previous, next []byte) (configApplication, error) {
	return applyConfiguration(previous, next, normalApplicationActions(p))
}

// Called while holding the shared lifecycle lock. Failed runtime application
// restores every affected profile/base/preference file, not only config.yaml.
func commitConfiguration(p paths.RuntimePaths, updates map[string][]byte) (configApplication, error) {
	old := map[string][]byte{}
	missing := []string{}
	for path := range updates {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			missing = append(missing, path)
			continue
		}
		if err != nil {
			return configApplication{}, err
		}
		old[path] = data
	}
	if err := profile.AtomicWriteSet(updates, 0o644); err != nil {
		return configApplication{}, err
	}
	result := configApplication{"inactive", "已保存；当前活动配置未改变。"}
	if candidate, changed := updates[p.ConfigYAML]; changed {
		var err error
		result, err = applyConfigurationRuntime(p, old[p.ConfigYAML], candidate)
		if err != nil {
			restoreErr := profile.AtomicWriteSet(old, 0o644)
			for _, path := range missing {
				restoreErr = errors.Join(restoreErr, os.Remove(path))
			}
			if restoreErr != nil {
				result = configApplication{"unknown", "配置文件恢复失败，当前生效状态需要重新检查。"}
				err = errors.Join(err, fmt.Errorf("恢复已保存文件失败：%w", restoreErr))
			}
			if result.State != "" {
				if statusErr := recordConfigApplication(p, result); statusErr != nil {
					err = errors.Join(err, fmt.Errorf("保存配置失败状态：%w", statusErr))
				}
			}
			return result, err
		}
		if err := recordConfigApplication(p, result); err != nil {
			result.Message += " 更新时间状态未能保存，请重新检查。"
		}
	}
	return result, nil
}

func recordConfigApplication(p paths.RuntimePaths, result configApplication) error {
	status, err := maintenance.Load(p.MaintenanceJSON)
	if err != nil {
		return err
	}
	status.ConfigApplicationState, status.ConfigApplicationMessage = result.State, result.Message
	content, err := os.ReadFile(p.ConfigYAML)
	if err != nil {
		return err
	}
	status.ConfigSHA256 = configurationHash(content)
	return maintenance.Save(p.MaintenanceJSON, status)
}

func configurationHash(content []byte) string { return fmt.Sprintf("%x", sha256.Sum256(content)) }
