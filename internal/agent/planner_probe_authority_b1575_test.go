package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func TestB1575ActualPlannerNoChangeHintUsesSharedQualification(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*types.ChangePlan, *types.ChangeReport)
		want bool
	}{
		{"changed_execution", nil, true},
		{"changed_behavior", func(_ *types.ChangePlan, r *types.ChangeReport) {
			r.ChangedPathCoverage[0].Capability = types.VerificationCapabilityTargetBehavior
		}, true},
		{"observation_only", func(_ *types.ChangePlan, r *types.ChangeReport) { r.ChangedPathCoverage = nil }, false},
		{"behavior_gap", func(_ *types.ChangePlan, r *types.ChangeReport) {
			r.VerificationConfidence = []types.VerificationConfidenceRecord{{Severity: "warning", Status: "missing", ReasonCode: "behavior_assertion_unproven"}}
		}, false},
		{"not_applied", func(p *types.ChangePlan, _ *types.ChangeReport) { p.AppliedCommitSHA = "" }, false},
		{"other_target", func(_ *types.ChangePlan, r *types.ChangeReport) { r.ChangedPathCoverage[0].Path = "other.py" }, false},
		{"other_plan", func(_ *types.ChangePlan, r *types.ChangeReport) { r.PlanID = "old-plan" }, false},
		{"failed_probe", func(_ *types.ChangePlan, r *types.ChangeReport) {
			r.Passed = false
			r.VerificationStatus = types.VerificationStatusFailed
			r.TestResults[0].Passed = false
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := &types.ChangePlan{ID: "applied", AppliedCommitSHA: "abc123", TargetPaths: []string{"pkg/worker.py"}}
			report := &types.ChangeReport{
				PlanID: "applied", Channel: types.ChangeReportChannelPlannerProbe,
				Passed: true, VerificationStatus: types.VerificationStatusPassed,
				TestResults:         []types.TestResult{{AssertionID: "probe-check", Passed: true}},
				ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{Path: "pkg/worker.py", Status: types.ChangedPathVerificationCovered, Capability: types.VerificationCapabilityTargetExecution}},
				FailureSummary:      "model-authored probe output stays unchanged",
			}
			if tc.edit != nil {
				tc.edit(plan, report)
			}
			handoff := &types.VerifyFailureHandoff{PlanID: "applied", FailureKind: types.FailureKindTestsFailed}
			qualification := writeflow.QualifyNoChangeReplanSentinel(writeflow.NoChangeReplanQualificationInput{VerifyFailureHandoff: handoff, PriorPlan: plan, PlannerProbeReports: []*types.ChangeReport{report}, RequireAppliedWork: true})
			if qualification.Allowed != tc.want {
				t.Fatalf("fixture qualifier = %+v, want allowed %v", qualification, tc.want)
			}
			before, err := json.Marshal([]any{plan, report, handoff})
			if err != nil {
				t.Fatal(err)
			}
			mu := types.NewMutableState("bounded probe recovery")
			mu.SetChangePlan(plan)
			mu.SetVerifyFailureHandoff(handoff)
			mu.AppendPlanStageProbeReport(report)
			got := (&plannerEvaluator{}).BuildInitialInstruction(&types.AgentContext{Mutable: mu}, nil)
			if strings.Contains(got, "No-change sentinel available") != tc.want {
				t.Errorf("actual prompt available=%v, shared qualifier=%+v", strings.Contains(got, "No-change sentinel available"), qualification)
			}
			if !strings.Contains(got, report.FailureSummary) {
				t.Error("lost original probe output")
			}
			after, err := json.Marshal([]any{plan, report, handoff})
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Error("rendering changed plan/report/handoff")
			}
		})
	}
}
