package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/grokify/omniproxy/pkg/backend"
	"github.com/grokify/omniproxy/pkg/ca"
	"github.com/grokify/omniproxy/pkg/capture"
	"github.com/grokify/omniproxy/pkg/observability"
	"github.com/grokify/omniproxy/pkg/proxy"
	"github.com/spf13/cobra"
)

type serveOptions struct {
	port         int
	host         string
	verbose      bool
	enableMITM   bool
	caPath       string
	keyPath      string
	output       string
	format       string
	skipHosts    []string
	filterHeader []string
	skipBinary   bool

	// Filtering options
	includeHosts   []string
	excludeHosts   []string
	includePaths   []string
	excludePaths   []string
	includeMethods []string
	excludeMethods []string

	// Upstream proxy
	upstream string

	// Observability options
	metricsPort   int
	enableMetrics bool

	// Database options (for team/production mode)
	db string

	// Async options
	asyncQueue     int
	asyncBatchSize int
	asyncWorkers   int
}

func newServeCmd() *cobra.Command {
	opts := &serveOptions{}

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the proxy server",
		Long: `Start an HTTP/HTTPS proxy server.

Deployment Modes:
  Laptop:     omniproxy serve --output traffic.ndjson
  Team:       omniproxy serve --db sqlite://data.db --metrics-port 9090
  Production: omniproxy serve --db postgres://... --metrics-port 9090

Examples:
  # Start proxy on default port (8080)
  omniproxy serve

  # Start proxy with MITM on custom port
  omniproxy serve --port 9090 --mitm

  # Start proxy and capture traffic to file
  omniproxy serve --output traffic.ndjson

  # Capture only specific hosts
  omniproxy serve --include-host "api.example.com" --include-host "*.example.org"

  # Exclude static assets
  omniproxy serve --exclude-path "*.js" --exclude-path "*.css" --exclude-path "*.png"

  # Capture only API calls
  omniproxy serve --include-path "/api/*" --include-method GET --include-method POST

  # Export to HAR format
  omniproxy serve --output traffic.har --format har

  # Chain through upstream proxy
  omniproxy serve --upstream http://corporate-proxy:8080

  # Enable Prometheus metrics
  omniproxy serve --metrics-port 9090`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(opts)
		},
	}

	// Basic options
	cmd.Flags().IntVarP(&opts.port, "port", "p", 8080, "Port to listen on")
	cmd.Flags().StringVar(&opts.host, "host", "127.0.0.1", "Host to bind to")
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "Enable verbose logging")
	cmd.Flags().BoolVar(&opts.enableMITM, "mitm", true, "Enable HTTPS MITM interception")

	// CA options
	cmd.Flags().StringVar(&opts.caPath, "ca-cert", "", "Path to CA certificate (default: ~/.omniproxy/ca/omniproxy-ca.crt)")
	cmd.Flags().StringVar(&opts.keyPath, "ca-key", "", "Path to CA private key (default: ~/.omniproxy/ca/omniproxy-ca.key)")

	// Output options
	cmd.Flags().StringVarP(&opts.output, "output", "o", "", "Output file for captured traffic (default: stdout)")
	cmd.Flags().StringVarP(&opts.format, "format", "f", "ndjson", "Output format: ndjson, json, har, ir")
	cmd.Flags().StringSliceVar(&opts.filterHeader, "filter-header", nil, "Additional headers to filter from output")
	cmd.Flags().BoolVar(&opts.skipBinary, "skip-binary", true, "Skip capturing binary content (images, videos, etc.)")

	// MITM skip options
	cmd.Flags().StringSliceVar(&opts.skipHosts, "skip-host", nil, "Hosts to skip MITM for (supports wildcards)")

	// Filtering options
	cmd.Flags().StringSliceVar(&opts.includeHosts, "include-host", nil, "Only capture requests to these hosts (supports wildcards)")
	cmd.Flags().StringSliceVar(&opts.excludeHosts, "exclude-host", nil, "Exclude requests to these hosts (supports wildcards)")
	cmd.Flags().StringSliceVar(&opts.includePaths, "include-path", nil, "Only capture requests matching these paths (supports wildcards)")
	cmd.Flags().StringSliceVar(&opts.excludePaths, "exclude-path", nil, "Exclude requests matching these paths (supports wildcards)")
	cmd.Flags().StringSliceVar(&opts.includeMethods, "include-method", nil, "Only capture these HTTP methods")
	cmd.Flags().StringSliceVar(&opts.excludeMethods, "exclude-method", nil, "Exclude these HTTP methods")

	// Upstream proxy
	cmd.Flags().StringVar(&opts.upstream, "upstream", "", "Upstream proxy URL (e.g., http://proxy:8080)")

	// Observability options
	cmd.Flags().IntVar(&opts.metricsPort, "metrics-port", 0, "Port for metrics/health endpoints (0 = disabled)")
	cmd.Flags().BoolVar(&opts.enableMetrics, "metrics", false, "Enable Prometheus metrics (requires --metrics-port)")

	// Database options
	cmd.Flags().StringVar(&opts.db, "db", "", "Database URL (sqlite://path or postgres://...)")

	// Async options
	cmd.Flags().IntVar(&opts.asyncQueue, "async-queue", 10000, "Async traffic queue size")
	cmd.Flags().IntVar(&opts.asyncBatchSize, "async-batch", 100, "Async batch size")
	cmd.Flags().IntVar(&opts.asyncWorkers, "async-workers", 2, "Number of async workers")

	return cmd
}

