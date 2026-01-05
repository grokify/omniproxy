package contentdetect

// magicSignature represents a file signature (magic bytes).
type magicSignature struct {
	bytes     []byte
	mask      []byte // optional mask for partial matching
	offset    int    // offset from start of file
	mimeType  string
	extension string
	isBinary  bool
}

// magicSignatures contains known file signatures ordered by specificity.
// More specific signatures should come before less specific ones.
var magicSignatures = []magicSignature{
	// Images
	{bytes: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, mimeType: "image/png", extension: "png", isBinary: true},
	{bytes: []byte{0xFF, 0xD8, 0xFF}, mimeType: "image/jpeg", extension: "jpg", isBinary: true},
	{bytes: []byte{0x47, 0x49, 0x46, 0x38, 0x37, 0x61}, mimeType: "image/gif", extension: "gif", isBinary: true}, // GIF87a
	{bytes: []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61}, mimeType: "image/gif", extension: "gif", isBinary: true}, // GIF89a
	{bytes: []byte{0x52, 0x49, 0x46, 0x46}, mimeType: "image/webp", extension: "webp", isBinary: true},           // RIFF (check WEBP later)
	{bytes: []byte{0x42, 0x4D}, mimeType: "image/bmp", extension: "bmp", isBinary: true},                         // BM
	{bytes: []byte{0x49, 0x49, 0x2A, 0x00}, mimeType: "image/tiff", extension: "tiff", isBinary: true},           // Little-endian TIFF
	{bytes: []byte{0x4D, 0x4D, 0x00, 0x2A}, mimeType: "image/tiff", extension: "tiff", isBinary: true},           // Big-endian TIFF
	{bytes: []byte{0x00, 0x00, 0x01, 0x00}, mimeType: "image/x-icon", extension: "ico", isBinary: true},          // ICO
	{bytes: []byte{0x00, 0x00, 0x02, 0x00}, mimeType: "image/x-icon", extension: "cur", isBinary: true},          // CUR

	// Audio
	{bytes: []byte{0x49, 0x44, 0x33}, mimeType: "audio/mpeg", extension: "mp3", isBinary: true},                     // ID3 tag
	{bytes: []byte{0xFF, 0xFB}, mimeType: "audio/mpeg", extension: "mp3", isBinary: true},                           // MP3 frame sync
	{bytes: []byte{0xFF, 0xFA}, mimeType: "audio/mpeg", extension: "mp3", isBinary: true},                           // MP3 frame sync
	{bytes: []byte{0xFF, 0xF3}, mimeType: "audio/mpeg", extension: "mp3", isBinary: true},                           // MP3 frame sync
	{bytes: []byte{0xFF, 0xF2}, mimeType: "audio/mpeg", extension: "mp3", isBinary: true},                           // MP3 frame sync
	{bytes: []byte{0x4F, 0x67, 0x67, 0x53}, mimeType: "audio/ogg", extension: "ogg", isBinary: true},                // OggS
	{bytes: []byte{0x66, 0x4C, 0x61, 0x43}, mimeType: "audio/flac", extension: "flac", isBinary: true},              // fLaC
	{bytes: []byte{0x52, 0x49, 0x46, 0x46}, mimeType: "audio/wav", extension: "wav", isBinary: true},                // RIFF (check WAVE later)
	{bytes: []byte{0x00, 0x00, 0x00, 0x20, 0x66, 0x74, 0x79, 0x70}, mimeType: "audio/mp4", extension: "m4a", isBinary: true},

	// Video
	{bytes: []byte{0x00, 0x00, 0x00, 0x1C, 0x66, 0x74, 0x79, 0x70}, mimeType: "video/mp4", extension: "mp4", isBinary: true},
	{bytes: []byte{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70}, mimeType: "video/mp4", extension: "mp4", isBinary: true},
	{bytes: []byte{0x00, 0x00, 0x00, 0x14, 0x66, 0x74, 0x79, 0x70}, mimeType: "video/mp4", extension: "mp4", isBinary: true},
	{bytes: []byte{0x1A, 0x45, 0xDF, 0xA3}, mimeType: "video/webm", extension: "webm", isBinary: true},              // EBML (WebM/MKV)
	{bytes: []byte{0x00, 0x00, 0x00, 0x14, 0x66, 0x74, 0x79, 0x70, 0x71, 0x74}, mimeType: "video/quicktime", extension: "mov", isBinary: true},
	{bytes: []byte{0x52, 0x49, 0x46, 0x46}, mimeType: "video/x-msvideo", extension: "avi", isBinary: true},          // RIFF (check AVI later)
	{bytes: []byte{0x46, 0x4C, 0x56, 0x01}, mimeType: "video/x-flv", extension: "flv", isBinary: true},              // FLV

	// Archives
	{bytes: []byte{0x50, 0x4B, 0x03, 0x04}, mimeType: "application/zip", extension: "zip", isBinary: true},          // PK.. (ZIP)
	{bytes: []byte{0x50, 0x4B, 0x05, 0x06}, mimeType: "application/zip", extension: "zip", isBinary: true},          // Empty ZIP
	{bytes: []byte{0x50, 0x4B, 0x07, 0x08}, mimeType: "application/zip", extension: "zip", isBinary: true},          // Spanned ZIP
	{bytes: []byte{0x1F, 0x8B, 0x08}, mimeType: "application/gzip", extension: "gz", isBinary: true},                // Gzip
	{bytes: []byte{0x42, 0x5A, 0x68}, mimeType: "application/x-bzip2", extension: "bz2", isBinary: true},            // BZh
	{bytes: []byte{0xFD, 0x37, 0x7A, 0x58, 0x5A, 0x00}, mimeType: "application/x-xz", extension: "xz", isBinary: true},
	{bytes: []byte{0x37, 0x7A, 0xBC, 0xAF, 0x27, 0x1C}, mimeType: "application/x-7z-compressed", extension: "7z", isBinary: true},
	{bytes: []byte{0x52, 0x61, 0x72, 0x21, 0x1A, 0x07, 0x00}, mimeType: "application/x-rar-compressed", extension: "rar", isBinary: true},      // RAR 1.5-4.0
	{bytes: []byte{0x52, 0x61, 0x72, 0x21, 0x1A, 0x07, 0x01, 0x00}, mimeType: "application/x-rar-compressed", extension: "rar", isBinary: true}, // RAR 5.0+
	{bytes: []byte{0x75, 0x73, 0x74, 0x61, 0x72}, offset: 257, mimeType: "application/x-tar", extension: "tar", isBinary: true},                 // ustar (TAR)

	// Documents
	{bytes: []byte{0x25, 0x50, 0x44, 0x46, 0x2D}, mimeType: "application/pdf", extension: "pdf", isBinary: true},    // %PDF-
	{bytes: []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, mimeType: "application/msword", extension: "doc", isBinary: true}, // OLE2 (DOC, XLS, PPT)

	// Executables
	{bytes: []byte{0x4D, 0x5A}, mimeType: "application/x-dosexec", extension: "exe", isBinary: true},                // MZ (DOS/Windows executable)
	{bytes: []byte{0x7F, 0x45, 0x4C, 0x46}, mimeType: "application/x-elf", extension: "", isBinary: true},           // ELF (Linux executable)
	{bytes: []byte{0xCF, 0xFA, 0xED, 0xFE}, mimeType: "application/x-mach-binary", extension: "", isBinary: true},   // Mach-O 32-bit
	{bytes: []byte{0xCE, 0xFA, 0xED, 0xFE}, mimeType: "application/x-mach-binary", extension: "", isBinary: true},   // Mach-O 32-bit (reverse)
	{bytes: []byte{0xCA, 0xFE, 0xBA, 0xBE}, mimeType: "application/x-mach-binary", extension: "", isBinary: true},   // Mach-O Universal/Java class
	{bytes: []byte{0xFE, 0xED, 0xFA, 0xCF}, mimeType: "application/x-mach-binary", extension: "", isBinary: true},   // Mach-O 64-bit
	{bytes: []byte{0xFE, 0xED, 0xFA, 0xCE}, mimeType: "application/x-mach-binary", extension: "", isBinary: true},   // Mach-O 64-bit (reverse)

	// Fonts
	{bytes: []byte{0x77, 0x4F, 0x46, 0x46}, mimeType: "font/woff", extension: "woff", isBinary: true},               // wOFF
	{bytes: []byte{0x77, 0x4F, 0x46, 0x32}, mimeType: "font/woff2", extension: "woff2", isBinary: true},             // wOF2
	{bytes: []byte{0x00, 0x01, 0x00, 0x00}, mimeType: "font/ttf", extension: "ttf", isBinary: true},                 // TrueType
	{bytes: []byte{0x4F, 0x54, 0x54, 0x4F}, mimeType: "font/otf", extension: "otf", isBinary: true},                 // OTTO (OpenType)

	// Other binary
	{bytes: []byte{0x00, 0x61, 0x73, 0x6D}, mimeType: "application/wasm", extension: "wasm", isBinary: true},        // WebAssembly
	{bytes: []byte{0x46, 0x57, 0x53}, mimeType: "application/x-shockwave-flash", extension: "swf", isBinary: true},  // FWS (uncompressed SWF)
	{bytes: []byte{0x43, 0x57, 0x53}, mimeType: "application/x-shockwave-flash", extension: "swf", isBinary: true},  // CWS (compressed SWF)
	{bytes: []byte{0x53, 0x51, 0x4C, 0x69, 0x74, 0x65, 0x20, 0x66, 0x6F, 0x72, 0x6D, 0x61, 0x74, 0x20, 0x33, 0x00}, mimeType: "application/x-sqlite3", extension: "sqlite", isBinary: true}, // SQLite

	// Text formats with specific signatures (these are text, not binary)
	// Note: We detect these to return accurate MIME types, but isBinary=false
}

