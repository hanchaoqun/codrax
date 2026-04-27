package repl

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// messages.go — short user-facing strings localized for the REPL.
// Mirrors internal/orchestrator/user_messages.go's pattern: each
// helper takes a language string and returns the appropriate text.
// The default is zh (matches codrax.yaml :: lang and the wider
// product convention); only `--lang=en` (or any English-shaped
// variant) flips to English. Other values fall back to zh.
//
// SCOPE — language-aware coverage in the REPL is a tiered effort:
//
//   Tier 1 (this file): high-visibility command flow text
//                       (/approve, /reject, /verify confirmation
//                       prompts and outcome nudges, /help banner
//                       hints, /mode switch confirmations). These
//                       are the surfaces a user reads on every
//                       successful or failed write-mode cycle.
//
//   Tier 2 (existing):  banner / degraded-env hints
//                       (renderDegradedEnvHints).
//
//   Tier 3 (left in en): low-impact / debug-flavored messages that
//                        embed file paths, agent IDs, or %v error
//                        values. These pair with stderr-style
//                        diagnostics; translating them adds noise
//                        without improving comprehension.
//
// Adding a new helper: define the zh + en variants in one function,
// take `lang` as the only parameter, return the localised string.
// Test pairs cover both branches via TestMessages_LangAware.

// isZh reports whether the configured language should render zh
// (the default). Only an explicit "en" (case-insensitive) flips to
// English; everything else — empty, "zh", "fr", "ja", "off" —
// stays zh because the REPL chrome is not the same as the answer
// language and zh covers the majority user base.
func isZh(lang string) bool {
	return !strings.EqualFold(strings.TrimSpace(lang), "en")
}

// approveFailedNudge — multi-line nudge surfaced after /approve
// terminates with TaskState.LastError != "". Tells the user that
// /approve does NOT auto-replan (intentional, preserves review
// intent) and walks through the explicit recovery path.
func approveFailedNudge(lang string) []string {
	if isZh(lang) {
		return []string{
			"approve 失败。/approve 不会自动 replan(这是为了保持你的审批意图)。",
			"如需基于本次失败重新规划:",
			"    1. /mode plan",
			"    2. 重新描述需求,提一下哪些测试或诊断失败 — planner 会通过 /history 看到本轮结果。",
		}
	}
	return []string{
		"approve failed. /approve does not auto-replan (preserves your review intent).",
		"To try again with a revised plan:",
		"    1. /mode plan",
		"    2. re-state your request, mentioning what failed — the planner sees this turn's outcome via /history.",
	}
}

// approveDispatchRequest builds the synthetic request string the
// REPL hands to runner.Run for /approve. It must NOT trip
// types.IsREPLControlInput (i.e. its first token cannot be a known
// slash-command alias like "/approve") or the analyzer's
// emit_analysis tool will reject it on every iteration and burn
// the analyzer's iter budget before the orchestrator gets to swap
// in BuildWriteTaskGraph. Using plan.Summary gives the analyzer
// real code-question content; the leading "Apply approved plan"
// phrasing carries the user's intent into memory transcripts.
func approveDispatchRequest(plan *types.ChangePlan) string {
	summary := strings.TrimSpace(plan.Summary)
	if summary == "" {
		summary = "apply the reviewed plan"
	}
	return fmt.Sprintf("Apply approved plan %s: %s", plan.ID, summary)
}

// verifyDispatchRequest mirrors approveDispatchRequest for /verify.
// Same rationale: avoid the REPL-control-input shape so the analyzer
// classifier terminates without rejection retries.
func verifyDispatchRequest(plan *types.ChangePlan) string {
	summary := strings.TrimSpace(plan.Summary)
	if summary == "" {
		summary = "verify the applied plan"
	}
	return fmt.Sprintf("Verify applied plan %s: %s", plan.ID, summary)
}

// approveTitlePrompt — confirmation-dialog title for /approve.
func approveTitlePrompt(lang, planID string, changeCount int) string {
	if isZh(lang) {
		return formatN(lang,
			"是否批准 plan %s (%d 处改动)?将在 git worktree 中 apply + 跑 verify。",
			planID, changeCount)
	}
	return formatN(lang,
		"Approve plan %s (%d change(s))? Apply inside a git worktree + run verify.",
		planID, changeCount)
}

