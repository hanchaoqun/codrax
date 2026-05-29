package orchestrator

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// user_messages.go — short localized strings rendered into
// EventOrchestratorNotice when the orchestrator needs to update the
// user about internal scheduler decisions (forced reads, convergence
// stalls, etc.). Pre-this-event the strings flowed through
// EventAgentReasoning and the dock rendered them with the same
// `⋯ <stage> · Round N …` style as genuine LLM thinking — users
// could not tell which lines were the LLM and which were the
// orchestrator. The new event drops the thinking marker + trace label so
// the two surfaces are visually distinct.
//
// Design contract:
//
//   - Soft symbols (⟳, ›, ·, ⊘) only — aligned with the project-wide
//     palette (status_tokens.go: glyphRecoverable / glyphProgress /
//     glyphPending / glyphCancelled). ⟳ is reserved for actual retry /
//     re-run / rewrite. › means forward progress or active non-retry
//     work. Bright glyphs (📊, ✅, 🔥) are avoided because they would
//     dominate the TUI at the cadence the orchestrator emits these events.
//   - The bucket color (yellow / gray / cyan, picked by the
//     OrchestratorNoticeKind passed to formatOrchestratorNotice) is
//     the primary semantic discriminator; the inline dot/⟳ glyph is
//     a secondary anchor. So a cyan-color "· Investigation done"
//     vs a gray-color "· Plan reviewed" share the same glyph but
//     read distinctly thanks to color.
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
		return "› 正在补充关键信息"
	}
	return "› Filling in missing context"
}

// softConvergenceStallMessage renders the user-visible line for the
// CGEC I4 convergence detector when the investigation has plateaued
// and the orchestrator force-completes with current evidence.
func softConvergenceStallMessage(lang string) string {
	if preferZhMessage(lang) {
		return "· 基于现有证据作答"
	}
	return "· Finalizing with available evidence"
}

func lowGroundingWarningMessage(lang, profile string, groundingPct, tier1Pct int) string {
	if preferZhMessage(lang) {
		return fmt.Sprintf("· 证据锚点偏弱（profile=%s，grounded=%d%%，line-text=%d%%），将继续用引用校验兜底", profile, groundingPct, tier1Pct)
	}
	return fmt.Sprintf("· Evidence grounding is weak (profile=%s, grounded=%d%%, line-text=%d%%); continuing with citation checks", profile, groundingPct, tier1Pct)
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
		return "› 检查答案是否前后一致"
	}
	return "› Checking the answer for inconsistencies"
}

// semanticQualityReviewStartMessage (P7, 2026-05-10): renders the
// user-visible status line shown when the G5 semantic-quality
// reviewer LLM dispatches. Symmetric counterpart of
// selfConsistencyReviewStartMessage — the user sees a parallel
// "› 正在审阅答案完整性 / Reviewing answer coverage" cue so the
// ~3-5s second-reviewer pause is visible (rather than silent dock
// time the operator can't account for).
//
// Red-line audit (R3/R4/R5/R6/R7/SST/CN+EN-only):
//   - R3 typed: no mutable token; static prose
//   - R4 generic: no project / case names
//   - R5: progress cue, never writes answer body
//   - R6 no internal vocab: "答案完整性" / "answer coverage" are
//     user-vocab paraphrases of FacetCoverage; "G5" / "reviewer"
//     never surface
//   - R7: pure status visibility, no redirect
//   - SST: shape mirrors selfConsistencyReviewStartMessage
//   - CN+EN-only per 2026-05-10 user direction
func semanticQualityReviewStartMessage(lang string) string {
	if preferZhMessage(lang) {
		return "› 正在审阅答案完整性"
	}
	return "› Reviewing answer coverage"
}

func combinedAnswerReviewStartMessage(lang string) string {
	if preferZhMessage(lang) {
		return "› 正在审阅答案完整性与一致性"
	}
	return "› Reviewing answer coverage and consistency"
}

// selfConsistencyContradictionMessage (commit 62): rendered when
// the reviewer reports >= 1 contradiction at confidence >= floor.
// Variants: "rewriting" only when the current soft/strict policy will
// actually re-dispatch finalizer; "logged" when the contradiction ships as an
// advisory caveat.
func selfConsistencyContradictionMessage(lang string, rewrite bool, count int) string {
	if preferZhMessage(lang) {
		if rewrite {
			return fmt.Sprintf("⟳ 检测到 %d 处前后不一致，正在重写答案", count)
		}
		return fmt.Sprintf("· 检测到 %d 处前后不一致（仅记录，未重写）", count)
	}
	if rewrite {
		return fmt.Sprintf("⟳ Found %d inconsistency(ies) — rewriting the answer", count)
	}
	return fmt.Sprintf("· Found %d inconsistency(ies) (logged, not rewritten)", count)
}

