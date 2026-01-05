// Package reverseproxy provides reverse proxy functionality with automatic TLS via ACME.
package reverseproxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"github.com/grokify/omniproxy/pkg/capture"
)

// Backend represents a backend server configuration.
type Backend struct {
	// Host is the hostname/pattern to match (e.g., "api.example.com" or "*.example.com")
	Host string `yaml:"host"`
	// Target is the backend URL (e.g., "http://localhost:3000")
	Target string `yaml:"target"`
	// StripPrefix removes a path prefix before forwarding
	StripPrefix string `yaml:"stripPrefix,omitempty"`
	// AddHeaders are headers to add to proxied requests
	AddHeaders map[string]string `yaml:"addHeaders,omitempty"`
	// HealthCheck is the health check path
	HealthCheck string `yaml:"healthCheck,omitempty"`
}

// Config holds reverse proxy configuration.
type Config struct {
	// HTTPPort is the port for HTTP traffic (default: 80)
	HTTPPort int
	// HTTPSPort is the port for HTTPS traffic (default: 443)
	HTTPSPort int
	// Backends is the list of backend configurations
	Backends []Backend
	// ACMEEmail is the email for Let's Encrypt registration
	ACMEEmail string
	// ACMECacheDir is the directory to cache certificates
	ACMECacheDir string
	// ACMEStaging uses Let's Encrypt staging environment (for testing)
	ACMEStaging bool
	// Capturer is the traffic capturer (optional)
	Capturer *capture.Capturer
	// Verbose enables verbose logging
	Verbose bool
	// RedirectHTTP redirects HTTP to HTTPS
	RedirectHTTP bool
}

// DefaultConfig returns default reverse proxy configuration.
func DefaultConfig() *Config {
	return &Config{
		HTTPPort:     80,
		HTTPSPort:    443,
		ACMECacheDir: "~/.omniproxy/acme",
		RedirectHTTP: true,
	}
}

// ReverseProxy represents a reverse proxy server with ACME support.
type ReverseProxy struct {
	config      *Config
	certManager *autocert.Manager
	proxies     map[string]*httputil.ReverseProxy
	mu          sync.RWMutex
	capturer    *capture.Capturer
}

// New creates a new reverse proxy with the given configuration.
func New(cfg *Config) (*ReverseProxy, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if len(cfg.Backends) == 0 {
		return nil, fmt.Errorf("at least one backend is required")
	}

	rp := &ReverseProxy{
		config:   cfg,
		proxies:  make(map[string]*httputil.ReverseProxy),
		capturer: cfg.Capturer,
	}

	// Setup reverse proxies for each backend
	for _, backend := range cfg.Backends {
		targetURL, err := url.Parse(backend.Target)
		if err != nil {
			return nil, fmt.Errorf("invalid backend target %q: %w", backend.Target, err)
		}

		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		proxy.ErrorHandler = rp.errorHandler

		// Customize director to add headers and strip prefix
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)

			// Strip prefix if configured
			if backend.StripPrefix != "" {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, backend.StripPrefix)
				if req.URL.Path == "" {
					req.URL.Path = "/"
				}
			}

			// Add custom headers
			for k, v := range backend.AddHeaders {
				req.Header.Set(k, v)
			}

			// Preserve original host for the backend
			req.Header.Set("X-Forwarded-Host", req.Host)
			req.Header.Set("X-Forwarded-Proto", "https")
			if req.TLS == nil {
				req.Header.Set("X-Forwarded-Proto", "http")
			}
		}

		rp.proxies[backend.Host] = proxy
	}

	// Setup ACME certificate manager
	hosts := make([]string, 0, len(cfg.Backends))
	for _, backend := range cfg.Backends {
		// Only add non-wildcard hosts to whitelist
		if !strings.HasPrefix(backend.Host, "*") {
			hosts = append(hosts, backend.Host)
		}
	}

	rp.certManager = &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(hosts...),
		Cache:      autocert.DirCache(expandPath(cfg.ACMECacheDir)),
		Email:      cfg.ACMEEmail,
	}

	return rp, nil
}

