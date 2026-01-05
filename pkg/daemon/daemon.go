// Package daemon provides daemon lifecycle management for omniproxyd.
package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/grokify/omniproxy/pkg/backend"
)

// Default paths for daemon files.
var (
	DefaultDir        = filepath.Join(os.Getenv("HOME"), ".omniproxy")
	DefaultPIDFile    = filepath.Join(DefaultDir, "omniproxyd.pid")
	DefaultSocketPath = filepath.Join(DefaultDir, "omniproxyd.sock")
	DefaultLogFile    = filepath.Join(DefaultDir, "omniproxyd.log")
)

// Status represents the daemon status.
type Status struct {
	Running     bool      `json:"running"`
	PID         int       `json:"pid,omitempty"`
	StartTime   time.Time `json:"start_time,omitempty"`
	Uptime      string    `json:"uptime,omitempty"`
	ProxyPort   int       `json:"proxy_port,omitempty"`
	MetricsPort int       `json:"metrics_port,omitempty"`
	Version     string    `json:"version,omitempty"`
	Database    string    `json:"database,omitempty"`
	Requests    int64     `json:"requests,omitempty"`
}

// Config holds daemon configuration.
type Config struct {
	PIDFile    string
	SocketPath string
	LogFile    string
	ProxyPort  int
	Host       string
	Database   string
	Version    string
}

// DefaultConfig returns default daemon configuration.
func DefaultConfig() *Config {
	return &Config{
		PIDFile:    DefaultPIDFile,
		SocketPath: DefaultSocketPath,
		LogFile:    DefaultLogFile,
		ProxyPort:  8080,
		Host:       "127.0.0.1",
	}
}

// Daemon manages the proxy daemon lifecycle.
type Daemon struct {
	config    *Config
	startTime time.Time
	server    *http.Server
	listener  net.Listener
	requests  int64
	mu        sync.RWMutex
	stopCh    chan struct{}
	running   bool

	// Callbacks for proxy control
	onStart  func() error
	onStop   func() error
	onReload func() error

	// Optional traffic querier for /traffic endpoint
	trafficQuerier backend.TrafficQuerier
}

// New creates a new daemon instance.
func New(cfg *Config) *Daemon {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Daemon{
		config: cfg,
		stopCh: make(chan struct{}),
	}
}

// SetCallbacks sets the daemon lifecycle callbacks.
func (d *Daemon) SetCallbacks(onStart, onStop, onReload func() error) {
	d.onStart = onStart
	d.onStop = onStop
	d.onReload = onReload
}

// SetTrafficQuerier sets the traffic querier for the /traffic endpoint.
func (d *Daemon) SetTrafficQuerier(tq backend.TrafficQuerier) {
	d.trafficQuerier = tq
}

// Start starts the daemon control server.
func (d *Daemon) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return errors.New("daemon already running")
	}
	d.running = true
	d.startTime = time.Now()
	d.mu.Unlock()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(d.config.SocketPath), 0700); err != nil {
		return fmt.Errorf("failed to create daemon directory: %w", err)
	}

	// Remove stale socket
	os.Remove(d.config.SocketPath)

	// Create Unix socket listener
	listener, err := net.Listen("unix", d.config.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}
	d.listener = listener

	// Set socket permissions (owner only)
	if err := os.Chmod(d.config.SocketPath, 0600); err != nil {
		listener.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	// Write PID file
	if err := d.writePIDFile(); err != nil {
		listener.Close()
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	// Setup HTTP server for control API
	mux := http.NewServeMux()
	mux.HandleFunc("/status", d.handleStatus)
	mux.HandleFunc("/stop", d.handleStop)
	mux.HandleFunc("/reload", d.handleReload)
	mux.HandleFunc("/stats", d.handleStats)
	mux.HandleFunc("/traffic", d.handleTraffic)
	mux.HandleFunc("/traffic/", d.handleTrafficDetail)

	d.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start the proxy
	if d.onStart != nil {
		if err := d.onStart(); err != nil {
			d.cleanup()
			return fmt.Errorf("failed to start proxy: %w", err)
		}
	}

	// Serve control API
	go func() {
		if err := d.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "control server error: %v\n", err)
		}
	}()

	return nil
}

