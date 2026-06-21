package loopkernel

import (
	"context"
	"strings"
	"time"
)

type LoopBudgetSpendKind string

const (
	LoopBudgetSpendUnit     LoopBudgetSpendKind = "unit"
	LoopBudgetSpendRepair   LoopBudgetSpendKind = "repair"
	LoopBudgetSpendApproval LoopBudgetSpendKind = "approval"

	LoopBudgetReasonAllowed            = "loop_budget_allowed"
	LoopBudgetReasonCanceled           = "loop_budget_canceled"
	LoopBudgetReasonDeadlineExceeded   = "loop_budget_deadline_exceeded"
	LoopBudgetReasonUnknownSpendKind   = "loop_budget_unknown_spend_kind"
	LoopBudgetReasonUnitsExhausted     = "loop_budget_units_exhausted"
	LoopBudgetReasonRepairsExhausted   = "loop_budget_repairs_exhausted"
	LoopBudgetReasonApprovalsExhausted = "loop_budget_approvals_exhausted"
)

type LoopBudgetOptions struct {
	MaxUnits     int
	MaxRepairs   int
	MaxApprovals int
	DeadlineAt   time.Time
	MaxElapsed   time.Duration
	Now          time.Time
}

type LoopBudgetDecision struct {
	Allowed    bool                `json:"allowed"`
	Kind       LoopBudgetSpendKind `json:"kind,omitempty"`
	ReasonCode string              `json:"reason_code,omitempty"`
	Budget     LoopBudget          `json:"budget,omitempty"`
}

func NewLoopBudget(options LoopBudgetOptions) LoopBudget {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	deadline := options.DeadlineAt
	if deadline.IsZero() && options.MaxElapsed > 0 {
		deadline = now.Add(options.MaxElapsed)
	}
	return NormalizeLoopBudget(LoopBudget{
		MaxUnits:     options.MaxUnits,
		MaxRepairs:   options.MaxRepairs,
		MaxApprovals: options.MaxApprovals,
		DeadlineAt:   deadline,
	})
}

func NormalizeLoopBudget(in LoopBudget) LoopBudget {
	if in.MaxUnits < 0 {
		in.MaxUnits = 0
	}
	if in.MaxRepairs < 0 {
		in.MaxRepairs = 0
	}
	if in.MaxApprovals < 0 {
		in.MaxApprovals = 0
	}
	if in.UnitsUsed < 0 {
		in.UnitsUsed = 0
	}
	if in.RepairsUsed < 0 {
		in.RepairsUsed = 0
	}
	if in.ApprovalsUsed < 0 {
		in.ApprovalsUsed = 0
	}
	in.InterruptedReason = normalizeLoopBudgetReason(in.InterruptedReason)
	return in
}

func (budget LoopBudget) Spend(ctx context.Context, now time.Time, kind LoopBudgetSpendKind) (LoopBudget, LoopBudgetDecision) {
	budget = NormalizeLoopBudget(budget)
	kind = LoopBudgetSpendKind(strings.TrimSpace(string(kind)))
	if decision := budget.Check(ctx, now); !decision.Allowed {
		decision.Kind = kind
		return decision.Budget, decision
	}
	switch kind {
	case LoopBudgetSpendUnit:
		if budget.MaxUnits > 0 && budget.UnitsUsed >= budget.MaxUnits {
			return budget, budgetDecision(false, kind, LoopBudgetReasonUnitsExhausted, budget)
		}
		budget.UnitsUsed++
	case LoopBudgetSpendRepair:
		if budget.MaxRepairs > 0 && budget.RepairsUsed >= budget.MaxRepairs {
			return budget, budgetDecision(false, kind, LoopBudgetReasonRepairsExhausted, budget)
		}
		budget.RepairsUsed++
	case LoopBudgetSpendApproval:
		if budget.MaxApprovals > 0 && budget.ApprovalsUsed >= budget.MaxApprovals {
			return budget, budgetDecision(false, kind, LoopBudgetReasonApprovalsExhausted, budget)
		}
		budget.ApprovalsUsed++
	default:
		return budget, budgetDecision(false, kind, LoopBudgetReasonUnknownSpendKind, budget)
	}
	budget = NormalizeLoopBudget(budget)
	return budget, budgetDecision(true, kind, LoopBudgetReasonAllowed, budget)
}

func (budget LoopBudget) SpendUnit(ctx context.Context, now time.Time) (LoopBudget, LoopBudgetDecision) {
	return budget.Spend(ctx, now, LoopBudgetSpendUnit)
}

func (budget LoopBudget) SpendRepair(ctx context.Context, now time.Time) (LoopBudget, LoopBudgetDecision) {
	return budget.Spend(ctx, now, LoopBudgetSpendRepair)
}

func (budget LoopBudget) SpendApproval(ctx context.Context, now time.Time) (LoopBudget, LoopBudgetDecision) {
	return budget.Spend(ctx, now, LoopBudgetSpendApproval)
}

func (budget LoopBudget) Check(ctx context.Context, now time.Time) LoopBudgetDecision {
	budget = NormalizeLoopBudget(budget)
	if budget.InterruptedReason != "" {
		return budgetDecision(false, "", budget.InterruptedReason, budget)
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			budget.InterruptedReason = LoopBudgetReasonCanceled
			return budgetDecision(false, "", LoopBudgetReasonCanceled, budget)
		}
	}
	if now.IsZero() {
		now = time.Now()
	}
	if !budget.DeadlineAt.IsZero() && now.After(budget.DeadlineAt) {
		budget.InterruptedReason = LoopBudgetReasonDeadlineExceeded
		return budgetDecision(false, "", LoopBudgetReasonDeadlineExceeded, budget)
	}
	return budgetDecision(true, "", LoopBudgetReasonAllowed, budget)
}

func normalizeLoopBudgetReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case LoopBudgetReasonAllowed:
		return ""
	case LoopBudgetReasonCanceled,
		LoopBudgetReasonDeadlineExceeded,
		LoopBudgetReasonUnknownSpendKind,
		LoopBudgetReasonUnitsExhausted,
		LoopBudgetReasonRepairsExhausted,
		LoopBudgetReasonApprovalsExhausted:
		return strings.TrimSpace(reason)
	default:
		return ""
	}
}

func budgetDecision(allowed bool, kind LoopBudgetSpendKind, reason string, budget LoopBudget) LoopBudgetDecision {
	reason = normalizeLoopBudgetReason(reason)
	if reason == "" && allowed {
		reason = LoopBudgetReasonAllowed
	}
	return LoopBudgetDecision{
		Allowed:    allowed,
		Kind:       kind,
		ReasonCode: reason,
		Budget:     NormalizeLoopBudget(budget),
	}
}
