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

// softRetryHintMessage renders the user-visible line for the generic
// stage-retry cue ("the previous agent turn did not cover every
// evidence requirement the question needs; running one more pass").
// Replaces the raw RetryHint body dump that used to land in the
// reasoning feed — the body is an internal LLM-directed prompt
// (with `## Evidence Gaps`, `[MISSING]`, `(entities: ...)` markup)
// that leaks terminology users cannot interpret. The full body
// remains in the debug log via `[explorer] retry hint built key=…`.
//
// Also used for the scheduler-level node SC retry cue (when an
// evidence/validate node's SuccessCriteria fails and the scheduler
// requeues for another pass) and the pre-finalize Tier-1 floor
// backtrack — both are "need more evidence before answering" from
// the user's point of view.
func softRetryHintMessage(lang string) string {
	if preferZhMessage(lang) {
		return "⟳ 正在补齐调查证据"
	}
	return "⟳ Gathering more evidence"
}

// softInvestigationReadyMessage renders the user-visible line for
// the investigation_complete override path: the active agent
// called emit_investigation_complete to signal it has collected
// enough facts, and the orchestrator skips remaining explore gates
// to move to the answer stage.
func softInvestigationReadyMessage(lang string) string {
	if preferZhMessage(lang) {
		return "› 调查就绪，准备作答"
	}
	return "› Investigation ready, preparing answer"
}

// softAnswerCheckRetryMessage renders the user-visible line for
// the post-finalize answer contract backtrack: the finalizer
// produced a draft that did not pass the quality checks, and the
// pipeline is running another pass with tightened hints. Distinct
// from softRetryHintMessage because the scope is different — this
// retry starts AFTER a candidate answer was produced, so the user
// message emphasises "answer" rather than "evidence".
func softAnswerCheckRetryMessage(lang string) string {
	if preferZhMessage(lang) {
		return "⟳ 答案待完善，再跑一轮"
	}
	return "⟳ Answer needs another pass"
}
