package render

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

// TestTruncByDisplayWidthFitsExact verifies the fast path: a styled
// line whose visible width is already ≤ maxCols passes through
// untouched (modulo any decisions truncByDisplayWidth might make
// about reset escapes — none in the no-truncation path).
func TestTruncByDisplayWidthFitsExact(t *testing.T) {
	in := "\x1b[36m►\x1b[0m hello"
	out := truncByDisplayWidth(in, 20)
	if out != in {
		t.Errorf("expected passthrough when in-budget, got %q (orig %q)", out, in)
	}
}

// TestTruncByDisplayWidthClampsAscii covers the simple ASCII overflow
// case. The result must fit in maxCols VISIBLE columns and end with
// the ellipsis + SGR reset so a colour can't bleed onto the next row.
func TestTruncByDisplayWidthClampsAscii(t *testing.T) {
	in := "  ⠏ ExplorerAgent(1) · ► thinking (round 15) 1s · 28 calls, last: read_file · 1m0s"
	out := truncByDisplayWidth(in, 50)
	visibleWidth := runewidth.StringWidth(stripAnsiEscapes(out))
	if visibleWidth > 50 {
		t.Errorf("truncated line is %d cols, exceeds budget 50: %q", visibleWidth, out)
	}
	if !strings.HasSuffix(out, "…\x1b[0m") {
		t.Errorf("truncated line should end with '…' + SGR reset, got %q", out)
	}
}

// TestTruncByDisplayWidthCJK is the regression lock for the
// 2026-04-12 刷屏 bug. The trigger was a status line whose visible
// width exceeded the terminal width by ONE column because the user's
// task title contained CJK characters (each 2 cols wide). pterm.Area
// counted logical newlines instead of visual rows, so the wrap-into-
// row-2 made every frame land below the previous one.
//
// This test asserts that when the input contains CJK and would
// overflow, the output's runewidth-measured visible width fits.
func TestTruncByDisplayWidthCJK(t *testing.T) {
	// Title used in the real reproduction.
	in := "  ► 列出可以调用SubAgent的代理"
	// runewidth says the original is roughly 4 (margin) + 16 (zh) +
	// 11 (SubAgent) ≈ 31 cols.
	original := runewidth.StringWidth(stripAnsiEscapes(in))
	if original < 25 {
		t.Fatalf("test fixture isn't long enough to exercise truncation: %d cols", original)
	}
	// Clamp to a width that forces truncation in the middle of the
	// CJK run — the bug would have happened around 80 cols, but any
	// width below the original triggers the same code path.
	out := truncByDisplayWidth(in, 20)
	if w := runewidth.StringWidth(stripAnsiEscapes(out)); w > 20 {
		t.Errorf("CJK truncation overshot: width=%d budget=20, out=%q", w, out)
	}
	if !strings.HasSuffix(out, "…\x1b[0m") {
		t.Errorf("CJK truncation should end with '…' + reset, got %q", out)
	}
}

// TestTruncByDisplayWidthAnsiNotCounted asserts that ANSI CSI escape
// bytes do NOT count toward the visible width budget. This is the
// whole point of separating "wire bytes" from "visible columns" —
// without it, a heavily-styled line would get over-truncated and
// look chopped even when it visually fits.
func TestTruncByDisplayWidthAnsiNotCounted(t *testing.T) {
	// 5 visible columns ("hello"), wrapped in ~20 bytes of SGR.
	in := "\x1b[36m\x1b[1mhello\x1b[0m"
	out := truncByDisplayWidth(in, 5)
	if got := runewidth.StringWidth(stripAnsiEscapes(out)); got != 5 {
		t.Errorf("expected 5 visible cols preserved, got %d (out=%q)", got, out)
	}
	if strings.HasSuffix(out, "…\x1b[0m") {
		t.Errorf("input fit exactly — should not have been truncated, got %q", out)
	}
}

// TestTruncByDisplayWidthZeroBudget guards the degenerate path so a
// terminal width misreport (or a tiny pop-up window) can't crash the
// renderer.
func TestTruncByDisplayWidthZeroBudget(t *testing.T) {
	if got := truncByDisplayWidth("anything", 0); got != "" {
		t.Errorf("expected empty string for zero budget, got %q", got)
	}
	if got := truncByDisplayWidth("anything", -5); got != "" {
		t.Errorf("expected empty string for negative budget, got %q", got)
	}
}

// TestTopicIndexFromNodeID covers the sub-topic row rename rule.
// `_tN` suffix → N (0-based); absence → not-found. Single-topic
// nodes (no suffix) must not trip the regex.
func TestTopicIndexFromNodeID(t *testing.T) {
	cases := []struct {
		id       string
		wantIdx  int
		wantOK   bool
	}{
		// Multi-subtopic evidence nodes — compiler.expandEvidenceNodes
		// writes "_tN" when len(rm.SubTopics) > 1.
		{"n1_evidence_t0", 0, true},
		{"n1_evidence_t1", 1, true},
		{"n1_evidence_t9", 9, true},
		{"n1_evidence_t12", 12, true},
		// Single-topic / non-evidence nodes — no suffix.
		{"n1_evidence", 0, false},
		{"n0_probe", 0, false},
		{"n4_finalize", 0, false},
		{"", 0, false},
		// Structural false-positives the regex must NOT match.
		{"t1", 0, false},             // no "_" prefix on suffix
		{"_t_notdigit", 0, false},    // digits required
		{"n1_evidence_t1a", 0, false},// trailing char after digit
	}
	for _, c := range cases {
		gotIdx, gotOK := topicIndexFromNodeID(c.id)
		if gotOK != c.wantOK || gotIdx != c.wantIdx {
			t.Errorf("topicIndexFromNodeID(%q) = (%d, %v), want (%d, %v)",
				c.id, gotIdx, gotOK, c.wantIdx, c.wantOK)
		}
	}
}
