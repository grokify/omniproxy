package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/grokify/omniproxy/pkg/capture"
	"github.com/grokify/omniproxy/ui/ent"
	"github.com/grokify/omniproxy/ui/ent/traffic"

	// Database drivers
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

// DatabaseTrafficStore stores traffic records in a database using Ent.
// Supports SQLite (laptop/team mode) and PostgreSQL (production mode).
type DatabaseTrafficStore struct {
	client  *ent.Client
	proxyID int
	metrics Metrics
	mu      sync.RWMutex
	closed  bool
}

// DatabaseTrafficStoreConfig configures the database traffic store.
type DatabaseTrafficStoreConfig struct {
	// DatabaseURL is the database connection URL.
	// Formats:
	//   - sqlite://path/to/file.db
	//   - postgres://user:password@host:port/database
	DatabaseURL string

	// ProxyID is the ID of the proxy to associate traffic with.
	// If 0, a default proxy will be created/used.
	ProxyID int

	// ProxyName is the name for the default proxy (if ProxyID is 0).
	ProxyName string

	// Metrics for observability (optional).
	Metrics Metrics

	// Debug enables SQL query logging.
	Debug bool
}

// NewDatabaseTrafficStore creates a new database-backed traffic store.
func NewDatabaseTrafficStore(ctx context.Context, cfg *DatabaseTrafficStoreConfig) (*DatabaseTrafficStore, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	// Parse the database URL
	dbCfg, err := ParseDatabaseURL(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Create the Ent client
	var client *ent.Client

	switch dbCfg.Type {
	case DBTypeSQLite:
		client, err = openSQLite(dbCfg, cfg.Debug)
	case DBTypePostgres:
		client, err = openPostgres(dbCfg, cfg.Debug)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbCfg.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Run migrations
	if err := client.Schema.Create(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	store := &DatabaseTrafficStore{
		client:  client,
		metrics: cfg.Metrics,
	}

	if store.metrics == nil {
		store.metrics = NoopMetrics{}
	}

	// Get or create the proxy
	proxyID := cfg.ProxyID
	if proxyID == 0 {
		proxyName := cfg.ProxyName
		if proxyName == "" {
			proxyName = "default"
		}
		proxy, err := ensureDefaultProxy(ctx, client, proxyName)
		if err != nil {
			client.Close()
			return nil, fmt.Errorf("failed to ensure default proxy: %w", err)
		}
		proxyID = proxy.ID
	}
	store.proxyID = proxyID

	return store, nil
}

// openSQLite opens a SQLite database connection.
func openSQLite(cfg *DBConfig, debug bool) (*ent.Client, error) {
	drv, err := entsql.Open(dialect.SQLite, cfg.DSN)
	if err != nil {
		return nil, err
	}

	// Enable WAL mode for better concurrency
	db := drv.DB()
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		drv.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		drv.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	opts := []ent.Option{ent.Driver(drv)}
	if debug {
		opts = append(opts, ent.Debug())
	}

	return ent.NewClient(opts...), nil
}

// openPostgres opens a PostgreSQL database connection.
func openPostgres(cfg *DBConfig, debug bool) (*ent.Client, error) {
	drv, err := entsql.Open(dialect.Postgres, cfg.DSN)
	if err != nil {
		return nil, err
	}

	opts := []ent.Option{ent.Driver(drv)}
	if debug {
		opts = append(opts, ent.Debug())
	}

	return ent.NewClient(opts...), nil
}

// ensureDefaultProxy creates a default proxy if it doesn't exist.
func ensureDefaultProxy(ctx context.Context, client *ent.Client, name string) (*ent.Proxy, error) {
	// First, ensure we have a default org
	org, err := ensureDefaultOrg(ctx, client)
	if err != nil {
		return nil, err
	}

	// Try to find existing proxy
	proxy, err := client.Proxy.Query().
		Where(func(s *entsql.Selector) {
			s.Where(entsql.EQ("name", name))
		}).
		Only(ctx)

	if err == nil {
		return proxy, nil
	}

	if !ent.IsNotFound(err) {
		return nil, err
	}

	// Create new proxy
	return client.Proxy.Create().
		SetName(name).
		SetSlug(name).
		SetOrg(org).
		Save(ctx)
}

// ensureDefaultOrg creates a default organization if it doesn't exist.
func ensureDefaultOrg(ctx context.Context, client *ent.Client) (*ent.Org, error) {
	org, err := client.Org.Query().
		Where(func(s *entsql.Selector) {
			s.Where(entsql.EQ("slug", "default"))
		}).
		Only(ctx)

	if err == nil {
		return org, nil
	}

	if !ent.IsNotFound(err) {
		return nil, err
	}

	// Create new org
	return client.Org.Create().
		SetName("Default").
		SetSlug("default").
		Save(ctx)
}

// Store saves a single traffic record to the database.
func (s *DatabaseTrafficStore) Store(ctx context.Context, rec *capture.Record) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return fmt.Errorf("store is closed")
	}
	s.mu.RUnlock()

	if rec == nil {
		return nil
	}

	start := time.Now()

	// Convert capture.Record to database fields
	create := s.client.Traffic.Create().
		SetMethod(rec.Request.Method).
		SetURL(rec.Request.URL).
		SetScheme(rec.Request.Scheme).
		SetHost(rec.Request.Host).
		SetPath(rec.Request.Path).
		SetStartedAt(rec.StartTime).
		SetDurationMs(rec.DurationMs).
		SetProxyID(s.proxyID)

	// Request headers
	if rec.Request.Headers != nil {
		headers := make(map[string][]string)
		for k, v := range rec.Request.Headers {
			headers[k] = []string{v}
		}
		create.SetRequestHeaders(headers)
	}

	// Request query
	if rec.Request.Query != nil {
		queryJSON, _ := json.Marshal(rec.Request.Query)
		create.SetQuery(string(queryJSON))
	}

	// Request body
	if rec.Request.Body != nil {
		bodyBytes, _ := json.Marshal(rec.Request.Body)
		create.SetRequestBody(bodyBytes)
		create.SetRequestBodySize(rec.Request.BodySize)
	}
	create.SetRequestIsBinary(rec.Request.IsBinary)
	if rec.Request.ContentType != "" {
		create.SetContentType(rec.Request.ContentType)
	}

	// Response fields
	create.SetStatusCode(rec.Response.Status)
	if rec.Response.StatusText != "" {
		create.SetStatusText(rec.Response.StatusText)
	}

	// Response headers
	if rec.Response.Headers != nil {
		headers := make(map[string][]string)
		for k, v := range rec.Response.Headers {
			headers[k] = []string{v}
		}
		create.SetResponseHeaders(headers)
	}

	// Response body
	if rec.Response.Body != nil {
		bodyBytes, _ := json.Marshal(rec.Response.Body)
		create.SetResponseBody(bodyBytes)
		create.SetResponseBodySize(rec.Response.Size)
	}
	create.SetResponseIsBinary(rec.Response.IsBinary)
	if rec.Response.ContentType != "" {
		create.SetResponseContentType(rec.Response.ContentType)
	}

	// Save
	_, err := create.Save(ctx)

	s.metrics.ObserveStoreDuration(time.Since(start))

	if err != nil {
		s.metrics.IncStoreError()
		return fmt.Errorf("failed to store traffic: %w", err)
	}

	s.metrics.IncStoreSuccess()
	return nil
}

// StoreBatch saves multiple traffic records efficiently using bulk insert.
func (s *DatabaseTrafficStore) StoreBatch(ctx context.Context, recs []*capture.Record) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return fmt.Errorf("store is closed")
	}
	s.mu.RUnlock()

	if len(recs) == 0 {
		return nil
	}

	start := time.Now()

	// Build bulk create
	builders := make([]*ent.TrafficCreate, 0, len(recs))
	for _, rec := range recs {
		if rec == nil {
			continue
		}

		create := s.client.Traffic.Create().
			SetMethod(rec.Request.Method).
			SetURL(rec.Request.URL).
			SetScheme(rec.Request.Scheme).
			SetHost(rec.Request.Host).
			SetPath(rec.Request.Path).
			SetStartedAt(rec.StartTime).
			SetDurationMs(rec.DurationMs).
			SetProxyID(s.proxyID).
			SetStatusCode(rec.Response.Status).
			SetRequestIsBinary(rec.Request.IsBinary).
			SetResponseIsBinary(rec.Response.IsBinary)

		// Request headers
		if rec.Request.Headers != nil {
			headers := make(map[string][]string)
			for k, v := range rec.Request.Headers {
				headers[k] = []string{v}
			}
			create.SetRequestHeaders(headers)
		}

		// Request query
		if rec.Request.Query != nil {
			queryJSON, _ := json.Marshal(rec.Request.Query)
			create.SetQuery(string(queryJSON))
		}

		// Request body
		if rec.Request.Body != nil {
			bodyBytes, _ := json.Marshal(rec.Request.Body)
			create.SetRequestBody(bodyBytes)
			create.SetRequestBodySize(rec.Request.BodySize)
		}
		if rec.Request.ContentType != "" {
			create.SetContentType(rec.Request.ContentType)
		}

		// Response
		if rec.Response.StatusText != "" {
			create.SetStatusText(rec.Response.StatusText)
		}
		if rec.Response.Headers != nil {
			headers := make(map[string][]string)
			for k, v := range rec.Response.Headers {
				headers[k] = []string{v}
			}
			create.SetResponseHeaders(headers)
		}
		if rec.Response.Body != nil {
			bodyBytes, _ := json.Marshal(rec.Response.Body)
			create.SetResponseBody(bodyBytes)
			create.SetResponseBodySize(rec.Response.Size)
		}
		if rec.Response.ContentType != "" {
			create.SetResponseContentType(rec.Response.ContentType)
		}

		builders = append(builders, create)
	}

	// Execute bulk create
	_, err := s.client.Traffic.CreateBulk(builders...).Save(ctx)

	s.metrics.ObserveStoreDuration(time.Since(start))

	if err != nil {
		s.metrics.IncStoreError()
		return fmt.Errorf("failed to store traffic batch: %w", err)
	}

	for range builders {
		s.metrics.IncStoreSuccess()
	}

	return nil
}