// softRetryHintMessage renders the user-visible line for the generic
// stage-retry cue ("the previous agent turn did not satisfy every
// validation requirement yet; running one more pass").
// Replaces the raw RetryHint body dump that used to land in the
// reasoning feed — the body is an internal LLM-directed prompt
// (with `## Evidence Gaps`, `[MISSING]`, `(entities: ...)` markup)
// that leaks terminology users cannot interpret. The full body
// remains in the debug log via `[explorer] retry hint built key=…`.
//
// Also used for the scheduler-level node SC retry cue (when an
// evidence/validate node's SuccessCriteria fails and the scheduler
// requeues for another pass) and the pre-finalize Tier-1 floor
// backtrack — both are "one more validation pass before answering"
// from the user's point of view.
// P7 follow-up (2026-05-17): the previous "更多上下文信息" wording was
// too vague for evidence-gate retries and made users think the
// system was wandering. Keep the copy user-facing, but name the
// actual action: a validation pass needs steadier evidence.
func softRetryHintMessage(lang string) string {
	if preferZhMessage(lang) {
		return "⟳ 验证还不够稳，正在补一轮"
	}
	return "⟳ Validation needs one more pass"
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
		return "本轮没生成改动方案。下一步两选一:\n\n" +
			"  • 咨询类问题(怎么装、为什么报错、是什么原因):/mode read 后再问一次\n" +
			"  • 真要改代码:把目标说具体(改哪个文件 / 加什么 / 接口长什么样)再发一遍"
	}
	return "No actionable change plan was produced this turn. Pick one:\n\n" +
		"  • Advice / how-to question (e.g. \"how do I install X?\"): /mode read, then re-ask\n" +
		"  • Real code change: re-send with a specific target — which file, what to add, what interface"
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
	case types.StageAnalyze:
		if zh {
			return "⟳ 模型响应出错,正在重新理解问题"
		}
		return "⟳ Model response error, re-understanding the question"
	case types.StageExplore:
		if zh {
			return "⟳ 模型响应出错,正在重新收集证据"
		}
		return "⟳ Model response error, re-collecting evidence"
	case types.StageExtract:
		if zh {
			return "⟳ 模型响应出错,正在重新提炼关键发现"
		}
		return "⟳ Model response error, re-extracting key findings"
	case types.StageFinalize:
		if zh {
			return "⟳ 模型响应出错,正在重新撰写答案"
		}
		return "⟳ Model response error, re-composing the final answer"
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

func softTransportRetryHintForStage(lang string, stage types.PipelineStage) string {
	labelZh, labelEn := transportRetryStageLabel(stage)
	if preferZhMessage(lang) {
		return fmt.Sprintf("⟳ 连接/流式响应异常，正在重试模型请求（%s）", labelZh)
	}
	return fmt.Sprintf("⟳ Connection/stream issue; retrying model request (%s)", labelEn)
}

func softTransportCheckpointRetryHintForStage(lang string, stage types.PipelineStage) string {
	labelZh, labelEn := transportRetryStageLabel(stage)
	if preferZhMessage(lang) {
		return fmt.Sprintf("⟳ 连接/流式响应异常，已保留阶段进展并续跑（%s）", labelZh)
	}
	return fmt.Sprintf("⟳ Connection/stream issue; continuing from preserved progress (%s)", labelEn)
}

func transportRetryStageLabel(stage types.PipelineStage) (zh, en string) {
	switch stage {
	case types.StageAnalyze:
		return "理解问题", "understanding the request"
	case types.StageExplore:
		return "收集证据", "collecting evidence"
	case types.StageExtract:
		return "提炼关键发现", "extracting key findings"
	case types.StageFinalize:
		return "撰写最终答案", "composing the final answer"
	case types.StagePlan:
		return "生成改动方案", "drafting the change plan"
	case types.StageApply:
		return "应用改动", "applying changes"
	case types.StageVerify:
		return "运行验证", "running verification"
	default:
		return "当前阶段", "current stage"
	}
}

func softAgentOutputRetryMessage(bus *types.BusContext, lang string, stage types.PipelineStage, missing types.MissingPiece) string {
	if stage == types.StageExplore && missing == types.MissingFacts {
		if preferZhMessage(lang) {
			if exploreRuntimeTraceContinuationLikely(bus, nil) || exploreChainContinuationLikely(bus) {
				return "⟳ 继续上次调查，沿已定位线索补齐关键链路"
			}
			return "⟳ 正在补充关键信息"
		}
		if exploreRuntimeTraceContinuationLikely(bus, nil) || exploreChainContinuationLikely(bus) {
			return "⟳ Continuing the prior investigation from located clues"
		}
		return "⟳ Filling in key context"
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
		return "› 调查完成，准备作答"
	}
	return "› Investigation done — preparing the answer"
}

func softProceedWithCollectedEvidenceMessage(lang string) string {
	if preferZhMessage(lang) {
		return "› 已保留收集到的证据，继续作答"
	}
	return "› Keeping collected evidence — preparing the answer"
}

