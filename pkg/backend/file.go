package backend

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"

	"github.com/grokify/omniproxy/pkg/capture"
)

// FileTrafficStore writes traffic records to a file in NDJSON format.
// This is the simplest backend, suitable for laptop mode.
type FileTrafficStore struct {
	mu      sync.Mutex
	writer  io.Writer
	file    *os.File // nil if using stdout or provided writer
	format  Format
	metrics Metrics
}

// Format specifies the output format for file-based storage.
type Format string

const (
	FormatNDJSON Format = "ndjson" // Newline-delimited JSON (default)
	FormatJSON   Format = "json"   // Pretty-printed JSON
)

// FileTrafficStoreConfig configures a FileTrafficStore.
type FileTrafficStoreConfig struct {
	// Path is the file path to write to. Empty means stdout.
	Path string

	// Writer is an alternative to Path for custom output destinations.
	// If set, Path is ignored.
	Writer io.Writer

	// Format specifies the output format (default: ndjson).
	Format Format

	// Metrics for observability (optional).
	Metrics Metrics
}

// NewFileTrafficStore creates a new file-based traffic store.
func NewFileTrafficStore(cfg *FileTrafficStoreConfig) (*FileTrafficStore, error) {
	if cfg == nil {
		cfg = &FileTrafficStoreConfig{}
	}

	store := &FileTrafficStore{
		format:  cfg.Format,
		metrics: cfg.Metrics,
	}

	if store.format == "" {
		store.format = FormatNDJSON
	}

	if store.metrics == nil {
		store.metrics = NoopMetrics{}
	}

	// Determine writer
	if cfg.Writer != nil {
		store.writer = cfg.Writer
	} else if cfg.Path != "" {
		f, err := os.OpenFile(cfg.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		store.file = f
		store.writer = f
	} else {
		store.writer = os.Stdout
	}

	return store, nil
}

// Store writes a single traffic record to the file.
func (s *FileTrafficStore) Store(ctx context.Context, rec *capture.Record) error {
	if rec == nil {
		return nil
	}

	var data []byte
	var err error

	switch s.format {
	case FormatJSON:
		data, err = json.MarshalIndent(rec, "", "  ")
	default: // FormatNDJSON
		data, err = json.Marshal(rec)
	}

	if err != nil {
		s.metrics.IncStoreError()
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.writer.Write(data); err != nil {
		s.metrics.IncStoreError()
		return err
	}

	if _, err := s.writer.Write([]byte("\n")); err != nil {
		s.metrics.IncStoreError()
		return err
	}

	s.metrics.IncStoreSuccess()
	return nil
}

// StoreBatch writes multiple traffic records to the file.
func (s *FileTrafficStore) StoreBatch(ctx context.Context, recs []*capture.Record) error {
	for _, rec := range recs {
		if err := s.Store(ctx, rec); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the file if one was opened.
func (s *FileTrafficStore) Close() error {
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}

// Handle implements capture.Handler for use with the capturer.
func (s *FileTrafficStore) Handle(rec *capture.Record) {
	_ = s.Store(context.Background(), rec)
}

// DiscardTrafficStore is a TrafficStore that discards all records.
// Useful for testing or when traffic capture is disabled.
type DiscardTrafficStore struct{}

func (DiscardTrafficStore) Store(ctx context.Context, rec *capture.Record) error { return nil }
func (DiscardTrafficStore) StoreBatch(ctx context.Context, recs []*capture.Record) error {
	return nil
}
func (DiscardTrafficStore) Close() error { return nil }