// approveCancelled — user said "no" at the confirmation prompt.
func approveCancelled(lang string) string {
	if isZh(lang) {
		return "已取消 approve"
	}
	return "approve cancelled"
}

// rejectConfirmedWithReason — message printed after /reject lands
// the rejection on disk, when the user supplied a reason.
func rejectConfirmedWithReason(lang, planID, reason string) string {
	if isZh(lang) {
		return formatN(lang, "已拒绝 plan %s — 原因: %s", planID, reason)
	}
	return formatN(lang, "rejected plan %s — reason: %s", planID, reason)
}

// rejectConfirmedNoReason — same, no reason given.
func rejectConfirmedNoReason(lang, planID string) string {
	if isZh(lang) {
		return formatN(lang, "已拒绝 plan %s", planID)
	}
	return formatN(lang, "rejected plan %s", planID)
}

// writeModeDisabled — printed when /mode plan|apply|verify or
// /approve fires while codrax.yaml :: write_enabled is false. The
// pre-fix path silently accepted the transition and the failure
// surfaced deep in the pipeline (analyzer "hypothesis_coverage" or
// "context canceled"); this message names the yaml knob explicitly so
// the user can fix it in one shot. settingsPath is the resolved
// codrax.yaml path (or "" if none found) — when present the error
// names the exact file the user needs to edit, not a generic
// "in codrax.yaml" that forces them to hunt for it.
func writeModeDisabled(lang, mode, settingsPath string) []string {
	target := "codrax.yaml"
	created := ""
	if settingsPath != "" {
		target = settingsPath
	} else {
		// No yaml resolved at startup — point the user at where to
		// CREATE one rather than where to edit a file that doesn't
		// exist yet.
		if isZh(lang) {
			created = "  当前未加载 codrax.yaml。新建一个并设置 write_enabled: true,默认查找路径见 codrax.yaml.example。"
		} else {
			created = "  No codrax.yaml is currently loaded. Create one with write_enabled: true; default lookup paths are documented in codrax.yaml.example."
		}
	}
	if isZh(lang) {
		out := []string{
			formatN(lang, "%s 已被拒绝: codrax.yaml :: write_enabled 为 false (或未设置)", mode),
			formatN(lang, "  在 %s 中设置 `write_enabled: true` 并重启 codrax 即可启用 plan / apply / verify 模式。", target),
			"  read 模式(默认)无需额外配置。",
		}
		if created != "" {
			out = append(out, created)
		}
		return out
	}
	out := []string{
		formatN(lang, "%s rejected: codrax.yaml :: write_enabled is false (or unset)", mode),
		formatN(lang, "  Set `write_enabled: true` in %s and restart codrax to enable plan / apply / verify modes.", target),
		"  Read mode (default) needs no extra configuration.",
	}
	if created != "" {
		out = append(out, created)
	}
	return out
}

// bannerCapabilityLine produces a single line describing the active
// pipeline modes + the codrax.yaml backing them. Surfaced under the
// version badge so the user sees write_enabled state at startup
// rather than discovering it via a /mode plan reject 30 turns in.
// Empty string when there's nothing useful to display (no yaml AND
// write_enabled defaulted to false).
func bannerCapabilityLine(lang string, writeEnabled bool, settingsPath string) string {
	zh := isZh(lang)
	cap := ""
	if writeEnabled {
		if zh {
			cap = "modes: read · plan · apply · verify (write_enabled=true)"
		} else {
			cap = "modes: read · plan · apply · verify (write_enabled=true)"
		}
	} else {
		if zh {
			cap = "modes: read (write_enabled=false — /mode plan / apply / verify 已禁用)"
		} else {
			cap = "modes: read (write_enabled=false — /mode plan / apply / verify disabled)"
		}
	}
	if settingsPath != "" {
		cap += " · " + settingsPath
	}
	return cap
}

// emptyResponseHint replaces the cryptic "??" rendering when a Run
// returned with no error AND no rendered text. Two real-world causes:
// the analyzer/quality-gate produced no consumable result, OR the
// renderer received a structured response without a body. Either way
// printing "??" leaves the user staring at the screen — at least name
// what to do next.
func emptyResponseHint(lang string) string {
	if isZh(lang) {
		return "  (无内容输出 — 可能是分析器拒绝了请求或上游 LLM 返回为空)。详情见 codrax-*.log;尝试换种问法或 /clear 后重试。"
	}
	return "  (no content rendered — likely the analyzer rejected the request or the upstream LLM returned empty). See codrax-*.log for details; rephrase or /clear and retry."
}

