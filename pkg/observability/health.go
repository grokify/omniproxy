package observability

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// HealthChecker manages health check state and endpoints.
type HealthChecker struct {
	mu        sync.RWMutex
	ready     bool
	checks    map[string]HealthCheck
	startedAt time.Time
}

// HealthCheck is a function that returns nil if healthy, error if not.
type HealthCheck func() error

// HealthStatus represents the health status response.
type HealthStatus struct {
	Status    string            `json:"status"`
	Timestamp string            `json:"timestamp"`
	Uptime    string            `json:"uptime,omitempty"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		ready:     false,
		checks:    make(map[string]HealthCheck),
		startedAt: time.Now(),
	}
}

// RegisterCheck registers a named health check.
func (h *HealthChecker) RegisterCheck(name string, check HealthCheck) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = check
}

// SetReady marks the service as ready to receive traffic.
func (h *HealthChecker) SetReady(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ready = ready
}

// IsReady returns whether the service is ready.
func (h *HealthChecker) IsReady() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ready
}

// LivenessHandler returns an http.Handler for the /healthz endpoint.
// Returns 200 if the process is alive, used by Kubernetes liveness probe.
func (h *HealthChecker) LivenessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := HealthStatus{
			Status:    "ok",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Uptime:    time.Since(h.startedAt).Round(time.Second).String(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(status); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

// ReadinessHandler returns an http.Handler for the /readyz endpoint.
// Returns 200 if ready to receive traffic, 503 if not.
// Used by Kubernetes readiness probe and load balancers.
func (h *HealthChecker) ReadinessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.RLock()
		ready := h.ready
		checks := make(map[string]HealthCheck, len(h.checks))
		for k, v := range h.checks {
			checks[k] = v
		}
		h.mu.RUnlock()

		status := HealthStatus{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Checks:    make(map[string]string),
		}

		allHealthy := ready
		for name, check := range checks {
			if err := check(); err != nil {
				status.Checks[name] = err.Error()
				allHealthy = false
			} else {
				status.Checks[name] = "ok"
			}
		}

		if !ready {
			status.Checks["ready"] = "not ready"
		}

		w.Header().Set("Content-Type", "application/json")
		if allHealthy {
			status.Status = "ok"
			w.WriteHeader(http.StatusOK)
		} else {
			status.Status = "unhealthy"
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		if err := json.NewEncoder(w).Encode(status); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

// Handler returns an http.Handler that serves both health endpoints.
// Routes:
//   - /healthz - liveness probe
//   - /readyz  - readiness probe
//   - /health  - alias for /healthz
func (h *HealthChecker) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/healthz", h.LivenessHandler())
	mux.Handle("/readyz", h.ReadinessHandler())
	mux.Handle("/health", h.LivenessHandler())
	return mux
}

// CommonChecks provides factory functions for common health checks.
type CommonChecks struct{}

// DatabaseCheck returns a health check for database connectivity.
func (CommonChecks) DatabaseCheck(pingFunc func() error) HealthCheck {
	return func() error {
		return pingFunc()
	}
}

// RedisCheck returns a health check for Redis connectivity.
func (CommonChecks) RedisCheck(pingFunc func() error) HealthCheck {
	return func() error {
		return pingFunc()
	}
}

// KafkaCheck returns a health check for Kafka connectivity.
func (CommonChecks) KafkaCheck(pingFunc func() error) HealthCheck {
	return func() error {
		return pingFunc()
	}
}

// NewHealthMux creates an http.ServeMux with health and metrics endpoints.
func NewHealthMux(health *HealthChecker, provider *Provider) *http.ServeMux {
	mux := http.NewServeMux()

	if health != nil {
		mux.Handle("/healthz", health.LivenessHandler())
		mux.Handle("/readyz", health.ReadinessHandler())
	}

	if provider != nil {
		mux.Handle("/metrics", provider.PrometheusHandler())
	}

	return mux
}

// ListenAndServe starts an HTTP server on the given address.
func ListenAndServe(addr string, handler http.Handler) error {
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return server.ListenAndServe()
}