func softProceedWithExtractedFindingsMessage(lang string) string {
	if preferZhMessage(lang) {
		return "› 已保留提炼结果，继续成文"
	}
	return "› Keeping extracted findings — composing the answer"
}

// softAnswerCheckRetryMessage renders the user-visible line for
// the post-finalize answer contract backtrack: the finalizer
// produced a draft that did not pass the quality checks, and the
// pipeline is running another pass with tightened hints. Distinct
// from softRetryHintMessage because the scope is different — this
// retry starts AFTER a candidate answer was produced, so the user
// message emphasises "answer" rather than "evidence".
// P7 (2026-05-10) wording polish: "再跑一轮" was technical ("跑"
// = "run"; "一轮" = "iteration") and didn't tell the user WHAT was
// running. New phrasing names the action explicitly. R6 / R4 /
// CN+EN-only audit clean.
func softAnswerCheckRetryMessage(lang string) string {
	if preferZhMessage(lang) {
		return "⟳ 答案待完善，正在重新生成"
	}
	return "⟳ Refining the answer — regenerating"
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
			return "· 当前答案已保留，并补充未解决边界"
		}
		return "· Keeping the answer with unresolved-boundary notes"
	case FallbackFinalizerOnly:
		if zh {
			return "⟳ 正在重写最终答案"
		}
		return "⟳ Rewriting the final answer"
	case FallbackBackToExtract:
		if zh {
			return "⟳ 正在重新整理答案结构"
		}
		return "⟳ Reworking the answer structure"
	case FallbackBackToExplore:
		// P7 (2026-05-10) wording polish: "证据不足" was easily
		// misread as "the user's input was insufficient" — actually
		// the system needs to gather more context internally. New
		// phrasing makes the subject explicit (the system, not the
		// user) and avoids "探索" which sounds technical.
		if zh {
			return "⟳ 回到证据阶段补充关键上下文"
		}
		return "⟳ Returning to evidence collection for key context"
	}
	return softAnswerCheckRetryMessage(lang)
}

func softFinalizerProseFallbackMessage(lang string) string {
	if preferZhMessage(lang) {
		return "· 结构化成文未完成，已降级展示模型散文或已收集证据"
	}
	return "· Structured answer rendering did not complete — showing model prose or collected evidence"
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
		return fmt.Sprintf("· 已达重试上限 (%d/%d)，基于现有证据作答", used, cap)
	}
	return fmt.Sprintf("· Retry budget reached (%d/%d) — finalizing with what we have", used, cap)
}

// softFinalizeRepairCapMessage (P6, 2026-05-10) renders the dock
// notice shown when the finalize-stage repair-loop hard cap is
// reached and the answer is shipped with residual-concern details
// instead of another LLM round. Mirrors the
// softUpstreamFallbackCapMessage shape but states "answer delivered
// + N concerns flagged" and gives an actionable next step.
//
// multiRepo gates the sub-repo-specific suggestion (/repos
// adjustment): single-repo runs never receive that hint because it
// is structurally inapplicable to a single-sub-repo workspace —
// surfacing it would mislead operators into thinking a non-existent
// option exists.
//
// Red-line audit (R3/R4/R5/R6/R7/SST/CN+EN-only):
//   - R3 typed: %d concern count + multiRepo bool are the only
//     mutable inputs
//   - R4 generic: no case-specific phrasing, no project names
//   - R5 not system-write-answer: dock/status surface, not answer body
//   - R6 no internal vocab: "次级关注点" / "residual concern(s)"
//     are user-vocab paraphrases, not ViolKind names
//   - R7 user intent: actionable next steps; multi-repo branch
//     names the actual REPL command (`/repos`) inline as a code
//     identifier (`backticks`) — not English prose mixed into
//     the Chinese sentence
//   - SST: shape matches existing softXxxCapMessage helpers
//   - CN+EN only: per 2026-05-10 user direction. Each branch is
//     in ONE natural language only; CLI command names (`/repos`)
//     count as code identifiers, not English prose, so they are
//     legitimately preserved across both branches (matches how
//     the rest of the codebase renders CLI commands inline).
func softFinalizeRepairCapMessage(lang string, concernCount int, multiRepo bool) string {
	if preferZhMessage(lang) {
		if concernCount <= 0 {
			return "· 答案已交付（已用满质量审阅重试预算）"
		}
		if multiRepo {
			return fmt.Sprintf("· 答案已交付。质量审阅检测到 %d 项次级关注点（详见下方说明）。如需更精确的覆盖，可细化问题，或通过 `/repos` 调整活跃子仓集合后重提",
				concernCount)
		}
		return fmt.Sprintf("· 答案已交付。质量审阅检测到 %d 项次级关注点（详见下方说明）。如需更精确的覆盖，可细化问题后重提",
			concernCount)
	}
	if concernCount <= 0 {
		return "· Answer delivered (quality-review retry budget exhausted)"
	}
	if multiRepo {
		return fmt.Sprintf("· Answer delivered. Quality review flagged %d residual concern(s) (details follow). For tighter coverage, narrow the question or adjust the active sub-repo set via `/repos` and re-ask",
			concernCount)
	}
	return fmt.Sprintf("· Answer delivered. Quality review flagged %d residual concern(s) (details follow). For tighter coverage, narrow the question and re-ask",
		concernCount)
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
		return "· 重试无新进展，基于现有结论作答"
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
			return "· 方案已审阅，未发现风险点"
		}
		return "· Plan reviewed — no risks flagged"
	}
	if zh {
		return fmt.Sprintf("· 方案已审阅，记录 %d 项风险点(查看 /plan show)", riskCount)
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
		return "› 正在生成最终答案"
	}
	return "› Composing the final answer"
}

