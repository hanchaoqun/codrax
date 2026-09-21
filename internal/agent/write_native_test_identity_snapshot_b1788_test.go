package agent

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the actual agent instruction surfaces, not only a formatting helper.
// An exact assertion receipt is execution context, never an inferred PTO binding.
func TestB1788NativeTestIdentityReachesActualAgentInstructions(t *testing.T) {
	for _, mixedFailure := range []bool{false, true} {
		name := "passed"
		if mixedFailure {
			name = "mixed_failure"
		}
		t.Run(name, func(t *testing.T) {
			ctx := b1788NativeIdentityContext()
			report := ctx.Mutable.ChangeReport()
			if mixedFailure {
				report.Passed = false
				report.TestResults = append(report.TestResults, types.TestResult{Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion, Suite: "pkg.Other", AssertionID: "test_failed", FailureDetail: "other native assertion failed"})
			}
			before := b1788IdentityJSON(t, []any{ctx.Mutable.ChangePlan(), report})
			proof := b1788IdentityJSON(t, types.BuildVerificationProofLedger(ctx.Mutable.ChangePlan(), report, nil))
			for surface, instruction := range map[string]string{
				"planner":    (&plannerEvaluator{}).BuildInitialInstruction(ctx, nil),
				"controller": (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil),
			} {
				for _, want := range []string{"## Current native test identity snapshot", `"assertion_suite":"project.tests.长名称.ValueCase"`, `"assertion_id":"test_value[quote=\"yes\"\\path]"`, "2026-09-21T04:00:00Z", "not proof of latest source bytes or execution generation", "does not bind a behavior contract"} {
					if !strings.Contains(instruction, want) {
						t.Errorf("%s lost current exact native identity or boundary %q", surface, want)
					}
				}
				if mixedFailure && !strings.Contains(instruction, "other native assertion failed") {
					t.Errorf("%s erased independent failure", surface)
				}
			}
			if !bytes.Equal(before, b1788IdentityJSON(t, []any{ctx.Mutable.ChangePlan(), report})) || !bytes.Equal(proof, b1788IdentityJSON(t, types.BuildVerificationProofLedger(ctx.Mutable.ChangePlan(), report, nil))) {
				t.Fatal("display mutated report, PTO, or proof authority")
			}
		})
	}
}

func b1788NativeIdentityContext() *types.AgentContext {
	mu := types.NewMutableState("verify the changed behavior")
	mu.SetChangePlan(&types.ChangePlan{ID: "current-plan", ProjectTestObservations: []types.ProjectTestObservation{{ID: "existing-binding", TestPath: "tests/test_value.py", AssertionSuite: "another-suite", AssertionID: "wrong-id", ContractRefs: []string{"value"}}}})
	mu.SetChangeReport(&types.ChangeReport{
		PlanID: "current-plan", Channel: types.ChangeReportChannelPostApplyVerify, Passed: true,
		GeneratedAt: time.Date(2026, 9, 21, 4, 0, 0, 0, time.UTC),
		TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion, Suite: "project.tests.长名称.ValueCase", AssertionID: "test_value[quote=\"yes\"\\path]", Passed: true}},
	})
	return &types.AgentContext{Mutable: mu, Mode: types.ModeApply}
}

func TestB1788NativeTestIdentitySnapshotNeverRevivesHistoricalContext(t *testing.T) {
	for _, name := range []string{"nil_context", "read_mode", "no_mutable", "no_plan", "blank_plan", "no_report", "empty_current_report", "foreign_report", "missing_report_id", "legacy_channel", "planner_probe", "historical_timestamp"} {
		t.Run(name, func(t *testing.T) {
			ctx := b1788NativeIdentityContext()
			ctx.Mutable.SetWriteContextPack(&types.WriteContextPack{Items: []types.WriteContextItem{{ID: "old-native", Kind: "verification_native_test_identity", SourceStage: "verify", SourceID: "current-plan", Text: "old-native-identity-do-not-revive"}}})
			ctx.Mutable.AppendPlanStageProbeReport(ctx.Mutable.ChangeReport())
			ctx.Mutable.SetWriteWorkflowRun(&types.WriteWorkflowRun{ActiveBatchID: "batch", Batches: []types.WriteWorkflowBatch{{ID: "batch", PlanID: "current-plan"}}})
			switch name {
			case "nil_context":
				ctx = nil
			case "read_mode":
				ctx.Mode = types.ModeRead
			case "no_mutable":
				ctx.Mutable = nil
			case "no_plan":
				ctx.Mutable.SetChangePlan(nil)
			case "blank_plan":
				ctx.Mutable.ChangePlan().ID = ""
			case "no_report":
				ctx.Mutable.SetChangeReport(nil)
			case "empty_current_report":
				ctx.Mutable.ChangeReport().TestResults = nil
			case "foreign_report":
				ctx.Mutable.ChangeReport().PlanID = "other-plan"
			case "missing_report_id":
				ctx.Mutable.ChangeReport().PlanID = ""
			case "legacy_channel":
				ctx.Mutable.ChangeReport().Channel = ""
			case "planner_probe":
				ctx.Mutable.ChangeReport().Channel = types.ChangeReportChannelPlannerProbe
			case "historical_timestamp":
				ctx.Mutable.ChangeReport().GeneratedAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			got := buildWriteNativeTestIdentitySnapshot(ctx)
			if name == "historical_timestamp" {
				if !strings.Contains(got, "2000-01-01T00:00:00Z") || !strings.Contains(got, "not proof of latest source bytes or execution generation") {
					t.Fatal("current held old report must disclose age without claiming current bytes")
				}
			} else if got != "" {
				t.Fatalf("wrong/empty current context recovered historical identities: %s", got)
			}
		})
	}
}

func b1788IdentityJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
