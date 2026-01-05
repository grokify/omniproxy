// Package contentdetect provides content type detection using multiple methods:
// Content-Type headers, magic bytes (file signatures), and byte analysis heuristics.
//
// This package can be used independently from OmniProxy.
//
// Example usage:
//
//	// Quick check
//	if contentdetect.IsBinary("", data) {
//	    fmt.Println("Binary content detected")
//	}
//
//	// With Content-Type hint
//	if contentdetect.IsBinary("image/png", data) {
//	    fmt.Println("Binary content detected")
//	}
//
//	// Detailed detection
//	info := contentdetect.Detect("", data)
//	fmt.Printf("Binary: %v, MIME: %s, Method: %s\n", info.IsBinary, info.MIMEType, info.Method)
package contentdetect

import (
	"strings"
)

// ContentInfo contains detailed information about detected content.
type ContentInfo struct {
	// IsBinary is true if the content appears to be binary
	IsBinary bool
	// IsText is true if the content appears to be text
	IsText bool
	// MIMEType is the detected or provided MIME type
	MIMEType string
	// Extension is the likely file extension (e.g., "png", "json")
	Extension string
	// Method indicates how the detection was made: "content-type", "magic", "heuristic"
	Method string
	// Confidence is a value from 0.0 to 1.0 indicating detection confidence
	Confidence float64
}

// Options configures the detection behavior.
type Options struct {
	// TrustContentType trusts the Content-Type header if provided and recognized
	TrustContentType bool
	// HighBitThreshold is the ratio of non-printable bytes to consider content binary (default: 0.30)
	HighBitThreshold float64
	// CheckBytes is the number of bytes to examine for detection (default: 512)
	CheckBytes int
}

// DefaultOptions returns the default detection options.
func DefaultOptions() *Options {
	return &Options{
		TrustContentType: true,
		HighBitThreshold: 0.30,
		CheckBytes:       512,
	}
}

// IsBinary returns true if the content appears to be binary.
// contentType is optional and can be empty.
func IsBinary(contentType string, data []byte) bool {
	return IsBinaryWithOptions(contentType, data, nil)
}

// IsBinaryWithOptions returns true if the content appears to be binary using custom options.
func IsBinaryWithOptions(contentType string, data []byte, opts *Options) bool {
	info := DetectWithOptions(contentType, data, opts)
	return info.IsBinary
}

// IsText returns true if the content appears to be text.
// contentType is optional and can be empty.
func IsText(contentType string, data []byte) bool {
	return !IsBinary(contentType, data)
}

// Detect analyzes content and returns detailed information.
// contentType is optional and can be empty.
func Detect(contentType string, data []byte) ContentInfo {
	return DetectWithOptions(contentType, data, nil)
}

// DetectWithOptions analyzes content with custom options.
func DetectWithOptions(contentType string, data []byte, opts *Options) ContentInfo {
	if opts == nil {
		opts = DefaultOptions()
	}

	// Normalize content type
	contentType = normalizeContentType(contentType)

	// Method 1: Check Content-Type header
	if opts.TrustContentType && contentType != "" {
		if info, ok := detectByContentType(contentType); ok {
			return info
		}
	}

	// Method 2: Check magic bytes
	if len(data) > 0 {
		if info, ok := detectByMagicBytes(data); ok {
			return info
		}
	}

	// Method 3: Heuristic analysis
	return detectByHeuristic(data, opts)
}

// normalizeContentType extracts and lowercases the MIME type from a Content-Type header.
func normalizeContentType(ct string) string {
	// Remove parameters (e.g., "text/html; charset=utf-8" -> "text/html")
	if idx := strings.Index(ct, ";"); idx != -1 {
		ct = ct[:idx]
	}
	return strings.TrimSpace(strings.ToLower(ct))
}

