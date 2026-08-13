package writeflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestApplyWorkflowDecisionToRunExploreThenPlan(t *testing.T) {
	run := types.WriteWorkflowRun{RunID: "wf-1"}
	var err error
	run, err = ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{
		Action:     ActionExploreCode,
		ReasonCode: "need_source_context",
		ExplorationRequest: &types.WriteExplorationRequest{
			BatchID:              "batch-1",
			Goal:                 "inspect approval gate",
			ExplorationQuestions: []string{"where does apply gate run?"},
		},
	})
	if err != nil {
		t.Fatalf("explore decision failed: %v", err)
	}
	if run.Status != types.WriteWorkflowRunInProgress || run.ActiveBatchID != "batch-1" {
		t.Fatalf("unexpected run after explore: %+v", run)
	}
	if len(run.Batches) != 1 || run.Batches[0].Status != types.WriteWorkflowBatchNeedsExploration {
		t.Fatalf("unexpected batches after explore: %+v", run.Batches)
	}
	if run.Budget.ExplorationRoundsUsed != 1 || len(run.Edges) != 1 || run.Edges[0].Kind != types.WriteWorkflowEdgeExplore {
		t.Fatalf("explore edge/budget not recorded: edges=%+v budget=%+v", run.Edges, run.Budget)
	}

	run, err = ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{
		Action:     ActionPlanBatch,
		ReasonCode: "enough_context",
		Batch: &WriteBatchPlan{
			ID:              "batch-1",
			Goal:            "patch apply gate",
			Purpose:         "verification_proof_followup",
			ExpectedPaths:   []string{"internal/orchestrator/stage_hooks.go"},
			SuccessCriteria: []string{"contract_ref=approval-boundary"},
		},
	})
	if err != nil {
		t.Fatalf("plan decision failed: %v", err)
	}
	if len(run.Batches) != 1 || run.Batches[0].Status != types.WriteWorkflowBatchReadyToPlan {
		t.Fatalf("plan should update existing batch, got %+v", run.Batches)
	}
	if run.Batches[0].Goal != "patch apply gate" {
		t.Fatalf("batch goal not updated: %+v", run.Batches[0])
	}
	if run.Batches[0].Purpose != "verification_proof_followup" ||
		len(run.Batches[0].ExpectedPaths) != 1 || run.Batches[0].ExpectedPaths[0] != "internal/orchestrator/stage_hooks.go" ||
		len(run.Batches[0].SuccessCriteria) != 1 || run.Batches[0].SuccessCriteria[0] != "contract_ref=approval-boundary" {
		t.Fatalf("batch metadata not persisted: %+v", run.Batches[0])
	}
	if len(run.Edges) != 2 || run.Edges[1].Kind != types.WriteWorkflowEdgePlan {
		t.Fatalf("plan edge not appended: %+v", run.Edges)
	}
}

func TestApplyWorkflowDecisionToRunAppendsVerifyOnlyBatchWithoutPlanningState(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-verify-only",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
		}},
	}
	got, err := ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{
		Action: ActionAppendBatch,
		Batch: &WriteBatchPlan{
			ID:              "batch-1-cumulative-review",
			Goal:            "verify the applied diff",
			Purpose:         "verification_proof_followup",
			ExecutionMode:   types.WriteWorkflowBatchExecutionVerifyOnly,
			ExpectedPaths:   []string{"pkg/fix.ts", "pkg/fix.test.ts"},
			SuccessCriteria: []string{"contract_ref=retains-existing-default"},
			DependsOn:       []string{"batch-1"},
		},
	})
	if err != nil {
		t.Fatalf("append verify-only decision failed: %v", err)
	}
	if got.ActiveBatchID != "batch-1-cumulative-review" || len(got.Batches) != 2 {
		t.Fatalf("verify-only batch not appended: %+v", got)
	}
	batch := got.Batches[1]
	if batch.Status != types.WriteWorkflowBatchVerifying ||
		batch.ExecutionMode != types.WriteWorkflowBatchExecutionVerifyOnly {
		t.Fatalf("verify-only batch entered a change-planning state: %+v", batch)
	}
	if batch.Purpose != "verification_proof_followup" ||
		len(batch.SuccessCriteria) != 1 ||
		batch.SuccessCriteria[0] != "contract_ref=retains-existing-default" {
		t.Fatalf("controller-owned verify contract missing: %+v", batch)
	}

	// A later model echo must not rewrite the controller-owned observation
	// contract into a change-capable cumulative review.
	echo := &WriteBatchPlan{
		ID:              batch.ID,
		Goal:            "rewrite the implementation",
		Purpose:         "free-form cumulative review",
		ExpectedPaths:   []string{"pkg/other.ts"},
		SuccessCriteria: []string{"replace ??= with ="},
	}
	applyWorkflowBatchPlanMetadata(&got, batch.ID, echo)
	batch = got.Batches[1]
	if batch.Goal != "verify the applied diff" ||
		batch.Purpose != "verification_proof_followup" ||
		len(batch.ExpectedPaths) != 2 ||
		batch.SuccessCriteria[0] != "contract_ref=retains-existing-default" {
		t.Fatalf("model echo overwrote verify-only authority: %+v", batch)
	}
}

