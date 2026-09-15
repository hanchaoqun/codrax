package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// renderChangePlanSummary formats a ChangePlan as a human-readable
// multi-line string suitable for the REPL or single-shot stdout.
// Day 5 ships a deliberately simple format (markdown-adjacent) —
// richer rendering can land later if user feedback demands it.
// Kept internal to orchestrator because no downstream package
// consumes the rendered form.
func renderChangePlanSummary(plan *types.ChangePlan, lang string) string {
	if plan == nil {
		return ""
	}
	zh := isLangZh(lang)
	var b strings.Builder
	if zh {
		fmt.Fprintf(&b, "## 提议的 ChangePlan:%s\n\n", plan.ID)
		if plan.Request != "" {
			fmt.Fprintf(&b, "**请求**:%s\n\n", plan.Request)
		}
		if plan.Summary != "" {
			fmt.Fprintf(&b, "**摘要**:%s\n\n", plan.Summary)
		}
		if len(plan.Changes) > 0 {
			b.WriteString("**变更列表**:\n\n")
			for i, c := range plan.Changes {
				fmt.Fprintf(&b, "%d. **%s** (`%s`) — %s\n", i+1, c.Kind, c.Path, c.Rationale)
			}
			b.WriteString("\n")
		}
		if len(plan.TargetPaths) > 0 {
			fmt.Fprintf(&b, "**目标路径**:%d 个文件 — %s\n\n",
				len(plan.TargetPaths), strings.Join(plan.TargetPaths, ", "))
		}
		if len(plan.AcceptanceTests) > 0 {
			b.WriteString("**验收测试**:\n\n")
			for _, t := range plan.AcceptanceTests {
				fmt.Fprintf(&b, "- %s\n", t)
			}
			b.WriteString("\n")
		}
		return b.String()
	}
	fmt.Fprintf(&b, "## Proposed change plan: %s\n\n", plan.ID)
	if plan.Request != "" {
		fmt.Fprintf(&b, "**Request**: %s\n\n", plan.Request)
	}
	if plan.Summary != "" {
		fmt.Fprintf(&b, "**Summary**: %s\n\n", plan.Summary)
	}
	if len(plan.Changes) > 0 {
		b.WriteString("**Changes**:\n\n")
		for i, c := range plan.Changes {
			fmt.Fprintf(&b, "%d. **%s** (`%s`) — %s\n", i+1, c.Kind, c.Path, c.Rationale)
		}
		b.WriteString("\n")
	}
	if len(plan.TargetPaths) > 0 {
		fmt.Fprintf(&b, "**Target paths**: %d file(s) — %s\n\n",
			len(plan.TargetPaths), strings.Join(plan.TargetPaths, ", "))
	}
	if len(plan.AcceptanceTests) > 0 {
		b.WriteString("**Acceptance tests**:\n\n")
		for _, t := range plan.AcceptanceTests {
			fmt.Fprintf(&b, "- %s\n", t)
		}
		b.WriteString("\n")
	}
	// Next-step line. The panel previously ended at the change list,
	// leaving the operator to guess the approval verbs (observed live:
	// a fresh user generated a plan and stalled — nothing on screen
	// said /approve). Keep it one line, verbs only.
	if zh {
		b.WriteString("**下一步**:`/plan show` 查看完整计划 · `/approve` 批准并执行 · `/reject [原因]` 退回重做\n")
	} else {
		b.WriteString("**Next**: `/plan show` to review · `/approve` to execute · `/reject [reason]` to send back\n")
	}
	return b.String()
}
