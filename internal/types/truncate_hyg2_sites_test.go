package types

// HYG-2 G18 pins (§27.5): representative sites for the SILENT bare-slice
// class (no ellipsis, object can carry CJK) and for the newline-boundary
// failure-signal cap whose fallback path used to slice mid-rune.

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHYG2TrimSourceLocalizationTextRuneSafe(t *testing.T) {
	// 100 CJK runes = 300 bytes > 240; byte 240 lands rune-aligned only by
	// luck — use an offset head so the cap falls mid-rune (1 + 239: 239 % 3
	// puts byte 240 inside a rune).
	out := trimSourceLocalizationText("a" + strings.Repeat("位", 100))
	if !utf8.ValidString(out) || strings.Contains(out, "�") {
		t.Fatalf("trimSourceLocalizationText produced invalid UTF-8: %q", out)
	}
	if len(out) > writeContextPackTextLen {
		t.Fatalf("trimSourceLocalizationText exceeded its byte cap: %d", len(out))
	}
}

func TestHYG2ClampProfileSnippetRuneSafe(t *testing.T) {
	// Review P2-2: RawRequest / observation-summary snippets are CJK user
	// prose reaching LLM prompts verbatim; the silent 240-byte cap must not
	// split a rune. Offset head so byte 240 lands mid-rune (runes start at
	// 1+3k; 240 is a continuation byte).
	out := clampProfileSnippet("a" + strings.Repeat("求", 100))
	if !utf8.ValidString(out) || strings.Contains(out, "�") {
		t.Fatalf("clampProfileSnippet produced invalid UTF-8: %q", out)
	}
	if len(out) > 240 {
		t.Fatalf("clampProfileSnippet exceeded its byte cap: %d", len(out))
	}
	// ASCII identity with the legacy silent shape (no ellipsis, exact cap).
	if got := clampProfileSnippet(strings.Repeat("a", 300)); got != strings.Repeat("a", 240) {
		t.Fatalf("clampProfileSnippet ASCII behavior changed: %d bytes", len(got))
	}
}

func TestHYG2TrimPlanRepairTextRuneSafe(t *testing.T) {
	out := trimPlanRepairText(strings.Repeat("修", 100), 100)
	if !utf8.ValidString(out) || strings.Contains(out, "�") {
		t.Fatalf("trimPlanRepairText produced invalid UTF-8: %q", out)
	}
	if !strings.HasSuffix(out, "...") {
		t.Fatalf("trimPlanRepairText lost its ellipsis: %q", out)
	}
	if got := trimPlanRepairText("abcdef", 4); got != "abcd..." {
		t.Fatalf("trimPlanRepairText ASCII behavior changed: %q", got)
	}
}

func TestHYG2CapFailureSignalWithBoundaryRuneSafeFallback(t *testing.T) {
	// No newline anywhere: the probe window finds no boundary and the cap
	// falls back to the raw byte cut — which must land on a rune boundary.
	out := capFailureSignalWithBoundary(strings.Repeat("障", 100), 100)
	if !utf8.ValidString(out) || strings.Contains(out, "�") {
		t.Fatalf("capFailureSignalWithBoundary produced invalid UTF-8: %q", out)
	}
	if !strings.HasSuffix(out, "…") {
		t.Fatalf("capFailureSignalWithBoundary lost its ellipsis: %q", out)
	}
	// Newline-boundary path stays intact (byte-safe on its own).
	in := strings.Repeat("x", 80) + "\n" + strings.Repeat("y", 40)
	if got := capFailureSignalWithBoundary(in, 100); got != strings.Repeat("x", 80)+"…" {
		t.Fatalf("newline-boundary path changed: %q", got)
	}
}
