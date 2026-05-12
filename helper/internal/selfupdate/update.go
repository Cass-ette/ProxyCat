package selfupdate

import (
	"encoding/json"
	"fmt"
	"io"
)

type Runner struct {
	CurrentVersion string
	Latest         Release
	CheckOnly      bool
}

type Event struct {
	Stage      string `json:"stage"`
	Message    string `json:"message"`
	Progress   int    `json:"progress,omitempty"`
	NewVersion string `json:"newVersion,omitempty"`
}

func (r Runner) Run(stdout io.Writer, jsonOutput bool) int {
	current, err := parseVersion(r.CurrentVersion)
	if err != nil {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: "更新失败：安装包格式不正确"})
		return 1
	}
	latest, err := parseVersion(r.Latest.Version)
	if err != nil {
		emit(stdout, jsonOutput, Event{Stage: "error", Message: "更新失败：安装包格式不正确"})
		return 1
	}
	if current.compare(latest) >= 0 {
		emit(stdout, jsonOutput, Event{Stage: "done", Message: "已经是最新版"})
		return 0
	}
	if r.CheckOnly {
		emit(stdout, jsonOutput, Event{Stage: "done", Message: "发现新版本 " + r.Latest.Version, NewVersion: r.Latest.Version})
		return 0
	}
	emit(stdout, jsonOutput, Event{Stage: "error", Message: "更新失败：安装尚未完成"})
	return 1
}

func emit(w io.Writer, jsonOutput bool, event Event) {
	if jsonOutput {
		_ = json.NewEncoder(w).Encode(event)
		return
	}
	fmt.Fprintln(w, event.Message)
}
