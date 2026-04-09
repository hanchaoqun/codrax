package patcher

import "strings"

// detectEOL returns the dominant line ending in text. It returns "\r\n" if
// any CRLF appears, otherwise "\n". Empty input defaults to "\n".
func detectEOL(text string) string {
	if strings.Contains(text, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// normalizeEOL converts text from the given EOL to "\n". This lets all
// string operations work on a single canonical form regardless of how the
// source file is encoded on disk.
func normalizeEOL(text, eol string) string {
	if eol == "\r\n" {
		return strings.ReplaceAll(text, "\r\n", "\n")
	}
	return text
}

// denormalizeEOL converts text from "\n" back to the given EOL so it can be
// written to disk with its original line endings preserved.
func denormalizeEOL(text, eol string) string {
	if eol == "\r\n" {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}
