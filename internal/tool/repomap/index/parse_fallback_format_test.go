package index

import (
	"os"
	"strings"
	"testing"
)

// TestFallbackWarnLogFormatPinned pins the fallback WARN log format at
// the source level: the CLAUDE.md repomap red line quotes this format
// verbatim ("repomap: <lang> <file> tier N→M: <reason>"), and the two
// drifted apart once already (the doc carried a stale variant for
// weeks with nothing pinning either side). Static source pin — precise
// and independent of logging plumbing.
func TestFallbackWarnLogFormatPinned(t *testing.T) {
	raw, err := os.ReadFile("parse_fallback.go")
	if err != nil {
		t.Fatal(err)
	}
	const want = `"repomap: %s %s tier %d→%d: %s"`
	if !strings.Contains(string(raw), want) {
		t.Fatalf("parse_fallback.go no longer carries the documented WARN format %s — update CLAUDE.md's repomap red line in the same change", want)
	}
}