// chitchatReplyHeader marks a chitchat-routed reply so the user can
// tell at a glance the answer came from the side LLM (no repo
// analysis, no plan), not from the main pipeline. Pre-fix: the only
// difference between a chitchat reply and a pipeline reply was logged
// at INFO level — invisible at the terminal — so a misrouted code
// question got a generic answer with no recovery hint.
func chitchatReplyHeader(lang string) string {
	if isZh(lang) {
		return "  [chat] 这是闲聊回复(未走代码分析流水线 / 未生成 plan)。如需仓库分析,显式提到具体文件 / 函数 / 行,或先 /chat off 再问。"
	}
	return "  [chat] chitchat reply (no repo analysis, no plan). For repo analysis, mention a specific file / function / line, or invoke the question explicitly without /chat."
}

// noPendingPlan — user typed /approve or /reject without a pending
// plan to act on. Surface the recovery path (/mode plan first), and
// if write_enabled is off, name THAT first since a /mode plan dispatch
// would just bounce off the L2 gate.
func noPendingPlan(lang string) string {
	if isZh(lang) {
		return "没有待处理的 plan — 先 /mode plan 生成一份"
	}
	return "no pending plan — run a /mode plan dispatch to generate one"
}

// noPendingPlanWriteDisabled — same surface as noPendingPlan but for
// the case where write_enabled is off, so the recovery path is "fix
// the yaml first" rather than "run /mode plan".
func noPendingPlanWriteDisabled(lang string) []string {
	if isZh(lang) {
		return []string{
			"没有待处理的 plan,且 write 模式被禁用。",
			"  在 codrax.yaml 中设置 `write_enabled: true` 并重启 codrax,然后 /mode plan 生成 plan。",
		}
	}
	return []string{
		"no pending plan, and write mode is disabled.",
		"  Set `write_enabled: true` in codrax.yaml and restart codrax, then /mode plan to generate one.",
	}
}

// autoInitConsentTitle — interactive y/N prompt body for the bare-
// directory scaffolding flow. /approve fires this before dispatching
// when DetectRepoState reports NeedsInit() and no pre-authorization
// is in effect (yaml write_auto_init_repo / CLI --auto-init-repo).
// stateLabel is "not_initialized" or "no_commits" so the user sees
// which precondition is being lifted.
func autoInitConsentTitle(lang, repoRoot, stateLabel string) string {
	if isZh(lang) {
		return formatN(lang,
			"目标目录 %s 状态:%s。codrax 可以自动 `git init` + 空 initial commit,然后在沙箱 worktree 里 apply。是否同意?",
			repoRoot, stateLabel)
	}
	return formatN(lang,
		"target %s is %s. codrax can run `git init` + an empty initial commit, then apply inside a sandbox worktree. Proceed?",
		repoRoot, stateLabel)
}

// autoInitDeclined — printed when the user answered No to the consent
// prompt. Tells them how to switch on yaml/CLI pre-authorization
// instead so they don't have to answer y every time.
func autoInitDeclined(lang string) []string {
	if isZh(lang) {
		return []string{
			"已取消。Plan 仍保留在 PlanStore (/plan show 可看)。",
			"  下次想直接同意:codrax.yaml :: write_auto_init_repo: true,或单次跑 codrax --auto-init-repo --mode=apply ...",
		}
	}
	return []string{
		"cancelled. The plan is still saved (run /plan show to inspect).",
		"  To skip this prompt next time: set `write_auto_init_repo: true` in codrax.yaml, or pass --auto-init-repo to one-shot CLI runs.",
	}
}

// autoInitProceeding — printed right before EnsureInitialCommit fires
// so the operator sees the state mutation as a deliberate step.
func autoInitProceeding(lang, repoRoot string) string {
	if isZh(lang) {
		return formatN(lang, "正在初始化 git repo: %s ...", repoRoot)
	}
	return formatN(lang, "initializing git repo: %s ...", repoRoot)
}

