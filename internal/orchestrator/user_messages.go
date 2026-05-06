package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// user_messages.go — short localized strings rendered into
// EventOrchestratorNotice when the orchestrator needs to update the
// user about internal scheduler decisions (forced reads, convergence
// stalls, etc.). Pre-this-event the strings flowed through
// EventAgentReasoning and the dock rendered them with the same
// `💭 [orchestrator-N] …` style as genuine LLM thinking — users
// could not tell which lines were the LLM and which were the
// orchestrator. The new event drops the 💭 prefix + agent tag so
// the two surfaces are visually distinct.
//
// Design contract:
//
//   - Soft symbols (⟳, ·, ›, –, ⊘) only. Bright glyphs (📊 · ✅) are
//     avoided because they dominate the TUI at the cadence the
//     orchestrator emits these events.
//   - User language, not internal jargon. "CGEC E2 / I4", counter
//     names, and node IDs stay in the operator log (logging.*) and
//     never appear in Reasoning strings.
//   - One sentence, describes what is happening from the user's
//     point of view, not what internal enforcer fired.
//   - Each emit site picks a render.OrchestratorNoticeKind that
//     matches the message's semantic bucket (retry / info /
//     progress); the renderer paints color from that classification.

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

// abandonForcedReadMessage renders the user-visible line for the
// CGEC E2 abandonment path when runForcedReads gives up on a file
// it cannot read (typically ENOENT / EACCES / IsDir / broken
// symlink). Without surfacing this to the dock, the user only sees
// the spinner stalling for 3 stall-detector rounds before the
// answer ships — they have no way to know the framework had to
// skip a primary anchor. cause is the LLM-facing summary
// (summarizeReadFailure output) so the operator-visible line and
// the LLM-facing rationale agree on the cause.
func abandonForcedReadMessage(lang, file, cause string) string {
	if preferZhMessage(lang) {
		return "⊘ 跳过 `" + file + "`(" + cause + "),继续推进"
	}
	return "⊘ Skipping `" + file + "` (" + cause + "), continuing"
}

// selfConsistencyReviewStartMessage (commit 62): renders the
// user-visible status line shown in the REPL bottom dock when
// the self-consistency reviewer LLM dispatches. Emitted BEFORE
// the Chat call so the user sees what the system is doing —
// not silent background work. Bilingual.
func selfConsistencyReviewStartMessage(lang string) string {
	if preferZhMessage(lang) {
		return "⟳ 审查答案前后一致性"
	}
	return "⟳ Reviewing answer self-consistency"
}

