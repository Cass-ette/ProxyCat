package config

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/redact"
	"gopkg.in/yaml.v3"
)

// ValidationResult contains the outcome and details of config validation.
type ValidationResult struct {
	Valid      bool   `json:"valid"`
	Message    string `json:"message"`
	ProxyCount int    `json:"proxyCount"`
	GroupCount int    `json:"groupCount"`
	RuleCount  int    `json:"ruleCount"`
}

// Minimal config structure for validation
type minimalConfig struct {
	Proxies     []yaml.Node `yaml:"proxies"`
	ProxyGroups []yaml.Node `yaml:"proxy-groups"`
	Rules       []yaml.Node `yaml:"rules"`
}

// Validate checks if content is a valid Mihomo/Clash YAML config.
// Requires at least one proxy, one proxy-group, and one rule to be considered valid.
func Validate(content []byte) ValidationResult {
	if len(content) == 0 {
		return ValidationResult{Valid: false, Message: "empty config"}
	}

	var cfg minimalConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return ValidationResult{Valid: false, Message: fmt.Sprintf("invalid YAML: %v", err)}
	}

	proxyCount := len(cfg.Proxies)
	groupCount := len(cfg.ProxyGroups)
	ruleCount := len(cfg.Rules)

	if proxyCount == 0 {
		return ValidationResult{
			Valid:      false,
			Message:    "config missing proxies section",
			ProxyCount: proxyCount,
			GroupCount: groupCount,
			RuleCount:  ruleCount,
		}
	}

	if groupCount == 0 {
		return ValidationResult{
			Valid:      false,
			Message:    "config missing proxy-groups section",
			ProxyCount: proxyCount,
			GroupCount: groupCount,
			RuleCount:  ruleCount,
		}
	}

	if ruleCount == 0 {
		return ValidationResult{
			Valid:      false,
			Message:    "config missing rules section",
			ProxyCount: proxyCount,
			GroupCount: groupCount,
			RuleCount:  ruleCount,
		}
	}

	return ValidationResult{
		Valid:      true,
		Message:    "valid Mihomo config",
		ProxyCount: proxyCount,
		GroupCount: groupCount,
		RuleCount:  ruleCount,
	}
}

// ValidateWithMihomo asks the installed engine to parse a candidate config
// before ProxyCat replaces the current one. The engine data directory matches
// the one used by ordinary and TUN sessions, so relative provider paths resolve
// consistently. The temporary config is private and removed on every exit.
func ValidateWithMihomo(binaryPath, dataDirectory string, content []byte) error {
	if err := ValidateForRuntime(content); err != nil {
		return fmt.Errorf("配置结构无效：%w", err)
	}
	if binaryPath == "" || dataDirectory == "" {
		return fmt.Errorf("无法定位 Mihomo 配置校验环境")
	}
	if err := os.MkdirAll(dataDirectory, 0o755); err != nil {
		return fmt.Errorf("准备 Mihomo 数据目录失败：%w", err)
	}
	temporary, err := os.CreateTemp(dataDirectory, ".proxycat-validate-*.yaml")
	if err != nil {
		return fmt.Errorf("创建临时校验配置失败：%w", err)
	}
	path := temporary.Name()
	defer os.Remove(path)
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("写入临时校验配置失败：%w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("保存临时校验配置失败：%w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭临时校验配置失败：%w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binaryPath, "-t", "-d", filepath.Clean(dataDirectory), "-f", path)
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("Mihomo 配置校验超时（20 秒）")
	}
	detail := strings.TrimSpace(redact.String(string(output)))
	if len(detail) > 800 {
		detail = detail[:800]
	}
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("Mihomo 配置校验失败：%s", detail)
}
