package repl

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/operation"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
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
	return "Approve cancelled"
}

func writeApprovalAutoProceeding(lang, planID string, assessment writeflow.RiskAssessment, decision writeflow.ApprovalDecision) string {
	if isZh(lang) {
		return formatN(lang, "写入审批: plan %s 将自动执行（策略=%s, 风险=%s, 原因=%s）",
			planID, decision.Policy, assessment.Level, decision.ReasonCode)
	}
	return formatN(lang, "write approval: plan %s will auto-execute (policy=%s, risk=%s, reason=%s)",
		planID, decision.Policy, assessment.Level, decision.ReasonCode)
}

func writeApprovalDenied(lang, planID string, assessment writeflow.RiskAssessment, decision writeflow.ApprovalDecision) string {
	reasons := assessment.TopReasons(3)
	var details []string
	for _, reason := range reasons {
		if reason.Path != "" {
			details = append(details, formatN(lang, "%s/%s %s (%s)", reason.Level, reason.Code, reason.Detail, reason.Path))
			continue
		}
		details = append(details, formatN(lang, "%s/%s %s", reason.Level, reason.Code, reason.Detail))
	}
	suffix := ""
	if len(details) > 0 {
		suffix = " — " + strings.Join(details, "; ")
	}
	if isZh(lang) {
		return formatN(lang, "写入审批: plan %s 已被拒绝（策略=%s, 风险=%s, 原因=%s）%s",
			planID, decision.Policy, assessment.Level, decision.ReasonCode, suffix)
	}
	return formatN(lang, "write approval: plan %s denied (policy=%s, risk=%s, reason=%s)%s",
		planID, decision.Policy, assessment.Level, decision.ReasonCode, suffix)
}

// rejectConfirmedWithReason — message printed after /reject lands
// the rejection on disk, when the user supplied a reason.
func rejectConfirmedWithReason(lang, planID, reason string) string {
	if isZh(lang) {
		return formatN(lang, "已拒绝 plan %s — 原因: %s", planID, reason)
	}
	return formatN(lang, "Rejected plan %s — reason: %s", planID, reason)
}

// rejectConfirmedNoReason — same, no reason given.
func rejectConfirmedNoReason(lang, planID string) string {
	if isZh(lang) {
		return formatN(lang, "已拒绝 plan %s", planID)
	}
	return formatN(lang, "Rejected plan %s", planID)
}

// writeModeDisabled — printed when /mode write, /write, or /approve
// fires while codrax.yaml :: write_enabled is false. The
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
			formatN(lang, "  在 %s 中设置 `write_enabled: true` 并重启即可启用写模式。", target),
			"  auto/code/operation/data 模式无需额外配置。",
		}
		if created != "" {
			out = append(out, created)
		}
		return out
	}
	out := []string{
		formatN(lang, "%s rejected: write_enabled is false (or unset)", mode),
		formatN(lang, "  Set `write_enabled: true` in %s and restart to enable write mode.", target),
		"  auto/code/operation/data modes need no extra configuration.",
	}
	if created != "" {
		out = append(out, created)
	}
	return out
}

// bannerCapabilityLine produces a single line describing the active
// pipeline modes + the codrax.yaml backing them. Surfaced under the
// version badge so the user sees write_enabled state at startup
// rather than discovering it via a /mode write reject 30 turns in.
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

// multiRepoBannerLine surfaces the workspace topology at REPL
// startup so multi-repo users immediately see N sub-repos discovered
// and the /repos command path. Single-repo posture (parentRoot is
// itself a git repo) returns "" so the banner stays quiet — no UX
// chrome for the dominant case.
//
// Style is applied by REPL.banner: this line is intentionally more
// prominent than the dim status rows because multi-repo mode changes
// routing scope and memory footprint.
func multiRepoBannerLine(lang string, enabled bool, subRepoCount int, focusCount int, capN int) string {
	if subRepoCount <= 1 {
		return ""
	}
	if !enabled {
		if isZh(lang) {
			return formatN(lang, "🗂  单仓模式:multi-repo 路由已关闭;已发现 %d 个子仓;保持单仓可用 --multi-repo=false;/repos 查看", subRepoCount)
		}
		return formatN(lang, "🗂  single-repo mode: multi-repo routing disabled; discovered %d sub-repos; keep single-repo with --multi-repo=false; /repos", subRepoCount)
	}
	if isZh(lang) {
		if focusCount > 0 {
			return formatN(lang, "🗂  multi-repo: %d 个子仓 (active cap=%d, focus 已 pin %d 个);非跨仓任务用 --multi-repo=false;/repos", subRepoCount, capN, focusCount)
		}
		return formatN(lang, "🗂  multi-repo: %d 个子仓 (active cap=%d);非跨仓任务用 --multi-repo=false;/repos focus", subRepoCount, capN)
	}
	if focusCount > 0 {
		return formatN(lang, "🗂  multi-repo: %d sub-repos (active cap=%d, %d focus-pinned); not cross-repo? use --multi-repo=false; /repos", subRepoCount, capN, focusCount)
	}
	return formatN(lang, "🗂  multi-repo: %d sub-repos (active cap=%d); not cross-repo? use --multi-repo=false; /repos focus", subRepoCount, capN)
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
			cap = "模式: auto · code · operation · data · write (write_enabled=true)"
		} else {
			cap = "Modes: auto · code · operation · data · write (write_enabled=true)"
		}
	} else {
		if zh {
			cap = "模式: auto · code · operation · data (write 已禁用)"
		} else {
			cap = "Modes: auto · code · operation · data (write disabled)"
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
		return "  (没有产出内容 — 可能是分析器拒绝了请求，或上游 LLM 返回为空)。详情见 codrax-*.log；可以换一种问法，或 /clear 后再试。"
	}
	return "  (No content produced — the analyzer may have rejected the request, or the upstream LLM returned empty.) See codrax-*.log for details; rephrase the question, or /clear and try again."
}

// chitchatRouteSummary returns the (label, segments) pair the
// renderer folds into the dock shutdown line for a chitchat reply.
// Mirror of localRouteSummary — same shape, same rendering path
// (Renderer.SetRouteSummary → ◇ <label> · <segments> · 总耗时 Xs).
//
// Sibling-safe: callers that go through StartSpinner / StopSpinner
// arm the Renderer; callers that bypass the dock cycle use the
// returned tuple with FormatLightRouteSummary directly. No pterm
// styling here — the renderer applies the project's status palette
// so the line reads as a peer of "◆ 已结束 · …".
func chitchatRouteSummary(lang string) (string, []string) {
	if isZh(lang) {
		return "闲聊回复", []string{"未读仓库", "未生成 plan"}
	}
	return "chat reply", []string{"no repo read", "no plan"}
}

// noPendingPlan — user typed /approve or /reject without a pending
// plan to act on. Surface the recovery path (/mode write first), and
// if write_enabled is off, name THAT first since a /mode write dispatch
// would just bounce off the L2 gate.
func noPendingPlan(lang string) string {
	if isZh(lang) {
		return "当前没有待处理的方案 — 先用 /mode write 进入写模式并生成一份"
	}
	return "No pending plan — run /mode write to generate one"
}

// noPendingPlanWriteDisabled — same surface as noPendingPlan but for
// the case where write_enabled is off, so the recovery path is "fix
// the yaml first" rather than "run /mode write".
func noPendingPlanWriteDisabled(lang string) []string {
	if isZh(lang) {
		return []string{
			"没有待处理的 plan,且 write 模式被禁用。",
			"  在 codrax.yaml 中设置 `write_enabled: true` 并重启,然后 /mode write 生成 plan。",
		}
	}
	return []string{
		"No pending plan, and write mode is disabled.",
		"  Set `write_enabled: true` in codrax.yaml and restart, then run /mode write to generate one.",
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
			"没有可合并的 worktree。/merge 需要一次成功 /approve 留下的 worktree。",
			"  先用 /mode write 生成方案，再 /approve 落地（请确认 codrax.yaml 里 pipeline_keep_worktree_on_success: true），最后 /merge。",
		}
	}
	return []string{
		"No worktree to merge from. /merge needs a worktree preserved by a successful /approve.",
		"  Run /mode write, then /approve (with pipeline_keep_worktree_on_success: true), then /merge.",
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

// unsettledModePlanReject is the message /mode write prints when an
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
				"  /merge         merge into the main repo",
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
		out = append(out, "  收尾后再敲 /mode write。")
		return out
	}
	header := []string{
		formatN(lang, "✗ /mode write refused: an unsettled plan exists (%s, status=%s).", planID, status),
		"  Settle it first so the next plan can be drafted against the current repo state:",
	}
	out := append(header, menu...)
	out = append(out, "  Then run /mode write again.")
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
			suffix = "  · worktree 已不在 — /reject 清理"
		} else {
			suffix = "  · worktree gone — /reject to clear"
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
		return "  已自动切回 auto 模式 — 直接提问即可。要继续改代码就 /mode write。"
	}
	return "  Auto-switched back to auto mode — ask your next question directly. Use /mode write when you want to make another change."
}

// mergeSuccess — printed after a clean MergeIntoBranch return.
func mergeSuccess(lang, strategy, finalBranch string, count int) []string {
	if isZh(lang) {
		switch strategy {
		case "fast_forward":
			return []string{
				formatN(lang, "  ✓ 已 fast-forward %d 个 commit 到 %s。", count, finalBranch),
				"  下一步：git push（可选）。",
			}
		default:
			return []string{
				formatN(lang, "  ✓ 已在主仓创建分支 %s，cherry-pick %d 个 commit。", finalBranch, count),
				formatN(lang, "  下一步：cd <主仓> && git push -u origin %s，然后开 PR。", finalBranch),
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
			formatN(lang, "  ✓ Branch %s created on the main repo with %d cherry-picked commit(s).", finalBranch, count),
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
		"  Note: %d other pending or verify-failed plan(s) exist. About to approve %s; target a different one via /approve <plan-id> (/plan list shows them).",
		others, planID)
}

// skipVerifyAcknowledged — printed when /approve --skip-verify is
// honored. Tells the user the verify stage is being deliberately
// skipped so they don't expect a "tests passed" verdict.
func skipVerifyAcknowledged(lang string) string {
	if isZh(lang) {
		return "  --skip-verify 已生效：本次 approve 只 apply，不跑 verify 测试"
	}
	return "  --skip-verify acknowledged: this approve will only apply, no verify tests will run"
}

// mergeForceFailedWarning — printed when the user passes
// `/merge --include-failed` (or --force) and the resolved plan's
// Status is verify_failed. We surface a deliberate warning so the
// operator's eye registers that they're overriding the safety
// gate before any git command runs.
func mergeForceFailedWarning(lang, planID string) []string {
	if isZh(lang) {
		return []string{
			formatN(lang, "  · 强制合入 plan %s — 该 plan 的 verify 阶段曾失败。", planID),
			"  请先用 /plan show 核对 diff 和失败摘要，确认失败属于环境/CI 类原因（不是代码缺陷）再继续。",
		}
	}
	return []string{
		formatN(lang, "  · Force-merging plan %s — its verify stage previously failed.", planID),
		"  Confirm the diff and failure summary via /plan show; only proceed if the failure is environmental (CI/infra), not a code defect.",
	}
}

// mergeFailure — printed when MergeIntoBranch returned an error.
// gitDiag is the raw git diagnostic from the helper; the second
// line tells the user the helper rolled back so the main repo is
// in its prior state.
func mergeFailure(lang, gitDiag string) []string {
	if isZh(lang) {
		return []string{
			formatN(lang, "  ✗ 合并失败：%s", oneLine(gitDiag)),
			"  主仓已回滚到合并前的状态。可用 /worktree show 检查冲突文件，或 /reject 丢弃 plan 后重新规划。",
		}
	}
	return []string{
		formatN(lang, "  ✗ Merge failed: %s", oneLine(gitDiag)),
		"  The main repo was restored to its prior state. Use /worktree show to inspect, or /reject to discard the plan.",
	}
}

// recoveredPendingPlan — printed when /plan show found pendingPlanPath
// empty but PlanStore.List had a Status=pending_approval entry. The
// REPL recovers the pointer transparently so a previous failed
// /approve doesn't make the plan invisible.
func recoveredPendingPlan(lang, planID string) string {
	if isZh(lang) {
		return formatN(lang, "  已恢复待审批方案: %s", planID)
	}
	return formatN(lang, "  Recovered pending plan: %s", planID)
}

// noPendingPlanReject — same as noPendingPlan but for /reject.
func noPendingPlanReject(lang string) string {
	if isZh(lang) {
		return "当前没有可拒绝的 plan"
	}
	return "No pending plan to reject"
}

// modeSwitched — info line when user runs /mode <X>.
func modeSwitched(lang, mode string) string {
	if isZh(lang) {
		return formatN(lang, "已切换到 %s 模式", mode)
	}
	return formatN(lang, "Switched to %s mode", mode)
}

func currentUserModeMsg(lang, mode string) string {
	if isZh(lang) {
		return formatN(lang, "当前任务模式：%s（使用 /mode <auto|code|operation|data|write> 切换）", mode)
	}
	return formatN(lang, "Current task mode: %s (use /mode <auto|code|operation|data|write> to change)", mode)
}

// modeWorkflowHint returns a 1-2 line nudge for a newly-entered
// mode. Concise, no internal terminology, only the slash commands
// the user actually needs next. Caller wraps with r.info so the
// pterm style is FgDarkGray (subdued).
func modeWorkflowHint(lang, mode string) []string {
	zh := isZh(lang)
	switch mode {
	case "auto":
		if zh {
			return []string{
				"  后续请求会由结构化路由判定选择代码、操作、数据或本地回答路径。",
			}
		}
		return []string{
			"  Future requests use structured routing to choose code, operation, data, or local answer paths.",
		}
	case "code":
		if zh {
			return []string{
				"  后续请求会直接进入代码/源码分析流水线，不先走自动路由。",
			}
		}
		return []string{
			"  Future requests go directly to the code/source analysis pipeline.",
		}
	case "operation":
		if zh {
			return []string{
				"  后续请求会直接进入电脑操作流水线，风险策略仍会拦截危险动作。",
			}
		}
		return []string{
			"  Future requests go directly to the computer-operation pipeline; risk policy still applies.",
		}
	case "data":
		if zh {
			return []string{
				"  后续请求会直接进入数据处理流水线，不走源码证据门。",
			}
		}
		return []string{
			"  Future requests go directly to the data-processing pipeline.",
		}
	case "write":
		if zh {
			return []string{
				"  后续请求会生成改动方案，不会直接改主仓。",
				"  随后可用：/plan show 查看 diff · /approve 落地 · /reject 丢弃 · /mode auto 回到自动模式",
			}
		}
		return []string{
			"  Future requests produce a change proposal; the main repo is not changed directly.",
			"  Then: /plan show · /approve · /reject · /mode auto",
		}
	}
	return nil
}

func oneShotUserModeUsage(lang, cmd string, mode UserMode) string {
	zh := isZh(lang)
	switch mode.Normalize() {
	case UserModeCode:
		if zh {
			return cmd + " <问题> — 单次强制走代码/源码分析"
		}
		return cmd + " <request> — run one request through code/source analysis"
	case UserModeOperation:
		if zh {
			return cmd + " <任务> — 单次强制走电脑操作"
		}
		return cmd + " <task> — run one request through computer operation"
	case UserModeData:
		if zh {
			return cmd + " <任务> — 单次强制走数据处理"
		}
		return cmd + " <task> — run one request through data processing"
	case UserModeWrite:
		if zh {
			return cmd + " <改动需求> — 单次强制走写模式"
		}
		return cmd + " <change request> — run one request through write mode"
	default:
		if zh {
			return cmd + " <请求>"
		}
		return cmd + " <request>"
	}
}

// planReadyNudge prints next-step actions after the orchestrator
// emitted a ChangePlan during write-mode dispatch and the REPL
// auto-saved it. Without this nudge the user sees "plan saved: <path>"
// and has to remember the slash-command vocabulary; with it the path
// to /approve / /reject / /mode auto is one line away.
func planReadyNudge(lang string, planID string, changeCount int) []string {
	if isZh(lang) {
		return []string{
			formatN(lang, "改动方案已就绪：%s（%d 处改动）。", planID, changeCount),
			"  /plan show · /approve · /approve --skip-verify · /reject · /mode auto",
		}
	}
	return []string{
		formatN(lang, "Change proposal ready: %s (%d change(s)).", planID, changeCount),
		"  /plan show · /approve · /approve --skip-verify · /reject · /mode auto",
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
			formatN(lang, "  · 多阶段方案 (%d 个 phase): /approve 后用 `/phase show` 追踪进度,`/phase rollback` 回退当前 phase。", phaseCount),
		}
	}
	return []string{
		formatN(lang, "  · Multi-phase proposal (%d phases): after /approve, use `/phase show` to track progress and `/phase rollback` to reset the active phase.", phaseCount),
	}
}

// applyDoneNudge prints next-step actions after /approve completed.
// When apply succeeded, point at /mode auto for further questions and
// /mode write for a new change. When apply failed, the
// renderVerifyFailure already carries the failure-path next-step inline — applyDoneNudge skips
// the failure path so we don't double-print.
func applyDoneNudge(lang string) []string {
	if isZh(lang) {
		return []string{
			"  Apply 完成，已自动切回 auto 模式。要继续改代码就 /mode write。",
		}
	}
	return []string{
		"  Apply complete — auto-switched to auto mode. Use /mode write to make another change.",
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
				"  下一步：/approve --retry 重试 · /merge --include-failed 强行合入 · /reject 丢弃",
			}
		}
		return []string{
			"  Next: /approve --retry to retry · /merge --include-failed to merge anyway · /reject to discard",
		}
	case "partially_applied":
		if zh {
			return []string{
				"  下一步：/approve --retry 续 apply 剩余路径 · /reject 丢弃（注意：/merge 拒收 partially_applied 状态）",
			}
		}
		return []string{
			"  Next: /approve --retry to apply remaining paths · /reject to discard (note: /merge refuses partially_applied)",
		}
	case "unverified":
		if zh {
			return []string{
				"  下一步：/approve --retry 重跑（加上 tests 后）· /merge --include-failed 跳过验证强行合入 · /reject 丢弃",
			}
		}
		return []string{
			"  Next: /approve --retry to re-run (after adding tests) · /merge --include-failed to merge without verification · /reject to discard",
		}
	case "applied":
		if zh {
			return []string{
				"  下一步：/merge 合并到主仓 · /verify 重跑测试 · /worktree list 查看保留的 worktree",
			}
		}
		return []string{
			"  Next: /merge to merge into main · /verify to re-run tests · /worktree list to see preserved worktrees",
		}
	}
	// Default (pending_approval / unknown): the original footer.
	if zh {
		return []string{
			"  下一步：/approve 落地 · /reject 丢弃 · /plan clear 仅删本地副本",
		}
	}
	return []string{
		"  Next: /approve to apply · /reject to discard · /plan clear to delete the local copy",
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
	// within streamFirstByteTimeout (default 40s). Match BEFORE
	// the StreamStalledError branch because the typed error chain
	// preserves both signatures and we want the more specific
	// "never started" prose to surface — different operator
	// remediation than mid-stream stall.
	var fb *llm.StreamFirstByteTimeoutError
	if errors.As(err, &fb) {
		idle := fb.IdleFor
		if isZh(lang) {
			return fmt.Sprintf("上游 LLM 收到请求后 %s 内仍无任何响应。可能是 provider 服务端死锁、网络中间设备拦截，或模型 cold-start 卡住。请重试，或换一个 provider/model。", idle)
		}
		return fmt.Sprintf("Upstream LLM never sent any response within %s of receiving the request. Likely causes: provider-side deadlock, middlebox interference, or a cold-start hang. Retry, or try a different provider/model.", idle)
	}
	var nv *llm.StreamNoVisibleOutputTimeoutError
	if errors.As(err, &nv) {
		idle := nv.IdleFor
		if isZh(lang) {
			return fmt.Sprintf("上游 LLM 的流式连接仍在活动，但 %s 内没有产生可用于用户答案或工具调用的输出。可能是模型长时间停留在隐藏思考中。已自动中止；请重试，或换一个 provider/model。", idle)
		}
		return fmt.Sprintf("Upstream LLM stream stayed active but produced no user-visible answer or tool-call output for %s. It may be stuck in hidden reasoning. Aborted automatically; retry, or try a different provider/model.", idle)
	}
	// Streaming-watchdog mid-stream stall: stream started but went
	// silent for more than streamStallTimeout (default 60s).
	var ss *llm.StreamStalledError
	if errors.As(err, &ss) {
		idle := ss.IdleFor
		if isZh(lang) {
			return fmt.Sprintf("上游 LLM 的流式响应停滞 %s 无新字节，已自动中止。可能是模型在生成中卡住、上游网络抖动，或 thinking 块过大。请重试，或换一个 provider/model。", idle)
		}
		return fmt.Sprintf("Upstream LLM stream stalled with no bytes for %s; aborted automatically. Likely causes: model stuck mid-stream, upstream network blip, or an oversized thinking block. Retry, or try a different provider/model.", idle)
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	if strings.Contains(low, "context canceled") || strings.Contains(low, "context cancelled") {
		if isZh(lang) {
			return "请求被中断（可能是 Ctrl+C 或上游连接关闭）。请重试或检查网络。"
		}
		return "Request interrupted (likely Ctrl+C or upstream connection closed). Retry or check the network."
	}
	if strings.Contains(low, "deadline exceeded") {
		if isZh(lang) {
			return formatN(lang, "请求超时：%s", msg)
		}
		return formatN(lang, "Request timed out: %s", msg)
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

// armDockTerminalState flips the renderer's dock activity to the
// fatal / cancelled terminal kind based on err's shape so the
// commitDockShutdownLocked path renders "✗ 已失败" / "✗ 已取消"
// instead of the misleading "◆ 已结束 · …" success summary. err==nil
// is a no-op so the caller can call this unconditionally before
// StopSpinner.
//
// Classification mirrors friendlyRunError:
//   - cancelErrorCarrier (orchestrator.CanceledError, raised by
//     Ctrl+C / /cancel) → MarkRunCancelled
//   - context.Canceled (raw context.Canceled or wrapped, e.g. via
//     "context canceled" embedded in upstream HTTP error) →
//     MarkRunCancelled
//   - everything else (HTTP 5xx after retries, stream stalls,
//     auth failures, schema rejects) → MarkRunFatal
//
// nil renderer is tolerated for parity with surrounding StopSpinner
// call patterns.
func armDockTerminalState(renderer dockTerminalArmer, err error) {
	if renderer == nil || err == nil {
		return
	}
	var ce cancelErrorCarrier
	if errors.As(err, &ce) {
		renderer.MarkRunCancelled()
		return
	}
	if errors.Is(err, context.Canceled) {
		renderer.MarkRunCancelled()
		return
	}
	low := strings.ToLower(err.Error())
	if strings.Contains(low, "context canceled") || strings.Contains(low, "context cancelled") {
		renderer.MarkRunCancelled()
		return
	}
	renderer.MarkRunFatal()
}

func turnPolicyClassifierFallbackHint(lang string) string {
	if isZh(lang) {
		return "自动闲聊路由未稳定返回，已按仓库问题继续分析（fail-safe）。"
	}
	return "Auto chat routing did not return a stable decision; continuing with repository analysis (fail-safe)."
}

func operationUnavailableMsg(lang string, policy TurnPolicy) string {
	kind := strings.TrimSpace(policy.OperationKind)
	if kind == "" {
		kind = strings.TrimSpace(policy.Operation)
	}
	if kind == "" {
		kind = "operation"
	}
	if isZh(lang) {
		if policy.RequiresConfirmation || strings.EqualFold(policy.RiskLevel, "high") {
			return formatN(lang, "已识别到电脑操作/制品生成请求（%s），但当前版本尚未启用独立操作管线；由于该请求可能有副作用，系统不会把它转入源码分析或自动执行。", kind)
		}
		return formatN(lang, "已识别到电脑操作/制品生成请求（%s），但当前版本尚未启用独立操作管线；系统不会把它转入源码分析或自动执行。", kind)
	}
	if policy.RequiresConfirmation || strings.EqualFold(policy.RiskLevel, "high") {
		return formatN(lang, "Detected a computer-operation/artifact-generation request (%s), but the independent operation pipeline is not enabled in this build. Because the request may have side effects, Codrax will not reroute it into source analysis or execute it automatically.", kind)
	}
	return formatN(lang, "Detected a computer-operation/artifact-generation request (%s), but the independent operation pipeline is not enabled in this build. Codrax will not reroute it into source analysis or execute it automatically.", kind)
}

func operationPlanMarkdown(lang string, plan operation.Plan) string {
	if isZh(lang) {
		var b strings.Builder
		b.WriteString("已识别为电脑操作/制品生成请求，当前已进入独立操作管线的计划阶段。\n\n")
		b.WriteString(fmt.Sprintf("- 类型：`%s`\n", plan.Kind))
		b.WriteString(fmt.Sprintf("- 目标界面：`%s`\n", plan.TargetSurface))
		b.WriteString(fmt.Sprintf("- 风险：`%s`\n", plan.RiskLevel))
		if len(plan.SideEffects) > 0 {
			b.WriteString(fmt.Sprintf("- 可能副作用：`%s`\n", strings.Join(plan.SideEffects, "`, `")))
		}
		if plan.NeedsRepoAccess {
			b.WriteString("- 需要源码事实：是，但不会误入普通源码分析；后续执行器需要显式接入源码事实收集。\n")
		}
		if plan.RequiresConfirmation {
			b.WriteString("- 需要确认：是\n")
		}
		b.WriteString("\n计划步骤：\n")
		for i, step := range plan.Steps {
			suffix := ""
			if step.Blocked {
				suffix = "（暂停）"
			}
			title, detail := localizedOperationStep(lang, step.Title, step.Detail)
			b.WriteString(fmt.Sprintf("%d. %s%s：%s\n", i+1, title, suffix, detail))
		}
		if plan.CanExecute {
			b.WriteString(fmt.Sprintf("\n已匹配 provider：`%s` / tool `%s`。运行 `/approve` 后才会执行。\n", plan.Provider, plan.ProviderTool))
		} else if plan.Provider != "" && plan.ProviderTool != "" {
			b.WriteString(fmt.Sprintf("\n已匹配 provider：`%s` / tool `%s`，但需要确认。运行 `/approve` 执行，或 `/reject <原因>` 拒绝。\n", plan.Provider, plan.ProviderTool))
		} else {
			b.WriteString(fmt.Sprintf("\n当前未执行：%s。\n", plan.MissingCapability))
		}
		b.WriteString("系统没有把该请求转入源码分析，也没有自动执行任何有副作用的工具。")
		return strings.TrimSpace(b.String())
	}

	var b strings.Builder
	b.WriteString("Codrax recognized this as a computer-operation/artifact-generation request and entered the independent operation planning path.\n\n")
	b.WriteString(fmt.Sprintf("- Type: `%s`\n", plan.Kind))
	b.WriteString(fmt.Sprintf("- Target surface: `%s`\n", plan.TargetSurface))
	b.WriteString(fmt.Sprintf("- Risk: `%s`\n", plan.RiskLevel))
	if len(plan.SideEffects) > 0 {
		b.WriteString(fmt.Sprintf("- Possible side effects: `%s`\n", strings.Join(plan.SideEffects, "`, `")))
	}
	if plan.NeedsRepoAccess {
		b.WriteString("- Needs source facts: yes, but Codrax did not reroute the request into ordinary source analysis; a future executor must attach source-fact collection explicitly.\n")
	}
	if plan.RequiresConfirmation {
		b.WriteString("- Requires confirmation: yes\n")
	}
	b.WriteString("\nPlanned steps:\n")
	for i, step := range plan.Steps {
		suffix := ""
		if step.Blocked {
			suffix = " (paused)"
		}
		b.WriteString(fmt.Sprintf("%d. %s%s: %s\n", i+1, step.Title, suffix, step.Detail))
	}
	if plan.CanExecute {
		b.WriteString(fmt.Sprintf("\nMatched provider: `%s` / tool `%s`. Run `/approve` to execute.\n", plan.Provider, plan.ProviderTool))
	} else if plan.Provider != "" && plan.ProviderTool != "" {
		b.WriteString(fmt.Sprintf("\nMatched provider: `%s` / tool `%s`, but confirmation is required. Run `/approve` to execute, or `/reject <reason>` to reject.\n", plan.Provider, plan.ProviderTool))
	} else {
		b.WriteString(fmt.Sprintf("\nNot executed: %s.\n", plan.MissingCapability))
	}
	b.WriteString("Codrax did not reroute this request into source analysis and did not execute side-effecting tools automatically.")
	return strings.TrimSpace(b.String())
}

func commandOperationPlanMarkdown(lang string, plan operation.CommandOperationPlan) string {
	if isZh(lang) {
		var b strings.Builder
		switch plan.Status {
		case operation.StatusNeedsClarification:
			b.WriteString("已识别为通用命令行操作请求，但还缺少关键信息。\n\n")
			b.WriteString("需要补充：\n")
			for i, q := range plan.ClarifyingQuestions {
				b.WriteString(fmt.Sprintf("%d. %s\n", i+1, q.Question))
				if len(q.Suggestions) > 0 {
					b.WriteString(fmt.Sprintf("   建议：%s\n", strings.Join(q.Suggestions, "；")))
				}
			}
			b.WriteString("\n系统不会猜测命令，也不会自动执行。直接回复缺失信息即可继续，或用 `/cancel` 取消。")
			return strings.TrimSpace(b.String())
		case operation.StatusBlocked:
			b.WriteString(fmt.Sprintf("操作计划 `%s` 已被策略阻止。\n\n", plan.ID))
			b.WriteString(fmt.Sprintf("- 风险：`%s`\n", plan.RiskLevel))
			b.WriteString(fmt.Sprintf("- 原因：%s\n", plan.BlockReason))
			b.WriteString("没有执行任何命令。")
			return strings.TrimSpace(b.String())
		case operation.StatusComplete, operation.StatusBudgetExhausted, operation.StatusPartialAnswer:
			b.WriteString(commandOperationTerminalPlanMarkdownZh(plan))
			return strings.TrimSpace(b.String())
		default:
			b.WriteString(fmt.Sprintf("操作计划 `%s` 已就绪，等待批准。\n\n", plan.ID))
			b.WriteString(fmt.Sprintf("- 风险：`%s`\n", plan.RiskLevel))
			b.WriteString(fmt.Sprintf("- 审批：`%s`\n", plan.ApprovalMode))
			if strings.TrimSpace(plan.ApprovalReason) != "" {
				b.WriteString(fmt.Sprintf("- 审批原因：%s\n", plan.ApprovalReason))
			}
			b.WriteString(fmt.Sprintf("- 工作目录：`%s`\n", plan.WorkDir))
			if strings.TrimSpace(plan.Goal) != "" {
				b.WriteString(fmt.Sprintf("- 目标：%s\n", plan.Goal))
			}
			if strings.TrimSpace(plan.NextBatch) != "" {
				b.WriteString(fmt.Sprintf("- 本批目的：%s\n", plan.NextBatch))
			}
			if plan.ContinueAfter {
				b.WriteString("- 规划方式：执行后继续规划下一批操作\n")
			}
			b.WriteString("\n计划步骤：\n")
			for i, step := range plan.Steps {
				cmd := step.Program
				if strings.TrimSpace(step.Shell) != "" {
					cmd = step.Shell
				} else if len(step.Args) > 0 {
					cmd = strings.TrimSpace(step.Program + " " + strings.Join(step.Args, " "))
				}
				b.WriteString(fmt.Sprintf("%d. %s\n", i+1, step.Title))
				b.WriteString(fmt.Sprintf("   `$ %s`\n", cmd))
				b.WriteString(fmt.Sprintf("   风险：`%s` · 审批：`%s` · %s\n", step.RiskLevel, step.AutoApproval, step.Reason))
			}
			b.WriteString("\n运行 `/approve` 执行，或 `/reject <原因>` 拒绝。")
			return strings.TrimSpace(b.String())
		}
	}

	var b strings.Builder
	switch plan.Status {
	case operation.StatusNeedsClarification:
		b.WriteString("Codrax recognized a command-line operation request, but key details are missing.\n\n")
		b.WriteString("Needed:\n")
		for i, q := range plan.ClarifyingQuestions {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, q.Question))
			if len(q.Suggestions) > 0 {
				b.WriteString(fmt.Sprintf("   Suggestions: %s\n", strings.Join(q.Suggestions, "; ")))
			}
		}
		b.WriteString("\nCodrax will not guess the command and did not execute anything. Reply with the missing details to continue, or use `/cancel` to cancel.")
		return strings.TrimSpace(b.String())
	case operation.StatusBlocked:
		b.WriteString(fmt.Sprintf("Operation plan `%s` was blocked by policy.\n\n", plan.ID))
		b.WriteString(fmt.Sprintf("- Risk: `%s`\n", plan.RiskLevel))
		b.WriteString(fmt.Sprintf("- Reason: %s\n", plan.BlockReason))
		b.WriteString("No command was executed.")
		return strings.TrimSpace(b.String())
	case operation.StatusComplete, operation.StatusBudgetExhausted, operation.StatusPartialAnswer:
		b.WriteString(commandOperationTerminalPlanMarkdownEn(plan))
		return strings.TrimSpace(b.String())
	default:
		b.WriteString(fmt.Sprintf("Operation plan `%s` is ready and awaiting approval.\n\n", plan.ID))
		b.WriteString(fmt.Sprintf("- Risk: `%s`\n", plan.RiskLevel))
		b.WriteString(fmt.Sprintf("- Approval: `%s`\n", plan.ApprovalMode))
		if strings.TrimSpace(plan.ApprovalReason) != "" {
			b.WriteString(fmt.Sprintf("- Approval reason: %s\n", plan.ApprovalReason))
		}
		b.WriteString(fmt.Sprintf("- Working directory: `%s`\n", plan.WorkDir))
		if strings.TrimSpace(plan.Goal) != "" {
			b.WriteString(fmt.Sprintf("- Goal: %s\n", plan.Goal))
		}
		if strings.TrimSpace(plan.NextBatch) != "" {
			b.WriteString(fmt.Sprintf("- This batch: %s\n", plan.NextBatch))
		}
		if plan.ContinueAfter {
			b.WriteString("- Planning: continue after this batch executes\n")
		}
		b.WriteString("\nPlanned steps:\n")
		for i, step := range plan.Steps {
			cmd := step.Program
			if strings.TrimSpace(step.Shell) != "" {
				cmd = step.Shell
			} else if len(step.Args) > 0 {
				cmd = strings.TrimSpace(step.Program + " " + strings.Join(step.Args, " "))
			}
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, step.Title))
			b.WriteString(fmt.Sprintf("   `$ %s`\n", cmd))
			b.WriteString(fmt.Sprintf("   Risk: `%s` · Approval: `%s` · %s\n", step.RiskLevel, step.AutoApproval, step.Reason))
		}
		b.WriteString("\nRun `/approve` to execute, or `/reject <reason>` to reject.")
		return strings.TrimSpace(b.String())
	}
}

func commandOperationTerminalPlanMarkdownZh(plan operation.CommandOperationPlan) string {
	var b strings.Builder
	switch plan.Status {
	case operation.StatusBudgetExhausted:
		b.WriteString(fmt.Sprintf("操作计划 `%s` 已达到本轮预算，停止继续执行。\n\n", plan.ID))
	case operation.StatusPartialAnswer:
		b.WriteString(fmt.Sprintf("操作计划 `%s` 已有部分结果，停止继续执行。\n\n", plan.ID))
	default:
		b.WriteString(fmt.Sprintf("操作计划 `%s` 已确认无需继续执行。\n\n", plan.ID))
	}
	b.WriteString(fmt.Sprintf("- 风险：`%s`\n", plan.RiskLevel))
	if strings.TrimSpace(plan.BlockReason) != "" {
		b.WriteString(fmt.Sprintf("- 说明：%s\n", plan.BlockReason))
	}
	b.WriteString("没有执行新的命令；将基于已收集结果生成最终答案。")
	return strings.TrimSpace(b.String())
}

func commandOperationTerminalPlanMarkdownEn(plan operation.CommandOperationPlan) string {
	var b strings.Builder
	switch plan.Status {
	case operation.StatusBudgetExhausted:
		b.WriteString(fmt.Sprintf("Operation plan `%s` reached the operation budget and will not continue.\n\n", plan.ID))
	case operation.StatusPartialAnswer:
		b.WriteString(fmt.Sprintf("Operation plan `%s` has a partial answer and will not continue.\n\n", plan.ID))
	default:
		b.WriteString(fmt.Sprintf("Operation plan `%s` determined that no further command is needed.\n\n", plan.ID))
	}
	b.WriteString(fmt.Sprintf("- Risk: `%s`\n", plan.RiskLevel))
	if strings.TrimSpace(plan.BlockReason) != "" {
		b.WriteString(fmt.Sprintf("- Note: %s\n", plan.BlockReason))
	}
	b.WriteString("No new command was executed; the final answer will use the observations already collected.")
	return strings.TrimSpace(b.String())
}

func commandOperationAutoExecuteMarkdown(lang string, plan operation.CommandOperationPlan) string {
	if isZh(lang) {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("操作计划 `%s` 将自动执行。\n\n", plan.ID))
		b.WriteString(fmt.Sprintf("- 审批：`%s`\n", plan.ApprovalMode))
		b.WriteString(fmt.Sprintf("- 风险：`%s`\n", plan.RiskLevel))
		b.WriteString(fmt.Sprintf("- 工作目录：`%s`\n", plan.WorkDir))
		if strings.TrimSpace(plan.Goal) != "" {
			b.WriteString(fmt.Sprintf("- 目标：%s\n", plan.Goal))
		}
		if strings.TrimSpace(plan.NextBatch) != "" {
			b.WriteString(fmt.Sprintf("- 本批目的：%s\n", plan.NextBatch))
		}
		if plan.ContinueAfter {
			b.WriteString("- 规划方式：执行后继续规划下一批操作\n")
		}
		b.WriteString("\n即将执行：\n")
		shown := 0
		for i, step := range plan.Steps {
			cmd := commandOperationStepCommand(step)
			if strings.TrimSpace(cmd) == "" {
				continue
			}
			shown++
			b.WriteString(fmt.Sprintf("%d. `$ %s`\n", i+1, cmd))
		}
		if shown == 0 {
			b.WriteString("（没有可执行命令；系统不会执行空计划。）\n")
		}
		return strings.TrimSpace(b.String())
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Operation plan `%s` will run automatically.\n\n", plan.ID))
	b.WriteString(fmt.Sprintf("- Approval: `%s`\n", plan.ApprovalMode))
	b.WriteString(fmt.Sprintf("- Risk: `%s`\n", plan.RiskLevel))
	b.WriteString(fmt.Sprintf("- Working directory: `%s`\n", plan.WorkDir))
	if strings.TrimSpace(plan.Goal) != "" {
		b.WriteString(fmt.Sprintf("- Goal: %s\n", plan.Goal))
	}
	if strings.TrimSpace(plan.NextBatch) != "" {
		b.WriteString(fmt.Sprintf("- This batch: %s\n", plan.NextBatch))
	}
	if plan.ContinueAfter {
		b.WriteString("- Planning: continue after this batch executes\n")
	}
	b.WriteString("\nCommands to run:\n")
	shown := 0
	for i, step := range plan.Steps {
		cmd := commandOperationStepCommand(step)
		if strings.TrimSpace(cmd) == "" {
			continue
		}
		shown++
		b.WriteString(fmt.Sprintf("%d. `$ %s`\n", i+1, cmd))
	}
	if shown == 0 {
		b.WriteString("(No executable commands; Codrax will not execute an empty plan.)\n")
	}
	return strings.TrimSpace(b.String())
}

func commandOperationAutoValidateMarkdown(lang string, plan operation.CommandOperationPlan) string {
	if isZh(lang) {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("操作计划 `%s` 将先自动校验；校验通过后才会执行。\n\n", plan.ID))
		b.WriteString(fmt.Sprintf("- 审批：`%s`\n", plan.ApprovalMode))
		b.WriteString(fmt.Sprintf("- 风险：`%s`\n", plan.RiskLevel))
		b.WriteString(fmt.Sprintf("- 工作目录：`%s`\n", plan.WorkDir))
		if strings.TrimSpace(plan.Goal) != "" {
			b.WriteString(fmt.Sprintf("- 目标：%s\n", plan.Goal))
		}
		if strings.TrimSpace(plan.NextBatch) != "" {
			b.WriteString(fmt.Sprintf("- 本批目的：%s\n", plan.NextBatch))
		}
		if plan.ContinueAfter {
			b.WriteString("- 规划方式：执行后继续规划下一批操作\n")
		}
		b.WriteString("\n待校验命令：\n")
		shown := 0
		for i, step := range plan.Steps {
			cmd := commandOperationStepCommand(step)
			if strings.TrimSpace(cmd) == "" {
				continue
			}
			shown++
			b.WriteString(fmt.Sprintf("%d. `$ %s`\n", i+1, cmd))
		}
		if shown == 0 {
			b.WriteString("（没有可执行命令；系统不会执行空计划。）\n")
		}
		return strings.TrimSpace(b.String())
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Operation plan `%s` will be validated automatically before any command runs.\n\n", plan.ID))
	b.WriteString(fmt.Sprintf("- Approval: `%s`\n", plan.ApprovalMode))
	b.WriteString(fmt.Sprintf("- Risk: `%s`\n", plan.RiskLevel))
	b.WriteString(fmt.Sprintf("- Working directory: `%s`\n", plan.WorkDir))
	if strings.TrimSpace(plan.Goal) != "" {
		b.WriteString(fmt.Sprintf("- Goal: %s\n", plan.Goal))
	}
	if strings.TrimSpace(plan.NextBatch) != "" {
		b.WriteString(fmt.Sprintf("- This batch: %s\n", plan.NextBatch))
	}
	if plan.ContinueAfter {
		b.WriteString("- Planning: continue after this batch executes\n")
	}
	b.WriteString("\nCommands to validate:\n")
	shown := 0
	for i, step := range plan.Steps {
		cmd := commandOperationStepCommand(step)
		if strings.TrimSpace(cmd) == "" {
			continue
		}
		shown++
		b.WriteString(fmt.Sprintf("%d. `$ %s`\n", i+1, cmd))
	}
	if shown == 0 {
		b.WriteString("(No executable commands; Codrax will not execute an empty plan.)\n")
	}
	return strings.TrimSpace(b.String())
}

func operationNoPendingMsg(lang string) string {
	if isZh(lang) {
		return "当前没有待处理的操作计划。"
	}
	return "No pending operation plan."
}

func pendingOperationBanner(lang, id, status string) string {
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(status)
	if id == "" {
		id = "operation"
	}
	if isZh(lang) {
		if status != "" {
			return fmt.Sprintf("待处理操作: %s · 状态 %s · /operation show · /approve · /reject · /cancel", id, status)
		}
		return fmt.Sprintf("待处理操作: %s · /operation show · /approve · /reject · /cancel", id)
	}
	if status != "" {
		return fmt.Sprintf("Pending operation: %s · status %s · /operation show · /approve · /reject · /cancel", id, status)
	}
	return fmt.Sprintf("Pending operation: %s · /operation show · /approve · /reject · /cancel", id)
}

func pendingOperationClarificationBanner(lang, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		id = "operation"
	}
	if isZh(lang) {
		return fmt.Sprintf("待补充操作: %s · 直接回复缺失信息继续 · /operation show · /cancel", id)
	}
	return fmt.Sprintf("Pending operation clarification: %s · reply with the missing details · /operation show · /cancel", id)
}

func operationNoHistoryMsg(lang string) string {
	if isZh(lang) {
		return "还没有操作计划历史。"
	}
	return "No operation plan history yet."
}

func operationHistoryRow(lang, id, status, risk string) string {
	if isZh(lang) {
		return fmt.Sprintf("操作 %s · 状态 %s · 风险 %s", id, status, risk)
	}
	return fmt.Sprintf("operation %s · status %s · risk %s", id, status, risk)
}

func operationAutoStatusMsg(lang string) string {
	if isZh(lang) {
		return "operation 自动审批默认开启：普通结构化命令会自动执行；高危动作等待批准，灾难性动作直接拒绝。可用 operation_command_auto_approve: false 退回低风险-only。"
	}
	return "Operation auto approval is on by default: ordinary structured commands auto-run; high-risk actions wait for approval and catastrophic actions are denied. Set operation_command_auto_approve: false for low-risk-only mode."
}

func operationHelpMsg(lang string) string {
	if isZh(lang) {
		return "/operation [show|history|auto] — 查看待处理操作、历史或自动批准状态。日常审批使用 /approve、/reject、/cancel。"
	}
	return "/operation [show|history|auto] — show pending operations, history, or auto-approval status. Use /approve, /reject, /cancel for day-to-day approval."
}

func workflowNoActiveMsg(lang string) string {
	if isZh(lang) {
		return "当前没有 active operation workflow。"
	}
	return "No active operation workflow."
}

func workflowCancelledMsg(lang, id string) string {
	if isZh(lang) {
		return fmt.Sprintf("operation workflow `%s` 已取消。", id)
	}
	return fmt.Sprintf("Operation workflow `%s` cancelled.", id)
}

func workflowHelpMsg(lang string) string {
	if isZh(lang) {
		return "/workflow [show|cancel] — 查看或取消当前 operation skill 工作流。日常继续执行仍使用 /approve。"
	}
	return "/workflow [show|cancel] — show or cancel the active operation skill workflow. Continue one step with /approve."
}

func providerWorkflowMarkdown(lang string, wf operation.WorkflowInstance) string {
	var b strings.Builder
	current, hasCurrent := wf.CurrentAction()
	if isZh(lang) {
		b.WriteString(fmt.Sprintf("Operation workflow `%s`\n\n", wf.ID))
		b.WriteString(fmt.Sprintf("- 当前：`%s`\n", firstNonEmptyString(current.Compact(), wf.CurrentID, "none")))
		b.WriteString(fmt.Sprintf("- 队列：%d 个待处理动作\n", len(wf.Queue)))
		b.WriteString(fmt.Sprintf("- 图：%d 个节点 / %d 条边\n", len(wf.Actions), len(wf.Edges)))
		if wf.Cancelled {
			b.WriteString("- 状态：已取消\n")
		}
		b.WriteString("\n动作：\n")
		for _, action := range wf.Actions {
			marker := " "
			if hasCurrent && action.ID == current.ID {
				marker = "*"
			}
			b.WriteString(fmt.Sprintf("%s `%s`\n", marker, action.Compact()))
		}
		if len(wf.Edges) > 0 {
			b.WriteString("\n边：\n")
			for _, edge := range wf.Edges {
				b.WriteString(fmt.Sprintf("- `%s`\n", edge.Compact()))
			}
		}
		b.WriteString("\n使用 `/approve` 执行当前动作，或 `/workflow cancel` 取消。")
		return strings.TrimSpace(b.String())
	}
	b.WriteString(fmt.Sprintf("Operation workflow `%s`\n\n", wf.ID))
	b.WriteString(fmt.Sprintf("- Current: `%s`\n", firstNonEmptyString(current.Compact(), wf.CurrentID, "none")))
	b.WriteString(fmt.Sprintf("- Queue: %d pending action(s)\n", len(wf.Queue)))
	b.WriteString(fmt.Sprintf("- Graph: %d node(s) / %d edge(s)\n", len(wf.Actions), len(wf.Edges)))
	if wf.Cancelled {
		b.WriteString("- Status: cancelled\n")
	}
	b.WriteString("\nActions:\n")
	for _, action := range wf.Actions {
		marker := " "
		if hasCurrent && action.ID == current.ID {
			marker = "*"
		}
		b.WriteString(fmt.Sprintf("%s `%s`\n", marker, action.Compact()))
	}
	if len(wf.Edges) > 0 {
		b.WriteString("\nEdges:\n")
		for _, edge := range wf.Edges {
			b.WriteString(fmt.Sprintf("- `%s`\n", edge.Compact()))
		}
	}
	b.WriteString("\nRun `/approve` to execute the current action, or `/workflow cancel` to cancel.")
	return strings.TrimSpace(b.String())
}

func operationExecutorNotReadyMsg(lang, id string) string {
	if isZh(lang) {
		return fmt.Sprintf("操作计划 `%s` 已等待执行，但当前批次尚未接入命令执行器；没有执行任何命令。", id)
	}
	return fmt.Sprintf("Operation plan `%s` is awaiting execution, but the command executor is not connected in this batch; no command was executed.", id)
}

