package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/config"
	"github.com/Cass-ette/ProxyCat/helper/internal/controller"
	"github.com/Cass-ette/ProxyCat/helper/internal/core"
	"github.com/Cass-ette/ProxyCat/helper/internal/diagnose"
	"github.com/Cass-ette/ProxyCat/helper/internal/guard"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
	"github.com/Cass-ette/ProxyCat/helper/internal/profile"
	"github.com/Cass-ette/ProxyCat/helper/internal/redact"
	"github.com/Cass-ette/ProxyCat/helper/internal/selfupdate"
	"github.com/Cass-ette/ProxyCat/helper/internal/subscription"
	"github.com/Cass-ette/ProxyCat/helper/internal/sysproxy"
	"github.com/Cass-ette/ProxyCat/helper/internal/tun"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printHelp(stdout)
		return 0
	}

	jsonOutput := false
	filteredArgs := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		filteredArgs = append(filteredArgs, arg)
	}
	args = filteredArgs
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}
	if needsLifecycleLock(args) {
		p, err := paths.Default()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		unlock, err := core.LockLifecycle(p.Config)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		defer unlock()
	}

	switch args[0] {
	case "tun":
		if len(args) == 2 && args[1] == "relay" {
			if err := tun.RunRelay(os.Stdin, stdout); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			return 0
		}
		if len(args) == 2 && args[1] == "install-helper" {
			if err := tun.InstallHelper(); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			return 0
		}
		if len(args) == 3 && (args[1] == "service" || args[1] == "install-service" || args[1] == "remove-service") {
			uid, err := strconv.Atoi(args[2])
			if err == nil && uid > 0 {
				switch args[1] {
				case "service":
					err = tun.RunService(uid)
				case "install-service":
					err = tun.InstallService(uid)
				case "remove-service":
					err = tun.UninstallService(uid)
				}
			} else {
				err = fmt.Errorf("invalid TUN helper owner")
			}
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			return 0
		}
		if len(args) == 4 && args[1] == "service-session" {
			uid, err := strconv.Atoi(args[3])
			if err == nil {
				err = tun.ServeServiceSession(args[2], uid)
			}
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			return 0
		}
		if len(args) == 2 && args[1] == "uninstall-helper" {
			p, err := paths.Default()
			if err == nil && tun.Active(p) {
				err = fmt.Errorf("请先关闭 TUN，再卸载系统助手")
			}
			if err == nil {
				err = tun.RemoveService()
			}
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			return 0
		}
		if len(args) == 3 && args[1] == "serve" {
			if err := tun.Serve(args[2]); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			return 0
		}
		if len(args) != 2 || (args[1] != "on" && args[1] != "off") {
			fmt.Fprintln(stderr, "tun requires on or off")
			return 2
		}
		p, err := paths.Default()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		var report tun.ProgressFunc
		if jsonOutput {
			encoder := json.NewEncoder(stdout)
			report = func(progress tun.Progress) { _ = encoder.Encode(progress) }
		}
		if args[1] == "on" {
			err = tun.EnableWithProgress(p, os.Getppid(), report)
		} else {
			err = tun.DisableWithProgress(p, report)
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "diagnose":
		return runDiagnose(args[1:], jsonOutput, stdout, stderr)
	case "bootstrap":
		if len(args) < 2 {
			fmt.Fprintf(stderr, "bootstrap requires subscription URL\n")
			return 2
		}
		return runBootstrap(args[1], stdout, stderr)
	case "subscription":
		if len(args) < 2 {
			fmt.Fprintf(stderr, "subscription subcommand required: add, list, update, probe\n")
			printHelp(stderr)
			return 2
		}
		switch args[1] {
		case "add":
			if len(args) < 3 {
				fmt.Fprintf(stderr, "subscription add requires URL\n")
				return 2
			}
			return runSubscriptionAdd(args[2], stdout, stderr)
		case "list":
			return runSubscriptionList(stdout, stderr)
		case "update":
			return runSubscriptionUpdate(stdout, stderr)
		case "probe":
			if len(args) < 3 {
				fmt.Fprintf(stderr, "subscription probe requires URL\n")
				return 2
			}
			return runSubscriptionProbe(args[2], stdout, stderr, jsonOutput)
		default:
			fmt.Fprintf(stderr, "unknown subscription subcommand: %s\n", redact.String(args[1]))
			return 2
		}
	case "config":
		if len(args) < 2 {
			fmt.Fprintf(stderr, "config subcommand required: generate\n")
			return 2
		}
		switch args[1] {
		case "generate":
			return runConfigGenerate(stdout, stderr)
		default:
			fmt.Fprintf(stderr, "unknown config subcommand: %s\n", redact.String(args[1]))
			return 2
		}
	case "core":
		if len(args) < 2 {
			fmt.Fprintf(stderr, "core subcommand required: start, stop, restart, status\n")
			return 2
		}
		switch args[1] {
		case "install":
			return runCoreInstall(stdout, stderr)
		case "start":
			return runCoreStart(stdout, stderr)
		case "stop":
			// An explicit stop is a disconnect, not a crash to be repaired. Clear
			// the system proxy and its watcher before releasing the core. Internal
			// restart paths call runCoreStop directly under the same lifecycle lock.
			if exit := runSystemProxyOff(stdout, stderr); exit != 0 {
				return exit
			}
			return runCoreStop(stdout, stderr)
		case "restart":
			return runCoreRestart(stdout, stderr)
		case "status":
			return runCoreStatusJSON(stdout, stderr, jsonOutput)
		default:
			fmt.Fprintf(stderr, "unknown core subcommand: %s\n", redact.String(args[1]))
			return 2
		}

	case "guard":
		for _, a := range args[1:] {
			if a == "--watch" {
				return runGuardWatch(stdout, stderr, jsonOutput)
			}
		}
		return runGuard(stdout, stderr, jsonOutput)

	case "system-proxy":
		if len(args) < 2 {
			fmt.Fprintf(stderr, "system-proxy subcommand required: on, off, status\n")
			return 2
		}
		switch args[1] {
		case "on":
			return runSystemProxyOn(stdout, stderr)
		case "off":
			return runSystemProxyOff(stdout, stderr)
		case "status":
			return runSystemProxyStatusJSON(stdout, stderr, jsonOutput)
		default:
			fmt.Fprintf(stderr, "unknown system-proxy subcommand: %s\n", redact.String(args[1]))
			return 2
		}

	case "mode":
		if len(args) < 2 {
			fmt.Fprintf(stderr, "mode subcommand required: status, set\n")
			return 2
		}
		switch args[1] {
		case "status":
			return runModeStatusJSON(stdout, stderr, jsonOutput)
		case "set":
			if len(args) < 3 {
				fmt.Fprintf(stderr, "mode set requires <rule|global|direct>\n")
				return 2
			}
			return runModeSet(args[2], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "unknown mode subcommand: %s\n", redact.String(args[1]))
			return 2
		}

	case "groups":
		if len(args) < 2 {
			return runGroupsJSON(stdout, stderr, jsonOutput)
		}
		switch args[1] {
		case "list":
			return runGroupsJSON(stdout, stderr, jsonOutput)
		case "delay":
			return runGroupsDelay(stdout, stderr, jsonOutput)
		case "select":
			if len(args) < 4 {
				fmt.Fprintf(stderr, "groups select requires <group> <proxy>\n")
				return 2
			}
			return runGroupsSelect(args[2], args[3], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "unknown groups subcommand: %s\n", redact.String(args[1]))
			return 2
		}

	case "proxy-delay":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "proxy-delay requires <proxy-or-group-name>")
			return 2
		}
		return runProxyDelay(args[1], stdout, stderr, jsonOutput)

	case "connections":
		if len(args) == 2 && args[1] == "prepare" {
			if err := runtimeController().EnableConnectionProcessLookup(context.Background()); err != nil {
				fmt.Fprintln(stderr, "应用识别暂不可用；仍可查看未识别应用的连接")
				return 1
			}
			return 0
		}
		if len(args) != 1 {
			fmt.Fprintln(stderr, "connections takes no arguments")
			return 2
		}
		result, err := runtimeController().GetConnections(context.Background())
		if err != nil {
			fmt.Fprintln(stderr, "无法读取连接记录，请确认代理引擎正在运行")
			return 1
		}
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			return 1
		}
		return 0
	case "website-check":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "website-check requires a URL")
			return 2
		}
		result, err := runtimeController().CheckWebsite(context.Background(), args[1])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			return 1
		}
		return 0
	case "test":
		return runTestJSON(stdout, stderr, jsonOutput)

	case "self-update", "update":
		expectedVersion := ""
		for i, arg := range args[1:] {
			if arg == "--expected-version" {
				if i+2 >= len(args) || strings.HasPrefix(args[i+2], "--") {
					fmt.Fprintln(stderr, "--expected-version requires a version")
					return 2
				}
				expectedVersion = args[i+2]
			}
		}
		return runSelfUpdate(stdout, stderr, jsonOutput, containsArg(args[1:], "--check-only"), expectedVersion)

	case "select":
		if len(args) < 3 {
			fmt.Fprintf(stderr, "select requires group and proxy arguments\n")
			return 2
		}
		return runSelect(args[1], args[2], stdout, stderr)

	case "profile":
		if len(args) < 2 {
			fmt.Fprintf(stderr, "profile subcommand required: list, activate, delete\n")
			return 2
		}
		switch args[1] {
		case "list":
			return runProfileList(stdout, stderr, jsonOutput)
		case "traffic":
			if len(args) != 3 {
				fmt.Fprintln(stderr, "profile traffic requires <id>")
				return 2
			}
			return runProfileTraffic(args[2], stdout, stderr)
		case "activate":
			if len(args) < 3 {
				fmt.Fprintf(stderr, "profile activate requires <id>\n")
				return 2
			}
			return runProfileActivate(args[2], stdout, stderr)
		case "delete":
			if len(args) < 3 {
				fmt.Fprintf(stderr, "profile delete requires <id>\n")
				return 2
			}
			return runProfileDelete(args[2], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "unknown profile subcommand: %s\n", redact.String(args[1]))
			return 2
		}

	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", redact.String(args[0]))
		printHelp(stderr)
		return 2
	}
}

