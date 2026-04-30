package render

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// dockRowCount is the fixed height of the dock. Three rows is the
// kubectl/docker-pull layout: row 1 high-speed (status + stream
// tail), row 2 mid-speed (stage + counters), row 3 low-speed (time +
// cancel hint). Hardcoded because every paintDock / clearDock
// arithmetic depends on it; a 2-row or 4-row variant is a separate
// dock primitive.
const dockRowCount = 3

// dock owns the bottom-anchored 3-row status region. Single
// instance per Renderer, lifecycle bound to StartSpinner /
// StopSpinner.
//
// Design contract (load-bearing):
//
//   1. dock paints in place. Cursor parks at "row 3 + 1, column 0"
//      after every paintDock — this is the only stable anchor.
//   2. commitToScrollback is the ONLY way to write durable text
//      while the dock is alive. It does: cursor up 3 → \x1b[J →
//      writes the durable body (one or many lines) → re-paints the
//      3-row dock → cursor parks back. The dock visually ends up
//      below the new scrollback content with zero flicker.
//   3. paintDock + clearDock + commitToScrollback all use \x1b[3A
//      (ANSI cursor up by 3) followed by \x1b[J (clear from cursor
//      to end of screen). That sequence is universal back to VT100.
//   4. dirty tracks "is the dock currently on screen". paintDock
//      flips it true; clearDock + the !dirty branch in
//      commitToScrollback flip it false. The first paint MUST
//      happen via paintDock (no \x1b[3A) — see firstPaint flag.
//   5. Concurrency: dock is NOT goroutine-safe; the Renderer's
//      r.mu serializes all dock methods.
type dock struct {
	w     io.Writer
	dirty bool // true when the 3 rows are currently visible

	// firstPaint is true until the first paintDock writes the 3
	// rows. The first paint must NOT issue \x1b[3A because there
	// is no prior dock to rewind over — doing so would scroll
	// terminal content above the cursor up off-screen.
	firstPaint bool

	// lastRows holds the three styled lines from the most recent
	// paint. paintDock(rows) compares against lastRows and skips
	// the screen write when rows are byte-identical AND the
	// terminal didn't scroll in between (animation tick noise).
	// Currently always re-writes; left as a hook for later
	// throttling.
	lastRows [dockRowCount]string
}

// newDock constructs a dock writing to w. Production passes
// os.Stdout; tests pass bytes.Buffer.
func newDock(w io.Writer) *dock {
	return &dock{w: w, firstPaint: true}
}

// paintDock writes the 3 styled rows so the user sees them at the
// bottom of the terminal. rows MUST have exactly dockRowCount
// elements; each element is a fully-styled single line WITHOUT
// trailing newline. paintDock adds the newlines itself.
//
// Idempotent: paintDock can be called any number of times. The
// rewind+clear sequence guarantees no orphan lines pile up.
func (d *dock) paintDock(rows [dockRowCount]string) {
	if d == nil {
		return
	}
	d.lastRows = rows
	if d.firstPaint {
		// First-ever paint: don't rewind (nothing to rewind over).
		// Just write the 3 rows + park cursor on the line below.
		var b strings.Builder
		for i := 0; i < dockRowCount; i++ {
			b.WriteString(rows[i])
			b.WriteString("\n")
		}
		fmt.Fprint(d.w, b.String())
		d.firstPaint = false
		d.dirty = true
		return
	}
	// Subsequent paint: rewind 3 lines, clear screen below cursor,
	// rewrite 3 rows, park cursor. The clear sequence wipes the
	// previous dock content AND any half-finished animation
	// remnants below.
	var b strings.Builder
	b.WriteString("\x1b[")
	b.WriteString(dockItoa(dockRowCount))
	b.WriteString("A\x1b[J")
	for i := 0; i < dockRowCount; i++ {
		b.WriteString(rows[i])
		b.WriteString("\n")
	}
	fmt.Fprint(d.w, b.String())
	d.dirty = true
}

