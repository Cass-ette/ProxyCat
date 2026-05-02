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
	case "core":
		if len(args) < 2 {
			fmt.Fprintf(stderr, "core subcommand required: start, stop, restart, status\n")
			return 2
		}
		switch args[1] {
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
	fmt.Fprintln(w, "  proxyctl subscription add <url>")
	fmt.Fprintln(w, "  proxyctl subscription list")
	fmt.Fprintln(w, "  proxyctl subscription update")
	fmt.Fprintln(w, "  proxyctl core {start|stop|restart|status}")
	fmt.Fprintln(w, "  proxyctl system-proxy {on|off|status}")
	fmt.Fprintln(w, "  proxyctl groups [list]")
	fmt.Fprintln(w, "  proxyctl groups select <group> <proxy>")
	fmt.Fprintln(w, "  proxyctl test")
	fmt.Fprintln(w, "  proxyctl select <group> <proxy>")
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

		// Validate if it's a valid config
		result := config.Validate(content)
		if result.Valid {
			fmt.Fprintf(stdout, "  Valid: %d proxies, %d groups, %d rules\n", result.ProxyCount, result.GroupCount, result.RuleCount)
			// Only update LastUpdate on successful validation
			records[i].LastUpdate = time.Now()
		} else {
			fmt.Fprintf(stdout, "  Invalid: %s\n", result.Message)
			// Skip updating timestamp for failed validation
			continue
		}
	}

	if err := subscription.Save(runtimePaths.SubscriptionsJSON, records); err != nil {
		fmt.Fprintf(stderr, "save subscriptions: %v\n", err)
		return 1
	}

	return 0
}