func needsLifecycleLock(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if args[0] == "bootstrap" {
		return true
	}
	if len(args) < 2 {
		return false
	}
	switch args[0] {
	case "tun":
		return args[1] == "on" || args[1] == "off" || args[1] == "install-helper" || args[1] == "uninstall-helper"
	case "core":
		return args[1] == "start" || args[1] == "stop" || args[1] == "restart"
	case "system-proxy":
		return args[1] == "on" || args[1] == "off"
	}
	return false
}

func runtimeController() *controller.Client {
	p, err := paths.Default()
	if err != nil {
		return controller.NewClient("")
	}
	return tun.Client(p)
}

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

// Command-line HTTP clients do not inherit macOS system proxy settings.
// Honor the enabled proxy so checking updates uses the same route as the app.
func updateHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if status, err := sysproxy.GetStatus(); err == nil && status.HTTPSEnabled && status.HTTPSHost != "" && status.HTTPSPort > 0 {
		transport.Proxy = http.ProxyURL(&url.URL{Scheme: "http", Host: net.JoinHostPort(status.HTTPSHost, strconv.Itoa(status.HTTPSPort))})
	}
	return &http.Client{Transport: transport, Timeout: 90 * time.Second}
}

func runSelfUpdate(stdout io.Writer, stderr io.Writer, jsonOutput bool, checkOnly bool, expectedVersion string) int {
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "resolve executable: %v\n", err)
		return 1
	}
	appPath, err := appPathFromProxyctlExecutable(executable)
	if err != nil {
		fmt.Fprintf(stderr, "resolve app path: %v\n", err)
		return 1
	}

	currentVersion, err := selfupdate.ReadBundleVersion(appPath)
	if err != nil {
		fmt.Fprintf(stderr, "read current version: %v\n", err)
		return 1
	}

	runner := selfupdate.Runner{
		CurrentVersion:   currentVersion,
		ExpectedVersion:  expectedVersion,
		CheckOnly:        checkOnly,
		Client:           updateHTTPClient(),
		Endpoint:         "https://github.com/Cass-ette/ProxyCat/releases/latest/download/update.json",
		FallbackEndpoint: "https://api.github.com/repos/Cass-ette/ProxyCat/releases/latest",
		AppPath:          appPath,
		BackupDir:        filepath.Join(filepath.Dir(appPath), "ProxyCat-Backups"),
	}
	return runner.Run(stdout, jsonOutput)
}

