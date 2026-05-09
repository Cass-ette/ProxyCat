package paths

import (
	"os"
	"path/filepath"
	"runtime"
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
	Backups           string `json:"backups"`
	Logs              string `json:"logs"`
	ProxyCatLog       string `json:"proxycatLog"`
	MihomoLog         string `json:"mihomoLog"`
	Reports           string `json:"reports"`
	DiagnoseLatest    string `json:"diagnoseLatest"`
}

func Default() (RuntimePaths, error) {
	base := PlatformBaseDir(runtime.GOOS)
	return ForHome(base), nil
}

func PlatformBaseDir(goos string) string {
	if goos == "windows" {
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			home, _ := os.UserHomeDir()
			local = filepath.Join(home, "AppData", "Local")
		}
		return local
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support")
}

func ForHome(base string) RuntimePaths {
	root := filepath.Join(base, appName)
	return fromBase(root)
}

func fromBase(base string) RuntimePaths {
	bin := filepath.Join(base, "bin")
	config := filepath.Join(base, "config")
	logs := filepath.Join(base, "logs")
	reports := filepath.Join(base, "reports")

	proxyctlName := "proxyctl"
	mihomoName := "mihomo"
	if runtime.GOOS == "windows" {
		proxyctlName = "proxyctl.exe"
		mihomoName = "mihomo.exe"
	}

	return RuntimePaths{
		Base:              base,
		Bin:               bin,
		Proxyctl:          filepath.Join(bin, proxyctlName),
		Mihomo:            filepath.Join(bin, mihomoName),
		Config:            config,
		ConfigYAML:        filepath.Join(config, "config.yaml"),
		SubscriptionsJSON: filepath.Join(config, "subscriptions.json"),
		Backups:           filepath.Join(config, "backups"),
		Logs:              logs,
		ProxyCatLog:       filepath.Join(logs, "proxycat.log"),
		MihomoLog:         filepath.Join(logs, "mihomo.log"),
		Reports:           reports,
		DiagnoseLatest:    filepath.Join(reports, "diagnose-latest.json"),
	}
}
