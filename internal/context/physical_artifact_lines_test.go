package context

import (
	"strings"
	"testing"
)

func TestAttachedArtifactPhysicalLines(t *testing.T) {
	for _, tc := range []struct {
		content string
		count   int
	}{{"", 0}, {"a", 1}, {"a\n", 1}, {"a\n\n", 2}, {"a\r\nb", 2}} {
		got := renderAttachedArtifactLines(tc.content, 7)
		if strings.Count(got, "│") != tc.count {
			t.Fatalf("physical preview %q became %q", tc.content, got)
		}
		if tc.content == "a\r\nb" && !strings.Contains(got, "a\r\n") {
			t.Fatalf("gutter rewrote original CRLF bytes: %q", got)
		}
	}
}
