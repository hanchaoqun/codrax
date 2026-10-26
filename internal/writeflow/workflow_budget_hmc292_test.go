package writeflow

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc292Run(limit, historical int) types.WriteWorkflowRun {
	return types.WriteWorkflowRun{RunID: "restored", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "source",
		Budget: types.WriteWorkflowBudget{MaxBatches: limit, BatchesUsed: historical},
		Batches: []types.WriteWorkflowBatch{{ID: "source", Status: types.WriteWorkflowBatchVerifying, PlanID: "source-plan", ApplyRef: "actual-commit",
			Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: "source-plan", ArtifactRef: "actual-commit"}}}},
		ProgressLedger: []types.WriteWorkflowProgress{{BatchID: "source", ReasonCode: "source_retained"}}}
}

func TestHMC292BudgetModelProposalDoesNotBecomeExecutionLimit(t *testing.T) {
	for _, limit := range []int{0, 1, 5} {
		run := hmc292Run(limit, 0)
		got, err := ApplyWorkflowDecisionToRun(run, WriteWorkflowDecision{Action: ActionVerifyBatch, Workflow: WriteWorkflowPlan{MaxBatches: 1}})
		if err != nil || got.Budget.MaxBatches != limit {
			t.Fatalf("ordinary verify changed execution limit %d from model proposal: got=%+v err=%v", limit, got.Budget, err)
		}
	}
}

func TestHMC292BudgetAdmissionIsAtomicAcrossCreationActions(t *testing.T) {
	for _, action := range []WorkflowAction{ActionExploreCode, ActionPlanBatch, ActionAppendBatch, ActionSplitBatch, ActionReplanBatch, ActionApplyPlan, ActionVerifyBatch, ActionAskUser} {
		t.Run(string(action), func(t *testing.T) {
			run := hmc292Run(1, 0)
			decision := WriteWorkflowDecision{Action: action, Batch: &WriteBatchPlan{ID: "next", Goal: "bounded next task"}}
			if action == ActionExploreCode {
				decision.ExplorationRequest = &types.WriteExplorationRequest{BatchID: "next", Goal: "bounded read"}
			}
			if action == ActionApplyPlan || action == ActionVerifyBatch || action == ActionAskUser {
				run.ActiveBatchID = "next"
			}
			if action == ActionAskUser {
				decision.QuestionsForUser = []string{"Choose the target"}
				decision.MissingFacts = []MissingUserFact{{Kind: "runtime_constraint", Description: "target is not present in evidence", Consumer: "controller"}}
			}
			before, _ := json.Marshal(run)
			got, err := ApplyWorkflowDecisionToRun(run, decision)
			var budgetErr *BatchBudgetExceededError
			if !errors.As(err, &budgetErr) || budgetErr.Used != 1 || budgetErr.Limit != 1 || budgetErr.BatchID != "next" {
				t.Fatalf("new batch admitted beyond persistent limit: %+v", got)
			}
			after, _ := json.Marshal(run)
			if string(before) != string(after) || !reflect.DeepEqual(got, run) {
				t.Fatalf("rejected decision mutated input/returned run: err=%v input=%s result=%+v", err, after, got)
			}
		})
	}
}

func TestHMC292BudgetRosterAndHistoricalConsumptionShareAdmission(t *testing.T) {
	for _, historical := range []int{0, 4} {
		run := hmc292Run(5, historical)
		// Echoed workflow plans are proposals, not multiple committed batches.
		decision := WriteWorkflowDecision{Action: ActionSplitBatch, Batch: &WriteBatchPlan{ID: "child-1", Goal: "first child"}, Workflow: WriteWorkflowPlan{MaxBatches: 99, PlannedBatches: []WriteBatchPlan{{ID: "uncommitted", Goal: "proposal only"}}}}
		got, err := ApplyWorkflowDecisionToRun(run, decision)
		wantUsed := 2
		if historical > 1 {
			wantUsed = historical + 1
		}
		if err != nil || len(got.Batches) != 2 || got.Budget.BatchesUsed != wantUsed || got.Budget.MaxBatches != 5 {
			t.Fatalf("roster/history accounting mismatch: historical=%d got=%+v err=%v", historical, got, err)
		}
		// Reusing a child does not consume capacity, even at the exact ceiling.
		reused, err := ApplyWorkflowDecisionToRun(got, WriteWorkflowDecision{Action: ActionReplanBatch, Batch: &WriteBatchPlan{ID: "child-1", Goal: "same child"}})
		if err != nil || reused.Budget.BatchesUsed != wantUsed || len(reused.Batches) != 2 {
			t.Fatalf("existing child consumed budget: %+v %v", reused, err)
		}
		if historical == 4 {
			rejected, err := ApplyWorkflowDecisionToRun(got, WriteWorkflowDecision{Action: ActionSplitBatch, Batch: &WriteBatchPlan{ID: "child-2", Goal: "second child"}})
			if err == nil || !reflect.DeepEqual(rejected, got) {
				t.Fatalf("second child bypassed historical ceiling: %+v %v", rejected, err)
			}
		}
	}
	// No configured bound remains compatible for standalone reducer callers.
	over := hmc292Run(1, 9)
	existing, err := ApplyWorkflowDecisionToRun(over, WriteWorkflowDecision{Action: ActionReplanBatch, Batch: &WriteBatchPlan{ID: "source", Goal: "existing source"}})
	if err != nil || existing.Budget.BatchesUsed != 9 || len(existing.Batches) != 1 {
		t.Fatalf("historical consumption forbids existing ID reuse or was refunded: %+v %v", existing, err)
	}
	run, err := ApplyWorkflowDecisionToRun(types.WriteWorkflowRun{RunID: "standalone"}, WriteWorkflowDecision{Action: ActionPlanBatch, Batch: &WriteBatchPlan{ID: "one", Goal: "plan"}})
	if err != nil || len(run.Batches) != 1 || run.Budget.MaxBatches != 0 {
		t.Fatalf("unbounded standalone changed: %+v %v", run, err)
	}
}
