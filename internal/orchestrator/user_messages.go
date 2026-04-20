package orchestrator

import "strings"

// user_messages.go — short localized strings rendered into
// EventAgentReasoning when the orchestrator needs to update the user
// about internal scheduler decisions (forced reads, convergence
// stalls, etc.).
//
// Design contract:
//
//   - Soft symbols (⟳, ·, ›, –) only. Bright glyphs (📊 ⚠️ ✅) are
//     avoided because they dominate the TUI at the cadence the
//     orchestrator emits these events.
//   - User language, not internal jargon. "CGEC E2 / I4", counter
//     names, and node IDs stay in the operator log (logging.*) and
//     never appear in Reasoning strings.
//   - One sentence, describes what is happening from the user's
//     point of view, not what internal enforcer fired.

// preferZhMessage reports whether the busCtx language indicator
// prefers Simplified Chinese. Mirrors the list render.answerdoc uses
// so UI / final-answer consistency is guaranteed.
func preferZhMessage(lang string) bool {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh", "zh-cn", "cn", "chinese", "简体中文":
		return true
	}
	return false
}

// softForcedReadMessage renders the user-visible line for the CGEC
// E2 forced-read enforcer ("the LLM skipped a file we know it needs,
// reading it now"). Count is elided from the message itself — users
// only care that we are filling in a gap, not the exact arity.
func softForcedReadMessage(lang string, _ int) string {
	if preferZhMessage(lang) {
		return "⟳ 正在补充关键信息"
	}
	return "⟳ Filling in missing context"
}

// softConvergenceStallMessage renders the user-visible line for the
// CGEC I4 convergence detector when the investigation has plateaued
// and the orchestrator force-completes with current evidence.
func softConvergenceStallMessage(lang string) string {
	if preferZhMessage(lang) {
		return "– 根据已有线索作答"
	}
	return "– Finalizing with current leads"
}