// textSignatures contains signatures for text-based formats.
var textSignatures = []magicSignature{
	// XML-based formats
	{bytes: []byte("<?xml"), mimeType: "application/xml", extension: "xml", isBinary: false},
	{bytes: []byte("<svg"), mimeType: "image/svg+xml", extension: "svg", isBinary: false},
	{bytes: []byte("<!DOCTYPE svg"), mimeType: "image/svg+xml", extension: "svg", isBinary: false},
	{bytes: []byte("<!DOCTYPE html"), mimeType: "text/html", extension: "html", isBinary: false},
	{bytes: []byte("<!doctype html"), mimeType: "text/html", extension: "html", isBinary: false},
	{bytes: []byte("<html"), mimeType: "text/html", extension: "html", isBinary: false},
	{bytes: []byte("<HTML"), mimeType: "text/html", extension: "html", isBinary: false},
	{bytes: []byte("<!DOCTYPE HTML"), mimeType: "text/html", extension: "html", isBinary: false},

	// JSON (starts with { or [)
	// Handled in heuristic since whitespace can precede

	// Shell scripts
	{bytes: []byte("#!/bin/bash"), mimeType: "application/x-sh", extension: "sh", isBinary: false},
	{bytes: []byte("#!/bin/sh"), mimeType: "application/x-sh", extension: "sh", isBinary: false},
	{bytes: []byte("#!/usr/bin/env"), mimeType: "application/x-sh", extension: "sh", isBinary: false},

	// Other text
	{bytes: []byte("%!PS"), mimeType: "application/postscript", extension: "ps", isBinary: false},
	{bytes: []byte("{\\rtf"), mimeType: "application/rtf", extension: "rtf", isBinary: false},
}

