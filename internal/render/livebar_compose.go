package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

// liveBarState is the input composeLiveBar reads. Narrow on
// purpose: every field is something the renderer already
// tracks, so the bar's content is a pure function of (focus,
// frame index, clocks).
type liveBarState struct {
	// focus is the row whose state the bar represents. Nil before
	// any node has started — the bar then renders a "preparing"
	// placeholder so the spinner stays alive.
	focus *taskRow

	// stageProgress is "K/N" — focus's global pipeline-stage
	// index over total stages. Pre-stages render "—" instead.
	stageProgress string

	// topicProgress is "关注点 K/M" rendered for evidence stage
	// when multiple sub-topics fan out. Empty otherwise.
	topicProgress string

	// streamMeta carries finalize-streaming meta when set
	// ("已收到 N 字 · 回复中: tail"). When non-empty, replaces
	// the iter / tool meta block.
	streamMeta string

	// frame is the current spinner glyph (one of spinnerFrames).
	frame string

	// now is the wall clock for elapsed math.
	now time.Time

	// lang is the locale.
	lang string

	// errorKind classifies error states. statusErrorNone for
	// normal running.
	errorKind statusErrorKind
}

// composeLiveBar produces the styled, terminal-truncated single
// line for the dispatch ticker.
//
// Layout:
//
//   "  [glyph] [K/N] [stage label] · [topic] · [meta] · [elapsed]"
//
// Brightness ladder:
//
//   glyph         — animated spinner frame (statusSpinner) for
//                   running, ✗ red for fatal, ⟳ yellow for
//                   recoverable
//   K/N           — statusMeta DarkGray
//   stage label   — statusPrimary LightBlue (running),
//                   statusPrimaryDone Gray (done),
//                   statusFatal/Recoverable for errors
//   topic         — statusObjective LightCyan
//   meta          — statusMeta + statusDetail mix
//   elapsed       — statusMeta DarkGray
func composeLiveBar(s liveBarState) string {
	if s.focus == nil {
		return composeLiveBarPreparing(s)
	}

	primaryStyle, glyphStyle := liveBarPalette(s.errorKind, s.focus)
	glyph := s.frame
	switch s.errorKind {
	case statusErrorFatal, statusErrorCancelled:
		glyph = string(glyphFatal)
	case statusErrorRecoverable:
		glyph = string(glyphRecoverable)
	}

	primaryText := liveBarPrimaryText(s.focus, s.lang)
	metaParts := liveBarMetaParts(s)
	elapsedText := liveBarElapsed(s)

	var b strings.Builder
	b.Grow(160)
	b.WriteString("  ")
	b.WriteString(glyphStyle.Sprint(glyph))
	b.WriteString(" ")
	if s.stageProgress != "" {
		b.WriteString(statusMeta.Sprint(s.stageProgress))
		b.WriteString(" ")
	}
	b.WriteString(primaryStyle.Sprint(primaryText))
	if s.topicProgress != "" {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusObjective.Sprint(s.topicProgress))
	}
	for _, part := range metaParts {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(part.style.Sprint(part.text))
	}
	if elapsedText != "" {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint(elapsedText))
	}
	return b.String()
}

// composeLiveBarPreparing is the placeholder line shown BEFORE
// any EventTaskNodeStart fires. Without it the screen would be
// blank during analyzer warm-up.
func composeLiveBarPreparing(s liveBarState) string {
	var b strings.Builder
	b.WriteString("  ")
	b.WriteString(statusSpinner.Sprint(s.frame))
	b.WriteString(" ")
	if isZh(s.lang) {
		b.WriteString(statusPrimary.Sprint("正在准备流水线"))
	} else {
		b.WriteString(statusPrimary.Sprint("Preparing pipeline"))
	}
	return b.String()
}

type liveBarPart struct {
	text  string
	style *pterm.Style
}

// liveBarMetaParts builds the variable-meta segment between
// primary and elapsed. Three modes:
//
//   * stream-mode (streamMeta non-empty)        → one statusDetail part
//   * iter / tool counters present              → two statusMeta parts
//   * idle                                       → no parts
func liveBarMetaParts(s liveBarState) []liveBarPart {
	if strings.TrimSpace(s.streamMeta) != "" {
		return []liveBarPart{{text: s.streamMeta, style: statusDetail}}
	}
	if s.focus == nil {
		return nil
	}
	var parts []liveBarPart
	if s.focus.iteration > 0 {
		parts = append(parts, liveBarPart{
			text:  metaRoundPhrase(s.focus.iteration, s.lang),
			style: statusMeta,
		})
	}
	if s.focus.toolCount > 0 {
		parts = append(parts, liveBarPart{
			text:  metaToolCountPhrase(s.focus.toolCount, s.lang),
			style: statusMeta,
		})
	}
	return parts
}

// liveBarElapsed renders the trailing elapsed segment.
func liveBarElapsed(s liveBarState) string {
	if s.focus == nil {
		return ""
	}
	row := s.focus
	if !row.endTime.IsZero() {
		return row.endTime.Sub(row.startTime).Truncate(time.Second).String()
	}
	if !row.detailStart.IsZero() && !s.now.IsZero() {
		d := s.now.Sub(row.detailStart)
		if d < 0 {
			d = 0
		}
		return d.Truncate(time.Second).String()
	}
	return ""
}

// liveBarPrimaryText resolves the focus's stage label via the
// existing stagePhrase localisation helper.
func liveBarPrimaryText(row *taskRow, lang string) string {
	if row == nil {
		return ""
	}
	key := stageKeyFor(row)
	state := stagePhraseRunning
	switch {
	case row.isNodeRow && row.pending:
		state = stagePhrasePending
	case !row.endTime.IsZero():
		state = stagePhraseDone
	}
	return stagePhrase(key, lang, state)
}

// liveBarPalette picks (primaryStyle, glyphStyle) for a focus row.
func liveBarPalette(errKind statusErrorKind, row *taskRow) (*pterm.Style, *pterm.Style) {
	switch errKind {
	case statusErrorFatal, statusErrorCancelled:
		return statusFatal, statusFatal
	case statusErrorRecoverable:
		return statusRecoverable, statusRecoverable
	}
	if row != nil && !row.endTime.IsZero() {
		return statusPrimaryDone, statusSuccessMuted
	}
	return statusPrimary, statusSpinner
}

// composeStreamMeta builds the streamMeta string for a finalize-
// streaming bar.
//
// Format (zh): "#N· 已收到 K 字 · 回复中: tail"
// Format (en): "#N· received K chars · replying: tail"
func composeStreamMeta(roundLabel string, chars int, tailPreview, lang string) string {
	tail := strings.TrimSpace(tailPreview)
	zh := isZh(lang)
	var head string
	if zh {
		if roundLabel != "" {
			head = fmt.Sprintf("%s· 已收到 %d 字", roundLabel, chars)
		} else {
			head = fmt.Sprintf("已收到 %d 字", chars)
		}
	} else {
		if roundLabel != "" {
			head = fmt.Sprintf("%s· received %d chars", roundLabel, chars)
		} else {
			head = fmt.Sprintf("received %d chars", chars)
		}
	}
	if tail == "" {
		return head
	}
	if zh {
		return head + " · 回复中: " + tail
	}
	return head + " · replying: " + tail
}
