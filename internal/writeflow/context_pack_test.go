package writeflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestContextPackFromRiskAssessmentProjectsApprovalAndReasons(t *testing.T) {
	assessment := RiskAssessment{
		Level: RiskHigh,
		Reasons: []RiskReason{{
			Level:  RiskHigh,
			Code:   "build_system_path",
			Detail: "planned path changes build or dependency configuration",
			Path:   "go.mod",
		}},
	}
	decision := ApprovalDecision{
		Action:     ApprovalActionManual,
		Policy:     ApprovalPolicyAutoSafe,
		ReasonCode: "high_risk_manual",
	}
	pack := ContextPackFromRiskAssessment("batch-1", "update dependency", assessment, decision)
	view := pack.View(types.WriteConsumerApproval, 10)
	for _, want := range []struct {
		kind string
		text string
	}{
		{"risk_assessment", "high"},
		{"approval_decision", "high_risk_manual"},
		{"risk_reason", "go.mod"},
	} {
		if !riskContextViewContains(view, want.kind, want.text) {
			t.Fatalf("missing %s/%q from risk context: %+v", want.kind, want.text, view.Items)
		}
	}
	if view.Items[0].Priority != types.WriteContextP0 {
		t.Fatalf("high risk should render as p0, got %+v", view.Items[0])
	}
}

func riskContextViewContains(view types.WriteContextView, kind, substring string) bool {
	for _, item := range view.Items {
		if item.Kind == kind && strings.Contains(item.Text, substring) {
			return true
		}
	}
	return false
}
