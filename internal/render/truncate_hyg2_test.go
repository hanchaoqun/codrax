package render

// HYG-2 G18 pin (§27.5): legacyReasoningSummary's space-boundary walk has no
// boundary to find in CJK prose — its fallback used to slice the raw byte cap
// mid-rune. The fallback must stay rune-safe; the ASCII space-boundary path
// keeps its legacy shape.

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHYG2LegacyReasoningSummaryCJKFallbackRuneSafe(t *testing.T) {
	out := legacyReasoningSummary(strings.Repeat("思", 100), true)
	if !utf8.ValidString(out) || strings.Contains(out, "�") {
		t.Fatalf("reasoning summary produced invalid UTF-8: %q", out)
	}
	if !strings.HasSuffix(out, "...") {
		t.Fatalf("reasoning summary lost its ellipsis: %q", out)
	}
}

func TestHYG2LegacyReasoningSummaryASCIISpaceBoundaryUnchanged(t *testing.T) {
	// 50 five-byte words: the space walk lands on a word boundary well
	// before the 197-byte fallback — legacy behavior preserved.
	in := strings.TrimSpace(strings.Repeat("word ", 50))
	out := legacyReasoningSummary(in, true)
	if !strings.HasSuffix(out, "...") {
		t.Fatalf("reasoning summary lost its ellipsis: %q", out)
	}
	if strings.Contains(out, "wor...") || strings.Contains(out, "wo...") {
		t.Fatalf("space-boundary walk regressed to a mid-word cut: %q", out)
	}
}
