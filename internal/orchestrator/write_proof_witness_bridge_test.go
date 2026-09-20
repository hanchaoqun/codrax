package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func TestProofPlanningBridgeDoesNotRequireAnUnproducibleAssertion(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "applied", Status: types.PlanStatusUnverified, TargetPaths: []string{"pkg/value.py"},
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID: "reject-fraction", Kind: types.WriteBehaviorException, Polarity: types.WriteBehaviorPolarityExpected,
			Subject: "make_value(1.5)", Operator: types.WriteBehaviorOpRaises, Expected: "ValueError",
			Required: true, Source: "write_analyzer",
		}},
	}
	report := &types.ChangeReport{
		PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify, Passed: true, VerificationStatus: types.VerificationStatusPassed,
		TestResults:      []types.TestResult{{Kind: types.TestResultKindUnit, Suite: "ValueTest", AssertionID: "test_value", Passed: true}},
		ExecutedCommands: []types.ExecutedCommand{{Runner: "python", Suite: "unittest", Outcome: types.ExecutedCommandOutcomeExecuted}},
		ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{
			Path: "pkg/value.py", Status: types.ChangedPathVerificationCovered,
			Caliber: types.ChangedPathVerificationProjectRunner, Capability: types.VerificationCapabilityTargetBehavior,
		}},
	}
	run := &types.WriteWorkflowRun{
		RunID: "unbound-native-assertion", ActiveBatchID: "proof-review", Status: types.WriteWorkflowRunInProgress,
		Batches: []types.WriteWorkflowBatch{{
			ID: "proof-review", Purpose: "verification_proof_followup", ExecutionMode: types.WriteWorkflowBatchExecutionVerifyOnly,
			Status: types.WriteWorkflowBatchComplete, ExpectedPaths: []string{"pkg/value.py"},
			SuccessCriteria: []string{"kind=behavior_contract contract_ref=reject-fraction verification_probe_required=true"},
			Attempts:        []types.WriteWorkflowAttempt{{Kind: "verify", Status: "unverified", ReasonCode: "verification_proof_incomplete", PlanID: plan.ID}},
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{BatchID: "source", ReasonCode: "verification_proof_followup_requested"}},
	}
	ledger := types.BuildVerificationProofLedger(plan, report, nil)
	if ledger.State != types.VerificationProofLedgerLowConfidence || ledger.UncoveredCount != 1 {
		t.Fatalf("fixture must isolate one uncovered behavior assertion: %+v", ledger)
	}
	before, _ := json.Marshal([]any{plan, report, run, ledger})
	if batch, ok := verificationProofProbePlanningFollowupDecisionWithRuntimeAvailability(run, plan, report, func(string) bool { return true }); ok || batch != nil {
		t.Fatalf("runtime presence cannot make this required assertion producible by a plain probe: %+v", batch)
	}
	after, _ := json.Marshal([]any{plan, report, run, types.BuildVerificationProofLedger(plan, report, nil)})
	if string(before) != string(after) {
		t.Fatal("suppressing an impossible dispatch changed evidence or closed debt")
	}
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "python3"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	mu := types.NewMutableState("verify applied target")
	mu.SetChangePlan(plan)
	mu.SetChangeReport(report)
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Mode: types.ModeApply}}
	next := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{
		Action: writeflow.ActionFinish, FinishDisposition: writeflow.FinishDispositionAllVerified,
	}, run)
	if next.Action != writeflow.ActionFinish || next.FinishDisposition != writeflow.FinishDispositionAcceptUnverified {
		t.Fatalf("missing assertion must stay unverified without mandatory replan: %+v", next)
	}
	if workflowProgressHasReason(run.ProgressLedger, "verification_proof_probe_plan_requested") {
		t.Fatal("impossible probe was requested by a later normalization branch")
	}
	if got := types.BuildVerificationProofLedger(plan, report, nil); got.UncoveredCount != 1 || got.State != types.VerificationProofLedgerLowConfidence {
		t.Fatalf("unverified finish upgraded assertion proof: %+v", got)
	}

	// Existing declared native bindings are not disabled by this narrow guard.
	plan.ProjectTestObservations = []types.ProjectTestObservation{{
		ID: "native", TestPath: "tests/test_value.py", AssertionSuite: "ValueTest", AssertionID: "test_value", ContractRefs: []string{"reject-fraction"},
	}}
	if batch, ok := verificationProofProbePlanningFollowupDecisionWithRuntimeAvailability(run, plan, report, func(string) bool { return true }); !ok || batch == nil {
		t.Fatalf("existing native assertion recovery was disabled: %+v", batch)
	}
}
