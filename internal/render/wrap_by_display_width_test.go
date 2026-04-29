package render

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

// TestWrapByDisplayWidth_AsciiNoWrapWhenFits is the trivial
// pass-through: ASCII content shorter than maxCols stays one line.
func TestWrapByDisplayWidth_AsciiNoWrapWhenFits(t *testing.T) {
	in := "hello world"
	out := wrapByDisplayWidth(in, 80)
	if out != in {
		t.Errorf("short ASCII must pass through unchanged; got %q", out)
	}
}

// TestWrapByDisplayWidth_AsciiWrapsAtBoundary inserts a '\n' when
// the next rune would push past maxCols.
func TestWrapByDisplayWidth_AsciiWrapsAtBoundary(t *testing.T) {
	in := "abcdefghij" // 10 cols
	out := wrapByDisplayWidth(in, 5)
	want := "abcde\nfghij"
	if out != want {
		t.Errorf("wrap at boundary: got %q want %q", out, want)
	}
}

// TestWrapByDisplayWidth_CJKWrapsByDisplayWidth pins the load-
// bearing case: each Chinese rune occupies 2 columns. A maxCols=4
// budget fits exactly 2 Han chars per line.
func TestWrapByDisplayWidth_CJKWrapsByDisplayWidth(t *testing.T) {
	in := "性能分析" // 4 runes × 2 cols = 8 cols total
	out := wrapByDisplayWidth(in, 4)
	want := "性能\n分析"
	if out != want {
		t.Errorf("CJK wrap at display-width boundary: got %q want %q", out, want)
	}
}

// TestWrapByDisplayWidth_PreservesExistingNewlines locks the rule
// that an explicit '\n' in input resets the column counter without
// being doubled.
func TestWrapByDisplayWidth_PreservesExistingNewlines(t *testing.T) {
	in := "ab\ncd"
	out := wrapByDisplayWidth(in, 80)
	if out != in {
		t.Errorf("pass-through with existing newline; got %q", out)
	}
}

// TestWrapByDisplayWidth_VisibleRowCountMatchesNewlineCount is the
// invariant pterm.Area depends on: the count of '\n' in the wrapped
// string + 1 must equal the count of visible rows the terminal
// would draw at the same width. Verified via runewidth.StringWidth
// per line.
func TestWrapByDisplayWidth_VisibleRowCountMatchesNewlineCount(t *testing.T) {
	const maxCols = 20
	// Mixed CJK + ASCII line that would visually wrap to 4 rows in
	// a 20-col terminal: each CJK rune is 2 cols, so 30 chars × 2
	// = 60 cols, /20 = 3 wraps + the partial last row.
	in := "中文与英文混排的测试用例 alpha beta gamma delta"
	out := wrapByDisplayWidth(in, maxCols)
	for _, line := range strings.Split(out, "\n") {
		if w := runewidth.StringWidth(line); w > maxCols {
			t.Errorf("wrapped line %q has display width %d > maxCols %d",
				line, w, maxCols)
		}
	}
}

// TestWrapByDisplayWidth_AnsiPassthrough verifies that CSI escape
// sequences do NOT consume column budget — colour reset / SGR
// codes embedded in styled text shouldn't trigger a wrap.
func TestWrapByDisplayWidth_AnsiPassthrough(t *testing.T) {
	// 5 ASCII visible chars (col=5), preceded by a bold-on SGR and
	// followed by SGR reset. maxCols=5 should fit on one line.
	in := "\x1b[1mhello\x1b[0m"
	out := wrapByDisplayWidth(in, 5)
	if strings.Contains(out, "\n") {
		t.Errorf("ANSI escapes must not consume columns; got %q", out)
	}
}

// TestWrapByDisplayWidth_DegenerateMaxCols guards the maxCols<1
// fast path: the function returns input verbatim rather than
// looping forever.
func TestWrapByDisplayWidth_DegenerateMaxCols(t *testing.T) {
	if got := wrapByDisplayWidth("anything", 0); got != "anything" {
		t.Errorf("maxCols<=0 must pass through; got %q", got)
	}
	if got := wrapByDisplayWidth("anything", -1); got != "anything" {
		t.Errorf("negative maxCols must pass through; got %q", got)
	}
}