func appPathFromProxyctlExecutable(executable string) (string, error) {
	return filepath.Abs(filepath.Join(filepath.Dir(executable), "..", ".."))
}

func runBootstrap(url string, stdout io.Writer, stderr io.Writer) int {
	fmt.Fprintf(stdout, "Saving subscription: %s\n", redact.URL(url))
	profileID, err := saveProfileSubscription(url)
	if err != nil {
		fmt.Fprintf(stderr, "save subscription: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "Generating config...")
	if exit := runConfigGenerateForProfile(profileID, stdout, stderr); exit != 0 {
		return exit
	}

	// Activate: copy profile config to active config.yaml
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}
	if err := profile.Activate(runtimePaths.ProfilesDir, profileID, runtimePaths.ConfigYAML); err != nil {
		fmt.Fprintf(stderr, "activate profile: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "Ensuring Mihomo core...")
	if exit := runCoreInstall(stdout, stderr); exit != 0 {
		return exit
	}

	fmt.Fprintln(stdout, "Restarting core...")
	if exit := runCoreRestart(stdout, stderr); exit != 0 {
		return exit
	}

	if err := waitForController(8 * time.Second); err != nil {
		fmt.Fprintf(stderr, "wait for controller: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "Enabling system proxy...")
	if exit := runSystemProxyOn(stdout, stderr); exit != 0 {
		return exit
	}

	fmt.Fprintln(stdout, "Testing connection...")
	if exit := runTestJSON(stdout, stderr, false); exit != 0 {
		return exit
	}

	fmt.Fprintln(stdout, "ProxyCat is connected")
	return 0
}

func waitForController(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := runtimeController()
	var lastErr error
	for time.Now().Before(deadline) {
		if _, err := client.GetConfig(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(300 * time.Millisecond)
	}
	return lastErr
}

func runDiagnose(args []string, jsonOutput bool, stdout io.Writer, stderr io.Writer) int {
	for _, arg := range args {
		fmt.Fprintf(stderr, "unknown diagnose flag: %s\n", redact.String(arg))
		return 2
	}

	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve runtime paths: %v\n", err)
		return 1
	}
	report := diagnose.Run(runtimePaths)
	if jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "encode diagnose report: %v\n", err)
			return 1
		}
		return 0
	}

	fmt.Fprintln(stdout, "ProxyCat Diagnose")
	for _, check := range report.Checks {
		fmt.Fprintf(stdout, "- %s: %s - %s\n", check.Name, check.Status, check.Message)
		if check.SuggestedFix != "" {
			fmt.Fprintf(stdout, "  Suggested fix: %s\n", check.SuggestedFix)
		}
	}
	return 0
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "ProxyCat proxyctl")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  proxyctl diagnose [--json]")
	fmt.Fprintln(w, "  proxyctl bootstrap <url>")
	fmt.Fprintln(w, "  proxyctl subscription add <url>")
	fmt.Fprintln(w, "  proxyctl subscription list")
	fmt.Fprintln(w, "  proxyctl subscription update")
	fmt.Fprintln(w, "  proxyctl subscription probe <url> [--json]")
	fmt.Fprintln(w, "  proxyctl config generate")
	fmt.Fprintln(w, "  proxyctl core {install|start|stop|restart|status}")
	fmt.Fprintln(w, "  proxyctl tun {on|off|install-helper|uninstall-helper}")
	fmt.Fprintln(w, "  proxyctl system-proxy {on|off|status}")
	fmt.Fprintln(w, "  proxyctl guard [--json] [--watch]")
	fmt.Fprintln(w, "  proxyctl mode {status|set <rule|global|direct>}")
	fmt.Fprintln(w, "  proxyctl groups [list]")
	fmt.Fprintln(w, "  proxyctl groups delay [--json]")
	fmt.Fprintln(w, "  proxyctl groups select <group> <proxy>")
	fmt.Fprintln(w, "  proxyctl proxy-delay <name> [--json]")
	fmt.Fprintln(w, "  proxyctl test")
	fmt.Fprintln(w, "  proxyctl website-check <url> [--json]")
	fmt.Fprintln(w, "  proxyctl connections [--json]")
	fmt.Fprintln(w, "  proxyctl connections prepare")
	fmt.Fprintln(w, "  proxyctl self-update [--json] [--check-only]")
	fmt.Fprintln(w, "  proxyctl update [--json] [--check-only]")
	fmt.Fprintln(w, "  proxyctl select <group> <proxy>")
	fmt.Fprintln(w, "  proxyctl profile list [--json]")
	fmt.Fprintln(w, "  proxyctl profile traffic <id> [--json]")
	fmt.Fprintln(w, "  proxyctl profile activate <id>")
	fmt.Fprintln(w, "  proxyctl profile delete <id>")
}

func runCoreInstall(stdout io.Writer, stderr io.Writer) int {
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "Installing Mihomo core...")
	installedPath, err := core.InstallMihomo(runtimePaths.Mihomo)
	if err != nil {
		fmt.Fprintf(stderr, "install mihomo: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Mihomo installed: %s\n", installedPath)
	return 0
}

func runCoreStart(stdout io.Writer, stderr io.Writer) int {
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}

	if err := os.MkdirAll(runtimePaths.Logs, 0o755); err != nil {
		fmt.Fprintf(stderr, "create logs directory: %v\n", err)
		return 1
	}
	if tun.Active(runtimePaths) {
		fmt.Fprintln(stdout, "TUN core is already running")
		return 0
	}

	pid, err := core.EnsureProxyReady(runtimePaths.Mihomo, runtimePaths.ConfigYAML, runtimePaths.MihomoLog)
	if err != nil {
		fmt.Fprintf(stderr, "start mihomo: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Mihomo started (pid: %d)\n", pid)
	return 0
}

func runCoreStop(stdout io.Writer, stderr io.Writer) int {
	if p, err := paths.Default(); err == nil {
		managed, err := tun.Stop(p)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if managed {
			fmt.Fprintln(stdout, "TUN core stopped")
			return 0
		}
	}
	running, pid, err := core.Status()
	if err != nil {
		fmt.Fprintf(stderr, "check status: %v\n", err)
		return 1
	}
	if !running {
		fmt.Fprintln(stdout, "Mihomo is not running")
		return 0
	}

	p, err := paths.Default()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := core.CheckOwner(pid, p.Mihomo); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := core.StopGraceful(pid, 3*time.Second); err != nil {
		fmt.Fprintf(stderr, "stop mihomo: %v\n", err)
		return 1
	}
	if err := core.WaitStopped(pid); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "Mihomo stopped (pid: %d)\n", pid)
	return 0
}

