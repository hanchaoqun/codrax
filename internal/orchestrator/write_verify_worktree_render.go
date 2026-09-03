package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// renderVerificationWorktreeAuditNote keeps test success and retained-tree
// cleanliness as separate user-visible facts. It renders only typed audit rows;
// runner output and model prose never participate.
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
		if audit.Status == types.VerificationWorktreeAuditUntrackedSideEffects {
			return untrackedNote
		}
	}
	if audit.Status == types.VerificationWorktreeAuditUnavailable {
		if zh {
			return "\n\n⚠ 未能完成验证前后的 worktree 完整性审计，不能宣称保留 worktree 洁净。"
		}
		return "\n\n⚠ The before/after worktree integrity audit was unavailable, so preserved-worktree cleanliness is not proven."
	}
	if audit.Status == types.VerificationWorktreeAuditTrackedDrift {
		// Refused drift is owned by the failure surface; only the untracked
		// lane (if any) is rendered here.
		return untrackedNote
	}
	if audit.Status == types.VerificationWorktreeAuditTrackedDriftDisclosed {
		// V5-2: disclosed side effects (lockfile refresh / formatter fixed
		// point / plan-declared generated output) keep the verdict but are
		// named: not part of the delivery commit, not auto-reverted. A
		// lockfile row whose fixed point is UNPROVEN says so in plain words
		// (single-sourced phrase).
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
		if zh {
			return "\n\n⚠ 本次验证在保留 worktree 中改动了受跟踪文件，属已披露的副作用类别：" + strings.Join(rows, "、") +
				"。验证结论保持有效；这些改动未纳入交付提交，也未被自动回退。" + untrackedNote
		}
		return "\n\n⚠ Verification changed tracked file(s) in the preserved worktree under a disclosed side-effect class: " + strings.Join(rows, ", ") +
			". The verdict stands; these changes are not part of the delivery commit and were not auto-reverted." + untrackedNote
	}
	return ""
}
