package capture

import (
	"regexp"
	"strings"
)

// Filter defines criteria for including/excluding requests.
type Filter struct {
	// IncludeHosts is a list of hosts to include (supports wildcards)
	IncludeHosts []string
	// ExcludeHosts is a list of hosts to exclude (supports wildcards)
	ExcludeHosts []string
	// IncludePaths is a list of path patterns to include (supports wildcards)
	IncludePaths []string
	// ExcludePaths is a list of path patterns to exclude (supports wildcards)
	ExcludePaths []string
	// IncludeMethods is a list of HTTP methods to include
	IncludeMethods []string
	// ExcludeMethods is a list of HTTP methods to exclude
	ExcludeMethods []string
	// IncludeStatusCodes is a list of status codes to include
	IncludeStatusCodes []int
	// ExcludeStatusCodes is a list of status codes to exclude
	ExcludeStatusCodes []int
	// MinStatusCode is the minimum status code to include
	MinStatusCode int
	// MaxStatusCode is the maximum status code to include
	MaxStatusCode int

	// Compiled patterns (internal)
	includeHostPatterns []*regexp.Regexp
	excludeHostPatterns []*regexp.Regexp
	includePathPatterns []*regexp.Regexp
	excludePathPatterns []*regexp.Regexp
}

// NewFilter creates a new filter with default settings.
func NewFilter() *Filter {
	return &Filter{
		MinStatusCode: 0,
		MaxStatusCode: 999,
	}
}

// Compile compiles the filter patterns for efficient matching.
func (f *Filter) Compile() error {
	var err error

	f.includeHostPatterns, err = compilePatterns(f.IncludeHosts)
	if err != nil {
		return err
	}

	f.excludeHostPatterns, err = compilePatterns(f.ExcludeHosts)
	if err != nil {
		return err
	}

	f.includePathPatterns, err = compilePatterns(f.IncludePaths)
	if err != nil {
		return err
	}

	f.excludePathPatterns, err = compilePatterns(f.ExcludePaths)
	if err != nil {
		return err
	}

	return nil
}

// MatchRequest checks if a request matches the filter criteria.
func (f *Filter) MatchRequest(host, path, method string) bool {
	// Check host filters
	if !f.matchHost(host) {
		return false
	}

	// Check path filters
	if !f.matchPath(path) {
		return false
	}

	// Check method filters
	if !f.matchMethod(method) {
		return false
	}

	return true
}

// MatchResponse checks if a response matches the filter criteria.
func (f *Filter) MatchResponse(statusCode int) bool {
	// Check status code range
	if statusCode < f.MinStatusCode || statusCode > f.MaxStatusCode {
		return false
	}

	// Check include status codes
	if len(f.IncludeStatusCodes) > 0 {
		found := false
		for _, code := range f.IncludeStatusCodes {
			if code == statusCode {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check exclude status codes
	for _, code := range f.ExcludeStatusCodes {
		if code == statusCode {
			return false
		}
	}

	return true
}

// Match checks if a complete record matches the filter criteria.
func (f *Filter) Match(rec *Record) bool {
	if !f.MatchRequest(rec.Request.Host, rec.Request.Path, rec.Request.Method) {
		return false
	}

	if !f.MatchResponse(rec.Response.Status) {
		return false
	}

	return true
}

// matchHost checks if a host matches the filter.
func (f *Filter) matchHost(host string) bool {
	// If include patterns are specified, host must match at least one
	if len(f.includeHostPatterns) > 0 {
		matched := false
		for _, pattern := range f.includeHostPatterns {
			if pattern.MatchString(host) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check exclude patterns
	for _, pattern := range f.excludeHostPatterns {
		if pattern.MatchString(host) {
			return false
		}
	}

	return true
}

// matchPath checks if a path matches the filter.
func (f *Filter) matchPath(path string) bool {
	// If include patterns are specified, path must match at least one
	if len(f.includePathPatterns) > 0 {
		matched := false
		for _, pattern := range f.includePathPatterns {
			if pattern.MatchString(path) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check exclude patterns
	for _, pattern := range f.excludePathPatterns {
		if pattern.MatchString(path) {
			return false
		}
	}

	return true
}

// matchMethod checks if a method matches the filter.
func (f *Filter) matchMethod(method string) bool {
	method = strings.ToUpper(method)

	// If include methods are specified, method must be in the list
	if len(f.IncludeMethods) > 0 {
		found := false
		for _, m := range f.IncludeMethods {
			if strings.ToUpper(m) == method {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check exclude methods
	for _, m := range f.ExcludeMethods {
		if strings.ToUpper(m) == method {
			return false
		}
	}

	return true
}

// compilePatterns converts wildcard patterns to regexps.
func compilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	result := make([]*regexp.Regexp, 0, len(patterns))

	for _, pattern := range patterns {
		re, err := wildcardToRegexp(pattern)
		if err != nil {
			return nil, err
		}
		result = append(result, re)
	}

	return result, nil
}

// wildcardToRegexp converts a wildcard pattern to a regexp.
// Supports * (match any characters) and ? (match single character).
func wildcardToRegexp(pattern string) (*regexp.Regexp, error) {
	// Escape special regex characters except * and ?
	var result strings.Builder
	result.WriteString("^")

	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			result.WriteString(".*")
		case '?':
			result.WriteString(".")
		case '.', '+', '^', '$', '[', ']', '(', ')', '{', '}', '|', '\\':
			result.WriteString("\\")
			result.WriteByte(c)
		default:
			result.WriteByte(c)
		}
	}

	result.WriteString("$")
	return regexp.Compile(result.String())
}
