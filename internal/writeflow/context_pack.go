package writeflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ContextPackFromRiskAssessment adapts writeflow's deterministic risk/approval
// verdict into the cross-stage write context pack without importing writeflow
// into types. It is a typed projection only; approval enforcement still reads
// RiskAssessment and ApprovalDecision directly.
func ContextPackFromRiskAssessment(batchID, goal string, assessment RiskAssessment, decision ApprovalDecision) types.WriteContextPack {
	pack := types.WriteContextPack{
		PackID:      "risk-assessment",
		BatchID:     strings.TrimSpace(batchID),
		Goal:        strings.TrimSpace(goal),
		SourceStage: "risk",
	}
	if assessment.Level != "" && assessment.Level != RiskNone {
		pack.Items = append(pack.Items, riskContextItem("risk_assessment", riskContextPriority(assessment.Level), string(assessment.Level), "risk"))
	}
	if decision.Action != "" {
		text := fmt.Sprintf("policy=%s action=%s reason=%s", decision.Policy, decision.Action, decision.ReasonCode)
		pack.Items = append(pack.Items, riskContextItem("approval_decision", types.WriteContextP0, text, "risk"))
	}
	for _, reason := range assessment.TopReasons(8) {
		text := reason.Detail
		if reason.Path != "" {
			text = strings.TrimSpace(text + " path=" + reason.Path)
		}
		if text == "" {
			text = reason.Code
		}
		pack.Items = append(pack.Items, riskContextItem("risk_reason", riskContextPriority(reason.Level), text, "risk"))
	}
	return types.NormalizeWriteContextPack(pack)
}

func riskContextItem(kind string, priority types.WriteContextPriority, text, source string) types.WriteContextItem {
	return types.WriteContextItem{
		Priority:    priority,
		Kind:        kind,
		Text:        text,
		SourceStage: source,
		Consumers: []types.WriteContextConsumer{
			types.WriteConsumerController,
			types.WriteConsumerPlanner,
			types.WriteConsumerVerifier,
			types.WriteConsumerApproval,
		},
	}
}

func riskContextPriority(level RiskLevel) types.WriteContextPriority {
	switch level {
	case RiskCritical, RiskHigh:
		return types.WriteContextP0
	case RiskMedium:
		return types.WriteContextP1
	default:
		return types.WriteContextP2
	}
}