func runCoreRestart(stdout io.Writer, stderr io.Writer) int {
	p, err := paths.Default()
	wasTUN := err == nil && tun.Active(p)
	if wasTUN {
		if err := tun.Restart(p, os.Getppid()); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if exit := runCoreStop(stdout, stderr); exit != 0 {
		return exit
	}
	return runCoreStart(stdout, stderr)
}

func runCoreStatus(stdout io.Writer, stderr io.Writer) int {
	return runCoreStatusJSON(stdout, stderr, false)
}

func runCoreStatusJSON(stdout io.Writer, stderr io.Writer, jsonOutput bool) int {
	running, pid, err := core.Status()
	if err != nil {
		fmt.Fprintf(stderr, "check status: %v\n", err)
		return 1
	}
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(struct {
			Running bool `json:"running"`
			PID     int  `json:"pid"`
		}{Running: running, PID: pid}); err != nil {
			fmt.Fprintf(stderr, "encode core status: %v\n", err)
			return 1
		}
		return 0
	}
	if running {
		fmt.Fprintf(stdout, "Mihomo is running (pid: %d)\n", pid)
	} else {
		fmt.Fprintln(stdout, "Mihomo is not running")
	}
	return 0
}

// Tests replace the direct TCP probes as well as system-command side effects.
var (
	systemProxyPortOpen = guard.PortOpen
	systemProxyWaitPort = guard.WaitForPort
)

// systemProxyEnableOps keeps enable ordering testable without OS mutations.
type systemProxyEnableOps struct {
	portOpen     func(int, time.Duration) bool
	waitPort     func(int, time.Duration, time.Duration) bool
	tunActive    func(paths.RuntimePaths) bool
	startCore    func(paths.RuntimePaths) error
	startWatch   func(string) error
	enable       func(int) error
	disableOwned func(int) error
}

func enableSystemProxy(p paths.RuntimePaths, port int, ops systemProxyEnableOps) error {
	if !ops.portOpen(port, 800*time.Millisecond) {
		if ops.tunActive(p) {
			return fmt.Errorf("TUN is active but local proxy port %d is not ready", port)
		}
		if err := ops.startCore(p); err != nil {
			return fmt.Errorf("core cannot be started: %w", err)
		}
		if !ops.waitPort(port, 5*time.Second, 800*time.Millisecond) {
			return fmt.Errorf("local proxy port %d is not ready", port)
		}
	}
	// The guard must answer its own health handshake before any system setting
	// changes. It does not need the lifecycle lock to answer that handshake.
	if err := ops.startWatch(p.Base); err != nil {
		return fmt.Errorf("watchdog is not ready: %w", err)
	}
	if err := ops.enable(port); err != nil {
		// Enable can fail after changing only some services. Remove just our
		// entries and keep the watcher available if cleanup also fails.
		return errors.Join(err, ops.disableOwned(port))
	}
	return nil
}

func runSystemProxyOn(stdout io.Writer, stderr io.Writer) int {
	p, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}
	const port = 7890
	err = enableSystemProxy(p, port, systemProxyEnableOps{
		portOpen: systemProxyPortOpen, waitPort: systemProxyWaitPort,
		tunActive: tun.Active,
		startCore: func(p paths.RuntimePaths) error {
			_, err := core.EnsureProxyReady(p.Mihomo, p.ConfigYAML, p.MihomoLog)
			return err
		},
		startWatch: spawnGuardWatch, enable: sysproxy.Enable,
		disableOwned: sysproxy.DisableOwned,
	})
	if err != nil {
		fmt.Fprintf(stderr, "refusing to enable system proxy: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "System proxy enabled (127.0.0.1:%d)\n", port)
	return 0
}

func runGuard(stdout io.Writer, stderr io.Writer, jsonOutput bool) int {
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}

	result, err := guard.Reconcile(guard.Options{Paths: runtimePaths})
	if err != nil {
		// Still emit what we observed: the caller may be a log the user reads
		// after the fact, where the failure reason matters more than the fix.
		if jsonOutput {
			_ = json.NewEncoder(stdout).Encode(result)
		}
		fmt.Fprintf(stderr, "guard: %v\n", err)
		return 1
	}

	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintf(stderr, "encode guard result: %v\n", err)
			return 1
		}
		return 0
	}

	if result.Action == guard.ActionNoop {
		fmt.Fprintln(stdout, "No ProxyCat system proxy to guard")
		return 0
	}
	if result.Action == guard.ActionHealthy {
		// Keep the healthy case quiet unless explicitly requested as JSON.
		return 0
	}
	fmt.Fprintf(stdout, "%s (port %d, failures %d)\n", result.Action, result.Port, result.Failures)
	if result.Detail != "" {
		fmt.Fprintf(stdout, "  detail: %s\n", result.Detail)
	}
	return 0
}

