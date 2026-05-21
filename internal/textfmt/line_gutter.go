package textfmt

import (
	"fmt"
	"strings"
)

// LineGutter joins a slice of text lines into a single string where every
// line is prefixed by its absolute 1-based line number followed by a U+2502
// visual separator and a space. The first element of lines is numbered
// startLineNo.
//
// This is shared by source reads and attached runtime-artifact previews so
// models learn one stable gutter shape for "copy the line number you saw".
func LineGutter(lines []string, startLineNo int) string {
	if len(lines) == 0 {
		return ""
	}
	avg := 0
	for _, l := range lines {
		avg += len(l)
	}
	avg /= len(lines)
	if avg < 1 {
		avg = 1
	}
	var b strings.Builder
	b.Grow(len(lines) * (avg + 11))
	for i, l := range lines {
		fmt.Fprintf(&b, "%6d│ %s", startLineNo+i, l)
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
