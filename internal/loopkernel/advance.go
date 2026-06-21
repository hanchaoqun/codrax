package loopkernel

import (
	"context"
	"strings"
	"time"
)

type LoopAdvanceInput struct {
	Events          []LoopEvent           `json:"events,omitempty"`
	RequestedAction LoopRecommendedAction `json:"requested_action,omitempty"`
	Now             time.Time             `json:"now,omitempty"`
}

type LoopAdvanceDecision struct {
	Allowed           bool                  `json:"allowed"`
	RecommendedAction LoopRecommendedAction `json:"recommended_action,omitempty"`
	ReasonCode        string                `json:"reason_code,omitempty"`
	State             LoopStateView         `json:"state,omitempty"`
	BudgetDecision    LoopBudgetDecision    `json:"budget_decision,omitempty"`
	Budget            LoopBudget            `json:"budget,omitempty"`
}

func (run LoopRun) Advance(ctx context.Context, input LoopAdvanceInput) (LoopRun, LoopAdvanceDecision) {
	state := ReduceEvents(input.Events)
	action := normalizeLoopRecommendedAction(input.RequestedAction)
	if action == "" {
		action = RecommendLoopAction(state)
	}
	if run.RunID == "" {
		run.RunID = state.RunID
	}
	run.ActiveUnitID = firstLoopNonEmpty(state.ActiveUnitID, run.ActiveUnitID)
	run.Status = loopRunStatusFromState(state, run.Status)
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	run.UpdatedAt = now
	budget := NormalizeLoopBudget(run.Budget)
	if kind, ok := loopAdvanceSpendKind(action); ok {
		var budgetDecision LoopBudgetDecision
		budget, budgetDecision = budget.Spend(ctx, now, kind)
		run.Budget = budget
		if !budgetDecision.Allowed {
			run.Status = LoopRunStatusBlocked
			return run, LoopAdvanceDecision{
				Allowed:           false,
				RecommendedAction: LoopActionBlock,
				ReasonCode:        budgetDecision.ReasonCode,
				State:             state,
				BudgetDecision:    budgetDecision,
				Budget:            budget,
			}
		}
		return run, LoopAdvanceDecision{
			Allowed:           true,
			RecommendedAction: action,
			ReasonCode:        loopAdvanceReasonCode(state, action),
			State:             state,
			BudgetDecision:    budgetDecision,
			Budget:            budget,
		}
	}
	run.Budget = budget
	return run, LoopAdvanceDecision{
		Allowed:           action != LoopActionBlock,
		RecommendedAction: action,
		ReasonCode:        loopAdvanceReasonCode(state, action),
		State:             state,
		Budget:            budget,
	}
}

func RecommendLoopAction(state LoopStateView) LoopRecommendedAction {
	switch state.Status {
	case LoopRunStatusComplete:
		return LoopActionContinue
	case LoopRunStatusBlocked:
		return LoopActionBlock
	}
	switch state.Permission.State {
	case PermissionAuthorityDeny:
		return LoopActionBlock
	case PermissionAuthorityAsk:
		return LoopActionAskApproval
	}
	for _, unit := range state.Units {
		if strings.TrimSpace(unit.UnitID) != strings.TrimSpace(state.ActiveUnitID) {
			continue
		}
		switch unit.Status {
		case RuntimeUnitBlocked:
			return LoopActionBlock
		case RuntimeUnitRepair:
			return LoopActionRepair
		case RuntimeUnitApplying, RuntimeUnitObserving:
			return LoopActionVerify
		}
	}
	switch state.Proof.State {
	case ProofCoverageFailed:
		return LoopActionRepair
	}
	if loopLocalizationActionable(state.Localization) {
		return LoopActionLocalize
	}
	switch state.Proof.State {
	case ProofCoverageMissing:
		return LoopActionVerify
	case ProofCoverageWeak:
		return LoopActionAddProof
	}
	return LoopActionContinue
}

func loopLocalizationActionable(view LocalizationAuthorityView) bool {
	if !view.RequiresMoreContext {
		return false
	}
	return len(view.SourcePaths) > 0 ||
		len(view.OwnerMissingPaths) > 0 ||
		len(view.AuxiliaryPaths) > 0 ||
		len(view.OwnerSupportedPaths) > 0
}

func loopAdvanceSpendKind(action LoopRecommendedAction) (LoopBudgetSpendKind, bool) {
	switch normalizeLoopRecommendedAction(action) {
	case LoopActionAskApproval:
		return LoopBudgetSpendApproval, true
	case LoopActionRepair:
		return LoopBudgetSpendRepair, true
	case LoopActionLocalize, LoopActionVerify, LoopActionAddProof:
		return LoopBudgetSpendUnit, true
	default:
		return "", false
	}
}

func loopAdvanceReasonCode(state LoopStateView, action LoopRecommendedAction) string {
	switch normalizeLoopRecommendedAction(action) {
	case LoopActionBlock:
		return firstLoopNonEmpty(state.Permission.ReasonCode, state.Truth.ReasonCode, state.Proof.ReasonCode, "loop_action_block")
	case LoopActionAskApproval:
		return firstLoopNonEmpty(state.Permission.ReasonCode, "loop_action_ask_approval")
	case LoopActionLocalize:
		return firstLoopNonEmpty(state.Localization.ReasonCode, "loop_action_localize")
	case LoopActionVerify, LoopActionAddProof, LoopActionRepair:
		return firstLoopNonEmpty(state.Proof.ReasonCode, state.Truth.ReasonCode, "loop_action_"+string(action))
	default:
		return "loop_action_continue"
	}
}

func loopRunStatusFromState(state LoopStateView, fallback LoopRunStatus) LoopRunStatus {
	switch state.Status {
	case LoopRunStatusComplete, LoopRunStatusBlocked, LoopRunStatusInProgress, LoopRunStatusSeeded:
		return state.Status
	default:
		if fallback != "" {
			return fallback
		}
		return LoopRunStatusSeeded
	}
}

func normalizeLoopRecommendedAction(action LoopRecommendedAction) LoopRecommendedAction {
	switch action {
	case LoopActionContinue, LoopActionLocalize, LoopActionVerify, LoopActionAddProof,
		LoopActionRepair, LoopActionAskApproval, LoopActionBlock:
		return action
	default:
		return ""
	}
}

func firstLoopNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
