package repl

import (
	"fmt"
	"strings"
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

// noPendingPlan — user typed /approve or /reject without a pending
// plan to act on. Surface the recovery path (/mode plan first).
func noPendingPlan(lang string) string {
	if isZh(lang) {
		return "没有待处理的 plan — 先 /mode plan 生成一份"
	}
	return "no pending plan — run a /mode plan dispatch to generate one"
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
//
// Multiple markers concatenate without spaces:
// "[mode:plan][log][plan] ❯❯". Empty when nothing sticky.
//
// All five markers are language-agnostic (zh and en both use the
// same bracketed labels) — they're terminal chrome, optimised for
// scan-ability, not for translation. The full localized hint when
// the user types /help or hits a memory-pressure threshold goes
// through the normal localized helpers.
func promptStickyTag(mode string, hasLog, hasTrace, hasPendingPlan, memPressure bool) string {
	var b strings.Builder
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
	// Width of widest command name for column alignment.
	maxName := 0
	for _, c := range slashCommands {
		if n := len(c.Name); n > maxName {
			maxName = n
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
		out = append(out, "可用命令(共 "+itoa(len(slashCommands))+" 条):")
	} else {
		out = append(out, "available commands ("+itoa(len(slashCommands))+"):")
	}
	for _, c := range slashCommands {
		out = append(out, "  "+pad(c.Name)+"  "+c.Help(lang))
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
