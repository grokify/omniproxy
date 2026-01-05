package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grokify/omniproxy/pkg/backend"
	"github.com/grokify/omniproxy/pkg/ca"
	"github.com/grokify/omniproxy/pkg/capture"
	"github.com/grokify/omniproxy/pkg/daemon"
	"github.com/grokify/omniproxy/pkg/observability"
	"github.com/grokify/omniproxy/pkg/proxy"
	"github.com/spf13/cobra"
)

type daemonOptions struct {
	// Control options
	foreground bool
	pidFile    string
	socketPath string
	logFile    string

	// Proxy options (same as serve)
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

	includeHosts   []string
	excludeHosts   []string
	includePaths   []string
	excludePaths   []string
	includeMethods []string
	excludeMethods []string

	upstream string

	metricsPort   int
	enableMetrics bool

	db string

	asyncQueue     int
	asyncBatchSize int
	asyncWorkers   int
}

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the omniproxyd daemon",
		Long: `Manage the omniproxyd background daemon.

The daemon runs the proxy in the background and can be controlled via
Unix socket commands. This is useful for integration with PlexusDesktop
or other management tools.

Examples:
  # Start daemon in background
  omniproxy daemon start --db sqlite://~/.omniproxy/traffic.db

  # Check daemon status
  omniproxy daemon status

  # Stop daemon
  omniproxy daemon stop

  # Start in foreground (for debugging)
  omniproxy daemon start --foreground`,
	}

	cmd.AddCommand(
		newDaemonStartCmd(),
		newDaemonStopCmd(),
		newDaemonStatusCmd(),
		newDaemonReloadCmd(),
	)

	return cmd
}

