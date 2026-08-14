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
	paths := make([]string, 0, len(audit.Effects))
	for _, effect := range audit.Effects {
		if effect.Kind != types.VerificationWorktreeEffectUntrackedCreated || strings.TrimSpace(effect.Path) == "" {
			continue
		}
		paths = append(paths, "`"+effect.Path+"`")
		if len(paths) >= 8 {
			break
		}
	}
	if audit.Status == types.VerificationWorktreeAuditUntrackedSideEffects && len(paths) > 0 {
		suffix := ""
		if audit.UntrackedEffectCount > len(paths) {
			suffix = fmt.Sprintf("（另有 %d 个）", audit.UntrackedEffectCount-len(paths))
			if !zh {
				suffix = fmt.Sprintf(" (+%d more)", audit.UntrackedEffectCount-len(paths))
			}
		}
		if zh {
			return "\n\n⚠ 本次验证在保留 worktree 中新增了未跟踪文件：" + strings.Join(paths, "、") + suffix +
				"。它们未纳入交付提交，也未被自动删除；交付 ref 与保留 worktree 的洁净状态是两个独立事实。"
		}
		return "\n\n⚠ Verification created untracked file(s) in the preserved worktree: " + strings.Join(paths, ", ") + suffix +
			". They are not part of the delivery commit and were not auto-deleted; delivery-ref integrity and preserved-worktree cleanliness are separate facts."
	}
	if audit.Status == types.VerificationWorktreeAuditUnavailable {
		if zh {
			return "\n\n⚠ 未能完成验证前后的 worktree 完整性审计，不能宣称保留 worktree 洁净。"
		}
		return "\n\n⚠ The before/after worktree integrity audit was unavailable, so preserved-worktree cleanliness is not proven."
	}
	return ""
}