func providerOperationResultMarkdown(lang string, plan operation.Plan, result providerOperationResult) string {
	if isZh(lang) {
		var b strings.Builder
		if result.Status == operation.StatusExecuted {
			b.WriteString(fmt.Sprintf("操作 provider `%s` 已执行完成。\n\n", result.Provider))
		} else {
			b.WriteString(fmt.Sprintf("操作 provider `%s` 执行失败。\n\n", result.Provider))
		}
		b.WriteString(fmt.Sprintf("- 类型：`%s`\n", plan.Kind))
		b.WriteString(fmt.Sprintf("- 工具：`%s`\n", result.Tool))
		if result.Observations > 0 {
			b.WriteString(fmt.Sprintf("- 外部观测：%d 条\n", result.Observations))
		}
		if len(result.ArtifactRefs) > 0 {
			b.WriteString(fmt.Sprintf("- 产物/引用：`%s`\n", strings.Join(providerArtifactRefsForDisplay(result.ArtifactRefs), "`, `")))
		}
		if result.VerificationStatus != "" {
			b.WriteString(fmt.Sprintf("- 验证：`%s`", result.VerificationStatus))
			if result.VerificationSummary != "" {
				b.WriteString(fmt.Sprintf(" — %s", result.VerificationSummary))
			}
			b.WriteString("\n")
		}
		if len(result.NextActions) > 0 {
			b.WriteString(fmt.Sprintf("- 建议后续动作：%d 个\n", len(result.NextActions)))
			for i, action := range result.NextActions {
				if i >= 3 {
					break
				}
				b.WriteString(fmt.Sprintf("  %d. `%s`\n", i+1, action.Compact()))
			}
		}
		if text := strings.TrimSpace(result.ReturnAction.Compact()); text != "" {
			b.WriteString(fmt.Sprintf("- 建议回跳动作：`%s`\n", text))
		}
		if !result.WorkflowState.IsZero() {
			b.WriteString(fmt.Sprintf("- 工作流状态：`%s`\n", result.WorkflowState.Compact()))
		}
		if len(result.WorkflowDiagnostics) > 0 {
			b.WriteString(fmt.Sprintf("- 工作流提示：%s\n", strings.Join(result.WorkflowDiagnostics, "; ")))
		}
		if result.Error != "" {
			b.WriteString(fmt.Sprintf("- 错误：%s\n", result.Error))
		}
		if result.Summary != "" {
			b.WriteString("\n结果摘要：\n")
			b.WriteString(indentBlock(commandOperationDisplayPreview(result.Summary, lang, result.PayloadRef != ""), "  "))
			b.WriteString("\n")
		}
		if result.PayloadRef != "" {
			b.WriteString(fmt.Sprintf("\n完整输出：`%s`", result.PayloadRef))
		}
		return strings.TrimSpace(b.String())
	}
	var b strings.Builder
	if result.Status == operation.StatusExecuted {
		b.WriteString(fmt.Sprintf("Operation provider `%s` completed.\n\n", result.Provider))
	} else {
		b.WriteString(fmt.Sprintf("Operation provider `%s` failed.\n\n", result.Provider))
	}
	b.WriteString(fmt.Sprintf("- Kind: `%s`\n", plan.Kind))
	b.WriteString(fmt.Sprintf("- Tool: `%s`\n", result.Tool))
	if result.Observations > 0 {
		b.WriteString(fmt.Sprintf("- External observations: %d\n", result.Observations))
	}
	if len(result.ArtifactRefs) > 0 {
		b.WriteString(fmt.Sprintf("- Artifacts/refs: `%s`\n", strings.Join(providerArtifactRefsForDisplay(result.ArtifactRefs), "`, `")))
	}
	if result.VerificationStatus != "" {
		b.WriteString(fmt.Sprintf("- Verification: `%s`", result.VerificationStatus))
		if result.VerificationSummary != "" {
			b.WriteString(fmt.Sprintf(" — %s", result.VerificationSummary))
		}
		b.WriteString("\n")
	}
	if len(result.NextActions) > 0 {
		b.WriteString(fmt.Sprintf("- Suggested next actions: %d\n", len(result.NextActions)))
		for i, action := range result.NextActions {
			if i >= 3 {
				break
			}
			b.WriteString(fmt.Sprintf("  %d. `%s`\n", i+1, action.Compact()))
		}
	}
	if text := strings.TrimSpace(result.ReturnAction.Compact()); text != "" {
		b.WriteString(fmt.Sprintf("- Suggested return action: `%s`\n", text))
	}
	if !result.WorkflowState.IsZero() {
		b.WriteString(fmt.Sprintf("- Workflow state: `%s`\n", result.WorkflowState.Compact()))
	}
	if len(result.WorkflowDiagnostics) > 0 {
		b.WriteString(fmt.Sprintf("- Workflow notes: %s\n", strings.Join(result.WorkflowDiagnostics, "; ")))
	}
	if result.Error != "" {
		b.WriteString(fmt.Sprintf("- Error: %s\n", result.Error))
	}
	if result.Summary != "" {
		b.WriteString("\nResult summary:\n")
		b.WriteString(indentBlock(commandOperationDisplayPreview(result.Summary, lang, result.PayloadRef != ""), "  "))
		b.WriteString("\n")
	}
	if result.PayloadRef != "" {
		b.WriteString(fmt.Sprintf("\nFull output: `%s`", result.PayloadRef))
	}
	return strings.TrimSpace(b.String())
}

func providerArtifactRefsForDisplay(refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	max := len(refs)
	if max > 4 {
		max = 4
	}
	out := make([]string, 0, max)
	for i := 0; i < max; i++ {
		out = append(out, oneLineClamp(refs[i], 180))
	}
	if len(refs) > max {
		out = append(out, fmt.Sprintf("+%d more", len(refs)-max))
	}
	return out
}

func operationRejectedMsg(lang, id, reason string) string {
	if isZh(lang) {
		if strings.TrimSpace(reason) == "" {
			return fmt.Sprintf("已拒绝操作计划 `%s`。", id)
		}
		return fmt.Sprintf("已拒绝操作计划 `%s`：%s", id, reason)
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Sprintf("Rejected operation plan `%s`.", id)
	}
	return fmt.Sprintf("Rejected operation plan `%s`: %s", id, reason)
}

func commandOperationProgressMsg(lang string, step operation.CommandStep) string {
	title := strings.TrimSpace(step.Title)
	if title == "" {
		title = strings.TrimSpace(step.Program)
	}
	if title == "" && strings.TrimSpace(step.Shell) != "" {
		title = "shell"
	}
	if title == "" {
		title = strings.TrimSpace(step.ID)
	}
	if title == "" {
		title = "step"
	}
	if isZh(lang) {
		return fmt.Sprintf("操作中：%s", title)
	}
	return fmt.Sprintf("Operating: %s", title)
}

func providerOperationProgressMsg(lang string, pending pendingProviderOperation) string {
	plan := pending.Plan
	provider := strings.TrimSpace(plan.Provider)
	kind := strings.TrimSpace(plan.Kind)
	if provider == "" {
		provider = "provider"
	}
	if kind == "" {
		kind = "operation"
	}
	if isZh(lang) {
		return fmt.Sprintf("操作中：调用 %s 处理 %s", provider, kind)
	}
	return fmt.Sprintf("Operating: calling %s for %s", provider, kind)
}

func operationCancelledMsg(lang, id string) string {
	if isZh(lang) {
		return fmt.Sprintf("已取消操作计划 `%s`。", id)
	}
	return fmt.Sprintf("Cancelled operation plan `%s`.", id)
}

