package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/Cass-ette/ProxyCat/helper/internal/diagnose"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
	"github.com/Cass-ette/ProxyCat/helper/internal/redact"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printHelp(stdout)
		return 0
	}

	switch args[0] {
	case "diagnose":
		return runDiagnose(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", redact.String(args[0]))
		printHelp(stderr)
		return 2
	}
}

func runDiagnose(args []string, stdout io.Writer, stderr io.Writer) int {
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			fmt.Fprintf(stderr, "unknown diagnose flag: %s\n", redact.String(arg))
			return 2
		}
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
}
