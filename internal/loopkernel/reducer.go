package loopkernel

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/types"
)

func ReduceEvents(events []LoopEvent) LoopStateView {
	events = NormalizeLoopEvents(events)
	view := LoopStateView{EventCount: len(events)}
	units := map[string]RuntimeUnitView{}
	for _, event := range events {
		if view.RunID == "" && event.RunID != "" {
			view.RunID = event.RunID
		}
		if event.RunID != "" {
			view.RunID = event.RunID
		}
		view.LastEventKind = event.Kind
		view.LastReasonCode = event.ReasonCode
		applyRunEvent(&view, event)
		applyUnitEvent(&view, units, event)
		applyAuthorityEvent(&view, event)
	}
	view.Units = loopUnitViews(units)
	if view.Status == "" && len(events) > 0 {
		view.Status = LoopRunStatusInProgress
	}
	return view
}

func applyRunEvent(view *LoopStateView, event LoopEvent) {
	switch event.Kind {
	case LoopEventRunSeeded:
		if view.Status == "" {
			view.Status = LoopRunStatusSeeded
		}
	case LoopEventRunCompleted:
		view.Status = LoopRunStatusComplete
	case LoopEventRunBlocked:
		view.Status = LoopRunStatusBlocked
	case LoopEventContextRequested, LoopEventContextObserved,
		LoopEventLocalizationProjected, LoopEventPlanRequested,
		LoopEventPlanEmitted, LoopEventUnitCreated,
		LoopEventEffectDescribed, LoopEventPermissionDecided,
		LoopEventApprovalRequested, LoopEventApprovalResolved,
		LoopEventCheckpointCreated, LoopEventUnitApplyStarted,
		LoopEventUnitApplyCompleted, LoopEventPatchEffectRecorded,
		LoopEventObserveStarted, LoopEventObserveCompleted,
		LoopEventProofProjected, LoopEventTruthProjected,
		LoopEventRepairRequested, LoopEventUnitCompleted:
		if view.Status == "" || view.Status == LoopRunStatusSeeded {
			view.Status = LoopRunStatusInProgress
		}
	case LoopEventUnitBlocked:
		if view.Status == "" || view.Status == LoopRunStatusSeeded {
			view.Status = LoopRunStatusInProgress
		}
	}
}

func applyUnitEvent(view *LoopStateView, units map[string]RuntimeUnitView, event LoopEvent) {
	if event.UnitID == "" {
		return
	}
	unit := units[event.UnitID]
	unit.UnitID = event.UnitID
	unit.ReasonCode = event.ReasonCode
	unit.UpdatedAt = event.At
	switch event.Kind {
	case LoopEventUnitCreated, LoopEventEffectDescribed, LoopEventPermissionDecided, LoopEventApprovalResolved:
		unit.Status = RuntimeUnitPending
		view.ActiveUnitID = event.UnitID
	case LoopEventUnitApplyStarted, LoopEventUnitApplyCompleted, LoopEventPatchEffectRecorded:
		unit.Status = RuntimeUnitApplying
		view.ActiveUnitID = event.UnitID
	case LoopEventObserveStarted, LoopEventObserveCompleted, LoopEventProofProjected, LoopEventTruthProjected:
		unit.Status = RuntimeUnitObserving
		view.ActiveUnitID = event.UnitID
		if event.Kind == LoopEventTruthProjected && len(event.Payload) > 0 {
			var ledger types.TruthLedger
			if json.Unmarshal(event.Payload, &ledger) == nil {
				ledger = types.NormalizeTruthLedger(ledger)
				if ledger.State != types.TruthLedgerUnknown {
					unit.Truth = ledger
				}
			}
		}
	case LoopEventRepairRequested:
		unit.Status = RuntimeUnitRepair
		view.ActiveUnitID = event.UnitID
	case LoopEventUnitCompleted:
		unit.Status = RuntimeUnitComplete
	case LoopEventUnitBlocked:
		unit.Status = RuntimeUnitBlocked
		view.ActiveUnitID = event.UnitID
	default:
		if unit.Status == "" {
			unit.Status = RuntimeUnitPending
		}
	}
	units[event.UnitID] = unit
}

func applyAuthorityEvent(view *LoopStateView, event LoopEvent) {
	if len(event.Payload) == 0 {
		return
	}
	switch event.Kind {
	case LoopEventLocalizationProjected:
		var authority LocalizationAuthorityView
		if json.Unmarshal(event.Payload, &authority) == nil && authority.State != "" {
			view.Localization = authority
		}
	case LoopEventProofProjected:
		var authority ProofCoverageAuthorityView
		if json.Unmarshal(event.Payload, &authority) == nil && authority.State != "" {
			view.Proof = authority
		}
	case LoopEventPermissionDecided:
		var authority PermissionAuthorityView
		if json.Unmarshal(event.Payload, &authority) == nil && authority.State != "" {
			view.Permission = authority
		}
	case LoopEventTruthProjected:
		var ledger types.TruthLedger
		if json.Unmarshal(event.Payload, &ledger) == nil {
			ledger = types.NormalizeTruthLedger(ledger)
			if ledger.State != types.TruthLedgerUnknown {
				view.Truth = ledger
			}
		}
	}
}

func loopUnitViews(units map[string]RuntimeUnitView) []RuntimeUnitView {
	if len(units) == 0 {
		return nil
	}
	views := make([]RuntimeUnitView, 0, len(units))
	for _, unit := range units {
		views = append(views, unit)
	}
	sortRuntimeUnitViews(views)
	return views
}

func sortRuntimeUnitViews(views []RuntimeUnitView) {
	for i := 1; i < len(views); i++ {
		for j := i; j > 0 && views[j].UnitID < views[j-1].UnitID; j-- {
			views[j], views[j-1] = views[j-1], views[j]
		}
	}
}
