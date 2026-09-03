package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// renderVerifyReportWorktreeAuditNote is the nil-safe report-level form of
// renderVerificationWorktreeAuditNote — the ONE predicate all three verify
// outcomes (success / failure / unverified) render, so a refused run and an
// unavailable verdict disclose the same audit facts a passing run does.
func renderVerifyReportWorktreeAuditNote(report *types.ChangeReport, zh bool) string {
	if report == nil {
		return ""
	}
	return renderVerificationWorktreeAuditNote(report.WorktreeAudit, zh)
}

// renderVerificationWorktreeAuditNote keeps test success and retained-tree
// cleanliness as separate user-visible facts. It renders only typed audit rows;
// runner output and model prose never participate. Three lanes, each read
// from typed rows independently of the audit status: the disclosed tracked
// rows (lockfile refresh / formatter fixed point / declared output, with an
// UNPROVEN lockfile fixed point named in plain words), the retained
// untracked outputs, and the unavailable audit. Refused rows never render
// here — the failure surface owns them.
func renderVerificationWorktreeAuditNote(audit *types.VerificationWorktreeAudit, zh bool) string {
	if audit == nil || audit.Status == types.VerificationWorktreeAuditClean {
		return ""
	}
	// The untracked lane renders independently of the tracked lane's
	// disposition (shared types predicate): a refused run names the outputs
	// it left behind exactly like a disclosed or untracked-only run.
	paths := make([]string, 0, 8)
	for _, path := range audit.UntrackedRetainedPaths(8) {
		paths = append(paths, "`"+path+"`")
	}
	untrackedNote := ""
	if len(paths) > 0 {
		suffix := ""
		if audit.UntrackedEffectCount > len(paths) {
			suffix = fmt.Sprintf("（另有 %d 个）", audit.UntrackedEffectCount-len(paths))
			if !zh {
				suffix = fmt.Sprintf(" (+%d more)", audit.UntrackedEffectCount-len(paths))
			}
		}
		if zh {
			untrackedNote = "\n\n⚠ 本次验证在保留 worktree 中新增了未跟踪文件：" + strings.Join(paths, "、") + suffix +
				"。它们未纳入交付提交，也未被自动删除；交付 ref 与保留 worktree 的洁净状态是两个独立事实。"
		} else {
			untrackedNote = "\n\n⚠ Verification created untracked file(s) in the preserved worktree: " + strings.Join(paths, ", ") + suffix +
				". They are not part of the delivery commit and were not auto-deleted; delivery-ref integrity and preserved-worktree cleanliness are separate facts."
		}
	}
	if audit.Status == types.VerificationWorktreeAuditUnavailable {
		if zh {
			return "\n\n⚠ 未能完成验证前后的 worktree 完整性审计，不能宣称保留 worktree 洁净。"
		}
		return "\n\n⚠ The before/after worktree integrity audit was unavailable, so preserved-worktree cleanliness is not proven."
	}
	// V5-2: disclosed side effects (lockfile refresh / formatter fixed point
	// / plan-declared generated output) are named: not part of the delivery
	// commit, not auto-reverted. A lockfile row whose fixed point is
	// UNPROVEN says so in plain words (single-sourced phrase). The rows are
	// read from their typed disposition, so a REFUSED run (tracked_drift for
	// other paths) still names its disclosed rows — with the refused
	// wording, never "the verdict stands".
	rows := make([]string, 0, len(audit.Effects))
	for _, effect := range audit.Effects {
		if effect.Disposition != types.VerificationWorktreeEffectDisclosed || strings.TrimSpace(effect.Path) == "" {
			continue
		}
		row := "`" + effect.Path + "` (" + string(effect.DriftClass) + ")"
		if zh {
			row = "`" + effect.Path + "`（" + string(effect.DriftClass) + "）"
		}
		if phrase := types.VerificationLockfileFixedPointDisclosure(effect.LockfileFixedPoint, zh); phrase != "" {
			if zh {
				row += "，" + phrase
			} else {
				row += " — " + phrase
			}
		}
		rows = append(rows, row)
		if len(rows) >= 8 {
			break
		}
	}
	if len(rows) == 0 {
		return untrackedNote
	}
	refused := audit.Status == types.VerificationWorktreeAuditTrackedDrift
	if zh {
		verdict := "验证结论保持有效；"
		if refused {
			verdict = "验证结论已因其他路径的改动被拒绝；"
		}
		return "\n\n⚠ 本次验证在保留 worktree 中改动了受跟踪文件，属已披露的副作用类别：" + strings.Join(rows, "、") +
			"。" + verdict + "这些改动未纳入交付提交，也未被自动回退。" + untrackedNote
	}
	verdict := "The verdict stands; "
	if refused {
		verdict = "The verdict was refused for other paths; "
	}
	return "\n\n⚠ Verification changed tracked file(s) in the preserved worktree under a disclosed side-effect class: " + strings.Join(rows, ", ") +
		". " + verdict + "these changes are not part of the delivery commit and were not auto-reverted." + untrackedNote
}
