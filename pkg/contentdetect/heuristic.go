package contentdetect

// detectByHeuristic analyzes byte patterns to determine if content is binary or text.
func detectByHeuristic(data []byte, opts *Options) ContentInfo {
	if len(data) == 0 {
		// Empty content is considered text
		return ContentInfo{
			IsBinary:   false,
			IsText:     true,
			Method:     "heuristic",
			Confidence: 0.5,
		}
	}

	// Limit analysis to CheckBytes
	checkLen := opts.CheckBytes
	if checkLen > len(data) {
		checkLen = len(data)
	}
	sample := data[:checkLen]

	// Count different byte categories
	var (
		nullBytes      int
		controlBytes   int
		highBitBytes   int
		printableBytes int
		whitespace     int
	)

	for _, b := range sample {
		switch {
		case b == 0:
			nullBytes++
		case b < 0x09:
			controlBytes++
		case b == 0x09 || b == 0x0A || b == 0x0D: // tab, newline, carriage return
			whitespace++
			printableBytes++
		case b < 0x20:
			controlBytes++
		case b < 0x7F:
			printableBytes++
		case b == 0x7F:
			controlBytes++
		default: // >= 0x80
			highBitBytes++
		}
	}

	totalBytes := len(sample)

	// Check for null bytes - strong indicator of binary
	if nullBytes > 0 {
		// Allow a few null bytes at the end (padding) but not in the middle
		nullRatio := float64(nullBytes) / float64(totalBytes)
		if nullRatio > 0.01 { // More than 1% null bytes
			return ContentInfo{
				IsBinary:   true,
				IsText:     false,
				Method:     "heuristic",
				Confidence: 0.85,
			}
		}
	}

	// Check control character ratio
	controlRatio := float64(controlBytes) / float64(totalBytes)
	if controlRatio > 0.05 { // More than 5% control characters
		return ContentInfo{
			IsBinary:   true,
			IsText:     false,
			Method:     "heuristic",
			Confidence: 0.80,
		}
	}

	// Check high-bit ratio (non-ASCII bytes)
	highBitRatio := float64(highBitBytes) / float64(totalBytes)

	// High-bit bytes could be UTF-8, so we need to check if they form valid UTF-8
	if highBitRatio > opts.HighBitThreshold {
		// Check if it's valid UTF-8
		if isValidUTF8(sample) {
			// Valid UTF-8, likely text with non-ASCII characters
			return ContentInfo{
				IsBinary:   false,
				IsText:     true,
				MIMEType:   "text/plain",
				Method:     "heuristic",
				Confidence: 0.70,
			}
		}
		// Invalid UTF-8 with many high-bit bytes, likely binary
		return ContentInfo{
			IsBinary:   true,
			IsText:     false,
			Method:     "heuristic",
			Confidence: 0.75,
		}
	}

	// Try to detect JSON
	if looksLikeJSON(sample) {
		return ContentInfo{
			IsBinary:   false,
			IsText:     true,
			MIMEType:   "application/json",
			Extension:  "json",
			Method:     "heuristic",
			Confidence: 0.75,
		}
	}

	// Mostly printable characters, likely text
	printableRatio := float64(printableBytes) / float64(totalBytes)
	if printableRatio > 0.85 {
		return ContentInfo{
			IsBinary:   false,
			IsText:     true,
			MIMEType:   "text/plain",
			Extension:  "txt",
			Method:     "heuristic",
			Confidence: 0.70,
		}
	}

	// Default to binary if unsure
	return ContentInfo{
		IsBinary:   true,
		IsText:     false,
		Method:     "heuristic",
		Confidence: 0.50,
	}
}

// isValidUTF8 checks if the data is valid UTF-8 encoded text.
// This is a simplified check that doesn't require the unicode package.
func isValidUTF8(data []byte) bool {
	i := 0
	for i < len(data) {
		if data[i] < 0x80 {
			// ASCII byte
			i++
			continue
		}

		// Multi-byte sequence
		var size int
		var min, max byte

		switch {
		case data[i]&0xE0 == 0xC0: // 110xxxxx - 2 byte sequence
			size = 2
			min, max = 0x80, 0xBF
			if data[i] < 0xC2 { // overlong encoding
				return false
			}
		case data[i]&0xF0 == 0xE0: // 1110xxxx - 3 byte sequence
			size = 3
			min, max = 0x80, 0xBF
		case data[i]&0xF8 == 0xF0: // 11110xxx - 4 byte sequence
			size = 4
			min, max = 0x80, 0xBF
			if data[i] > 0xF4 { // beyond valid Unicode range
				return false
			}
		default:
			return false
		}

		// Check we have enough bytes
		if i+size > len(data) {
			return false
		}

		// Check continuation bytes
		for j := 1; j < size; j++ {
			if data[i+j] < min || data[i+j] > max {
				return false
			}
		}

		i += size
	}
	return true
}

// looksLikeJSON checks if data appears to be JSON.
func looksLikeJSON(data []byte) bool {
	// Skip leading whitespace
	i := 0
	for i < len(data) && isWhitespace(data[i]) {
		i++
	}

	if i >= len(data) {
		return false
	}

	// JSON must start with { or [
	if data[i] != '{' && data[i] != '[' {
		return false
	}

	opening := data[i]
	closing := byte('}')
	if opening == '[' {
		closing = ']'
	}

	// Quick scan for matching structure
	depth := 1
	inString := false
	escaped := false

	for j := i + 1; j < len(data); j++ {
		b := data[j]

		if escaped {
			escaped = false
			continue
		}

		if b == '\\' && inString {
			escaped = true
			continue
		}

		if b == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		if b == opening {
			depth++
		} else if b == closing {
			depth--
			if depth == 0 {
				// Check remaining is only whitespace
				for k := j + 1; k < len(data); k++ {
					if !isWhitespace(data[k]) {
						return false
					}
				}
				return true
			}
		}
	}

	// Didn't find matching close, but still might be truncated JSON
	// If we have at least some valid JSON structure, consider it JSON
	return depth > 0 && depth < 100 // Reasonable nesting depth
}

// isWhitespace returns true if b is a JSON whitespace character.
func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