func runSystemProxyOff(stdout io.Writer, stderr io.Writer) int {
	if err := sysproxy.Disable(); err != nil {
		fmt.Fprintf(stderr, "disable system proxy: %v\n", err)
		return 1
	}
	// The watchdog exists only to defend a switch that is turned on. Once the
	// switch is off it has nothing to defend, and leaving it running would keep a
	// process alive forever for no reason.
	if p, err := paths.Default(); err == nil {
		stopGuardWatch(p.Base)
	}
	fmt.Fprintln(stdout, "System proxy disabled")
	return 0
}

func runSystemProxyStatus(stdout io.Writer, stderr io.Writer) int {
	return runSystemProxyStatusJSON(stdout, stderr, false)
}

func runSystemProxyStatusJSON(stdout io.Writer, stderr io.Writer, jsonOutput bool) int {
	status, err := sysproxy.GetStatus()
	if err != nil {
		fmt.Fprintf(stderr, "get system proxy status: %v\n", err)
		return 1
	}

	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(status); err != nil {
			fmt.Fprintf(stderr, "encode system proxy status: %v\n", err)
			return 1
		}
		return 0
	}

	if status.HTTPEnabled || status.HTTPSEnabled || status.SOCKSEnabled {
		fmt.Fprintln(stdout, "System proxy: ON")
		if status.HTTPEnabled {
			fmt.Fprintf(stdout, "  HTTP: %s:%d\n", status.HTTPHost, status.HTTPPort)
		}
		if status.HTTPSEnabled {
			fmt.Fprintf(stdout, "  HTTPS: %s:%d\n", status.HTTPSHost, status.HTTPSPort)
		}
		if status.SOCKSEnabled {
			fmt.Fprintf(stdout, "  SOCKS: %s:%d\n", status.SOCKSHost, status.SOCKSPort)
		}
	} else {
		fmt.Fprintln(stdout, "System proxy: OFF")
	}
	return 0
}

