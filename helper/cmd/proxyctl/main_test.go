package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnoseJSONCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"diagnose", "--json"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var decoded struct {
		App       string `json:"app"`
		Milestone string `json:"milestone"`
		Checks    []struct {
			Name         string `json:"name"`
			Status       string `json:"status"`
			Message      string `json:"message"`
			SuggestedFix string `json:"suggestedFix"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if decoded.App != "ProxyCat" || decoded.Milestone != "milestone-1" || len(decoded.Checks) != 5 {
		t.Fatalf("unexpected diagnose output: %+v", decoded)
	}
}

func TestDiagnoseHumanCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"diagnose"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "ProxyCat Diagnose") {
		t.Fatalf("human output missing heading: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "runtime-paths") {
		t.Fatalf("human output missing check name: %s", stdout.String())
	}
}

func TestUnknownCommandRedactsSecrets(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"https://user:secret@example.com/sub?token=abc123"}, stdout, stderr)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	got := stderr.String()
	if strings.Contains(got, "user") || strings.Contains(got, "secret") || strings.Contains(got, "abc123") {
		t.Fatalf("stderr leaked secret: %s", got)
	}
	if !strings.Contains(got, "unknown command") {
		t.Fatalf("stderr missing unknown command message: %s", got)
	}
}

func TestSubscriptionAddCommand(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"subscription", "add", "https://example.com/sub?token=test123"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}

	// Verify subscriptions.json was created
	configDir := filepath.Join(tempHome, "Library", "Application Support", "ProxyCat", "config")
	_, err := os.Stat(filepath.Join(configDir, "subscriptions.json"))
	if err != nil {
		t.Fatalf("subscriptions.json not created: %v", err)
	}
}

func TestSubscriptionListCommand(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// First add a subscription
	run([]string{"subscription", "add", "https://example.com/sub?token=test123"}, io.Discard, io.Discard)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"subscription", "list"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "example.com") {
		t.Fatalf("list output missing subscription: %s", output)
	}
}
