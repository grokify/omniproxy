package observability

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Provider holds the OpenTelemetry providers and exporters.
type Provider struct {
	MeterProvider *sdkmetric.MeterProvider
	Metrics       *Metrics
	promExporter  *prometheus.Exporter
}

// Config configures the observability provider.
type Config struct {
	// ServiceName is the name of the service for telemetry.
	ServiceName string

	// ServiceVersion is the version of the service.
	ServiceVersion string

	// EnablePrometheus enables the Prometheus metrics exporter.
	EnablePrometheus bool
}

// DefaultConfig returns default configuration.
func DefaultConfig() *Config {
	return &Config{
		ServiceName:      "omniproxy",
		ServiceVersion:   "dev",
		EnablePrometheus: true,
	}
}

// NewProvider creates a new observability provider.
func NewProvider(cfg *Config) (*Provider, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	p := &Provider{}

	// Create Prometheus exporter if enabled
	if cfg.EnablePrometheus {
		exporter, err := prometheus.New()
		if err != nil {
			return nil, err
		}
		p.promExporter = exporter

		// Create meter provider with Prometheus exporter
		p.MeterProvider = sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(exporter),
		)
	} else {
		// Create meter provider without exporter (noop)
		p.MeterProvider = sdkmetric.NewMeterProvider()
	}

	// Set as global provider
	otel.SetMeterProvider(p.MeterProvider)

	// Create metrics
	metrics, err := NewMetrics(p.MeterProvider)
	if err != nil {
		return nil, err
	}
	p.Metrics = metrics

	return p, nil
}

// PrometheusHandler returns an http.Handler for the /metrics endpoint.
func (p *Provider) PrometheusHandler() http.Handler {
	return promhttp.Handler()
}

// Shutdown gracefully shuts down the provider.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.MeterProvider != nil {
		return p.MeterProvider.Shutdown(ctx)
	}
	return nil
}

// BackendMetrics adapts the observability.Metrics to the backend.Metrics interface.
type BackendMetrics struct {
	m   *Metrics
	ctx context.Context
}

// NewBackendMetrics creates a backend.Metrics adapter.
func NewBackendMetrics(m *Metrics) *BackendMetrics {
	return &BackendMetrics{
		m:   m,
		ctx: context.Background(),
	}
}

// IncStoreSuccess increments successful store counter.
func (b *BackendMetrics) IncStoreSuccess() {
	b.m.RecordTrafficStored(b.ctx)
}

// IncStoreError increments store error counter.
func (b *BackendMetrics) IncStoreError() {
	b.m.RecordTrafficStoreError(b.ctx)
}

// ObserveStoreDuration records store operation duration.
// Note: Currently not implemented in metrics, could add if needed.
func (b *BackendMetrics) ObserveStoreDuration(d time.Duration) {}

// IncCacheHit increments cache hit counter.
func (b *BackendMetrics) IncCacheHit() {
	b.m.RecordCertCacheHit(b.ctx)
}

// IncCacheMiss increments cache miss counter.
func (b *BackendMetrics) IncCacheMiss() {
	b.m.RecordCertCacheMiss(b.ctx)
}

// SetQueueDepth sets the current queue depth gauge.
// Note: Queue depth is handled via callback in the main metrics.
func (b *BackendMetrics) SetQueueDepth(n int) {}