// Stop stops the daemon.
func (d *Daemon) Stop(ctx context.Context) error {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return errors.New("daemon not running")
	}
	d.mu.Unlock()

	// Stop the proxy
	if d.onStop != nil {
		if err := d.onStop(); err != nil {
			return fmt.Errorf("failed to stop proxy: %w", err)
		}
	}

	// Shutdown control server
	if d.server != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := d.server.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "control server shutdown error: %v\n", err)
		}
	}

	d.cleanup()

	d.mu.Lock()
	d.running = false
	close(d.stopCh)
	d.mu.Unlock()

	return nil
}

// Wait blocks until the daemon stops.
func (d *Daemon) Wait() {
	<-d.stopCh
}

// IncrementRequests increments the request counter.
func (d *Daemon) IncrementRequests() {
	d.mu.Lock()
	d.requests++
	d.mu.Unlock()
}

// Status returns the current daemon status.
func (d *Daemon) Status() *Status {
	d.mu.RLock()
	defer d.mu.RUnlock()

	status := &Status{
		Running:   d.running,
		Version:   d.config.Version,
		ProxyPort: d.config.ProxyPort,
		Database:  d.config.Database,
		Requests:  d.requests,
	}

	if d.running {
		status.PID = os.Getpid()
		status.StartTime = d.startTime
		status.Uptime = time.Since(d.startTime).Round(time.Second).String()
	}

	return status
}

func (d *Daemon) writePIDFile() error {
	if err := os.MkdirAll(filepath.Dir(d.config.PIDFile), 0700); err != nil {
		return err
	}
	return os.WriteFile(d.config.PIDFile, []byte(strconv.Itoa(os.Getpid())), 0600)
}

func (d *Daemon) cleanup() {
	os.Remove(d.config.PIDFile)
	os.Remove(d.config.SocketPath)
}

func (d *Daemon) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(d.Status()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (d *Daemon) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "stopping"}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Stop in background
	go func() {
		time.Sleep(100 * time.Millisecond) // Allow response to be sent
		if err := d.Stop(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "daemon stop error: %v\n", err)
		}
	}()
}

func (d *Daemon) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if d.onReload != nil {
		if err := d.onReload(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "reloaded"}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (d *Daemon) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(d.Status()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// TrafficResponse is the response format for the /traffic endpoint.
type TrafficResponse struct {
	Records []*backend.TrafficRecord `json:"records"`
	Total   int64                    `json:"total"`
	Limit   int                      `json:"limit"`
	Offset  int                      `json:"offset"`
}

func (d *Daemon) handleTraffic(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if traffic querier is available
	if d.trafficQuerier == nil {
		http.Error(w, `{"error":"traffic querying not available (no database configured)"}`, http.StatusServiceUnavailable)
		return
	}

	// Parse query parameters
	q := r.URL.Query()

	filter := &backend.TrafficFilter{
		Limit:  100, // Default limit
		Offset: 0,
		Desc:   true, // Newest first by default
	}

	// Limit
	if limitStr := q.Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit <= 1000 {
			filter.Limit = limit
		}
	}

	// Offset
	if offsetStr := q.Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			filter.Offset = offset
		}
	}

	// Host filter
	if host := q.Get("host"); host != "" {
		filter.Hosts = []string{host}
	}

	// Method filter
	if method := q.Get("method"); method != "" {
		filter.Methods = []string{method}
	}

	// Status code filter
	if statusStr := q.Get("status"); statusStr != "" {
		if status, err := strconv.Atoi(statusStr); err == nil {
			filter.StatusCodes = []int{status}
		}
	}

	// Min status (e.g., 400 for errors only)
	if minStatusStr := q.Get("min_status"); minStatusStr != "" {
		if minStatus, err := strconv.Atoi(minStatusStr); err == nil {
			filter.MinStatus = minStatus
		}
	}

	// Query traffic records
	ctx := r.Context()
	records, err := d.trafficQuerier.Query(ctx, filter)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to query traffic: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Get total count (without limit/offset)
	countFilter := &backend.TrafficFilter{
		Hosts:       filter.Hosts,
		Methods:     filter.Methods,
		StatusCodes: filter.StatusCodes,
		MinStatus:   filter.MinStatus,
	}
	total, _ := d.trafficQuerier.Count(ctx, countFilter)

	response := TrafficResponse{
		Records: records,
		Total:   total,
		Limit:   filter.Limit,
		Offset:  filter.Offset,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (d *Daemon) handleTrafficDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if traffic querier is available
	if d.trafficQuerier == nil {
		http.Error(w, `{"error":"traffic querying not available (no database configured)"}`, http.StatusServiceUnavailable)
		return
	}

	// Extract ID from path: /traffic/{id}
	path := r.URL.Path
	if len(path) <= len("/traffic/") {
		http.Error(w, `{"error":"missing traffic ID"}`, http.StatusBadRequest)
		return
	}
	id := path[len("/traffic/"):]
	if id == "" {
		http.Error(w, `{"error":"missing traffic ID"}`, http.StatusBadRequest)
		return
	}

	// Get traffic detail
	ctx := r.Context()
	detail, err := d.trafficQuerier.GetByID(ctx, id)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to get traffic: %s"}`, err.Error()), http.StatusNotFound)
		return
	}

	if err := json.NewEncoder(w).Encode(detail); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Client provides methods to communicate with the daemon.
