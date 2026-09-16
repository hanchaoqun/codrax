package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func TestB1678PublicProofFollowupPreservesUnknownExecution(t *testing.T) {
	for _, tc := range []struct {
		capability       types.VerificationCapability
		samePathBehavior bool
	}{
		{types.VerificationCapabilitySourceStatic, false},
		{types.VerificationCapabilitySyntaxOnly, false},
		{types.VerificationCapabilityUnknown, false},
		{types.VerificationCapabilityTargetBehavior, false},
		{types.VerificationCapabilityTargetExecution, false},
		{types.VerificationCapabilityUnknown, true},
	} {
		name := string(tc.capability)
		if tc.samePathBehavior {
			name += "_with_same_path_behavior"
		}
		t.Run(name, func(t *testing.T) {
			capability := tc.capability
			binDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(binDir, "node"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", binDir)
			plan := &types.ChangePlan{ID: "plan-execution-boundary", Status: types.PlanStatusApplied, TargetPaths: []string{"src/widget.ts"}}
			report := &types.ChangeReport{
				PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify,
				Passed: true, VerificationStatus: types.VerificationStatusPassed,
				TestResults:      []types.TestResult{{Kind: types.TestResultKindUnit, AssertionID: "check", Passed: true}},
				ExecutedCommands: []types.ExecutedCommand{{Runner: "node", Suite: "check", Outcome: types.ExecutedCommandOutcomeExecuted}},
				ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{
					Path: "src/widget.ts", Status: types.ChangedPathVerificationCovered,
					Caliber: types.ChangedPathVerificationProjectRunner, Capability: capability,
				}},
			}
			if tc.samePathBehavior {
				report.ChangedPathCoverage = append(report.ChangedPathCoverage, types.ChangedPathVerificationCoverage{
					Path: "src/widget.ts", Status: types.ChangedPathVerificationCovered,
					Caliber: types.ChangedPathVerificationProjectRunner, Capability: types.VerificationCapabilityTargetBehavior,
				})
			}
			mu := types.NewMutableState("preserve the typed execution boundary")
			mu.SetChangePlan(plan)
			mu.SetChangeReport(report)
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
			run := &types.WriteWorkflowRun{
				RunID: "wf-execution-boundary", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "batch-1",
				Batches: []types.WriteWorkflowBatch{{
					ID: "batch-1", PlanID: plan.ID, Status: types.WriteWorkflowBatchComplete,
					Completion: &types.WriteWorkflowCompletion{Verdict: types.WriteWorkflowCompletionVerified, ReasonCode: "tests_passed"},
					Attempts: []types.WriteWorkflowAttempt{
						{Kind: "apply", Status: "applied", PlanID: plan.ID},
						{Kind: "verify", Status: "passed", ReasonCode: "tests_passed", PlanID: plan.ID},
					},
				}},
			}
			before, _ := json.Marshal([]any{plan, report, types.BuildVerificationProofProfile(plan, report), types.BuildVerificationProofLedger(plan, report, nil)})
			decision := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
				Action: writeflow.ActionFinish, ReasonCode: "done", FinishDisposition: writeflow.FinishDispositionAllVerified,
			}, run)
			strong := tc.samePathBehavior || capability == types.VerificationCapabilityTargetBehavior || capability == types.VerificationCapabilityTargetExecution
			if strong {
				if decision.Action != writeflow.ActionFinish || decision.FinishDisposition != writeflow.FinishDispositionAllVerified || decision.Batch != nil {
					t.Fatalf("established target execution acquired a proof follow-up: %+v", decision)
				}
			} else {
				if decision.Action != writeflow.ActionAppendBatch || decision.Batch == nil ||
					decision.Batch.Purpose != "verification_proof_followup" || decision.Batch.ExecutionMode != "" ||
					decision.Batch.Status != writeflow.BatchReadyForChangePlan {
					t.Fatalf("existing proof-followup authority changed: %+v", decision)
				}
				if decision.Batch.ID != "batch-1-proof-repair" || !reflect.DeepEqual(decision.Batch.ExpectedPaths, []string{"src/widget.ts"}) {
					t.Fatalf("follow-up identity/path changed: %+v", decision.Batch)
				}
				wantCriterion := "impact_obligation=source-static-inline-proof:src/widget.ts kind=changed_symbol code=production_path_source_static_only path=src/widget.ts verification_probe_required=true probe_language=javascript evidence_ref=changed_path_coverage:src/widget.ts source=changed_path_coverage"
				if !reflect.DeepEqual(decision.Batch.SuccessCriteria, []string{wantCriterion}) {
					t.Fatalf("diagnostic change rewrote typed obligation identity/criteria: %+v", decision.Batch.SuccessCriteria)
				}
				o.seedControllerBatchPlanningHint(*decision.Batch)
				hint := mu.PlanningHint()
				for _, want := range []string{
					types.VerificationTargetExecutionEvidenceBoundary,
					"without established target-execution proof",
					"unknown capability does not establish what the check exercised",
					"without modifying source unless the typed probe itself proves a remaining defect",
					wantCriterion,
				} {
					if !strings.Contains(hint, want) {
						t.Errorf("planner lost exact evidence boundary %q:\n%s", want, hint)
					}
				}
				if strings.Contains(hint, "currently have source-static coverage only") {
					t.Errorf("unresolved execution capability was asserted to be static-only:\n%s", hint)
				}
			}
			after, _ := json.Marshal([]any{plan, report, types.BuildVerificationProofProfile(plan, report), types.BuildVerificationProofLedger(plan, report, nil)})
			if string(before) != string(after) {
				t.Fatal("disclosure changed the original artifacts or proof authority")
			}
		})
	}
}
