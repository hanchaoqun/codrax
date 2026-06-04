package repl

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func normalizeREPLWriteApprovalPolicy(policy writeflow.ApprovalPolicy) writeflow.ApprovalPolicy {
	if policy == "" {
		return writeflow.ApprovalPolicyAutoSafe
	}
	return writeflow.NormalizeApprovalPolicy(policy)
}

func renderWriteRiskAssessment(lang string, plan *types.ChangePlan, policy writeflow.ApprovalPolicy) []string {
	assessment := writeflow.AssessWriteRisk(writeflow.AssessmentInput{Plan: plan})
	policy = normalizeREPLWriteApprovalPolicy(policy)
	decision := writeflow.DecideWriteApproval(policy, assessment)
	zh := isZh(lang)
	lines := make([]string, 0, 6)
	if zh {
		lines = append(lines, fmt.Sprintf("\n  · 写入风险：%s", assessment.Level))
		lines = append(lines, fmt.Sprintf("    审批预览：%s => %s", policy, decision.Action))
	} else {
		lines = append(lines, fmt.Sprintf("\n  · write risk: %s", assessment.Level))
		lines = append(lines, fmt.Sprintf("    approval preview: %s => %s", policy, decision.Action))
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
