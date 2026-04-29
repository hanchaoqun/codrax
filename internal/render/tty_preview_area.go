package render

import (
	"fmt"
	"io"
	"strings"
)

// ttyPreviewArea is a minimal in-place text region for the streaming
// finalize-preview. It bypasses pterm.AreaPrinter (whose per-line
// clear loop relies on the printed string's '\n' count exactly
// matching the visible row count — a precondition that is hard to
// guarantee under terminal auto-wrap, EAW-ambiguous runes, or
// multiplexer width misreporting) in favour of a single
// `\x1b[J` (erase-from-cursor-to-end-of-screen) sequence per
// Update. As long as the cursor is at-or-above the topmost printed
// row when erase fires, the entire previously-drawn region is
// wiped, regardless of how many physical rows the terminal added
// during display wrapping. The result: the next content prints on
// a clean canvas every time, and Stop() leaves no leftover prose
// for the bordered final answer to print over.
//
// Output protocol per Update(content):
//
//   1. If a previous Update wrote N '\n' characters (height>0),
//      emit `\r\x1b[<N>A` to move cursor to column 0 of N rows
//      above the current row. That is the row where the previous
//      Update started writing.
//   2. Emit `\x1b[J` to clear from cursor to end of screen,
//      regardless of how many physical rows the prior content
//      occupied (auto-wraps, expanded tabs, anything).
//   3. Write the new content verbatim.
//   4. Record the new content's '\n' count as height for the next
//      Update.
//
// Stop() runs the same rewind+clear sequence with empty content so
// the area's footprint disappears entirely.
//
// Cross-platform note: every escape used (\r, \x1b[NA, \x1b[J) is
// part of the canonical ANSI X3.64 / ECMA-48 set that Windows 10+
// console (with VT mode), macOS Terminal, and every Linux terminal
// emulator interpret natively. Older Windows console (cmd.exe pre-
// VT) would render the escapes as literal text — but the renderer
// only constructs ttyPreviewArea on a TTY where ANSI colour codes
// are already in use, so by the time we reach this code the host
// has already committed to ANSI-on. No build-tagged platform
// branches are necessary.
type ttyPreviewArea struct {
	w      io.Writer
	height int // newline count of the last-written content
}

// newTTYPreviewArea constructs a preview area writing to w. The
// caller is expected to pass os.Stdout in production; tests pass a
// bytes.Buffer so the emitted byte sequence can be inspected.
func newTTYPreviewArea(w io.Writer) *ttyPreviewArea {
	return &ttyPreviewArea{w: w}
}

// Update overwrites the preview region with content. The first call
// (height == 0) has nothing to erase, so it just clears the current
// line's leftover before writing — defensive against the cursor
// having mid-line debris from a prior bare write.
func (p *ttyPreviewArea) Update(content string) {
	if p == nil {
		return
	}
	if p.height > 0 {
		// `\r` reset to col 0; `\x1b[NA` go up N rows; `\x1b[J`
		// erase from cursor to end of screen.
		fmt.Fprintf(p.w, "\r\x1b[%dA\x1b[J", p.height)
	} else {
		// Clear current line + any wrap-residue below before the
		// first write. `\r` returns to col 0 so `\x1b[J` covers
		// from the start of the line.
		fmt.Fprint(p.w, "\r\x1b[J")
	}
	fmt.Fprint(p.w, content)
	p.height = strings.Count(content, "\n")
}

// Stop wipes the preview region completely and resets internal
// state. After Stop the next Update behaves as the first.
func (p *ttyPreviewArea) Stop() {
	if p == nil {
		return
	}
	if p.height > 0 {
		fmt.Fprintf(p.w, "\r\x1b[%dA\x1b[J", p.height)
	} else {
		fmt.Fprint(p.w, "\r\x1b[J")
	}
	p.height = 0
}
