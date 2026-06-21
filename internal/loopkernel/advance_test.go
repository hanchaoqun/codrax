package loopkernel

import (
	"context"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestLoopRunAdvanceFromWriteWorkflowProjection(t *testing.T) {
	now := time.Date(2026, 6, 21, 13, 0, 0, 0, time.UTC)
	writeRun := types.WriteWorkflowRun{
		RunID:         "wf-approval",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchPendingApproval,
			PlanID: "plan-1",
		}},
	}
	loopRun := LoopRun{
		RunID:  "wf-approval",
		Mode:   "write",
		Status: LoopRunStatusSeeded,
		Budget: NewLoopBudget(LoopBudgetOptions{
			MaxApprovals: 1,
			Now:          now,
			MaxElapsed:   time.Minute,
		}),
	}
	next, decision := loopRun.Advance(context.Background(), LoopAdvanceInput{
		Events: EventsFromWriteWorkflowRun(writeRun),
		Now:    now,
	})
	if !decision.Allowed || decision.RecommendedAction != LoopActionAskApproval {
		t.Fatalf("advance decision = %+v, want approval", decision)
	}
	if decision.BudgetDecision.Kind != LoopBudgetSpendApproval || next.Budget.ApprovalsUsed != 1 {
		t.Fatalf("approval budget not spent: next=%+v decision=%+v", next.Budget, decision.BudgetDecision)
	}
	if next.Status != LoopRunStatusInProgress || next.ActiveUnitID != "batch:batch-1" {
		t.Fatalf("loop run state not advanced from write workflow projection: %+v", next)
	}
}

func TestLoopRunAdvanceBlocksOnTypedBudget(t *testing.T) {
	now := time.Date(2026, 6, 21, 13, 0, 0, 0, time.UTC)
	writeRun := types.WriteWorkflowRun{
		RunID:         "wf-repair",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchReadyToPlan,
			PlanID:        "plan-1",
			ActiveSliceID: "slice-1",
			Slices: []types.WriteWorkflowSlice{{
				ID:        "slice-1",
				Status:    types.ChangePlanSliceFailed,
				VerifyRef: "report-1",
			}},
		}},
	}
	loopRun := LoopRun{
		RunID:  "wf-repair",
		Mode:   "write",
		Status: LoopRunStatusInProgress,
		Budget: NewLoopBudget(LoopBudgetOptions{
			MaxRepairs: 1,
			Now:        now,
			MaxElapsed: time.Minute,
		}),
	}
	loopRun.Budget.RepairsUsed = 1
	next, decision := loopRun.Advance(context.Background(), LoopAdvanceInput{
		Events: EventsFromWriteWorkflowRun(writeRun),
		Now:    now,
	})
	if decision.Allowed || decision.RecommendedAction != LoopActionBlock {
		t.Fatalf("advance decision = %+v, want budget block", decision)
	}
	if decision.ReasonCode != LoopBudgetReasonRepairsExhausted ||
		decision.BudgetDecision.ReasonCode != LoopBudgetReasonRepairsExhausted {
		t.Fatalf("wrong budget reason: %+v", decision)
	}
	if next.Status != LoopRunStatusBlocked || next.Budget.RepairsUsed != 1 {
		t.Fatalf("blocked loop run mismatch: %+v", next)
	}
}

func TestLoopRunAdvanceRecommendsActionableLocalization(t *testing.T) {
	now := time.Date(2026, 6, 21, 13, 0, 0, 0, time.UTC)
	writeRun := types.WriteWorkflowRun{
		RunID:         "wf-localize",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchReadyToPlan,
			ExpectedPaths: []string{"pkg/maybe.py"},
		}},
	}
	loopRun := LoopRun{
		RunID:  "wf-localize",
		Mode:   "write",
		Status: LoopRunStatusSeeded,
		Budget: NewLoopBudget(LoopBudgetOptions{MaxUnits: 2, Now: now, MaxElapsed: time.Minute}),
	}
	next, decision := loopRun.Advance(context.Background(), LoopAdvanceInput{
		Events: EventsFromWriteWorkflowRun(writeRun),
		Now:    now,
	})
	if !decision.Allowed || decision.RecommendedAction != LoopActionLocalize {
		t.Fatalf("advance decision = %+v, want localize", decision)
	}
	if next.Budget.UnitsUsed != 1 || decision.BudgetDecision.Kind != LoopBudgetSpendUnit {
		t.Fatalf("localize should spend one unit: next=%+v decision=%+v", next.Budget, decision.BudgetDecision)
	}
}

func TestLoopRunAdvanceRecommendsVerifyForMissingProof(t *testing.T) {
	now := time.Date(2026, 6, 21, 13, 0, 0, 0, time.UTC)
	writeRun := types.WriteWorkflowRun{
		RunID:         "wf-verify",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchVerifying,
			PlanID: "plan-1",
		}},
	}
	loopRun := LoopRun{
		RunID:  "wf-verify",
		Mode:   "write",
		Status: LoopRunStatusSeeded,
		Budget: NewLoopBudget(LoopBudgetOptions{MaxUnits: 2, Now: now, MaxElapsed: time.Minute}),
	}
	next, decision := loopRun.Advance(context.Background(), LoopAdvanceInput{
		Events: EventsFromWriteWorkflowRun(writeRun),
		Now:    now,
	})
	if !decision.Allowed || decision.RecommendedAction != LoopActionVerify {
		t.Fatalf("advance decision = %+v, want verify", decision)
	}
	if next.Budget.UnitsUsed != 1 || decision.BudgetDecision.Kind != LoopBudgetSpendUnit {
		t.Fatalf("verify should spend one unit: next=%+v decision=%+v", next.Budget, decision.BudgetDecision)
	}
}