// selfConsistencyContradictionMessage (commit 62): rendered when
// the reviewer reports >= 1 contradiction at confidence >= floor.
// Variants: "rewriting" when rewrite-on-contradiction is on (the
// finalizer will re-dispatch); "logged" when off (advisory only).
func selfConsistencyContradictionMessage(lang string, rewrite bool, count int) string {
	if preferZhMessage(lang) {
		if rewrite {
			return fmt.Sprintf("⟳ 检测到 %d 处前后矛盾，正在重写答案", count)
		}
		return fmt.Sprintf("· 检测到 %d 处前后矛盾（仅记录，未重写）", count)
	}
	if rewrite {
		return fmt.Sprintf("⟳ Found %d self-contradiction(s) — rewriting answer", count)
	}
	return fmt.Sprintf("· Found %d self-contradiction(s) (logged, not rewritten)", count)
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

// plannerProseFallbackMessage is the user-visible explanation when
// the planner returns a prose answer without calling
// emit_change_plan. The pre-fix message leaked the internal tool
// name ("planner did not call emit_change_plan") into the answer
// surface. Two real triggers:
//
//   - Sticky /mode plan + the user asks a "how do I X" question
//     that is fundamentally a read-mode question (no code change
//     needed). The planner LLM understood the question but had no
//     legitimate change to plan.
//   - Planner streamed a partial reply that was killed by the
//     stream watchdog before the tool call landed. Rare with the
//     2026-04-30 stream-first-byte timeout fix; still possible.
//
// The message walks the user through both possibilities with a
// concrete next-action menu (`/mode read` for advice questions,
// "再问一次" for transient stalls). Stays in `SetResultPlain`
// territory because it embeds slash commands.
func plannerProseFallbackMessage(ctx *types.BusContext) string {
	zh := true
	if ctx != nil {
		zh = preferZhMessage(ctx.Language)
	}
	if zh {
		return "本轮没产出可执行的改动方案。下一步两选一:\n\n" +
			"  • 咨询类问题(怎么修复 / 怎么安装 / 是什么原因):/mode read 后原样再问\n" +
			"  • 确实要改代码:把目标说具体(改哪个文件 / 加什么 / 接口长什么样)再发一遍"
	}
	return "no executable change plan was produced this turn. Pick one:\n\n" +
		"  • advisory question (how to install, why this failed): /mode read then re-ask\n" +
		"  • genuine code change: re-send the request with a concrete target (which file, what to add, what interface)"
}

// softRetryHintForStage is the stage-aware variant. Write-mode
// stages (plan / apply / verify) do not have "investigation
// evidence" — that's read-mode language. Picking a generic
// "filling in evidence" message for a planner stall produces UX
// drift the user reported as "dock contents don't reflect actual
// state". Return phrasing that matches what the stage is actually
// doing on retry. Read-mode stages fall through to the original
// softRetryHintMessage so existing read-path behaviour is
// byte-identical.
func softRetryHintForStage(lang string, stage types.PipelineStage) string {
	zh := preferZhMessage(lang)
	switch stage {
	case types.StagePlan:
		// "正在补完" / "Refining" implied a plan was already drafted
		// and we were just polishing it. On real plan-stage retries
		// (transient-stall path or verify→plan SC failure) NO plan
		// has landed yet — saying "refining" is a lie. Use neutral
		// wording that matches reality on every retry occasion.
		if zh {
			return "⟳ 正在重新生成改动方案"
		}
		return "⟳ Regenerating the change plan"
	case types.StageApply:
		if zh {
			return "⟳ 正在重新应用改动"
		}
		return "⟳ Re-applying changes"
	case types.StageVerify:
		if zh {
			return "⟳ 正在重新跑测试"
		}
		return "⟳ Re-running verification tests"
	}
	return softRetryHintMessage(lang)
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

// softFallbackTargetMessage (B.7+ audit followup, 2026-05-02) renders a
// stage-specific dock line when Block 3's selective-fallback policy
// requeues a particular layer. Pre-this-helper, every fallback target
// emitted the same softAnswerCheckRetryMessage ("answer needs another
// pass"), which was opaque about WHICH part of the pipeline was being
// re-run — users could not tell whether the system was just polishing
// the answer or going back to re-investigate from scratch.
//
// One sentence per target, soft glyph, no internal jargon ("FailLoud" /
// "BackToExtract" never reach the user).
//
//   - FailLoud: terminal — answer ships with caveat, no further retry.
//   - FinalizerOnly: just the finalizer LLM re-runs; evidence intact.
//   - BackToExtract: extract layer re-runs (extractor draft regenerated;
//     evidence + scanned set preserved).
//   - BackToExplore: explore layer re-runs (full re-investigation;
//     evidence cleared, scanned set + read set preserved).
//   - BackToAnalyze: reserved enum, never used in defaults; degrades
//     to the generic answer-check message for safety.
func softFallbackTargetMessage(lang string, target FallbackTarget) string {
	zh := preferZhMessage(lang)
	switch target {
	case FallbackFailLoud:
		if zh {
			return "· 答案存在未解决问题,已无法通过重试修复"
		}
		return "· Answer has unresolved issues that retry cannot fix"
	case FallbackFinalizerOnly:
		if zh {
			return "⟳ 答案待完善,正在重新组织表述"
		}
		return "⟳ Answer needs polishing — re-composing"
	case FallbackBackToExtract:
		if zh {
			return "⟳ 答案结构待修正,回到结构化阶段"
		}
		return "⟳ Answer structure needs fixing — restructuring"
	case FallbackBackToExplore:
		if zh {
			return "⟳ 答案证据不足,回到调查阶段"
		}
		return "⟳ Answer needs more evidence — re-investigating"
	}
	return softAnswerCheckRetryMessage(lang)
}

// noticeKindForFallbackTarget maps a Block 3 fallback target to the
// dock NoticeKind used when surfacing the corresponding soft message.
// FailLoud is the terminal "shipping with caveat" path — informational,
// gray. The three layer-rerun targets are active retry signals — yellow.
// Keeps the call site at orchestrator.go:emit clean.
func noticeKindForFallbackTarget(target FallbackTarget) render.OrchestratorNoticeKind {
	switch target {
	case FallbackFailLoud:
		return render.NoticeFallbackFailLoud
	case FallbackFinalizerOnly:
		return render.NoticeFallbackFinalizerOnly
	case FallbackBackToExtract:
		return render.NoticeFallbackBackToExtract
	case FallbackBackToExplore:
		return render.NoticeFallbackBackToExplore
	}
	// Unknown / reserved (BackToAnalyze) — degrade to gray info so the
	// dock still reads as a soft notice rather than dropping silent.
	return render.NoticeFallbackFailLoud
}

// softUpstreamFallbackCapMessage (B.7+ audit followup, 2026-05-02)
// renders the dock line shown when Block 3's max-upstream-fallbacks
// cap is reached and the next iteration is forced into FailLoud
// regardless of the kind→target policy mapping. Pre-this-helper, the
// user only saw the generic "answer needs another pass" message
// followed silently by the fail-loud header in the answer text;
// the cap event itself was invisible to the dock, leaving the user
// unaware that the retry was capped rather than naturally completed.
func softUpstreamFallbackCapMessage(lang string, used, cap int) string {
	if preferZhMessage(lang) {
		return fmt.Sprintf("· 已达调查回退上限 (%d/%d),不再回退,以现有证据作答", used, cap)
	}
	return fmt.Sprintf("· Upstream fallback cap reached (%d/%d) — finalizing with current evidence", used, cap)
}

// softYieldKillMessage (A.2 audit followup, 2026-05-02) renders the
// dock line shown when the yield-delta kill gate fires (a retry
// window produced no new information on any tracked axis). After
// A.2, this means the same violation kind kept re-firing without
// progress on ForcedReads / ScannedSet / DistinctViolationKindCount /
// PatchesApplied — the loop is structurally stuck and shipping is
// safer than another wasted dispatch. Pre-this-helper, the dock
// showed the generic answer-check retry then jumped to fail-loud
// with no explanation of WHY the retry stopped early.
func softYieldKillMessage(lang string) string {
	if preferZhMessage(lang) {
		return "· 重试无新进展,以现有结论作答"
	}
	return "· Retry produced no new progress — finalizing"
}

// softPlanCriticReviewMessage (Block 1 audit followup, 2026-05-02)
// renders the dock line shown right after the plan_critic LLM
// returned its risks[]. The reviewer's full critique persists on
// ChangePlan.PlanCritique (visible via `/plan show`); the dock
// message confirms the review ran and how many risks it flagged so
// the user knows the system is doing its review before they /approve.
//
// Risks count of 0 = clean review (still worth showing to confirm the
// reviewer ran); any positive count = "you may want to read the full
// critique via /plan show before approving".
func softPlanCriticReviewMessage(lang string, riskCount int) string {
	zh := preferZhMessage(lang)
	if riskCount == 0 {
		if zh {
			return "· 方案已审阅,未发现风险点"
		}
		return "· Plan reviewed — no risks flagged"
	}
	if zh {
		return fmt.Sprintf("· 方案已审阅,记录 %d 项风险点 (查看 /plan show)", riskCount)
	}
	return fmt.Sprintf("· Plan reviewed — %d risk(s) flagged (see /plan show)", riskCount)
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
