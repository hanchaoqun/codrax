package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

type completionNativeFactHandoff struct {
	RuntimeMeasurements []types.AnswerRuntimeMeasurementReceipt `json:"runtime_measurements"`
	MeasurementRefs     []string                                `json:"measurement_refs,omitempty"`
	AnswerDisplay       []types.AnswerRuntimeMeasurementReceipt `json:"answer_display,omitempty"`
	Interpretation      json.RawMessage                         `json:"interpretation"`
	InterpretationRole  types.InvestigationInterpretationRole   `json:"interpretation_role,omitempty"`
	InterpretationUnits []types.InvestigationInterpretationUnit `json:"interpretation_units,omitempty"`
}

// NativeFactMeasurementRef identifies an exact existing observation/view pair.
func NativeFactMeasurementRef(receipt types.AnswerRuntimeMeasurementReceipt) string {
	return receipt.ObservationID + "|" + string(receipt.View)
}

func completionNativeFactHandoffSelection(ctx *types.BusContext, p *completionNativeFactHandoff) (*types.InvestigationNativeFactHandoff, error) {
	required := ManagedDeliveryRequiresNativeFactHandoff(ctx)
	compact := UsesCompactNativeFactHandoff(ctx)
	if p == nil {
		if required {
			return nil, managedDeliveryNativeHandoffError{required: true, compact: compact, violations: []nativeFactHandoffViolation{{"native_fact_handoff", "missing_handoff"}}, choices: NativeFactHandoffChoices(ctx)}
		}
		return nil, nil
	}
	fail := func(issues ...nativeFactHandoffViolation) (*types.InvestigationNativeFactHandoff, error) {
		choices := NativeFactHandoffChoices(ctx)
		return nil, managedDeliveryNativeHandoffError{required: required, compact: compact, violations: issues, choices: choices, displayChoices: NativeFactAnswerDisplayChoices(ctx), coordinates: nativeFactHandoffCoordinateRepairs(p.RuntimeMeasurements, issues, choices)}
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
	fromRows := compact && p.RuntimeMeasurements == nil
	if p.MeasurementRefs != nil || fromRows {
		const field = "native_fact_handoff.measurement_refs"
		if p.RuntimeMeasurements != nil || (p.MeasurementRefs != nil && len(p.MeasurementRefs) == 0) || len(p.MeasurementRefs) > 16 {
			return fail(nativeFactHandoffViolation{field, "select_1_to_16_refs_without_runtime_measurements"})
		}
		byRef := make(map[string]types.AnswerRuntimeMeasurementReceipt)
		for _, table := range view.RuntimeMeasurementContract.Choices() {
			if table.EventData == nil {
				receipt := types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
				byRef[NativeFactMeasurementRef(receipt)] = receipt
			}
		}
		selected := make(map[string]bool)
		for i, ref := range p.MeasurementRefs {
			receipt, ok := byRef[ref]
			if !ok {
				return fail(nativeFactHandoffViolation{fmt.Sprintf("%s[%d]", field, i), "unknown_current_ref"})
			}
			h.RuntimeMeasurements = append(h.RuntimeMeasurements, receipt)
			selected[ref] = true
		}
		if fromRows {
			normalizeTraceObservationRefs(ctx, h.InterpretationUnits, byRef)
			// Each row citation already explicitly selects an exact table.
			// Add that receipt once. The normal seal validates the row address.
			for _, unit := range h.InterpretationUnits {
				for _, row := range unit.NativeRefs {
					ref := NativeFactMeasurementRef(types.AnswerRuntimeMeasurementReceipt{ObservationID: row.ObservationID, View: row.View})
					if receipt, current := byRef[ref]; current && !selected[ref] {
						h.RuntimeMeasurements = append(h.RuntimeMeasurements, receipt)
						selected[ref] = true
					}
				}
			}
		}
	}
	if _, ok := types.BindInvestigationNativeFactHandoff(h, view.RuntimeMeasurementContract); !ok {
		if fromRows && len(h.RuntimeMeasurements) == 0 {
			return fail(nativeFactHandoffViolation{"native_fact_handoff.measurement_refs", "select_current_refs_or_native_rows"})
		}
		return fail(nativeFactHandoffSelectionViolations(h, view.RuntimeMeasurementContract)...)
	}
	if ok, invalid := types.SealInvestigationNativeRowReferences(h, view.RuntimeMeasurementContract); !ok {
		return nil, managedDeliveryNativeHandoffError{
			required: required, compact: compact, choices: nativeFactHandoffChoices(view.RuntimeMeasurementContract),
			violations:  []nativeFactHandoffViolation{{"native_fact_handoff.interpretation_units.native_refs", "not_current_selected_native_row_address"}},
			invalidRows: invalid, rowUnits: h.InterpretationUnits,
		}
	}
	displayCount := len(h.AnswerDisplay)
	sealDisplay := types.SealInvestigationNativeAnswerDisplay
	if fromRows {
		sealDisplay = types.SealPreparedFrameAnswerDisplay
	}
	if displayCount > 0 && contextFromBus(ctx).Err() != nil || !sealDisplay(h, nativeAnswerDisplayRequest(ctx), view.RuntimeMeasurementContract) {
		return fail(nativeFactHandoffViolation{"native_fact_handoff.answer_display", "not_current_requested_selected_view"})
	}
	if dropped := displayCount - len(h.AnswerDisplay); dropped > 0 {
		logging.Debug("[emit_investigation_complete] omitted %d unavailable optional answer_display selections", dropped)
	}
	return h.Clone(), nil
}

// Some native Trace observations are published as one record without a table.
// Retain the exact observation as a locator when the model wrapped it in a
// single-row summary address. Table addresses retain their original checks.
func normalizeTraceObservationRefs(ctx *types.BusContext, units []types.InvestigationInterpretationUnit, tables map[string]types.AnswerRuntimeMeasurementReceipt) {
	isRawSummary := func(ref types.InvestigationNativeRowReference) bool {
		if ref.View != types.RuntimeMeasurementSummary || len(ref.RowIndices) != 1 || ref.RowIndices[0] != 0 {
			return false
		}
		_, hasTable := tables[ref.ObservationID+"|"+string(ref.View)]
		return !hasTable
	}
	wanted := map[string]bool{}
	for _, unit := range units {
		for _, ref := range unit.NativeRefs {
			if isRawSummary(ref) {
				wanted[ref.ObservationID] = true
			}
		}
	}
	if len(wanted) == 0 {
		return
	}
	current := map[string]bool{}
	for _, result := range types.ObservationLedgerInputFromBusContext(ctx, 0).ToolResults {
		for _, row := range result.Observations {
			if !wanted[row.ID] || current[row.ID] {
				continue
			}
			if ctx.Mutable.CurrentNativeTracePublication(result) {
				for _, observation := range result.Observations {
					current[observation.ID] = true
				}
			}
			break
		}
	}
	normalized := 0
	for i := range units {
		unit := &units[i]
		unit.NativeRefs = slices.DeleteFunc(unit.NativeRefs, func(ref types.InvestigationNativeRowReference) bool {
			if !isRawSummary(ref) || !current[ref.ObservationID] {
				return false
			}
			if !slices.Contains(unit.SupportRefs, ref.ObservationID) {
				unit.SupportRefs = append(unit.SupportRefs, ref.ObservationID)
			}
			normalized++
			return true
		})
	}
	if normalized > 0 {
		logging.Debug("[emit_investigation_complete] normalized %d raw Trace observations into support_refs", normalized)
	}
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
	return nativeFactAnswerDisplayChoices(ctx, view.RuntimeMeasurementContract, nativeFactHandoffChoices(view.RuntimeMeasurementContract))
}

func nativeFactAnswerDisplayChoices(ctx *types.BusContext, contract *types.RuntimeMeasurementContract, choices []types.AnswerRuntimeMeasurementReceipt) []types.AnswerRuntimeMeasurementReceipt {
	allowed := map[string]bool{}
	for _, r := range types.NativeAnswerDisplayChoices(nativeAnswerDisplayRequest(ctx), contract) {
		allowed[r.ObservationID+"\x00"+string(r.View)] = true
	}
	var out []types.AnswerRuntimeMeasurementReceipt
	for _, r := range choices {
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
