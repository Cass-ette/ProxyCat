package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/config"
	"github.com/Cass-ette/ProxyCat/helper/internal/controller"
	"github.com/Cass-ette/ProxyCat/helper/internal/core"
	"github.com/Cass-ette/ProxyCat/helper/internal/diagnose"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
	"github.com/Cass-ette/ProxyCat/helper/internal/profile"
	"github.com/Cass-ette/ProxyCat/helper/internal/redact"
	"github.com/Cass-ette/ProxyCat/helper/internal/selfupdate"
	"github.com/Cass-ette/ProxyCat/helper/internal/subscription"
	"github.com/Cass-ette/ProxyCat/helper/internal/sysproxy"
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

	switch args[0] {
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
			return runCoreStop(stdout, stderr)
		case "restart":
			return runCoreRestart(stdout, stderr)
		case "status":
			return runCoreStatusJSON(stdout, stderr, jsonOutput)
		default:
			fmt.Fprintf(stderr, "unknown core subcommand: %s\n", redact.String(args[1]))
			return 2
		}

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

	case "test":
		return runTestJSON(stdout, stderr, jsonOutput)

	case "self-update":
		return runSelfUpdate(stdout, stderr, jsonOutput, containsArg(args[1:], "--check-only"))

	case "update":
		return runSelfUpdate(stdout, stderr, jsonOutput, containsArg(args[1:], "--check-only"))

	case "select":
		if len(args) < 3 {
			fmt.Fprintf(stderr, "select requires group and proxy arguments\n")
			return 2
		}
		return runSelect(args[1], args[2], stdout, stderr)

	case "profile":
		if len(args) < 2 {
			fmt.Fprintf(stderr, "profile subcommand required: list, activate\n")
			return 2
		}
		switch args[1] {
		case "list":
			return runProfileList(stdout, stderr, jsonOutput)
		case "activate":
			if len(args) < 3 {
				fmt.Fprintf(stderr, "profile activate requires <id>\n")
				return 2
			}
			return runProfileActivate(args[2], stdout, stderr)
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

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func runSelfUpdate(stdout io.Writer, stderr io.Writer, jsonOutput bool, checkOnly bool) int {
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
		CurrentVersion: currentVersion,
		CheckOnly:      checkOnly,
		Client:         &http.Client{},
		Endpoint:       "https://api.github.com/repos/Cass-ette/ProxyCat/releases/latest",
		AppPath:        appPath,
		BackupDir:      filepath.Join(filepath.Dir(appPath), "ProxyCat-Backups"),
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
	client := controller.NewClient("")
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
	fmt.Fprintln(w, "  proxyctl system-proxy {on|off|status}")
	fmt.Fprintln(w, "  proxyctl mode {status|set <rule|global|direct>}")
	fmt.Fprintln(w, "  proxyctl groups [list]")
	fmt.Fprintln(w, "  proxyctl groups delay [--json]")
	fmt.Fprintln(w, "  proxyctl groups select <group> <proxy>")
	fmt.Fprintln(w, "  proxyctl test")
	fmt.Fprintln(w, "  proxyctl self-update [--json] [--check-only]")
	fmt.Fprintln(w, "  proxyctl update [--json] [--check-only]")
	fmt.Fprintln(w, "  proxyctl select <group> <proxy>")
	fmt.Fprintln(w, "  proxyctl profile list [--json]")
	fmt.Fprintln(w, "  proxyctl profile activate <id>")
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

	pid, err := core.Start(runtimePaths.Mihomo, runtimePaths.ConfigYAML, runtimePaths.MihomoLog)
	if err != nil {
		fmt.Fprintf(stderr, "start mihomo: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Mihomo started (pid: %d)\n", pid)
	return 0
}

func runCoreStop(stdout io.Writer, stderr io.Writer) int {
	running, pid, err := core.Status()
	if err != nil {
		fmt.Fprintf(stderr, "check status: %v\n", err)
		return 1
	}
	if !running {
		fmt.Fprintln(stdout, "Mihomo is not running")
		return 0
	}

	if err := core.Stop(pid); err != nil {
		fmt.Fprintf(stderr, "stop mihomo: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Mihomo stopped (pid: %d)\n", pid)
	return 0
}

func runCoreRestart(stdout io.Writer, stderr io.Writer) int {
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

func runSystemProxyOn(stdout io.Writer, stderr io.Writer) int {
	port := 7890
	if err := sysproxy.Enable(port); err != nil {
		fmt.Fprintf(stderr, "enable system proxy: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "System proxy enabled (127.0.0.1:%d)\n", port)
	return 0
}

func runSystemProxyOff(stdout io.Writer, stderr io.Writer) int {
	if err := sysproxy.Disable(); err != nil {
		fmt.Fprintf(stderr, "disable system proxy: %v\n", err)
		return 1
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
	client := controller.NewClient("")
	cfg, err := client.GetConfig()
	if err != nil {
		fmt.Fprintf(stderr, "get mode: %v\n", err)
		return 1
	}

	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(map[string]string{"mode": cfg.Mode}); err != nil {
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

	client := controller.NewClient("")
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
	client := controller.NewClient("")
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
	client := controller.NewClient("")
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

func runGroupsSelect(group string, proxy string, stdout io.Writer, stderr io.Writer) int {
	return runSelect(group, proxy, stdout, stderr)
}

func runSelect(group string, proxy string, stdout io.Writer, stderr io.Writer) int {
	client := controller.NewClient("")
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
		Name:      "Subscription",
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

	fmt.Fprintf(stdout, "Config generated: %s\n", profileConfigPath)
	return 0
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
		if err := json.NewEncoder(stdout).Encode(profiles); err != nil {
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