// softCompletenessGapMessage renders the user-visible advisory
// emitted when a Phase 2 Tier 2 CompletenessValidator detects a
// structural-coverage gap at finalize entry (count question without
// deterministic-tool output, call-chain answer with insufficient
// function mentions, config precedence missing a layer, ...). The
// message names the dimension that fell short so the user sees what
// the system noticed; the LLM's actionable FixHint goes into the
// finalize prompt separately via Mutable.SetTier2CompletenessHint.
//
// R6-clean — no internal pipeline term names.
func softCompletenessGapMessage(lang string, fail *agent.CompletenessFailure) string {
	if fail == nil {
		return ""
	}
	dim := dimensionUserLabel(lang, fail.Dimension)
	// P7 (2026-05-10) wording polish: "维度" was technical; "方面"
	// reads cleanly. "提示模型" leaked the internal "model" abstraction;
	// "引导补足" stays at the user-vocabulary layer. R6 audit clean.
	if preferZhMessage(lang) {
		return "⟳ 答案在「" + dim + "」方面可能覆盖不足，正在引导补足"
	}
	return "⟳ Answer may under-cover the " + dim + " aspect; guiding the next pass to fill the gap"
}

// dimensionUserLabel converts a CompletionDimension into a user-
// readable phrase for the soft-notice. Keeps the LLM-facing FixHint
// (which gives concrete instructions) separate from the user-facing
// notice (which only names the dimension). R6-clean.
func dimensionUserLabel(lang string, dim agent.CompletionDimension) string {
	zh := preferZhMessage(lang)
	switch dim {
	case agent.DimensionScalarCount:
		if zh {
			return "数量统计"
		}
		return "scalar count"
	case agent.DimensionCardinality:
		if zh {
			return "枚举完整性"
		}
		return "enumeration cardinality"
	case agent.DimensionPathDepth:
		if zh {
			return "调用链覆盖"
		}
		return "call-chain coverage"
	case agent.DimensionEntityParity:
		if zh {
			return "对比平衡"
		}
		return "comparison balance"
	default:
		if zh {
			return "答案完整性"
		}
		return "answer completeness"
	}
}

// softProceedingWithoutExtractMessage renders the user-visible line
// emitted when the pre-finalize extract dispatch failed but the
// orchestrator decided to keep going (the next step is finalize on
// whatever evidence was already collected). Pre-this-fix the user
// saw "✗ 未能提炼关键发现" followed immediately by "正在生成最终答案"
// with NOTHING in between explaining the transition — they had no way
// to tell the system was deliberately falling back rather than
// silently swallowing a failure.
//
// Surface text spells out (a) which step failed, (b) what the system
// is doing about it. Uses the progress glyph because the system is
// proceeding to finalize with available evidence, not retrying extract.
func softProceedingWithoutExtractMessage(lang string) string {
	if preferZhMessage(lang) {
		return "› 提炼关键发现失败，将基于已有证据继续撰写最终答案"
	}
	return "› Key-finding distillation failed; finalizing with the evidence already collected"
}

// multiRepoScanStartMessage / multiRepoScanEndMessage render the
// user-visible push lines for Phase 4 multi-repo scan progress.
//
// 2026-05-08 glyph audit (operator feedback):
//
//	⟳ (yellow rotation arrow) → RETRY ONLY (planner regenerate /
//	                            apply re-run / verify re-run /
//	                            answer rewrite). NOT a generic
//	                            "in-progress" cue.
//	· (gray dot)              → in-progress. The system is doing
//	                            work that has not yet completed.
//	✓ (checkmark)             → milestone complete (success).
//	✗ (cross)                 → failure / scan errored.
//
// First-time scan is in-progress (NOT a retry). Concrete shapes:
//
//	· 正在扫描子仓 `repo-x`           / · Scanning sub-repo `repo-x`
//	✓ 子仓 `repo-x` 已就绪 (1.2s)      / ✓ Sub-repo `repo-x` ready (1.2s)
//	✗ 子仓 `repo-x` 扫描失败 (820ms)   / ✗ Sub-repo `repo-x` scan failed (820ms)
//
// All three runes are in status_tokens.go's canonical set so
// peelGlyphPrefix splits glyph from body and the dock applies the
// bucket palette to the glyph + the muted prose palette to the body
// — same treatment as the rest of the soft-notice family. Bilingual
// zh / en parity preserved.
func multiRepoScanStartMessage(lang, rootRel string) string {
	if preferZhMessage(lang) {
		return "· 正在扫描子仓 `" + rootRel + "`"
	}
	return "· Scanning sub-repo `" + rootRel + "`"
}

