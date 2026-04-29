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

// TestWrapByDisplayWidth_CRLF locks the Windows-line-ending case:
// a literal "\r\n" in the input must yield exactly one wrap (the
// '\n') with no extra blank line. '\r' itself is a zero-width
// control rune in runewidth, so it consumes no column budget; the
// '\n' resets the column counter normally. The LLM never emits
// CRLF, but a user-supplied source-quote pasted into the streaming
// summary could carry it on Windows; defensive coverage keeps the
// wrap math correct cross-platform.
func TestWrapByDisplayWidth_CRLF(t *testing.T) {
	in := "ab\r\ncd"
	out := wrapByDisplayWidth(in, 80)
	// Either preserve as-is (carrying the \r through) or strip the
	// \r — both behaviours are acceptable as long as the resulting
	// '\n' count matches the visible row count. Pin the visible-row
	// invariant: 1 '\n' → 2 visible rows.
	rows := strings.Count(out, "\n") + 1
	if rows != 2 {
		t.Errorf("CRLF input must yield 2 visible rows; got %d (out=%q)", rows, out)
	}
}

// TestWrapByDisplayWidth_TabRetainedAsZeroWidth keeps tab handling
// well-defined: runewidth treats '\t' as control (width=-1 →
// clamped to 0 by our wrap helper) so a tab in input does not
// trigger a wrap. Cross-platform note: the renderer never asks the
// terminal to expand tabs — that's the terminal's job; we just
// must not double-count.
func TestWrapByDisplayWidth_TabRetainedAsZeroWidth(t *testing.T) {
	in := "ab\tcd" // 4 visible chars + a tab
	out := wrapByDisplayWidth(in, 80)
	if out != in {
		t.Errorf("tab must pass through with no wrap; got %q want %q", out, in)
	}
}

// TestWrapByDisplayWidth_NarrowTerminalSafety models the smallest
// reasonable terminal width (e.g. 24-col split panes on a phone
// terminal app or a vertical mobile-emulator pane). The wrap must
// still behave correctly without infinite loops or produce
// pathological output. Cross-platform: the same low-width guard
// applies on Windows/macOS/Linux because runewidth + UTF-8
// decoding are platform-neutral.
func TestWrapByDisplayWidth_NarrowTerminalSafety(t *testing.T) {
	in := "中文字符在窄终端下也能正确换行"
	out := wrapByDisplayWidth(in, 6) // 3 Han chars per row
	for _, line := range strings.Split(out, "\n") {
		if w := runewidth.StringWidth(line); w > 6 {
			t.Errorf("narrow-terminal wrap exceeded budget: line=%q width=%d", line, w)
		}
	}
}