func TestApplyWorkflowDecisionToRunPreservesControllerOwnedDirectProofBatchRouting(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-direct-proof",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1-cumulative-review",
		Batches: []types.WriteWorkflowBatch{{
			ID:              "batch-1-cumulative-review",
			Goal:            "author one bounded direct-runtime verification probe",
			Purpose:         "verification_proof_followup",
			ExpectedPaths:   []string{"pkg/fix.ts"},
			SuccessCriteria: []string{"contract_ref=retains-falsy-default"},
			DependsOn:       []string{"batch-1"},
			Status:          types.WriteWorkflowBatchReadyToPlan,
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-1",
			ReasonCode: "verification_proof_followup_requested",
		}},
	}

	got, err := ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{
		Action: ActionPlanBatch,
		Batch: &WriteBatchPlan{
			ID:              "batch-1-cumulative-review",
			Goal:            "edit more tests",
			Purpose:         "execute runtime checks and patch whatever is missing",
			ExecutionMode:   types.WriteWorkflowBatchExecutionVerifyOnly,
			ExpectedPaths:   []string{"pkg/fix.test.ts"},
			SuccessCriteria: []string{"new test file passes"},
			DependsOn:       []string{"some-other-batch"},
		},
	})
	if err != nil {
		t.Fatalf("plan direct-proof batch failed: %v", err)
	}
	batch := got.Batches[0]
	if batch.Goal != "author one bounded direct-runtime verification probe" ||
		batch.Purpose != "verification_proof_followup" ||
		batch.ExecutionMode != "" ||
		len(batch.ExpectedPaths) != 1 || batch.ExpectedPaths[0] != "pkg/fix.ts" ||
		len(batch.SuccessCriteria) != 1 || batch.SuccessCriteria[0] != "contract_ref=retains-falsy-default" ||
		len(batch.DependsOn) != 1 || batch.DependsOn[0] != "batch-1" {
		t.Fatalf("model batch echo overwrote controller-owned direct-proof routing: %+v", batch)
	}

	// The lower metadata merge seam must enforce the same ownership rule for
	// callers that already resolved the workflow action.
	applyWorkflowBatchPlanMetadata(&got, batch.ID, &WriteBatchPlan{
		ID:              batch.ID,
		Goal:            "rewrite production",
		Purpose:         "ordinary implementation",
		ExpectedPaths:   []string{"pkg/other.ts"},
		SuccessCriteria: []string{"replacement succeeds"},
	})
	batch = got.Batches[0]
	if batch.Goal != "author one bounded direct-runtime verification probe" ||
		batch.Purpose != "verification_proof_followup" ||
		batch.ExpectedPaths[0] != "pkg/fix.ts" ||
		batch.SuccessCriteria[0] != "contract_ref=retains-falsy-default" {
		t.Fatalf("direct metadata merge overwrote controller-owned routing: %+v", batch)
	}
}

func TestApplyWorkflowDecisionToRunAllowsOrdinaryBatchMetadataRefinement(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-ordinary",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:      "batch-1",
			Goal:    "inspect issue",
			Purpose: "model-authored initial purpose",
			Status:  types.WriteWorkflowBatchReadyToPlan,
		}},
	}
	got, err := ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{
		Action: ActionPlanBatch,
		Batch: &WriteBatchPlan{
			ID:              "batch-1",
			Goal:            "implement bounded fix",
			Purpose:         "model-authored refined purpose",
			ExpectedPaths:   []string{"pkg/fix.go"},
			SuccessCriteria: []string{"targeted test passes"},
		},
	})
	if err != nil {
		t.Fatalf("refine ordinary batch failed: %v", err)
	}
	if got.Batches[0].Goal != "implement bounded fix" ||
		got.Batches[0].Purpose != "model-authored refined purpose" ||
		len(got.Batches[0].ExpectedPaths) != 1 || got.Batches[0].ExpectedPaths[0] != "pkg/fix.go" {
		t.Fatalf("ordinary model-owned batch was incorrectly locked: %+v", got.Batches[0])
	}
}