func multiRepoScanEndMessage(lang, rootRel string, elapsedMs int64, ok bool) string {
	zh := preferZhMessage(lang)
	elapsed := formatScanElapsed(elapsedMs)
	if !ok {
		if zh {
			return "✗ 子仓 `" + rootRel + "` 扫描失败 (" + elapsed + ")"
		}
		return "✗ Sub-repo `" + rootRel + "` scan failed (" + elapsed + ")"
	}
	if zh {
		return "✓ 子仓 `" + rootRel + "` 已就绪 (" + elapsed + ")"
	}
	return "✓ Sub-repo `" + rootRel + "` ready (" + elapsed + ")"
}

func repoMapScanMessage(lang string, ev types.RepoMapScanEvent) string {
	label := repoMapScanLabel(ev)
	if ev.Finished {
		return repoMapScanEndMessage(lang, label, ev)
	}
	if ev.Progress {
		return repoMapScanProgressMessage(lang, label, ev)
	}
	return repoMapScanStartMessage(lang, label, ev)
}

func repoMapScanStartMessage(lang, label string, ev types.RepoMapScanEvent) string {
	subjectZH, subjectEN := repoMapScanSubject(ev)
	if preferZhMessage(lang) {
		if ev.Phase == types.RepoMapScanPhaseFileScan && ev.TotalFiles <= 0 {
			return fmt.Sprintf("· 正在统计%s `%s` 文件", subjectZH, label)
		}
		if action := repoMapScanPhaseActionZH(ev.Phase); action != "" && ev.TotalFiles <= 0 {
			return fmt.Sprintf("· %s `%s` %s", subjectZH, label, action)
		}
		if ev.Phase == types.RepoMapScanPhaseCacheLoad {
			return fmt.Sprintf("· 正在读取%s `%s` 缓存：%s", subjectZH, label, repoMapScanCountsZH(ev, false))
		}
		if action := repoMapScanPhaseActionZH(ev.Phase); action != "" {
			return fmt.Sprintf("· %s `%s` %s：%s", subjectZH, label, action, repoMapScanCountsZH(ev, false))
		}
		return fmt.Sprintf("· 正在扫描%s `%s`：%s", subjectZH, label, repoMapScanCountsZH(ev, false))
	}
	if ev.Phase == types.RepoMapScanPhaseFileScan && ev.TotalFiles <= 0 {
		return fmt.Sprintf("· Counting files for %s `%s`", subjectEN, label)
	}
	if action := repoMapScanPhaseActionEN(ev.Phase); action != "" && ev.TotalFiles <= 0 {
		return fmt.Sprintf("· %s `%s` %s", subjectEN, label, action)
	}
	if ev.Phase == types.RepoMapScanPhaseCacheLoad {
		return fmt.Sprintf("· Loading cached %s `%s`: %s", subjectEN, label, repoMapScanCountsEN(ev, false))
	}
	if action := repoMapScanPhaseActionEN(ev.Phase); action != "" {
		return fmt.Sprintf("· %s `%s` %s: %s", subjectEN, label, action, repoMapScanCountsEN(ev, false))
	}
	return fmt.Sprintf("· Scanning %s `%s`: %s", subjectEN, label, repoMapScanCountsEN(ev, false))
}

func repoMapScanProgressMessage(lang, label string, ev types.RepoMapScanEvent) string {
	subjectZH, subjectEN := repoMapScanSubject(ev)
	if preferZhMessage(lang) {
		if action := repoMapScanPhaseActionZH(ev.Phase); action != "" {
			return fmt.Sprintf("· %s `%s` %s：%s", subjectZH, label, action, repoMapScanCountsZH(ev, true))
		}
		return fmt.Sprintf("· %s `%s` 扫描中：%s", subjectZH, label, repoMapScanCountsZH(ev, true))
	}
	if action := repoMapScanPhaseActionEN(ev.Phase); action != "" {
		return fmt.Sprintf("· %s `%s` %s: %s", subjectEN, label, action, repoMapScanCountsEN(ev, true))
	}
	return fmt.Sprintf("· %s `%s` scanning: %s", subjectEN, label, repoMapScanCountsEN(ev, true))
}

