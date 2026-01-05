package system

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func (w *windowsProxy) Name() string {
	return "windows"
}

// SetProxy configures Windows to use a proxy via registry.
func (w *windowsProxy) SetProxy(host string, port int) error {
	proxyServer := fmt.Sprintf("%s:%d", host, port)

	// Enable proxy
	if err := w.setRegistryValue("ProxyEnable", "1"); err != nil {
		return err
	}

	// Set proxy server
	if err := w.setRegistryValue("ProxyServer", proxyServer); err != nil {
		return err
	}

	// Notify Windows of the change
	return w.notifySettingsChange()
}

// UnsetProxy removes Windows proxy configuration.
func (w *windowsProxy) UnsetProxy() error {
	// Disable proxy
	if err := w.setRegistryValue("ProxyEnable", "0"); err != nil {
		return err
	}

	// Notify Windows of the change
	return w.notifySettingsChange()
}

// GetProxy returns the current proxy configuration.
func (w *windowsProxy) GetProxy() (*ProxyConfig, error) {
	config := &ProxyConfig{}

	// Check if proxy is enabled
	enabled := w.getRegistryValue("ProxyEnable")
	config.Enabled = enabled == "1" || enabled == "0x1"

	// Get proxy server
	server := w.getRegistryValue("ProxyServer")

	if server != "" {
		config.HTTPProxy = "http://" + server
		config.HTTPSProxy = config.HTTPProxy

		// Parse host:port
		parts := strings.Split(server, ":")
		if len(parts) == 2 {
			config.Host = parts[0]
			if port, err := strconv.Atoi(parts[1]); err == nil {
				config.Port = port
			}
		}
	}

	return config, nil
}

// InstallCA installs a CA certificate into the Windows certificate store.
func (w *windowsProxy) InstallCA(certPath string, certName string) error {
	// Import to Root certificate store
	cmd := exec.Command("certutil", "-addstore", "-user", "Root", certPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to install CA: %s: %w", string(output), err)
	}
	return nil
}

// UninstallCA removes a CA certificate from the Windows certificate store.
func (w *windowsProxy) UninstallCA(certName string) error {
	cmd := exec.Command("certutil", "-delstore", "-user", "Root", certName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to uninstall CA: %s: %w", string(output), err)
	}
	return nil
}

// IsCAInstalled checks if a CA certificate is installed.
func (w *windowsProxy) IsCAInstalled(certName string) (bool, error) {
	cmd := exec.Command("certutil", "-store", "-user", "Root")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to list certificates: %w", err)
	}
	return strings.Contains(string(output), certName), nil
}

// setRegistryValue sets a value in the Windows Internet Settings registry key.
func (w *windowsProxy) setRegistryValue(name, value string) error {
	regPath := `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`

	var valueType string
	if name == "ProxyEnable" {
		valueType = "REG_DWORD"
	} else {
		valueType = "REG_SZ"
	}

	cmd := exec.Command("reg", "add", regPath, "/v", name, "/t", valueType, "/d", value, "/f")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set registry value %s: %s: %w", name, string(output), err)
	}
	return nil
}

// getRegistryValue gets a value from the Windows Internet Settings registry key.
// Returns empty string if value doesn't exist.
func (w *windowsProxy) getRegistryValue(name string) string {
	regPath := `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`

	cmd := exec.Command("reg", "query", regPath, "/v", name)
	output, err := cmd.Output()
	if err != nil {
		return "" // Value doesn't exist
	}

	// Parse output to extract value
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, name) {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				return fields[len(fields)-1]
			}
		}
	}

	return ""
}

// notifySettingsChange notifies Windows that Internet settings have changed.
func (w *windowsProxy) notifySettingsChange() error {
	// Use PowerShell to refresh proxy settings
	script := `
$source = @"
using System;
using System.Runtime.InteropServices;
public class WinINet {
    [DllImport("wininet.dll", SetLastError = true)]
    public static extern bool InternetSetOption(IntPtr hInternet, int dwOption, IntPtr lpBuffer, int lpdwBufferLength);
}
"@
Add-Type -TypeDefinition $source
$INTERNET_OPTION_SETTINGS_CHANGED = 39
$INTERNET_OPTION_REFRESH = 37
[WinINet]::InternetSetOption([IntPtr]::Zero, $INTERNET_OPTION_SETTINGS_CHANGED, [IntPtr]::Zero, 0) | Out-Null
[WinINet]::InternetSetOption([IntPtr]::Zero, $INTERNET_OPTION_REFRESH, [IntPtr]::Zero, 0) | Out-Null
`
	cmd := exec.Command("powershell", "-Command", script)
	if _, err := cmd.CombinedOutput(); err != nil {
		// Non-fatal - settings will apply on next connection
		return nil
	}
	return nil
}
