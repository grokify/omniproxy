package reverseproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewReverseProxy(t *testing.T) {
	cfg := &Config{
		Backends: []Backend{
			{Host: "api.example.com", Target: "http://localhost:3000"},
		},
	}

	rp, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create reverse proxy: %v", err)
	}
	if rp == nil {
		t.Fatal("expected reverse proxy")
	}
}

func TestNewReverseProxyNoBackends(t *testing.T) {
	cfg := &Config{}

	_, err := New(cfg)
	if err == nil {
		t.Fatal("expected error for no backends")
	}
}

func TestNewReverseProxyInvalidTarget(t *testing.T) {
	cfg := &Config{
		Backends: []Backend{
			{Host: "api.example.com", Target: "://invalid"},
		},
	}

	_, err := New(cfg)
	if err == nil {
		t.Fatal("expected error for invalid target")
	}
}

func TestFindProxy(t *testing.T) {
	cfg := &Config{
		Backends: []Backend{
			{Host: "api.example.com", Target: "http://localhost:3000"},
			{Host: "*.test.com", Target: "http://localhost:4000"},
		},
	}

	rp, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create reverse proxy: %v", err)
	}

	tests := []struct {
		host     string
		expected bool
	}{
		{"api.example.com", true},
		{"api.example.com:443", true},
		{"other.example.com", false},
		{"foo.test.com", true},
		{"bar.test.com", true},
		{"test.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			proxy := rp.findProxy(tt.host)
			found := proxy != nil
			if found != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, found)
			}
		})
	}
}

func TestServeHTTPBackendNotFound(t *testing.T) {
	cfg := &Config{
		Backends: []Backend{
			{Host: "api.example.com", Target: "http://localhost:3000"},
		},
	}

	rp, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create reverse proxy: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://unknown.example.com/test", nil)
	w := httptest.NewRecorder()

	rp.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected status %d, got %d", http.StatusBadGateway, w.Code)
	}
}

func TestHealthCheck(t *testing.T) {
	// Create a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	cfg := &Config{
		Backends: []Backend{
			{Host: "api.example.com", Target: ts.URL, HealthCheck: "/health"},
		},
	}

	rp, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create reverse proxy: %v", err)
	}

	results := rp.HealthCheck(context.Background())
	if !results["api.example.com"] {
		t.Error("expected health check to pass")
	}
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		input    string
		contains string
	}{
		{"~/.omniproxy", ".omniproxy"},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		result := expandPath(tt.input)
		found := false
		for i := 0; i <= len(result)-len(tt.contains); i++ {
			if result[i:i+len(tt.contains)] == tt.contains {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expandPath(%q) = %q, expected to contain %q", tt.input, result, tt.contains)
		}
	}
}

func TestParseBackend(t *testing.T) {
	tests := []struct {
		input      string
		wantHost   string
		wantTarget string
		wantErr    bool
	}{
		{"api.example.com=http://localhost:3000", "api.example.com", "http://localhost:3000", false},
		{"*.example.com=http://backend:8080", "*.example.com", "http://backend:8080", false},
		{"invalid", "", "", true},
		{"=empty", "", "", true},
		{"empty=", "", "", true},
	}

	for _, tt := range tests {
		host, target, err := parseBackendString(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseBackendString(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if host != tt.wantHost {
			t.Errorf("parseBackendString(%q) host = %q, want %q", tt.input, host, tt.wantHost)
		}
		if target != tt.wantTarget {
			t.Errorf("parseBackendString(%q) target = %q, want %q", tt.input, target, tt.wantTarget)
		}
	}
}

// parseBackendString parses a backend string for testing (same logic as CLI)
func parseBackendString(s string) (host, target string, err error) {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			host = s[:i]
			target = s[i+1:]
			if host == "" || target == "" {
				return "", "", errEmpty
			}
			return host, target, nil
		}
	}
	return "", "", errMissing
}

var errEmpty = &parseError{"empty host or target"}
var errMissing = &parseError{"missing '=' separator"}

type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }
