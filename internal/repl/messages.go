package repl

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pterm/pterm"

	"github.com/hanchaoqun/codrax/internal/llm"
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
// skipVerify changes the trailing clause: when true the title says
// "apply only" so the operator's eye registers that they're about
// to land bytes WITHOUT running tests (matching what
// --skip-verify will actually do). A pre-fix bug always said
// "apply + run verify" even when the flag was on.
func approveTitlePrompt(lang, planID string, changeCount int, skipVerify bool) string {
	if isZh(lang) {
		if skipVerify {
			return formatN(lang,
				"是否批准 plan %s (%d 处改动)?将在 git worktree 中只 apply,跳过 verify(--skip-verify 已生效)。",
				planID, changeCount)
		}
		return formatN(lang,
			"是否批准 plan %s (%d 处改动)?将在 git worktree 中 apply + 跑 verify。",
			planID, changeCount)
	}
	if skipVerify {
		return formatN(lang,
			"Approve plan %s (%d change(s))? Apply inside a git worktree (skip verify — --skip-verify is set).",
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
			formatN(lang, "%s 已被拒绝: write_enabled 为 false (或未设置)", mode),
			formatN(lang, "  在 %s 中设置 `write_enabled: true` 并重启即可启用 plan / apply / verify 模式。", target),
			"  read 模式(默认)无需额外配置。",
		}
		if created != "" {
			out = append(out, created)
		}
		return out
	}
	out := []string{
		formatN(lang, "%s rejected: write_enabled is false (or unset)", mode),
		formatN(lang, "  Set `write_enabled: true` in %s and restart to enable plan / apply / verify modes.", target),
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
// clampToTermWidth caps a banner line to fit comfortably on
// the operator's terminal. Pre-commit-42 long settings paths
// or long pitfall counts could push a banner row past 80
// columns and wrap onto a second visual line, breaking the
// vertical alignment with the other banner rows.
//
// Strategy: trim trailing whitespace, then if the rune count
// exceeds maxWidth, drop the middle and replace with "…".
// Keeps the start (most informative) and the end (action /
// command names) visible. maxWidth ≤ 0 falls back to 120 — a
// sensible upper bound for every reasonable terminal that
// still keeps tail commands visible.
func clampToTermWidth(s string, maxWidth int) string {
	s = strings.TrimRight(s, " \t")
	if maxWidth <= 0 {
		maxWidth = 120
	}
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	// Reserve 1 rune for the ellipsis. Split the budget so
	// the start gets ~60% and the end gets ~40% — head usually
	// carries the noun (plan id, count); tail carries the
	// commands.
	keep := maxWidth - 1
	headLen := keep * 6 / 10
	tailLen := keep - headLen
	if headLen <= 0 || tailLen <= 0 {
		return string(runes[:maxWidth])
	}
	return string(runes[:headLen]) + "…" + string(runes[len(runes)-tailLen:])
}

// failureTaxonomyBannerLine surfaces the count of learned
// pitfalls + the inspection command at REPL startup so users
// who never read the full /help discover the feature exists.
// Empty count is filtered at the call site (no banner row);
// non-zero shows count + /pitfalls list path.
func failureTaxonomyBannerLine(lang string, count int) string {
	if isZh(lang) {
		return formatN(lang, "📌 本仓库已积累 %d 条 pitfall (planner 未来 plan 时会读取);用 /pitfalls list 查看", count)
	}
	return formatN(lang, "📌 %d learned pitfall(s) recorded for this repo (planner reads on future plans); use /pitfalls list to inspect", count)
}

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
// analysis, no plan), not from the main pipeline.
//
// Style matches localReplyHeader / turnPolicyClarifyMessage: dim
// FgDarkGray, single line, two-space indent — recedes visually
// so the bordered answer below dominates. Customer feedback on
// the prior shape was that it "抢眼" / dominated attention.
func chitchatReplyHeader(lang string) string {
	var detail string
	if isZh(lang) {
		detail = "chat · 闲聊回复,未读仓库,未生成 plan"
	} else {
		detail = "chat · chitchat reply, no repo read, no plan"
	}
	return "  " + pterm.FgDarkGray.Sprint(detail)
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
			"  在 codrax.yaml 中设置 `write_enabled: true` 并重启,然后 /mode plan 生成 plan。",
		}
	}
	return []string{
		"no pending plan, and write mode is disabled.",
		"  Set `write_enabled: true` in codrax.yaml and restart, then /mode plan to generate one.",
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
			"目标目录 %s 状态:%s。可以自动 `git init` + 空 initial commit,然后在沙箱 worktree 里 apply。是否同意?",
			repoRoot, stateLabel)
	}
	return formatN(lang,
		"target %s is %s. I can run `git init` + an empty initial commit, then apply inside a sandbox worktree. Proceed?",
		repoRoot, stateLabel)
}

