package contentdetect

import (
	"testing"
)

func TestIsBinaryByContentType(t *testing.T) {
	tests := []struct {
		contentType string
		wantBinary  bool
	}{
		// Binary types
		{"image/png", true},
		{"image/jpeg", true},
		{"image/gif", true},
		{"audio/mpeg", true},
		{"video/mp4", true},
		{"application/pdf", true},
		{"application/zip", true},
		{"application/octet-stream", true},
		{"font/woff2", true},

		// Text types
		{"text/plain", false},
		{"text/html", false},
		{"text/css", false},
		{"text/javascript", false},
		{"application/json", false},
		{"application/xml", false},
		{"application/javascript", false},
		{"image/svg+xml", false}, // SVG is text

		// Content-type with parameters
		{"text/html; charset=utf-8", false},
		{"application/json; charset=utf-8", false},
		{"image/png; name=image.png", true},

		// +json and +xml suffixes
		{"application/vnd.api+json", false},
		{"application/atom+xml", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			got := IsBinary(tt.contentType, nil)
			if got != tt.wantBinary {
				t.Errorf("IsBinary(%q, nil) = %v, want %v", tt.contentType, got, tt.wantBinary)
			}
		})
	}
}

func TestIsBinaryByMagicBytes(t *testing.T) {
	tests := []struct {
		name       string
		data       []byte
		wantBinary bool
		wantMIME   string
	}{
		// Images
		{
			name:       "PNG",
			data:       []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00},
			wantBinary: true,
			wantMIME:   "image/png",
		},
		{
			name:       "JPEG",
			data:       []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46},
			wantBinary: true,
			wantMIME:   "image/jpeg",
		},
		{
			name:       "GIF89a",
			data:       []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00},
			wantBinary: true,
			wantMIME:   "image/gif",
		},

		// Archives
		{
			name:       "ZIP",
			data:       []byte{0x50, 0x4B, 0x03, 0x04, 0x14, 0x00, 0x00, 0x00},
			wantBinary: true,
			wantMIME:   "application/zip",
		},
		{
			name:       "GZIP",
			data:       []byte{0x1F, 0x8B, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00},
			wantBinary: true,
			wantMIME:   "application/gzip",
		},

		// Documents
		{
			name:       "PDF",
			data:       []byte{0x25, 0x50, 0x44, 0x46, 0x2D, 0x31, 0x2E, 0x34},
			wantBinary: true,
			wantMIME:   "application/pdf",
		},

		// Executables
		{
			name:       "ELF",
			data:       []byte{0x7F, 0x45, 0x4C, 0x46, 0x02, 0x01, 0x01, 0x00},
			wantBinary: true,
			wantMIME:   "application/x-elf",
		},
		{
			name:       "MachO",
			data:       []byte{0xCF, 0xFA, 0xED, 0xFE, 0x07, 0x00, 0x00, 0x01},
			wantBinary: true,
			wantMIME:   "application/x-mach-binary",
		},
		{
			name:       "PE/EXE",
			data:       []byte{0x4D, 0x5A, 0x90, 0x00, 0x03, 0x00, 0x00, 0x00},
			wantBinary: true,
			wantMIME:   "application/x-dosexec",
		},

		// WebAssembly
		{
			name:       "WASM",
			data:       []byte{0x00, 0x61, 0x73, 0x6D, 0x01, 0x00, 0x00, 0x00},
			wantBinary: true,
			wantMIME:   "application/wasm",
		},

		// Fonts
		{
			name:       "WOFF2",
			data:       []byte{0x77, 0x4F, 0x46, 0x32, 0x00, 0x00, 0x00, 0x00},
			wantBinary: true,
			wantMIME:   "font/woff2",
		},

		// Text formats detected by magic
		{
			name:       "XML",
			data:       []byte("<?xml version=\"1.0\"?>"),
			wantBinary: false,
			wantMIME:   "application/xml",
		},
		{
			name:       "HTML",
			data:       []byte("<!DOCTYPE html><html>"),
			wantBinary: false,
			wantMIME:   "text/html",
		},
		{
			name:       "SVG",
			data:       []byte("<svg xmlns=\"http://www.w3.org/2000/svg\">"),
			wantBinary: false,
			wantMIME:   "image/svg+xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := Detect("", tt.data)
			if info.IsBinary != tt.wantBinary {
				t.Errorf("Detect(%q) IsBinary = %v, want %v", tt.name, info.IsBinary, tt.wantBinary)
			}
			if tt.wantMIME != "" && info.MIMEType != tt.wantMIME {
				t.Errorf("Detect(%q) MIMEType = %q, want %q", tt.name, info.MIMEType, tt.wantMIME)
			}
		})
	}
}

