package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func (l *linuxProxy) Name() string {
	return "linux"
}

// SetProxy configures Linux to use a proxy.
// This sets environment variables and attempts to configure GNOME/KDE settings.
func (l *linuxProxy) SetProxy(host string, port int) error {
	proxyURL := fmt.Sprintf("http://%s:%d", host, port)

	// Try GNOME settings
	if l.hasGnome() {
		if err := l.setGnomeProxy(host, port); err != nil {
			// Non-fatal, continue
			fmt.Fprintf(os.Stderr, "Warning: failed to set GNOME proxy: %v\n", err)
		}
	}

	// Try KDE settings
	if l.hasKDE() {
		if err := l.setKDEProxy(host, port); err != nil {
			// Non-fatal, continue
			fmt.Fprintf(os.Stderr, "Warning: failed to set KDE proxy: %v\n", err)
		}
	}

	// Set environment variables for current session
	os.Setenv("HTTP_PROXY", proxyURL)
	os.Setenv("HTTPS_PROXY", proxyURL)
	os.Setenv("http_proxy", proxyURL)
	os.Setenv("https_proxy", proxyURL)

	// Print instructions for shell configuration
	fmt.Printf("To use the proxy in your terminal, run:\n")
	fmt.Printf("  export HTTP_PROXY=%s\n", proxyURL)
	fmt.Printf("  export HTTPS_PROXY=%s\n", proxyURL)

	return nil
}

// UnsetProxy removes Linux proxy configuration.
func (l *linuxProxy) UnsetProxy() error {
	// Try GNOME settings
	if l.hasGnome() {
		if err := l.unsetGnomeProxy(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to unset GNOME proxy: %v\n", err)
		}
	}

	// Try KDE settings
	if l.hasKDE() {
		if err := l.unsetKDEProxy(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to unset KDE proxy: %v\n", err)
		}
	}

	// Unset environment variables
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("http_proxy")
	os.Unsetenv("https_proxy")

	fmt.Printf("To disable the proxy in your terminal, run:\n")
	fmt.Printf("  unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy\n")

	return nil
}

// GetProxy returns the current proxy configuration.
func (l *linuxProxy) GetProxy() (*ProxyConfig, error) {
	config := &ProxyConfig{}

	// Check environment variables
	httpProxy := os.Getenv("HTTP_PROXY")
	if httpProxy == "" {
		httpProxy = os.Getenv("http_proxy")
	}
	httpsProxy := os.Getenv("HTTPS_PROXY")
	if httpsProxy == "" {
		httpsProxy = os.Getenv("https_proxy")
	}

	if httpProxy != "" {
		config.HTTPProxy = httpProxy
		config.Enabled = true

		// Parse host:port from URL
		proxyURL := strings.TrimPrefix(httpProxy, "http://")
		proxyURL = strings.TrimPrefix(proxyURL, "https://")
		parts := strings.Split(proxyURL, ":")
		if len(parts) >= 1 {
			config.Host = parts[0]
		}
		if len(parts) >= 2 {
			if port, err := strconv.Atoi(parts[1]); err == nil {
				config.Port = port
			}
		}
	}

	if httpsProxy != "" {
		config.HTTPSProxy = httpsProxy
		config.Enabled = true
	}

	return config, nil
}

// InstallCA installs a CA certificate into the system trust store.
func (l *linuxProxy) InstallCA(certPath string, certName string) error {
	// Try different locations based on distribution

	// Debian/Ubuntu: /usr/local/share/ca-certificates/
	debianPath := "/usr/local/share/ca-certificates/" + certName + ".crt"
	if l.fileExists("/usr/local/share/ca-certificates") {
		if err := l.copyFile(certPath, debianPath); err != nil {
			return fmt.Errorf("failed to copy certificate: %w", err)
		}
		if output, err := exec.Command("update-ca-certificates").CombinedOutput(); err != nil {
			return fmt.Errorf("failed to update certificates: %s: %w", string(output), err)
		}
		return nil
	}

	// RHEL/CentOS/Fedora: /etc/pki/ca-trust/source/anchors/
	rhelPath := "/etc/pki/ca-trust/source/anchors/" + certName + ".crt"
	if l.fileExists("/etc/pki/ca-trust/source/anchors") {
		if err := l.copyFile(certPath, rhelPath); err != nil {
			return fmt.Errorf("failed to copy certificate: %w", err)
		}
		if output, err := exec.Command("update-ca-trust", "extract").CombinedOutput(); err != nil {
			return fmt.Errorf("failed to update certificates: %s: %w", string(output), err)
		}
		return nil
	}

	// Arch Linux: /etc/ca-certificates/trust-source/anchors/
	archPath := "/etc/ca-certificates/trust-source/anchors/" + certName + ".crt"
	if l.fileExists("/etc/ca-certificates/trust-source/anchors") {
		if err := l.copyFile(certPath, archPath); err != nil {
			return fmt.Errorf("failed to copy certificate: %w", err)
		}
		if output, err := exec.Command("trust", "extract-compat").CombinedOutput(); err != nil {
			return fmt.Errorf("failed to update certificates: %s: %w", string(output), err)
		}
		return nil
	}

	return fmt.Errorf("could not detect Linux distribution for CA installation")
}