func newDaemonStartCmd() *cobra.Command {
	opts := &daemonOptions{}

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the daemon",
		Long: `Start the omniproxyd daemon.

By default, the daemon starts in the background. Use --foreground to run
in the foreground (useful for debugging or when managed by systemd).

The daemon exposes a Unix socket API at ~/.omniproxy/omniproxyd.sock
for control operations (status, stop, reload).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemonStart(opts)
		},
	}

	// Control options
	cmd.Flags().BoolVarP(&opts.foreground, "foreground", "f", false, "Run in foreground (don't daemonize)")
	cmd.Flags().StringVar(&opts.pidFile, "pid-file", daemon.DefaultPIDFile, "PID file path")
	cmd.Flags().StringVar(&opts.socketPath, "socket", daemon.DefaultSocketPath, "Unix socket path")
	cmd.Flags().StringVar(&opts.logFile, "log-file", daemon.DefaultLogFile, "Log file path")

	// Proxy options (same as serve command)
	cmd.Flags().IntVarP(&opts.port, "port", "p", 8080, "Port to listen on")
	cmd.Flags().StringVar(&opts.host, "host", "127.0.0.1", "Host to bind to")
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "Enable verbose logging")
	cmd.Flags().BoolVar(&opts.enableMITM, "mitm", true, "Enable HTTPS MITM interception")

	cmd.Flags().StringVar(&opts.caPath, "ca-cert", "", "Path to CA certificate")
	cmd.Flags().StringVar(&opts.keyPath, "ca-key", "", "Path to CA private key")

	cmd.Flags().StringVarP(&opts.output, "output", "o", "", "Output file for captured traffic")
	cmd.Flags().StringVar(&opts.format, "format", "ndjson", "Output format: ndjson, json, har, ir")
	cmd.Flags().StringSliceVar(&opts.filterHeader, "filter-header", nil, "Headers to filter")
	cmd.Flags().BoolVar(&opts.skipBinary, "skip-binary", true, "Skip binary content")

	cmd.Flags().StringSliceVar(&opts.skipHosts, "skip-host", nil, "Hosts to skip MITM for")
	cmd.Flags().StringSliceVar(&opts.includeHosts, "include-host", nil, "Only capture these hosts")
	cmd.Flags().StringSliceVar(&opts.excludeHosts, "exclude-host", nil, "Exclude these hosts")
	cmd.Flags().StringSliceVar(&opts.includePaths, "include-path", nil, "Only capture these paths")
	cmd.Flags().StringSliceVar(&opts.excludePaths, "exclude-path", nil, "Exclude these paths")
	cmd.Flags().StringSliceVar(&opts.includeMethods, "include-method", nil, "Only capture these methods")
	cmd.Flags().StringSliceVar(&opts.excludeMethods, "exclude-method", nil, "Exclude these methods")

	cmd.Flags().StringVar(&opts.upstream, "upstream", "", "Upstream proxy URL")

	cmd.Flags().IntVar(&opts.metricsPort, "metrics-port", 9090, "Metrics/health port")
	cmd.Flags().BoolVar(&opts.enableMetrics, "metrics", true, "Enable Prometheus metrics")

	cmd.Flags().StringVar(&opts.db, "db", "", "Database URL (sqlite://... or postgres://...)")

	cmd.Flags().IntVar(&opts.asyncQueue, "async-queue", 10000, "Async queue size")
	cmd.Flags().IntVar(&opts.asyncBatchSize, "async-batch", 100, "Async batch size")
	cmd.Flags().IntVar(&opts.asyncWorkers, "async-workers", 2, "Async workers")

	return cmd
}

func newDaemonStopCmd() *cobra.Command {
	var pidFile string
	var socketPath string

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Try graceful stop via socket first
			client := daemon.NewClient(socketPath)
			if err := client.Stop(); err == nil {
				// Wait for daemon to stop
				for i := 0; i < 30; i++ {
					time.Sleep(100 * time.Millisecond)
					running, _, _ := daemon.IsRunning(pidFile)
					if !running {
						fmt.Println("Daemon stopped")
						return nil
					}
				}
			}

			// Fall back to PID-based stop
			return daemon.StopByPID(pidFile)
		},
	}

	cmd.Flags().StringVar(&pidFile, "pid-file", daemon.DefaultPIDFile, "PID file path")
	cmd.Flags().StringVar(&socketPath, "socket", daemon.DefaultSocketPath, "Unix socket path")

	return cmd
}

func newDaemonStatusCmd() *cobra.Command {
	var pidFile string
	var socketPath string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check if running via PID
			running, pid, err := daemon.IsRunning(pidFile)
			if err != nil {
				return fmt.Errorf("failed to check status: %w", err)
			}

			if !running {
				if jsonOutput {
					fmt.Println(`{"running":false}`)
				} else {
					fmt.Println("Daemon is not running")
				}
				return nil
			}

			// Get detailed status via socket
			client := daemon.NewClient(socketPath)
			status, err := client.GetStatus()
			if err != nil {
				// Socket not responding, but PID exists
				if jsonOutput {
					fmt.Printf(`{"running":true,"pid":%d,"socket_error":true}`, pid)
					fmt.Println()
				} else {
					fmt.Printf("Daemon running (PID %d) but socket not responding\n", pid)
				}
				return nil
			}

			if jsonOutput {
				fmt.Printf(`{"running":true,"pid":%d,"uptime":"%s","proxy_port":%d,"metrics_port":%d,"requests":%d,"database":"%s"}`,
					status.PID, status.Uptime, status.ProxyPort, status.MetricsPort, status.Requests, status.Database)
				fmt.Println()
			} else {
				fmt.Printf("Daemon Status:\n")
				fmt.Printf("  Running:      yes\n")
				fmt.Printf("  PID:          %d\n", status.PID)
				fmt.Printf("  Uptime:       %s\n", status.Uptime)
				fmt.Printf("  Proxy Port:   %d\n", status.ProxyPort)
				if status.MetricsPort > 0 {
					fmt.Printf("  Metrics Port: %d\n", status.MetricsPort)
				}
				fmt.Printf("  Requests:     %d\n", status.Requests)
				if status.Database != "" {
					fmt.Printf("  Database:     %s\n", status.Database)
				}
				if status.Version != "" {
					fmt.Printf("  Version:      %s\n", status.Version)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&pidFile, "pid-file", daemon.DefaultPIDFile, "PID file path")
	cmd.Flags().StringVar(&socketPath, "socket", daemon.DefaultSocketPath, "Unix socket path")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	return cmd
}

func newDaemonReloadCmd() *cobra.Command {
	var socketPath string

	cmd := &cobra.Command{
		Use:   "reload",
		Short: "Reload daemon configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := daemon.NewClient(socketPath)
			if err := client.Reload(); err != nil {
				return err
			}
			fmt.Println("Configuration reloaded")
			return nil
		},
	}

	cmd.Flags().StringVar(&socketPath, "socket", daemon.DefaultSocketPath, "Unix socket path")

	return cmd
}

func runDaemonStart(opts *daemonOptions) error {
	// Check if already running
	running, pid, err := daemon.IsRunning(opts.pidFile)
	if err != nil {
		return fmt.Errorf("failed to check daemon status: %w", err)
	}
	if running {
		return fmt.Errorf("daemon already running with PID %d", pid)
	}

	// If not foreground, start in background
	if !opts.foreground {
		args := buildDaemonArgs(opts)
		return daemon.StartBackground(args)
	}

	// Run in foreground
	return runDaemonForeground(opts)
}

func buildDaemonArgs(opts *daemonOptions) []string {
	args := []string{"daemon", "start", "--foreground"}

	args = append(args, "--port", fmt.Sprintf("%d", opts.port))
	args = append(args, "--host", opts.host)
	args = append(args, "--pid-file", opts.pidFile)
	args = append(args, "--socket", opts.socketPath)
	args = append(args, "--log-file", opts.logFile)

	if opts.verbose {
		args = append(args, "--verbose")
	}
	if !opts.enableMITM {
		args = append(args, "--mitm=false")
	}
	if opts.caPath != "" {
		args = append(args, "--ca-cert", opts.caPath)
	}
	if opts.keyPath != "" {
		args = append(args, "--ca-key", opts.keyPath)
	}
	if opts.output != "" {
		args = append(args, "--output", opts.output)
	}
	if opts.format != "ndjson" {
		args = append(args, "--format", opts.format)
	}
	if opts.db != "" {
		args = append(args, "--db", opts.db)
	}
	if opts.upstream != "" {
		args = append(args, "--upstream", opts.upstream)
	}
	if opts.metricsPort > 0 {
		args = append(args, "--metrics-port", fmt.Sprintf("%d", opts.metricsPort))
	}
	if opts.enableMetrics {
		args = append(args, "--metrics")
	}

	for _, h := range opts.skipHosts {
		args = append(args, "--skip-host", h)
	}
	for _, h := range opts.includeHosts {
		args = append(args, "--include-host", h)
	}
	for _, h := range opts.excludeHosts {
		args = append(args, "--exclude-host", h)
	}
	for _, p := range opts.includePaths {
		args = append(args, "--include-path", p)
	}
	for _, p := range opts.excludePaths {
		args = append(args, "--exclude-path", p)
	}
	for _, m := range opts.includeMethods {
		args = append(args, "--include-method", m)
	}
	for _, m := range opts.excludeMethods {
		args = append(args, "--exclude-method", m)
	}

	return args
}

func runDaemonForeground(opts *daemonOptions) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup observability
	var obs *observability.Provider
	var health *observability.HealthChecker
	var err error

	if opts.metricsPort > 0 || opts.enableMetrics {
		obs, err = observability.NewProvider(&observability.Config{
			ServiceName:      "omniproxyd",
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

	// Setup CA
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
	capturerCfg := capture.DefaultConfig()
	capturerCfg.Filter = filter
	capturerCfg.SkipBinary = opts.skipBinary

	if len(opts.filterHeader) > 0 {
		capturerCfg.FilterHeaders = append(capturerCfg.FilterHeaders, opts.filterHeader...)
	}

	capturer := capture.NewCapturer(capturerCfg)

	// Setup traffic store
	var trafficStore backend.TrafficStore
	var trafficQuerier backend.TrafficQuerier
	var backendMetrics backend.Metrics

	if obs != nil {
		backendMetrics = observability.NewBackendMetrics(obs.Metrics)
	}

	dbDisplay := "none"
	if opts.db != "" {
		dbCfg, err := backend.ParseDatabaseURL(opts.db)
		if err != nil {
			return fmt.Errorf("invalid database URL: %w", err)
		}
		dbDisplay = dbCfg.String()

		dbStore, err := backend.NewDatabaseTrafficStore(ctx, &backend.DatabaseTrafficStoreConfig{
			DatabaseURL: opts.db,
			ProxyName:   "omniproxyd",
			Metrics:     backendMetrics,
			Debug:       opts.verbose,
		})
		if err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}

		// Store reference for traffic querying via daemon API
		trafficQuerier = dbStore

		trafficStore = backend.NewAsyncTrafficStore(dbStore, &backend.AsyncConfig{
			QueueSize: opts.asyncQueue,
			BatchSize: opts.asyncBatchSize,
			Workers:   opts.asyncWorkers,
			Metrics:   backendMetrics,
		})

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

	// Create daemon
	daemonCfg := &daemon.Config{
		PIDFile:    opts.pidFile,
		SocketPath: opts.socketPath,
		LogFile:    opts.logFile,
		ProxyPort:  opts.port,
		Host:       opts.host,
		Database:   dbDisplay,
		Version:    version,
	}

	d := daemon.New(daemonCfg)

	// Wire traffic querier for /traffic endpoint
	if trafficQuerier != nil {
		d.SetTrafficQuerier(trafficQuerier)
	}

	// Set callbacks
	var proxyErrCh chan error
	d.SetCallbacks(
		func() error {
			// onStart - start proxy in goroutine
			proxyErrCh = make(chan error, 1)
			go func() {
				addr := fmt.Sprintf("%s:%d", opts.host, opts.port)
				proxyErrCh <- p.ListenAndServe(addr)
			}()
			return nil
		},
		func() error {
			// onStop - stop proxy
			if trafficStore != nil {
				if asyncStore, ok := trafficStore.(backend.AsyncTrafficStore); ok {
					asyncStore.Flush(context.Background())
				}
				trafficStore.Close()
			}
			return nil
		},
		func() error {
			// onReload - reload config (not implemented yet)
			return nil
		},
	)

	// Start daemon control server
	if err := d.Start(ctx); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	fmt.Printf("omniproxyd started\n")
	fmt.Printf("  Proxy:   %s:%d\n", opts.host, opts.port)
	if opts.metricsPort > 0 {
		fmt.Printf("  Metrics: :%d\n", opts.metricsPort)
	}
	fmt.Printf("  Socket:  %s\n", opts.socketPath)
	fmt.Printf("  PID:     %s\n", opts.pidFile)
	if opts.db != "" {
		fmt.Printf("  Database: %s\n", dbDisplay)
	}

	// Start metrics server
	if opts.metricsPort > 0 && obs != nil {
		go func() {
			mux := observability.NewHealthMux(health, obs)
			addr := fmt.Sprintf(":%d", opts.metricsPort)
			if err := observability.ListenAndServe(addr, mux); err != nil {
				fmt.Fprintf(os.Stderr, "metrics server error: %v\n", err)
			}
		}()

		if health != nil {
			health.SetReady(true)
		}
	}

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		select {
		case sig := <-sigChan:
			switch sig {
			case syscall.SIGHUP:
				fmt.Println("Reloading configuration...")
				// Reload logic would go here
			case syscall.SIGINT, syscall.SIGTERM:
				fmt.Println("\nShutting down daemon...")
				if health != nil {
					health.SetReady(false)
				}
				if err := d.Stop(ctx); err != nil {
					fmt.Fprintf(os.Stderr, "daemon stop error: %v\n", err)
				}
				return nil
			}
		case err := <-proxyErrCh:
			if err != nil {
				return fmt.Errorf("proxy error: %w", err)
			}
			return nil
		}
	}
}
