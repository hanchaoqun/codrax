package render

import (
	"bytes"
	"strings"
	"testing"
)

// TestTTYPreviewArea_FirstUpdateClearsLineThenWrites pins the very
// first Update behaviour: cursor is repositioned to col 0, the
// current line + everything below is erased, then content writes.
// Defends against bug where the first write leaves debris from
// prior CLI output mid-line.
func TestTTYPreviewArea_FirstUpdateClearsLineThenWrites(t *testing.T) {
	var buf bytes.Buffer
	a := newTTYPreviewArea(&buf)
	a.Update("hello")
	got := buf.String()
	// Must START with the clear sequence — `\r\x1b[J`.
	if !strings.HasPrefix(got, "\r\x1b[J") {
		t.Errorf("first Update must begin with \\r\\x1b[J; got %q", got)
	}
	if !strings.HasSuffix(got, "hello") {
		t.Errorf("first Update must end with the content; got %q", got)
	}
}

// TestTTYPreviewArea_SecondUpdateRewindsByPreviousNewlineCount is
// the canonical correctness test: after Update writes content with
// N newlines, the NEXT Update must emit `\x1b[NA` (cursor up N
// rows) so the next clear-to-EOS lands at the row the previous
// Update started writing on.
func TestTTYPreviewArea_SecondUpdateRewindsByPreviousNewlineCount(t *testing.T) {
	var buf bytes.Buffer
	a := newTTYPreviewArea(&buf)
	a.Update("line1\nline2\nline3") // 2 newlines
	buf.Reset()
	a.Update("only-one-line")
	got := buf.String()
	want := "\r\x1b[2A\x1b[J" + "only-one-line"
	if got != want {
		t.Errorf("second Update: got %q want %q", got, want)
	}
}

// TestTTYPreviewArea_StopWipesArea verifies Stop emits the same
// rewind+clear sequence and resets internal state so a subsequent
// Update behaves as a first-time write again.
func TestTTYPreviewArea_StopWipesArea(t *testing.T) {
	var buf bytes.Buffer
	a := newTTYPreviewArea(&buf)
	a.Update("one\ntwo\nthree\nfour") // 3 newlines
	buf.Reset()
	a.Stop()
	got := buf.String()
	want := "\r\x1b[3A\x1b[J"
	if got != want {
		t.Errorf("Stop: got %q want %q", got, want)
	}
	if a.height != 0 {
		t.Errorf("Stop must reset height; got %d", a.height)
	}
	// After Stop, the next Update should be treated as first-time.
	buf.Reset()
	a.Update("post-stop")
	if !strings.HasPrefix(buf.String(), "\r\x1b[J") {
		t.Errorf("post-stop Update must begin with first-time clear; got %q", buf.String())
	}
}

// TestTTYPreviewArea_ClearFromCursorToEOSEvenWhenWrapHappens is the
// load-bearing test for the "leftover wrap" defence: even when the
// printed content visually wraps in the terminal (i.e. produces
// MORE physical rows than the '\n' count), the next Update's
// `\x1b[J` clears from cursor to end of screen, wiping the wrap
// residue. We can't simulate terminal wrap in a bytes.Buffer
// (there's no terminal width), but we CAN verify the emitted
// sequence is `\x1b[J` (clear-to-EOS) rather than per-line clears
// that would leave wrap residue. Pin the sequence shape.
func TestTTYPreviewArea_ClearFromCursorToEOSEvenWhenWrapHappens(t *testing.T) {
	var buf bytes.Buffer
	a := newTTYPreviewArea(&buf)
	a.Update("x\ny") // 1 newline
	buf.Reset()
	a.Update("z")
	got := buf.String()
	// MUST contain `\x1b[J` (NOT `\x1b[2K` + per-line up loop) so
	// the clear is screen-region-bulk, robust against under-counted
	// wrap rows.
	if !strings.Contains(got, "\x1b[J") {
		t.Errorf("erase must use \\x1b[J (clear-to-EOS); got %q", got)
	}
	if strings.Contains(got, "\x1b[2K") {
		t.Errorf("MUST NOT use per-line \\x1b[2K (vulnerable to wrap residue); got %q", got)
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

// TestTTYPreviewArea_HeightTracksContentNewlines ensures height
// updates after each write so subsequent rewinds use the correct
// row count.
func TestTTYPreviewArea_HeightTracksContentNewlines(t *testing.T) {
	a := newTTYPreviewArea(&bytes.Buffer{})
	a.Update("a")
	if a.height != 0 {
		t.Errorf("single-line content must yield height=0; got %d", a.height)
	}
	a.Update("a\nb\nc")
	if a.height != 2 {
		t.Errorf("two newlines must yield height=2; got %d", a.height)
	}
	a.Update("a\nb\nc\nd\ne")
	if a.height != 4 {
		t.Errorf("four newlines must yield height=4; got %d", a.height)
	}
}