func repoMapScanEndMessage(lang, label string, ev types.RepoMapScanEvent) string {
	elapsed := formatScanElapsed(ev.ElapsedMs)
	subjectZH, subjectEN := repoMapScanSubject(ev)
	if ev.Phase == types.RepoMapScanPhaseViewRender {
		if !ev.OK {
			if preferZhMessage(lang) {
				return fmt.Sprintf("✗ repo_map 视图 `%s` 生成失败 (%s)", label, elapsed)
			}
			return fmt.Sprintf("✗ repo_map view `%s` failed (%s)", label, elapsed)
		}
		if preferZhMessage(lang) {
			return fmt.Sprintf("✓ repo_map 视图 `%s` 已生成：%s (%s)", label, repoMapScanCountsZH(ev, false), elapsed)
		}
		return fmt.Sprintf("✓ repo_map view `%s` generated: %s (%s)", label, repoMapScanCountsEN(ev, false), elapsed)
	}
	if !ev.OK {
		if preferZhMessage(lang) {
			return fmt.Sprintf("✗ %s `%s` 扫描失败 (%s)", subjectZH, label, elapsed)
		}
		return fmt.Sprintf("✗ %s `%s` scan failed (%s)", subjectEN, label, elapsed)
	}
	if preferZhMessage(lang) {
		return fmt.Sprintf("✓ %s `%s` 已就绪：%s (%s)", subjectZH, label, repoMapScanCountsZH(ev, false), elapsed)
	}
	return fmt.Sprintf("✓ %s `%s` ready: %s (%s)", subjectEN, label, repoMapScanCountsEN(ev, false), elapsed)
}

func repoMapScanCountsZH(ev types.RepoMapScanEvent, progress bool) string {
	total := ev.TotalFiles
	parseable := ev.ParseableFiles
	if ev.Phase == types.RepoMapScanPhaseFileScan && total <= 0 {
		return "文件统计中"
	}
	if ev.Phase == types.RepoMapScanPhaseFileScan && progress {
		if strings.TrimSpace(ev.CurrentFile) != "" {
			if parseable > 0 {
				return fmt.Sprintf("正在检查 %s；已统计 %d 个文件，%d 个源文件", ev.CurrentFile, total, parseable)
			}
			return fmt.Sprintf("正在检查 %s；已统计 %d 个文件", ev.CurrentFile, total)
		}
		if parseable > 0 {
			return fmt.Sprintf("已统计 %d 个文件，%d 个源文件", total, parseable)
		}
		return fmt.Sprintf("已统计 %d 个文件", total)
	}
	if ev.Phase == types.RepoMapScanPhaseChangeScan && progress {
		return fmt.Sprintf("已校验 %d/%d 个文件", ev.ParsedFiles, total)
	}
	if ev.Phase == types.RepoMapScanPhaseChangeScan {
		return fmt.Sprintf("准备校验 %d 个文件", total)
	}
	if ev.Phase == types.RepoMapScanPhaseCacheLoad {
		loaded := ev.CacheRecordsLoaded
		if loaded <= 0 && progress {
			loaded = ev.ParsedFiles
		}
		cacheTotal := ev.CacheRecordsTotal
		if cacheTotal <= 0 {
			cacheTotal = total
		}
		if progress {
			if ev.CacheChunksTotal > 0 {
				return fmt.Sprintf("已读取 %d/%d 条索引记录，缓存分块 %d/%d", loaded, cacheTotal, ev.CacheChunksLoaded, ev.CacheChunksTotal)
			}
			return fmt.Sprintf("已读取 %d/%d 条索引记录", loaded, cacheTotal)
		}
		return fmt.Sprintf("准备读取 %d 条索引记录", cacheTotal)
	}
	if ev.Phase == types.RepoMapScanPhaseViewRender {
		if progress {
			step := strings.TrimSpace(ev.ViewStep)
			if step != "" && ev.ViewStepsTotal > 0 {
				return fmt.Sprintf("正在生成 %s；步骤 %d/%d（图包含 %d 个文件）", step, ev.ViewStepsDone, ev.ViewStepsTotal, total)
			}
			return fmt.Sprintf("图包含 %d 个文件", total)
		}
		return fmt.Sprintf("图包含 %d 个文件", total)
	}
	if progress && ev.Phase == types.RepoMapScanPhaseCacheWrite && strings.TrimSpace(ev.CurrentFile) != "" && ev.BytesWritten > 0 {
		return fmt.Sprintf("正在写入 %s（%s）；源文件已解析 %d/%d（总文件 %d）",
			ev.CurrentFile, formatByteProgressZH(ev.BytesWritten, ev.BytesTotal), parseable, parseable, total)
	}
	if progress && ev.Mode == types.RepoMapScanCacheHit && ev.Phase != "" {
		if parseable > 0 {
			return fmt.Sprintf("缓存已读取，已加载 %d 个源文件（总文件 %d）", parseable, total)
		}
		return fmt.Sprintf("缓存已读取，%d 个文件", total)
	}
	if ev.Mode == types.RepoMapScanCacheHit {
		return fmt.Sprintf("缓存命中，%d 个文件", total)
	}
	if parseable <= 0 {
		return fmt.Sprintf("%d 个文件", total)
	}
	if progress {
		if ev.Phase == types.RepoMapScanPhaseParse && strings.TrimSpace(ev.CurrentFile) != "" {
			return fmt.Sprintf("正在解析 %s；已完成 %d/%d 个源文件（总文件 %d）", ev.CurrentFile, ev.ParsedFiles, parseable, total)
		}
		if ev.Phase != "" && ev.Phase != types.RepoMapScanPhaseParse {
			return fmt.Sprintf("源文件已解析 %d/%d（总文件 %d）", parseable, parseable, total)
		}
		return fmt.Sprintf("已解析 %d/%d 个源文件（总文件 %d）", ev.ParsedFiles, parseable, total)
	}
	if (ev.Mode == types.RepoMapScanIncremental || ev.Mode == types.RepoMapScanFullRescan) && ev.ChangedFiles > 0 && ev.ChangedFiles < total {
		return fmt.Sprintf("%d 个文件，需更新 %d 个，解析 %d 个源文件", total, ev.ChangedFiles, parseable)
	}
	return fmt.Sprintf("%d 个文件，解析 %d 个源文件", total, parseable)
}

