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

// softFinalizingMessage renders the user-visible line emitted just
// before the finalizer LLM call starts. Unlike explore / extract
// which cycle through tool calls at visible cadence, the finalizer
// runs one synchronous LLM call that composes the whole answer, so
// the task row would otherwise sit on "thinking" for 20-60s with no
// other signal. This cue tells the user the answer is being
// composed right now — shipped immediately after the row transitions
// into the finalize stage.
func softFinalizingMessage(lang string) string {
	if preferZhMessage(lang) {
		return "⟳ 正在组织最终答案"
	}
	return "⟳ Composing the final answer"
}

// forcedFinalizeFailureMessage classifies a force-finalize terminal
// error and surfaces a localized, plain-language fatal message. The
// raw error string ("forced finalize: agent finalizer execution:
// read stream: unexpected EOF") is internal terminology that reads
// as a stack trace to the user — replaced here with a sentence
// that names the actual user-facing problem (model connection
// dropped, server unavailable, timeout) plus a concrete next-step
// suggestion.
//
// The returned text lands on TaskState.LastError, which the
// renderer surfaces via classifyStatusError → fatalDetailPhrase
// (the localized "无法继续" / "Cannot continue" prefix) for
// terminal failure styling.
func forcedFinalizeFailureMessage(err error, lang string) string {
	zh := preferZhMessage(lang)
	probe := strings.ToLower(err.Error())
	switch {
	case strings.Contains(probe, "unexpected eof"),
		strings.Contains(probe, "stream stalled"),
		strings.Contains(probe, "stream first byte"),
		strings.Contains(probe, "broken pipe"),
		strings.Contains(probe, "connection reset"),
		strings.Contains(probe, "connection refused"):
		if zh {
			return "无法继续：与模型的连接中断,可能是网络抖动或上游临时不可用,请稍后重试"
		}
		return "Cannot continue: connection to the model dropped (transient network or upstream issue); please retry"
	case strings.Contains(probe, "no deployments available"),
		strings.Contains(probe, "rate limit"),
		strings.Contains(probe, "429"):
		if zh {
			return "无法继续：模型服务暂不可用或限流,请稍后重试"
		}
		return "Cannot continue: model service is unavailable or rate-limited; please retry"
	case strings.Contains(probe, "context canceled"),
		strings.Contains(probe, "context cancelled"):
		if zh {
			return "已取消：用户中断了本次任务"
		}
		return "Cancelled: interrupted by user"
	case strings.Contains(probe, "deadline exceeded"),
		strings.Contains(probe, "timeout"):
		if zh {
			return "无法继续：模型响应超时"
		}
		return "Cannot continue: model response timed out"
	}
	if zh {
		return "无法继续：最终答案生成失败"
	}
	return "Cannot continue: failed to generate the final answer"
}