// mergeNothingToDo — printed when /merge runs against a worktree
// that hasn't produced any commits beyond the base. This usually
// means /merge fired before /approve, or /approve produced an empty
// plan.
func mergeNothingToDo(lang, baseBranch string) string {
	if isZh(lang) {
		return formatN(lang, "  没有可合并的 commit:worktree HEAD 和 %s 一致", baseBranch)
	}
	return formatN(lang, "  nothing to merge: worktree HEAD is at %s tip", baseBranch)
}

// mergeNoApplyYet — /merge ran without a preserved worktree from a
// successful /approve. Tell the user the prerequisite chain.
func mergeNoApplyYet(lang string) []string {
	if isZh(lang) {
		return []string{
			"没有可合并的 worktree。/merge 需要一次成功的 /approve 留下的 worktree。",
			"  先 /mode plan 生成 plan,/approve 落地(确保 codrax.yaml 里 pipeline_keep_worktree_on_success: true),再 /merge。",
		}
	}
	return []string{
		"no worktree to merge from. /merge consumes a worktree preserved by a successful /approve.",
		"  Run /mode plan, then /approve (with pipeline_keep_worktree_on_success: true), then /merge.",
	}
}

// mergeConfirmTitle — interactive y/N before any git command runs.
func mergeConfirmTitle(lang, strategy, target string, count int) string {
	if isZh(lang) {
		switch strategy {
		case "fast_forward":
			return formatN(lang, "把 %d 个 commit fast-forward 到主仓 %s 分支?", count, target)
		default:
			return formatN(lang, "在主仓上拉新分支 %s 并 cherry-pick %d 个 commit?", target, count)
		}
	}
	switch strategy {
	case "fast_forward":
		return formatN(lang, "Fast-forward %d commit(s) onto main repo branch %s?", count, target)
	default:
		return formatN(lang, "Create branch %s on main repo and cherry-pick %d commit(s) onto it?", target, count)
	}
}

// mergeSuccess — printed after a clean MergeIntoBranch return.
func mergeSuccess(lang, strategy, finalBranch string, count int) []string {
	if isZh(lang) {
		switch strategy {
		case "fast_forward":
			return []string{
				formatN(lang, "  ✓ 已 fast-forward %d 个 commit 到 %s。", count, finalBranch),
				"  下一步:git push(可选)。",
			}
		default:
			return []string{
				formatN(lang, "  ✓ 已在主仓创建分支 %s,cherry-pick %d 个 commit。", finalBranch, count),
				formatN(lang, "  下一步:cd <主仓> && git push -u origin %s,然后开 PR。", finalBranch),
			}
		}
	}
	switch strategy {
	case "fast_forward":
		return []string{
			formatN(lang, "  ✓ Fast-forwarded %d commit(s) onto %s.", count, finalBranch),
			"  Next: git push (optional).",
		}
	default:
		return []string{
			formatN(lang, "  ✓ Branch %s created on main repo with %d cherry-picked commit(s).", finalBranch, count),
			formatN(lang, "  Next: cd <main repo> && git push -u origin %s, then open a PR.", finalBranch),
		}
	}
}

// otherPendingPlansHint — printed before the /approve confirm
// prompt when PlanStore has more than one re-approvable plan
// (pending_approval or verify_failed). Surfaces the count + how
// to target a specific one so the user doesn't accidentally
// approve the auto-resolved most-recent.
func otherPendingPlansHint(lang, planID string, others int) string {
	if isZh(lang) {
		return formatN(lang,
			"  注意:还有 %d 个其它可批准的 plan(pending_approval / verify_failed)。当前要批的是 %s;指定其它用 `/approve <plan-id>`(/plan list 看 ID)",
			others, planID)
	}
	return formatN(lang,
		"  note: %d other approvable plan(s) exist (pending_approval / verify_failed). about to approve %s; target a different one with `/approve <plan-id>` (see /plan list)",
		others, planID)
}

// skipVerifyAcknowledged — printed when /approve --skip-verify is
// honored. Tells the user the verify stage is being deliberately
// skipped so they don't expect a "tests passed" verdict.
func skipVerifyAcknowledged(lang string) string {
	if isZh(lang) {
		return "  --skip-verify 已生效:本次 approve 跳过 verify 阶段(只 apply,不跑测试)"
	}
	return "  --skip-verify acknowledged: this approve skips the verify stage (apply only, no tests)"
}

