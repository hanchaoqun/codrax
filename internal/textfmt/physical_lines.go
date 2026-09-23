package textfmt

import "strings"

// PhysicalLines splits LF-delimited file bytes without inventing an EOF row.
// An empty file has no lines; each LF terminates one physical line, and a
// nonempty unterminated suffix is one line. Only the delimiter is removed:
// real blank lines, CR bytes, whitespace and all other content are retained.
func PhysicalLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if content[len(content)-1] == '\n' {
		lines = lines[:len(lines)-1]
	}
	return lines
}
