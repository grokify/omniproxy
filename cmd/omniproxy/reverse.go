package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/grokify/omniproxy/pkg/capture"
	"github.com/grokify/omniproxy/pkg/reverseproxy"
	"github.com/spf13/cobra"
)

type reverseOptions struct {
	httpPort       int
	httpsPort      int
	backends       []string
	acmeEmail      string
	acmeCacheDir   string
	acmeStaging    bool
	verbose        bool
	redirectHTTP   bool
	output         string
	format         string
	filterHeader   []string
	stripPrefix    string
	addHeader      []string
	includeHosts   []string
	excludeHosts   []string
	includePaths   []string
	excludePaths   []string
	includeMethods []string
	excludeMethods []string
}

func newReverseCmd() *cobra.Command {
	opts := &reverseOptions{}

	cmd := &cobra.Command{
		Use:   "reverse",
		Short: "Start the reverse proxy server with ACME/Let's Encrypt",
		Long: `Start a reverse proxy server with automatic TLS certificate management via ACME (Let's Encrypt).

The reverse proxy sits in front of your backend servers and handles TLS termination.
Certificates are automatically obtained and renewed from Let's Encrypt.

Examples:
  # Basic reverse proxy (requires ports 80 and 443)
  sudo omniproxy reverse --backend "api.example.com=http://localhost:3000" --acme-email admin@example.com

  # Multiple backends
  sudo omniproxy reverse \
    --backend "api.example.com=http://localhost:3000" \
    --backend "web.example.com=http://localhost:8080" \
    --acme-email admin@example.com

  # Custom ports (for testing)
  omniproxy reverse --http-port 8080 --https-port 8443 --backend "localhost:8443=http://localhost:3000"

  # Use Let's Encrypt staging (for testing)
  sudo omniproxy reverse --backend "api.example.com=http://localhost:3000" --acme-staging

  # With traffic capture
  sudo omniproxy reverse --backend "api.example.com=http://localhost:3000" --output traffic.ndjson

Note: Running on ports 80 and 443 typically requires root/sudo.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReverse(opts)
		},
	}

	// Port options
	cmd.Flags().IntVar(&opts.httpPort, "http-port", 80, "HTTP port (for ACME challenges and redirect)")
	cmd.Flags().IntVar(&opts.httpsPort, "https-port", 443, "HTTPS port")

	// Backend options
	cmd.Flags().StringSliceVarP(&opts.backends, "backend", "b", nil, "Backend mapping: host=target (e.g., api.example.com=http://localhost:3000)")

	// ACME options
	cmd.Flags().StringVar(&opts.acmeEmail, "acme-email", "", "Email for Let's Encrypt registration (required)")
	cmd.Flags().StringVar(&opts.acmeCacheDir, "acme-cache", "~/.omniproxy/acme", "Directory to cache ACME certificates")
	cmd.Flags().BoolVar(&opts.acmeStaging, "acme-staging", false, "Use Let's Encrypt staging environment (for testing)")

	// Server options
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "Enable verbose logging")
	cmd.Flags().BoolVar(&opts.redirectHTTP, "redirect-http", true, "Redirect HTTP to HTTPS")

	// Output options
	cmd.Flags().StringVarP(&opts.output, "output", "o", "", "Output file for captured traffic")
	cmd.Flags().StringVarP(&opts.format, "format", "f", "ndjson", "Output format: ndjson, json, har, ir")
	cmd.Flags().StringSliceVar(&opts.filterHeader, "filter-header", nil, "Additional headers to filter from output")

	// Routing options
	cmd.Flags().StringVar(&opts.stripPrefix, "strip-prefix", "", "Strip path prefix before forwarding")
	cmd.Flags().StringSliceVar(&opts.addHeader, "add-header", nil, "Headers to add to proxied requests (key=value)")

	// Filtering options
	cmd.Flags().StringSliceVar(&opts.includeHosts, "include-host", nil, "Only capture requests to these hosts")
	cmd.Flags().StringSliceVar(&opts.excludeHosts, "exclude-host", nil, "Exclude requests to these hosts")
	cmd.Flags().StringSliceVar(&opts.includePaths, "include-path", nil, "Only capture requests matching these paths")
	cmd.Flags().StringSliceVar(&opts.excludePaths, "exclude-path", nil, "Exclude requests matching these paths")
	cmd.Flags().StringSliceVar(&opts.includeMethods, "include-method", nil, "Only capture these HTTP methods")
	cmd.Flags().StringSliceVar(&opts.excludeMethods, "exclude-method", nil, "Exclude these HTTP methods")

	return cmd
}

func runReverse(opts *reverseOptions) error {
	if len(opts.backends) == 0 {
		return fmt.Errorf("at least one backend is required (--backend host=target)")
	}

	// Parse backends
	backends := make([]reverseproxy.Backend, 0, len(opts.backends))
	for _, b := range opts.backends {
		host, target, err := parseBackend(b)
		if err != nil {
			return fmt.Errorf("invalid backend %q: %w", b, err)
		}

		backend := reverseproxy.Backend{
			Host:        host,
			Target:      target,
			StripPrefix: opts.stripPrefix,
		}

		// Parse additional headers
		if len(opts.addHeader) > 0 {
			backend.AddHeaders = make(map[string]string)
			for _, h := range opts.addHeader {
				key, value, err := parseHeader(h)
				if err != nil {
					return fmt.Errorf("invalid header %q: %w", h, err)
				}
				backend.AddHeaders[key] = value
			}
		}

		backends = append(backends, backend)
	}

	// Setup filter
	filter := capture.NewFilter()
	filter.IncludeHosts = opts.includeHosts
	filter.ExcludeHosts = opts.excludeHosts
	filter.IncludePaths = opts.includePaths
	filter.ExcludePaths = opts.excludePaths
	filter.IncludeMethods = opts.includeMethods
	filter.ExcludeMethods = opts.excludeMethods

	if err := filter.Compile(); err != nil {
		return fmt.Errorf("failed to compile filter: %w", err)
	}

	// Setup capturer
	var capturer *capture.Capturer
	var outputFile *os.File
	var harWriter *capture.HARWriter

	if opts.output != "" {
		var err error
		outputFile, err = os.Create(opts.output)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer outputFile.Close()

		capturerCfg := capture.DefaultConfig()
		capturerCfg.Output = outputFile
		capturerCfg.Filter = filter

		switch opts.format {
		case "ndjson":
			capturerCfg.Format = capture.FormatNDJSON
		case "json":
			capturerCfg.Format = capture.FormatJSON
		case "har":
			capturerCfg.Format = capture.FormatHAR
			harWriter = capture.NewHARWriter(outputFile)
			capturerCfg.Output = nil
		case "ir":
			capturerCfg.Format = capture.FormatIR
		default:
			return fmt.Errorf("unknown format: %s", opts.format)
		}

		if len(opts.filterHeader) > 0 {
			capturerCfg.FilterHeaders = append(capturerCfg.FilterHeaders, opts.filterHeader...)
		}

		capturer = capture.NewCapturer(capturerCfg)

		if harWriter != nil {
			capturer.AddHandler(func(rec *capture.Record) {
				harWriter.AddRecord(rec)
			})
		}
	}

	// Setup reverse proxy
	cfg := &reverseproxy.Config{
		HTTPPort:     opts.httpPort,
		HTTPSPort:    opts.httpsPort,
		Backends:     backends,
		ACMEEmail:    opts.acmeEmail,
		ACMECacheDir: opts.acmeCacheDir,
		ACMEStaging:  opts.acmeStaging,
		Capturer:     capturer,
		Verbose:      opts.verbose,
		RedirectHTTP: opts.redirectHTTP,
	}

	rp, err := reverseproxy.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create reverse proxy: %w", err)
	}

	// Print startup info
	fmt.Printf("OmniProxy Reverse Proxy\n")
	fmt.Printf("HTTP port: %d\n", opts.httpPort)
	fmt.Printf("HTTPS port: %d\n", opts.httpsPort)
	fmt.Printf("\nBackends:\n")
	for _, b := range backends {
		fmt.Printf("  %s -> %s\n", b.Host, b.Target)
	}

	if opts.acmeEmail != "" {
		fmt.Printf("\nACME Email: %s\n", opts.acmeEmail)
	}
	if opts.acmeStaging {
		fmt.Printf("Using Let's Encrypt STAGING environment\n")
	}

	if opts.redirectHTTP {
		fmt.Printf("HTTP -> HTTPS redirect enabled\n")
	}

	if opts.output != "" {
		fmt.Printf("Capturing traffic to: %s (%s format)\n", opts.output, opts.format)
	}

	fmt.Printf("\nStarting server...\n")

	// Handle graceful shutdown for HAR format
	if harWriter != nil {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-sigChan
			fmt.Println("\nShutting down, writing HAR file...")
			if err := harWriter.Write(); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing HAR: %v\n", err)
			}
			os.Exit(0)
		}()
	}

	return rp.ListenAndServe()
}

// parseBackend parses a backend string like "host=target" into host and target.
func parseBackend(s string) (host, target string, err error) {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			host = s[:i]
			target = s[i+1:]
			if host == "" || target == "" {
				return "", "", fmt.Errorf("empty host or target")
			}
			return host, target, nil
		}
	}
	return "", "", fmt.Errorf("missing '=' separator")
}

// parseHeader parses a header string like "key=value" into key and value.
func parseHeader(s string) (key, value string, err error) {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			key = s[:i]
			value = s[i+1:]
			if key == "" {
				return "", "", fmt.Errorf("empty key")
			}
			return key, value, nil
		}
	}
	return "", "", fmt.Errorf("missing '=' separator")
}
