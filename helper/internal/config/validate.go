package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type ValidationResult struct {
	Valid       bool   `json:"valid"`
	Message     string `json:"message"`
	ProxyCount  int    `json:"proxyCount"`
	GroupCount  int    `json:"groupCount"`
	RuleCount   int    `json:"ruleCount"`
}

// Minimal config structure for validation
type minimalConfig struct {
	Proxies     []yaml.Node `yaml:"proxies"`
	ProxyGroups []yaml.Node `yaml:"proxy-groups"`
	Rules       []yaml.Node `yaml:"rules"`
}

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
			Valid:       false,
			Message:     "config missing proxies section",
			ProxyCount:  proxyCount,
			GroupCount:  groupCount,
			RuleCount:   ruleCount,
		}
	}

	if groupCount == 0 {
		return ValidationResult{
			Valid:       false,
			Message:     "config missing proxy-groups section",
			ProxyCount:  proxyCount,
			GroupCount:  groupCount,
			RuleCount:   ruleCount,
		}
	}

	if ruleCount == 0 {
		return ValidationResult{
			Valid:       false,
			Message:     "config missing rules section",
			ProxyCount:  proxyCount,
			GroupCount:  groupCount,
			RuleCount:   ruleCount,
		}
	}

	return ValidationResult{
		Valid:       true,
		Message:     "valid Mihomo config",
		ProxyCount:  proxyCount,
		GroupCount:  groupCount,
		RuleCount:   ruleCount,
	}
}
