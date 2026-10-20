// Package sysproxy provides macOS system proxy control.
package sysproxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const proxyChangeTimeout = 15 * time.Second

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

	// Cancelling bootstrap must also stop its current networksetup child before
	// the app clears proxy settings during shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, proxyChangeTimeout)
	defer cancel()

	services, err := getNetworkServices(ctx, false)
	if err != nil {
		return fmt.Errorf("get network services: %w", err)
	}

	host := "127.0.0.1"
	portStr := strconv.Itoa(port)

	for _, svc := range services {
		// Set HTTP proxy
		if err := runNetworksetup(ctx, "-setwebproxy", svc, host, portStr); err != nil {
			return fmt.Errorf("set http proxy for %s: %w", svc, err)
		}
		// Set HTTPS proxy
		if err := runNetworksetup(ctx, "-setsecurewebproxy", svc, host, portStr); err != nil {
			return fmt.Errorf("set https proxy for %s: %w", svc, err)
		}
		// Set SOCKS proxy
		if err := runNetworksetup(ctx, "-setsocksfirewallproxy", svc, host, portStr); err != nil {
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

	ctx, cancel := context.WithTimeout(context.Background(), proxyChangeTimeout)
	defer cancel()
	return disable(ctx)
}

func disable(ctx context.Context) error {
	// Disabled services can still retain proxy settings from an earlier session.
	services, err := getNetworkServices(ctx, true)
	if err != nil {
		return fmt.Errorf("get network services: %w", err)
	}

	var failures []error
	for _, svc := range services {
		for _, setting := range []struct{ flag, name string }{
			{"-setwebproxystate", "http"},
			{"-setsecurewebproxystate", "https"},
			{"-setsocksfirewallproxystate", "socks"},
		} {
			if err := ctx.Err(); err != nil {
				return errors.Join(append(failures, err)...)
			}
			if err := runNetworksetup(ctx, setting.flag, svc, "off"); err != nil {
				// A failure on one service must not leave every other service enabled.
				failures = append(failures, fmt.Errorf("disable %s proxy for %s: %w", setting.name, svc, err))
			}
		}
	}

	return errors.Join(failures...)
}

func runNetworksetup(ctx context.Context, args ...string) error {
	output, err := exec.CommandContext(ctx, "networksetup", args...).CombinedOutput()
	message := strings.TrimSpace(string(output))
	if err != nil {
		return fmt.Errorf("%w: %s", errors.Join(err, ctx.Err()), message)
	}
	// networksetup can report configuration errors with a zero exit status.
	if strings.Contains(message, "Error:") {
		return errors.New(message)
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
func getNetworkServices(ctx context.Context, includeDisabled bool) ([]string, error) {
	output, err := exec.CommandContext(ctx, "networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return nil, fmt.Errorf("list network services: %w", errors.Join(err, ctx.Err()))
	}

	var services []string
	lines := strings.Split(string(output), "\n")
	for i, line := range lines {
		// Skip header line and explanation line
		if i == 0 || strings.HasPrefix(line, "An asterisk") {
			continue
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "*") {
			if !includeDisabled {
				continue
			}
			line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		}
		if line != "" {
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