func runServe(opts *serveOptions) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup observability
	var obs *observability.Provider
	var health *observability.HealthChecker
	var err error

	if opts.metricsPort > 0 || opts.enableMetrics {
		obs, err = observability.NewProvider(&observability.Config{
			ServiceName:      "omniproxy",
			EnablePrometheus: true,
		})
		if err != nil {
			return fmt.Errorf("failed to setup observability: %w", err)
		}
		defer func() {
			if err := obs.Shutdown(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "observability shutdown error: %v\n", err)
			}
		}()

		health = observability.NewHealthChecker()
	}

	// Setup CA if MITM is enabled
	var proxyCA *ca.CA

	if opts.enableMITM {
		certPath := opts.caPath
		keyPath := opts.keyPath

		if certPath == "" {
			certPath = ca.DefaultCertPath()
		}
		if keyPath == "" {
			keyPath = ca.DefaultKeyPath()
		}

		proxyCA, err = ca.LoadOrCreate(certPath, keyPath, nil)
		if err != nil {
			return fmt.Errorf("failed to setup CA: %w", err)
		}

		fmt.Printf("Using CA certificate: %s\n", certPath)
		fmt.Printf("To trust this CA, run: omniproxy ca install\n\n")
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

	capturerCfg := capture.DefaultConfig()
	capturerCfg.Filter = filter
	capturerCfg.SkipBinary = opts.skipBinary

	if opts.output != "" {
		outputFile, err = os.Create(opts.output)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer outputFile.Close()
		capturerCfg.Output = outputFile
	}

	switch opts.format {
	case "ndjson":
		capturerCfg.Format = capture.FormatNDJSON
	case "json":
		capturerCfg.Format = capture.FormatJSON
	case "har":
		capturerCfg.Format = capture.FormatHAR
		// For HAR, we buffer records and write at end
		if outputFile != nil {
			harWriter = capture.NewHARWriter(outputFile)
			capturerCfg.Output = nil // Don't write incrementally
		}
	case "ir":
		capturerCfg.Format = capture.FormatIR
	default:
		return fmt.Errorf("unknown format: %s", opts.format)
	}

	if len(opts.filterHeader) > 0 {
		capturerCfg.FilterHeaders = append(capturerCfg.FilterHeaders, opts.filterHeader...)
	}

	capturer = capture.NewCapturer(capturerCfg)

	// Add HAR handler if using HAR format
	if harWriter != nil {
		capturer.AddHandler(func(rec *capture.Record) {
			harWriter.AddRecord(rec)
		})
	}

	// Setup traffic store backend
	var trafficStore backend.TrafficStore
	var backendMetrics backend.Metrics

	if obs != nil {
		backendMetrics = observability.NewBackendMetrics(obs.Metrics)
	}

	if opts.db != "" {
		// Database mode (SQLite or PostgreSQL)
		dbCfg, err := backend.ParseDatabaseURL(opts.db)
		if err != nil {
			return fmt.Errorf("invalid database URL: %w", err)
		}

		fmt.Printf("Connecting to database: %s\n", dbCfg.String())

		dbStore, err := backend.NewDatabaseTrafficStore(ctx, &backend.DatabaseTrafficStoreConfig{
			DatabaseURL: opts.db,
			ProxyName:   "default",
			Metrics:     backendMetrics,
			Debug:       opts.verbose,
		})
		if err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}

		// Wrap with async for non-blocking writes
		trafficStore = backend.NewAsyncTrafficStore(dbStore, &backend.AsyncConfig{
			QueueSize: opts.asyncQueue,
			BatchSize: opts.asyncBatchSize,
			Workers:   opts.asyncWorkers,
			Metrics:   backendMetrics,
		})

		// Register queue depth callback
		if obs != nil {
			if asyncStore, ok := trafficStore.(*backend.AsyncTrafficStoreWrapper); ok {
				if err := obs.Metrics.RegisterQueueDepthCallback(obs.MeterProvider, func() int64 {
					return int64(asyncStore.QueueDepth())
				}); err != nil {
					fmt.Fprintf(os.Stderr, "failed to register queue depth callback: %v\n", err)
				}
			}
		}

		capturer.AddHandler(func(rec *capture.Record) {
			if err := trafficStore.Store(context.Background(), rec); err != nil {
				fmt.Fprintf(os.Stderr, "traffic store error: %v\n", err)
			}
		})

		fmt.Printf("Database connected (proxy_id=%d)\n", dbStore.ProxyID())
	} else if opts.output != "" {
		// File mode (laptop mode)
		fileStore, err := backend.NewFileTrafficStore(&backend.FileTrafficStoreConfig{
			Path:    opts.output,
			Format:  backend.Format(opts.format),
			Metrics: backendMetrics,
		})
		if err != nil {
			return fmt.Errorf("failed to create file store: %w", err)
		}

		// Wrap with async for better performance
		trafficStore = backend.NewAsyncTrafficStore(fileStore, &backend.AsyncConfig{
			QueueSize: opts.asyncQueue,
			BatchSize: opts.asyncBatchSize,
			Workers:   opts.asyncWorkers,
			Metrics:   backendMetrics,
		})

		// Register queue depth callback
		if obs != nil {
			if asyncStore, ok := trafficStore.(*backend.AsyncTrafficStoreWrapper); ok {
				if err := obs.Metrics.RegisterQueueDepthCallback(obs.MeterProvider, func() int64 {
					return int64(asyncStore.QueueDepth())
				}); err != nil {
					fmt.Fprintf(os.Stderr, "failed to register queue depth callback: %v\n", err)
				}
			}
		}

		capturer.AddHandler(func(rec *capture.Record) {
			if err := trafficStore.Store(context.Background(), rec); err != nil {
				fmt.Fprintf(os.Stderr, "traffic store error: %v\n", err)
			}
		})
	}

	// Setup proxy
	proxyCfg := &proxy.Config{
		Port:       opts.port,
		Verbose:    opts.verbose,
		EnableMITM: opts.enableMITM,
		CA:         proxyCA,
		Capturer:   capturer,
		SkipHosts:  opts.skipHosts,
		Upstream:   opts.upstream,
	}

	p, err := proxy.New(proxyCfg)
	if err != nil {
		return fmt.Errorf("failed to create proxy: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", opts.host, opts.port)
	fmt.Printf("OmniProxy starting on %s\n", addr)
	fmt.Printf("Configure your system/browser to use HTTP proxy: %s\n", addr)

	if opts.enableMITM {
		fmt.Printf("MITM enabled - HTTPS traffic will be decrypted\n")
		if len(opts.skipHosts) > 0 {
			fmt.Printf("Skipping MITM for: %v\n", opts.skipHosts)
		}
	}

	if opts.upstream != "" {
		fmt.Printf("Upstream proxy: %s\n", opts.upstream)
	}

	// Print filter info
	if len(opts.includeHosts) > 0 {
		fmt.Printf("Including hosts: %v\n", opts.includeHosts)
	}
	if len(opts.excludeHosts) > 0 {
		fmt.Printf("Excluding hosts: %v\n", opts.excludeHosts)
	}
	if len(opts.includePaths) > 0 {
		fmt.Printf("Including paths: %v\n", opts.includePaths)
	}
	if len(opts.excludePaths) > 0 {
		fmt.Printf("Excluding paths: %v\n", opts.excludePaths)
	}

	if opts.db != "" {
		dbCfg, err := backend.ParseDatabaseURL(opts.db)
		if err != nil {
			// This should never happen since we already validated earlier
			panic(fmt.Sprintf("unexpected error parsing database URL: %v", err))
		}
		fmt.Printf("Capturing traffic to: %s database\n", dbCfg.Type)
	} else if opts.output != "" {
		fmt.Printf("Capturing traffic to: %s (%s format)\n", opts.output, opts.format)
	} else {
		fmt.Printf("Capturing traffic to stdout (%s format)\n", opts.format)
	}

	// Start metrics/health server if configured
	if opts.metricsPort > 0 {
		metricsAddr := fmt.Sprintf(":%d", opts.metricsPort)
		mux := http.NewServeMux()

		if health != nil {
			mux.Handle("/healthz", health.LivenessHandler())
			mux.Handle("/readyz", health.ReadinessHandler())
		}

		if obs != nil {
			mux.Handle("/metrics", obs.PrometheusHandler())
		}

		go func() {
			fmt.Printf("Metrics/health server on %s\n", metricsAddr)
			if err := http.ListenAndServe(metricsAddr, mux); err != nil {
				fmt.Fprintf(os.Stderr, "Metrics server error: %v\n", err)
			}
		}()

		// Mark as ready
		if health != nil {
			health.SetReady(true)
		}
	}

	fmt.Println()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")

		if health != nil {
			health.SetReady(false)
		}

		if harWriter != nil {
			fmt.Println("Writing HAR file...")
			if err := harWriter.Write(); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing HAR: %v\n", err)
			}
		}

		if trafficStore != nil {
			if asyncStore, ok := trafficStore.(backend.AsyncTrafficStore); ok {
				fmt.Println("Flushing traffic queue...")
				asyncStore.Flush(context.Background())
			}
			trafficStore.Close()
		}

		os.Exit(0)
	}()

	return p.ListenAndServe(addr)
}
