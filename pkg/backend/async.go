package backend

import (
	"context"
	"sync"
	"time"

	"github.com/grokify/omniproxy/pkg/capture"
)

// AsyncTrafficStoreWrapper wraps a TrafficStore with async buffered writes.
// This prevents the proxy from blocking on storage operations.
type AsyncTrafficStoreWrapper struct {
	store       TrafficStore
	queue       chan *capture.Record
	batchSize   int
	flushPeriod time.Duration
	workers     int
	metrics     Metrics

	wg       sync.WaitGroup
	stopChan chan struct{}
	stopped  bool
	mu       sync.RWMutex
}

// AsyncConfig configures the async wrapper.
type AsyncConfig struct {
	// QueueSize is the buffer size for pending records (default: 10000).
	QueueSize int

	// BatchSize is the number of records to batch before writing (default: 100).
	BatchSize int

	// FlushPeriod is how often to flush partial batches (default: 100ms).
	FlushPeriod time.Duration

	// Workers is the number of concurrent workers (default: 2).
	Workers int

	// Metrics for observability (optional).
	Metrics Metrics
}

// DefaultAsyncConfig returns default async configuration.
func DefaultAsyncConfig() *AsyncConfig {
	return &AsyncConfig{
		QueueSize:   10000,
		BatchSize:   100,
		FlushPeriod: 100 * time.Millisecond,
		Workers:     2,
	}
}

// NewAsyncTrafficStore wraps a TrafficStore with async buffered writes.
func NewAsyncTrafficStore(store TrafficStore, cfg *AsyncConfig) *AsyncTrafficStoreWrapper {
	if cfg == nil {
		cfg = DefaultAsyncConfig()
	}

	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 10000
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.FlushPeriod <= 0 {
		cfg.FlushPeriod = 100 * time.Millisecond
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 2
	}

	metrics := cfg.Metrics
	if metrics == nil {
		metrics = NoopMetrics{}
	}

	wrapper := &AsyncTrafficStoreWrapper{
		store:       store,
		queue:       make(chan *capture.Record, cfg.QueueSize),
		batchSize:   cfg.BatchSize,
		flushPeriod: cfg.FlushPeriod,
		workers:     cfg.Workers,
		metrics:     metrics,
		stopChan:    make(chan struct{}),
	}

	// Start workers
	for i := 0; i < cfg.Workers; i++ {
		wrapper.wg.Add(1)
		go wrapper.worker()
	}

	return wrapper
}

// Store queues a record for async storage.
// This method is non-blocking unless the queue is full.
func (w *AsyncTrafficStoreWrapper) Store(ctx context.Context, rec *capture.Record) error {
	w.mu.RLock()
	stopped := w.stopped
	w.mu.RUnlock()

	if stopped {
		return nil
	}

	select {
	case w.queue <- rec:
		w.metrics.SetQueueDepth(len(w.queue))
		return nil
	default:
		// Queue full - drop the record
		w.metrics.IncStoreError()
		return nil
	}
}

// StoreBatch queues multiple records for async storage.
func (w *AsyncTrafficStoreWrapper) StoreBatch(ctx context.Context, recs []*capture.Record) error {
	for _, rec := range recs {
		if err := w.Store(ctx, rec); err != nil {
			return err
		}
	}
	return nil
}

// QueueDepth returns the current number of records waiting to be stored.
func (w *AsyncTrafficStoreWrapper) QueueDepth() int {
	return len(w.queue)
}

// Flush blocks until all queued records are stored.
func (w *AsyncTrafficStoreWrapper) Flush(ctx context.Context) error {
	// Wait for queue to drain
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if len(w.queue) == 0 {
				return nil
			}
		}
	}
}

// Close stops all workers and flushes remaining records.
func (w *AsyncTrafficStoreWrapper) Close() error {
	w.mu.Lock()
	w.stopped = true
	w.mu.Unlock()

	// Signal workers to stop
	close(w.stopChan)

	// Wait for workers to finish
	w.wg.Wait()

	// Close underlying store
	return w.store.Close()
}