type Client struct {
	socketPath string
	httpClient *http.Client
}

// NewClient creates a new daemon client.
func NewClient(socketPath string) *Client {
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	return &Client{
		socketPath: socketPath,
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
			Timeout: 5 * time.Second,
		},
	}
}

// GetStatus retrieves the daemon status.
func (c *Client) GetStatus() (*Status, error) {
	resp, err := c.httpClient.Get("http://unix/status")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	var status Status
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode status: %w", err)
	}

	return &status, nil
}

// Stop sends a stop request to the daemon.
func (c *Client) Stop() error {
	resp, err := c.httpClient.Post("http://unix/stop", "application/json", nil)
	if err != nil {
		return fmt.Errorf("failed to stop daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stop failed: %s", string(body))
	}

	return nil
}

// Reload sends a reload request to the daemon.
func (c *Client) Reload() error {
	resp, err := c.httpClient.Post("http://unix/reload", "application/json", nil)
	if err != nil {
		return fmt.Errorf("failed to reload daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("reload failed: %s", string(body))
	}

	return nil
}

// IsRunning checks if the daemon is running.
func IsRunning(pidFile string) (bool, int, error) {
	if pidFile == "" {
		pidFile = DefaultPIDFile
	}

	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		return false, 0, err
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return false, 0, nil
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, 0, nil
	}

	// Send signal 0 to check if process is alive
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// Process doesn't exist, clean up stale PID file
		os.Remove(pidFile)
		return false, 0, nil
	}

	return true, pid, nil
}

// StartBackground starts the daemon in the background.
func StartBackground(args []string) error {
	// Find the current executable
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable: %w", err)
	}

	// Prepare log file
	if err := os.MkdirAll(filepath.Dir(DefaultLogFile), 0700); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	logFile, err := os.OpenFile(DefaultLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Start the process in the background
	cmd := exec.Command(executable, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session (detach from terminal)
	}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	logFile.Close()

	// Wait a bit and check if it started successfully
	time.Sleep(500 * time.Millisecond)

	running, pid, err := IsRunning("")
	if err != nil {
		return fmt.Errorf("failed to check daemon status: %w", err)
	}

	if !running {
		return errors.New("daemon failed to start, check log file")
	}

	fmt.Printf("Daemon started with PID %d\n", pid)
	return nil
}

// StopByPID stops the daemon by sending SIGTERM to the PID.
func StopByPID(pidFile string) error {
	if pidFile == "" {
		pidFile = DefaultPIDFile
	}

	running, pid, err := IsRunning(pidFile)
	if err != nil {
		return err
	}

	if !running {
		return errors.New("daemon is not running")
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send signal: %w", err)
	}

	// Wait for process to exit
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		running, _, _ := IsRunning(pidFile)
		if !running {
			fmt.Printf("Daemon (PID %d) stopped\n", pid)
			return nil
		}
	}

	return errors.New("daemon did not stop in time, try SIGKILL")
}
