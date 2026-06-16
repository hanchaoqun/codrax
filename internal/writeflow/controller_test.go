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
			ID:   "batch-1",
			Goal: "patch apply gate",
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
	if len(run.Edges) != 2 || run.Edges[1].Kind != types.WriteWorkflowEdgePlan {
		t.Fatalf("plan edge not appended: %+v", run.Edges)
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
