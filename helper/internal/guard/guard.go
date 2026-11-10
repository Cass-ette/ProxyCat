// Package guard repairs a dead local proxy even when the app's normal exit
// callback cannot run. It only changes endpoints configured by ProxyCat.
package guard

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/core"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
	"github.com/Cass-ette/ProxyCat/helper/internal/sysproxy"
	"github.com/Cass-ette/ProxyCat/helper/internal/tun"
)

// Actions returned by Reconcile.
const (
	// ActionNoop means the system proxy is off, so there is nothing to keep
	// alive and nothing to repair.
	ActionNoop = "noop"
	// ActionHealthy means the proxy is on and the core is reachable.
	ActionHealthy = "healthy"
	// ActionStartedCore means the core had died and was restarted.
	ActionStartedCore = "started-core"
	// ActionDisabledProxy means the core could not be restored, so the system
	// proxy was turned off to keep the machine online.
	ActionDisabledProxy = "disabled-proxy"
	// ActionPending means this run could not decide yet — typically a core that
	// is still booting. The failure counter carries over to the next run.
	ActionPending = "pending"
)

// DefaultFailureThreshold allows transient startup failures before clearing a
// dead local proxy. The watcher checks every five seconds; OS commands and core
// readiness also take time, so this is not an exact outage deadline.
const DefaultFailureThreshold = 3

// Options configures a Reconcile run.
type Options struct {
	Paths paths.RuntimePaths
	// Port is ProxyCat's own mixed port. Zero means 7890. Never infer this from
	// another application's proxy settings.
	Port int
	// FailureThreshold overrides DefaultFailureThreshold when positive.
	FailureThreshold int
	// StatePath stores the failure counter between runs. Empty means
	// <base>/guard-state.json.
	StatePath string
	// DialTimeout is the per-attempt TCP probe timeout. Zero means 800ms.
	DialTimeout time.Duration
}

// Result describes what Reconcile observed and did.
type Result struct {
	Action        string `json:"action"`
	ProxyEnabled  bool   `json:"proxyEnabled"`
	CoreRunning   bool   `json:"coreRunning"`
	TunActive     bool   `json:"tunActive"`
	PortListening bool   `json:"portListening"`
	Port          int    `json:"port"`
	Failures      int    `json:"failures"`
	Repaired      bool   `json:"repaired"`
	Detail        string `json:"detail,omitempty"`
}

type state struct {
	Failures   int    `json:"failures"`
	LastAction string `json:"lastAction"`
	LastRun    string `json:"lastRun"`
}

// The side-effecting calls are variables so tests can exercise every branch of
// the decision logic without touching the real system proxy settings, spawning a
// core, or depending on what happens to be running on the machine.
var (
	getProxyStatus = sysproxy.GetStatus
	disableProxy   = sysproxy.DisableOwned
	coreStatus     = core.Status
	tunIsActive    = tun.Active
	startCore      = ensureCore
	probePort      = PortOpen
	waitPort       = WaitForPort
)