// mergeForceFailedWarning — printed when the user passes
// `/merge --include-failed` (or --force) and the resolved plan's
// Status is verify_failed. We surface a deliberate warning so the
// operator's eye registers that they're overriding the safety
// gate before any git command runs.
func mergeForceFailedWarning(lang, planID string) []string {
	if isZh(lang) {
		return []string{
			formatN(lang, "  ⚠ 强制合入 plan %s — 该 plan 的 verify 阶段曾失败。", planID),
			"  请先确认 /plan show 的 diff 与失败摘要,确保失败是环境/CI 类原因(非代码缺陷)。",
		}
	}
	return []string{
		formatN(lang, "  ⚠ Force-merging plan %s — its verify stage previously failed.", planID),
		"  Confirm /plan show diff + failure summary; only proceed if the failure is environmental (CI/infra), not a code defect.",
	}
}

// mergeFailure — printed when MergeIntoBranch returned an error.
// gitDiag is the raw git diagnostic from the helper; the second
// line tells the user the helper rolled back so the main repo is
// in its prior state.
func mergeFailure(lang, gitDiag string) []string {
	if isZh(lang) {
		return []string{
			formatN(lang, "  ✗ 合并失败:%s", oneLine(gitDiag)),
			"  主仓已回滚到合并前的状态。可以 /worktree show 检查冲突文件,或 /reject 弃掉 plan 重新规划。",
		}
	}
	return []string{
		formatN(lang, "  ✗ Merge failed: %s", oneLine(gitDiag)),
		"  Main repo restored to prior state. /worktree show to inspect, or /reject to discard the plan.",
	}
}

// recoveredPendingPlan — printed when /plan show found pendingPlanPath
// empty but PlanStore.List had a Status=pending_approval entry. The
// REPL recovers the pointer transparently so a previous failed
// /approve doesn't make the plan invisible.
func recoveredPendingPlan(lang, planID string) string {
	if isZh(lang) {
		return formatN(lang, "  从 PlanStore 恢复待审批 plan: %s", planID)
	}
	return formatN(lang, "  recovered pending plan from PlanStore: %s", planID)
}

// noPendingPlanReject — same as noPendingPlan but for /reject.
func noPendingPlanReject(lang string) string {
	if isZh(lang) {
		return "没有待拒绝的 plan"
	}
	return "no pending plan to reject"
}

// modeSwitched — info line when user runs /mode <X>.
func modeSwitched(lang, mode string) string {
	if isZh(lang) {
		return formatN(lang, "已切换到 %s 模式", mode)
	}
	return formatN(lang, "switched to %s mode", mode)
}

// modeWorkflowHint returns a 1-3 line hint explaining what the newly-
// entered mode actually does, returned as a slice the caller emits via
// r.info one line at a time. Surfaced ONCE per /mode transition so a
// user new to write mode knows the plan→approve→read loop without
// having to read the docs. Empty slice for ModeRead (no special
// workflow to explain).
func modeWorkflowHint(lang, mode string) []string {
	zh := isZh(lang)
	switch mode {
	case "plan":
		if zh {
			return []string{
				"  接下来你的下一条请求会产生 ChangePlan(改动提议),不是直接回答。",
				"  生成后用 /plan show 审阅、/approve 落地、/reject 丢弃、/mode read 回读模式继续提问。",
			}
		}
		return []string{
			"  Your next request will produce a ChangePlan instead of an answer.",
			"  After that: /plan show to review, /approve to apply, /reject to discard, /mode read to keep questioning.",
		}
	case "apply":
		if zh {
			return []string{
				"  apply 模式直接执行已批准的 plan。一般通过 /approve 进入,而不是手动 /mode apply。",
				"  如果你只想审 plan 再决定,先 /mode plan 生成,再 /approve。",
			}
		}
		return []string{
			"  Apply mode runs an approved plan. Most users reach this via /approve, not /mode apply directly.",
			"  To review before applying: /mode plan first, then /approve.",
		}
	case "verify":
		if zh {
			return []string{
				"  verify 模式只重跑测试,不再生成或 apply。需要已 apply 的 plan(/history 找)。",
			}
		}
		return []string{
			"  Verify mode reruns tests against an already-applied plan (find one in /history).",
		}
	}
	return nil
}

