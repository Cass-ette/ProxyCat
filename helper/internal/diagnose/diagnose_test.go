package diagnose

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
)

func TestRunReportsStaticLocalChecks(t *testing.T) {
	temp := t.TempDir()
	p := paths.ForHome(temp)
	if err := os.MkdirAll(p.Config, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(p.SubscriptionsJSON, []byte(`[]`), 0o644); err != nil {
		t.Fatalf("write subscriptions: %v", err)
	}

	report := Run(p)
	if report.App != "ProxyCat" {
		t.Fatalf("App = %q", report.App)
	}
	if report.Milestone != "milestone-1" {
		t.Fatalf("Milestone = %q", report.Milestone)
	}
	if len(report.Checks) != 5 {
		t.Fatalf("len(Checks) = %d, want 5", len(report.Checks))
	}

	checks := checksByName(report.Checks)
	if checks["runtime-paths"].Status != StatusPass {
		t.Fatalf("runtime-paths status = %q", checks["runtime-paths"].Status)
	}
	if checks["subscription-storage"].Status != StatusPass {
		t.Fatalf("subscription-storage status = %q", checks["subscription-storage"].Status)
	}
	if checks["generated-config"].Status != StatusWarn {
		t.Fatalf("generated-config status = %q", checks["generated-config"].Status)
	}
	if checks["mihomo-binary"].Status != StatusWarn {
		t.Fatalf("mihomo-binary status = %q", checks["mihomo-binary"].Status)
	}
	if checks["network-checks"].Status != StatusWarn {
		t.Fatalf("network-checks status = %q", checks["network-checks"].Status)
	}
}

func TestRunReportJSONShape(t *testing.T) {
	report := Run(paths.ForHome(t.TempDir()))
	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}

	var decoded struct {
		App       string `json:"app"`
		Milestone string `json:"milestone"`
		Checks    []struct {
			Name         string `json:"name"`
			Status       string `json:"status"`
			Message      string `json:"message"`
			SuggestedFix string `json:"suggestedFix,omitempty"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if decoded.App != "ProxyCat" || decoded.Milestone != "milestone-1" || len(decoded.Checks) == 0 {
		t.Fatalf("unexpected decoded report: %+v", decoded)
	}
}

func checksByName(checks []Check) map[string]Check {
	byName := make(map[string]Check, len(checks))
	for _, check := range checks {
		byName[check.Name] = check
	}
	return byName
}
