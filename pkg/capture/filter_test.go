package capture

import (
	"testing"
)

func TestNewFilter(t *testing.T) {
	f := NewFilter()
	if f == nil {
		t.Fatal("expected filter")
	}
	if f.MinStatusCode != 0 {
		t.Errorf("expected MinStatusCode 0, got %d", f.MinStatusCode)
	}
	if f.MaxStatusCode != 999 {
		t.Errorf("expected MaxStatusCode 999, got %d", f.MaxStatusCode)
	}
}

func TestFilterCompile(t *testing.T) {
	f := NewFilter()
	f.IncludeHosts = []string{"*.example.com", "api.test.org"}
	f.ExcludePaths = []string{"/health", "/metrics"}

	if err := f.Compile(); err != nil {
		t.Fatalf("failed to compile: %v", err)
	}

	if len(f.includeHostPatterns) != 2 {
		t.Errorf("expected 2 include host patterns, got %d", len(f.includeHostPatterns))
	}
	if len(f.excludePathPatterns) != 2 {
		t.Errorf("expected 2 exclude path patterns, got %d", len(f.excludePathPatterns))
	}
}

func TestFilterMatchHost(t *testing.T) {
	tests := []struct {
		name         string
		includeHosts []string
		excludeHosts []string
		host         string
		expected     bool
	}{
		{
			name:     "no filters - match all",
			host:     "api.example.com",
			expected: true,
		},
		{
			name:         "include wildcard match",
			includeHosts: []string{"*.example.com"},
			host:         "api.example.com",
			expected:     true,
		},
		{
			name:         "include wildcard no match",
			includeHosts: []string{"*.example.com"},
			host:         "api.other.com",
			expected:     false,
		},
		{
			name:         "include exact match",
			includeHosts: []string{"api.example.com"},
			host:         "api.example.com",
			expected:     true,
		},
		{
			name:         "exclude wildcard",
			excludeHosts: []string{"*.cdn.com"},
			host:         "images.cdn.com",
			expected:     false,
		},
		{
			name:         "exclude doesn't match",
			excludeHosts: []string{"*.cdn.com"},
			host:         "api.example.com",
			expected:     true,
		},
		{
			name:         "include and exclude",
			includeHosts: []string{"*.example.com"},
			excludeHosts: []string{"internal.example.com"},
			host:         "api.example.com",
			expected:     true,
		},
		{
			name:         "include but excluded",
			includeHosts: []string{"*.example.com"},
			excludeHosts: []string{"internal.example.com"},
			host:         "internal.example.com",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFilter()
			f.IncludeHosts = tt.includeHosts
			f.ExcludeHosts = tt.excludeHosts
			if err := f.Compile(); err != nil {
				t.Fatalf("failed to compile: %v", err)
			}

			result := f.MatchRequest(tt.host, "/test", "GET")
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestFilterMatchPath(t *testing.T) {
	tests := []struct {
		name         string
		includePaths []string
		excludePaths []string
		path         string
		expected     bool
	}{
		{
			name:     "no filters",
			path:     "/api/users",
			expected: true,
		},
		{
			name:         "include path prefix",
			includePaths: []string{"/api/*"},
			path:         "/api/users",
			expected:     true,
		},
		{
			name:         "include path no match",
			includePaths: []string{"/api/*"},
			path:         "/health",
			expected:     false,
		},
		{
			name:         "exclude static assets",
			excludePaths: []string{"*.js", "*.css", "*.png"},
			path:         "/app.js",
			expected:     false,
		},
		{
			name:         "exclude doesn't match",
			excludePaths: []string{"*.js", "*.css"},
			path:         "/api/users",
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFilter()
			f.IncludePaths = tt.includePaths
			f.ExcludePaths = tt.excludePaths
			if err := f.Compile(); err != nil {
				t.Fatalf("failed to compile: %v", err)
			}

			result := f.MatchRequest("example.com", tt.path, "GET")
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestFilterMatchMethod(t *testing.T) {
	tests := []struct {
		name           string
		includeMethods []string
		excludeMethods []string
		method         string
		expected       bool
	}{
		{
			name:     "no filters",
			method:   "GET",
			expected: true,
		},
		{
			name:           "include GET POST",
			includeMethods: []string{"GET", "POST"},
			method:         "GET",
			expected:       true,
		},
		{
			name:           "include GET POST - DELETE not included",
			includeMethods: []string{"GET", "POST"},
			method:         "DELETE",
			expected:       false,
		},
		{
			name:           "exclude OPTIONS",
			excludeMethods: []string{"OPTIONS", "HEAD"},
			method:         "OPTIONS",
			expected:       false,
		},
		{
			name:           "exclude OPTIONS - GET allowed",
			excludeMethods: []string{"OPTIONS", "HEAD"},
			method:         "GET",
			expected:       true,
		},
		{
			name:           "case insensitive",
			includeMethods: []string{"get", "post"},
			method:         "GET",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFilter()
			f.IncludeMethods = tt.includeMethods
			f.ExcludeMethods = tt.excludeMethods
			if err := f.Compile(); err != nil {
				t.Fatalf("failed to compile: %v", err)
			}

			result := f.MatchRequest("example.com", "/test", tt.method)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestFilterMatchResponse(t *testing.T) {
	tests := []struct {
		name               string
		includeStatusCodes []int
		excludeStatusCodes []int
		minStatusCode      int
		maxStatusCode      int
		statusCode         int
		expected           bool
	}{
		{
			name:          "default - all codes",
			minStatusCode: 0,
			maxStatusCode: 999,
			statusCode:    200,
			expected:      true,
		},
		{
			name:          "only success codes",
			minStatusCode: 200,
			maxStatusCode: 299,
			statusCode:    200,
			expected:      true,
		},
		{
			name:          "only success codes - 404 excluded",
			minStatusCode: 200,
			maxStatusCode: 299,
			statusCode:    404,
			expected:      false,
		},
		{
			name:               "include specific codes",
			includeStatusCodes: []int{200, 201, 204},
			minStatusCode:      0,
			maxStatusCode:      999,
			statusCode:         200,
			expected:           true,
		},
		{
			name:               "include specific codes - 500 not included",
			includeStatusCodes: []int{200, 201, 204},
			minStatusCode:      0,
			maxStatusCode:      999,
			statusCode:         500,
			expected:           false,
		},
		{
			name:               "exclude specific codes",
			excludeStatusCodes: []int{304},
			minStatusCode:      0,
			maxStatusCode:      999,
			statusCode:         304,
			expected:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFilter()
			f.IncludeStatusCodes = tt.includeStatusCodes
			f.ExcludeStatusCodes = tt.excludeStatusCodes
			f.MinStatusCode = tt.minStatusCode
			f.MaxStatusCode = tt.maxStatusCode
			if err := f.Compile(); err != nil {
				t.Fatalf("failed to compile: %v", err)
			}

			result := f.MatchResponse(tt.statusCode)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestWildcardToRegexp(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		match   bool
	}{
		{"*.example.com", "api.example.com", true},
		{"*.example.com", "example.com", false},
		{"api.example.com", "api.example.com", true},
		{"api.example.com", "other.example.com", false},
		{"/api/*", "/api/users", true},
		{"/api/*", "/api/users/123", true},
		{"/api/*", "/health", false},
		{"*.js", "app.js", true},
		{"*.js", "app.css", false},
		{"test?", "test1", true},
		{"test?", "test12", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.input, func(t *testing.T) {
			re, err := wildcardToRegexp(tt.pattern)
			if err != nil {
				t.Fatalf("failed to compile pattern: %v", err)
			}

			result := re.MatchString(tt.input)
			if result != tt.match {
				t.Errorf("pattern %q, input %q: expected %v, got %v", tt.pattern, tt.input, tt.match, result)
			}
		})
	}
}
