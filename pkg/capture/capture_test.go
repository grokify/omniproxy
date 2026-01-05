package capture

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestNewCapturer(t *testing.T) {
	c := NewCapturer(nil)
	if c == nil {
		t.Fatal("expected capturer")
	}

	if c.config.Format != FormatNDJSON {
		t.Errorf("expected NDJSON format, got %s", c.config.Format)
	}

	if !c.config.IncludeHeaders {
		t.Error("expected IncludeHeaders to be true")
	}
}

func TestCapturerWithConfig(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := &Config{
		Output:         buf,
		Format:         FormatJSON,
		IncludeHeaders: false,
		IncludeBody:    true,
		MaxBodySize:    1024,
	}

	c := NewCapturer(cfg)
	if c.config.Format != FormatJSON {
		t.Errorf("expected JSON format, got %s", c.config.Format)
	}

	if c.config.IncludeHeaders {
		t.Error("expected IncludeHeaders to be false")
	}
}

func TestStartCapture(t *testing.T) {
	c := NewCapturer(nil)

	req, _ := http.NewRequest("GET", "https://api.example.com/users?limit=10", nil)
	req.Header.Set("Accept", "application/json")

	rec := c.StartCapture(req)

	if rec.Request.Method != "GET" {
		t.Errorf("expected GET, got %s", rec.Request.Method)
	}

	if rec.Request.Path != "/users" {
		t.Errorf("expected /users, got %s", rec.Request.Path)
	}

	if rec.Request.Host != "api.example.com" {
		t.Errorf("expected api.example.com, got %s", rec.Request.Host)
	}

	if rec.Request.Scheme != "https" {
		t.Errorf("expected https, got %s", rec.Request.Scheme)
	}

	if rec.Request.Query["limit"] != "10" {
		t.Errorf("expected limit=10, got %v", rec.Request.Query)
	}
}

func TestFinishCapture(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := &Config{
		Output:         buf,
		Format:         FormatNDJSON,
		IncludeHeaders: true,
		IncludeBody:    true,
		MaxBodySize:    1024,
		FilterHeaders:  []string{"authorization"},
	}
	c := NewCapturer(cfg)

	req, _ := http.NewRequest("GET", "https://api.example.com/test", nil)
	rec := c.StartCapture(req)

	resp := &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	c.FinishCapture(rec, resp)

	if rec.Response.Status != 200 {
		t.Errorf("expected 200, got %d", rec.Response.Status)
	}

	// Duration may be 0 if start and end are in the same microsecond
	if rec.DurationMs < 0 {
		t.Error("expected non-negative duration")
	}

	// Check output was written
	if buf.Len() == 0 {
		t.Error("expected output to be written")
	}
}

func TestHeaderFiltering(t *testing.T) {
	cfg := &Config{
		Output:         &bytes.Buffer{},
		Format:         FormatNDJSON,
		IncludeHeaders: true,
		IncludeBody:    false,
		FilterHeaders:  []string{"authorization", "cookie", "x-api-key"},
	}
	c := NewCapturer(cfg)

	req, _ := http.NewRequest("GET", "https://api.example.com/test", nil)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Cookie", "session=abc")
	req.Header.Set("X-Api-Key", "key123")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "test")

	rec := c.StartCapture(req)

	// Sensitive headers should be filtered
	if _, ok := rec.Request.Headers["authorization"]; ok {
		t.Error("authorization header should be filtered")
	}
	if _, ok := rec.Request.Headers["cookie"]; ok {
		t.Error("cookie header should be filtered")
	}
	if _, ok := rec.Request.Headers["x-api-key"]; ok {
		t.Error("x-api-key header should be filtered")
	}

	// Non-sensitive headers should be present
	if rec.Request.Headers["accept"] != "application/json" {
		t.Error("accept header should be present")
	}
	if rec.Request.Headers["user-agent"] != "test" {
		t.Error("user-agent header should be present")
	}
}