func repoMapScanCountsEN(ev types.RepoMapScanEvent, progress bool) string {
	total := ev.TotalFiles
	parseable := ev.ParseableFiles
	if ev.Phase == types.RepoMapScanPhaseFileScan && total <= 0 {
		return "counting files"
	}
	if ev.Phase == types.RepoMapScanPhaseFileScan && progress {
		if strings.TrimSpace(ev.CurrentFile) != "" {
			if parseable > 0 {
				return fmt.Sprintf("checking %s; counted %d files, %d source files", ev.CurrentFile, total, parseable)
			}
			return fmt.Sprintf("checking %s; counted %d files", ev.CurrentFile, total)
		}
		if parseable > 0 {
			return fmt.Sprintf("counted %d files, %d source files", total, parseable)
		}
		return fmt.Sprintf("counted %d files", total)
	}
	if ev.Phase == types.RepoMapScanPhaseChangeScan && progress {
		return fmt.Sprintf("checked %d/%d files", ev.ParsedFiles, total)
	}
	if ev.Phase == types.RepoMapScanPhaseChangeScan {
		return fmt.Sprintf("preparing to check %d files", total)
	}
	if ev.Phase == types.RepoMapScanPhaseCacheLoad {
		loaded := ev.CacheRecordsLoaded
		if loaded <= 0 && progress {
			loaded = ev.ParsedFiles
		}
		cacheTotal := ev.CacheRecordsTotal
		if cacheTotal <= 0 {
			cacheTotal = total
		}
		if progress {
			if ev.CacheChunksTotal > 0 {
				return fmt.Sprintf("loaded %d/%d index records, cache chunks %d/%d", loaded, cacheTotal, ev.CacheChunksLoaded, ev.CacheChunksTotal)
			}
			return fmt.Sprintf("loaded %d/%d index records", loaded, cacheTotal)
		}
		return fmt.Sprintf("preparing to load %d index records", cacheTotal)
	}
	if ev.Phase == types.RepoMapScanPhaseViewRender {
		if progress {
			step := strings.TrimSpace(ev.ViewStep)
			if step != "" && ev.ViewStepsTotal > 0 {
				return fmt.Sprintf("generating %s; step %d/%d (%d files in graph)", step, ev.ViewStepsDone, ev.ViewStepsTotal, total)
			}
			return fmt.Sprintf("%d files in graph", total)
		}
		return fmt.Sprintf("%d files in graph", total)
	}
	if progress && ev.Phase == types.RepoMapScanPhaseCacheWrite && strings.TrimSpace(ev.CurrentFile) != "" && ev.BytesWritten > 0 {
		return fmt.Sprintf("writing %s (%s); parsed %d/%d source files (%d files total)",
			ev.CurrentFile, formatByteProgressEN(ev.BytesWritten, ev.BytesTotal), parseable, parseable, total)
	}
	if progress && ev.Mode == types.RepoMapScanCacheHit && ev.Phase != "" {
		if parseable > 0 {
			return fmt.Sprintf("cache loaded, loaded %d source files (%d files total)", parseable, total)
		}
		return fmt.Sprintf("cache loaded, %d files", total)
	}
	if ev.Mode == types.RepoMapScanCacheHit {
		return fmt.Sprintf("cache hit, %d files", total)
	}
	if parseable <= 0 {
		return fmt.Sprintf("%d files", total)
	}
	if progress {
		if ev.Phase == types.RepoMapScanPhaseParse && strings.TrimSpace(ev.CurrentFile) != "" {
			return fmt.Sprintf("parsing %s; parsed %d/%d source files (%d files total)", ev.CurrentFile, ev.ParsedFiles, parseable, total)
		}
		if ev.Phase != "" && ev.Phase != types.RepoMapScanPhaseParse {
			return fmt.Sprintf("parsed %d/%d source files (%d files total)", parseable, parseable, total)
		}
		return fmt.Sprintf("parsed %d/%d source files (%d files total)", ev.ParsedFiles, parseable, total)
	}
	if (ev.Mode == types.RepoMapScanIncremental || ev.Mode == types.RepoMapScanFullRescan) && ev.ChangedFiles > 0 && ev.ChangedFiles < total {
		return fmt.Sprintf("%d files, %d changed, %d source files to parse", total, ev.ChangedFiles, parseable)
	}
	return fmt.Sprintf("%d files, %d source files to parse", total, parseable)
}

func formatByteSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(n)
	for _, unit := range units {
		value /= 1024
		if value < 1024 {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%.1f PB", value/1024)
}

func formatByteProgressZH(written, total int64) string {
	if total > 0 && written <= total {
		pct := float64(written) * 100 / float64(total)
		return fmt.Sprintf("%s/%s，%.0f%%", formatByteSize(written), formatByteSize(total), pct)
	}
	return "已写入 " + formatByteSize(written)
}

func formatByteProgressEN(written, total int64) string {
	if total > 0 && written <= total {
		pct := float64(written) * 100 / float64(total)
		return fmt.Sprintf("%s/%s, %.0f%%", formatByteSize(written), formatByteSize(total), pct)
	}
	return formatByteSize(written) + " written"
}

func repoMapScanSubject(ev types.RepoMapScanEvent) (string, string) {
	if strings.TrimSpace(ev.SubRepoRootRel) != "" {
		return "子仓索引", "sub-repo index"
	}
	return "仓库索引", "repo index"
}

func repoMapScanPhaseActionZH(phase types.RepoMapScanPhase) string {
	switch phase {
	case types.RepoMapScanPhaseFileScan:
		return "正在统计文件"
	case types.RepoMapScanPhaseChangeScan:
		return "正在校验缓存差异"
	case types.RepoMapScanPhaseBuildGraph:
		return "正在构建符号关系图"
	case types.RepoMapScanPhaseRank:
		return "正在计算文件排序"
	case types.RepoMapScanPhaseCacheLoad:
		return "正在读取缓存"
	case types.RepoMapScanPhaseCacheWrite:
		return "正在写入索引缓存"
	case types.RepoMapScanPhaseViewRender:
		return "正在生成 repo_map 视图"
	case types.RepoMapScanPhaseWait:
		return "正在等待已有索引构建"
	}
	return ""
}

func repoMapScanPhaseActionEN(phase types.RepoMapScanPhase) string {
	switch phase {
	case types.RepoMapScanPhaseFileScan:
		return "counting files"
	case types.RepoMapScanPhaseChangeScan:
		return "checking cache differences"
	case types.RepoMapScanPhaseBuildGraph:
		return "building the symbol graph"
	case types.RepoMapScanPhaseRank:
		return "ranking files"
	case types.RepoMapScanPhaseCacheLoad:
		return "loading the cache"
	case types.RepoMapScanPhaseCacheWrite:
		return "writing the index cache"
	case types.RepoMapScanPhaseViewRender:
		return "generating the repo_map view"
	case types.RepoMapScanPhaseWait:
		return "waiting for the in-progress index build"
	}
	return ""
}

func repoMapScanLabel(ev types.RepoMapScanEvent) string {
	if label := strings.TrimSpace(ev.DisplayName); label != "" {
		return label
	}
	root := strings.TrimSpace(ev.RepoRoot)
	if root == "" {
		return "."
	}
	label := filepath.Base(filepath.Clean(root))
	if label == "." || label == string(filepath.Separator) || label == "" {
		return root
	}
	return label
}

// formatScanElapsed renders the per-sub-repo scan duration for the
// dock push line. Sub-second scans land in millis, ≥1s gets a
// human-friendly "1.2s" form so a quick scan does not look broken
// next to a slow one.
func formatScanElapsed(ms int64) string {
	if ms < 1000 {
		if ms < 1 {
			return "<1ms"
		}
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
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
			return "无法继续：与模型的连接中断，可能是网络抖动或上游临时不可用，请稍后重试"
		}
		return "Cannot continue: connection to the model dropped (transient network or upstream issue); please retry"
	case strings.Contains(probe, "no deployments available"),
		strings.Contains(probe, "rate limit"),
		strings.Contains(probe, "429"):
		if zh {
			return "无法继续：模型服务暂不可用或限流，请稍后重试"
		}
		return "Cannot continue: model service is unavailable or rate-limited; please retry"
	case strings.Contains(probe, "context canceled"),
		strings.Contains(probe, "context cancelled"):
		if zh {
			return "已取消：本次任务被中断"
		}
		return "Cancelled by user"
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