// Handle implements capture.Handler for use with the capturer.
func (w *AsyncTrafficStoreWrapper) Handle(rec *capture.Record) {
	_ = w.Store(context.Background(), rec)
}

// worker processes records from the queue.
func (w *AsyncTrafficStoreWrapper) worker() {
	defer w.wg.Done()

	batch := make([]*capture.Record, 0, w.batchSize)
	ticker := time.NewTicker(w.flushPeriod)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}

		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := w.store.StoreBatch(ctx, batch)
		cancel()

		if err != nil {
			w.metrics.IncStoreError()
		} else {
			for range batch {
				w.metrics.IncStoreSuccess()
			}
		}
		w.metrics.ObserveStoreDuration(time.Since(start))

		batch = batch[:0]
		w.metrics.SetQueueDepth(len(w.queue))
	}

	for {
		select {
		case rec := <-w.queue:
			batch = append(batch, rec)
			if len(batch) >= w.batchSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-w.stopChan:
			// Drain remaining records
			for {
				select {
				case rec := <-w.queue:
					batch = append(batch, rec)
					if len(batch) >= w.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// SamplingTrafficStore wraps a TrafficStore with sampling support.
// Used for high-volume deployments where capturing every request is not feasible.
type SamplingTrafficStore struct {
	store         TrafficStore
	sampleRate    float64
	alwaysCapture []string // Hosts to always capture
	neverCapture  []string // Hosts to never capture
	counter       uint64
	mu            sync.Mutex
}

// SamplingConfig configures traffic sampling.
type SamplingConfig struct {
	// SampleRate is the fraction of requests to capture (0.0 to 1.0).
	SampleRate float64

	// AlwaysCapture is a list of hosts to always capture (overrides SampleRate).
	AlwaysCapture []string

	// NeverCapture is a list of hosts to never capture.
	NeverCapture []string
}

// NewSamplingTrafficStore wraps a TrafficStore with sampling.
func NewSamplingTrafficStore(store TrafficStore, cfg *SamplingConfig) *SamplingTrafficStore {
	if cfg == nil {
		cfg = &SamplingConfig{SampleRate: 1.0}
	}

	return &SamplingTrafficStore{
		store:         store,
		sampleRate:    cfg.SampleRate,
		alwaysCapture: cfg.AlwaysCapture,
		neverCapture:  cfg.NeverCapture,
	}
}

// Store samples and potentially stores a record.
func (s *SamplingTrafficStore) Store(ctx context.Context, rec *capture.Record) error {
	if rec == nil {
		return nil
	}

	host := rec.Request.Host

	// Check never capture
	for _, h := range s.neverCapture {
		if matchHost(host, h) {
			return nil
		}
	}

	// Check always capture
	for _, h := range s.alwaysCapture {
		if matchHost(host, h) {
			return s.store.Store(ctx, rec)
		}
	}

	// Sample
	if !s.shouldSample() {
		return nil
	}

	return s.store.Store(ctx, rec)
}

// StoreBatch samples and stores multiple records.
func (s *SamplingTrafficStore) StoreBatch(ctx context.Context, recs []*capture.Record) error {
	for _, rec := range recs {
		if err := s.Store(ctx, rec); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the underlying store.
func (s *SamplingTrafficStore) Close() error {
	return s.store.Close()
}

// shouldSample returns true if the current request should be sampled.
func (s *SamplingTrafficStore) shouldSample() bool {
	if s.sampleRate >= 1.0 {
		return true
	}
	if s.sampleRate <= 0.0 {
		return false
	}

	s.mu.Lock()
	s.counter++
	counter := s.counter
	s.mu.Unlock()

	// Simple modulo-based sampling
	interval := uint64(1.0 / s.sampleRate)
	return counter%interval == 0
}

// matchHost checks if a host matches a pattern (supports * wildcard).
func matchHost(host, pattern string) bool {
	if pattern == "" {
		return false
	}

	// Simple wildcard matching
	if pattern[0] == '*' {
		suffix := pattern[1:]
		if len(suffix) > 0 && suffix[0] == '.' {
			suffix = suffix[1:]
		}
		return len(host) >= len(suffix) && host[len(host)-len(suffix):] == suffix
	}

	return host == pattern
}
