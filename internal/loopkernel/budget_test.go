package loopkernel

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool/sourceinventory"
)

func TestLoopBudgetEnforcesDeadlineLikeSourceInventory(t *testing.T) {
	now := time.Now()
	deadline := now.Add(-time.Second)
	loopBudget := NewLoopBudget(LoopBudgetOptions{
		MaxUnits:   1,
		DeadlineAt: deadline,
		Now:        now,
	})
	sourceBudget := sourceinventory.NewBudget(sourceinventory.BudgetOptions{
		Context:           context.Background(),
		TopN:              50,
		RepoFileCount:     10000,
		GraphFileCount:    10000,
		ForceAdvisoryOnly: true,
		Deadline:          deadline,
	})
	if !sourceBudget.Interrupted() {
		t.Fatal("sourceinventory budget should report the past deadline as interrupted")
	}
	next, decision := loopBudget.SpendUnit(context.Background(), now)
	if decision.Allowed || decision.ReasonCode != LoopBudgetReasonDeadlineExceeded {
		t.Fatalf("deadline decision = %+v, want deny deadline", decision)
	}
	if next.UnitsUsed != 0 {
		t.Fatalf("deadline-denied spend must not increment units: %+v", next)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	loopBudget = NewLoopBudget(LoopBudgetOptions{MaxUnits: 1, Now: now, MaxElapsed: time.Minute})
	next, decision = loopBudget.SpendUnit(ctx, now)
	if decision.Allowed || decision.ReasonCode != LoopBudgetReasonCanceled {
		t.Fatalf("cancel decision = %+v, want deny canceled", decision)
	}
	if next.InterruptedReason != LoopBudgetReasonCanceled || decision.Budget.InterruptedReason != LoopBudgetReasonCanceled {
		t.Fatalf("spend and decision should carry interrupted budget snapshot: next=%+v decision=%+v", next, decision)
	}
}

func TestLoopBudgetSpendCapsArePreciseAndSerializable(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	budget := NewLoopBudget(LoopBudgetOptions{
		MaxUnits:     1,
		MaxRepairs:   1,
		MaxApprovals: 1,
		MaxElapsed:   time.Minute,
		Now:          now,
	})
	var decision LoopBudgetDecision
	budget, decision = budget.SpendUnit(context.Background(), now)
	if !decision.Allowed || budget.UnitsUsed != 1 {
		t.Fatalf("first unit spend = budget %+v decision %+v, want allow", budget, decision)
	}
	budget, decision = budget.SpendUnit(context.Background(), now)
	if decision.Allowed || decision.ReasonCode != LoopBudgetReasonUnitsExhausted || budget.UnitsUsed != 1 {
		t.Fatalf("second unit spend = budget %+v decision %+v, want exhausted without increment", budget, decision)
	}
	budget, decision = budget.SpendRepair(context.Background(), now)
	if !decision.Allowed || budget.RepairsUsed != 1 {
		t.Fatalf("repair spend = budget %+v decision %+v, want allow", budget, decision)
	}
	budget, decision = budget.SpendApproval(context.Background(), now)
	if !decision.Allowed || budget.ApprovalsUsed != 1 {
		t.Fatalf("approval spend = budget %+v decision %+v, want allow", budget, decision)
	}
	_, decision = budget.Spend(context.Background(), now, LoopBudgetSpendKind("unknown"))
	if decision.Allowed || decision.ReasonCode != LoopBudgetReasonUnknownSpendKind {
		t.Fatalf("unknown spend decision = %+v, want typed denial", decision)
	}

	data, err := json.Marshal(budget)
	if err != nil {
		t.Fatalf("marshal budget: %v", err)
	}
	var roundTrip LoopBudget
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("unmarshal budget: %v", err)
	}
	roundTrip = NormalizeLoopBudget(roundTrip)
	if roundTrip.UnitsUsed != 1 || roundTrip.RepairsUsed != 1 || roundTrip.ApprovalsUsed != 1 || roundTrip.DeadlineAt.IsZero() {
		t.Fatalf("round-trip budget lost counters/deadline: %+v", roundTrip)
	}
}

func TestLoopBudgetDropsUnknownInterruptedReasonProse(t *testing.T) {
	budget := NormalizeLoopBudget(LoopBudget{InterruptedReason: "model said this is risky"})
	if budget.InterruptedReason != "" {
		t.Fatalf("unknown interrupted reason must not survive normalization: %+v", budget)
	}
}
