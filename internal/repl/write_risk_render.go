package repl

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func renderWriteRiskAssessment(lang string, plan *types.ChangePlan) []string {
	assessment := writeflow.AssessWriteRisk(writeflow.AssessmentInput{Plan: plan})
	decision := writeflow.DecideWriteApproval(writeflow.ApprovalPolicyAutoSafe, assessment)
	zh := isZh(lang)
	lines := make([]string, 0, 6)
	if zh {
		lines = append(lines, fmt.Sprintf("\n  · 写入风险：%s", assessment.Level))
		lines = append(lines, fmt.Sprintf("    审批预览：auto_safe => %s（仅展示；当前 apply 行为未改变）", decision.Action))
	} else {
		lines = append(lines, fmt.Sprintf("\n  · write risk: %s", assessment.Level))
		lines = append(lines, fmt.Sprintf("    approval preview: auto_safe => %s (display only; apply behavior unchanged)", decision.Action))
	}
	reasons := assessment.TopReasons(4)
	for _, reason := range reasons {
		if reason.Path != "" {
			lines = append(lines, fmt.Sprintf("    - %s/%s: %s (%s)", reason.Level, reason.Code, reason.Detail, reason.Path))
			continue
		}
		lines = append(lines, fmt.Sprintf("    - %s/%s: %s", reason.Level, reason.Code, reason.Detail))
	}
	return lines
}