func commandOperationResultMarkdown(lang string, plan operation.CommandOperationPlan, result operation.CommandOperationResult) string {
	if isZh(lang) {
		var b strings.Builder
		switch result.Status {
		case operation.StatusExecuted:
			b.WriteString(fmt.Sprintf("操作计划 `%s` 已执行完成。\n\n", plan.ID))
		case operation.StatusComplete:
			b.WriteString(fmt.Sprintf("操作计划 `%s` 已确认无需继续执行。\n\n", plan.ID))
		case operation.StatusBudgetExhausted:
			b.WriteString(fmt.Sprintf("操作计划 `%s` 已达到本轮预算。\n\n", plan.ID))
		case operation.StatusPartialAnswer:
			b.WriteString(fmt.Sprintf("操作计划 `%s` 已形成部分结果。\n\n", plan.ID))
		case operation.StatusCancelled:
			b.WriteString(fmt.Sprintf("操作计划 `%s` 已取消。\n\n", plan.ID))
		default:
			b.WriteString(fmt.Sprintf("操作计划 `%s` 执行失败。\n\n", plan.ID))
		}
		for i, step := range result.StepResults {
			planStep := commandOperationStepByID(plan, step.StepID)
			b.WriteString(fmt.Sprintf("%d. step `%s`：%s\n", i+1, step.StepID, step.Status))
			if cmd := commandOperationStepCommand(planStep); cmd != "" {
				b.WriteString(fmt.Sprintf("   命令：`$ %s`\n", cmd))
			}
			if step.Error != "" {
				b.WriteString(fmt.Sprintf("   错误：%s\n", step.Error))
			}
			if step.Verification.Status != "" {
				b.WriteString(fmt.Sprintf("   验证：%s", step.Verification.Status))
				if step.Verification.Summary != "" {
					b.WriteString(fmt.Sprintf(" — %s", step.Verification.Summary))
				}
				b.WriteString("\n")
			}
			if step.OutputPreview != "" {
				b.WriteString("   输出：\n")
				b.WriteString(indentBlock(commandOperationDisplayPreview(step.OutputPreview, lang, step.PayloadRef != ""), "   "))
				b.WriteString("\n")
			}
			if step.PayloadRef != "" {
				b.WriteString(fmt.Sprintf("   完整输出：`%s`\n", step.PayloadRef))
			}
		}
		if len(result.StepResults) == 0 && strings.TrimSpace(result.OutputPreview) != "" {
			b.WriteString("说明：\n")
			b.WriteString(indentBlock(commandOperationDisplayPreview(result.OutputPreview, lang, result.PayloadRef != ""), ""))
			b.WriteString("\n")
		}
		return strings.TrimSpace(b.String())
	}
	var b strings.Builder
	switch result.Status {
	case operation.StatusExecuted:
		b.WriteString(fmt.Sprintf("Operation plan `%s` completed.\n\n", plan.ID))
	case operation.StatusComplete:
		b.WriteString(fmt.Sprintf("Operation plan `%s` determined that no further command is needed.\n\n", plan.ID))
	case operation.StatusBudgetExhausted:
		b.WriteString(fmt.Sprintf("Operation plan `%s` reached the operation budget.\n\n", plan.ID))
	case operation.StatusPartialAnswer:
		b.WriteString(fmt.Sprintf("Operation plan `%s` produced a partial result.\n\n", plan.ID))
	case operation.StatusCancelled:
		b.WriteString(fmt.Sprintf("Operation plan `%s` was cancelled.\n\n", plan.ID))
	default:
		b.WriteString(fmt.Sprintf("Operation plan `%s` failed.\n\n", plan.ID))
	}
	for i, step := range result.StepResults {
		planStep := commandOperationStepByID(plan, step.StepID)
		b.WriteString(fmt.Sprintf("%d. step `%s`: %s\n", i+1, step.StepID, step.Status))
		if cmd := commandOperationStepCommand(planStep); cmd != "" {
			b.WriteString(fmt.Sprintf("   Command: `$ %s`\n", cmd))
		}
		if step.Error != "" {
			b.WriteString(fmt.Sprintf("   Error: %s\n", step.Error))
		}
		if step.Verification.Status != "" {
			b.WriteString(fmt.Sprintf("   Verification: %s", step.Verification.Status))
			if step.Verification.Summary != "" {
				b.WriteString(fmt.Sprintf(" — %s", step.Verification.Summary))
			}
			b.WriteString("\n")
		}
		if step.OutputPreview != "" {
			b.WriteString("   Output:\n")
			b.WriteString(indentBlock(commandOperationDisplayPreview(step.OutputPreview, lang, step.PayloadRef != ""), "   "))
			b.WriteString("\n")
		}
		if step.PayloadRef != "" {
			b.WriteString(fmt.Sprintf("   Full output: `%s`\n", step.PayloadRef))
		}
	}
	if len(result.StepResults) == 0 && strings.TrimSpace(result.OutputPreview) != "" {
		b.WriteString("Note:\n")
		b.WriteString(indentBlock(commandOperationDisplayPreview(result.OutputPreview, lang, result.PayloadRef != ""), ""))
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func commandOperationRecordsMarkdown(lang string, records []commandOperationResultRecord) string {
	if len(records) == 0 {
		return ""
	}
	if len(records) == 1 {
		return commandOperationResultMarkdown(lang, records[0].Plan, records[0].Result)
	}
	var b strings.Builder
	if isZh(lang) {
		b.WriteString(fmt.Sprintf("操作任务分 %d 轮执行完成。\n\n", len(records)))
		for i, rec := range records {
			b.WriteString(fmt.Sprintf("### 第 %d 轮\n\n", i+1))
			b.WriteString(commandOperationResultMarkdown(lang, rec.Plan, rec.Result))
			if i < len(records)-1 {
				b.WriteString("\n\n")
			}
		}
		return strings.TrimSpace(b.String())
	}
	b.WriteString(fmt.Sprintf("Operation task completed in %d rounds.\n\n", len(records)))
	for i, rec := range records {
		b.WriteString(fmt.Sprintf("### Round %d\n\n", i+1))
		b.WriteString(commandOperationResultMarkdown(lang, rec.Plan, rec.Result))
		if i < len(records)-1 {
			b.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func commandOperationContinuationIntro(lang string, plan operation.CommandOperationPlan) string {
	if isZh(lang) {
		switch plan.Status {
		case operation.StatusReady:
			if plan.ApprovalMode == operation.ApprovalAutoLowRisk {
				return fmt.Sprintf("上一轮结果还不足以完整回答，已生成下一批操作计划 `%s`，将继续自动执行。", plan.ID)
			}
			return fmt.Sprintf("上一轮结果还不足以完整回答，已生成下一批操作计划 `%s`，需要批准后继续。", plan.ID)
		case operation.StatusNeedsClarification:
			return "上一轮结果显示还缺少关键信息，需要你补充后才能继续。"
		case operation.StatusBlocked:
			return "上一轮结果显示后续操作被策略阻止。"
		case operation.StatusComplete:
			return "上一轮结果已经足够回答，后续不再执行命令。"
		case operation.StatusBudgetExhausted:
			return "上一轮结果已达到操作预算，将基于已有结果作答。"
		case operation.StatusPartialAnswer:
			return "上一轮结果只能形成部分答案，将基于已有结果作答。"
		default:
			return "上一轮结果已交回规划器，生成了后续处理。"
		}
	}
	switch plan.Status {
	case operation.StatusReady:
		if plan.ApprovalMode == operation.ApprovalAutoLowRisk {
			return fmt.Sprintf("The previous round was not enough to answer fully, so Codrax generated the next operation plan `%s` and will continue automatically.", plan.ID)
		}
		return fmt.Sprintf("The previous round was not enough to answer fully, so Codrax generated the next operation plan `%s`; approval is required to continue.", plan.ID)
	case operation.StatusNeedsClarification:
		return "The previous round showed that key details are still missing; clarification is required before continuing."
	case operation.StatusBlocked:
		return "The previous round showed that the next operation is blocked by policy."
	case operation.StatusComplete:
		return "The previous round is sufficient to answer; no further command will run."
	case operation.StatusBudgetExhausted:
		return "The operation budget was reached; the answer will use the observations already collected."
	case operation.StatusPartialAnswer:
		return "Only a partial answer is possible; the answer will use the observations already collected."
	default:
		return "The previous round was returned to the planner and produced a follow-up action."
	}
}

func operationFinalReportWithDetails(lang, answer, details string) string {
	answer = strings.TrimSpace(answer)
	details = strings.TrimSpace(details)
	if answer == "" {
		return details
	}
	return answer
}

func commandOperationTerminalState(records []commandOperationResultRecord) (operation.OperationStatus, int) {
	if len(records) == 0 {
		return operation.StatusFailed, 0
	}
	return records[len(records)-1].Result.Status, len(records)
}

func commandOperationHasUncoveredMaterialRef(records []commandOperationResultRecord) bool {
	for i, record := range records {
		refs := commandOperationRecordPayloadRefs(record)
		if len(refs) == 0 {
			continue
		}
		later := commandOperationLaterCommandText(records, i+1)
		for _, ref := range refs {
			if commandPayloadHasMaterialExcerpt(ref) {
				continue
			}
			if ref != "" && !strings.Contains(later, ref) {
				return true
			}
		}
	}
	return false
}

func commandOperationRecordPayloadRefs(record commandOperationResultRecord) []string {
	seen := map[string]bool{}
	var refs []string
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			return
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	add(record.Result.PayloadRef)
	for _, step := range record.Result.StepResults {
		add(step.PayloadRef)
	}
	return refs
}

func commandOperationLaterCommandText(records []commandOperationResultRecord, start int) string {
	var b strings.Builder
	for i := start; i < len(records); i++ {
		for _, step := range records[i].Plan.Steps {
			b.WriteString(commandOperationStepCommand(step))
			b.WriteString("\n")
		}
	}
	return b.String()
}

func operationFinalReportWithRecordStatus(lang, answer string, records []commandOperationResultRecord) string {
	status, rounds := commandOperationTerminalState(records)
	answer = operationFinalReportWithStatus(lang, answer, status, rounds)
	if status != operation.StatusExecuted || !commandOperationHasUncoveredMaterialRef(records) {
		return answer
	}
	prefix := operationFinalMaterialCoveragePrefix(lang, rounds)
	if strings.TrimSpace(answer) == "" {
		return prefix
	}
	return prefix + "\n\n" + strings.TrimSpace(answer)
}

func operationFinalMaterialCoveragePrefix(lang string, rounds int) string {
	if isZh(lang) {
		return fmt.Sprintf("状态：已执行，材料覆盖未完全验证。存在已保存但未继续读取/提取的完整输出引用；以下报告基于已展示和已处理的观察，本轮共执行 %d 轮。", rounds)
	}
	return fmt.Sprintf("Status: executed, material coverage not fully verified. Some saved payload refs were not read or extracted in later steps; this report uses the displayed and processed observations across %d round(s).", rounds)
}

func operationFinalReportWithStatus(lang, answer string, status operation.OperationStatus, rounds int) string {
	answer = strings.TrimSpace(answer)
	prefix := operationFinalStatusPrefix(lang, status, rounds)
	if answer == "" {
		return operationFinalReportFallback(lang, status, rounds)
	}
	if prefix == "" {
		return answer
	}
	return prefix + "\n\n" + answer
}

func operationFinalStatusPrefix(lang string, status operation.OperationStatus, rounds int) string {
	if isZh(lang) {
		switch status {
		case operation.StatusBudgetExhausted:
			return fmt.Sprintf("状态：部分结果。操作已达到预算上限，以下报告仅基于已收集到的观察；本轮共执行 %d 轮。", rounds)
		case operation.StatusPartialAnswer:
			return fmt.Sprintf("状态：部分结果。当前只能基于已收集到的观察作答；本轮共执行 %d 轮。", rounds)
		case operation.StatusFailed:
			return fmt.Sprintf("状态：未完成。最新操作失败，以下报告只包含可用的部分观察；本轮共执行 %d 轮。", rounds)
		case operation.StatusBlocked:
			return fmt.Sprintf("状态：未执行/已阻止。后续操作被策略或能力边界阻止；本轮共执行 %d 轮。", rounds)
		case operation.StatusNeedsClarification:
			return "状态：需要补充信息。缺少用户侧关键信息前，系统不会继续猜测执行。"
		case operation.StatusCancelled:
			return "状态：已取消。"
		case operation.StatusRejected:
			return "状态：已拒绝。"
		default:
			return ""
		}
	}
	switch status {
	case operation.StatusBudgetExhausted:
		return fmt.Sprintf("Status: partial result. The operation budget was reached before the goal was fully satisfied; this report uses the observations collected across %d round(s).", rounds)
	case operation.StatusPartialAnswer:
		return fmt.Sprintf("Status: partial result. Only a partial answer is possible from the observations collected across %d round(s).", rounds)
	case operation.StatusFailed:
		return fmt.Sprintf("Status: not complete. The latest operation failed; this report includes only usable partial observations from %d round(s).", rounds)
	case operation.StatusBlocked:
		return fmt.Sprintf("Status: not executed/blocked. Further operation was blocked by policy or capability limits after %d round(s).", rounds)
	case operation.StatusNeedsClarification:
		return "Status: clarification needed. The system will not guess user-owned missing details."
	case operation.StatusCancelled:
		return "Status: cancelled."
	case operation.StatusRejected:
		return "Status: rejected."
	default:
		return ""
	}
}

func operationFinalReportFallback(lang string, status operation.OperationStatus, rounds int) string {
	if isZh(lang) {
		var b strings.Builder
		switch status {
		case operation.StatusExecuted:
			b.WriteString("操作已完成。")
		case operation.StatusCancelled:
			b.WriteString("操作已取消。")
		case operation.StatusBlocked:
			b.WriteString("操作已被策略阻止。")
		case operation.StatusComplete:
			b.WriteString("操作已完成。")
		case operation.StatusBudgetExhausted:
			b.WriteString("操作已达到预算上限。")
		case operation.StatusPartialAnswer:
			b.WriteString("操作已形成部分结果。")
		case operation.StatusNeedsClarification:
			b.WriteString("操作需要补充信息后才能继续。")
		default:
			b.WriteString("操作未完成。")
		}
		if rounds > 0 {
			b.WriteString(fmt.Sprintf("已完成 %d 轮操作，", rounds))
		}
		b.WriteString("每轮执行结果已在上方过程面板展示；最终报告生成未返回可用摘要。")
		return b.String()
	}
	var b strings.Builder
	switch status {
	case operation.StatusExecuted:
		b.WriteString("The operation completed.")
	case operation.StatusCancelled:
		b.WriteString("The operation was cancelled.")
	case operation.StatusBlocked:
		b.WriteString("The operation was blocked by policy.")
	case operation.StatusComplete:
		b.WriteString("The operation completed.")
	case operation.StatusBudgetExhausted:
		b.WriteString("The operation reached its budget.")
	case operation.StatusPartialAnswer:
		b.WriteString("The operation produced a partial result.")
	case operation.StatusNeedsClarification:
		b.WriteString("The operation needs clarification before it can continue.")
	default:
		b.WriteString("The operation did not complete.")
	}
	if rounds > 0 {
		b.WriteString(fmt.Sprintf(" %d operation round(s) completed.", rounds))
	}
	b.WriteString(" Per-round execution results are shown above; final report synthesis did not return a usable summary.")
	return b.String()
}

func operationThoughtsHeaderMsg(lang string, block, total int) string {
	if block <= 0 {
		block = 1
	}
	if total <= 0 {
		total = 1
	}
	if isZh(lang) {
		if total == 1 {
			return "模型思考（不进入答案）："
		}
		return fmt.Sprintf("模型思考（不进入答案）%d/%d：", block, total)
	}
	if total == 1 {
		return "Model thinking (not included in the answer):"
	}
	return fmt.Sprintf("Model thinking (not included in the answer) %d/%d:", block, total)
}

func splitVisibleThinkBlocks(text string) ([]string, string) {
	rest := text
	var thoughts []string
	var cleaned strings.Builder
	for {
		lower := strings.ToLower(rest)
		start := strings.Index(lower, "<think>")
		if start < 0 {
			cleaned.WriteString(rest)
			break
		}
		cleaned.WriteString(rest[:start])
		afterStart := start + len("<think>")
		endRel := strings.Index(lower[afterStart:], "</think>")
		if endRel < 0 {
			if thought := strings.TrimSpace(rest[afterStart:]); thought != "" {
				thoughts = append(thoughts, thought)
			}
			break
		}
		end := afterStart + endRel
		if thought := strings.TrimSpace(rest[afterStart:end]); thought != "" {
			thoughts = append(thoughts, thought)
		}
		rest = rest[end+len("</think>"):]
	}
	return thoughts, strings.TrimSpace(cleaned.String())
}

const (
	commandOperationDisplayPreviewRunes = 200
	commandOperationDisplayPreviewLines = 20
)

func commandOperationDisplayPreview(s, lang string, hasPayloadRefOpt ...bool) string {
	s = strings.TrimRight(s, "\n")
	hasPayloadRef := len(hasPayloadRefOpt) > 0 && hasPayloadRefOpt[0]
	lines := strings.Split(s, "\n")
	keptLines := lines
	truncatedByLine := false
	if len(lines) > commandOperationDisplayPreviewLines {
		keptLines = lines[:commandOperationDisplayPreviewLines]
		truncatedByLine = true
	}
	preview := strings.Join(keptLines, "\n")
	runes := []rune(preview)
	truncatedByChars := false
	if len(runes) > commandOperationDisplayPreviewRunes {
		preview = string(runes[:commandOperationDisplayPreviewRunes])
		truncatedByChars = true
	}
	if !truncatedByLine && !truncatedByChars {
		return preview
	}
	omitted := len([]rune(s)) - len([]rune(preview))
	if omitted < 0 {
		omitted = 0
	}
	suffix := fmt.Sprintf("\n...[panel preview truncated %d chars", omitted)
	if isZh(lang) {
		suffix = fmt.Sprintf("\n...[面板预览已截断 %d 字符", omitted)
	}
	if hasPayloadRef {
		if isZh(lang) {
			suffix += "；完整输出见下方引用]"
		} else {
			suffix += "; see full output ref below]"
		}
	} else if isZh(lang) {
		suffix += "]"
	} else {
		suffix += "]"
	}
	return preview + suffix
}

func dataTaskUnavailableMarkdown(lang string) string {
	if isZh(lang) {
		return "数据处理能力未配置。当前请求已识别为只读数据计算任务，但没有可用的数据任务规划器。"
	}
	return "Data processing is not configured. The request was classified as a read-only data calculation task, but no data task planner is available."
}

func dataTaskErrorMarkdown(lang, errText string) string {
	if isZh(lang) {
		return "数据处理未完成。\n\n原因：" + strings.TrimSpace(errText)
	}
	return "Data processing did not complete.\n\nReason: " + strings.TrimSpace(errText)
}

func dataTaskClarificationMarkdown(lang string, plan dataquery.TaskPlan) string {
	if len(plan.Questions) == 0 {
		return dataTaskErrorMarkdown(lang, "data task planner requested clarification without questions")
	}
	var b strings.Builder
	if isZh(lang) {
		b.WriteString("数据处理需要补充信息：\n")
	} else {
		b.WriteString("Data processing needs more information:\n")
	}
	for i, q := range plan.Questions {
		fmt.Fprintf(&b, "\n%d. %s\n", i+1, strings.TrimSpace(q.Question))
		if len(q.Suggestions) > 0 {
			if isZh(lang) {
				fmt.Fprintf(&b, "   建议：%s\n", strings.Join(q.Suggestions, " / "))
			} else {
				fmt.Fprintf(&b, "   Suggestions: %s\n", strings.Join(q.Suggestions, " / "))
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func dataTaskBlockedMarkdown(lang string, plan dataquery.TaskPlan) string {
	reason := strings.TrimSpace(plan.BlockReason)
	if reason == "" {
		reason = "data task planner blocked the request"
	}
	if isZh(lang) {
		return "数据处理已阻止。\n\n原因：" + reason
	}
	return "Data processing was blocked.\n\nReason: " + reason
}

func dataTaskEvaluationMarkdown(lang string, eval dataquery.Evaluation) string {
	reason := strings.TrimSpace(eval.Reason)
	if reason == "" {
		reason = "data workflow evaluation did not mark the task complete"
	}
	var b strings.Builder
	if isZh(lang) {
		switch eval.Status {
		case dataquery.EvalNeedsClarification:
			b.WriteString("数据处理需要补充信息。")
		case dataquery.EvalBlocked:
			b.WriteString("数据处理已阻止。")
		default:
			b.WriteString("数据处理未完成。")
		}
		fmt.Fprintf(&b, "\n\n原因：%s", reason)
		if len(eval.MissingInputs) > 0 {
			fmt.Fprintf(&b, "\n\n缺失输入：%s", strings.Join(eval.MissingInputs, " / "))
		}
		return strings.TrimSpace(b.String())
	}
	switch eval.Status {
	case dataquery.EvalNeedsClarification:
		b.WriteString("Data processing needs more information.")
	case dataquery.EvalBlocked:
		b.WriteString("Data processing was blocked.")
	default:
		b.WriteString("Data processing did not complete.")
	}
	fmt.Fprintf(&b, "\n\nReason: %s", reason)
	if len(eval.MissingInputs) > 0 {
		fmt.Fprintf(&b, "\n\nMissing inputs: %s", strings.Join(eval.MissingInputs, " / "))
	}
	return strings.TrimSpace(b.String())
}

func dataTaskAnswerMarkdown(lang string, result dataquery.Result) string {
	contract := result.OutputContract.Normalize()
	if !contract.ExplanationAllowed {
		return strings.TrimSpace(result.Answer)
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(result.Answer))
	if strings.TrimSpace(result.AuditSummary) != "" {
		if isZh(lang) {
			fmt.Fprintf(&b, "\n\n审计摘要：%s", strings.TrimSpace(result.AuditSummary))
		} else {
			fmt.Fprintf(&b, "\n\nAudit summary: %s", strings.TrimSpace(result.AuditSummary))
		}
	}
	if len(result.ContractWarnings) > 0 {
		if isZh(lang) {
			fmt.Fprintf(&b, "\n\n输出契约提示：%s", strings.Join(result.ContractWarnings, "; "))
		} else {
			fmt.Fprintf(&b, "\n\nOutput contract notes: %s", strings.Join(result.ContractWarnings, "; "))
		}
	}
	if len(result.Rows) > 0 {
		if isZh(lang) {
			fmt.Fprintf(&b, "\n\n决策记录：%d 条", len(result.Rows))
		} else {
			fmt.Fprintf(&b, "\n\nDecision records: %d", len(result.Rows))
		}
	}
	return strings.TrimSpace(b.String())
}

func commandOperationReplanIntro(lang string, plan operation.CommandOperationPlan) string {
	if isZh(lang) {
		switch plan.Status {
		case operation.StatusReady:
			if plan.ApprovalMode == operation.ApprovalAutoLowRisk {
				return "已根据失败输出生成可自动执行的修订计划，并将继续执行。"
			}
			return "已根据失败输出生成修订计划，等待批准。"
		case operation.StatusNeedsClarification:
			return "已根据失败输出尝试修订计划，但仍缺少关键信息。"
		case operation.StatusBlocked:
			return "已根据失败输出尝试修订计划，但新计划被策略阻止。"
		case operation.StatusComplete:
			return "已根据失败输出确认已有结果足够回答，不再继续执行。"
		case operation.StatusBudgetExhausted:
			return "已根据失败输出确认达到操作预算，将基于已有结果作答。"
		case operation.StatusPartialAnswer:
			return "已根据失败输出确认只能形成部分答案，将基于已有结果作答。"
		default:
			return "已根据失败输出尝试修订计划。"
		}
	}
	switch plan.Status {
	case operation.StatusReady:
		if plan.ApprovalMode == operation.ApprovalAutoLowRisk {
			return "Generated an auto-executable revised command plan from the failed output and will continue execution."
		}
		return "Generated a revised command plan from the failed output. It is waiting for approval."
	case operation.StatusNeedsClarification:
		return "Tried to revise the command plan from the failed output, but key details are still missing."
	case operation.StatusBlocked:
		return "Tried to revise the command plan from the failed output, but the new plan is blocked by policy."
	case operation.StatusComplete:
		return "The failed output still contains enough observations to answer, so no further command will run."
	case operation.StatusBudgetExhausted:
		return "The operation budget was reached; the answer will use the observations already collected."
	case operation.StatusPartialAnswer:
		return "Only a partial answer is possible; the answer will use the observations already collected."
	default:
		return "Tried to revise the command plan from the failed output."
	}
}

func indentBlock(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func localizedOperationStep(lang, title, detail string) (string, string) {
	if !isZh(lang) {
		return title, detail
	}
	switch title {
	case "gather source facts":
		title = "收集源码事实"
		detail = "先在源码证据通道收集仓库事实，再进入制品生成"
	case "outline deck":
		title = "规划演示结构"
		detail = "把请求材料整理为幻灯片章节和讲述结构"
	case "create slides":
		title = "生成幻灯片"
		detail = "生成本地演示文稿制品，并检查渲染版式"
	case "outline document":
		title = "规划文档结构"
		detail = "把请求材料整理为章节和论点"
	case "create document":
		title = "生成文档"
		detail = "生成本地文档制品，并检查页面渲染"
	case "shape workbook":
		title = "规划工作簿"
		detail = "确定工作表、表格、公式和图表"
	case "create spreadsheet":
		title = "生成表格"
		detail = "生成本地工作簿，并检查计算和渲染"
	case "open browser workflow":
		title = "打开浏览器流程"
		detail = "以可见、可取消的步骤操作浏览器界面"
	case "select external workflow":
		title = "选择外部工作流"
		detail = "按 typed capability 选择已配置的外部 skill/MCP 工作流"
	case "run workflow":
		title = "执行工作流"
		detail = "通过 provider 的有界接口执行，并返回制品或外部观测"
	case "plan operation":
		title = "规划操作"
	case "verify result":
		title = "验证结果"
		detail = "检查生成制品或界面状态后再报告完成"
	case "execution paused":
		title = "执行暂停"
	}
	return title, detail
}

// dockTerminalArmer is the narrow surface armDockTerminalState needs
// from the renderer. Defined as an interface so the helper can be
// tested without a real Renderer (the test file passes a stub),
// AND so a nil *render.Renderer never panics — typed-nil interface
// values flow through cleanly because both methods are nil-safe on
// the concrete type.
type dockTerminalArmer interface {
	MarkRunFatal()
	MarkRunCancelled()
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

// promptStickyTag returns compact bracketed markers the prompt renderer
// prepends so the user sees at a glance which sticky task lane and
// attachments are live for this turn:
//
//	[task:code]                 — sticky code/source-analysis lane
//	[task:op]                   — sticky computer-operation lane
//	[task:data]                 — sticky data-processing lane
//	[task:write]                — sticky write lane
//	[phase:apply]               — internal transient write phase, when useful
//	[log]                       — AttachedLog non-empty
//	[trace]                     — AttachedHitrace non-empty
//	[plan]                      — pendingPlanPath non-empty
//	[mem!]                      — memory under pressure (cleanup nudge)
//	[git:<branch>]              — current git branch in repoRoot
//	[git:detached@<sha7>]       — detached HEAD (rebase / cherry-pick
//	                              / explicit checkout of a SHA)
//
// Multiple markers concatenate without spaces:
// "[git:main][task:write][plan] ❯❯". Empty when nothing sticky.
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
func promptStickyTag(taskMode, pipelineMode, branch string, hasLog, hasTrace, hasPendingPlan, memPressure bool, focus []string) string {
	var b strings.Builder
	if branch != "" {
		b.WriteString("[git:")
		b.WriteString(branch)
		b.WriteString("]")
	}
	// 2026-05-08 add (Phase 3 multi-repo UX): when the operator has
	// pinned ≥1 sub-repo via /repos focus, surface the pin set in
	// the prompt so they can tell at a glance which workspace scope
	// the next request will run against. The signal is typed
	// (explicit operator action), not noisy (auto routing) — only
	// pinned sub-repos render here; the auto-active set fluctuates
	// per question and is left to the dock + /repos query.
	if tag := composeFocusTag(focus); tag != "" {
		b.WriteString(tag)
	}
	if tag := composeTaskTag(taskMode); tag != "" {
		b.WriteString(tag)
	}
	if tag := composePhaseTag(taskMode, pipelineMode); tag != "" {
		b.WriteString(tag)
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

func composeTaskTag(taskMode string) string {
	switch strings.ToLower(strings.TrimSpace(taskMode)) {
	case "", "auto":
		return ""
	case "code":
		return "[task:code]"
	case "operation":
		return "[task:op]"
	case "data":
		return "[task:data]"
	case "write":
		return "[task:write]"
	default:
		return "[task:" + strings.ToLower(strings.TrimSpace(taskMode)) + "]"
	}
}

func composePhaseTag(taskMode, pipelineMode string) string {
	phase := strings.ToLower(strings.TrimSpace(pipelineMode))
	if phase == "" || phase == "read" {
		return ""
	}
	task := strings.ToLower(strings.TrimSpace(taskMode))
	// Sticky write mode maps to the internal plan phase, but the user-facing
	// task marker is clearer and shorter than showing both task+phase.
	if task == "write" && phase == "plan" {
		return ""
	}
	return "[phase:" + phase + "]"
}

// composeFocusTag formats the [focus:...] sticky tag for the current
// pin set:
//
//	0 pinned                 → ""
//	1 pinned                 → "[focus:repo-a]"
//	2 pinned                 → "[focus:repo-a,repo-b]"
//	≥3 pinned                → "[focus:N pinned]"
//
// A consistent threshold prevents the tag from growing unbounded in
// session terminals when an operator pins 5+ sub-repos at once. The
// authoritative listing stays in `/repos` — the prompt tag is a
// reminder, not the canonical view.
func composeFocusTag(focus []string) string {
	switch len(focus) {
	case 0:
		return ""
	case 1:
		return "[focus:" + focus[0] + "]"
	case 2:
		return "[focus:" + focus[0] + "," + focus[1] + "]"
	default:
		return "[focus:" + itoa(len(focus)) + " pinned]"
	}
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
//
// 2026-05-08: dropped "/branch" — it is a universal git
// checkout helper (works in any mode, no write_enabled
// dependency). Keeping it under the write-mode header was
// misleading, especially because non-write commands like
// /cancel / /env / /repos / /mermaid had also been pulled
// under the header by the slashCommands order. The slice
// has been reordered so EVERY non-write command appears
// above the first write entry; this classifier therefore
// only needs to flag the genuinely write-mode commands.
func isWriteModeCommand(name string) bool {
	switch name {
	case "/write",
		"/plan",
		"/approve",
		"/reject",
		"/verify",
		"/worktree",
		"/merge",
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
		return formatN(lang, "%s 已禁用（%s）", cmd, reason)
	}
	return formatN(lang, "%s disabled (%s)", cmd, reason)
}

// noPlanStoreReason — common reason embedded in commandDisabled.
func noPlanStoreReason(lang string) string {
	if isZh(lang) {
		return "方案存储未配置"
	}
	return "plan store unavailable"
}

// unknownSlashCommand — printed when handleSlash falls through.
func unknownSlashCommand(lang, cmd string) string {
	if isZh(lang) {
		return formatN(lang, "未知命令 %q —— 输入 /help 查看完整列表\n", cmd)
	}
	return formatN(lang, "Unknown command %q — try /help\n", cmd)
}

// unknownPlanSubcommand — /plan <unknown>.
func unknownPlanSubcommand(lang, sub string) string {
	if isZh(lang) {
		return formatN(lang, "未知的 /plan 子命令 %q —— 应是 show / clear / list 之一\n", sub)
	}
	return formatN(lang, "Unknown /plan subcommand %q — expected: show, clear, list\n", sub)
}

// unknownModeValue — /mode <unknown>.
func unknownModeValue(lang, val string) string {
	if isZh(lang) {
		return formatN(lang, "未知模式 %q —— 应是 auto / code / operation / data / write 之一\n", val)
	}
	return formatN(lang, "Unknown mode %q — expected one of: auto, code, operation, data, write\n", val)
}

// planNotFound — /approve <id> / /plan show <id> with non-existent ID.
func planNotFound(lang, planID string) string {
	if isZh(lang) {
		return formatN(lang, "找不到 plan %q（可用 /plan list 查看现有 ID）\n", planID)
	}
	return formatN(lang, "Plan %q not found (try /plan list)\n", planID)
}

// noPendingPlanToClear — /plan clear with empty pendingPlanPath.
func noPendingPlanToClear(lang string) string {
	if isZh(lang) {
		return "当前没有可清除的 plan"
	}
	return "No pending plan to clear"
}

// noPlansInStore — /plan list on an empty PlanStore.
func noPlansInStore(lang, dir string) string {
	if isZh(lang) {
		return formatN(lang, "%s 里还没有 plan", dir)
	}
	return formatN(lang, "No plans saved in %s", dir)
}

// approveRefusedStatusMsg — re-approving a non-pending /
// non-verify_failed plan.
func approveRefusedStatusMsg(lang, planID, status, pending, retry string) string {
	if isZh(lang) {
		return formatN(lang,
			"approve 被拒：plan %s 当前状态为 %q。可重新 approve 的状态：%q（新生成的方案）、%q（修复环境后重试）。"+
				"用 /mode write 生成新方案。\n",
			planID, status, pending, retry)
	}
	return formatN(lang,
		"Approve refused: plan %s is in status %q. "+
			"Re-approvable statuses: %q (fresh plan), %q (env-fix retry). "+
			"Run /mode write to generate a fresh plan.\n",
		planID, status, pending, retry)
}

// approveStubRunnerMsg — runner doesn't implement modeSetter /
// planPathSetter. Hits in tests; surfaces fail-loud in real REPL.
func approveStubRunnerMsg(lang string) string {
	if isZh(lang) {
		return "/approve 需要 runner 支持 SetMode + SetPlanPath（检测到 stub runner）\n"
	}
	return "/approve requires a runner with SetMode + SetPlanPath (stub runner detected)\n"
}

// approveSkipVerifyStubMsg — runner doesn't implement
// skipVerifySetter (test stubs). Operator sees this only in
// scripted-test scenarios.
func approveSkipVerifyStubMsg(lang string) string {
	if isZh(lang) {
		return "--skip-verify 已请求，但 runner 未实现 SetSkipVerify（test stub?）；忽略\n"
	}
	return "--skip-verify requested but runner does not implement SetSkipVerify (test stub?); ignoring\n"
}

// approveBareDirNoAuthMsg — non-interactive REPL or scripted run
// with no --auto-init-repo when the target needs init.
func approveBareDirNoAuthMsg(lang, repoRoot, state string) string {
	if isZh(lang) {
		return formatN(lang,
			"approve：目标 %s 状态为 %s —— 请用 --auto-init-repo 重新运行，或在 codrax.yaml 设置 write_auto_init_repo: true\n",
			repoRoot, state)
	}
	return formatN(lang,
		"Approve: target %s is %s — re-run with --auto-init-repo, or set codrax.yaml :: write_auto_init_repo: true\n",
		repoRoot, state)
}

// mergeToIgnoredNoWorktreeMsg — --merge-to= without keep-on-success.
func mergeToIgnoredNoWorktreeMsg(lang, branch string) string {
	if isZh(lang) {
		return formatN(lang,
			"--merge-to=%s 已忽略：没有保留的 worktree（在 codrax.yaml 设置 pipeline_keep_worktree_on_success: true 即可）\n",
			branch)
	}
	return formatN(lang,
		"--merge-to=%s ignored: no worktree was preserved (set codrax.yaml :: pipeline_keep_worktree_on_success: true)\n",
		branch)
}

// mergeListPlansFailedMsg — PlanStore.List error in /merge.
func mergeListPlansFailedMsg(lang string, err error) string {
	if isZh(lang) {
		return formatN(lang, "merge：列出 plan 失败：%v\n", err)
	}
	return formatN(lang, "merge: list plans: %v\n", err)
}

// branchCheckoutFailedMsg — /branch <name> with bad name.
func branchCheckoutFailedMsg(lang string, err error) string {
	if isZh(lang) {
		return formatN(lang, "branch：git checkout 失败：%v\n", err)
	}
	return formatN(lang, "branch: git checkout failed: %v\n", err)
}

// noLogAttached / noTraceAttached / attachedLogCleared etc.
func noLogAttached(lang string) string {
	if isZh(lang) {
		return "未附加日志"
	}
	return "No log attached"
}

func noTraceAttached(lang string) string {
	if isZh(lang) {
		return "未附加 trace"
	}
	return "No hitrace attached"
}

func attachedLogClearedMsg(lang string) string {
	if isZh(lang) {
		return "已清除附加日志"
	}
	return "Attached log cleared"
}

func attachedTraceClearedMsg(lang string) string {
	if isZh(lang) {
		return "已清除附加 trace"
	}
	return "Attached hitrace cleared"
}

func htraceUsage(lang string) string {
	if isZh(lang) {
		return "/htrace <path> | append <path> | convert <binary> [out.systrace] | clear | show — 附加或转换 HiTrace / atrace / systrace / perfetto 文件"
	}
	return "/htrace <path> | append <path> | convert <binary> [out.systrace] | clear | show — attach or convert HiTrace / atrace / systrace / perfetto files"
}

func htraceConvertUsage(lang string) string {
	if isZh(lang) {
		return "/htrace convert <binary-hitrace> [output.systrace]\n将二进制 Harmony/OpenHarmony HiTrace 手动转换为文本 systrace；不会自动附加。省略输出路径时默认写 <input>.systrace；若文件已存在，请先删除或指定新输出路径。"
	}
	return "/htrace convert <binary-hitrace> [output.systrace]\nConvert a binary Harmony/OpenHarmony HiTrace file to text systrace; this does not attach the output automatically. When output is omitted, Codrax writes <input>.systrace; if it already exists, delete it first or choose another output path."
}

func htraceConvertSuccess(lang, outputPath string, events int) string {
	if isZh(lang) {
		return formatN(lang, "已转换 hitrace：%s（%d 个事件）", outputPath, events)
	}
	return formatN(lang, "converted hitrace: %s (%d events)", outputPath, events)
}

func htraceConvertCaveatMsg(lang, caveat string) string {
	if isZh(lang) {
		return formatN(lang, "hitrace 转换提示：%s\n", caveat)
	}
	return formatN(lang, "hitrace convert caveat: %s\n", caveat)
}

func htraceConvertFailedMsg(lang string, err error) string {
	if isZh(lang) {
		return formatN(lang, "hitrace 转换失败：%v", err)
	}
	return formatN(lang, "convert hitrace: %v", err)
}

func htraceConvertNextMsg(lang, outputPath string) string {
	if isZh(lang) {
		return formatN(lang, "下一步：/htrace %s", outputPath)
	}
	return formatN(lang, "next: /htrace %s", outputPath)
}

// pasteCapturePromptLog — /log paste capture mode prompt.
func pasteCapturePromptLog(lang string) string {
	if isZh(lang) {
		return "粘贴日志，单独一行输入 /end 结束捕获"
	}
	return "Paste the log; type /end on its own line to finish"
}

// pasteCapturePromptGeneric — /paste mode prompt.
func pasteCapturePromptGeneric(lang string) string {
	if isZh(lang) {
		return "粘贴内容，单独一行输入 /end 结束捕获；不输入直接按 Enter 即取消"
	}
	return "Paste the content; type /end on its own line to finish, or press Enter to cancel"
}

// pasteNoCapture — log/generic paste mode aborted with no input.
func pasteNoCaptureLog(lang string) string {
	if isZh(lang) {
		return "未捕获到内容；附加日志保持不变"
	}
	return "No input captured; the attached log is unchanged"
}

func pasteNoCaptureGeneric(lang string) string {
	if isZh(lang) {
		return "未捕获到内容"
	}
	return "No input captured"
}

// memoryClearCancelled / Cleared / EmptyCancelled.
func memoryClearCancelled(lang string) string {
	if isZh(lang) {
		return "已取消清除"
	}
	return "Clear cancelled"
}

func memoryClearedMsg(lang string) string {
	if isZh(lang) {
		return "已清除会话 memory、操作上下文和学习记忆。"
	}
	return "Conversation memory, operation context, and learned operation memory cleared."
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
		return "Ctrl+C 取消（连按 2 次强制退出）"
	}
	return "Ctrl+C to cancel (double-tap within 2s to force-exit)"
}

// cancelInProgressMsg — surfaced after the user presses Ctrl+C (or
// types /cancel) while a Run is in flight. Tells them the cancel has
// been requested but lands at the next pipeline checkpoint, so the
// "spinner stuck for ~30s" surprise is preempted.
func cancelInProgressMsg(lang string) string {
	if isZh(lang) {
		return "✗ 取消已请求，正在中断当前 LLM 请求。若仍无响应，再按一次 Ctrl+C 可强制退出。"
	}
	return "✗ Cancel requested; interrupting the current LLM request. Press Ctrl+C again to force exit if it still does not respond."
}

// idleConfirmExitMsg — first Ctrl+C at the idle prompt (no Run in
// flight). Pre-2026-05-02 the idle path immediately os.Exit'd on
// the first signal, contradicting the dock's "连按 2 次强制退出"
// promise and trapping operators who hit Ctrl+C expecting the
// readline-conventional "abandon current input line, stay at
// prompt" semantic. Now the idle path matches the in-Run path:
// first tap warns + asks for confirmation, second tap within 2 s
// exits. Operators can press once more to leave OR continue using
// the REPL — Ctrl+C at the prompt is no longer a one-way trap.
func idleConfirmExitMsg(lang string) string {
	if isZh(lang) {
		return "✗ 2 秒内再按一次 Ctrl+C 即退出 codrax；或继续使用 REPL。"
	}
	return "✗ Press Ctrl+C again within 2s to exit codrax, or keep using the REPL."
}

// memoryClearConfirmTitle renders the /clear confirmation prompt
// (the huh.Confirm Title or the line-mode print line). Bilingual +
// peer-count aware so a multi-instance operator is warned that the
// wipe affects sibling REPLs sharing the same memory dir.
//
// Pre-2026-05-02 the title was a hardcoded English string and the
// peer-count addendum was English-only — operators running in zh
// locale saw mixed-language UI.
func memoryClearConfirmTitle(lang string, peers int) string {
	if isZh(lang) {
		base := "/clear 将清空当前会话记忆(MEMORY.md + turns/)、操作上下文和学习记忆。"
		switch {
		case peers == 1:
			return base + " 当前另有 1 个 codrax 实例共享此目录。"
		case peers > 1:
			return base + fmt.Sprintf(" 当前另有 %d 个 codrax 实例共享此目录。", peers)
		}
		return base
	}
	base := "/clear wipes this conversation memory (MEMORY.md + turns/), operation context, and learned operation memory."
	switch {
	case peers == 1:
		return base + " 1 other live codrax instance shares this directory."
	case peers > 1:
		return base + fmt.Sprintf(" %d other live codrax instances share this directory.", peers)
	}
	return base
}

// memoryClearConfirmAffirmative renders the affirmative button label
// for the /clear confirm widget. The leading "[Y]" / "[是]" exposes
// the keyboard hotkey huh binds by default (first alphanumeric
// character of the label), so operators see the shortcut without
// reading docs.
func memoryClearConfirmAffirmative(lang string) string {
	if isZh(lang) {
		return "Y - 是,清空"
	}
	return "Y - Yes, wipe"
}

// memoryClearConfirmNegative — same as Affirmative but for the No
// option. Hotkey is "n" via the leading character.
func memoryClearConfirmNegative(lang string) string {
	if isZh(lang) {
		return "N - 否,取消"
	}
	return "N - No, keep"
}

// memoryClearConfirmLineHint renders the y/n hint for the
// non-interactive (scripted / piped stdin) line-oriented mode. The
// huh.Confirm widget is replaced by a plain readline so operators
// need an explicit "type y or n" hint.
func memoryClearConfirmLineHint(lang string) string {
	if isZh(lang) {
		return "  输入 y 确认清空,其它任意输入取消:"
	}
	return "  Type 'y' to confirm wipe, anything else to cancel:"
}

// cancelNothingRunningMsg — `/cancel` typed at the idle prompt or
// when the runner doesn't expose a cancel surface (test stub). One
// line, no exit — operators can still /exit themselves.
func cancelNothingRunningMsg(lang string) string {
	if isZh(lang) {
		return "没有正在执行的请求可取消。/exit 退出 codrax。"
	}
	return "No request in flight to cancel. /exit to leave."
}

// canceledByUserMsg — final user-facing summary line printed in the
// approve / pipeline result rendering when the Run unwound via
// CanceledError. Stage label is the most-specific stage observed
// (dispatchStage emits it; agent_loop fallback otherwise). Empty
// stage hides the parenthetical.
func canceledByUserMsg(lang, stage string) string {
	if isZh(lang) {
		if stage != "" {
			return fmt.Sprintf("✗ 已取消（在 %s 阶段中断）。worktree（若有）已保留 — /worktree show 查看。", stage)
		}
		return "✗ 已取消。worktree（若有）已保留 — /worktree show 查看。"
	}
	if stage != "" {
		return fmt.Sprintf("✗ Cancelled at the %s stage. Worktree (if any) preserved — /worktree show.", stage)
	}
	return "✗ Cancelled. Worktree (if any) preserved — /worktree show."
}
