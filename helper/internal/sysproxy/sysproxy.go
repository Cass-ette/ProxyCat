// Package sysproxy provides macOS system proxy control.
package sysproxy

import (
	"bufio"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Status represents the current system proxy configuration.
type Status struct {
	HTTPEnabled  bool   `json:"httpEnabled"`
	HTTPHost     string `json:"httpHost"`
	HTTPPort     int    `json:"httpPort"`
	HTTPSEnabled bool   `json:"httpsEnabled"`
	HTTPSHost    string `json:"httpsHost"`
	HTTPSPort    int    `json:"httpsPort"`
	SOCKSEnabled bool   `json:"socksEnabled"`
	SOCKSHost    string `json:"socksHost"`
	SOCKSPort    int    `json:"socksPort"`
}

// Enable enables system proxy with the given port.
func Enable(port int) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("system proxy control is only supported on macOS")
	}

	services, err := getNetworkServices()
	if err != nil {
		return fmt.Errorf("get network services: %w", err)
	}

	host := "127.0.0.1"
	portStr := strconv.Itoa(port)

	for _, svc := range services {
		// Set HTTP proxy
		if err := exec.Command("networksetup", "-setwebproxy", svc, host, portStr).Run(); err != nil {
			return fmt.Errorf("set http proxy for %s: %w", svc, err)
		}
		// Set HTTPS proxy
		if err := exec.Command("networksetup", "-setsecurewebproxy", svc, host, portStr).Run(); err != nil {
			return fmt.Errorf("set https proxy for %s: %w", svc, err)
		}
		// Set SOCKS proxy
		if err := exec.Command("networksetup", "-setsocksfirewallproxy", svc, host, portStr).Run(); err != nil {
			return fmt.Errorf("set socks proxy for %s: %w", svc, err)
		}
	}

	return nil
}

// Disable disables system proxy.
func Disable() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("system proxy control is only supported on macOS")
	}

	services, err := getNetworkServices()
	if err != nil {
		return fmt.Errorf("get network services: %w", err)
	}

	for _, svc := range services {
		if err := exec.Command("networksetup", "-setwebproxystate", svc, "off").Run(); err != nil {
			return fmt.Errorf("disable http proxy for %s: %w", svc, err)
		}
		if err := exec.Command("networksetup", "-setsecurewebproxystate", svc, "off").Run(); err != nil {
			return fmt.Errorf("disable https proxy for %s: %w", svc, err)
		}
		if err := exec.Command("networksetup", "-setsocksfirewallproxystate", svc, "off").Run(); err != nil {
			return fmt.Errorf("disable socks proxy for %s: %w", svc, err)
		}
	}

	return nil
}

// GetStatus returns the current system proxy status.
func GetStatus() (*Status, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("system proxy control is only supported on macOS")
	}

	output, err := exec.Command("scutil", "--proxy").Output()
	if err != nil {
		return nil, fmt.Errorf("get proxy status: %w", err)
	}

	return parseScutilOutput(string(output))
}

// getNetworkServices returns a list of network service names.
func getNetworkServices() ([]string, error) {
	output, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return nil, fmt.Errorf("list network services: %w", err)
	}

	var services []string
	lines := strings.Split(string(output), "\n")
	for i, line := range lines {
		// Skip header line and explanation line
		if i == 0 || strings.HasPrefix(line, "An asterisk") {
			continue
		}
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "*") {
			services = append(services, line)
		}
	}
	return services, nil
}

// parseScutilOutput parses the output of scutil --proxy.
func parseScutilOutput(output string) (*Status, error) {
	if strings.TrimSpace(output) == "" {
		return nil, errors.New("empty scutil output")
	}

	status := &Status{}
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "HTTPEnable":
			status.HTTPEnabled = value == "1"
		case "HTTPProxy":
			if value != "(null)" {
				status.HTTPHost = value
			}
		case "HTTPPort":
			status.HTTPPort, _ = strconv.Atoi(value)
		case "HTTPSEnable":
			status.HTTPSEnabled = value == "1"
		case "HTTPSProxy":
			if value != "(null)" {
				status.HTTPSHost = value
			}
		case "HTTPSPort":
			status.HTTPSPort, _ = strconv.Atoi(value)
		case "SOCKSEnable":
			status.SOCKSEnabled = value == "1"
		case "SOCKSProxy":
			if value != "(null)" {
				status.SOCKSHost = value
			}
		case "SOCKSPort":
			status.SOCKSPort, _ = strconv.Atoi(value)
		}
	}

	return status, nil
}
