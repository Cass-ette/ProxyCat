package diagnose

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/controller"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
	"github.com/Cass-ette/ProxyCat/helper/internal/sysproxy"
)

func fixture(t *testing.T) (paths.RuntimePaths, probes) {
	t.Helper()
	p := paths.ForHome(t.TempDir())
	if err := os.MkdirAll(p.Config, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p.Bin, 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{p.ConfigYAML, p.Mihomo} {
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return p, probes{
		core: func(context.Context) (bool, int, error) { return true, 42, nil },
		config: func(context.Context) (*controller.Config, error) {
			return &controller.Config{MixedPort: 7890, Mode: "rule"}, nil
		},
		port: func(_ context.Context, port int) error {
			if port != 7890 {
				t.Fatalf("unexpected port %d", port)
			}
			return nil
		},
		proxy: func(context.Context) (*sysproxy.Status, error) {
			return &sysproxy.Status{
				HTTPEnabled: true, HTTPHost: "127.0.0.1", HTTPPort: 7890,
				HTTPSEnabled: true, HTTPSHost: "127.0.0.1", HTTPSPort: 7890,
				SOCKSEnabled: true, SOCKSHost: "127.0.0.1", SOCKSPort: 7890}, nil
		},
		website: func(_ context.Context, target string) (*controller.WebsiteResult, error) {
			if target != testURL || !strings.HasPrefix(target, "https://") {
				t.Fatalf("unexpected test target %s", target)
			}
			return &controller.WebsiteResult{Outcome: "responded", StatusCode: 204, ElapsedMS: 25}, nil
		},
		tunReady: func(*controller.Config) bool { return true },
	}
}

func TestRuntimeDiagnosisSeparatesFilesAndNetwork(t *testing.T) {
	p, deps := fixture(t)
	report := run(context.Background(), p, deps)
	checks := checksByName(report.Checks)
	for _, name := range []string{"controller", "proxy-listener", "traffic-capture", "https-request"} {
		if checks[name].Status != StatusPass {
			t.Fatalf("%s: %+v", name, checks[name])
		}
	}
	if report.SchemaVersion != 2 || report.ObservedAt == "" || !strings.Contains(report.Scope, "不代表所有网站") {
		t.Fatalf("missing evidence scope: %+v", report)
	}
	for _, check := range report.Checks {
		if check.Category == "" || check.Title == "" {
			t.Fatalf("ungrouped check %+v", check)
		}
	}
}

func TestMissingListenerCannotPassDespiteLivingProcess(t *testing.T) {
	p, deps := fixture(t)
	deps.port = func(context.Context, int) error { return errors.New("connection refused") }
	deps.website = func(context.Context, string) (*controller.WebsiteResult, error) {
		t.Fatal("must not test an unready chain")
		return nil, nil
	}
	checks := checksByName(run(context.Background(), p, deps).Checks)
	if checks["core-process"].Status != StatusPass || checks["proxy-listener"].Status != StatusFail ||
		checks["proxy-listener"].Action != "recoverCore" || checks["traffic-capture"].Status != StatusFail || checks["https-request"].Status != StatusSkipped {
		t.Fatalf("living process certified a dead listener: %+v", checks)
	}
}

func TestStoppedCoreAndUnknownProcessStateStayDistinct(t *testing.T) {
	for _, unreadable := range []bool{false, true} {
		t.Run(strconv.FormatBool(unreadable), func(t *testing.T) {
			p, deps := fixture(t)
			deps.core = func(context.Context) (bool, int, error) {
				if unreadable {
					return false, 0, errors.New("denied")
				}
				return false, 0, nil
			}
			deps.config = func(context.Context) (*controller.Config, error) { return nil, errors.New("offline") }
			deps.port = func(context.Context, int) error { return errors.New("closed") }
			checks := checksByName(run(context.Background(), p, deps).Checks)
			want := StatusWarn
			if unreadable {
				want = StatusSkipped
			}
			if checks["core-process"].Status != want || checks["controller"].Status != StatusFail || checks["https-request"].Status != StatusSkipped {
				t.Fatal(checks)
			}
		})
	}
}

func TestOpenUnrelatedPortAndInvalidControllerAreNotSuccess(t *testing.T) {
	for _, invalid := range []string{"offline", "auth", "invalid", "wrong-port"} {
		t.Run(invalid, func(t *testing.T) {
			p, deps := fixture(t)
			deps.config = func(context.Context) (*controller.Config, error) {
				switch invalid {
				case "offline", "auth":
					return nil, errors.New("http://private.invalid?token=do-not-leak")
				case "invalid":
					return &controller.Config{}, nil
				default:
					return &controller.Config{Mode: "rule", MixedPort: 0}, nil
				}
			}
			deps.website = func(context.Context, string) (*controller.WebsiteResult, error) {
				t.Fatal("unexpected request")
				return nil, nil
			}
			report := run(context.Background(), p, deps)
			checks := checksByName(report.Checks)
			if checks["proxy-listener"].Status == StatusPass || checks["https-request"].Status != StatusSkipped {
				t.Fatal(checks)
			}
			data, _ := json.Marshal(report)
			if strings.Contains(string(data), "do-not-leak") {
				t.Fatal("raw error leaked")
			}
		})
	}
}

func TestTUNAndSystemProxyAreIndependentAndProtected(t *testing.T) {
	for _, ready := range []bool{true, false} {
		t.Run(strconv.FormatBool(ready), func(t *testing.T) {
			p, deps := fixture(t)
			deps.tunSession = true
			deps.config = func(context.Context) (*controller.Config, error) {
				return &controller.Config{Mode: "rule", MixedPort: 7890, Tun: map[string]interface{}{"enable": true}}, nil
			}
			deps.proxy = func(context.Context) (*sysproxy.Status, error) { return &sysproxy.Status{}, nil }
			deps.tunReady = func(*controller.Config) bool { return ready }
			report := run(context.Background(), p, deps)
			capture := checksByName(report.Checks)["traffic-capture"]
			if ready && capture.Status != StatusPass || !ready && capture.Status != StatusWarn {
				t.Fatal(capture)
			}
			deps.port = func(context.Context, int) error { return errors.New("closed") }
			for _, check := range run(context.Background(), p, deps).Checks {
				if check.Action == "recoverCore" || check.Action == "enableSystemProxy" {
					t.Fatalf("unsafe TUN repair %+v", check)
				}
			}
		})
	}
	p, deps := fixture(t)
	deps.tunSession = true
	deps.config = func(context.Context) (*controller.Config, error) { return nil, errors.New("unavailable") }
	for _, check := range run(context.Background(), p, deps).Checks {
		if check.Action == "recoverCore" {
			t.Fatal("stale TUN session permitted replacement")
		}
	}
}

func TestCaptureStatesDoNotClaimAutomaticProxying(t *testing.T) {
	for _, test := range []struct {
		name   string
		status *sysproxy.Status
		err    error
		want   Status
		action string
	}{
		{"off", &sysproxy.Status{}, nil, StatusWarn, "enableSystemProxy"},
		{"other", &sysproxy.Status{HTTPSEnabled: true, HTTPSHost: "192.0.2.1", HTTPSPort: 7890}, nil, StatusWarn, ""},
		{"partial", &sysproxy.Status{HTTPSEnabled: true, HTTPSHost: "127.0.0.1", HTTPSPort: 7890}, nil, StatusWarn, ""},
		{"unreadable", nil, errors.New("denied"), StatusSkipped, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, deps := fixture(t)
			deps.proxy = func(context.Context) (*sysproxy.Status, error) { return test.status, test.err }
			check := checksByName(run(context.Background(), p, deps).Checks)["traffic-capture"]
			if check.Status != test.want || check.Action != test.action {
				t.Fatal(check)
			}
		})
	}
}

func TestModeChangeDuringDiagnosisRetiresNetworkResult(t *testing.T) {
	p, deps := fixture(t)
	deps.website = func(context.Context, string) (*controller.WebsiteResult, error) {
		return &controller.WebsiteResult{Outcome: "responded", StatusCode: 204, Mode: "global"}, nil
	}
	check := checksByName(run(context.Background(), p, deps).Checks)["https-request"]
	if check.Status != StatusSkipped || !strings.Contains(check.Message, "模式发生变化") {
		t.Fatal(check)
	}
}

func TestHTTPSOutcomesAreNotInferredFromTCP(t *testing.T) {
	for _, test := range []struct {
		outcome string
		code    int
		want    Status
	}{
		{"responded", 204, StatusPass}, {"responded", 200, StatusWarn}, {"redirect", 302, StatusWarn},
		{"denied", 403, StatusWarn}, {"httpError", 503, StatusWarn}, {"timeout", 0, StatusFail},
		{"certificate", 0, StatusFail}, {"tlsFailed", 0, StatusFail}, {"tunnelFailed", 0, StatusFail},
		{"connectionFailed", 0, StatusFail}, {"engineUnavailable", 0, StatusFail}, {"proxyUnavailable", 0, StatusFail}, {"unknown", 0, StatusFail},
	} {
		t.Run(test.outcome+strconv.Itoa(test.code), func(t *testing.T) {
			p, deps := fixture(t)
			deps.website = func(ctx context.Context, _ string) (*controller.WebsiteResult, error) {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 8*time.Second {
					t.Fatal("unbounded network test")
				}
				return &controller.WebsiteResult{Outcome: test.outcome, StatusCode: test.code}, nil
			}
			check := checksByName(run(context.Background(), p, deps).Checks)["https-request"]
			if check.Status != test.want {
				t.Fatal(check)
			}
			if test.want != StatusPass && check.Action != "checkWebsite" {
				t.Fatal("missing corresponding action")
			}
		})
	}
}

func TestDiagnosisCancellationAndReadOnlyController(t *testing.T) {
	p, deps := fixture(t)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/configs" {
			t.Errorf("diagnostic mutation: %s %s", r.Method, r.URL.Path)
		}
		<-r.Context().Done()
	}))
	defer api.Close()
	c := controller.NewClient(api.URL)
	deps.config = c.GetConfigContext
	deps.website = func(context.Context, string) (*controller.WebsiteResult, error) {
		t.Fatal("request after cancellation")
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	report := run(ctx, p, deps)
	if time.Since(started) > time.Second || checksByName(report.Checks)["https-request"].Status != StatusSkipped {
		t.Fatal("cancellation did not bound diagnosis")
	}
	before, err := os.ReadFile(p.ConfigYAML)
	if err != nil || string(before) != "fixture" {
		t.Fatal("config changed")
	}
	if _, err := os.Stat(filepath.Join(p.Logs, "mihomo.log")); !os.IsNotExist(err) {
		t.Fatal("diagnosis created a runtime log")
	}
}

func TestPortProbeUsesOnlyIsolatedEphemeralListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := probePort(context.Background(), port); err != nil {
		t.Fatal(err)
	}
	listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := probePort(ctx, port); err == nil {
		t.Fatal("closed port passed")
	}
}
