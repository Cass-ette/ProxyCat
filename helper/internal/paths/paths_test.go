package paths

import "testing"

func TestForHomeBuildsProxyCatRuntimePaths(t *testing.T) {
	p := ForHome("/Users/example")

	wantBase := "/Users/example/Library/Application Support/ProxyCat"
	if p.Base != wantBase {
		t.Fatalf("Base = %q, want %q", p.Base, wantBase)
	}
	if p.Bin != wantBase+"/bin" {
		t.Fatalf("Bin = %q", p.Bin)
	}
	if p.Proxyctl != wantBase+"/bin/proxyctl" {
		t.Fatalf("Proxyctl = %q", p.Proxyctl)
	}
	if p.Mihomo != wantBase+"/bin/mihomo" {
		t.Fatalf("Mihomo = %q", p.Mihomo)
	}
	if p.Config != wantBase+"/config" {
		t.Fatalf("Config = %q", p.Config)
	}
	if p.ConfigYAML != wantBase+"/config/config.yaml" {
		t.Fatalf("ConfigYAML = %q", p.ConfigYAML)
	}
	if p.SubscriptionsJSON != wantBase+"/config/subscriptions.json" {
		t.Fatalf("SubscriptionsJSON = %q", p.SubscriptionsJSON)
	}
	if p.Backups != wantBase+"/config/backups" {
		t.Fatalf("Backups = %q", p.Backups)
	}
	if p.Logs != wantBase+"/logs" {
		t.Fatalf("Logs = %q", p.Logs)
	}
	if p.ProxyCatLog != wantBase+"/logs/proxycat.log" {
		t.Fatalf("ProxyCatLog = %q", p.ProxyCatLog)
	}
	if p.MihomoLog != wantBase+"/logs/mihomo.log" {
		t.Fatalf("MihomoLog = %q", p.MihomoLog)
	}
	if p.Reports != wantBase+"/reports" {
		t.Fatalf("Reports = %q", p.Reports)
	}
	if p.DiagnoseLatest != wantBase+"/reports/diagnose-latest.json" {
		t.Fatalf("DiagnoseLatest = %q", p.DiagnoseLatest)
	}
}

func TestDefaultUsesUserHomeDir(t *testing.T) {
	t.Setenv("HOME", "/tmp/proxycat-home")

	p, err := Default()
	if err != nil {
		t.Fatalf("Default returned error: %v", err)
	}
	if p.Base != "/tmp/proxycat-home/Library/Application Support/ProxyCat" {
		t.Fatalf("Base = %q", p.Base)
	}
}
