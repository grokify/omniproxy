// Package config provides configuration file support for OmniProxy.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the OmniProxy configuration file.
type Config struct {
	// Server configuration
	Server ServerConfig `yaml:"server"`

	// MITM configuration
	MITM MITMConfig `yaml:"mitm"`

	// Reverse proxy configuration
	Reverse ReverseConfig `yaml:"reverse,omitempty"`

	// Capture configuration
	Capture CaptureConfig `yaml:"capture"`

	// Filter configuration
	Filter FilterConfig `yaml:"filter"`

	// Upstream proxy configuration
	Upstream string `yaml:"upstream,omitempty"`
}

// ServerConfig holds server-related configuration.
type ServerConfig struct {
	// Host to bind to
	Host string `yaml:"host"`
	// Port to listen on
	Port int `yaml:"port"`
	// Verbose logging
	Verbose bool `yaml:"verbose"`
}

// MITMConfig holds MITM-related configuration.
type MITMConfig struct {
	// Enabled enables HTTPS interception
	Enabled bool `yaml:"enabled"`
	// CertPath is the path to the CA certificate
	CertPath string `yaml:"certPath,omitempty"`
	// KeyPath is the path to the CA private key
	KeyPath string `yaml:"keyPath,omitempty"`
	// SkipHosts is a list of hosts to skip MITM for
	SkipHosts []string `yaml:"skipHosts,omitempty"`
}

// CaptureConfig holds capture-related configuration.
type CaptureConfig struct {
	// Output is the output file path
	Output string `yaml:"output,omitempty"`
	// Format is the output format (ndjson, json, har, ir)
	Format string `yaml:"format"`
	// IncludeHeaders controls whether to include headers
	IncludeHeaders bool `yaml:"includeHeaders"`
	// IncludeBody controls whether to include bodies
	IncludeBody bool `yaml:"includeBody"`
	// MaxBodySize is the maximum body size to capture
	MaxBodySize int64 `yaml:"maxBodySize"`
	// FilterHeaders is a list of headers to exclude
	FilterHeaders []string `yaml:"filterHeaders,omitempty"`
}

// FilterConfig holds request/response filtering configuration.
type FilterConfig struct {
	// IncludeHosts is a list of hosts to include
	IncludeHosts []string `yaml:"includeHosts,omitempty"`
	// ExcludeHosts is a list of hosts to exclude
	ExcludeHosts []string `yaml:"excludeHosts,omitempty"`
	// IncludePaths is a list of paths to include
	IncludePaths []string `yaml:"includePaths,omitempty"`
	// ExcludePaths is a list of paths to exclude
	ExcludePaths []string `yaml:"excludePaths,omitempty"`
	// IncludeMethods is a list of methods to include
	IncludeMethods []string `yaml:"includeMethods,omitempty"`
	// ExcludeMethods is a list of methods to exclude
	ExcludeMethods []string `yaml:"excludeMethods,omitempty"`
}

// ReverseConfig holds reverse proxy configuration.
type ReverseConfig struct {
	// HTTPPort is the HTTP port (default: 80)
	HTTPPort int `yaml:"httpPort"`
	// HTTPSPort is the HTTPS port (default: 443)
	HTTPSPort int `yaml:"httpsPort"`
	// Backends is the list of backend configurations
	Backends []BackendConfig `yaml:"backends,omitempty"`
	// ACMEEmail is the email for Let's Encrypt registration
	ACMEEmail string `yaml:"acmeEmail,omitempty"`
	// ACMECacheDir is the directory to cache ACME certificates
	ACMECacheDir string `yaml:"acmeCacheDir,omitempty"`
	// ACMEStaging uses Let's Encrypt staging environment
	ACMEStaging bool `yaml:"acmeStaging,omitempty"`
	// RedirectHTTP redirects HTTP to HTTPS
	RedirectHTTP bool `yaml:"redirectHTTP"`
}

// BackendConfig holds backend server configuration.
type BackendConfig struct {
	// Host is the hostname to match
	Host string `yaml:"host"`
	// Target is the backend URL
	Target string `yaml:"target"`
	// StripPrefix removes a path prefix before forwarding
	StripPrefix string `yaml:"stripPrefix,omitempty"`
	// AddHeaders are headers to add to proxied requests
	AddHeaders map[string]string `yaml:"addHeaders,omitempty"`
	// HealthCheck is the health check path
	HealthCheck string `yaml:"healthCheck,omitempty"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:    "127.0.0.1",
			Port:    8080,
			Verbose: false,
		},
		MITM: MITMConfig{
			Enabled:   true,
			SkipHosts: []string{},
		},
		Reverse: ReverseConfig{
			HTTPPort:     80,
			HTTPSPort:    443,
			ACMECacheDir: "~/.omniproxy/acme",
			RedirectHTTP: true,
		},
		Capture: CaptureConfig{
			Format:         "ndjson",
			IncludeHeaders: true,
			IncludeBody:    true,
			MaxBodySize:    1024 * 1024, // 1MB
			FilterHeaders: []string{
				"authorization",
				"cookie",
				"set-cookie",
				"x-api-key",
				"x-auth-token",
				"proxy-authorization",
			},
		},
		Filter: FilterConfig{},
	}
}

// Load loads configuration from a YAML file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

// LoadOrDefault loads configuration from a file, or returns default if not found.
func LoadOrDefault(path string) (*Config, error) {
	if path == "" {
		return DefaultConfig(), nil
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	return Load(path)
}

// Save saves configuration to a YAML file.
func (c *Config) Save(path string) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// DefaultConfigPath returns the default configuration file path.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "omniproxy.yaml"
	}
	return filepath.Join(home, ".omniproxy", "config.yaml")
}

// ExampleConfig returns an example configuration as YAML string.
func ExampleConfig() string {
	cfg := DefaultConfig()
	cfg.Capture.Output = "traffic.ndjson"
	cfg.Filter.IncludeHosts = []string{"api.example.com", "*.example.org"}
	cfg.Filter.ExcludePaths = []string{"*.js", "*.css", "*.png", "*.jpg"}
	cfg.MITM.SkipHosts = []string{"*.pinned-app.com"}

	data, _ := yaml.Marshal(cfg)
	return string(data)
}