func TestRecords(t *testing.T) {
	c := NewCapturer(nil)

	// Capture a few requests
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("GET", "https://api.example.com/test", nil)
		rec := c.StartCapture(req)
		c.FinishCapture(rec, &http.Response{StatusCode: 200})
	}

	records := c.Records()
	if len(records) != 3 {
		t.Errorf("expected 3 records, got %d", len(records))
	}
}

func TestClear(t *testing.T) {
	c := NewCapturer(nil)

	req, _ := http.NewRequest("GET", "https://api.example.com/test", nil)
	rec := c.StartCapture(req)
	c.FinishCapture(rec, &http.Response{StatusCode: 200})

	if len(c.Records()) != 1 {
		t.Error("expected 1 record before clear")
	}

	c.Clear()

	if len(c.Records()) != 0 {
		t.Error("expected 0 records after clear")
	}
}

func TestHandler(t *testing.T) {
	c := NewCapturer(nil)

	var handled *Record
	c.AddHandler(func(r *Record) {
		handled = r
	})

	req, _ := http.NewRequest("GET", "https://api.example.com/test", nil)
	rec := c.StartCapture(req)
	c.FinishCapture(rec, &http.Response{StatusCode: 200})

	if handled == nil {
		t.Error("handler should have been called")
	}

	if handled.Request.URL != "https://api.example.com/test" {
		t.Errorf("unexpected URL: %s", handled.Request.URL)
	}
}

func TestJSONOutput(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := &Config{
		Output:         buf,
		Format:         FormatNDJSON,
		IncludeHeaders: false,
		IncludeBody:    false,
	}
	c := NewCapturer(cfg)

	req, _ := http.NewRequest("POST", "https://api.example.com/users", nil)
	rec := c.StartCapture(req)
	c.FinishCapture(rec, &http.Response{StatusCode: 201})

	// Parse output as JSON
	var output Record
	if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if output.Request.Method != "POST" {
		t.Errorf("expected POST, got %s", output.Request.Method)
	}

	if output.Response.Status != 201 {
		t.Errorf("expected 201, got %d", output.Response.Status)
	}
}

func TestRequestBodyCapture(t *testing.T) {
	cfg := &Config{
		Output:         &bytes.Buffer{},
		Format:         FormatNDJSON,
		IncludeHeaders: false,
		IncludeBody:    true,
		MaxBodySize:    1024,
	}
	c := NewCapturer(cfg)

	body := `{"name":"Alice","email":"alice@example.com"}`
	req, _ := http.NewRequest("POST", "https://api.example.com/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	rec := c.StartCapture(req)

	if rec.Request.Body == nil {
		t.Error("expected request body to be captured")
	}

	bodyMap, ok := rec.Request.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", rec.Request.Body)
	}

	if bodyMap["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", bodyMap["name"])
	}
}

func TestSchemeDetection(t *testing.T) {
	c := NewCapturer(nil)

	// HTTP URL
	req1, _ := http.NewRequest("GET", "http://example.com/test", nil)
	rec1 := c.StartCapture(req1)
	if rec1.Request.Scheme != "http" {
		t.Errorf("expected http, got %s", rec1.Request.Scheme)
	}

	// HTTPS URL
	req2, _ := http.NewRequest("GET", "https://example.com/test", nil)
	rec2 := c.StartCapture(req2)
	if rec2.Request.Scheme != "https" {
		t.Errorf("expected https, got %s", rec2.Request.Scheme)
	}
}

func TestQueryParams(t *testing.T) {
	c := NewCapturer(nil)

	u, _ := url.Parse("https://api.example.com/search?q=test&limit=10&offset=0")
	req := &http.Request{
		Method: "GET",
		URL:    u,
		Host:   "api.example.com",
		Header: http.Header{},
	}

	rec := c.StartCapture(req)

	if rec.Request.Query["q"] != "test" {
		t.Errorf("expected q=test, got %v", rec.Request.Query["q"])
	}
	if rec.Request.Query["limit"] != "10" {
		t.Errorf("expected limit=10, got %v", rec.Request.Query["limit"])
	}
	if rec.Request.Query["offset"] != "0" {
		t.Errorf("expected offset=0, got %v", rec.Request.Query["offset"])
	}
}