func runModeStatusJSON(stdout io.Writer, stderr io.Writer, jsonOutput bool) int {
	client := runtimeController()
	cfg, err := client.GetConfig()
	if err != nil {
		fmt.Fprintf(stderr, "get mode: %v\n", err)
		return 1
	}

	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(map[string]interface{}{"mode": cfg.Mode, "tunEnabled": tun.Ready(cfg), "tunDevice": cfg.Tun["device"]}); err != nil {
			fmt.Fprintf(stderr, "encode mode: %v\n", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(stdout, "Mode: %s\n", cfg.Mode)
	return 0
}

func runModeSet(mode string, stdout io.Writer, stderr io.Writer) int {
	switch mode {
	case "rule", "global", "direct":
	default:
		fmt.Fprintf(stderr, "mode must be one of: rule, global, direct\n")
		return 2
	}

	client := runtimeController()
	if err := client.SetMode(mode); err != nil {
		fmt.Fprintf(stderr, "set mode: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Mode set to %s\n", mode)
	return 0
}

func runGroupsList(stdout io.Writer, stderr io.Writer) int {
	return runGroupsJSON(stdout, stderr, false)
}

func runGroups(stdout io.Writer, stderr io.Writer) int {
	return runGroupsList(stdout, stderr)
}

func runGroupsJSON(stdout io.Writer, stderr io.Writer, jsonOutput bool) int {
	client := runtimeController()
	groups, err := client.GetProxyGroups()
	if err != nil {
		fmt.Fprintf(stderr, "get proxy groups: %v\n", err)
		return 1
	}

	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(groups); err != nil {
			fmt.Fprintf(stderr, "encode proxy groups: %v\n", err)
			return 1
		}
		return 0
	}

	if len(groups) == 0 {
		fmt.Fprintln(stdout, "No proxy groups")
		return 0
	}

	for name, group := range groups {
		fmt.Fprintf(stdout, "%s (%s): current=%s\n", name, group.Type, group.Now)
		for i, proxy := range group.All {
			fmt.Fprintf(stdout, "  %d. %s\n", i+1, proxy)
		}
	}
	return 0
}

type proxyDelayResult struct {
	Delay int    `json:"delay"`
	Error string `json:"error,omitempty"`
}

func runGroupsDelay(stdout io.Writer, stderr io.Writer, jsonOutput bool) int {
	client := runtimeController()
	groups, err := client.GetProxyGroups()
	if err != nil {
		fmt.Fprintf(stderr, "get proxy groups: %v\n", err)
		return 1
	}

	groupNames := make(map[string]bool, len(groups))
	for name := range groups {
		groupNames[name] = true
	}

	results := make(map[string]proxyDelayResult)
	const testURL = "http://www.gstatic.com/generate_204"
	const timeoutMS = 5000
	for _, group := range groups {
		for _, proxy := range group.All {
			if isDelaySkippedProxy(proxy, groupNames) {
				continue
			}
			if _, exists := results[proxy]; exists {
				continue
			}
			delay, err := client.TestProxyDelay(proxy, testURL, timeoutMS)
			if err != nil {
				results[proxy] = proxyDelayResult{Error: err.Error()}
				continue
			}
			results[proxy] = proxyDelayResult{Delay: delay}
		}
	}

	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(results); err != nil {
			fmt.Fprintf(stderr, "encode proxy delays: %v\n", err)
			return 1
		}
		return 0
	}

	if len(results) == 0 {
		fmt.Fprintln(stdout, "No proxies to test")
		return 0
	}
	for name, result := range results {
		if result.Error != "" {
			fmt.Fprintf(stdout, "%s: timeout\n", name)
			continue
		}
		fmt.Fprintf(stdout, "%s: %dms\n", name, result.Delay)
	}
	return 0
}

func isDelaySkippedProxy(name string, groupNames map[string]bool) bool {
	switch name {
	case "DIRECT", "REJECT", "":
		return true
	}
	return groupNames[name]
}

func runProxyDelay(name string, stdout io.Writer, stderr io.Writer, jsonOutput bool) int {
	client := runtimeController()
	const testURL = "http://www.gstatic.com/generate_204"
	const timeoutMS = 5000
	delay, err := client.TestProxyDelay(name, testURL, timeoutMS)
	if err != nil {
		result := proxyDelayResult{Error: err.Error()}
		if jsonOutput {
			if encErr := json.NewEncoder(stdout).Encode(result); encErr != nil {
				fmt.Fprintf(stderr, "encode proxy delay: %v\n", encErr)
				return 1
			}
			// Report the failure in-band rather than via exit code so the Swift UI sees the error string.
			return 0
		}
		fmt.Fprintf(stderr, "test delay for %s: %v\n", name, err)
		return 1
	}
	result := proxyDelayResult{Delay: delay}
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintf(stderr, "encode proxy delay: %v\n", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(stdout, "%s: %dms\n", name, delay)
	return 0
}

func runGroupsSelect(group string, proxy string, stdout io.Writer, stderr io.Writer) int {
	return runSelect(group, proxy, stdout, stderr)
}

func runSelect(group string, proxy string, stdout io.Writer, stderr io.Writer) int {
	client := runtimeController()
	if err := client.SelectProxy(group, proxy); err != nil {
		fmt.Fprintf(stderr, "select proxy: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Selected %s for group %s\n", proxy, group)
	return 0
}

func runTest(stdout io.Writer, stderr io.Writer) int {
	return runTestJSON(stdout, stderr, false)
}

func runTestJSON(stdout io.Writer, stderr io.Writer, jsonOutput bool) int {
	// Test connectivity through the local proxy at port 7890
	result, err := controller.TestConnection("http://127.0.0.1:7890")
	if err != nil {
		fmt.Fprintf(stderr, "connection test failed: %v\n", err)
		return 1
	}
	return writeTestResult(result, stdout, stderr, jsonOutput)
}

func writeTestResult(result *controller.TestResult, stdout io.Writer, stderr io.Writer, jsonOutput bool) int {
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintf(stderr, "encode test result: %v\n", err)
			return 1
		}
	} else {
		if result.Error != "" {
			fmt.Fprintf(stderr, "connection test error: %s\n", result.Error)
		}

		// Print results
		if result.GoogleOK {
			fmt.Fprintln(stdout, "Google: OK")
		} else {
			fmt.Fprintln(stdout, "Google: FAIL")
		}

		if result.GitHubOK {
			fmt.Fprintln(stdout, "GitHub: OK")
		} else {
			fmt.Fprintln(stdout, "GitHub: FAIL")
		}
	}

	if !result.GoogleOK || !result.GitHubOK {
		// JSON callers need the structured result to explain which endpoint
		// failed. Keep the process successful so the UI can decode it instead
		// of replacing it with a generic "Command failed" message.
		if jsonOutput {
			return 0
		}
		return 1
	}
	return 0
}

func saveProfileSubscription(url string) (string, error) {
	runtimePaths, err := paths.Default()
	if err != nil {
		return "", fmt.Errorf("resolve paths: %w", err)
	}
	if err := os.MkdirAll(runtimePaths.ProfilesDir, 0o755); err != nil {
		return "", fmt.Errorf("create profiles dir: %w", err)
	}

	profiles, err := profile.LoadAll(runtimePaths.ProfilesDir)
	if err != nil {
		return "", fmt.Errorf("load profiles: %w", err)
	}

	existing := profile.FindByURL(profiles, url)
	if existing != nil {
		existing.UpdatedAt = time.Now()
		if err := profile.SaveAll(runtimePaths.ProfilesDir, profiles); err != nil {
			return "", err
		}
		return existing.ID, nil
	}

	id := profile.NextID(runtimePaths.ProfilesDir)
	p := profile.Profile{
		ID:        id,
		Name:      uniqueProfileName(defaultProfileName(url, len(profiles)+1), profiles),
		URL:       url,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if _, err := profile.EnsureProfileDir(runtimePaths.ProfilesDir, id); err != nil {
		return "", err
	}
	profiles = append(profiles, p)
	if err := profile.SaveAll(runtimePaths.ProfilesDir, profiles); err != nil {
		return "", err
	}
	return id, nil
}

func defaultProfileName(rawURL string, fallbackIndex int) string {
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	return fmt.Sprintf("Subscription %d", fallbackIndex)
}

func uniqueProfileName(base string, profiles []profile.Profile) string {
	used := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		used[p.Name] = true
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s %d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func runSubscriptionAdd(url string, stdout io.Writer, stderr io.Writer) int {
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(runtimePaths.SubscriptionsJSON), 0o755); err != nil {
		fmt.Fprintf(stderr, "create config directory: %v\n", err)
		return 1
	}

	records, err := subscription.Load(runtimePaths.SubscriptionsJSON)
	if err != nil {
		fmt.Fprintf(stderr, "load subscriptions: %v\n", err)
		return 1
	}

	// Check for duplicate URL
	for _, r := range records {
		if r.URL == url {
			fmt.Fprintf(stderr, "subscription already exists\n")
			return 1
		}
	}

	newRecord := subscription.Record{
		URL:        url,
		Name:       "Subscription",
		LastUpdate: time.Now(),
	}
	records = append(records, newRecord)

	if err := subscription.Save(runtimePaths.SubscriptionsJSON, records); err != nil {
		fmt.Fprintf(stderr, "save subscriptions: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Added subscription: %s\n", redact.URL(url))
	return 0
}

func runSubscriptionProbe(rawURL string, stdout io.Writer, stderr io.Writer, jsonOutput bool) int {
	result := subscription.Probe(&http.Client{}, rawURL)
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintf(stderr, "encode subscription probe: %v\n", err)
			return 1
		}
		if result.Selected == nil {
			return 1
		}
		return 0
	}

	if result.Selected != nil {
		fmt.Fprintf(stdout, "订阅可用：%s via %s\n", result.Selected.Format, result.Selected.UserAgent)
		if result.Selected.ProxyCount > 0 {
			fmt.Fprintf(stdout, "节点：%d，策略组：%d，规则：%d\n", result.Selected.ProxyCount, result.Selected.GroupCount, result.Selected.RuleCount)
		} else if result.Selected.NodeCount > 0 {
			fmt.Fprintf(stdout, "节点：%d\n", result.Selected.NodeCount)
		}
		return 0
	}

	fmt.Fprintln(stderr, result.Message)
	if result.SuggestedFix != "" {
		fmt.Fprintf(stderr, "建议：%s\n", result.SuggestedFix)
	}
	return 1
}

func runSubscriptionList(stdout io.Writer, stderr io.Writer) int {
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}

	records, err := subscription.Load(runtimePaths.SubscriptionsJSON)
	if err != nil {
		fmt.Fprintf(stderr, "load subscriptions: %v\n", err)
		return 1
	}

	if len(records) == 0 {
		fmt.Fprintln(stdout, "No subscriptions")
		return 0
	}

	for i, r := range records {
		fmt.Fprintf(stdout, "%d. %s (%s)\n", i+1, r.Name, redact.URL(r.URL))
	}
	return 0
}

