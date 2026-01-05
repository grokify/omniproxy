package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/grokify/mogo/log/slogutil"
)

func TestDaemonConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.PIDFile == "" {
		t.Error("PIDFile should not be empty")
	}
	if cfg.SocketPath == "" {
		t.Error("SocketPath should not be empty")
	}
	if cfg.ProxyPort != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.ProxyPort)
	}
}

func TestDaemonStatus(t *testing.T) {
	cfg := &Config{
		ProxyPort: 8888,
		Host:      "127.0.0.1",
		Database:  "sqlite://test.db",
		Version:   "1.0.0",
	}

	d := New(cfg)

	status := d.Status()
	if status.Running {
		t.Error("daemon should not be running initially")
	}
	if status.ProxyPort != 8888 {
		t.Errorf("expected port 8888, got %d", status.ProxyPort)
	}
	if status.Database != "sqlite://test.db" {
		t.Errorf("expected database sqlite://test.db, got %s", status.Database)
	}
}

func TestDaemonStartStop(t *testing.T) {
	// Create temp directory for test files
	tmpDir, err := os.MkdirTemp("", "omniproxyd-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{
		PIDFile:    filepath.Join(tmpDir, "test.pid"),
		SocketPath: filepath.Join(tmpDir, "test.sock"),
		ProxyPort:  18080,
		Host:       "127.0.0.1",
		Version:    "test",
	}

	d := New(cfg)

	// Set a simple start callback
	started := false
	stopped := false
	d.SetCallbacks(
		func() error { started = true; return nil },
		func() error { stopped = true; return nil },
		func() error { return nil },
	)

	ctx := context.Background()

	// Start daemon
	if err := d.Start(ctx); err != nil {
		t.Fatalf("failed to start daemon: %v", err)
	}

	if !started {
		t.Error("start callback was not called")
	}

	// Check PID file exists
	if _, err := os.Stat(cfg.PIDFile); os.IsNotExist(err) {
		t.Error("PID file was not created")
	}

	// Check socket exists
	if _, err := os.Stat(cfg.SocketPath); os.IsNotExist(err) {
		t.Error("socket file was not created")
	}

	// Check status
	status := d.Status()
	if !status.Running {
		t.Error("daemon should be running")
	}
	if status.PID == 0 {
		t.Error("PID should be set")
	}

	// Test control API via HTTP
	client := NewClient(cfg.SocketPath)

	apiStatus, err := client.GetStatus()
	if err != nil {
		t.Fatalf("failed to get status via API: %v", err)
	}
	if !apiStatus.Running {
		t.Error("API should report daemon as running")
	}

	// Stop daemon
	if err := d.Stop(ctx); err != nil {
		t.Fatalf("failed to stop daemon: %v", err)
	}

	if !stopped {
		t.Error("stop callback was not called")
	}

	// Check cleanup
	if _, err := os.Stat(cfg.PIDFile); !os.IsNotExist(err) {
		t.Error("PID file should be removed after stop")
	}
	if _, err := os.Stat(cfg.SocketPath); !os.IsNotExist(err) {
		t.Error("socket file should be removed after stop")
	}
}

func TestDaemonDoubleStart(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "omniproxyd-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{
		PIDFile:    filepath.Join(tmpDir, "test.pid"),
		SocketPath: filepath.Join(tmpDir, "test.sock"),
	}

	d := New(cfg)
	ctx := context.Background()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("first start failed: %v", err)
	}
	defer func() { _ = d.Stop(ctx) }()

	// Second start should fail
	if err := d.Start(ctx); err == nil {
		t.Error("second start should fail")
	}
}

func TestClientConnectionError(t *testing.T) {
	// Try to connect to non-existent socket
	client := NewClient("/tmp/nonexistent-socket-12345.sock")

	_, err := client.GetStatus()
	if err == nil {
		t.Error("expected error connecting to non-existent socket")
	}
}

func TestIsRunning(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "omniproxyd-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pidFile := filepath.Join(tmpDir, "test.pid")

	// Non-existent PID file
	running, _, err := IsRunning(pidFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if running {
		t.Error("should not be running with no PID file")
	}

	// Write current process PID (we know we're running)
	currentPID := os.Getpid()
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", currentPID)), 0600); err != nil {
		t.Fatalf("failed to write PID file: %v", err)
	}

	running, pid, err := IsRunning(pidFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !running {
		t.Error("current process should be running")
	}
	if pid != currentPID {
		t.Errorf("expected PID %d, got %d", currentPID, pid)
	}

	// Write invalid PID
	if err := os.WriteFile(pidFile, []byte("invalid"), 0600); err != nil {
		t.Fatalf("failed to write PID file: %v", err)
	}

	running, _, err = IsRunning(pidFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if running {
		t.Error("should not be running with invalid PID")
	}
}

func TestDaemonIncrementRequests(t *testing.T) {
	d := New(nil)

	if d.Status().Requests != 0 {
		t.Error("initial requests should be 0")
	}

	d.IncrementRequests()
	d.IncrementRequests()
	d.IncrementRequests()

	if d.Status().Requests != 3 {
		t.Errorf("expected 3 requests, got %d", d.Status().Requests)
	}
}

func TestDaemonControlAPI(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "omniproxyd-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{
		PIDFile:    filepath.Join(tmpDir, "test.pid"),
		SocketPath: filepath.Join(tmpDir, "test.sock"),
		ProxyPort:  18080,
		Version:    "test-1.0.0",
	}

	d := New(cfg)
	reloaded := false
	d.SetCallbacks(
		func() error { return nil },
		func() error { return nil },
		func() error { reloaded = true; return nil },
	)

	ctx := context.Background()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("failed to start daemon: %v", err)
	}
	defer func() {
		if err := d.Stop(ctx); err != nil {
			logger := slogutil.LoggerFromContext(ctx, slogutil.Null())
			logger.Error("failed to stop daemon", "error", err)
		}
	}()

	client := NewClient(cfg.SocketPath)

	// Test reload
	if err := client.Reload(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if !reloaded {
		t.Error("reload callback was not called")
	}

	// Test status with all fields
	status, err := client.GetStatus()
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}
	if status.Version != "test-1.0.0" {
		t.Errorf("expected version test-1.0.0, got %s", status.Version)
	}
	if status.ProxyPort != 18080 {
		t.Errorf("expected port 18080, got %d", status.ProxyPort)
	}
}

func TestDaemonControlAPIMethodNotAllowed(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "omniproxyd-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{
		PIDFile:    filepath.Join(tmpDir, "test.pid"),
		SocketPath: filepath.Join(tmpDir, "test.sock"),
	}

	d := New(cfg)
	ctx := context.Background()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("failed to start daemon: %v", err)
	}
	defer func() {
		if err := d.Stop(ctx); err != nil {
			logger := slogutil.LoggerFromContext(ctx, slogutil.Null())
			logger.Error("failed to stop daemon", "error", err)
		}
	}()

	// Create client that uses Unix socket
	client := NewClient(cfg.SocketPath)

	// GET requests should work for status
	status, err := client.GetStatus()
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}
	if !status.Running {
		t.Error("daemon should be running")
	}
}
