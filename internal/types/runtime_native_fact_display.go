package types

// RuntimeNativeFactDisplaySelections is a display default, not a hard gate.
// Required dimensions select this finite presentation domain; no labels,
// request text, answer prose or inferred keyword relevance is inspected.
func RuntimeNativeFactDisplaySelections(rm *RequestModel, contract *RuntimeMeasurementContract) []AnswerRuntimeMeasurementReceipt {
	if rm == nil || !rm.RuntimeQuestionProfile.BoundedFactSet() || rm.SourceInventoryProfile.Active() ||
		!rm.RequestedAnswerDimensions.Active() {
		return nil
	}
	requested := false
	for _, dimension := range rm.RequestedAnswerDimensions.Dimensions {
		if !dimension.Required {
			continue
		}
		switch dimension.Role {
		case RequestedAnswerDimensionObservedValue, RequestedAnswerDimensionMemberSet, RequestedAnswerDimensionCount:
			requested = true
		}
	}
	if !requested {
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
	return selected
}

const RuntimeNativeFactDisplayTeaching = "For the current requested finite facts, the system automatically displays the published default native tables after your answer: all retained rows, original values, time intervals, unknown fields and coverage boundaries. Do not retype those tables or infer names, units, state meanings, time alignment or causality. Write the business interpretation and supported relationships in separate blocks. You may still choose an exact runtime_measurement table to position that view yourself; the same observation_id/view is displayed only once. A summary does not replace the full record view. Independent source-code and causal obligations remain unchanged."
