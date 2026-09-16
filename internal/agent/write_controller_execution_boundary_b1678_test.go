package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1678ControllerExecutionBoundaryPreservesUnknown(t *testing.T) {
	for _, tc := range []struct {
		name             string
		capability       types.VerificationCapability
		path             string
		want             bool
		samePathBehavior bool
	}{
		{"source_static", types.VerificationCapabilitySourceStatic, "src/widget.ts", true, false},
		{"syntax_only", types.VerificationCapabilitySyntaxOnly, "src/widget.ts", true, false},
		{"unknown", types.VerificationCapabilityUnknown, "src/widget.ts", true, false},
		{"target_behavior", types.VerificationCapabilityTargetBehavior, "src/widget.ts", false, false},
		{"target_execution", types.VerificationCapabilityTargetExecution, "src/widget.ts", false, false},
		{"auxiliary_unknown", types.VerificationCapabilityUnknown, "tests/widget.test.ts", false, false},
		{"unknown_with_same_path_behavior", types.VerificationCapabilityUnknown, "src/widget.ts", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := &types.ChangePlan{ID: "plan-execution-boundary", Status: types.PlanStatusApplied}
			report := &types.ChangeReport{
				PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify,
				Passed: true, VerificationStatus: types.VerificationStatusPassed,
				TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, AssertionID: "check", Passed: true}},
				ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{
					Path: tc.path, Status: types.ChangedPathVerificationCovered,
					Caliber: types.ChangedPathVerificationProjectRunner, Capability: tc.capability,
				}},
			}
			if tc.samePathBehavior {
				report.ChangedPathCoverage = append(report.ChangedPathCoverage, types.ChangedPathVerificationCoverage{
					Path: tc.path, Status: types.ChangedPathVerificationCovered,
					Caliber: types.ChangedPathVerificationProjectRunner, Capability: types.VerificationCapabilityTargetBehavior,
				})
				if report.HasProductionPathWithoutTargetExecutionCoverage() || types.BuildVerificationProofProfile(plan, report).Status != types.VerificationProofStrong {
					t.Fatal("mixed-row premise must retain the independent strong proof")
				}
			}
			mu := types.NewMutableState("preserve the typed execution boundary")
			mu.SetChangePlan(plan)
			mu.SetChangeReport(report)
			ctx := &types.AgentContext{Mutable: mu, Mode: types.ModeApply}
			before, _ := json.Marshal([]any{plan, report, types.BuildVerificationProofProfile(plan, report), types.BuildVerificationProofLedger(plan, report, nil)})
			got := (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil)
			if strings.Contains(got, "changed_path_verification_boundary:") != tc.want {
				t.Fatalf("existing disclosure selection changed for %s:\n%s", tc.capability, got)
			}
			if tc.want {
				for _, want := range []string{
					types.VerificationTargetExecutionEvidenceBoundary,
					"capability=" + string(tc.capability),
					"source_static/syntax_only coverage proves source shape only",
					"unknown capability does not establish what the check exercised",
					"do not select all_verified from report passed status alone",
				} {
					if !strings.Contains(got, want) {
						t.Errorf("controller lost execution evidence boundary %q:\n%s", want, got)
					}
				}
			}
			after, _ := json.Marshal([]any{plan, report, types.BuildVerificationProofProfile(plan, report), types.BuildVerificationProofLedger(plan, report, nil)})
			if string(before) != string(after) {
				t.Fatal("presentation changed the report or proof authority")
			}
			if again := (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil); got != again {
				t.Fatal("execution boundary rendering is not idempotent")
			}
		})
	}
}