// Close closes the database connection.
func (s *DatabaseTrafficStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	return s.client.Close()
}

// Client returns the underlying Ent client for advanced queries.
func (s *DatabaseTrafficStore) Client() *ent.Client {
	return s.client
}

// ProxyID returns the proxy ID used by this store.
func (s *DatabaseTrafficStore) ProxyID() int {
	return s.proxyID
}

// Query returns traffic records matching the filter.
func (s *DatabaseTrafficStore) Query(ctx context.Context, filter *TrafficFilter) ([]*TrafficRecord, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, fmt.Errorf("store is closed")
	}
	s.mu.RUnlock()

	query := s.client.Traffic.Query()

	// Apply filters
	if filter != nil {
		if !filter.StartTime.IsZero() {
			query = query.Where(traffic.StartedAtGTE(filter.StartTime))
		}
		if !filter.EndTime.IsZero() {
			query = query.Where(traffic.StartedAtLTE(filter.EndTime))
		}
		if len(filter.Methods) > 0 {
			query = query.Where(traffic.MethodIn(filter.Methods...))
		}
		if filter.MinStatus > 0 {
			query = query.Where(traffic.StatusCodeGTE(filter.MinStatus))
		}
		if filter.MaxStatus > 0 {
			query = query.Where(traffic.StatusCodeLTE(filter.MaxStatus))
		}
		if len(filter.StatusCodes) > 0 {
			query = query.Where(traffic.StatusCodeIn(filter.StatusCodes...))
		}

		// Host filtering
		if len(filter.Hosts) > 0 {
			query = query.Where(traffic.HostIn(filter.Hosts...))
		}

		// Pagination
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}

		// Ordering
		if filter.OrderBy != "" {
			if filter.Desc {
				query = query.Order(ent.Desc(filter.OrderBy))
			} else {
				query = query.Order(ent.Asc(filter.OrderBy))
			}
		} else {
			// Default to newest first
			query = query.Order(ent.Desc(traffic.FieldStartedAt))
		}
	}

	// Execute query
	records, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query traffic: %w", err)
	}

	// Convert to TrafficRecord
	result := make([]*TrafficRecord, len(records))
	for i, r := range records {
		result[i] = &TrafficRecord{
			ID:        fmt.Sprintf("%d", r.ID),
			Method:    r.Method,
			URL:       r.URL,
			Host:      r.Host,
			Path:      r.Path,
			Status:    r.StatusCode,
			Duration:  time.Duration(r.DurationMs * float64(time.Millisecond)),
			StartTime: r.StartedAt,
			Error:     r.Error,
		}
	}

	return result, nil
}

