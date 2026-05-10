package diagnose

import (
	"os"

	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
)

type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

type Report struct {
	App       string  `json:"app"`
	Milestone string  `json:"milestone"`
	Checks    []Check `json:"checks"`
}

type Check struct {
	Name         string `json:"name"`
	Status       Status `json:"status"`
	Message      string `json:"message"`
	SuggestedFix string `json:"suggestedFix"`
}

func Run(p paths.RuntimePaths) Report {
	return Report{
		App:       "ProxyCat",
		Milestone: "milestone-1",
		Checks: []Check{
			checkRuntimePaths(p),
			checkSubscriptionStorage(p),
			checkGeneratedConfig(p),
			checkMihomoBinary(p),
		},
	}
}

func checkRuntimePaths(p paths.RuntimePaths) Check {
	if p.Base == "" || p.Config == "" || p.ConfigYAML == "" || p.SubscriptionsJSON == "" || p.Mihomo == "" || p.DiagnoseLatest == "" {
		return Check{Name: "runtime-paths", Status: StatusFail, Message: "Runtime paths are incomplete.", SuggestedFix: "Check ProxyCat path configuration."}
	}
	return Check{Name: "runtime-paths", Status: StatusPass, Message: "Runtime paths resolved."}
}

func checkSubscriptionStorage(p paths.RuntimePaths) Check {
	if fileExists(p.SubscriptionsJSON) {
		return Check{Name: "subscription-storage", Status: StatusPass, Message: "Subscription storage exists."}
	}
	return Check{Name: "subscription-storage", Status: StatusWarn, Message: "No subscription storage found.", SuggestedFix: "Run subscription add in Milestone 2."}
}

func checkGeneratedConfig(p paths.RuntimePaths) Check {
	if fileExists(p.ConfigYAML) {
		return Check{Name: "generated-config", Status: StatusPass, Message: "Generated config exists."}
	}
	return Check{Name: "generated-config", Status: StatusWarn, Message: "No generated config found.", SuggestedFix: "Run config generate after adding a subscription."}
}

func checkMihomoBinary(p paths.RuntimePaths) Check {
	if fileExists(p.Mihomo) {
		return Check{Name: "mihomo-binary", Status: StatusPass, Message: "Mihomo binary exists."}
	}
	return Check{Name: "mihomo-binary", Status: StatusWarn, Message: "Mihomo binary not found.", SuggestedFix: "Choose a Mihomo binary distribution mode before core management."}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