func TestApplyWorkflowDecisionToRunDoesNotTrustPurposeWithoutTypedAuthorization(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-unowned-purpose",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:      "batch-1",
			Goal:    "model chose a reserved-looking label",
			Purpose: "verification_proof_followup",
			Status:  types.WriteWorkflowBatchReadyToPlan,
		}},
	}
	got, err := ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{
		Action: ActionPlanBatch,
		Batch: &WriteBatchPlan{
			ID:              "batch-1",
			Goal:            "refine ordinary batch",
			Purpose:         "ordinary implementation",
			ExpectedPaths:   []string{"pkg/fix.go"},
			SuccessCriteria: []string{"targeted test passes"},
		},
	})
	if err != nil {
		t.Fatalf("refine unowned purpose failed: %v", err)
	}
	if got.Batches[0].Purpose != "ordinary implementation" || got.Batches[0].Goal != "refine ordinary batch" {
		t.Fatalf("reserved-looking prose without typed progress was treated as controller authority: %+v", got.Batches[0])
	}
}

func TestApplyWorkflowDecisionToRunFinishAndBlock(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-1",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchVerifying,
		}},
	}
	done, err := ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{Action: ActionFinish})
	if err != nil {
		t.Fatalf("finish failed: %v", err)
	}
	if done.Status != types.WriteWorkflowRunComplete || done.Batches[0].Status != types.WriteWorkflowBatchComplete {
		t.Fatalf("finish did not mark terminal complete: %+v", done)
	}

	blocked, err := ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{Action: ActionBlock, ReasonCode: "critical_risk"})
	if err != nil {
		t.Fatalf("block failed: %v", err)
	}
	if blocked.Status != types.WriteWorkflowRunBlocked || blocked.Batches[0].Status != types.WriteWorkflowBatchBlocked {
		t.Fatalf("block did not mark terminal blocked: %+v", blocked)
	}
	if len(blocked.Edges) != 1 || blocked.Edges[0].Kind != types.WriteWorkflowEdgeBlocked {
		t.Fatalf("block edge missing: %+v", blocked.Edges)
	}
}

func TestApplyWorkflowDecisionToRunFinishCarriesTypedCompletionVerdict(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-1",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{{
				Kind:       "verify",
				Status:     "unverified",
				ReasonCode: "parser_error",
			}},
		}},
	}
	done, err := ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{
		Action:            ActionFinish,
		FinishDisposition: FinishDispositionAcceptUnverified,
	})
	if err != nil {
		t.Fatalf("finish failed: %v", err)
	}
	if done.Batches[0].Completion == nil || done.Batches[0].Completion.Verdict != types.WriteWorkflowCompletionUnverified {
		t.Fatalf("batch completion verdict missing: %+v", done.Batches[0].Completion)
	}
	if done.Completion == nil || done.Completion.Verdict != types.WriteWorkflowCompletionUnverified {
		t.Fatalf("run completion verdict missing: %+v", done.Completion)
	}
}

func TestApplyWorkflowDecisionToRunFinishRejectsFailedVerifyEvenWithDisposition(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-1",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
			Attempts: []types.WriteWorkflowAttempt{{
				Kind:       "verify",
				Status:     "failed",
				ReasonCode: "tests_failed",
			}},
		}},
	}
	if _, err := ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{Action: ActionFinish}); err == nil {
		t.Fatal("finish should reject failed verify without typed disposition")
	}
	if _, err := ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{
		Action:            ActionFinish,
		FinishDisposition: FinishDispositionAcceptUnverified,
	}); err == nil {
		t.Fatal("accept_unverified should not permit finish when failed verification is typed as code failure")
	}
}

func TestApplyWorkflowDecisionToRunFinishAllowsUnavailableVerifyDisposition(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-1",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
			Attempts: []types.WriteWorkflowAttempt{{
				Kind:       "verify",
				Status:     "failed",
				ReasonCode: "parser_error",
			}},
		}},
	}
	done, err := ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{
		Action:            ActionFinish,
		FinishDisposition: FinishDispositionAcceptUnverified,
	})
	if err != nil {
		t.Fatalf("accept_unverified should permit unavailable verification: %v", err)
	}
	if done.Completion == nil || done.Completion.Verdict != types.WriteWorkflowCompletionUnverified {
		t.Fatalf("unverified completion not recorded: %+v", done.Completion)
	}
}

func TestApplyWorkflowDecisionToRunDoesNotInferFromReasonProse(t *testing.T) {
	_, err := ApplyWorkflowDecisionToRun(types.WriteWorkflowRun{RunID: "wf-1"}, WriteWorkflowDecision{
		Action: ActionExploreCode,
		Reason: "The next step is obvious from prose, but no typed exploration payload is present.",
	})
	if err == nil || !strings.Contains(err.Error(), "exploration_request") {
		t.Fatalf("expected typed payload validation error, got %v", err)
	}
}
