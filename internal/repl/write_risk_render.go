package repl

import (
	"fmt"
	"strings"

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
		lines = append(lines, fmt.Sprintf("\n  · 计划变更风险：%s", assessment.Level))
		lines = append(lines, fmt.Sprintf("    审批预览：%s => %s", policy, decision.Action))
		if line := renderWriteAnalysisRiskAdvisory(lang, plan); line != "" {
			lines = append(lines, line)
		}
	} else {
		lines = append(lines, fmt.Sprintf("\n  · planned-change risk: %s", assessment.Level))
		lines = append(lines, fmt.Sprintf("    approval preview: %s => %s", policy, decision.Action))
		if line := renderWriteAnalysisRiskAdvisory(lang, plan); line != "" {
			lines = append(lines, line)
		}
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

func renderWriteAnalysisRiskAdvisory(lang string, plan *types.ChangePlan) string {
	if plan == nil || plan.WriteAnalysisIR == nil {
		return ""
	}
	risk := plan.WriteAnalysisIR.Request.Risk
	overall := strings.TrimSpace(string(risk.Overall))
	if overall == "" {
		overall = "unspecified"
	}
	axes := make([]string, 0, 3)
	if risk.AffectsPublicAPI {
		axes = append(axes, "public_api")
	}
	if risk.ChangesPersistence {
		axes = append(axes, "persistence")
	}
	if risk.ChangesBuildSystem {
		axes = append(axes, "build_system")
	}
	axisText := "none"
	if len(axes) > 0 {
		axisText = strings.Join(axes, ",")
	}
	if isZh(lang) {
		return fmt.Sprintf("    分析阶段风险提示：overall=%s axes=%s（仅提示，不作为审批硬门）", overall, axisText)
	}
	return fmt.Sprintf("    analysis risk advisory: overall=%s axes=%s (advisory only, not the approval gate)", overall, axisText)
}