func runSubscriptionUpdate(stdout io.Writer, stderr io.Writer) int {
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}

	// Try profile-based update first
	profiles, err := profile.LoadAll(runtimePaths.ProfilesDir)
	if err != nil {
		fmt.Fprintf(stderr, "load profiles: %v\n", err)
		return 1
	}

	if len(profiles) > 0 {
		client := &http.Client{}
		for i, p := range profiles {
			probe := subscription.Probe(client, p.URL)
			if probe.Selected == nil {
				fmt.Fprintf(stdout, "Profile %s (%s): invalid - %s\n", p.ID, p.Name, probe.Message)
				if probe.SuggestedFix != "" {
					fmt.Fprintf(stdout, "  建议：%s\n", probe.SuggestedFix)
				}
				continue
			}
			profiles[i].UpdatedAt = time.Now()
			format := config.Format(probe.Selected.Format)
			content := probe.SelectedContent
			var configYAML string
			switch format {
			case config.FormatClashYAML:
				configYAML, err = config.NormalizeClashYAML(content)
			case config.FormatBase64List, config.FormatPlainList:
				configYAML, err = config.ConvertURIToYAML(content, format)
			default:
				err = fmt.Errorf("unknown format: %s", format)
			}
			if err != nil {
				fmt.Fprintf(stdout, "Profile %s: generate failed: %v\n", p.ID, err)
				continue
			}
			pDir, _ := profile.EnsureProfileDir(runtimePaths.ProfilesDir, p.ID)
			if err := os.WriteFile(filepath.Join(pDir, "config.yaml"), []byte(configYAML), 0644); err != nil {
				fmt.Fprintf(stdout, "Profile %s: write failed: %v\n", p.ID, err)
				continue
			}
			if _, err := profile.SaveTraffic(runtimePaths.ProfilesDir, p, probe.Traffic, time.Now()); err != nil {
				fmt.Fprintln(stderr, "流量信息未能缓存，可稍后刷新")
			}
			fmt.Fprintf(stdout, "Profile %s (%s): updated\n", p.ID, p.Name)
		}
		if err := profile.SaveAll(runtimePaths.ProfilesDir, profiles); err != nil {
			fmt.Fprintf(stderr, "save profiles: %v\n", err)
			return 1
		}
		return 0
	}

	// Legacy fallback: subscriptions.json
	records, err := subscription.Load(runtimePaths.SubscriptionsJSON)
	if err != nil {
		fmt.Fprintf(stderr, "load subscriptions: %v\n", err)
		return 1
	}

	if len(records) == 0 {
		fmt.Fprintln(stderr, "No subscriptions to update")
		return 1
	}

	client := &http.Client{}
	for i, r := range records {
		probe := subscription.Probe(client, r.URL)
		if probe.Selected == nil {
			fmt.Fprintf(stdout, "Subscription %d: invalid - %s\n", i+1, probe.Message)
			if probe.SuggestedFix != "" {
				fmt.Fprintf(stdout, "  建议：%s\n", probe.SuggestedFix)
			}
			continue
		}

		fmt.Fprintf(stdout, "Subscription %d: selected %s via %s\n", i+1, probe.Selected.Format, probe.Selected.UserAgent)
		if probe.Selected.ProxyCount > 0 {
			fmt.Fprintf(stdout, "  Valid: %d proxies, %d groups, %d rules\n", probe.Selected.ProxyCount, probe.Selected.GroupCount, probe.Selected.RuleCount)
		} else {
			fmt.Fprintf(stdout, "  Valid: %d nodes (requires config generation)\n", probe.Selected.NodeCount)
		}
		records[i].LastUpdate = time.Now()
	}

	if err := subscription.Save(runtimePaths.SubscriptionsJSON, records); err != nil {
		fmt.Fprintf(stderr, "save subscriptions: %v\n", err)
		return 1
	}

	return 0
}

func runConfigGenerate(stdout io.Writer, stderr io.Writer) int {
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}

	records, err := subscription.Load(runtimePaths.SubscriptionsJSON)
	if err != nil {
		fmt.Fprintf(stderr, "load subscriptions: %v\n", err)
		return 1
	}

	if len(records) == 0 {
		fmt.Fprintln(stderr, "No subscriptions found. Add a subscription first.")
		return 1
	}

	// For now, use the first subscription
	r := records[0]
	fmt.Fprintf(stdout, "Generating config from subscription: %s\n", redact.URL(r.URL))

	client := &http.Client{}
	probe := subscription.Probe(client, r.URL)
	if probe.Selected == nil {
		fmt.Fprintf(stderr, "%s\n", probe.Message)
		if probe.SuggestedFix != "" {
			fmt.Fprintf(stderr, "建议：%s\n", probe.SuggestedFix)
		}
		return 1
	}
	content := probe.SelectedContent
	fmt.Fprintf(stdout, "Selected subscription format: %s via %s\n", probe.Selected.Format, probe.Selected.UserAgent)

	format := config.Format(probe.Selected.Format)

	var configYAML string
	switch format {
	case config.FormatClashYAML:
		configYAML, err = config.NormalizeClashYAML(content)
		if err != nil {
			fmt.Fprintf(stderr, "normalize YAML: %v\n", err)
			return 1
		}
	case config.FormatBase64List, config.FormatPlainList:
		// Convert URI list to YAML
		configYAML, err = config.ConvertURIToYAML(content, format)
		if err != nil {
			fmt.Fprintf(stderr, "convert to YAML: %v\n", err)
			return 1
		}
	default:
		fmt.Fprintf(stderr, "Unknown subscription format\n")
		return 1
	}

	if err := os.MkdirAll(filepath.Dir(runtimePaths.ConfigYAML), 0o755); err != nil {
		fmt.Fprintf(stderr, "create config directory: %v\n", err)
		return 1
	}

	// Write config file
	if err := os.WriteFile(runtimePaths.ConfigYAML, []byte(configYAML), 0644); err != nil {
		fmt.Fprintf(stderr, "write config: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Config generated: %s\n", runtimePaths.ConfigYAML)
	return 0
}

