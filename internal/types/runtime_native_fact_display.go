package types

// RuntimeNativeFactDisplaySelections is a display default, not a hard gate.
// A finite fact profile or required dimensions select this presentation domain; no labels,
// request text, answer prose or inferred keyword relevance is inspected.
func RuntimeNativeFactDisplaySelections(rm *RequestModel, contract *RuntimeMeasurementContract) []AnswerRuntimeMeasurementReceipt {
	if !RuntimeNativeFactDisplayDomain(rm) {
		return nil
	}
	var selected []AnswerRuntimeMeasurementReceipt
	choices := contract.Choices()
	defaults := map[string]int{}
	for _, table := range choices {
		if table.DefaultPresentation {
			defaults[table.ObservationID]++
		}
	}
	for _, table := range choices {
		if table.DefaultPresentation && defaults[table.ObservationID] == 1 {
			selected = append(selected, AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View})
		}
	}
	// Keep every original selector available, but display a complete exact
	// repeated population once. Query-envelope diagnostics remain in their
	// original publications; differing table notes do not share this identity.
	seen := map[string]bool{}
	out := selected[:0]
	for _, receipt := range selected {
		key := RuntimeMeasurementDisplayKey(receipt, contract)
		if !seen[key] {
			seen[key] = true
			out = append(out, receipt)
		}
	}
	return out
}

// This existing typed fact domain is shared by producer defaults and explicit
// investigation display intent. It does not confer any evidence authority.
func RuntimeNativeFactDisplayDomain(rm *RequestModel) bool {
	if rm == nil || rm.RuntimeQuestionProfile == nil || rm.SourceInventoryProfile.Active() {
		return false
	}
	// Overall analysis breadth does not erase an independently requested
	// finite fact dimension. This supplies display only, never causal scope.
	switch rm.RuntimeQuestionProfile.Scope {
	case RuntimeQuestionScopeBoundedFactSet, RuntimeQuestionScopeBoundedEffectVerdict,
		RuntimeQuestionScopeCausalDiagnosis, RuntimeQuestionScopeRelationAnalysis, RuntimeQuestionScopeSystemOverview:
	default:
		return false
	}
	// A bounded fact request already declares its finite domain. Do not require
	// the analyzer to repeat it in a second optional profile just for display.
	// Explicit dimensions still own presentation when present, and broader
	// analysis scopes continue to require an independent fact dimension.
	requested := !rm.RequestedAnswerDimensions.Active() &&
		rm.RuntimeQuestionProfile.Scope == RuntimeQuestionScopeBoundedFactSet &&
		len(rm.RuntimeQuestionProfile.FactFamilies) > 0
	if rm.RequestedAnswerDimensions.Active() {
		for _, dimension := range rm.RequestedAnswerDimensions.Dimensions {
			if !dimension.Required {
				continue
			}
			switch dimension.Role {
			case RequestedAnswerDimensionObservedValue, RequestedAnswerDimensionMemberSet, RequestedAnswerDimensionCount:
				requested = true
			}
		}
	}
	return requested
}

const RuntimeNativeFactDisplayTeaching = "For the current requested finite facts, the system automatically displays the published default native tables after your answer: all retained rows, original values, time intervals, unknown fields and coverage boundaries. Do not retype those tables or infer names, units, state meanings, time alignment or causality. Write the business interpretation and supported relationships in separate blocks. You may still choose an exact runtime_measurement table to position that view yourself; the same observation_id/view is displayed only once. A summary does not replace the full record view. Independent source-code and causal obligations remain unchanged."
