// Package system provides OS-level proxy configuration.
package system

import (
	"fmt"
	"runtime"
)

// ProxyConfig represents proxy configuration settings.
type ProxyConfig struct {
	HTTPProxy  string
	HTTPSProxy string
	Host       string
	Port       int
	Enabled    bool
}

// SystemProxy provides an interface for OS-level proxy configuration.
type SystemProxy interface {
	// Name returns the OS name
	Name() string

	// SetProxy configures the system to use a proxy
	SetProxy(host string, port int) error

	// UnsetProxy removes the system proxy configuration
	UnsetProxy() error

	// GetProxy returns the current proxy configuration
	GetProxy() (*ProxyConfig, error)

	// InstallCA installs a CA certificate into the system trust store
	InstallCA(certPath string, certName string) error

	// UninstallCA removes a CA certificate from the system trust store
	UninstallCA(certName string) error

	// IsCAInstalled checks if a CA certificate is installed
	IsCAInstalled(certName string) (bool, error)
}

// Platform-specific proxy types (implementations in platform files)
type darwinProxy struct{}
type windowsProxy struct{}
type linuxProxy struct{}

// New returns a SystemProxy implementation for the current OS.
func New() (SystemProxy, error) {
	switch runtime.GOOS {
	case "darwin":
		return &darwinProxy{}, nil
	case "windows":
		return &windowsProxy{}, nil
	case "linux":
		return &linuxProxy{}, nil
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// SetSystemProxy is a convenience function to set the system proxy.
func SetSystemProxy(host string, port int) error {
	sp, err := New()
	if err != nil {
		return err
	}
	return sp.SetProxy(host, port)
}

// UnsetSystemProxy is a convenience function to unset the system proxy.
func UnsetSystemProxy() error {
	sp, err := New()
	if err != nil {
		return err
	}
	return sp.UnsetProxy()
}

// InstallCA is a convenience function to install a CA certificate.
func InstallCA(certPath, certName string) error {
	sp, err := New()
	if err != nil {
		return err
	}
	return sp.InstallCA(certPath, certName)
}

// UninstallCA is a convenience function to uninstall a CA certificate.
func UninstallCA(certName string) error {
	sp, err := New()
	if err != nil {
		return err
	}
	return sp.UninstallCA(certName)
}