// autoInitDeclined — printed when the user answered No to the consent
// prompt. Tells them how to switch on yaml/CLI pre-authorization
// instead so they don't have to answer y every time.
func autoInitDeclined(lang string) []string {
	if isZh(lang) {
		return []string{
			"已取消。改动方案仍保留 (/plan show 可看)。",
			"  下次想直接同意:在 codrax.yaml 设 write_auto_init_repo: true,或运行时加 --auto-init-repo。",
		}
	}
	return []string{
		"cancelled. The plan is still saved (/plan show to inspect).",
		"  To skip this prompt next time: set `write_auto_init_repo: true` in codrax.yaml, or pass --auto-init-repo at run time.",
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

// unsettledModePlanReject is the message /mode plan prints when an
// unsettled plan blocks switching into plan mode. Three-way menu
// is tailored to the offending plan's status: only commands that
// move THIS status forward to a terminal state (merged / rejected)
// are listed. Pre-2026-04-30 the 用户 had to know the relationship
// between pending / applied / verify_failed and which command
// applies; surfacing the right menu directly removes the friction.
//
// Returns a slice for the caller to emit one r.info() per line so
// the FgDarkGray styling applies consistently.
func unsettledModePlanReject(lang, planID, status string) []string {
	zh := isZh(lang)
	var menu []string
	switch status {
	case "pending_approval":
		if zh {
			menu = []string{
				"  /plan show     看 diff",
				"  /approve       批准并落地",
				"  /reject        丢弃改动(保留事后审查记录)",
				"  /plan clear    彻底删除(无审查记录)",
			}
		} else {
			menu = []string{
				"  /plan show     inspect diff",
				"  /approve       apply it",
				"  /reject        discard (keep audit record)",
				"  /plan clear    delete outright (no audit)",
			}
		}
	case "applied":
		if zh {
			menu = []string{
				"  /merge         合并到主仓",
				"  /reject        丢弃改动(保留事后审查记录)",
				"  /plan clear    彻底删除(无审查记录)",
			}
		} else {
			menu = []string{
				"  /merge         fold back into the main repo",
				"  /reject        discard (keep audit record)",
				"  /plan clear    delete outright (no audit)",
			}
		}
	case "verify_failed":
		if zh {
			menu = []string{
				"  /approve <id>              重新跑 apply + verify",
				"  /merge --include-failed    review 后强行合并",
				"  /reject                    丢弃改动(保留事后审查记录)",
				"  /plan clear                彻底删除(无审查记录)",
			}
		} else {
			menu = []string{
				"  /approve <id>              re-run apply + verify",
				"  /merge --include-failed    force-merge after review",
				"  /reject                    discard (keep audit record)",
				"  /plan clear                delete outright (no audit)",
			}
		}
	default:
		// Should never happen — IsUnsettledStatus only matches the
		// three above. Defensive fallback.
		if zh {
			menu = []string{"  /plan list  看现有方案的状态,然后用 /reject 或 /plan clear 收尾"}
		} else {
			menu = []string{"  /plan list  to inspect, then /reject or /plan clear to settle"}
		}
	}
	if zh {
		header := []string{
			formatN(lang, "✗ 切换被拒:已存在未结算的改动方案 %s(状态:%s)。", planID, status),
			"  新方案要基于当前仓状态生成,先把上一个收尾再来:",
		}
		out := append(header, menu...)
		out = append(out, "  收尾后再敲 /mode plan。")
		return out
	}
	header := []string{
		formatN(lang, "✗ /mode plan refused: an unsettled plan exists (%s, status=%s).", planID, status),
		"  Settle it first so the next plan can be drafted against the current repo state:",
	}
	out := append(header, menu...)
	out = append(out, "  Then re-run /mode plan.")
	return out
}

// unsettledBanner is the dim FgDarkGray one-liner printed in the
// REPL startup banner when PlanStore has an unsettled plan from a
// prior session. Wording is status-specific so the user immediately
// knows which command moves it forward.
//
// worktreeMissing (commit 42 P1): when the plan's persisted
// WorktreePath was deleted out-of-band (e.g., rm -rf), the
// banner appends "(worktree gone — use /reject)" so the user
// doesn't try /merge / /verify and hit a runtime error.
func unsettledBanner(lang, planID, status string, worktreeMissing bool) string {
	zh := isZh(lang)
	suffix := ""
	if worktreeMissing {
		if zh {
			suffix = "  ⚠ worktree 已不在 — /reject 清理"
		} else {
			suffix = "  ⚠ worktree gone — /reject to clear"
		}
	}
	switch status {
	case "pending_approval":
		if zh {
			return formatN(lang, "%s 待批准 — /plan show · /approve · /reject · /plan clear%s", planID, suffix)
		}
		return formatN(lang, "%s pending approval — /plan show · /approve · /reject · /plan clear%s", planID, suffix)
	case "applied":
		if zh {
			return formatN(lang, "%s 已 apply 但未合并 — /merge · /reject · /plan clear%s", planID, suffix)
		}
		return formatN(lang, "%s applied, not merged — /merge · /reject · /plan clear%s", planID, suffix)
	case "verify_failed":
		if zh {
			return formatN(lang, "%s 验证失败 — /approve <id> · /merge --include-failed · /reject · /plan clear%s", planID, suffix)
		}
		return formatN(lang, "%s verify failed — /approve <id> · /merge --include-failed · /reject · /plan clear%s", planID, suffix)
	}
	if zh {
		return formatN(lang, "%s 状态未结算 — /plan list 查看 · /reject · /plan clear%s", planID, suffix)
	}
	return formatN(lang, "%s unsettled — /plan list · /reject · /plan clear%s", planID, suffix)
}

// autoModeReadAfterMergeNudge is the one-line confirmation printed
// right after /merge auto-switches back to read mode. Pre-fix the
// REPL stayed sticky in plan mode after a successful merge; the
// user's next "how do I X" question routed back to the planner
// (which then either generated an unwanted plan, or returned prose
// and surfaced the "did not call emit_change_plan" error). Auto-
// switching avoids both failure modes; this message tells the user
// the mode flipped so they don't get surprised.
func autoModeReadAfterMergeNudge(lang string) string {
	if isZh(lang) {
		return "  已自动切回 read 模式 — 直接提问就行。再 /mode plan 进入 plan 模式即可继续改代码。"
	}
	return "  Auto-switched back to read mode — ask your next question directly. /mode plan to make another change."
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
			"  注意:还有 %d 个其它待批准 / 验证失败的方案。当前要批的是 %s;批别的用 /approve <plan-id>(/plan list 看 ID)。",
			others, planID)
	}
	return formatN(lang,
		"  note: %d other pending or verify-failed plan(s) exist. About to approve %s; target a different one via /approve <plan-id> (/plan list shows ids).",
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
		return formatN(lang, "  恢复待审批改动方案: %s", planID)
	}
	return formatN(lang, "  recovered pending plan: %s", planID)
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

// modeWorkflowHint returns a 1-2 line nudge for a newly-entered
// mode. Concise, no internal terminology, only the slash commands
// the user actually needs next. Caller wraps with r.info so the
// pterm style is FgDarkGray (subdued).
func modeWorkflowHint(lang, mode string) []string {
	zh := isZh(lang)
	switch mode {
	case "plan":
		if zh {
			return []string{
				"  下一条请求会产生改动方案,不直接回答。",
				"  之后:/plan show 看 diff · /approve 落地 · /reject 丢弃 · /mode read 回读模式",
			}
		}
		return []string{
			"  Your next request produces a change proposal instead of an answer.",
			"  Then: /plan show · /approve · /reject · /mode read",
		}
	case "apply":
		if zh {
			return []string{
				"  通常用 /approve 进入 apply,而不是 /mode apply。",
			}
		}
		return []string{
			"  Reach apply via /approve; /mode apply directly is rarely what you want.",
		}
	case "verify":
		if zh {
			return []string{
				"  仅重跑已 apply plan 的测试。/history 查 plan id。",
			}
		}
		return []string{
			"  Reruns tests against an already-applied plan. /history lists plan ids.",
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
			formatN(lang, "改动方案已就绪: %s (%d 处改动)。", planID, changeCount),
			"  /plan show · /approve · /approve --skip-verify · /reject · /mode read",
		}
	}
	return []string{
		formatN(lang, "Change proposal ready: %s (%d change(s)).", planID, changeCount),
		"  /plan show · /approve · /approve --skip-verify · /reject · /mode read",
	}
}

// planReadyMultiPhaseNudge is the multi-phase variant printed
// AFTER planReadyNudge when the just-emitted proposal carries a
// PhaseProposal with N >= 2 sequential phases. Pre-commit-41 the
// operator only saw the single-plan nudge and had no signal that
// /approve was about to drive N phases — `/phase show` was
// undiscoverable until they ran it. This second line surfaces
// the multi-phase nature + points at the inspection tool.
func planReadyMultiPhaseNudge(lang string, phaseCount int) []string {
	if isZh(lang) {
		return []string{
			formatN(lang, "  ⚠ 多阶段方案 (%d 个 phase): /approve 后用 `/phase show` 追踪进度,`/phase rollback` 回退当前 phase。", phaseCount),
		}
	}
	return []string{
		formatN(lang, "  ⚠ Multi-phase proposal (%d phases): after /approve, use `/phase show` to track progress and `/phase rollback` to reset the active phase.", phaseCount),
	}
}

// applyDoneNudge prints next-step actions after /approve completed.
// When apply succeeded, point at /mode read for further questions and
// /mode plan for a new change. When apply failed, the
// renderVerifyFailure already carries the failure-path next-step inline — applyDoneNudge skips
// the failure path so we don't double-print.
func applyDoneNudge(lang string) []string {
	if isZh(lang) {
		return []string{
			"  apply 完成,已自动切回 read 模式。继续改代码用 /mode plan。",
		}
	}
	return []string{
		"  apply complete; auto-switched to read mode. /mode plan to make another change.",
	}
}

// planShowFooter prints the next-step actions at the bottom of /plan
// show output so the user does not have to remember the slash command
// vocabulary after reviewing the diff.
//
// Status-aware (UX#5, commit 41): pre-commit-41 the footer always
// printed the same generic line — but a plan in verify_failed /
// partially_applied / unverified state has DIFFERENT recovery
// commands, and surfacing them only at /approve-time meant operators
// who paused to inspect lost the cue. Now the footer adapts.
func planShowFooter(lang string, planStatus string) []string {
	zh := isZh(lang)
	switch planStatus {
	case "verify_failed":
		if zh {
			return []string{
				"  下一步: /approve --retry 重试 · /merge --include-failed 接受失败强合 · /reject 丢弃",
			}
		}
		return []string{
			"  next: /approve --retry to retry · /merge --include-failed to merge anyway · /reject to discard",
		}
	case "partially_applied":
		if zh {
			return []string{
				"  下一步: /approve --retry 续 apply 剩余 paths · /reject 丢弃 (注意:/merge 拒收 partially_applied)",
			}
		}
		return []string{
			"  next: /approve --retry to apply remaining paths · /reject to discard (note: /merge refuses partially_applied)",
		}
	case "unverified":
		if zh {
			return []string{
				"  下一步: /approve --retry 重跑 (加 tests 后) · /merge --include-failed 接受未验证强合 · /reject 丢弃",
			}
		}
		return []string{
			"  next: /approve --retry to re-run (after adding tests) · /merge --include-failed to land without verification · /reject to discard",
		}
	case "applied":
		if zh {
			return []string{
				"  下一步: /merge 合回主仓 · /verify 重跑测试 · /worktree list 查看保留的 worktree",
			}
		}
		return []string{
			"  next: /merge to fold into main · /verify to re-run tests · /worktree list to see preserved worktrees",
		}
	}
	// Default (pending_approval / unknown): the original footer.
	if zh {
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
	// User-driven cancel (Ctrl+C / `/cancel`) gets the polished
	// message with the most-specific stage label. Match by the typed
	// CanceledError shape so HTTP-level "context canceled" (which
	// surfaces a different shape) doesn't masquerade as a deliberate
	// user cancel.
	var ce cancelErrorCarrier
	if errors.As(err, &ce) {
		return canceledByUserMsg(lang, ce.Stage())
	}
	// Streaming-watchdog first-byte timeout: handshake completed
	// (provider returned 200 OK) but never emitted any SSE chunk
	// within streamFirstByteTimeout (default 20s). Match BEFORE
	// the StreamStalledError branch because the typed error chain
	// preserves both signatures and we want the more specific
	// "never started" prose to surface — different operator
	// remediation than mid-stream stall.
	var fb *llm.StreamFirstByteTimeoutError
	if errors.As(err, &fb) {
		idle := fb.IdleFor
		if isZh(lang) {
			return fmt.Sprintf("上游 LLM 在请求被接受 %s 后仍未返回任何 SSE 字节。可能是 provider 服务侧死锁、网络中间设备劫持、或模型 cold-start 卡死。再试一次,或换 provider/model 看看。", idle)
		}
		return fmt.Sprintf("upstream LLM produced no SSE bytes within %s of the request being accepted. Likely causes: provider-side deadlock, middlebox interference, or cold-start hang. Retry, or try a different provider/model.", idle)
	}
	// Streaming-watchdog mid-stream stall: stream started but went
	// silent for more than streamStallTimeout (default 60s).
	var ss *llm.StreamStalledError
	if errors.As(err, &ss) {
		idle := ss.IdleFor
		if isZh(lang) {
			return fmt.Sprintf("上游 LLM 流式响应停滞 %s 无新字节,已自动中止。原因可能是模型 mid-emit 卡住、上游网络抖动、或 thinking 块过大。再试一次,或换一个 provider/model 看看。", idle)
		}
		return fmt.Sprintf("upstream LLM stream stalled with no bytes for %s; aborted automatically. Likely causes: model stuck mid-emit, upstream network blip, or oversized thinking block. Retry, or try a different provider/model.", idle)
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

// cancelErrorCarrier is the messages-package boundary type for the
// orchestrator's CanceledError. Defined here as a tiny interface so
// internal/repl/messages.go does not depend on internal/orchestrator
// (REPL → orchestrator depends on the inverse direction; circular
// import is avoided). orchestrator.CanceledError implements Stage()
// out of the box.
type cancelErrorCarrier interface {
	error
	Stage() string
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
	out := make([]string, 0, len(slashCommands)+4)
	if isZh(lang) {
		out = append(out, "可用命令(共 "+itoa(len(slashCommands))+" 条;子命令缩进显示):")
	} else {
		out = append(out, "available commands ("+itoa(len(slashCommands))+"; subcommands shown indented):")
	}
	// UX#3 (commit 41): group write-mode commands under a
	// single header so first-time users see them as a coherent
	// workflow instead of scattered through the read-mode
	// commands. The split point is the first "isWriteCommand"
	// entry; we emit the header once just before it. Other
	// commands stay in their original order.
	writeHeaderEmitted := false
	emitWriteHeader := func() {
		if writeHeaderEmitted {
			return
		}
		if isZh(lang) {
			out = append(out, "  ── 写模式命令(需 codrax.yaml :: write_enabled: true) ──")
		} else {
			out = append(out, "  ── Write-mode commands (require codrax.yaml :: write_enabled: true) ──")
		}
		writeHeaderEmitted = true
	}
	for _, c := range slashCommands {
		if isWriteModeCommand(c.Name) {
			emitWriteHeader()
		}
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
		out = append(out, "提示:行尾加 \\ 进入多行输入;以 ! 开头执行系统 shell 命令,工作目录是仓根。")
	} else {
		out = append(out, "tip: end a line with \\ for multi-line input; prefix a line with ! to run a system shell command, cwd = repo root.")
	}
	return out
}

// isWriteModeCommand classifies a slashCommand name as
// belonging to the write-mode workflow. Used by helpLines
// (UX#3) to insert a single grouping header before the
// first write command. Centralised here so adding a new
// write command — or moving one — only needs an update in
// this list rather than re-flowing the help renderer.
func isWriteModeCommand(name string) bool {
	switch name {
	case "/mode",
		"/plan",
		"/approve",
		"/reject",
		"/verify",
		"/worktree",
		"/merge",
		"/branch",
		"/baseline",
		"/phase",
		"/pitfalls":
		return true
	}
	return false
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

// ── Bilingual helpers for short user-facing REPL messages.
//
// These cover the long tail of warn/info/errorf strings the REPL
// emits during command handling. zh-default per the project
// convention (BusContext.Language; empty/zh/anything-not-en → zh,
// "en" → English). Each helper is a single function so the call
// site stays a one-liner and adding new languages later (ja, fr,
// etc.) is a single-file edit.
//
// Naming convention: <surface>Msg(...) returning string for
// single-line messages; ...Lines for multi-line.

// shellBangCdNonPersistent — printed when the user types
// `!cd <dir>`, `!pushd <dir>`, or `!popd`. The bare form has no
// effect because each `!` invocation is a fresh shell. We still
// pass the line through (so chained shapes like `!cd /tmp && cat foo`
// work — the && side-effects stay in one shell process), but
// surface a warning so the operator learns the shape.
func shellBangCdNonPersistent(lang, builtin string) string {
	if isZh(lang) {
		return formatN(lang,
			"`%s` 在 `!` 内不会持久 —— 每个 `!` 调用都新起一个 shell。"+
				"用 --repo /new/path 重启,或链式写 `!cd /tmp && cat foo.txt` "+
				"(同一个 shell 内执行)。\n",
			builtin)
	}
	return formatN(lang,
		"`%s` inside `!` doesn't persist — every `!` invocation spawns a fresh shell. "+
			"Restart with --repo /new/path, or chain in one command: "+
			"`!cd /tmp && cat foo.txt`.\n",
		builtin)
}

// shellBangEmpty — `!` with no command after it.
func shellBangEmpty(lang string) string {
	if isZh(lang) {
		return "(空 `!`,请在感叹号后输入命令)"
	}
	return "(empty `!` — type a command after the bang)"
}

// shellBangExit — non-zero exit reported back.
func shellBangExit(lang string, err error) string {
	if isZh(lang) {
		return formatN(lang, "! 退出码 %v\n", err)
	}
	return formatN(lang, "! exit %v\n", err)
}

// commandDisabled — "/X disabled (no PlanStore configured)" /
// similar. Used by /plan, /approve, /reject, /merge when the
// PlanStore wiring is missing (test stubs typically).
func commandDisabled(lang, cmd, reason string) string {
	if isZh(lang) {
		return formatN(lang, "%s 已禁用 (%s)", cmd, reason)
	}
	return formatN(lang, "%s disabled (%s)", cmd, reason)
}

// noPlanStoreReason — common reason embedded in commandDisabled.
func noPlanStoreReason(lang string) string {
	if isZh(lang) {
		return "改动方案存储未配置"
	}
	return "plan store unavailable"
}

// unknownSlashCommand — printed when handleSlash falls through.
func unknownSlashCommand(lang, cmd string) string {
	if isZh(lang) {
		return formatN(lang, "未知命令 %q —— 输入 /help 看完整列表\n", cmd)
	}
	return formatN(lang, "unknown command %q — try /help\n", cmd)
}

// unknownPlanSubcommand — /plan <unknown>.
func unknownPlanSubcommand(lang, sub string) string {
	if isZh(lang) {
		return formatN(lang, "未知的 /plan 子命令 %q —— 应是 show / clear / list 之一\n", sub)
	}
	return formatN(lang, "unknown /plan subcommand %q — expected: show, clear, list\n", sub)
}

// unknownModeValue — /mode <unknown>.
func unknownModeValue(lang, val string) string {
	if isZh(lang) {
		return formatN(lang, "未知模式 %q —— 应是 read / plan / apply / verify 之一\n", val)
	}
	return formatN(lang, "unknown mode %q — expected one of: read, plan, apply, verify\n", val)
}

// planNotFound — /approve <id> / /plan show <id> with non-existent ID.
func planNotFound(lang, planID string) string {
	if isZh(lang) {
		return formatN(lang, "在 PlanStore 里找不到 plan %q (用 /plan list 看 ID)\n", planID)
	}
	return formatN(lang, "plan %q not found in PlanStore (try /plan list)\n", planID)
}

// noPendingPlanToClear — /plan clear with empty pendingPlanPath.
func noPendingPlanToClear(lang string) string {
	if isZh(lang) {
		return "没有待清除的 plan"
	}
	return "no pending plan to clear"
}

// noPlansInStore — /plan list on an empty PlanStore.
func noPlansInStore(lang, dir string) string {
	if isZh(lang) {
		return formatN(lang, "%s 里还没有 plan", dir)
	}
	return formatN(lang, "no plans saved in %s", dir)
}

// approveRefusedStatusMsg — re-approving a non-pending /
// non-verify_failed plan.
func approveRefusedStatusMsg(lang, planID, status, pending, retry string) string {
	if isZh(lang) {
		return formatN(lang,
			"approve 被拒:plan %s 当前状态为 %q。可重新 approve 的状态:%q (新生成的 plan)、%q (修复环境后重试)。"+
				"用 /mode plan 生成新 plan。\n",
			planID, status, pending, retry)
	}
	return formatN(lang,
		"approve refused: plan %s is in status %q. "+
			"Re-approvable statuses: %q (fresh plan), %q (env-fix retry). "+
			"Run /mode plan to generate a fresh plan.\n",
		planID, status, pending, retry)
}

// approveStubRunnerMsg — runner doesn't implement modeSetter /
// planPathSetter. Hits in tests; surfaces fail-loud in real REPL.
func approveStubRunnerMsg(lang string) string {
	if isZh(lang) {
		return "/approve 需要 runner 支持 SetMode + SetPlanPath (检测到 stub runner)\n"
	}
	return "/approve requires a runner with SetMode + SetPlanPath (stub runner detected)\n"
}

// approveSkipVerifyStubMsg — runner doesn't implement
// skipVerifySetter (test stubs). Operator sees this only in
// scripted-test scenarios.
func approveSkipVerifyStubMsg(lang string) string {
	if isZh(lang) {
		return "--skip-verify 已请求但 runner 未实现 SetSkipVerify (test stub?);忽略\n"
	}
	return "--skip-verify requested but runner does not implement SetSkipVerify (test stub?); ignoring\n"
}

// approveBareDirNoAuthMsg — non-interactive REPL or scripted run
// with no --auto-init-repo when the target needs init.
func approveBareDirNoAuthMsg(lang, repoRoot, state string) string {
	if isZh(lang) {
		return formatN(lang,
			"approve:目标 %s 状态为 %s —— 用 --auto-init-repo 重跑或在 codrax.yaml 设 write_auto_init_repo: true\n",
			repoRoot, state)
	}
	return formatN(lang,
		"approve: target %s is %s — re-run with --auto-init-repo or set codrax.yaml :: write_auto_init_repo: true\n",
		repoRoot, state)
}

// mergeToIgnoredNoWorktreeMsg — --merge-to= without keep-on-success.
func mergeToIgnoredNoWorktreeMsg(lang, branch string) string {
	if isZh(lang) {
		return formatN(lang,
			"--merge-to=%s 已忽略:没有保留的 worktree (在 codrax.yaml 设 pipeline_keep_worktree_on_success: true 即可)\n",
			branch)
	}
	return formatN(lang,
		"--merge-to=%s ignored: no worktree was preserved (set codrax.yaml :: pipeline_keep_worktree_on_success: true)\n",
		branch)
}

// mergeListPlansFailedMsg — PlanStore.List error in /merge.
func mergeListPlansFailedMsg(lang string, err error) string {
	if isZh(lang) {
		return formatN(lang, "merge:列出 plan 失败:%v\n", err)
	}
	return formatN(lang, "merge: list plans: %v\n", err)
}

// branchCheckoutFailedMsg — /branch <name> with bad name.
func branchCheckoutFailedMsg(lang string, err error) string {
	if isZh(lang) {
		return formatN(lang, "branch: git checkout 失败:%v\n", err)
	}
	return formatN(lang, "branch: git checkout failed: %v\n", err)
}

// noLogAttached / noTraceAttached / attachedLogCleared etc.
func noLogAttached(lang string) string {
	if isZh(lang) {
		return "未附加日志"
	}
	return "no log attached"
}

func noTraceAttached(lang string) string {
	if isZh(lang) {
		return "未附加 trace"
	}
	return "no hitrace attached"
}

func attachedLogClearedMsg(lang string) string {
	if isZh(lang) {
		return "已清除附加日志"
	}
	return "attached log cleared"
}

func attachedTraceClearedMsg(lang string) string {
	if isZh(lang) {
		return "已清除附加 trace"
	}
	return "attached hitrace cleared"
}

// pasteCapturePromptLog — /log paste capture mode prompt.
func pasteCapturePromptLog(lang string) string {
	if isZh(lang) {
		return "粘贴日志,以单独一行 /end 结束捕获"
	}
	return "paste log, terminate with a lone /end line"
}

// pasteCapturePromptGeneric — /paste mode prompt.
func pasteCapturePromptGeneric(lang string) string {
	if isZh(lang) {
		return "粘贴内容,以单独一行 /end 结束捕获;空捕获按 Enter 取消"
	}
	return "paste content, terminate with a lone /end line; press Enter to cancel an empty capture"
}

// pasteNoCapture — log/generic paste mode aborted with no input.
func pasteNoCaptureLog(lang string) string {
	if isZh(lang) {
		return "未捕获到内容;附加日志保持不变"
	}
	return "no input captured; attached log unchanged"
}

func pasteNoCaptureGeneric(lang string) string {
	if isZh(lang) {
		return "未捕获到内容"
	}
	return "no input captured"
}

// memoryClearCancelled / Cleared / EmptyCancelled.
func memoryClearCancelled(lang string) string {
	if isZh(lang) {
		return "已取消清除"
	}
	return "clear cancelled"
}

func memoryClearedMsg(lang string) string {
	if isZh(lang) {
		return "已清除会话 memory。"
	}
	return "conversation memory cleared."
}

func memoryEmpty(lang string) string {
	if isZh(lang) {
		return "(空)"
	}
	return "(empty)"
}

// spinnerCancelHint — dim trailer line surfaced under the spinner
// while the input box is closed. Tells the operator HOW to interrupt
// since they cannot type a slash command. Two seconds of double-tap
// escalates to process exit.
func spinnerCancelHint(lang string) string {
	if isZh(lang) {
		return "Ctrl+C 取消(连按 2 次强制退出)"
	}
	return "Ctrl+C to cancel (double-tap within 2s to force-exit)"
}

// cancelInProgressMsg — surfaced after the user presses Ctrl+C (or
// types /cancel) while a Run is in flight. Tells them the cancel has
// been requested but lands at the next pipeline checkpoint, so the
// "spinner stuck for ~30s" surprise is preempted.
func cancelInProgressMsg(lang string) string {
	if isZh(lang) {
		return "✗ 取消已请求,等当前 LLM call 返回后生效(最多 ~30s)。再按一次 Ctrl+C 强制退出。"
	}
	return "✗ cancel requested; takes effect when the current LLM call returns (up to ~30s). Press Ctrl+C again to force exit."
}

// cancelNothingRunningMsg — `/cancel` typed at the idle prompt or
// when the runner doesn't expose a cancel surface (test stub). One
// line, no exit — operators can still /exit themselves.
func cancelNothingRunningMsg(lang string) string {
	if isZh(lang) {
		return "没有正在执行的请求可取消。/exit 退出。"
	}
	return "no Run in flight to cancel. /exit to leave."
}

// canceledByUserMsg — final user-facing summary line printed in the
// approve / pipeline result rendering when the Run unwound via
// CanceledError. Stage label is the most-specific stage observed
// (dispatchStage emits it; agent_loop fallback otherwise). Empty
// stage hides the parenthetical.
func canceledByUserMsg(lang, stage string) string {
	if isZh(lang) {
		if stage != "" {
			return fmt.Sprintf("✗ 已取消(在 stage=%s 处中断)。worktree(若有)已保留以便检查 — /worktree show 列出。", stage)
		}
		return "✗ 已取消。worktree(若有)已保留以便检查 — /worktree show 列出。"
	}
	if stage != "" {
		return fmt.Sprintf("✗ canceled (interrupted at stage=%s). Worktree (if any) preserved for inspection — /worktree show.", stage)
	}
	return "✗ canceled. Worktree (if any) preserved for inspection — /worktree show."
}