func TestSkipBinaryRequest(t *testing.T) {
	cfg := &Config{
		Output:         &bytes.Buffer{},
		Format:         FormatNDJSON,
		IncludeHeaders: true,
		IncludeBody:    true,
		MaxBodySize:    1024,
		SkipBinary:     true,
	}
	c := NewCapturer(cfg)

	// PNG magic bytes
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	req, _ := http.NewRequest("POST", "https://api.example.com/upload", bytes.NewReader(pngData))
	req.Header.Set("Content-Type", "image/png")
	req.ContentLength = int64(len(pngData))

	rec := c.StartCapture(req)

	if !rec.Request.IsBinary {
		t.Error("expected IsBinary to be true for PNG data")
	}
	if rec.Request.Body != "[binary content]" {
		t.Errorf("expected '[binary content]', got %v", rec.Request.Body)
	}
	if rec.Request.BodySize != int64(len(pngData)) {
		t.Errorf("expected BodySize %d, got %d", len(pngData), rec.Request.BodySize)
	}
}

func TestSkipBinaryResponse(t *testing.T) {
	cfg := &Config{
		Output:         &bytes.Buffer{},
		Format:         FormatNDJSON,
		IncludeHeaders: true,
		IncludeBody:    true,
		MaxBodySize:    1024,
		SkipBinary:     true,
	}
	c := NewCapturer(cfg)

	req, _ := http.NewRequest("GET", "https://api.example.com/image.png", nil)
	rec := c.StartCapture(req)

	// PNG magic bytes
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	resp := &http.Response{
		StatusCode:    200,
		Status:        "200 OK",
		Header:        http.Header{"Content-Type": []string{"image/png"}},
		Body:          io.NopCloser(bytes.NewReader(pngData)),
		ContentLength: int64(len(pngData)),
	}

	c.FinishCapture(rec, resp)

	if !rec.Response.IsBinary {
		t.Error("expected IsBinary to be true for PNG response")
	}
	if rec.Response.Body != "[binary content]" {
		t.Errorf("expected '[binary content]', got %v", rec.Response.Body)
	}
	if rec.Response.Size != int64(len(pngData)) {
		t.Errorf("expected Size %d, got %d", len(pngData), rec.Response.Size)
	}
}

func TestSkipBinaryDisabled(t *testing.T) {
	cfg := &Config{
		Output:         &bytes.Buffer{},
		Format:         FormatNDJSON,
		IncludeHeaders: true,
		IncludeBody:    true,
		MaxBodySize:    1024,
		SkipBinary:     false, // Disabled
	}
	c := NewCapturer(cfg)

	// PNG magic bytes
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	req, _ := http.NewRequest("POST", "https://api.example.com/upload", bytes.NewReader(pngData))
	req.Header.Set("Content-Type", "image/png")
	req.ContentLength = int64(len(pngData))

	rec := c.StartCapture(req)

	// With SkipBinary disabled, binary content should still be captured (as string)
	if rec.Request.IsBinary {
		t.Error("expected IsBinary to be false when SkipBinary is disabled")
	}
	if rec.Request.Body == "[binary content]" {
		t.Error("expected actual body content when SkipBinary is disabled")
	}
}

func TestTextContentNotSkipped(t *testing.T) {
	cfg := &Config{
		Output:         &bytes.Buffer{},
		Format:         FormatNDJSON,
		IncludeHeaders: true,
		IncludeBody:    true,
		MaxBodySize:    1024,
		SkipBinary:     true,
	}
	c := NewCapturer(cfg)

	jsonData := `{"name": "test", "value": 123}`
	req, _ := http.NewRequest("POST", "https://api.example.com/data", strings.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(jsonData))

	rec := c.StartCapture(req)

	if rec.Request.IsBinary {
		t.Error("expected IsBinary to be false for JSON data")
	}
	if rec.Request.Body == "[binary content]" {
		t.Error("expected actual JSON body, not '[binary content]'")
	}

	// Body should be parsed as JSON
	bodyMap, ok := rec.Request.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", rec.Request.Body)
	}
	if bodyMap["name"] != "test" {
		t.Errorf("expected name=test, got %v", bodyMap["name"])
	}
}