func TestIsBinaryByHeuristic(t *testing.T) {
	tests := []struct {
		name       string
		data       []byte
		wantBinary bool
	}{
		{
			name:       "plain text",
			data:       []byte("Hello, World! This is plain text."),
			wantBinary: false,
		},
		{
			name:       "JSON object",
			data:       []byte(`{"name": "test", "value": 123}`),
			wantBinary: false,
		},
		{
			name:       "JSON array",
			data:       []byte(`[1, 2, 3, "four", "five"]`),
			wantBinary: false,
		},
		{
			name:       "JSON with whitespace",
			data:       []byte("  \n\t{\"key\": \"value\"}\n"),
			wantBinary: false,
		},
		{
			name:       "UTF-8 text",
			data:       []byte("日本語テキスト Hello 世界"),
			wantBinary: false,
		},
		{
			name:       "binary with nulls",
			data:       []byte{0x00, 0x01, 0x02, 0x03, 0x00, 0x00, 0x04, 0x05},
			wantBinary: true,
		},
		{
			name:       "binary random bytes",
			data:       []byte{0x89, 0xAB, 0xCD, 0xEF, 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC},
			wantBinary: true,
		},
		{
			name:       "control characters",
			data:       []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07},
			wantBinary: true,
		},
		{
			name:       "empty",
			data:       []byte{},
			wantBinary: false,
		},
		{
			name:       "whitespace only",
			data:       []byte("   \n\t\r\n   "),
			wantBinary: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use empty content type to force heuristic
			got := IsBinary("", tt.data)
			if got != tt.wantBinary {
				t.Errorf("IsBinary(\"\", %q) = %v, want %v", tt.name, got, tt.wantBinary)
			}
		})
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		data        []byte
		wantMethod  string
	}{
		{
			name:        "content-type takes precedence",
			contentType: "application/json",
			data:        []byte{0x89, 0x50, 0x4E, 0x47}, // PNG magic, but CT says JSON
			wantMethod:  "content-type",
		},
		{
			name:        "magic bytes when no content-type",
			contentType: "",
			data:        []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			wantMethod:  "magic",
		},
		{
			name:        "heuristic when unknown",
			contentType: "",
			data:        []byte("Just some plain text without magic bytes"),
			wantMethod:  "heuristic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := Detect(tt.contentType, tt.data)
			if info.Method != tt.wantMethod {
				t.Errorf("Detect() Method = %q, want %q", info.Method, tt.wantMethod)
			}
		})
	}
}

func TestDetectWithOptions(t *testing.T) {
	opts := &Options{
		TrustContentType: false, // Don't trust content-type
		HighBitThreshold: 0.30,
		CheckBytes:       512,
	}

	// Even with content-type, should use magic bytes
	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	info := DetectWithOptions("text/plain", data, opts)

	if info.Method != "magic" {
		t.Errorf("expected magic method when TrustContentType=false, got %q", info.Method)
	}
	if info.MIMEType != "image/png" {
		t.Errorf("expected image/png, got %q", info.MIMEType)
	}
}

func TestIsValidUTF8(t *testing.T) {
	tests := []struct {
		name  string
		data  []byte
		valid bool
	}{
		{"ASCII", []byte("Hello World"), true},
		{"UTF-8 2-byte", []byte("Héllo"), true},
		{"UTF-8 3-byte", []byte("日本語"), true},
		{"UTF-8 4-byte", []byte("😀"), true},
		{"Invalid continuation", []byte{0xC0, 0x80}, false},
		{"Overlong encoding", []byte{0xC0, 0xAF}, false},
		{"Invalid start byte", []byte{0xFF, 0xFE}, false},
		{"Truncated", []byte{0xE0, 0xA0}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidUTF8(tt.data)
			if got != tt.valid {
				t.Errorf("isValidUTF8(%v) = %v, want %v", tt.data, got, tt.valid)
			}
		})
	}
}

func TestLooksLikeJSON(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"empty object", []byte("{}"), true},
		{"empty array", []byte("[]"), true},
		{"simple object", []byte(`{"key": "value"}`), true},
		{"nested object", []byte(`{"a": {"b": "c"}}`), true},
		{"array of numbers", []byte(`[1, 2, 3]`), true},
		{"with whitespace", []byte("  \n  {\"key\": \"value\"}  \n"), true},
		{"not json - text", []byte("hello world"), false},
		{"not json - xml", []byte("<xml>data</xml>"), false},
		{"truncated", []byte(`{"key": "val`), true}, // Partial JSON is still "looks like"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikeJSON(tt.data)
			if got != tt.want {
				t.Errorf("looksLikeJSON(%q) = %v, want %v", string(tt.data), got, tt.want)
			}
		})
	}
}

func TestNormalizeContentType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"text/html", "text/html"},
		{"TEXT/HTML", "text/html"},
		{"text/html; charset=utf-8", "text/html"},
		{"application/json; charset=utf-8; boundary=something", "application/json"},
		{"  image/png  ", "image/png"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeContentType(tt.input)
			if got != tt.want {
				t.Errorf("normalizeContentType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsText(t *testing.T) {
	// IsText should be the opposite of IsBinary
	if !IsText("text/plain", nil) {
		t.Error("text/plain should be text")
	}
	if IsText("image/png", nil) {
		t.Error("image/png should not be text")
	}
}

func BenchmarkDetect(b *testing.B) {
	data := []byte(`{"name": "benchmark", "values": [1, 2, 3, 4, 5]}`)

	b.Run("with content-type", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			Detect("application/json", data)
		}
	})

	b.Run("magic bytes only", func(b *testing.B) {
		pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		for i := 0; i < b.N; i++ {
			Detect("", pngData)
		}
	})

	b.Run("heuristic", func(b *testing.B) {
		textData := []byte("Just some plain text that needs heuristic analysis")
		for i := 0; i < b.N; i++ {
			Detect("", textData)
		}
	})
}
