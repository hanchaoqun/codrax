package render

import "github.com/pterm/pterm"

// Status-stream style tokens. Centralised so taskRow rendering paths
// don't sprinkle pterm.FgGreen / FgCyan / FgWhite ad-hoc — the
// pre-2026-04-30 renderer had a dozen direct calls that drifted in
// brightness and made the spinner area visually overpower the final
// answer prose. Each token below maps to a single semantic role; the
// brightness ladder (most → least intense) is:
//
//	final answer prose (rendered by glamour, NOT a token here)
//	  ▶ statusPrimary           — current stage label
//	  ▶ statusReasoningBody     — model thinking body
//	  ▶ statusTopicLabel        — "关注点 1：" / "Focus 1:"
//	  ▶ statusDetail            — tool-call / live status detail
//	  ▶ statusTopicText         — focus-area body text
//	  ▶ statusSecondary         — "识别到 N 个关注点" / "N focus areas"
//	  ▶ statusFatal             — fatal error text body
//	  ▶ statusRecoverable       — recoverable error text body
//	  ▶ statusWarningMuted      — minor warnings
//	  ▶ statusMeta              — elapsed / round / Ctrl+C
//	  ▶ statusSpinner           — ⠇ frame, · bullet
//	  ▶ statusSuccessMuted      — ✓ checkmark
//
// Every token uses pterm.NewStyle so the same Sprint API works
// everywhere; passing pterm.Style by value means each call gets an
// isolated copy and parallel writes don't race on shared state.
//
// Colour pick rationale:
//   - statusPrimary uses xterm slot 75 (#5fafff mid blue) on dark
//     and slot 25 on light — clear without competing with the
//     bordered final answer's H1 white.
//   - statusReasoningBody uses high-intensity ANSI white, without bold
//     or a background, because pterm's FgGray is an alias of
//     FgDarkGray in the pinned version and plain FgWhite still reads
//     too dim in several black-background REPL themes. The muted tag
//     and lack of bold/background keep thinking below answer prose.
//   - statusFatal uses 167 (#d75f5f) NOT 196 (#ff0000) — same red
//     family but ~30% less luminance so a verify-failure doesn't
//     blast the whole pane red.
//   - statusRecoverable uses 215 (#ffaf5f) muted amber — the
//     "I'm working on it" hue, NOT the "danger" red.
var (
	// Running primary — the active stage label catches the eye.
	// Light blue is the brightness peak of the status family;
	// done labels demote to a muted grey so the timeline reads
	// as "this is happening now" / "this already finished".
	statusPrimary     = pterm.NewStyle(pterm.FgLightBlue)
	statusPrimaryDone = pterm.NewStyle(pterm.FgGray)

	// Objective line — the user's own question echoed back at the
	// top of the status area. Visually distinct from statusPrimary
	// so the user reads "this is what I asked" vs "this is the
	// stage running right now". The hue choice is the discriminator:
	// FgLightCyan pulls out of the blue family used for running
	// stages and remains readable on both dark and light terminals
	// (cyan is one of the few colours with consistent luminance
	// across schemes). Bold modifier is intentionally NOT applied —
	// the pterm Bold attribute is unevenly supported across CI log
	// emulators, some tmux themes, and IDE-integrated terminals, so
	// "objective looks distinct" must be carried by hue alone for
	// universal correctness.
	statusObjective     = pterm.NewStyle(pterm.FgLightCyan)
	statusObjectiveDone = pterm.NewStyle(pterm.FgCyan)

	statusReasoningBody = pterm.NewStyle(pterm.FgLightWhite)
	statusDetail        = pterm.NewStyle(pterm.FgGray)
	statusSecondary     = pterm.NewStyle(pterm.FgGray)
	statusTopicLabel    = pterm.NewStyle(pterm.FgLightBlue)
	// Topic body lives below the spotlighted parent line; dim it
	// to a regular grey so the topic_label "关注点 N：" carries
	// the eye and the body is supporting context rather than
	// competing with prose answers further down.
	statusTopicText = pterm.NewStyle(pterm.FgGray)
	statusMeta      = pterm.NewStyle(pterm.FgDarkGray)
	statusSpinner   = pterm.NewStyle(pterm.FgGray)
	// statusStream paints the rolling 20-30 char streaming tail at
	// the END of the live bar. Light magenta breaks visually from the
	// gray-family meta segments above so the user reads "this part is
	// the live data flow" without having to compare positions. NOT
	// used for any other surface — anchored to streaming data only.
	statusStream       = pterm.NewStyle(pterm.FgLightMagenta)
	statusSuccessMuted = pterm.NewStyle(pterm.FgGreen)
	statusFatal        = pterm.NewStyle(pterm.FgRed)
	statusRecoverable  = pterm.NewStyle(pterm.FgYellow)
	statusWarningMuted = pterm.NewStyle(pterm.FgYellow)
)

// Status-line glyphs. Centralised so a future glyph change touches
// one location and unit tests can pin the exact rune used.
const (
	glyphRunning     = '⠇' // animated spinner frame placeholder; real frame comes from spinnerFrames
	glyphSuccess     = '✓'
	glyphFatal       = '✗'
	glyphRecoverable = '⟳'
	glyphCancelled   = '⊘' // user-initiated stop; distinct from ✗ (system failure)
	glyphWarning     = '·'
	glyphPending     = '·'
	glyphTopicBullet = '·'
)
