package writeflow

import (
	"reflect"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func pendingProofIdentityFixture() types.WriteWorkflowRun {
	return types.WriteWorkflowRun{
		RunID: "pending-proof", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "proof",
		Batches: []types.WriteWorkflowBatch{{
			ID: "proof", Goal: "verify existing target", Purpose: "verification_proof_followup",
			CreatedAt:     time.Unix(100, 0),
			Status:        types.WriteWorkflowBatchReadyToPlan,
			ExpectedPaths: []string{"pkg/target.py"}, DependsOn: []string{"applied"},
			SuccessCriteria: []string{"contract_ref=boundary verification_probe_required=true"},
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{BatchID: "applied", ReasonCode: "verification_proof_followup_requested"}},
	}
}

func TestApplyWorkflowDecisionToRunPendingProofCannotBeRenamed(t *testing.T) {
	for _, purpose := range []string{"verification_proof_followup", "impact_and_verification_proof_followup"} {
		for _, echoID := range []string{"model-renamed-proof", "proof", ""} {
			t.Run(purpose+"/"+echoID, func(t *testing.T) {
				run := pendingProofIdentityFixture()
				run.Batches[0].Purpose = purpose
				run = types.NormalizeWriteWorkflowRun(run)
				before := run.Batches[0]
				got, err := ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{
					Action: ActionPlanBatch,
					Batch: &WriteBatchPlan{
						ID: echoID, Goal: "patch source again", Purpose: "ordinary implementation",
						ExecutionMode: types.WriteWorkflowBatchExecutionVerifyOnly,
						ExpectedPaths: []string{"unrelated.js"}, DependsOn: []string{"other"},
						SuccessCriteria: []string{"all done"},
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				if got.ActiveBatchID != before.ID || len(got.Batches) != 1 || got.Budget.BatchesUsed != run.Budget.BatchesUsed {
					t.Fatalf("renamed echo escaped pending proof authority: %+v", got)
				}
				actual := got.Batches[0]
				actual.UpdatedAt = before.UpdatedAt // Transition bookkeeping may advance time.
				if !reflect.DeepEqual(actual, before) {
					t.Fatalf("pending proof envelope changed: got %+v, want %+v", got.Batches[0], before)
				}
			})
		}
	}
}

func TestPendingControllerProofPlanBatchReturnsIndependentEnvelope(t *testing.T) {
	if PendingControllerProofPlanBatch(nil) != nil {
		t.Fatal("nil run minted a proof plan")
	}
	run := pendingProofIdentityFixture()
	got := PendingControllerProofPlanBatch(&run)
	if got == nil {
		t.Fatal("pending proof not found")
	}
	got.ExpectedPaths[0] = "other"
	got.SuccessCriteria[0] = "other"
	got.DependsOn[0] = "other"
	if !reflect.DeepEqual(run, pendingProofIdentityFixture()) {
		t.Fatal("returned metadata aliases the durable run")
	}
}

func TestApplyWorkflowDecisionToRunPendingProofIdentityBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*types.WriteWorkflowRun)
	}{
		{"no_controller_receipt", func(r *types.WriteWorkflowRun) { r.ProgressLedger = nil }},
		{"wrong_receipt", func(r *types.WriteWorkflowRun) { r.ProgressLedger[0].ReasonCode = "model_says_proof_requested" }},
		{"ordinary_purpose", func(r *types.WriteWorkflowRun) { r.Batches[0].Purpose = "feature" }},
		{"completed", func(r *types.WriteWorkflowRun) { r.Batches[0].Status = types.WriteWorkflowBatchComplete }},
		{"needs_exploration", func(r *types.WriteWorkflowRun) { r.Batches[0].Status = types.WriteWorkflowBatchNeedsExploration }},
		{"plan_already_minted", func(r *types.WriteWorkflowRun) { r.Batches[0].PlanID = "existing-plan" }},
		{"verify_only", func(r *types.WriteWorkflowRun) {
			r.Batches[0].ExecutionMode = types.WriteWorkflowBatchExecutionVerifyOnly
		}},
		{"no_probe_requirement", func(r *types.WriteWorkflowRun) { r.Batches[0].SuccessCriteria = []string{"contract_ref=boundary"} }},
		{"false_requirement", func(r *types.WriteWorkflowRun) {
			r.Batches[0].SuccessCriteria = []string{"verification_probe_required=false"}
		}},
		{"noisy_requirement", func(r *types.WriteWorkflowRun) {
			r.Batches[0].SuccessCriteria = []string{"not_verification_probe_required=true"}
		}},
		{"different_active_batch", func(r *types.WriteWorkflowRun) { r.ActiveBatchID = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := pendingProofIdentityFixture()
			tc.mutate(&run)
			got, err := ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{Action: ActionPlanBatch, Batch: &WriteBatchPlan{ID: "new", Goal: "new bounded task"}})
			if err != nil {
				t.Fatal(err)
			}
			if got.ActiveBatchID != "new" {
				t.Fatalf("unrelated transition was captured: %+v", got)
			}
		})
	}
	// Explicit append/split/replan are distinct actions, not pending-plan echoes.
	for _, action := range []WorkflowAction{ActionAppendBatch, ActionSplitBatch, ActionReplanBatch} {
		run := pendingProofIdentityFixture()
		got, err := ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{Action: action, Batch: &WriteBatchPlan{ID: "new", Goal: "separate followup"}})
		if err != nil || got.ActiveBatchID != "new" {
			t.Fatalf("%s changed: %+v, %v", action, got, err)
		}
	}
}
