package render

import (
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// ttyPreviewArea is a SINGLE-LINE in-place ticker for the streaming
// finalize-preview. Two prior multi-line implementations
// (pterm.AreaPrinter, then a self-managed `\x1b[J` clear-to-EOS
// region) both failed in real terminals: any time the printed
// content's physical row count drifted from our predicted row count
// — auto-wrap at the right edge, EAW-ambiguous runes that the
// terminal renders wider than runewidth predicts, multiplexer
// (tmux/screen) WINSZ desync between the time we asked and the time
// the terminal drew, IDE-integrated terminals with their own line
// buffering — the cursor-up math left visible debris that the next
// update or final answer printed on top of.
//
// Single-line ticker via `\r` (carriage return) plus `\x1b[2K`
// (erase entire line) avoids the failure modes entirely:
//
//   - `\r` is universally supported. Every terminal ever made,
//     including dumb terminals and CI log emulators, returns the
//     cursor to column 0 of the CURRENT line. Zero ambiguity.
//   - `\x1b[2K` erases the entire current line regardless of cursor
//     column. ANSI X3.64 / ECMA-48 standard since the 1970s; every
//     Windows 10+ VT-mode console, macOS Terminal, Linux emulator,
//     tmux, and screen all honour it.
//   - The line never wraps because we truncate every Update to fit
//     terminal width via display-column counting (CJK = 2 cols).
//     Auto-wrap can never produce ghost rows we'd need to clear.
//   - No multi-line cursor positioning, no row-count tracking, no
//     possibility of off-by-one between predicted and actual rows.
//
// The trade-off: the user sees a one-line indicator instead of the
// multi-line streaming text. That is acceptable: the prior
// behaviour was already broken for many users and produced a worse
// experience than no preview at all. A reliable one-liner ("正在
// 生成最终答案 12s · 已收到 234 字 · …最近内容…") gives the user
// the progress signal without the visual chaos.
//
// Cross-platform: \r and \x1b[2K work on Windows 10+ console with
// VT mode (the renderer's TTY check already gates this), macOS
// Terminal, every Linux terminal emulator. No build tags needed.
type ttyPreviewArea struct {
	w     io.Writer
	dirty bool // true once Update has written at least once and the line needs clearing on the next Update or Stop
}

// newTTYPreviewArea constructs a single-line ticker writing to w.
// Production passes os.Stdout; tests pass a bytes.Buffer to inspect
// the emitted byte sequence.
func newTTYPreviewArea(w io.Writer) *ttyPreviewArea {
	return &ttyPreviewArea{w: w}
}

// Update overwrites the ticker line with text. text MUST be
// pre-truncated by the caller to fit one terminal row — Update
// itself does not truncate beyond the safety cap (truncByTerminalWidth)
// because the caller knows the right format string and where to
// insert ellipsis. Embedded '\n' in text is replaced with a visible
// glyph (' · ') so the line never breaks.
func (p *ttyPreviewArea) Update(text string) {
	if p == nil {
		return
	}
	clean := flattenToOneLine(text)
	// `\r` returns to column 0 of the current line; `\x1b[2K` erases
	// the entire current line. Together: clean slate before writing.
	fmt.Fprint(p.w, "\r\x1b[2K")
	fmt.Fprint(p.w, clean)
	p.dirty = true
}

// Stop wipes the ticker line entirely and resets state. After Stop,
// the next Update behaves as the first.
//
// The trailing '\n' adds one row of breathing room between the
// (now-empty) ticker line and whatever prints next — typically the
// bordered final answer. Pre-2026-04-30 the bordered answer's top
// edge butted directly against the wiped ticker row, reading as
// "the answer is jammed against the spinner". One blank line is
// enough to visually separate without wasting screen real estate.
func (p *ttyPreviewArea) Stop() {
	if p == nil {
		return
	}
	if p.dirty {
		fmt.Fprint(p.w, "\r\x1b[2K\n")
		p.dirty = false
	}
}

// flattenToOneLine collapses any '\n' / '\r' in s into ' · '
// separators so the ticker output stays on a single physical line.
// Carriage returns inside content would otherwise rewind cursor
// mid-render, scrambling the display.
func flattenToOneLine(s string) string {
	if s == "" {
		return ""
	}
	out := make([]byte, 0, len(s))
	prevSpace := false
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == '\n' || r == '\r' {
			if !prevSpace {
				out = append(out, ' ', 0xc2, 0xb7, ' ') // " · "
				prevSpace = true
			}
			i += size
			continue
		}
		out = append(out, s[i:i+size]...)
		prevSpace = false
		i += size
	}
	return string(out)
}

// truncByTerminalWidth clamps text so its display width fits within
// maxCols columns. CJK chars cost 2 cols; ANSI escapes cost 0. When
// truncation happens, ' …' (ellipsis with leading space) is appended
// so the user knows the suffix was clipped. Returns text unchanged
// when it already fits.
//
// The function is the SINGLE-LINE counterpart to wrapByDisplayWidth
// (which wraps multi-line content). Used by handlePreviewChunkLocked
// to size the ticker text to terminal width before passing to
// ttyPreviewArea.Update.
func truncByTerminalWidth(text string, maxCols int) string {
	if maxCols < 4 {
		maxCols = 4
	}
	width := runewidth.StringWidth(text)
	if width <= maxCols {
		return text
	}
	// Reserve 2 columns for the trailing " …".
	limit := maxCols - 2
	if limit < 1 {
		limit = 1
	}
	var b []byte
	col := 0
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		rw := runewidth.RuneWidth(r)
		if rw < 0 {
			rw = 0
		}
		if col+rw > limit {
			break
		}
		b = append(b, text[i:i+size]...)
		col += rw
		i += size
	}
	return string(b) + " …"
}
