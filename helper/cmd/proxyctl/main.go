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
	"github.com/Cass-ette/ProxyCat/helper/internal/redact"
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
			fmt.Fprintf(stderr, "subscription subcommand required: add, list, update\n")
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

	case "select":
		if len(args) < 3 {
			fmt.Fprintf(stderr, "select requires group and proxy arguments\n")
			return 2
		}
		return runSelect(args[1], args[2], stdout, stderr)

	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", redact.String(args[0]))
		printHelp(stderr)
		return 2
	}
}

func runBootstrap(url string, stdout io.Writer, stderr io.Writer) int {
	fmt.Fprintf(stdout, "Saving subscription: %s\n", redact.URL(url))
	if err := saveSingleSubscription(url); err != nil {
		fmt.Fprintf(stderr, "save subscription: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "Generating config...")
	if exit := runConfigGenerate(stdout, stderr); exit != 0 {
		return exit
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
	fmt.Fprintln(w, "  proxyctl config generate")
	fmt.Fprintln(w, "  proxyctl core {install|start|stop|restart|status}")
	fmt.Fprintln(w, "  proxyctl system-proxy {on|off|status}")
	fmt.Fprintln(w, "  proxyctl mode {status|set <rule|global|direct>}")
	fmt.Fprintln(w, "  proxyctl groups [list]")
	fmt.Fprintln(w, "  proxyctl groups select <group> <proxy>")
	fmt.Fprintln(w, "  proxyctl test")
	fmt.Fprintln(w, "  proxyctl select <group> <proxy>")
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

func saveSingleSubscription(url string) error {
	runtimePaths, err := paths.Default()
	if err != nil {
		return fmt.Errorf("resolve paths: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(runtimePaths.SubscriptionsJSON), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	records, err := subscription.Load(runtimePaths.SubscriptionsJSON)
	if err != nil {
		return fmt.Errorf("load subscriptions: %w", err)
	}
	for i, r := range records {
		if r.URL == url {
			records[i].LastUpdate = time.Now()
			return subscription.Save(runtimePaths.SubscriptionsJSON, records)
		}
	}
	records = []subscription.Record{{URL: url, Name: "Subscription", LastUpdate: time.Now()}}
	return subscription.Save(runtimePaths.SubscriptionsJSON, records)
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
		content, err := subscription.Download(client, r.URL, "ProxyCat/1.0")
		if err != nil {
			fmt.Fprintf(stderr, "download subscription %d: %v\n", i+1, err)
			continue
		}

		// Detect format
		format, confidence := config.DetectFormat(content)
		fmt.Fprintf(stdout, "Subscription %d: detected %s (confidence: %s)\n", i+1, format, confidence)

		// Validate based on format
		switch format {
		case config.FormatClashYAML:
			result := config.Validate(content)
			if result.Valid {
				fmt.Fprintf(stdout, "  Valid: %d proxies, %d groups, %d rules\n", result.ProxyCount, result.GroupCount, result.RuleCount)
				records[i].LastUpdate = time.Now()
			} else {
				fmt.Fprintf(stdout, "  Invalid: %s\n", result.Message)
				continue
			}
		case config.FormatBase64List, config.FormatPlainList:
			// For URI list formats, count nodes instead of validating YAML structure
			// These formats need to be converted to YAML config separately
			nodeCount := config.CountNodes(content, format)
			if nodeCount > 0 {
				fmt.Fprintf(stdout, "  Valid: %d nodes (requires config generation)\n", nodeCount)
				records[i].LastUpdate = time.Now()
			} else {
				fmt.Fprintf(stdout, "  Invalid: no valid nodes found\n")
				continue
			}
		default:
			fmt.Fprintf(stdout, "  Unknown format, skipping validation\n")
			continue
		}
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
	content, err := subscription.Download(client, r.URL, "ProxyCat/1.0")
	if err != nil {
		fmt.Fprintf(stderr, "download subscription: %v\n", err)
		return 1
	}

	format, _ := config.DetectFormat(content)

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
