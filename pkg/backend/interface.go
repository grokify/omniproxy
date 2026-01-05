// Package backend provides pluggable interfaces for OmniProxy storage and caching.
//
// OmniProxy supports three deployment modes with different backend implementations:
//
//   - Laptop Mode: File-based traffic storage, in-memory cert cache
//   - Team Mode: SQLite database, in-memory cert cache
//   - Production Mode: Kafka traffic pipeline, Redis cert cache, PostgreSQL config
//
// The same OmniProxy binary works in all modes - just configure which backends to use.
package backend

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/grokify/omniproxy/pkg/capture"
)

// TrafficStore is the interface for storing captured HTTP traffic.
// Implementations must be safe for concurrent use.
type TrafficStore interface {
	// Store saves a single traffic record.
	Store(ctx context.Context, rec *capture.Record) error

	// StoreBatch saves multiple traffic records efficiently.
	StoreBatch(ctx context.Context, recs []*capture.Record) error

	// Close releases any resources held by the store.
	Close() error
}

// TrafficQuerier is an optional interface for querying stored traffic.
// Not all TrafficStore implementations support querying (e.g., Kafka producer).
type TrafficQuerier interface {
	// Query returns traffic records matching the filter.
	Query(ctx context.Context, filter *TrafficFilter) ([]*TrafficRecord, error)

	// GetByID returns full traffic details for a single record.
	GetByID(ctx context.Context, id string) (*TrafficDetail, error)

	// Count returns the number of records matching the filter.
	Count(ctx context.Context, filter *TrafficFilter) (int64, error)

	// Stats returns aggregate statistics.
	Stats(ctx context.Context, filter *TrafficFilter) (*TrafficStats, error)
}

// TrafficFilter specifies criteria for querying traffic records.
type TrafficFilter struct {
	// Time range
	StartTime time.Time
	EndTime   time.Time

	// Request filters
	Hosts   []string // Filter by host (supports wildcards)
	Paths   []string // Filter by path (supports wildcards)
	Methods []string // Filter by HTTP method

	// Response filters
	StatusCodes []int // Filter by status code
	MinStatus   int   // Minimum status code (e.g., 400 for errors)
	MaxStatus   int   // Maximum status code

	// Pagination
	Limit  int
	Offset int

	// Sorting
	OrderBy string // Field to sort by
	Desc    bool   // Sort descending
}

// TrafficRecord represents a stored traffic record for querying (list view).
type TrafficRecord struct {
	ID        string
	Method    string
	URL       string
	Host      string
	Path      string
	Status    int
	Duration  time.Duration
	StartTime time.Time
	Error     string
}

// TrafficDetail represents full traffic details for a single record (detail view).
type TrafficDetail struct {
	TrafficRecord

	// Request details
	Scheme             string              `json:"scheme"`
	Query              string              `json:"query,omitempty"`
	RequestHeaders     map[string][]string `json:"request_headers,omitempty"`
	RequestBody        string              `json:"request_body,omitempty"`
	RequestBodySize    int64               `json:"request_body_size"`
	RequestIsBinary    bool                `json:"request_is_binary"`
	RequestContentType string              `json:"request_content_type,omitempty"`

	// Response details
	StatusText          string              `json:"status_text,omitempty"`
	ResponseHeaders     map[string][]string `json:"response_headers,omitempty"`
	ResponseBody        string              `json:"response_body,omitempty"`
	ResponseBodySize    int64               `json:"response_body_size"`
	ResponseIsBinary    bool                `json:"response_is_binary"`
	ResponseContentType string              `json:"response_content_type,omitempty"`

	// Additional timing
	TTFBMs *float64 `json:"ttfb_ms,omitempty"`

	// Metadata
	ClientIP string   `json:"client_ip,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

// TrafficStats contains aggregate traffic statistics.
type TrafficStats struct {
	TotalRequests    int64
	TotalErrors      int64
	AvgDurationMs    float64
	P50DurationMs    float64
	P95DurationMs    float64
	P99DurationMs    float64
	UniqueHosts      int64
	RequestsByMethod map[string]int64
	RequestsByStatus map[int]int64
}

// CertCache is the interface for caching generated TLS certificates.
// Implementations must be safe for concurrent use.
type CertCache interface {
	// Get retrieves a certificate for the given hostname.
	// Returns nil, false if not found or expired.
	Get(host string) (*tls.Certificate, bool)

	// Set stores a certificate for the given hostname.
	Set(host string, cert *tls.Certificate)

	// Delete removes a certificate from the cache.
	Delete(host string)

	// Clear removes all certificates from the cache.
	Clear()

	// Close releases any resources held by the cache.
	Close() error
}

// ConfigStore is the interface for storing proxy configuration.
// Used in team and production modes for persistent configuration.
type ConfigStore interface {
	// GetProxyConfig retrieves configuration for a proxy instance.
	GetProxyConfig(ctx context.Context, proxyID string) (*ProxyConfig, error)

	// SaveProxyConfig stores configuration for a proxy instance.
	SaveProxyConfig(ctx context.Context, cfg *ProxyConfig) error

	// ListProxyConfigs returns all proxy configurations for an org.
	ListProxyConfigs(ctx context.Context, orgID string) ([]*ProxyConfig, error)

	// DeleteProxyConfig removes a proxy configuration.
	DeleteProxyConfig(ctx context.Context, proxyID string) error

	// Close releases any resources held by the store.
	Close() error
}

// ProxyConfig represents stored proxy configuration.
type ProxyConfig struct {
	ID           string
	OrgID        string
	Name         string
	Slug         string
	Mode         string // "forward", "reverse", "transparent"
	Port         int
	Host         string
	MITMEnabled  bool
	SkipHosts    []string
	IncludeHosts []string
	ExcludeHosts []string
	IncludePaths []string
	ExcludePaths []string
	Upstream     string
	SkipBinary   bool
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// AsyncTrafficStore wraps a TrafficStore with async buffered writes.
// This is used in production mode to prevent blocking the proxy on storage.
type AsyncTrafficStore interface {
	TrafficStore

	// QueueDepth returns the current number of records waiting to be stored.
	QueueDepth() int

	// Flush blocks until all queued records are stored.
	Flush(ctx context.Context) error
}

// Metrics provides observability for backend operations.
type Metrics interface {
	// IncStoreSuccess increments successful store counter.
	IncStoreSuccess()

	// IncStoreError increments store error counter.
	IncStoreError()

	// ObserveStoreDuration records store operation duration.
	ObserveStoreDuration(d time.Duration)

	// IncCacheHit increments cache hit counter.
	IncCacheHit()

	// IncCacheMiss increments cache miss counter.
	IncCacheMiss()

	// SetQueueDepth sets the current queue depth gauge.
	SetQueueDepth(n int)
}

// NoopMetrics is a Metrics implementation that does nothing.
type NoopMetrics struct{}

func (NoopMetrics) IncStoreSuccess()                   {}
func (NoopMetrics) IncStoreError()                     {}
func (NoopMetrics) ObserveStoreDuration(time.Duration) {}
func (NoopMetrics) IncCacheHit()                       {}
func (NoopMetrics) IncCacheMiss()                      {}
func (NoopMetrics) SetQueueDepth(int)                  {}
