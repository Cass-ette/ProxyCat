package paths

import (
	"os"
	"path/filepath"
)

const appName = "ProxyCat"

type RuntimePaths struct {
	Base              string `json:"base"`
	Bin               string `json:"bin"`
	Proxyctl          string `json:"proxyctl"`
	Mihomo            string `json:"mihomo"`
	Config            string `json:"config"`
	ConfigYAML        string `json:"configYaml"`
	SubscriptionsJSON string `json:"subscriptionsJson"`
	ProfilesDir       string `json:"profilesDir"`
	Backups           string `json:"backups"`
	Logs              string `json:"logs"`
	ProxyCatLog       string `json:"proxycatLog"`
	MihomoLog         string `json:"mihomoLog"`
	Reports           string `json:"reports"`
	DiagnoseLatest    string `json:"diagnoseLatest"`
}

func Default() (RuntimePaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return RuntimePaths{}, err
	}
	return ForHome(home), nil
}

func ForHome(home string) RuntimePaths {
	base := filepath.Join(home, "Library", "Application Support", appName)
	bin := filepath.Join(base, "bin")
	config := filepath.Join(base, "config")
	profiles := filepath.Join(config, "profiles")
	logs := filepath.Join(base, "logs")
	reports := filepath.Join(base, "reports")

	return RuntimePaths{
		Base:              base,
		Bin:               bin,
		Proxyctl:          filepath.Join(bin, "proxyctl"),
		Mihomo:            filepath.Join(bin, "mihomo"),
		Config:            config,
		ConfigYAML:        filepath.Join(config, "config.yaml"),
		SubscriptionsJSON: filepath.Join(config, "subscriptions.json"),
		Backups:           filepath.Join(config, "backups"),
		ProfilesDir:       profiles,
		Logs:              logs,
		ProxyCatLog:       filepath.Join(logs, "proxycat.log"),
		MihomoLog:         filepath.Join(logs, "mihomo.log"),
		Reports:           reports,
		DiagnoseLatest:    filepath.Join(reports, "diagnose-latest.json"),
	}
}