// ServeHTTP implements the http.Handler interface.
func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Find matching backend
	proxy := rp.findProxy(r.Host)
	if proxy == nil {
		http.Error(w, "Backend not found", http.StatusBadGateway)
		return
	}

	// Capture request if capturer is configured
	var rec *capture.Record
	if rp.capturer != nil {
		rec = rp.capturer.StartCapture(r)
	}

	// Wrap response writer to capture response
	wrapper := &responseWrapper{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	// Proxy the request
	proxy.ServeHTTP(wrapper, r)

	// Finish capture
	if rec != nil {
		rp.capturer.FinishCaptureWithStatus(rec, wrapper.statusCode, wrapper.bytesWritten)
	}
}

// findProxy finds the reverse proxy for a given host.
func (rp *ReverseProxy) findProxy(host string) *httputil.ReverseProxy {
	rp.mu.RLock()
	defer rp.mu.RUnlock()

	// Strip port from host
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}

	// Try exact match first
	if proxy, ok := rp.proxies[host]; ok {
		return proxy
	}

	// Try wildcard matches
	for pattern, proxy := range rp.proxies {
		if strings.HasPrefix(pattern, "*.") {
			suffix := pattern[1:] // Remove *
			if strings.HasSuffix(host, suffix) {
				return proxy
			}
		}
	}

	return nil
}

// errorHandler handles proxy errors.
func (rp *ReverseProxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	if rp.config.Verbose {
		log.Printf("Proxy error for %s: %v", r.Host, err)
	}
	http.Error(w, "Bad Gateway", http.StatusBadGateway)
}

// ListenAndServe starts the reverse proxy server.
func (rp *ReverseProxy) ListenAndServe() error {
	errChan := make(chan error, 2)

	// Start HTTP server
	go func() {
		httpAddr := fmt.Sprintf(":%d", rp.config.HTTPPort)

		var handler http.Handler
		if rp.config.RedirectHTTP {
			// Redirect HTTP to HTTPS
			handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Handle ACME HTTP-01 challenge
				if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
					rp.certManager.HTTPHandler(nil).ServeHTTP(w, r)
					return
				}

				// Redirect to HTTPS
				target := "https://" + r.Host + r.RequestURI
				http.Redirect(w, r, target, http.StatusMovedPermanently)
			})
		} else {
			handler = rp
		}

		log.Printf("HTTP server listening on %s", httpAddr)
		if err := http.ListenAndServe(httpAddr, handler); err != nil {
			errChan <- fmt.Errorf("HTTP server: %w", err)
		}
	}()

	// Start HTTPS server
	go func() {
		httpsAddr := fmt.Sprintf(":%d", rp.config.HTTPSPort)

		server := &http.Server{
			Addr:      httpsAddr,
			Handler:   rp,
			TLSConfig: rp.TLSConfig(),
		}

		log.Printf("HTTPS server listening on %s", httpsAddr)
		if err := server.ListenAndServeTLS("", ""); err != nil {
			errChan <- fmt.Errorf("HTTPS server: %w", err)
		}
	}()

	return <-errChan
}

// TLSConfig returns the TLS configuration with ACME support.
func (rp *ReverseProxy) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: rp.certManager.GetCertificate,
		NextProtos:     []string{"h2", "http/1.1"},
	}
}

// HealthCheck performs health checks on all backends.
func (rp *ReverseProxy) HealthCheck(ctx context.Context) map[string]bool {
	results := make(map[string]bool)
	client := &http.Client{Timeout: 5 * time.Second}

	for _, backend := range rp.config.Backends {
		if backend.HealthCheck == "" {
			results[backend.Host] = true
			continue
		}

		checkURL := backend.Target + backend.HealthCheck
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
		if err != nil {
			results[backend.Host] = false
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			results[backend.Host] = false
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body) // Discard body; errors don't affect health check result
		resp.Body.Close()

		results[backend.Host] = resp.StatusCode >= 200 && resp.StatusCode < 400
	}

	return results
}

// responseWrapper wraps http.ResponseWriter to capture status code and bytes written.
type responseWrapper struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (rw *responseWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWrapper) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// expandPath expands ~ to home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return strings.Replace(path, "~", home, 1)
		}
	}
	return path
}