// planReadyNudge prints next-step actions after the orchestrator
// emitted a ChangePlan during plan-mode dispatch and the REPL
// auto-saved it. Without this nudge the user sees "plan saved: <path>"
// and has to remember the slash-command vocabulary; with it the path
// to /approve / /reject / /mode read is one line away.
func planReadyNudge(lang string, planID string, changeCount int) []string {
	if isZh(lang) {
		return []string{
			formatN(lang, "Plan 已就绪: %s (%d 处改动)。下一步:", planID, changeCount),
			"    /plan show   — 查看每个文件的改动 diff",
			"    /approve     — 在 worktree 内 apply + 跑 verify",
			"    /reject      — 丢弃本 plan",
			"    /mode read   — 切回读模式继续问代码问题(plan 仍保留可后续 /approve)",
		}
	}
	return []string{
		formatN(lang, "Plan ready: %s (%d change(s)). Next:", planID, changeCount),
		"    /plan show   — inspect the proposed diff per file",
		"    /approve     — apply inside a worktree + run verify",
		"    /reject      — discard this plan",
		"    /mode read   — return to read mode (plan stays saved for later /approve)",
	}
}

// applyDoneNudge prints next-step actions after /approve completed.
// When apply succeeded, point at /mode read for further questions and
// /mode plan for a new change. When apply failed, the
// approveFailedNudge already covers recovery — applyDoneNudge skips
// the failure path so we don't double-print.
func applyDoneNudge(lang string) []string {
	if isZh(lang) {
		return []string{
			"  apply 完成。下一步:",
			"    /mode read   — 切回读模式问代码问题",
			"    /mode plan   — 生成下一个改动的 plan",
		}
	}
	return []string{
		"  apply complete. Next:",
		"    /mode read   — return to read mode for questions",
		"    /mode plan   — generate another change",
	}
}

// planShowFooter prints the next-step actions at the bottom of /plan
// show output so the user does not have to remember the slash command
// vocabulary after reviewing the diff.
func planShowFooter(lang string) []string {
	if isZh(lang) {
		return []string{
			"  下一步: /approve 落地 · /reject 丢弃 · /plan clear 仅删本地副本",
		}
	}
	return []string{
		"  next: /approve to apply · /reject to discard · /plan clear to delete the local copy",
	}
}

// friendlyRunError translates a few well-known Run errors into a more
// actionable user-facing form. "context canceled" is the canonical
// case — it surfaces when the user hits Ctrl+C mid-LLM-call but the
// raw text gives no recovery hint. Returns the original message when
// no friendly mapping applies.
func friendlyRunError(lang string, err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	if strings.Contains(low, "context canceled") || strings.Contains(low, "context cancelled") {
		if isZh(lang) {
			return "请求被中断(可能是 Ctrl+C 或上游连接关闭)。再试一次或检查网络。"
		}
		return "request interrupted (likely Ctrl+C or upstream connection closed). Retry or check the network."
	}
	if strings.Contains(low, "deadline exceeded") {
		if isZh(lang) {
			return formatN(lang, "请求超时:%s", msg)
		}
		return formatN(lang, "request timed out: %s", msg)
	}
	return msg
}

// verifyDispatching — short status line printed by /verify before
// the orchestrator spinner takes over.
func verifyDispatching(lang, planID string) string {
	if isZh(lang) {
		return formatN(lang, "重跑 verify: plan=%s (在保留的 worktree 内)", planID)
	}
	return formatN(lang, "rerunning verify: plan=%s (against preserved worktree)", planID)
}