// detectByMagicBytes checks the data against known file signatures.
func detectByMagicBytes(data []byte) (ContentInfo, bool) {
	if len(data) == 0 {
		return ContentInfo{}, false
	}

	// Check binary signatures first
	for _, sig := range magicSignatures {
		if matchSignature(data, sig) {
			return ContentInfo{
				IsBinary:   sig.isBinary,
				IsText:     !sig.isBinary,
				MIMEType:   sig.mimeType,
				Extension:  sig.extension,
				Method:     "magic",
				Confidence: 0.95,
			}, true
		}
	}

	// Check text signatures
	for _, sig := range textSignatures {
		if matchSignature(data, sig) {
			return ContentInfo{
				IsBinary:   sig.isBinary,
				IsText:     !sig.isBinary,
				MIMEType:   sig.mimeType,
				Extension:  sig.extension,
				Method:     "magic",
				Confidence: 0.90,
			}, true
		}
	}

	return ContentInfo{}, false
}

// matchSignature checks if data matches a signature.
func matchSignature(data []byte, sig magicSignature) bool {
	offset := sig.offset
	if offset+len(sig.bytes) > len(data) {
		return false
	}

	chunk := data[offset : offset+len(sig.bytes)]

	if len(sig.mask) > 0 {
		// Apply mask for partial matching
		for i := 0; i < len(sig.bytes); i++ {
			if (chunk[i] & sig.mask[i]) != (sig.bytes[i] & sig.mask[i]) {
				return false
			}
		}
		return true
	}

	// Direct comparison
	for i := 0; i < len(sig.bytes); i++ {
		if chunk[i] != sig.bytes[i] {
			return false
		}
	}
	return true
}
