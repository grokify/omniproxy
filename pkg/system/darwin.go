package system

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func (d *darwinProxy) Name() string {
	return "darwin"
}

// SetProxy configures macOS to use a proxy.
func (d *darwinProxy) SetProxy(host string, port int) error {
	services, err := d.listNetworkServices()
	if err != nil {
		return err
	}

	portStr := strconv.Itoa(port)

	for _, service := range services {
		// Set HTTP proxy
		if err := exec.Command("networksetup", "-setwebproxy", service, host, portStr).Run(); err != nil {
			return fmt.Errorf("failed to set HTTP proxy for %s: %w", service, err)
		}
		if err := exec.Command("networksetup", "-setwebproxystate", service, "on").Run(); err != nil {
			return fmt.Errorf("failed to enable HTTP proxy for %s: %w", service, err)
		}

		// Set HTTPS proxy
		if err := exec.Command("networksetup", "-setsecurewebproxy", service, host, portStr).Run(); err != nil {
			return fmt.Errorf("failed to set HTTPS proxy for %s: %w", service, err)
		}
		if err := exec.Command("networksetup", "-setsecurewebproxystate", service, "on").Run(); err != nil {
			return fmt.Errorf("failed to enable HTTPS proxy for %s: %w", service, err)
		}
	}

	return nil
}

// UnsetProxy removes macOS proxy configuration.
func (d *darwinProxy) UnsetProxy() error {
	services, err := d.listNetworkServices()
	if err != nil {
		return err
	}

	for _, service := range services {
		// Disable HTTP proxy
		if err := exec.Command("networksetup", "-setwebproxystate", service, "off").Run(); err != nil {
			return fmt.Errorf("failed to disable HTTP proxy for %s: %w", service, err)
		}

		// Disable HTTPS proxy
		if err := exec.Command("networksetup", "-setsecurewebproxystate", service, "off").Run(); err != nil {
			return fmt.Errorf("failed to disable HTTPS proxy for %s: %w", service, err)
		}
	}

	return nil
}

// GetProxy returns the current proxy configuration.
func (d *darwinProxy) GetProxy() (*ProxyConfig, error) {
	services, err := d.listNetworkServices()
	if err != nil {
		return nil, err
	}

	if len(services) == 0 {
		return &ProxyConfig{Enabled: false}, nil
	}

	// Get proxy info from the first service
	service := services[0]

	output, err := exec.Command("networksetup", "-getwebproxy", service).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get proxy info: %w", err)
	}

	config := &ProxyConfig{}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "Enabled":
			config.Enabled = value == "Yes"
		case "Server":
			config.Host = value
		case "Port":
			if port, err := strconv.Atoi(value); err == nil {
				config.Port = port
			}
		}
	}

	if config.Host != "" && config.Port > 0 {
		config.HTTPProxy = fmt.Sprintf("http://%s:%d", config.Host, config.Port)
		config.HTTPSProxy = config.HTTPProxy
	}

	return config, nil
}

// InstallCA installs a CA certificate into the macOS Keychain.
func (d *darwinProxy) InstallCA(certPath string, certName string) error {
	// Add to System keychain (requires sudo)
	cmd := exec.Command("security", "add-trusted-cert", "-d", "-r", "trustRoot",
		"-k", "/Library/Keychains/System.keychain", certPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to install CA (may require sudo): %s: %w", string(output), err)
	}
	return nil
}

// UninstallCA removes a CA certificate from the macOS Keychain.
func (d *darwinProxy) UninstallCA(certName string) error {
	cmd := exec.Command("security", "delete-certificate", "-c", certName,
		"/Library/Keychains/System.keychain")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to uninstall CA: %s: %w", string(output), err)
	}
	return nil
}

// IsCAInstalled checks if a CA certificate is installed.
func (d *darwinProxy) IsCAInstalled(certName string) (bool, error) {
	cmd := exec.Command("security", "find-certificate", "-c", certName,
		"/Library/Keychains/System.keychain")
	err := cmd.Run()
	if err != nil {
		// Certificate not found is not an error, just means it's not installed
		return false, nil
	}
	return true, nil
}

// listNetworkServices returns a list of active network services.
func (d *darwinProxy) listNetworkServices() ([]string, error) {
	output, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list network services: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	services := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip header line and disabled services (marked with *)
		if line == "" || strings.HasPrefix(line, "An asterisk") || strings.HasPrefix(line, "*") {
			continue
		}
		services = append(services, line)
	}

	return services, nil
}