func runConfigGenerateForProfile(profileID string, stdout io.Writer, stderr io.Writer) int {
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}

	profiles, err := profile.LoadAll(runtimePaths.ProfilesDir)
	if err != nil {
		fmt.Fprintf(stderr, "load profiles: %v\n", err)
		return 1
	}

	var target *profile.Profile
	for i := range profiles {
		if profiles[i].ID == profileID {
			target = &profiles[i]
			break
		}
	}
	if target == nil {
		fmt.Fprintf(stderr, "profile %s not found\n", profileID)
		return 1
	}

	fmt.Fprintf(stdout, "Generating config from subscription: %s\n", redact.URL(target.URL))

	client := &http.Client{}
	probe := subscription.Probe(client, target.URL)
	if probe.Selected == nil {
		fmt.Fprintf(stderr, "%s\n", probe.Message)
		if probe.SuggestedFix != "" {
			fmt.Fprintf(stderr, "建议：%s\n", probe.SuggestedFix)
		}
		return 1
	}
	content := probe.SelectedContent
	fmt.Fprintf(stdout, "Selected subscription format: %s via %s\n", probe.Selected.Format, probe.Selected.UserAgent)

	format := config.Format(probe.Selected.Format)
	var configYAML string
	switch format {
	case config.FormatClashYAML:
		configYAML, err = config.NormalizeClashYAML(content)
		if err != nil {
			fmt.Fprintf(stderr, "normalize YAML: %v\n", err)
			return 1
		}
	case config.FormatBase64List, config.FormatPlainList:
		configYAML, err = config.ConvertURIToYAML(content, format)
		if err != nil {
			fmt.Fprintf(stderr, "convert to YAML: %v\n", err)
			return 1
		}
	default:
		fmt.Fprintf(stderr, "Unknown subscription format\n")
		return 1
	}

	profileConfigPath := profile.ProfileConfigPath(runtimePaths.ProfilesDir, profileID)
	if err := os.MkdirAll(filepath.Dir(profileConfigPath), 0o755); err != nil {
		fmt.Fprintf(stderr, "create profile dir: %v\n", err)
		return 1
	}
	if err := os.WriteFile(profileConfigPath, []byte(configYAML), 0644); err != nil {
		fmt.Fprintf(stderr, "write profile config: %v\n", err)
		return 1
	}
	if _, err := profile.SaveTraffic(runtimePaths.ProfilesDir, *target, probe.Traffic, time.Now()); err != nil {
		fmt.Fprintln(stderr, "流量信息未能缓存，可稍后刷新")
	}

	fmt.Fprintf(stdout, "Config generated: %s\n", profileConfigPath)
	return 0
}

func runProfileTraffic(id string, stdout, stderr io.Writer) int {
	p, err := paths.Default()
	if err != nil {
		fmt.Fprintln(stderr, "无法读取配置路径")
		return 1
	}
	profiles, err := profile.LoadAll(p.ProfilesDir)
	if err != nil {
		fmt.Fprintln(stderr, "无法读取订阅配置")
		return 1
	}
	for _, entry := range profiles {
		if entry.ID != id {
			continue
		}
		client := updateHTTPClient()
		client.Timeout = 12 * time.Second
		defer client.CloseIdleConnections()
		traffic, err := subscription.FetchTraffic(client, entry.URL)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		result, err := profile.SaveTraffic(p.ProfilesDir, entry, traffic, time.Now())
		if err != nil {
			fmt.Fprintln(stderr, "暂时无法保存流量信息，请稍后重试")
			return 1
		}
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			return 1
		}
		return 0
	}
	fmt.Fprintln(stderr, "订阅配置已不存在，请重新选择")
	return 1
}

func runProfileList(stdout io.Writer, stderr io.Writer, jsonOutput bool) int {
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}
	profiles, err := profile.LoadAll(runtimePaths.ProfilesDir)
	if err != nil {
		fmt.Fprintf(stderr, "load profiles: %v\n", err)
		return 1
	}
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(redactedProfiles(profiles)); err != nil {
			fmt.Fprintf(stderr, "encode profiles: %v\n", err)
			return 1
		}
		return 0
	}
	if len(profiles) == 0 {
		fmt.Fprintln(stdout, "No profiles")
		return 0
	}
	for _, p := range profiles {
		fmt.Fprintf(stdout, "%s  %s  (%s)\n", p.ID, p.Name, redact.URL(p.URL))
	}
	return 0
}

func redactedProfiles(profiles []profile.Profile) []profile.Profile {
	redacted := make([]profile.Profile, len(profiles))
	copy(redacted, profiles)
	for i := range redacted {
		redacted[i].URL = redact.URL(redacted[i].URL)
	}
	return redacted
}

func runProfileActivate(profileID string, stdout io.Writer, stderr io.Writer) int {
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}
	if err := profile.Activate(runtimePaths.ProfilesDir, profileID, runtimePaths.ConfigYAML); err != nil {
		fmt.Fprintf(stderr, "activate profile: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Activated profile %s\n", profileID)
	return 0
}

func runProfileDelete(profileID string, stdout io.Writer, stderr io.Writer) int {
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}
	if err := profile.Delete(runtimePaths.ProfilesDir, profileID); err != nil {
		fmt.Fprintf(stderr, "delete profile: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Deleted profile %s\n", profileID)
	return 0
}