// commitToScrollback writes durable text ABOVE the dock. The dock
// stays visible at the bottom; the new scrollback content lands
// between any prior scrollback and the dock.
//
// body MAY contain multiple lines (separated by '\n'). The function
// adds a single trailing newline so the dock prints on a fresh
// line after the body. Empty body is a no-op.
//
// Mechanism:
//   1. Rewind 3 + clear-to-end (wipes the current dock visual)
//   2. Write body + '\n'
//   3. Repaint the 3 dock rows
//   4. Cursor parks at "below row 3"
//
// Pre-condition: dock has been painted at least once (dirty=true).
// When dirty=false, body is written as-is and the next paintDock
// will run as firstPaint.
func (d *dock) commitToScrollback(body string, rows [dockRowCount]string) {
	if d == nil || body == "" {
		return
	}
	if !d.dirty {
		// No dock on screen; just write body + paintDock fresh.
		fmt.Fprint(d.w, body)
		if !strings.HasSuffix(body, "\n") {
			fmt.Fprint(d.w, "\n")
		}
		d.paintDock(rows)
		return
	}
	var b strings.Builder
	b.WriteString("\x1b[")
	b.WriteString(dockItoa(dockRowCount))
	b.WriteString("A\x1b[J")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	for i := 0; i < dockRowCount; i++ {
		b.WriteString(rows[i])
		b.WriteString("\n")
	}
	fmt.Fprint(d.w, b.String())
	d.lastRows = rows
}

// clearDock erases the 3 dock rows and resets state. The cursor
// parks at the original dock-top line so subsequent writes land
// where the dock used to be.
//
// Idempotent: clearDock on an already-cleared dock is a no-op.
func (d *dock) clearDock() {
	if d == nil || !d.dirty {
		return
	}
	var b strings.Builder
	b.WriteString("\x1b[")
	b.WriteString(dockItoa(dockRowCount))
	b.WriteString("A\x1b[J")
	fmt.Fprint(d.w, b.String())
	d.dirty = false
	d.firstPaint = true
	for i := 0; i < dockRowCount; i++ {
		d.lastRows[i] = ""
	}
}

// IsActive reports whether the dock currently occupies 3 rows on
// screen. Callers use this to decide whether to commit-then-paint
// (alive) or just println (dock disabled / not yet started).
func (d *dock) IsActive() bool {
	return d != nil && d.dirty
}

// dockItoa is a no-allocation integer-to-string for small positive ints.
// Used in the paint hot path so we don't pull strconv into the dock
// rendering loop. dockRowCount is 3, so callers only ever pass 3.
// Renamed from `itoa` to avoid collision with the test helper of the
// same name in status_blocks_test.go.
func dockItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// dockRowState carries everything composeRow{1,2,3} need to render
// the three lines. Built once per paint by the renderer; passed by
// value so the composer is a pure function.
type dockRowState struct {
	// Row 1 — activity layer.
	activity   activityState
	streamTail string // truncated tail used when activity has stream
	frame      string // current spinner glyph (one of spinnerFrames)

	// Row 2 — stage layer.
	stageKey      string // canonical stage key for stagePhrase / progress
	stageProgress string // "K/N" or "—"
	stageLabel    string // localized stage name (running form)
	topicProgress string // "关注点 K/M" / "focus K/M" or empty
	iteration     int    // 0 = hide, ≥1 shows "第 N 轮"
	toolCount     int    // 0 = hide, ≥1 shows "N 工具"
	streamChars   int    // finalize-only "已收到 N 字" counter; 0 = hide

	// Row 3 — time layer.
	stageElapsed string // "5s" or empty
	totalElapsed string // "45s"
	cancelHint   string // "Ctrl+C 取消" — empty = hide

	// Common.
	lang string
}

// composeDockRows builds the three styled rows from a dockRowState.
// Each row is fully styled (ANSI escapes embedded) and pre-truncated
// to fit the current terminal width.
func composeDockRows(s dockRowState) [dockRowCount]string {
	var rows [dockRowCount]string
	maxCols := previewLineMaxCols()
	rows[0] = truncByDisplayWidth(composeDockRow1(s), maxCols)
	rows[1] = truncByDisplayWidth(composeDockRow2(s), maxCols)
	rows[2] = truncByDisplayWidth(composeDockRow3(s), maxCols)
	return rows
}

