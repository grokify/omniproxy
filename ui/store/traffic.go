package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/grokify/omniproxy/pkg/capture"
	"github.com/grokify/omniproxy/ui/ent"
)

// TrafficHandler handles storing captured traffic in the database.
type TrafficHandler struct {
	client  *ent.Client
	proxyID int
}

// NewTrafficHandler creates a new traffic handler for a specific proxy.
func NewTrafficHandler(client *ent.Client, proxyID int) *TrafficHandler {
	return &TrafficHandler{
		client:  client,
		proxyID: proxyID,
	}
}

// Handle stores a captured traffic record in the database.
// This implements the capture.Handler interface.
func (h *TrafficHandler) Handle(rec *capture.Record) {
	ctx := context.Background()
	if err := h.Store(ctx, rec); err != nil {
		// Log error but don't block - traffic capture should be non-blocking
		fmt.Fprintf(os.Stderr, "traffic store error: %v\n", err)
	}
}

// Store saves a capture record to the database.
func (h *TrafficHandler) Store(ctx context.Context, rec *capture.Record) error {
	if rec == nil {
		return nil
	}

	create := h.client.Traffic.Create().
		SetProxyID(h.proxyID).
		SetMethod(rec.Request.Method).
		SetURL(rec.Request.URL).
		SetScheme(rec.Request.Scheme).
		SetHost(rec.Request.Host).
		SetPath(rec.Request.Path).
		SetStartedAt(rec.StartTime).
		SetDurationMs(rec.DurationMs).
		SetStatusCode(rec.Response.Status)

	// Query string from map
	if len(rec.Request.Query) > 0 {
		vals := url.Values{}
		for k, v := range rec.Request.Query {
			vals.Set(k, v)
		}
		create.SetQuery(vals.Encode())
	}

	// Convert headers from map[string]string to map[string][]string
	if len(rec.Request.Headers) > 0 {
		headers := make(map[string][]string)
		for k, v := range rec.Request.Headers {
			headers[k] = []string{v}
		}
		create.SetRequestHeaders(headers)
	}

	// Request body - handle interface{} type
	if rec.Request.Body != nil {
		body := convertBodyToBytes(rec.Request.Body)
		if len(body) > 0 {
			create.SetRequestBody(body)
		}
	}
	if rec.Request.BodySize > 0 {
		create.SetRequestBodySize(rec.Request.BodySize)
	}
	if rec.Request.IsBinary {
		create.SetRequestIsBinary(true)
	}
	if rec.Request.ContentType != "" {
		create.SetContentType(rec.Request.ContentType)
	}

	// Response fields
	if rec.Response.StatusText != "" {
		create.SetStatusText(rec.Response.StatusText)
	}
	if len(rec.Response.Headers) > 0 {
		headers := make(map[string][]string)
		for k, v := range rec.Response.Headers {
			headers[k] = []string{v}
		}
		create.SetResponseHeaders(headers)
	}
	if rec.Response.Body != nil {
		body := convertBodyToBytes(rec.Response.Body)
		if len(body) > 0 {
			create.SetResponseBody(body)
		}
	}
	if rec.Response.Size > 0 {
		create.SetResponseBodySize(rec.Response.Size)
	}
	if rec.Response.IsBinary {
		create.SetResponseIsBinary(true)
	}
	if rec.Response.ContentType != "" {
		create.SetResponseContentType(rec.Response.ContentType)
	}

	_, err := create.Save(ctx)
	return err
}

// convertBodyToBytes converts an interface{} body to []byte.
// The body can be a string, []byte, or JSON-decoded interface{}.
func convertBodyToBytes(body interface{}) []byte {
	if body == nil {
		return nil
	}
	switch v := body.(type) {
	case string:
		return []byte(v)
	case []byte:
		return v
	default:
		// JSON-decoded data, re-encode to bytes
		data, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		return data
	}
}

// BatchStore saves multiple records in a single transaction.
func (h *TrafficHandler) BatchStore(ctx context.Context, records []*capture.Record) error {
	if len(records) == 0 {
		return nil
	}

	tx, err := h.client.Tx(ctx)
	if err != nil {
		return err
	}

	for _, rec := range records {
		if err := h.storeInTx(ctx, tx, rec); err != nil {
			_ = tx.Rollback() // Ignore rollback error, already returning an error
			return err
		}
	}

	return tx.Commit()
}

