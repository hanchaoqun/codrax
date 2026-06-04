package writeflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAssessWriteRiskDocsOnlyLow(t *testing.T) {
	plan := planWithChanges(types.FileChange{Path: "docs/user_guide.md", Kind: "modify"})

	got := AssessWriteRisk(AssessmentInput{Plan: plan})
	if got.Level != RiskLow {
		t.Fatalf("risk = %s; want %s; reasons=%+v", got.Level, RiskLow, got.Reasons)
	}
	decision := DecideWriteApproval(ApprovalPolicyAutoSafe, got)
	if decision.Action != ApprovalActionAutoExecute {
		t.Fatalf("approval = %s; want %s", decision.Action, ApprovalActionAutoExecute)
	}
}

func TestAssessWriteRiskSourceMedium(t *testing.T) {
	plan := planWithChanges(types.FileChange{Path: "internal/foo/bar.go", Kind: "modify"})

	got := AssessWriteRisk(AssessmentInput{Plan: plan})
	if got.Level != RiskMedium {
		t.Fatalf("risk = %s; want %s; reasons=%+v", got.Level, RiskMedium, got.Reasons)
	}
	if decision := DecideWriteApproval(ApprovalPolicyAutoLowOnly, got); decision.Action != ApprovalActionManual {
		t.Fatalf("auto_low_only approval = %s; want %s", decision.Action, ApprovalActionManual)
	}
}

func TestAssessWriteRiskBuildManifestHigh(t *testing.T) {
	plan := planWithChanges(types.FileChange{Path: "go.mod", Kind: "modify"})

	got := AssessWriteRisk(AssessmentInput{Plan: plan})
	if got.Level != RiskHigh {
		t.Fatalf("risk = %s; want %s; reasons=%+v", got.Level, RiskHigh, got.Reasons)
	}
	if decision := DecideWriteApproval(ApprovalPolicyAutoSafe, got); decision.Action != ApprovalActionManual {
		t.Fatalf("auto_safe approval = %s; want %s", decision.Action, ApprovalActionManual)
	}
}

func TestAssessWriteRiskAnalysisAxesHigh(t *testing.T) {
	plan := planWithChanges(types.FileChange{Path: "internal/foo/bar.go", Kind: "modify"})
	plan.WriteAnalysisIR = &types.WriteAnalysisIR{}
	plan.WriteAnalysisIR.Request.Risk.AffectsPublicAPI = true

	got := AssessWriteRisk(AssessmentInput{Plan: plan})
	if got.Level != RiskHigh {
		t.Fatalf("risk = %s; want %s; reasons=%+v", got.Level, RiskHigh, got.Reasons)
	}
}

func TestAssessWriteRiskRepoEscapeCritical(t *testing.T) {
	for _, p := range []string{"../outside.go", "/tmp/outside.go", "C:\\tmp\\outside.go", ".git/config"} {
		t.Run(p, func(t *testing.T) {
			plan := planWithChanges(types.FileChange{Path: p, Kind: "modify"})

			got := AssessWriteRisk(AssessmentInput{Plan: plan})
			if got.Level != RiskCritical {
				t.Fatalf("risk = %s; want %s; reasons=%+v", got.Level, RiskCritical, got.Reasons)
			}
			if decision := DecideWriteApproval(ApprovalPolicyAutoSafe, got); decision.Action != ApprovalActionDeny {
				t.Fatalf("approval = %s; want %s", decision.Action, ApprovalActionDeny)
			}
		})
	}
}

func TestDecideWriteApprovalManualAlwaysManualUnlessCritical(t *testing.T) {
	decision := DecideWriteApproval(ApprovalPolicyManual, RiskAssessment{Level: RiskLow})
	if decision.Action != ApprovalActionManual {
		t.Fatalf("manual low approval = %s; want %s", decision.Action, ApprovalActionManual)
	}
}

func planWithChanges(changes ...types.FileChange) *types.ChangePlan {
	targets := make([]string, 0, len(changes))
	for _, c := range changes {
		targets = append(targets, c.Path)
	}
	return &types.ChangePlan{
		ID:          "plan-test",
		Status:      types.PlanStatusPending,
		Changes:     changes,
		TargetPaths: targets,
	}
}