// GetByID returns full traffic details for a single record.
func (s *DatabaseTrafficStore) GetByID(ctx context.Context, id string) (*TrafficDetail, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, fmt.Errorf("store is closed")
	}
	s.mu.RUnlock()

	// Parse ID
	var recordID int
	if _, err := fmt.Sscanf(id, "%d", &recordID); err != nil {
		return nil, fmt.Errorf("invalid traffic ID: %w", err)
	}

	// Query by ID
	r, err := s.client.Traffic.Get(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("failed to get traffic record: %w", err)
	}

	// Convert to TrafficDetail
	detail := &TrafficDetail{
		TrafficRecord: TrafficRecord{
			ID:        fmt.Sprintf("%d", r.ID),
			Method:    r.Method,
			URL:       r.URL,
			Host:      r.Host,
			Path:      r.Path,
			Status:    r.StatusCode,
			Duration:  time.Duration(r.DurationMs * float64(time.Millisecond)),
			StartTime: r.StartedAt,
			Error:     r.Error,
		},
		Scheme:              r.Scheme,
		Query:               r.Query,
		RequestHeaders:      r.RequestHeaders,
		RequestBodySize:     r.RequestBodySize,
		RequestIsBinary:     r.RequestIsBinary,
		RequestContentType:  r.ContentType,
		StatusText:          r.StatusText,
		ResponseHeaders:     r.ResponseHeaders,
		ResponseBodySize:    r.ResponseBodySize,
		ResponseIsBinary:    r.ResponseIsBinary,
		ResponseContentType: r.ResponseContentType,
		TTFBMs:              r.TtfbMs,
		ClientIP:            r.ClientIP,
		Tags:                r.Tags,
	}

	// Convert body bytes to string (only if not binary)
	if !r.RequestIsBinary && r.RequestBody != nil {
		detail.RequestBody = string(r.RequestBody)
	}
	if !r.ResponseIsBinary && r.ResponseBody != nil {
		detail.ResponseBody = string(r.ResponseBody)
	}

	return detail, nil
}

