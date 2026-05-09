//go:build windows

package sysproxy

import (
	"fmt"
	"net"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

const internetSettingsPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Internet Settings`

// Enable sets the Windows system HTTP/HTTPS proxy via the registry.
func Enable(port int) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, internetSettingsPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer k.Close()

	proxyAddr := "127.0.0.1:" + strconv.Itoa(port)
	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return fmt.Errorf("set ProxyEnable: %w", err)
	}
	if err := k.SetStringValue("ProxyServer", proxyAddr); err != nil {
		return fmt.Errorf("set ProxyServer: %w", err)
	}
	return nil
}

// Disable clears the Windows system proxy setting.
func Disable() error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, internetSettingsPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer k.Close()

	if err := k.SetDWordValue("ProxyEnable", 0); err != nil {
		return fmt.Errorf("set ProxyEnable: %w", err)
	}
	return nil
}

// GetStatus reads the current Windows system proxy state from the registry.
func GetStatus() (*Status, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.READ)
	if err != nil {
		return nil, fmt.Errorf("open registry: %w", err)
	}
	defer k.Close()

	enabled, _, err := k.GetIntegerValue("ProxyEnable")
	if err != nil {
		return nil, fmt.Errorf("read ProxyEnable: %w", err)
	}

	proxyServer, _, err := k.GetStringValue("ProxyServer")
	if err != nil {
		proxyServer = ""
	}

	status := &Status{}
	isOn := enabled == 1
	status.HTTPEnabled = isOn
	status.HTTPSEnabled = isOn
	status.SOCKSEnabled = false

	if isOn && proxyServer != "" {
		host, portStr, err := net.SplitHostPort(proxyServer)
		if err == nil {
			port, _ := strconv.Atoi(portStr)
			status.HTTPHost = host
			status.HTTPPort = port
			status.HTTPSHost = host
			status.HTTPSPort = port
		}
	}

	return status, nil
}
