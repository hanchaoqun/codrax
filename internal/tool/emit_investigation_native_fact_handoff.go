package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

type completionNativeFactHandoff struct {
	RuntimeMeasurements []types.AnswerRuntimeMeasurementReceipt `json:"runtime_measurements"`
	Interpretation      *string                                 `json:"interpretation"`
}

func completionNativeFactHandoffSelection(ctx *types.BusContext, p *completionNativeFactHandoff) (*types.InvestigationNativeFactHandoff, error) {
	required := ManagedDeliveryRequiresNativeFactHandoff(ctx)
	if p == nil {
		if required {
			return nil, managedDeliveryNativeHandoffError{required: true, violations: []nativeFactHandoffViolation{{"native_fact_handoff", "missing_handoff"}}, choices: NativeFactHandoffChoices(ctx)}
		}
		return nil, nil
	}
	fail := func(issues ...nativeFactHandoffViolation) (*types.InvestigationNativeFactHandoff, error) {
		return nil, managedDeliveryNativeHandoffError{required: required, violations: issues, choices: NativeFactHandoffChoices(ctx)}
	}
	if p.Interpretation == nil {
		return fail(nativeFactHandoffViolation{"native_fact_handoff.interpretation", "missing_interpretation"})
	}
	if ctx == nil || ctx.Mutable == nil {
		return fail(nativeFactHandoffViolation{"native_fact_handoff.runtime_measurements", "no_current_contract"})
	}
	h := &types.InvestigationNativeFactHandoff{RuntimeMeasurements: p.RuntimeMeasurements, Interpretation: *p.Interpretation}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if view == nil {
		return fail(nativeFactHandoffViolation{"native_fact_handoff.runtime_measurements", "no_current_contract"})
	}
	if _, ok := types.BindInvestigationNativeFactHandoff(h, view.RuntimeMeasurementContract); !ok {
		return fail(nativeFactHandoffSelectionViolations(h, view.RuntimeMeasurementContract)...)
	}
	return h.Clone(), nil
}

// Diagnose only after the shared binder rejects. This is field-local feedback,
// not a second authority gate or a fuzzy replacement selector.
func nativeFactHandoffSelectionViolations(h *types.InvestigationNativeFactHandoff, c *types.RuntimeMeasurementContract) []nativeFactHandoffViolation {
	const field = "native_fact_handoff.runtime_measurements"
	if len(h.RuntimeMeasurements) == 0 {
		return []nativeFactHandoffViolation{{field, "empty_selection"}}
	}
	if len(h.RuntimeMeasurements) > 16 {
		return []nativeFactHandoffViolation{{field, "selection_limit_16"}}
	}
	var issues []nativeFactHandoffViolation
	seen := map[string]bool{}
	for i, r := range h.RuntimeMeasurements {
		code := ""
		key := r.ObservationID + "\x00" + string(r.View)
		switch {
		case seen[key]:
			code = "duplicate"
		case r.EventSelection != nil:
			code = "event_selection"
		case !types.BindRuntimeMeasurementReceipt(&r, c) || !r.IsBound():
			code = "unknown_id"
			for _, choice := range c.Choices() {
				if choice.ObservationID == r.ObservationID {
					code = "wrong_view"
					break
				}
			}
		}
		seen[key] = true
		if code != "" {
			issues = append(issues, nativeFactHandoffViolation{fmt.Sprintf("%s[%d]", field, i), code})
		}
	}
	return issues
}