// Reconcile inspects the system proxy and the core, repairs the mismatch if it
// finds one, and reports what it did. It is safe to call repeatedly; a healthy
// system results in ActionHealthy and no side effects.
func Reconcile(opts Options) (Result, error) {
	unlock, err := core.TryLockLifecycle(opts.Paths.Config)
	if errors.Is(err, core.ErrLifecycleBusy) {
		return Result{Action: ActionPending, Detail: "proxy lifecycle operation in progress"}, nil
	}
	if err != nil {
		return Result{}, fmt.Errorf("lock proxy lifecycle: %w", err)
	}
	defer unlock()

	if opts.DialTimeout <= 0 {
		opts.DialTimeout = 800 * time.Millisecond
	}
	threshold := opts.FailureThreshold
	if threshold <= 0 {
		threshold = DefaultFailureThreshold
	}
	statePath := opts.StatePath
	if statePath == "" {
		statePath = filepath.Join(opts.Paths.Base, "guard-state.json")
	}
	prev := readState(statePath)

	status, err := getProxyStatus()
	if err != nil {
		return Result{}, fmt.Errorf("read system proxy status: %w", err)
	}

	port := opts.Port
	if port <= 0 {
		port = 7890
	}
	if !status.UsesLocalPort(port) {
		// Foreign proxy settings are outside this guard's responsibility.
		writeState(statePath, state{Failures: 0, LastAction: ActionNoop, LastRun: now()})
		return Result{Action: ActionNoop}, nil
	}

	result := Result{ProxyEnabled: true, Port: port, Failures: prev.Failures}

	result.TunActive = tunIsActive(opts.Paths)
	running, _, statusErr := coreStatus()
	result.CoreRunning = running || result.TunActive
	reachable := probePort(port, opts.DialTimeout)
	result.PortListening = reachable

	if reachable {
		result.Action = ActionHealthy
		result.Failures = 0
		writeState(statePath, state{Failures: 0, LastAction: ActionHealthy, LastRun: now()})
		return result, nil
	}

	// TUN still needs a mixed listener when system proxies explicitly point to
	// it. Do not launch a competing normal core into an active TUN session.
	detail := "local proxy port closed"
	if result.TunActive {
		detail += "; TUN active, not starting a second core"
	} else if statusErr != nil {
		detail += "; process lookup failed: " + statusErr.Error()
	} else if startErr := startCore(opts.Paths); startErr == nil {
		// Give mihomo a moment to bind the listener. It normally takes well
		// under a second, but a cold config load can be slower.
		if waitPort(port, 5*time.Second, opts.DialTimeout) {
			result.Action = ActionStartedCore
			result.CoreRunning = true
			result.PortListening = true
			result.Repaired = true
			result.Failures = 0
			writeState(statePath, state{Failures: 0, LastAction: ActionStartedCore, LastRun: now()})
			return result, nil
		}
		detail += "; core started but port still closed"
	} else {
		detail += "; core restart failed: " + startErr.Error()
	}

	// The core cannot be brought back. Count the failure and, once the streak
	// proves this is not a one-off race, drop the system proxy so the machine
	// keeps a working direct path to the network.
	prev.Failures++
	result.Failures = prev.Failures
	result.Detail = detail

	if prev.Failures >= threshold {
		if err := disableProxy(port); err != nil {
			result.Action = ActionPending
			writeState(statePath, state{Failures: prev.Failures, LastAction: ActionPending, LastRun: now()})
			return result, fmt.Errorf("disable system proxy after %d failed repairs: %w", prev.Failures, err)
		}
		result.Action = ActionDisabledProxy
		result.ProxyEnabled = false
		result.Repaired = true
		result.Failures = 0
		writeState(statePath, state{Failures: 0, LastAction: ActionDisabledProxy, LastRun: now()})
		return result, nil
	}

	result.Action = ActionPending
	writeState(statePath, state{Failures: prev.Failures, LastAction: ActionPending, LastRun: now()})
	return result, nil
}

// ensureCore starts the core when it is not running. It reuses the same entry
// point as `proxyctl core start` so ownership checks stay in one place.
func ensureCore(p paths.RuntimePaths) error {
	_, err := core.EnsureProxyReady(p.Mihomo, p.ConfigYAML, p.MihomoLog)
	return err
}

func WaitForPort(port int, total, perAttempt time.Duration) bool {
	deadline := time.Now().Add(total)
	for {
		if PortOpen(port, perAttempt) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func PortOpen(port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func readState(path string) state {
	data, err := os.ReadFile(path)
	if err != nil {
		return state{}
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return state{}
	}
	return s
}

func writeState(path string, s state) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	// Best-effort: a missing state file only costs us the failure streak, and
	// the next run simply starts counting again.
	_ = os.WriteFile(path, data, 0o644)
}

func now() string {
	return time.Now().Format(time.RFC3339)
}