// composeDockRow1 renders the activity row:
//
//   "  ⠋ 接收中 ▸ ...with Hi there how can I"
//
// Glyph + status word + (optional) "▸ <tail>". The status word
// carries the "right now" semantics; the tail carries the live
// data for kinds where data exists (receiving / finalizing /
// callingTool).
func composeDockRow1(s dockRowState) string {
	glyph := s.frame
	glyphStyle := statusSpinner
	switch activityGlyphFor(s.activity.kind) {
	case activityGlyphRecoverable:
		glyph = string(glyphRecoverable)
		glyphStyle = statusRecoverable
	case activityGlyphFatal:
		glyph = string(glyphFatal)
		glyphStyle = statusFatal
	}
	word := activityPhrase(s.activity, s.lang)
	wordStyle := statusStream
	switch s.activity.kind {
	case activityWaitingPipeline, activityWaitingDispatch, activityWaitingNode:
		wordStyle = statusMeta // calmer hue when nothing is actively happening
	case activityErrorRecoverable, activityRetrying, activitySwitchingProvider:
		wordStyle = statusRecoverable
	case activityErrorFatal, activityCancelled:
		wordStyle = statusFatal
	}
	var b strings.Builder
	b.WriteString("  ")
	b.WriteString(glyphStyle.Sprint(glyph))
	b.WriteString(" ")
	b.WriteString(wordStyle.Sprint(word))
	if activityHasStreamTail(s.activity.kind) {
		tail := strings.TrimSpace(s.streamTail)
		if s.activity.kind == activityCallingTool {
			tail = strings.TrimSpace(s.activity.detail)
		}
		if tail != "" {
			b.WriteString(" ")
			b.WriteString(statusStream.Sprint("▸ "))
			b.WriteString(statusStream.Sprint(tail))
		}
	}
	return b.String()
}

// composeDockRow2 renders the stage row:
//
//   "  ▪ 2/6 探索证据 · 关注点 1/3 · 第 1 轮 · 5 工具 · 已收到 312 字"
//
// glyph ▪ in statusObjective (cyan). K/N + stage label always
// rendered; counters appended only when > 0. streamChars is finalize-
// specific (rendered as "已收到 N 字" / "received N chars").
func composeDockRow2(s dockRowState) string {
	var b strings.Builder
	b.WriteString("  ")
	b.WriteString(statusObjective.Sprint("▪"))
	b.WriteString(" ")
	progress := s.stageProgress
	if progress == "" {
		progress = "—"
	}
	b.WriteString(statusMeta.Sprint(progress))
	if s.stageLabel != "" {
		b.WriteString(" ")
		b.WriteString(statusPrimary.Sprint(s.stageLabel))
	}
	if s.topicProgress != "" {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusObjective.Sprint(s.topicProgress))
	}
	if s.iteration > 0 {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint(metaRoundPhrase(s.iteration, s.lang)))
	}
	if s.toolCount > 0 {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint(metaToolCountPhrase(s.toolCount, s.lang)))
	}
	if s.streamChars > 0 {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		if isZh(s.lang) {
			b.WriteString(statusMeta.Sprint(fmt.Sprintf("已收到 %d 字", s.streamChars)))
		} else {
			b.WriteString(statusMeta.Sprint(fmt.Sprintf("received %d chars", s.streamChars)))
		}
	}
	return b.String()
}

// composeDockRow3 renders the time row:
//
//   "  · 本 12s · 总 45s · Ctrl+C 取消"
//
// glyph · in statusMeta (darkest). stageElapsed prefixed with "本 / stage"
// when present; totalElapsed always shown; cancelHint suffix when set.
func composeDockRow3(s dockRowState) string {
	var b strings.Builder
	b.WriteString("  ")
	b.WriteString(statusMeta.Sprint("·"))
	b.WriteString(" ")
	wrote := false
	if s.stageElapsed != "" {
		b.WriteString(statusMeta.Sprint(stageElapsedPhrase(s.stageElapsed, s.lang)))
		wrote = true
	}
	if s.totalElapsed != "" {
		if wrote {
			b.WriteString(" ")
			b.WriteString(statusMeta.Sprint("·"))
			b.WriteString(" ")
		}
		b.WriteString(statusMeta.Sprint(totalElapsedPhrase(s.totalElapsed, s.lang)))
		wrote = true
	}
	if s.cancelHint != "" {
		if wrote {
			b.WriteString(" ")
			b.WriteString(statusMeta.Sprint("·"))
			b.WriteString(" ")
		}
		b.WriteString(statusMeta.Sprint(s.cancelHint))
	}
	return b.String()
}

// truncDurationToString returns the seconds-truncated string of d
// or empty when d is zero / negative. Helper centralised so the
// renderer's per-stage and per-pipeline elapsed callers agree.
func truncDurationToString(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return d.Truncate(time.Second).String()
}