// Count returns the number of records matching the filter.
func (s *DatabaseTrafficStore) Count(ctx context.Context, filter *TrafficFilter) (int64, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return 0, fmt.Errorf("store is closed")
	}
	s.mu.RUnlock()

	query := s.client.Traffic.Query()

	// Apply filters (same as Query)
	if filter != nil {
		if !filter.StartTime.IsZero() {
			query = query.Where(traffic.StartedAtGTE(filter.StartTime))
		}
		if !filter.EndTime.IsZero() {
			query = query.Where(traffic.StartedAtLTE(filter.EndTime))
		}
		if len(filter.Methods) > 0 {
			query = query.Where(traffic.MethodIn(filter.Methods...))
		}
		if filter.MinStatus > 0 {
			query = query.Where(traffic.StatusCodeGTE(filter.MinStatus))
		}
		if filter.MaxStatus > 0 {
			query = query.Where(traffic.StatusCodeLTE(filter.MaxStatus))
		}
		if len(filter.StatusCodes) > 0 {
			query = query.Where(traffic.StatusCodeIn(filter.StatusCodes...))
		}
		if len(filter.Hosts) > 0 {
			query = query.Where(traffic.HostIn(filter.Hosts...))
		}
	}

	count, err := query.Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count traffic: %w", err)
	}

	return int64(count), nil
}

// Stats returns aggregate statistics for traffic matching the filter.
func (s *DatabaseTrafficStore) Stats(ctx context.Context, filter *TrafficFilter) (*TrafficStats, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, fmt.Errorf("store is closed")
	}
	s.mu.RUnlock()

	// Get total count
	total, err := s.Count(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Get error count (status >= 400)
	errorFilter := &TrafficFilter{MinStatus: 400}
	if filter != nil {
		errorFilter.StartTime = filter.StartTime
		errorFilter.EndTime = filter.EndTime
		errorFilter.Hosts = filter.Hosts
	}
	errors, err := s.Count(ctx, errorFilter)
	if err != nil {
		return nil, err
	}

	stats := &TrafficStats{
		TotalRequests:    total,
		TotalErrors:      errors,
		RequestsByMethod: make(map[string]int64),
		RequestsByStatus: make(map[int]int64),
	}

	// Get method counts using raw SQL for efficiency
	// This is a simplified implementation - production would use proper aggregates
	records, err := s.Query(ctx, &TrafficFilter{Limit: 10000})
	if err != nil {
		return stats, nil // Return partial stats on error
	}

	var totalDuration float64
	for _, r := range records {
		stats.RequestsByMethod[r.Method]++
		stats.RequestsByStatus[r.Status]++
		totalDuration += float64(r.Duration.Milliseconds())
	}

	if len(records) > 0 {
		stats.AvgDurationMs = totalDuration / float64(len(records))
	}

	// Get unique hosts
	hostMap := make(map[string]bool)
	for _, r := range records {
		hostMap[r.Host] = true
	}
	stats.UniqueHosts = int64(len(hostMap))

	return stats, nil
}

// Ensure DatabaseTrafficStore implements TrafficStore and TrafficQuerier.
var (
	_ TrafficStore   = (*DatabaseTrafficStore)(nil)
	_ TrafficQuerier = (*DatabaseTrafficStore)(nil)
)