// UninstallCA removes a CA certificate from the system trust store.
func (l *linuxProxy) UninstallCA(certName string) error {
	// Try different locations based on distribution
	paths := []string{
		"/usr/local/share/ca-certificates/" + certName + ".crt",
		"/etc/pki/ca-trust/source/anchors/" + certName + ".crt",
		"/etc/ca-certificates/trust-source/anchors/" + certName + ".crt",
	}

	for _, path := range paths {
		if l.fileExists(path) {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("failed to remove certificate: %w", err)
			}
		}
	}

	// Update certificate store - try each method, ignore errors as not all methods exist on all distros
	if _, err := exec.Command("update-ca-certificates").CombinedOutput(); err != nil {
		if _, err := exec.Command("update-ca-trust", "extract").CombinedOutput(); err != nil {
			_, _ = exec.Command("trust", "extract-compat").CombinedOutput() // Last resort, ignore error
		}
	}

	return nil
}

// IsCAInstalled checks if a CA certificate is installed.
func (l *linuxProxy) IsCAInstalled(certName string) (bool, error) {
	paths := []string{
		"/usr/local/share/ca-certificates/" + certName + ".crt",
		"/etc/pki/ca-trust/source/anchors/" + certName + ".crt",
		"/etc/ca-certificates/trust-source/anchors/" + certName + ".crt",
	}

	for _, path := range paths {
		if l.fileExists(path) {
			return true, nil
		}
	}

	return false, nil
}

// hasGnome checks if GNOME settings are available.
func (l *linuxProxy) hasGnome() bool {
	_, err := exec.LookPath("gsettings")
	return err == nil
}

// hasKDE checks if KDE settings are available.
func (l *linuxProxy) hasKDE() bool {
	_, err := exec.LookPath("kwriteconfig5")
	return err == nil
}

// setGnomeProxy configures GNOME proxy settings.
func (l *linuxProxy) setGnomeProxy(host string, port int) error {
	portStr := strconv.Itoa(port)

	commands := [][]string{
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "manual"},
		{"gsettings", "set", "org.gnome.system.proxy.http", "host", host},
		{"gsettings", "set", "org.gnome.system.proxy.http", "port", portStr},
		{"gsettings", "set", "org.gnome.system.proxy.https", "host", host},
		{"gsettings", "set", "org.gnome.system.proxy.https", "port", portStr},
	}

	for _, args := range commands {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil { //nolint:gosec // G204: args are not user-controlled shell input
			return err
		}
	}

	return nil
}

// unsetGnomeProxy disables GNOME proxy settings.
func (l *linuxProxy) unsetGnomeProxy() error {
	return exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none").Run()
}

// setKDEProxy configures KDE proxy settings.
func (l *linuxProxy) setKDEProxy(host string, port int) error {
	proxyURL := fmt.Sprintf("http://%s:%d", host, port)

	commands := [][]string{
		{"kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ProxyType", "1"},
		{"kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "httpProxy", proxyURL},
		{"kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "httpsProxy", proxyURL},
	}

	for _, args := range commands {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil { //nolint:gosec // G204: args are not user-controlled shell input
			return err
		}
	}

	return nil
}

// unsetKDEProxy disables KDE proxy settings.
func (l *linuxProxy) unsetKDEProxy() error {
	return exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ProxyType", "0").Run()
}

// fileExists checks if a file or directory exists.
func (l *linuxProxy) fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// copyFile copies a file to a destination (used for CA certificates).
func (l *linuxProxy) copyFile(src, dst string) error {
	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, input, 0644) //nolint:gosec // G306: CA certificates are public and need to be readable
}