// formatN is a thin alias over fmt.Sprintf so the bilingual helpers
// stay readable and a future static analyzer can grep for "format
// string in messages.go" by symbol name.
func formatN(_ string, format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

// promptStickyTag returns a compact bracketed marker the prompt
// renderer prepends so the user sees at a glance which sticky
// attachments are live for this turn:
//
//	[mode:plan]                 — sticky write-mode (non-read)
//	[log]                       — AttachedLog non-empty
//	[trace]                     — AttachedHitrace non-empty
//	[plan]                      — pendingPlanPath non-empty
//	[mem!]                      — memory under pressure (cleanup nudge)
//	[git:<branch>]              — current git branch in repoRoot
//	[git:detached@<sha7>]       — detached HEAD (rebase / cherry-pick
//	                              / explicit checkout of a SHA)
//
// Multiple markers concatenate without spaces:
// "[git:main][mode:plan][plan] ❯❯". Empty when nothing sticky.
//
// All markers are language-agnostic (zh and en both use the same
// bracketed labels) — they're terminal chrome, optimised for
// scan-ability, not for translation. The full localized hint when
// the user types /help or hits a memory-pressure threshold goes
// through the normal localized helpers.
//
// branch is the resolved git HEAD (from gitBranchProbe). Empty
// when the path is not a git repo or git is missing — the marker
// is dropped entirely, since absence is unambiguous.
func promptStickyTag(mode, branch string, hasLog, hasTrace, hasPendingPlan, memPressure bool) string {
	var b strings.Builder
	if branch != "" {
		b.WriteString("[git:")
		b.WriteString(branch)
		b.WriteString("]")
	}
	if mode != "" && !strings.EqualFold(mode, "read") {
		b.WriteString("[mode:")
		b.WriteString(mode)
		b.WriteString("]")
	}
	if hasLog {
		b.WriteString("[log]")
	}
	if hasTrace {
		b.WriteString("[trace]")
	}
	if hasPendingPlan {
		b.WriteString("[plan]")
	}
	if memPressure {
		b.WriteString("[mem!]")
	}
	return b.String()
}

// helpLines auto-generates the /help command's bilingual output from
// the canonical slashCommands table. Returns one line per command
// followed by a generic multi-line-input tip. Drift-proof: a new
// command added to slashCommands shows up here automatically.
//
// Format per line: "  <name>  <padding>  <help>" with name padded to
// the longest command width so the help columns line up. Header is
// localized; per-command help text comes from slashCommand.Help(lang).
func helpLines(lang string) []string {
	// Two-column alignment: the parent-command and subcommand
	// columns share a width budget so all rows line up. Width is
	// the longest name across BOTH levels (parent + sub) since
	// subs render with a 2-space indent prefix and need to fit
	// inside the same column.
	maxName := 0
	for _, c := range slashCommands {
		if n := len(c.Name); n > maxName {
			maxName = n
		}
		for _, s := range c.Subs {
			// The sub label rendered as `<parent> <sub>` (the
			// shape the user types). That shape is wider than a
			// bare sub name, so include the parent prefix when
			// computing width.
			candidate := c.Name + " " + s.Name
			if n := len(candidate); n > maxName {
				maxName = n
			}
		}
	}
	pad := func(s string) string {
		if len(s) >= maxName {
			return s
		}
		return s + strings.Repeat(" ", maxName-len(s))
	}
	out := make([]string, 0, len(slashCommands)+2)
	if isZh(lang) {
		out = append(out, "可用命令(共 "+itoa(len(slashCommands))+" 条;子命令缩进显示):")
	} else {
		out = append(out, "available commands ("+itoa(len(slashCommands))+"; subcommands shown indented):")
	}
	for _, c := range slashCommands {
		out = append(out, "  "+pad(c.Name)+"  "+c.Help(lang))
		// Render subs as `<parent> <sub>` so the user sees the
		// exact shape they would type. Indented by 4 spaces so
		// the visual hierarchy is unambiguous.
		for _, s := range c.Subs {
			label := c.Name + " " + s.Name
			out = append(out, "    "+pad(label)+"  "+s.Help(lang))
		}
	}
	if isZh(lang) {
		out = append(out, "提示:行尾加 \\ 进入多行输入。")
	} else {
		out = append(out, "tip: end a line with \\ for multi-line input.")
	}
	return out
}

// itoa is a tiny stdlib-free int-to-string helper; messages.go
// avoids importing strconv to keep the dependency surface minimal.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// memoryPressureHint returns a one-line nudge surfaced when the
// memory store crosses a soft pressure threshold (e.g. recent-turns
// buffer full / index entries past N). zh-default; pairs with the
// [mem!] tag in promptStickyTag to make the recovery action
// obvious. recentBytes / indexCount are folded into the user-facing
// number so the nudge is concrete.
func memoryPressureHint(lang string, recentTurns, indexEntries int) string {
	if isZh(lang) {
		return formatN(lang,
			"memory 已积累 %d 个最近回合 + %d 条索引条目,可能影响检索质量。建议 /compact 或 /clear",
			recentTurns, indexEntries)
	}
	return formatN(lang,
		"memory has accumulated %d recent turns + %d index entries — retrieval quality may degrade. Run /compact or /clear",
		recentTurns, indexEntries)
}
