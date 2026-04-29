package render

import (
	"bytes"
	"strings"
	"testing"
)

// TestTTYPreviewArea_FirstUpdateClearsLineThenWrites pins the very
// first Update behaviour: cursor is repositioned to col 0, the
// current line is erased, then content writes. Defends against
// debris from prior CLI output mid-line.
func TestTTYPreviewArea_FirstUpdateClearsLineThenWrites(t *testing.T) {
	var buf bytes.Buffer
	a := newTTYPreviewArea(&buf)
	a.Update("hello")
	got := buf.String()
	if !strings.HasPrefix(got, "\r\x1b[2K") {
		t.Errorf("first Update must begin with \\r\\x1b[2K; got %q", got)
	}
	if !strings.HasSuffix(got, "hello") {
		t.Errorf("first Update must end with the content; got %q", got)
	}
}

// TestTTYPreviewArea_SecondUpdateAlsoClearsLine is the canonical
// correctness test: every Update emits a fresh single-line clear
// before writing — no multi-line cursor positioning, no
// row-count tracking, no off-by-one possible.
func TestTTYPreviewArea_SecondUpdateAlsoClearsLine(t *testing.T) {
	var buf bytes.Buffer
	a := newTTYPreviewArea(&buf)
	a.Update("first")
	buf.Reset()
	a.Update("second")
	got := buf.String()
	want := "\r\x1b[2Ksecond"
	if got != want {
		t.Errorf("second Update: got %q want %q", got, want)
	}
}

// TestTTYPreviewArea_StopWipesLineAndResets verifies Stop emits the
// single-line clear sequence and resets internal state so the
// subsequent Update behaves as a first-time write again.
func TestTTYPreviewArea_StopWipesLineAndResets(t *testing.T) {
	var buf bytes.Buffer
	a := newTTYPreviewArea(&buf)
	a.Update("anything")
	buf.Reset()
	a.Stop()
	got := buf.String()
	want := "\r\x1b[2K"
	if got != want {
		t.Errorf("Stop: got %q want %q", got, want)
	}
	if a.dirty {
		t.Errorf("Stop must reset dirty; got dirty=true")
	}
}

// TestTTYPreviewArea_StopWithoutUpdate_NoOp ensures a stray Stop
// (e.g. from the orchestrator's defensive defer at Run exit) does
// nothing when no Update preceded it. Defends against ghost-clear
// sequences leaking into the user's piped output.
func TestTTYPreviewArea_StopWithoutUpdate_NoOp(t *testing.T) {
	var buf bytes.Buffer
	a := newTTYPreviewArea(&buf)
	a.Stop()
	if buf.Len() != 0 {
		t.Errorf("Stop without prior Update must emit nothing; got %q", buf.String())
	}
}

// TestTTYPreviewArea_FlattensNewlinesIntoSeparator pins the
// single-line guarantee: any '\n' or '\r' embedded in the content
// must be collapsed to a visible separator so the line never breaks.
// A literal newline mid-render would push subsequent text to a new
// row that the next Update's `\x1b[2K` would not clear.
func TestTTYPreviewArea_FlattensNewlinesIntoSeparator(t *testing.T) {
	var buf bytes.Buffer
	a := newTTYPreviewArea(&buf)
	a.Update("line1\nline2\rline3\n\nline4")
	got := buf.String()
	if strings.ContainsAny(got, "\n") {
		t.Errorf("content '\\n' must be flattened; got %q", got)
	}
	// Bare \r is the head escape sequence (\r\x1b[2K), but no other
	// \r should appear.
	if strings.Count(got, "\r") != 1 {
		t.Errorf("only the leading \\r escape allowed; got %q", got)
	}
	// All four logical lines must still appear in order.
	for _, frag := range []string{"line1", "line2", "line3", "line4"} {
		if !strings.Contains(got, frag) {
			t.Errorf("missing fragment %q in %q", frag, got)
		}
	}
}

// TestTTYPreviewArea_NilSafe locks the nil-receiver fast path —
// the renderer's previewArea field can be nil and the handler
// must short-circuit gracefully (defensive against double-Clear
// races between EventLivePreviewClear handlers and the orchestrator
// defer at Run exit).
func TestTTYPreviewArea_NilSafe(t *testing.T) {
	var a *ttyPreviewArea
	a.Update("ignored") // must not panic
	a.Stop()            // must not panic
}

// TestTruncByTerminalWidth_FitsExact verifies the fast path: text
// already within the column budget passes through unchanged.
func TestTruncByTerminalWidth_FitsExact(t *testing.T) {
	got := truncByTerminalWidth("hello", 80)
	if got != "hello" {
		t.Errorf("short text must pass through; got %q", got)
	}
}

// TestTruncByTerminalWidth_TruncatesWithEllipsis verifies overflow
// path: text wider than budget is clipped and " …" is appended.
func TestTruncByTerminalWidth_TruncatesWithEllipsis(t *testing.T) {
	got := truncByTerminalWidth("abcdefghij", 6)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated text must end in ellipsis; got %q", got)
	}
}

// TestTruncByTerminalWidth_CJKCountsAsTwoColumns verifies CJK
// runes consume 2 cols each — so a 5-char Chinese string fits in
// 10 cols, not 5.
func TestTruncByTerminalWidth_CJKCountsAsTwoColumns(t *testing.T) {
	in := "你好世界abc" // 4×2 + 3×1 = 11 cols
	got := truncByTerminalWidth(in, 10)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("11-col content into 10-col budget must truncate; got %q", got)
	}
	got2 := truncByTerminalWidth(in, 12)
	if got2 != in {
		t.Errorf("11-col content into 12-col budget must pass; got %q", got2)
	}
}
