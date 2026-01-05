// Package proxy provides HTTP/HTTPS proxy functionality using goproxy.
package proxy

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/grokify/omniproxy/pkg/ca"
	"github.com/grokify/omniproxy/pkg/capture"
)

// Proxy represents an HTTP/HTTPS proxy server.
type Proxy struct {
	server   *goproxy.ProxyHttpServer
	ca       *ca.CA
	capturer *capture.Capturer
	config   *Config
}

// Config holds proxy configuration options.
type Config struct {
	// Port to listen on
	Port int
	// Verbose enables verbose logging
	Verbose bool
	// EnableMITM enables HTTPS interception
	EnableMITM bool
	// CA is the certificate authority for MITM (required if EnableMITM is true)
	CA *ca.CA
	// Capturer is the traffic capturer (optional)
	Capturer *capture.Capturer
	// SkipHosts is a list of hosts to skip MITM for (e.g., hosts with cert pinning)
	SkipHosts []string
	// Upstream is the upstream proxy URL (e.g., http://proxy:8080)
	Upstream string
}

// DefaultConfig returns default proxy configuration.
func DefaultConfig() *Config {
	return &Config{
		Port:       8080,
		Verbose:    false,
		EnableMITM: true,
		SkipHosts:  []string{},
	}
}

// New creates a new proxy with the given configuration.
func New(cfg *Config) (*Proxy, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	server := goproxy.NewProxyHttpServer()
	server.Verbose = cfg.Verbose

	p := &Proxy{
		server:   server,
		ca:       cfg.CA,
		capturer: cfg.Capturer,
		config:   cfg,
	}

	// Setup upstream proxy if configured
	if cfg.Upstream != "" {
		if err := p.setupUpstream(cfg.Upstream); err != nil {
			return nil, err
		}
	}

	// Setup MITM if enabled
	if cfg.EnableMITM && cfg.CA != nil {
		if err := p.setupMITM(); err != nil {
			return nil, err
		}
	}

	// Setup request/response capture
	if cfg.Capturer != nil {
		p.setupCapture()
	}

	return p, nil
}

// setupUpstream configures upstream proxy chaining.
func (p *Proxy) setupUpstream(upstreamURL string) error {
	upstream, err := url.Parse(upstreamURL)
	if err != nil {
		return err
	}

	// Set the proxy's transport to use the upstream proxy
	p.server.Tr = &http.Transport{
		Proxy: http.ProxyURL(upstream),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // Intentional for MITM proxy to intercept HTTPS
			MinVersion:         tls.VersionTLS12,
		},
	}

	// For CONNECT requests, use the upstream proxy
	p.server.ConnectDial = p.server.NewConnectDialToProxy(upstreamURL)

	return nil
}

// setupMITM configures HTTPS interception.
func (p *Proxy) setupMITM() error {
	tlsCert, err := p.ca.TLSCertificate()
	if err != nil {
		return err
	}

	// Set up goproxy's MITM config
	goproxy.GoproxyCa = tlsCert
	goproxy.OkConnect = &goproxy.ConnectAction{Action: goproxy.ConnectAccept, TLSConfig: goproxy.TLSConfigFromCA(&tlsCert)}
	goproxy.MitmConnect = &goproxy.ConnectAction{Action: goproxy.ConnectMitm, TLSConfig: goproxy.TLSConfigFromCA(&tlsCert)}
	goproxy.HTTPMitmConnect = &goproxy.ConnectAction{Action: goproxy.ConnectHTTPMitm, TLSConfig: goproxy.TLSConfigFromCA(&tlsCert)}
	goproxy.RejectConnect = &goproxy.ConnectAction{Action: goproxy.ConnectReject, TLSConfig: goproxy.TLSConfigFromCA(&tlsCert)}

	// Configure TLS
	tlsConfig := func(host string, ctx *goproxy.ProxyCtx) (*tls.Config, error) {
		return &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // Intentional for MITM proxy to intercept HTTPS
			MinVersion:         tls.VersionTLS12,
		}, nil
	}

	// Handle CONNECT requests
	p.server.OnRequest().HandleConnect(goproxy.FuncHttpsHandler(
		func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
			// Check if we should skip MITM for this host
			for _, skipHost := range p.config.SkipHosts {
				if host == skipHost || matchWildcard(skipHost, host) {
					return goproxy.OkConnect, host
				}
			}
			return goproxy.MitmConnect, host
		}))

	// Set custom TLS config
	p.server.OnRequest().HandleConnect(goproxy.AlwaysMitm)
	_ = tlsConfig // Will be used in more advanced setup

	return nil
}

// setupCapture configures request/response capture.
func (p *Proxy) setupCapture() {
	// Capture requests
	p.server.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		if p.capturer != nil {
			ctx.UserData = p.capturer.StartCapture(req)
		}
		return req, nil
	})

	// Capture responses
	p.server.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if p.capturer != nil && ctx.UserData != nil {
			if rec, ok := ctx.UserData.(*capture.Record); ok {
				p.capturer.FinishCapture(rec, resp)
			}
		}
		return resp
	})
}

// Server returns the underlying goproxy server.
func (p *Proxy) Server() *goproxy.ProxyHttpServer {
	return p.server
}

// ListenAndServe starts the proxy server.
func (p *Proxy) ListenAndServe(addr string) error {
	log.Printf("OmniProxy listening on %s", addr)
	if p.config.EnableMITM {
		log.Printf("MITM enabled - HTTPS traffic will be intercepted")
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           p.server,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return server.ListenAndServe()
}

// matchWildcard performs simple wildcard matching (e.g., *.example.com).
func matchWildcard(pattern, host string) bool {
	if len(pattern) == 0 {
		return false
	}
	if pattern[0] == '*' {
		suffix := pattern[1:]
		return len(host) >= len(suffix) && host[len(host)-len(suffix):] == suffix
	}
	return pattern == host
}
