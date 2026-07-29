package tool

import (
	"strings"
	"testing"
)

// PIB-3 (ledger docs/design/pi_borrow_analysis_20260729.md §3.4):
// transport-layer truncation discipline — never emit half a line, never
// tear a UTF-8 sequence, and teach the model the parameter names it can
// actually see in the schema.

// TestCutHeadAtBoundary_LineAndRuneSafety pins the head-cut contract.
func TestCutHeadAtBoundary_LineAndRuneSafety(t *testing.T) {
	// Multi-line data: cut lands on a line boundary, never mid-line.
	data := []byte("line-one\nline-two\nline-three\n")
	got := cutHeadAtBoundary(data, len("line-one\nline-tw"))
	if got != "line-one\n" {
		t.Errorf("head cut must end on a line boundary; got %q", got)
	}

	// Budget covers everything: unchanged.
	if got := cutHeadAtBoundary(data, len(data)+10); got != string(data) {
		t.Errorf("full-budget cut must return data unchanged; got %q", got)
	}

	// One enormous line of multi-byte runes: rune-safe fallback — the
	// kept prefix must remain valid UTF-8 at every budget.
	long := []byte(strings.Repeat("汉字宽行", 400)) // 12 bytes per repeat, no newlines
	for budget := 22; budget <= 26; budget++ {
		head := cutHeadAtBoundary(long, budget)
		if !utf8ValidString(head) {
			t.Errorf("budget=%d: head cut tore a UTF-8 rune: %q", budget, head)
		}
		if len(head) > budget {
			t.Errorf("budget=%d: head cut exceeded budget (%d bytes)", budget, len(head))
		}
	}
}

// TestCutTailAtBoundary_LineAndRuneSafety pins the tail-cut contract.
func TestCutTailAtBoundary_LineAndRuneSafety(t *testing.T) {
	data := []byte("alpha\nbeta\ngamma-tail")
	got := cutTailAtBoundary(data, len("ta\ngamma-tail"))
	if got != "gamma-tail" {
		t.Errorf("tail cut must start on a line boundary; got %q", got)
	}

	long := []byte(strings.Repeat("宽", 300))
	for budget := 10; budget <= 13; budget++ {
		tail := cutTailAtBoundary(long, budget)
		if !utf8ValidString(tail) {
			t.Errorf("budget=%d: tail cut tore a UTF-8 rune: %q", budget, tail)
		}
	}
}

func utf8ValidString(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

// TestBlobPreviews_TeachLineOffsetNotLegacyOffset pins the wording fix:
// the model-visible read_file schema exposes line_offset (legacy
// "offset" is decode-tolerated but banned from the model face), so
// every paging hint must say line_offset.
func TestBlobPreviews_TeachLineOffsetNotLegacyOffset(t *testing.T) {
	big := []byte(strings.Repeat("a line of filler text\n", 3000))
	for name, preview := range map[string]string{
		"head_readfile": buildHeadPreview(big, "blob://x", "read_file"),
		"head_other":    buildHeadPreview(big, "blob://x", "grep"),
		"head_noref":    buildHeadPreview(big, "", "read_file"),
		"head_tail":     buildPreview(big, "blob://x"),
	} {
		if strings.Contains(preview, "offset=") && !strings.Contains(preview, "line_offset=") {
			t.Errorf("%s: hint teaches bare offset=: %q", name, tailOf(preview, 300))
		}
		if strings.Contains(preview, "use offset/limit") {
			t.Errorf("%s: hint teaches legacy offset/limit pair: %q", name, tailOf(preview, 300))
		}
	}
	// The read_file head hint must still carry an executable
	// continuation instruction with the CORRECT parameter name.
	head := buildHeadPreview(big, "blob://x", "read_file")
	if !strings.Contains(head, "line_offset=") {
		t.Errorf("read_file head hint lost its executable continuation: %q", tailOf(head, 300))
	}
	// And the head slice itself must end on a line boundary before the
	// hint separator.
	if idx := strings.Index(head, "\n\n…["); idx > 0 && head[idx-1] != '\n' {
		t.Errorf("head preview does not end on a line boundary before the hint")
	}
}

func tailOf(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// TestCapGrepLineForInline pins the single-line cap: a rendered match
// line larger than the governor cap is rune-safe cut with an explicit
// marker carrying the recovery move; short lines pass untouched.
func TestCapGrepLineForInline(t *testing.T) {
	short := "src/a.go:42: normal match line"
	if got := capGrepLineForInline(short); got != short {
		t.Errorf("short line must pass untouched; got %q", got)
	}
	long := "src/min.js:1: " + strings.Repeat("宽", 4000)
	got := capGrepLineForInline(long)
	if len(got) >= len(long) {
		t.Fatalf("long line was not capped (len=%d)", len(got))
	}
	if !strings.Contains(got, "…[line truncated; read_file the path at this line for the full content]") {
		t.Errorf("capped line missing the recovery marker: %q", tailOf(got, 200))
	}
	if !utf8ValidString(got) {
		t.Errorf("capped line tore a UTF-8 rune")
	}
}
