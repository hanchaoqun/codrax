package tool

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

type completionNativeFactHandoff struct {
	RuntimeMeasurements []types.AnswerRuntimeMeasurementReceipt `json:"runtime_measurements"`
	AnswerDisplay       []types.AnswerRuntimeMeasurementReceipt `json:"answer_display,omitempty"`
	Interpretation      json.RawMessage                         `json:"interpretation"`
	InterpretationRole  types.InvestigationInterpretationRole   `json:"interpretation_role,omitempty"`
	InterpretationUnits []types.InvestigationInterpretationUnit `json:"interpretation_units,omitempty"`
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
		return nil, managedDeliveryNativeHandoffError{required: required, violations: issues, choices: NativeFactHandoffChoices(ctx), displayChoices: NativeFactAnswerDisplayChoices(ctx)}
	}
	// Presence is not content: omitted legacy text is valid when units carry
	// the interpretation, but an explicitly wrong scalar must not disappear
	// behind those units. Keep the legacy empty string as explicit no analysis.
	legacy := bytes.TrimSpace(p.Interpretation)
	var interpretation string
	if len(legacy) > 0 && (bytes.Equal(legacy, []byte("null")) || json.Unmarshal(legacy, &interpretation) != nil) {
		return fail(nativeFactHandoffViolation{"native_fact_handoff.interpretation", "expected_string"})
	}
	if !p.InterpretationRole.Valid() {
		return fail(nativeFactHandoffViolation{"native_fact_handoff.interpretation_role", "unknown_role"})
	}
	var unitIssues []nativeFactHandoffViolation
	if len(p.InterpretationUnits) > types.MaxInvestigationInterpretationUnits {
		unitIssues = append(unitIssues, nativeFactHandoffViolation{"native_fact_handoff.interpretation_units", "selection_limit_16"})
	}
	for i, unit := range p.InterpretationUnits {
		if !unit.Valid() {
			unitIssues = append(unitIssues, nativeFactHandoffViolation{fmt.Sprintf("native_fact_handoff.interpretation_units[%d]", i), "invalid_unit_shape"})
		}
	}
	if len(unitIssues) > 0 {
		return fail(unitIssues...)
	}
	if len(legacy) == 0 && len(p.InterpretationUnits) == 0 {
		return fail(nativeFactHandoffViolation{"native_fact_handoff.interpretation", "missing_interpretation"})
	}
	if ctx == nil || ctx.Mutable == nil {
		return fail(nativeFactHandoffViolation{"native_fact_handoff.runtime_measurements", "no_current_contract"})
	}
	h := &types.InvestigationNativeFactHandoff{RuntimeMeasurements: p.RuntimeMeasurements, AnswerDisplay: p.AnswerDisplay, Interpretation: interpretation, InterpretationRole: p.InterpretationRole, InterpretationUnits: p.InterpretationUnits}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if view == nil {
		return fail(nativeFactHandoffViolation{"native_fact_handoff.runtime_measurements", "no_current_contract"})
	}
	if _, ok := types.BindInvestigationNativeFactHandoff(h, view.RuntimeMeasurementContract); !ok {
		return fail(nativeFactHandoffSelectionViolations(h, view.RuntimeMeasurementContract)...)
	}
	if len(h.AnswerDisplay) > 0 && contextFromBus(ctx).Err() != nil || !types.SealInvestigationNativeAnswerDisplay(h, nativeAnswerDisplayRequest(ctx), view.RuntimeMeasurementContract) {
		return fail(nativeFactHandoffViolation{"native_fact_handoff.answer_display", "not_current_requested_selected_view"})
	}
	return h.Clone(), nil
}

func nativeAnswerDisplayRequest(ctx *types.BusContext) *types.RequestModel {
	if ctx != nil && ctx.AnalysisIR != nil {
		return &ctx.AnalysisIR.RequestModel
	}
	if ctx != nil && ctx.Mutable != nil {
		return ctx.Mutable.RequestModel()
	}
	return nil
}

// Match the existing bounded handoff roster, not a second guessed catalog.
func NativeFactAnswerDisplayChoices(ctx *types.BusContext) []types.AnswerRuntimeMeasurementReceipt {
	if ctx == nil || ctx.Mutable == nil || contextFromBus(ctx).Err() != nil {
		return nil
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if view == nil {
		return nil
	}
	allowed := map[string]bool{}
	for _, r := range types.NativeAnswerDisplayChoices(nativeAnswerDisplayRequest(ctx), view.RuntimeMeasurementContract) {
		allowed[r.ObservationID+"\x00"+string(r.View)] = true
	}
	var out []types.AnswerRuntimeMeasurementReceipt
	for _, r := range NativeFactHandoffChoices(ctx) {
		if allowed[r.ObservationID+"\x00"+string(r.View)] {
			out = append(out, r)
		}
	}
	return out
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
