// Package capture provides traffic capture functionality for the proxy.
package capture

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/grokify/omniproxy/pkg/contentdetect"
)

// Record represents a captured HTTP transaction.
type Record struct {
	// Request information
	Request RequestRecord `json:"request"`
	// Response information
	Response ResponseRecord `json:"response,omitempty"`
	// Timing information
	StartTime  time.Time `json:"startTime"`
	EndTime    time.Time `json:"endTime,omitempty"`
	DurationMs float64   `json:"durationMs,omitempty"`
}

// RequestRecord represents a captured HTTP request.
type RequestRecord struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Host        string            `json:"host"`
	Path        string            `json:"path"`
	Scheme      string            `json:"scheme"`
	Headers     map[string]string `json:"headers,omitempty"`
	Query       map[string]string `json:"query,omitempty"`
	Body        interface{}       `json:"body,omitempty"`
	BodySize    int64             `json:"bodySize,omitempty"`
	IsBinary    bool              `json:"isBinary,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
}

// ResponseRecord represents a captured HTTP response.
type ResponseRecord struct {
	Status      int               `json:"status"`
	StatusText  string            `json:"statusText,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        interface{}       `json:"body,omitempty"`
	IsBinary    bool              `json:"isBinary,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
	Size        int64             `json:"size,omitempty"`
}

// Capturer captures HTTP transactions.
type Capturer struct {
	mu       sync.Mutex
	records  []Record
	output   io.Writer
	format   Format
	config   *Config
	handlers []Handler
}

// Format specifies the output format for captured traffic.
type Format string

const (
	// FormatJSON outputs records as JSON objects
	FormatJSON Format = "json"
	// FormatNDJSON outputs records as newline-delimited JSON
	FormatNDJSON Format = "ndjson"
	// FormatHAR outputs records in HAR format
	FormatHAR Format = "har"
	// FormatIR outputs records in APISpecRift IR format
	FormatIR Format = "ir"
)

// Handler is called for each captured record.
type Handler func(*Record)

// Config holds capturer configuration.
type Config struct {
	// Output is where to write captured traffic (default: stdout)
	Output io.Writer
	// Format is the output format (default: ndjson)
	Format Format
	// IncludeHeaders controls whether to include headers
	IncludeHeaders bool
	// FilterHeaders is a list of headers to exclude (case-insensitive)
	FilterHeaders []string
	// IncludeBody controls whether to include request/response bodies
	IncludeBody bool
	// MaxBodySize is the maximum body size to capture (default: 1MB)
	MaxBodySize int64
	// SkipBinary skips capturing binary content (images, videos, etc.)
	SkipBinary bool
	// Filter is the request/response filter (optional)
	Filter *Filter
}

// DefaultConfig returns default capturer configuration.
func DefaultConfig() *Config {
	return &Config{
		Output:         os.Stdout,
		Format:         FormatNDJSON,
		IncludeHeaders: true,
		FilterHeaders: []string{
			"authorization",
			"cookie",
			"set-cookie",
			"x-api-key",
			"x-auth-token",
			"proxy-authorization",
		},
		IncludeBody: true,
		MaxBodySize: 1024 * 1024, // 1MB
		SkipBinary:  true,        // Skip binary content by default
	}
}

// NewCapturer creates a new traffic capturer.
func NewCapturer(cfg *Config) *Capturer {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Capturer{
		records: make([]Record, 0),
		output:  cfg.Output,
		format:  cfg.Format,
		config:  cfg,
	}
}

// AddHandler adds a handler to be called for each captured record.
func (c *Capturer) AddHandler(h Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers = append(c.handlers, h)
}

// StartCapture begins capturing a request.
func (c *Capturer) StartCapture(req *http.Request) *Record {
	rec := &Record{
		StartTime: time.Now(),
		Request: RequestRecord{
			Method: req.Method,
			URL:    req.URL.String(),
			Host:   req.Host,
			Path:   req.URL.Path,
			Scheme: req.URL.Scheme,
		},
	}

	// Determine scheme
	if rec.Request.Scheme == "" {
		if req.TLS != nil {
			rec.Request.Scheme = "https"
		} else {
			rec.Request.Scheme = "http"
		}
	}

	// Capture headers
	if c.config.IncludeHeaders && len(req.Header) > 0 {
		rec.Request.Headers = c.filterHeaders(req.Header)
		if ct := req.Header.Get("Content-Type"); ct != "" {
			rec.Request.ContentType = ct
		}
	}

	// Capture query parameters
	if len(req.URL.Query()) > 0 {
		rec.Request.Query = make(map[string]string)
		for k, v := range req.URL.Query() {
			if len(v) > 0 {
				rec.Request.Query[k] = v[0]
			}
		}
	}

	// Capture request body
	if c.config.IncludeBody && req.Body != nil && req.ContentLength > 0 && req.ContentLength <= c.config.MaxBodySize {
		body, err := io.ReadAll(io.LimitReader(req.Body, c.config.MaxBodySize))
		if err == nil && len(body) > 0 {
			req.Body = io.NopCloser(bytes.NewReader(body))
			rec.Request.BodySize = int64(len(body))

			// Check if binary content
			if c.config.SkipBinary && contentdetect.IsBinary(rec.Request.ContentType, body) {
				rec.Request.IsBinary = true
				rec.Request.Body = "[binary content]"
			} else {
				rec.Request.Body = c.parseBody(body, rec.Request.ContentType)
			}
		}
	}

	return rec
}

// FinishCapture completes capturing a response.
func (c *Capturer) FinishCapture(rec *Record, resp *http.Response) error {
	rec.EndTime = time.Now()
	rec.DurationMs = float64(rec.EndTime.Sub(rec.StartTime).Microseconds()) / 1000.0

	if resp != nil {
		rec.Response = ResponseRecord{
			Status:     resp.StatusCode,
			StatusText: resp.Status,
		}

		// Capture response headers
		if c.config.IncludeHeaders && len(resp.Header) > 0 {
			rec.Response.Headers = c.filterHeaders(resp.Header)
			if ct := resp.Header.Get("Content-Type"); ct != "" {
				rec.Response.ContentType = ct
			}
		}

		// Capture response body
		if c.config.IncludeBody && resp.Body != nil && resp.ContentLength <= c.config.MaxBodySize {
			body, err := io.ReadAll(io.LimitReader(resp.Body, c.config.MaxBodySize))
			if err == nil && len(body) > 0 {
				resp.Body = io.NopCloser(bytes.NewReader(body))
				rec.Response.Size = int64(len(body))

				// Check if binary content
				if c.config.SkipBinary && contentdetect.IsBinary(rec.Response.ContentType, body) {
					rec.Response.IsBinary = true
					rec.Response.Body = "[binary content]"
				} else {
					rec.Response.Body = c.parseBody(body, rec.Response.ContentType)
				}
			}
		}
	}

	return c.finishRecord(rec)
}

// FinishCaptureWithStatus completes capturing with just status code and size (for reverse proxy).
func (c *Capturer) FinishCaptureWithStatus(rec *Record, statusCode int, bytesWritten int64) error {
	rec.EndTime = time.Now()
	rec.DurationMs = float64(rec.EndTime.Sub(rec.StartTime).Microseconds()) / 1000.0
	rec.Response = ResponseRecord{
		Status: statusCode,
		Size:   bytesWritten,
	}

	return c.finishRecord(rec)
}

// finishRecord stores and writes the record.
func (c *Capturer) finishRecord(rec *Record) error {
	// Store record
	c.mu.Lock()
	c.records = append(c.records, *rec)
	handlers := c.handlers
	c.mu.Unlock()

	// Call handlers
	for _, h := range handlers {
		h(rec)
	}

	// Write to output
	return c.writeRecord(rec)
}

// filterHeaders filters sensitive headers.
func (c *Capturer) filterHeaders(headers http.Header) map[string]string {
	result := make(map[string]string)
	for k, v := range headers {
		skip := false
		kLower := normalizeHeader(k)
		for _, filter := range c.config.FilterHeaders {
			if normalizeHeader(filter) == kLower {
				skip = true
				break
			}
		}
		if !skip && len(v) > 0 {
			result[kLower] = v[0]
		}
	}
	return result
}

// parseBody attempts to parse body as JSON, otherwise returns as string.
func (c *Capturer) parseBody(body []byte, contentType string) interface{} {
	// Try JSON parsing
	if isJSONContentType(contentType) || len(body) > 0 && (body[0] == '{' || body[0] == '[') {
		var v interface{}
		if err := json.Unmarshal(body, &v); err == nil {
			return v
		}
	}
	return string(body)
}

// writeRecord writes a record to the output.
func (c *Capturer) writeRecord(rec *Record) error {
	if c.output == nil {
		return nil
	}

	var data []byte
	var err error

	switch c.format {
	case FormatJSON:
		data, err = json.MarshalIndent(rec, "", "  ")
	case FormatNDJSON, FormatIR:
		data, err = json.Marshal(rec)
	default:
		data, err = json.Marshal(rec)
	}

	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.output.Write(data); err != nil {
		return err
	}
	if _, err := c.output.Write([]byte("\n")); err != nil {
		return err
	}
	return nil
}

// Records returns all captured records.
func (c *Capturer) Records() []Record {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]Record, len(c.records))
	copy(result, c.records)
	return result
}

// Clear clears all captured records.
func (c *Capturer) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = make([]Record, 0)
}

func normalizeHeader(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func isJSONContentType(ct string) bool {
	return len(ct) >= 16 && (ct[:16] == "application/json" || contains(ct, "json"))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