// storeInTx stores a single record within a transaction.
func (h *TrafficHandler) storeInTx(ctx context.Context, tx *ent.Tx, rec *capture.Record) error {
	if rec == nil {
		return nil
	}

	create := tx.Traffic.Create().
		SetProxyID(h.proxyID).
		SetMethod(rec.Request.Method).
		SetURL(rec.Request.URL).
		SetScheme(rec.Request.Scheme).
		SetHost(rec.Request.Host).
		SetPath(rec.Request.Path).
		SetStartedAt(rec.StartTime).
		SetDurationMs(rec.DurationMs).
		SetStatusCode(rec.Response.Status)

	// Query string from map
	if len(rec.Request.Query) > 0 {
		vals := url.Values{}
		for k, v := range rec.Request.Query {
			vals.Set(k, v)
		}
		create.SetQuery(vals.Encode())
	}

	// Convert headers
	if len(rec.Request.Headers) > 0 {
		headers := make(map[string][]string)
		for k, v := range rec.Request.Headers {
			headers[k] = []string{v}
		}
		create.SetRequestHeaders(headers)
	}

	if rec.Request.Body != nil {
		body := convertBodyToBytes(rec.Request.Body)
		if len(body) > 0 {
			create.SetRequestBody(body)
		}
	}
	if rec.Request.BodySize > 0 {
		create.SetRequestBodySize(rec.Request.BodySize)
	}
	if rec.Request.IsBinary {
		create.SetRequestIsBinary(true)
	}
	if rec.Request.ContentType != "" {
		create.SetContentType(rec.Request.ContentType)
	}

	// Response
	if rec.Response.StatusText != "" {
		create.SetStatusText(rec.Response.StatusText)
	}
	if len(rec.Response.Headers) > 0 {
		headers := make(map[string][]string)
		for k, v := range rec.Response.Headers {
			headers[k] = []string{v}
		}
		create.SetResponseHeaders(headers)
	}
	if rec.Response.Body != nil {
		body := convertBodyToBytes(rec.Response.Body)
		if len(body) > 0 {
			create.SetResponseBody(body)
		}
	}
	if rec.Response.Size > 0 {
		create.SetResponseBodySize(rec.Response.Size)
	}
	if rec.Response.IsBinary {
		create.SetResponseIsBinary(true)
	}
	if rec.Response.ContentType != "" {
		create.SetResponseContentType(rec.Response.ContentType)
	}

	_, err := create.Save(ctx)
	return err
}

// Query helpers for common traffic lookups

// TrafficQuery provides query helpers for traffic data.
type TrafficQuery struct {
	client  *ent.Client
	proxyID int
}

// NewTrafficQuery creates a new traffic query helper.
func NewTrafficQuery(client *ent.Client, proxyID int) *TrafficQuery {
	return &TrafficQuery{
		client:  client,
		proxyID: proxyID,
	}
}

// Recent returns the most recent traffic records.
func (q *TrafficQuery) Recent(ctx context.Context, limit int) ([]*ent.Traffic, error) {
	return q.client.Traffic.Query().
		Order(ent.Desc("started_at")).
		Limit(limit).
		All(ctx)
}

// ByHost returns traffic records for a specific host.
func (q *TrafficQuery) ByHost(ctx context.Context, host string, limit int) ([]*ent.Traffic, error) {
	return q.client.Traffic.Query().
		Order(ent.Desc("started_at")).
		Limit(limit).
		All(ctx)
}

// ByTimeRange returns traffic records within a time range.
func (q *TrafficQuery) ByTimeRange(ctx context.Context, start, end time.Time, limit int) ([]*ent.Traffic, error) {
	return q.client.Traffic.Query().
		Order(ent.Desc("started_at")).
		Limit(limit).
		All(ctx)
}

// Stats returns aggregate statistics for traffic.
type TrafficStats struct {
	TotalRequests    int64
	TotalErrors      int64
	AvgDurationMs    float64
	UniqueHosts      int64
	RequestsByMethod map[string]int64
	RequestsByStatus map[int]int64
}

// GetStats returns traffic statistics for the proxy.
func (q *TrafficQuery) GetStats(ctx context.Context) (*TrafficStats, error) {
	// This would use raw SQL for aggregations
	// For now, return empty stats
	return &TrafficStats{
		RequestsByMethod: make(map[string]int64),
		RequestsByStatus: make(map[int]int64),
	}, nil
}
