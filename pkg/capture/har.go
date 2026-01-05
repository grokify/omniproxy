package capture

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// HAR represents an HTTP Archive file.
type HAR struct {
	Log HARLog `json:"log"`
}

// HARLog is the root of the HAR format.
type HARLog struct {
	Version string     `json:"version"`
	Creator HARCreator `json:"creator"`
	Entries []HAREntry `json:"entries"`
}

// HARCreator identifies the tool that created the HAR.
type HARCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// HAREntry represents a single HTTP transaction.
type HAREntry struct {
	StartedDateTime string      `json:"startedDateTime"`
	Time            float64     `json:"time"`
	Request         HARRequest  `json:"request"`
	Response        HARResponse `json:"response"`
	Cache           HARCache    `json:"cache"`
	Timings         HARTimings  `json:"timings"`
}

// HARRequest represents an HTTP request.
type HARRequest struct {
	Method      string         `json:"method"`
	URL         string         `json:"url"`
	HTTPVersion string         `json:"httpVersion"`
	Cookies     []HARCookie    `json:"cookies"`
	Headers     []HARHeader    `json:"headers"`
	QueryString []HARQueryPair `json:"queryString"`
	PostData    *HARPostData   `json:"postData,omitempty"`
	HeadersSize int            `json:"headersSize"`
	BodySize    int            `json:"bodySize"`
}

// HARResponse represents an HTTP response.
type HARResponse struct {
	Status      int          `json:"status"`
	StatusText  string       `json:"statusText"`
	HTTPVersion string       `json:"httpVersion"`
	Cookies     []HARCookie  `json:"cookies"`
	Headers     []HARHeader  `json:"headers"`
	Content     HARContent   `json:"content"`
	RedirectURL string       `json:"redirectURL"`
	HeadersSize int          `json:"headersSize"`
	BodySize    int          `json:"bodySize"`
}

// HARHeader represents an HTTP header.
type HARHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// HARCookie represents a cookie.
type HARCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Expires  string `json:"expires,omitempty"`
	HTTPOnly bool   `json:"httpOnly,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
}

// HARQueryPair represents a query string parameter.
type HARQueryPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// HARPostData represents POST data.
type HARPostData struct {
	MimeType string         `json:"mimeType"`
	Text     string         `json:"text,omitempty"`
	Params   []HARPostParam `json:"params,omitempty"`
}

// HARPostParam represents a POST parameter.
type HARPostParam struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	FileName    string `json:"fileName,omitempty"`
	ContentType string `json:"contentType,omitempty"`
}

// HARContent represents response content.
type HARContent struct {
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text,omitempty"`
	Encoding string `json:"encoding,omitempty"`
}

// HARCache represents cache information.
type HARCache struct{}

// HARTimings represents timing information.
type HARTimings struct {
	Blocked float64 `json:"blocked"`
	DNS     float64 `json:"dns"`
	Connect float64 `json:"connect"`
	Send    float64 `json:"send"`
	Wait    float64 `json:"wait"`
	Receive float64 `json:"receive"`
	SSL     float64 `json:"ssl"`
}

// HARWriter writes records in HAR format.
type HARWriter struct {
	mu      sync.Mutex
	entries []HAREntry
	output  io.Writer
}

// NewHARWriter creates a new HAR writer.
func NewHARWriter(output io.Writer) *HARWriter {
	return &HARWriter{
		entries: make([]HAREntry, 0),
		output:  output,
	}
}

// AddRecord converts a capture Record to HAR format and adds it.
func (w *HARWriter) AddRecord(rec *Record) {
	entry := w.recordToHAREntry(rec)

	w.mu.Lock()
	w.entries = append(w.entries, entry)
	w.mu.Unlock()
}

// Write outputs the complete HAR file.
func (w *HARWriter) Write() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	har := HAR{
		Log: HARLog{
			Version: "1.2",
			Creator: HARCreator{
				Name:    "OmniProxy",
				Version: "0.1.0",
			},
			Entries: w.entries,
		},
	}

	encoder := json.NewEncoder(w.output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(har)
}

// recordToHAREntry converts a Record to a HAREntry.
func (w *HARWriter) recordToHAREntry(rec *Record) HAREntry {
	entry := HAREntry{
		StartedDateTime: rec.StartTime.Format(time.RFC3339Nano),
		Time:            rec.DurationMs,
		Request: HARRequest{
			Method:      rec.Request.Method,
			URL:         rec.Request.URL,
			HTTPVersion: "HTTP/1.1",
			Cookies:     []HARCookie{},
			Headers:     headersToHAR(rec.Request.Headers),
			QueryString: queryToHAR(rec.Request.Query),
			HeadersSize: -1,
			BodySize:    0,
		},
		Response: HARResponse{
			Status:      rec.Response.Status,
			StatusText:  rec.Response.StatusText,
			HTTPVersion: "HTTP/1.1",
			Cookies:     []HARCookie{},
			Headers:     headersToHAR(rec.Response.Headers),
			Content: HARContent{
				Size:     rec.Response.Size,
				MimeType: rec.Response.ContentType,
			},
			RedirectURL: "",
			HeadersSize: -1,
			BodySize:    int(rec.Response.Size),
		},
		Cache: HARCache{},
		Timings: HARTimings{
			Blocked: -1,
			DNS:     -1,
			Connect: -1,
			Send:    0,
			Wait:    rec.DurationMs,
			Receive: 0,
			SSL:     -1,
		},
	}

	// Add request body
	if rec.Request.Body != nil {
		bodyText := bodyToString(rec.Request.Body)
		entry.Request.PostData = &HARPostData{
			MimeType: rec.Request.ContentType,
			Text:     bodyText,
		}
		entry.Request.BodySize = len(bodyText)
	}

	// Add response body
	if rec.Response.Body != nil {
		entry.Response.Content.Text = bodyToString(rec.Response.Body)
	}

	return entry
}

// headersToHAR converts a header map to HAR headers.
func headersToHAR(headers map[string]string) []HARHeader {
	if headers == nil {
		return []HARHeader{}
	}

	result := make([]HARHeader, 0, len(headers))
	for name, value := range headers {
		result = append(result, HARHeader{Name: name, Value: value})
	}
	return result
}

// queryToHAR converts a query map to HAR query pairs.
func queryToHAR(query map[string]string) []HARQueryPair {
	if query == nil {
		return []HARQueryPair{}
	}

	result := make([]HARQueryPair, 0, len(query))
	for name, value := range query {
		result = append(result, HARQueryPair{Name: name, Value: value})
	}
	return result
}

// bodyToString converts a body interface to string.
func bodyToString(body interface{}) string {
	if body == nil {
		return ""
	}

	switch v := body.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		// Try JSON encoding
		data, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(data)
	}
}
