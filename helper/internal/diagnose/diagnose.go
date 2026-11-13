package diagnose

import (
	"os"

	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
)

type Status string

const (
	StatusPass    Status = "pass"
	StatusWarn    Status = "warn"
	StatusFail    Status = "fail"
	StatusSkipped Status = "skipped"
)

type Report struct {
	App           string  `json:"app"`
	Milestone     string  `json:"milestone"`
	Checks        []Check `json:"checks"`
	SchemaVersion int     `json:"schemaVersion,omitempty"`
	ObservedAt    string  `json:"observedAt,omitempty"`
	Scope         string  `json:"scope,omitempty"`
}

type Check struct {
	Name         string `json:"name"`
	Status       Status `json:"status"`
	Message      string `json:"message"`
	SuggestedFix string `json:"suggestedFix"`
	Category     string `json:"category,omitempty"`
	Title        string `json:"title,omitempty"`
	Action       string `json:"action,omitempty"`
}

func RunStatic(p paths.RuntimePaths) Report {
	report := Report{
		App:       "ProxyCat",
		Milestone: "milestone-1",
		Checks: []Check{
			checkRuntimePaths(p),
			checkProfilesStorage(p),
			checkSubscriptionStorage(p),
			checkGeneratedConfig(p),
			checkMihomoBinary(p),
		},
	}
	for i := range report.Checks {
		report.Checks[i].Category = "static"
	}
	return report
}

func checkRuntimePaths(p paths.RuntimePaths) Check {
	if p.Base == "" || p.Config == "" || p.ConfigYAML == "" || p.SubscriptionsJSON == "" || p.Mihomo == "" || p.DiagnoseLatest == "" {
		return Check{Name: "runtime-paths", Title: "文件路径", Status: StatusFail, Message: "运行路径不完整。", SuggestedFix: "请重新安装应用，或查看日志。", Action: "openLogs"}
	}
	return Check{Name: "runtime-paths", Title: "文件路径", Status: StatusPass, Message: "运行路径已确定；不代表网络可用。"}
}

func checkProfilesStorage(p paths.RuntimePaths) Check {
	info, err := os.Stat(p.ProfilesDir)
	if err == nil && info.IsDir() {
		return Check{Name: "profiles-storage", Title: "配置存储", Status: StatusPass, Message: "配置目录存在。"}
	}
	return Check{Name: "profiles-storage", Title: "配置存储", Status: StatusWarn, Message: "尚无配置目录。", SuggestedFix: "在快速开始中导入订阅。"}
}

func checkSubscriptionStorage(p paths.RuntimePaths) Check {
	if fileExists(p.SubscriptionsJSON) {
		return Check{Name: "subscription-storage", Title: "订阅存储", Status: StatusPass, Message: "订阅存储文件存在。"}
	}
	return Check{Name: "subscription-storage", Title: "订阅存储", Status: StatusWarn, Message: "尚无订阅存储文件。", SuggestedFix: "在快速开始中导入订阅。"}
}

func checkGeneratedConfig(p paths.RuntimePaths) Check {
	if fileExists(p.ConfigYAML) {
		return Check{Name: "generated-config", Title: "内核配置文件", Status: StatusPass, Message: "配置文件存在；未验证其语法或规则。"}
	}
	return Check{Name: "generated-config", Title: "内核配置文件", Status: StatusWarn, Message: "尚无内核配置文件。", SuggestedFix: "先导入订阅并生成配置。"}
}

func checkMihomoBinary(p paths.RuntimePaths) Check {
	if fileExists(p.Mihomo) {
		return Check{Name: "mihomo-binary", Title: "内核文件", Status: StatusPass, Message: "内核文件存在；是否能运行需看运行检查。"}
	}
	return Check{Name: "mihomo-binary", Title: "内核文件", Status: StatusWarn, Message: "尚未找到内核文件。", SuggestedFix: "先导入订阅并启动；仍失败时查看日志。", Action: "openLogs"}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
