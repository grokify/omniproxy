// Package observability provides OpenTelemetry instrumentation for OmniProxy.
package observability

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	instrumentationName = "github.com/grokify/omniproxy"
)

// Metrics holds all OmniProxy metrics.
type Metrics struct {
	// Request metrics
	RequestsTotal   metric.Int64Counter
	RequestDuration metric.Float64Histogram
	ActiveRequests  metric.Int64UpDownCounter

	// Response metrics
	ResponseSize metric.Int64Histogram

	// Certificate metrics
	CertsGenerated metric.Int64Counter
	CertsCacheHits metric.Int64Counter
	CertsCacheMiss metric.Int64Counter

	// Traffic store metrics
	TrafficStored      metric.Int64Counter
	TrafficStoreErrors metric.Int64Counter
	TrafficQueueDepth  metric.Int64ObservableGauge

	// Connection metrics
	ActiveConnections metric.Int64UpDownCounter

	// For queue depth callback
	queueDepthFunc func() int64
}

// NewMetrics creates a new Metrics instance with all instruments registered.
func NewMetrics(meterProvider metric.MeterProvider) (*Metrics, error) {
	if meterProvider == nil {
		meterProvider = otel.GetMeterProvider()
	}

	meter := meterProvider.Meter(instrumentationName)
	m := &Metrics{}

	var err error

	// Request metrics
	m.RequestsTotal, err = meter.Int64Counter(
		"omniproxy.requests.total",
		metric.WithDescription("Total number of requests processed"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	m.RequestDuration, err = meter.Float64Histogram(
		"omniproxy.request.duration",
		metric.WithDescription("Request duration in milliseconds"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000),
	)
	if err != nil {
		return nil, err
	}

	m.ActiveRequests, err = meter.Int64UpDownCounter(
		"omniproxy.requests.active",
		metric.WithDescription("Number of requests currently being processed"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	// Response metrics
	m.ResponseSize, err = meter.Int64Histogram(
		"omniproxy.response.size",
		metric.WithDescription("Response body size in bytes"),
		metric.WithUnit("By"),
		metric.WithExplicitBucketBoundaries(100, 1000, 10000, 100000, 1000000, 10000000),
	)
	if err != nil {
		return nil, err
	}

	// Certificate metrics
	m.CertsGenerated, err = meter.Int64Counter(
		"omniproxy.certs.generated",
		metric.WithDescription("Total number of certificates generated"),
		metric.WithUnit("{certificate}"),
	)
	if err != nil {
		return nil, err
	}

	m.CertsCacheHits, err = meter.Int64Counter(
		"omniproxy.certs.cache.hits",
		metric.WithDescription("Number of certificate cache hits"),
		metric.WithUnit("{hit}"),
	)
	if err != nil {
		return nil, err
	}

	m.CertsCacheMiss, err = meter.Int64Counter(
		"omniproxy.certs.cache.misses",
		metric.WithDescription("Number of certificate cache misses"),
		metric.WithUnit("{miss}"),
	)
	if err != nil {
		return nil, err
	}

	// Traffic store metrics
	m.TrafficStored, err = meter.Int64Counter(
		"omniproxy.traffic.stored",
		metric.WithDescription("Total number of traffic records stored"),
		metric.WithUnit("{record}"),
	)
	if err != nil {
		return nil, err
	}

	m.TrafficStoreErrors, err = meter.Int64Counter(
		"omniproxy.traffic.store.errors",
		metric.WithDescription("Total number of traffic store errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, err
	}

	// Connection metrics
	m.ActiveConnections, err = meter.Int64UpDownCounter(
		"omniproxy.connections.active",
		metric.WithDescription("Number of active connections"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// RegisterQueueDepthCallback registers a callback to observe queue depth.
func (m *Metrics) RegisterQueueDepthCallback(meterProvider metric.MeterProvider, fn func() int64) error {
	if meterProvider == nil {
		meterProvider = otel.GetMeterProvider()
	}

	meter := meterProvider.Meter(instrumentationName)
	m.queueDepthFunc = fn

	var err error
	m.TrafficQueueDepth, err = meter.Int64ObservableGauge(
		"omniproxy.traffic.queue.depth",
		metric.WithDescription("Current number of records in the traffic queue"),
		metric.WithUnit("{record}"),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			if m.queueDepthFunc != nil {
				o.Observe(m.queueDepthFunc())
			}
			return nil
		}),
	)
	return err
}

// RecordRequest records metrics for a completed request.
func (m *Metrics) RecordRequest(ctx context.Context, method, host string, statusCode int, duration time.Duration, responseSize int64) {
	attrs := []attribute.KeyValue{
		attribute.String("method", method),
		attribute.String("host", host),
		attribute.Int("status_code", statusCode),
		attribute.String("status_class", statusClass(statusCode)),
	}

	m.RequestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.RequestDuration.Record(ctx, float64(duration.Milliseconds()), metric.WithAttributes(attrs...))

	if responseSize > 0 {
		m.ResponseSize.Record(ctx, responseSize, metric.WithAttributes(
			attribute.String("host", host),
		))
	}
}

// RequestStart should be called when a request starts.
func (m *Metrics) RequestStart(ctx context.Context) {
	m.ActiveRequests.Add(ctx, 1)
}

// RequestEnd should be called when a request ends.
func (m *Metrics) RequestEnd(ctx context.Context) {
	m.ActiveRequests.Add(ctx, -1)
}

// RecordCertGenerated records a certificate generation.
func (m *Metrics) RecordCertGenerated(ctx context.Context, host string) {
	m.CertsGenerated.Add(ctx, 1, metric.WithAttributes(
		attribute.String("host", host),
	))
}

// RecordCertCacheHit records a certificate cache hit.
func (m *Metrics) RecordCertCacheHit(ctx context.Context) {
	m.CertsCacheHits.Add(ctx, 1)
}

// RecordCertCacheMiss records a certificate cache miss.
func (m *Metrics) RecordCertCacheMiss(ctx context.Context) {
	m.CertsCacheMiss.Add(ctx, 1)
}

// RecordTrafficStored records a successful traffic store.
func (m *Metrics) RecordTrafficStored(ctx context.Context) {
	m.TrafficStored.Add(ctx, 1)
}

// RecordTrafficStoreError records a traffic store error.
func (m *Metrics) RecordTrafficStoreError(ctx context.Context) {
	m.TrafficStoreErrors.Add(ctx, 1)
}

// ConnectionOpened should be called when a connection is opened.
func (m *Metrics) ConnectionOpened(ctx context.Context) {
	m.ActiveConnections.Add(ctx, 1)
}

// ConnectionClosed should be called when a connection is closed.
func (m *Metrics) ConnectionClosed(ctx context.Context) {
	m.ActiveConnections.Add(ctx, -1)
}

// statusClass returns the status class (1xx, 2xx, etc.)
func statusClass(code int) string {
	switch {
	case code >= 100 && code < 200:
		return "1xx"
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500 && code < 600:
		return "5xx"
	default:
		return "unknown"
	}
}

// MetricsMiddleware wraps an http.Handler with metrics collection.
func (m *Metrics) MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		start := time.Now()

		m.RequestStart(ctx)
		defer m.RequestEnd(ctx)

		// Wrap response writer to capture status code and size
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		m.RecordRequest(ctx, r.Method, r.Host, wrapped.statusCode, duration, wrapped.bytesWritten)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (w *responseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += int64(n)
	return n, err
}
