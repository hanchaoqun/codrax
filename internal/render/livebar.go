package render

import (
	"fmt"
	"io"
)

// liveBar is the dispatch-time single-line progress ticker. The
// renderer keeps exactly one liveBar live at a time. All multi-row
// status visualisation that used to live in pterm.Area is now
// delivered through two channels:
//
//  1. THIS bar — one in-place line that animates the CURRENT focus
//     (the row receiving live LLM events). Updated via `\r\x1b[2K`
//     + new content; never wraps because every Update is pre-
//     truncated to terminal width.
//  2. Scrollback println — durable record of stage completions,
//     reasoning events, sub-topic enumeration, and the final
//     answer. Never overwritten, never animated.
//
// This split eliminates the pterm.Area failure modes the prior
// design accumulated patches for (cursor-up row-count drift,
// printAboveArea race, frozen-frame duplication, paused/dispatch-
// gen heuristics needed because multi-row state could fake
// concurrent stages). Single line + append-only scrollback are
// the two safest primitives every terminal back to a 1970s VT100
// supports.
//
// Concurrency: liveBar is NOT goroutine-safe on its own — the
// caller (Renderer) holds r.mu when invoking any liveBar method.
// This matches the rest of the renderer's locking model.
type liveBar struct {
	w     io.Writer
	dirty bool // true after first Update; Stop emits a clearing newline only when dirty
	last  string
}

// newLiveBar constructs a fresh single-line ticker writing to w.
// Production passes os.Stdout; tests pass a bytes.Buffer.
func newLiveBar(w io.Writer) *liveBar {
	return &liveBar{w: w}
}

// Update overwrites the bar's line with text. text MUST be pre-
// truncated by the caller to fit one terminal row — Update only
// applies a final safety cap (truncByTerminalWidth) so a runaway
// caller can't blow past the screen. Any embedded newlines /
// carriage returns are flattened to ` · ` so the ticker stays on
// exactly ONE physical row regardless of input.
func (b *liveBar) Update(text string) {
	if b == nil {
		return
	}
	flat := flattenToOneLine(text)
	cols := previewLineMaxCols()
	if cols < 4 {
		cols = 4
	}
	clean := truncByTerminalWidth(flat, cols)
	// `\r` returns to col 0 of the current line; `\x1b[2K` erases
	// the entire current line. Together: clean slate before we
	// write the new content. Universally supported (ANSI X3.64,
	// every terminal back to VT100).
	fmt.Fprint(b.w, "\r\x1b[2K")
	fmt.Fprint(b.w, clean)
	b.dirty = true
	b.last = clean
}

// Refresh repaints the last-set content. Used by the animation
// goroutine to advance the spinner glyph between explicit Updates
// — without it the leading frame stays static between events.
// Caller is responsible for re-rendering the content with the
// current animation frame BEFORE calling Update; Refresh is
// purely a reflush of the last buffered text.
//
// In practice the renderer just calls Update with freshly
// composed text on every animation tick, so Refresh is a thin
// wrapper kept for API symmetry with Stop.
func (b *liveBar) Refresh() {
	if b == nil || !b.dirty {
		return
	}
	fmt.Fprint(b.w, "\r\x1b[2K")
	fmt.Fprint(b.w, b.last)
}

// Stop wipes the bar's row and emits a single trailing newline so
// scrollback content prints below the (now-empty) ticker line
// without overlapping. Idempotent: Stop on an already-stopped bar
// is a no-op. After Stop, the next Update behaves as the first
// (no carriage return needed because the cursor is already on a
// fresh line — but Update emits \r\x1b[2K anyway, no-op-safe).
func (b *liveBar) Stop() {
	if b == nil || !b.dirty {
		return
	}
	fmt.Fprint(b.w, "\r\x1b[2K\n")
	b.dirty = false
	b.last = ""
}

// IsActive reports whether the bar has unflushed content.
func (b *liveBar) IsActive() bool {
	return b != nil && b.dirty
}

// liveBarFreshness — convenience accessor used by the renderer's
// animation goroutine to skip expensive recomposition when the
// bar isn't even active yet (start-of-run window before any event
// fires).
func (b *liveBar) Started() bool { return b != nil && b.dirty }

