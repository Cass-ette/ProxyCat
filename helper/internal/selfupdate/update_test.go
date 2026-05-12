package selfupdate

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunnerReportsAlreadyLatest(t *testing.T) {
	var out bytes.Buffer
	runner := Runner{CurrentVersion: "0.2.0", Latest: Release{Version: "0.2.0"}}
	code := runner.Run(&out, false)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out.String(), "已经是最新版") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunnerReportsAlreadyLatestJSON(t *testing.T) {
	var out bytes.Buffer
	runner := Runner{CurrentVersion: "0.2.0", Latest: Release{Version: "0.2.0"}}
	code := runner.Run(&out, true)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out.String(), `"stage":"done"`) || !strings.Contains(out.String(), "已经是最新版") {
		t.Fatalf("json output = %q", out.String())
	}
}

func TestRunnerCheckOnlyReportsAvailableUpdate(t *testing.T) {
	var out bytes.Buffer
	runner := Runner{CurrentVersion: "0.1.0", Latest: Release{Version: "0.2.0"}, CheckOnly: true}
	code := runner.Run(&out, true)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out.String(), `"stage":"done"`) || !strings.Contains(out.String(), "发现新版本 0.2.0") {
		t.Fatalf("json output = %q", out.String())
	}
}