// detectByContentType checks if the content type indicates binary or text.
func detectByContentType(ct string) (ContentInfo, bool) {
	// Known binary types
	binaryTypes := map[string]string{
		// Images
		"image/png":                "png",
		"image/jpeg":               "jpg",
		"image/gif":                "gif",
		"image/webp":               "webp",
		"image/bmp":                "bmp",
		"image/tiff":               "tiff",
		"image/x-icon":             "ico",
		"image/avif":               "avif",
		"image/heic":               "heic",
		"image/heif":               "heif",
		// Audio
		"audio/mpeg":               "mp3",
		"audio/wav":                "wav",
		"audio/ogg":                "ogg",
		"audio/webm":               "webm",
		"audio/flac":               "flac",
		"audio/aac":                "aac",
		"audio/mp4":                "m4a",
		// Video
		"video/mp4":                "mp4",
		"video/webm":               "webm",
		"video/ogg":                "ogv",
		"video/quicktime":          "mov",
		"video/x-msvideo":          "avi",
		"video/x-matroska":         "mkv",
		// Archives
		"application/zip":          "zip",
		"application/gzip":         "gz",
		"application/x-tar":        "tar",
		"application/x-rar-compressed": "rar",
		"application/x-7z-compressed":  "7z",
		"application/x-bzip2":      "bz2",
		"application/x-xz":         "xz",
		// Documents
		"application/pdf":          "pdf",
		"application/msword":       "doc",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "docx",
		"application/vnd.ms-excel": "xls",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       "xlsx",
		"application/vnd.ms-powerpoint": "ppt",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation": "pptx",
		// Executables
		"application/octet-stream": "",
		"application/x-executable": "exe",
		"application/x-mach-binary": "",
		"application/x-elf":        "",
		"application/x-dosexec":    "exe",
		// Fonts
		"font/woff":                "woff",
		"font/woff2":               "woff2",
		"font/ttf":                 "ttf",
		"font/otf":                 "otf",
		"application/font-woff":    "woff",
		"application/font-woff2":   "woff2",
		// Other binary
		"application/x-shockwave-flash": "swf",
		"application/wasm":         "wasm",
	}

	// Known text types
	textTypes := map[string]string{
		"text/plain":               "txt",
		"text/html":                "html",
		"text/css":                 "css",
		"text/javascript":          "js",
		"text/xml":                 "xml",
		"text/csv":                 "csv",
		"text/markdown":            "md",
		"text/calendar":            "ics",
		"application/json":         "json",
		"application/xml":          "xml",
		"application/javascript":   "js",
		"application/x-javascript": "js",
		"application/ecmascript":   "js",
		"application/ld+json":      "jsonld",
		"application/x-yaml":       "yaml",
		"application/yaml":         "yaml",
		"application/x-www-form-urlencoded": "",
		"application/graphql":      "graphql",
		"image/svg+xml":            "svg",
	}

	// Check binary types
	if ext, ok := binaryTypes[ct]; ok {
		return ContentInfo{
			IsBinary:   true,
			IsText:     false,
			MIMEType:   ct,
			Extension:  ext,
			Method:     "content-type",
			Confidence: 0.9,
		}, true
	}

	// Check text types
	if ext, ok := textTypes[ct]; ok {
		return ContentInfo{
			IsBinary:   false,
			IsText:     true,
			MIMEType:   ct,
			Extension:  ext,
			Method:     "content-type",
			Confidence: 0.9,
		}, true
	}

	// Check type prefixes
	if strings.HasPrefix(ct, "text/") {
		return ContentInfo{
			IsBinary:   false,
			IsText:     true,
			MIMEType:   ct,
			Method:     "content-type",
			Confidence: 0.8,
		}, true
	}

	// Check image/audio/video prefixes, but exclude SVG which is text-based
	if strings.HasPrefix(ct, "audio/") || strings.HasPrefix(ct, "video/") {
		return ContentInfo{
			IsBinary:   true,
			IsText:     false,
			MIMEType:   ct,
			Method:     "content-type",
			Confidence: 0.8,
		}, true
	}
	if strings.HasPrefix(ct, "image/") && ct != "image/svg+xml" {
		return ContentInfo{
			IsBinary:   true,
			IsText:     false,
			MIMEType:   ct,
			Method:     "content-type",
			Confidence: 0.8,
		}, true
	}

	// Check for +json or +xml suffixes (e.g., application/vnd.api+json)
	if strings.HasSuffix(ct, "+json") || strings.HasSuffix(ct, "+xml") {
		return ContentInfo{
			IsBinary:   false,
			IsText:     true,
			MIMEType:   ct,
			Method:     "content-type",
			Confidence: 0.85,
		}, true
	}

	return ContentInfo{}, false
}
