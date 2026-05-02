package main

import (
	"bytes"
	"io"
	"testing"
)

func TestSubscriptionUpdateAllFailures(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	
	// Add a subscription with unreachable URL
	run([]string{"subscription", "add", "http://localhost:1/sub"}, io.Discard, io.Discard)
	
	// Run update
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"subscription", "update"}, stdout, stderr)
	
	t.Logf("exitCode: %d", exitCode)
	t.Logf("stdout: %s", stdout.String())
	t.Logf("stderr: %s", stderr.String())
	
	// Document current buggy behavior - returns 0 even when all downloads fail
	if exitCode == 0 && stderr.Len() > 0 {
		t.Logf("BUG CONFIRMED: exitCode is 0 but errors were written to stderr")
	}
}
