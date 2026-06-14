package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func TestRenderWriteRiskAssessmentShowsDeterministicPreview(t *testing.T) {
	plan := &types.ChangePlan{
		ID:     "plan-test",
		Status: types.PlanStatusPending,
		Changes: []types.FileChange{
			{Path: "go.mod", Kind: "modify", Rationale: "update dependency"},
		},
		TargetPaths: []string{"go.mod"},
	}

	got := strings.Join(renderWriteRiskAssessment("en", plan, writeflow.ApprovalPolicyAutoSafe), "\n")
	if !strings.Contains(got, "planned-change risk: high") {
		t.Fatalf("rendered risk missing high band:\n%s", got)
	}
	if !strings.Contains(got, "approval preview: auto_safe => manual_approval") {
		t.Fatalf("rendered approval preview missing manual action:\n%s", got)
	}
	if !strings.Contains(got, "build_or_dependency_manifest") {
		t.Fatalf("rendered reasons missing manifest path class:\n%s", got)
	}
}

func TestRenderWriteRiskAssessmentChinese(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-test",
		Status:      types.PlanStatusPending,
		Changes:     []types.FileChange{{Path: "docs/guide.md", Kind: "modify"}},
		TargetPaths: []string{"docs/guide.md"},
	}

	got := strings.Join(renderWriteRiskAssessment("zh-CN", plan, writeflow.ApprovalPolicyAutoSafe), "\n")
	if !strings.Contains(got, "计划变更风险：low") {
		t.Fatalf("rendered zh risk missing low band:\n%s", got)
	}
	if !strings.Contains(got, "审批预览") {
		t.Fatalf("rendered zh approval preview missing:\n%s", got)
	}
}

func TestRenderWriteRiskAssessmentSeparatesAnalysisAdvisory(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-test",
		Status:      types.PlanStatusPending,
		Changes:     []types.FileChange{{Path: "internal/foo.go", Kind: "modify"}},
		TargetPaths: []string{"internal/foo.go"},
		WriteAnalysisIR: &types.WriteAnalysisIR{
			Request: types.WriteRequestModel{
				Risk: types.WriteRiskProfile{
					AffectsPublicAPI:   true,
					ChangesPersistence: true,
					Overall:            types.RiskBandHigh,
				},
			},
		},
	}

	got := strings.Join(renderWriteRiskAssessment("en", plan, writeflow.ApprovalPolicyAutoSafe), "\n")
	for _, want := range []string{
		"planned-change risk: medium",
		"approval preview: auto_safe => auto_execute",
		"analysis risk advisory: overall=high axes=public_api,persistence",
		"advisory only, not the approval gate",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered advisory separation missing %q:\n%s", want, got)
		}
	}
}
