package tool

import "github.com/hanchaoqun/codrax/internal/types"

const runtimeEventSelectionTeaching = "For a published native event inventory, event_selection chooses row_ids and field_keys from that same inventory; omitted arrays select all retained rows/fields. Alternatively group_by (without field_keys) counts selected retained rows by exact typed fields, not a full query when coverage is partial. Use the same selector on an available summary, table or diagram: the system renders facts and source/window. Omit text/items/columns/diagram and causal relation metadata; a fact diagram has no causal edges. If the principal summary requires a causal caliber, only no_causal_conclusion applies to this factual selector. Selection never proves a business filter, complete membership, or root cause."

func runtimeEventCaliberCompatible(kind types.AnswerBlockKind, caliber string) bool {
	return caliber == "" || kind == types.BlockSummary && caliber == string(types.TraceCausalClaimNoConclusion)
}

func runtimeEventChoicesAvailable(view *types.AnswerSemanticView) bool {
	if view == nil {
		return false
	}
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View, EventSelection: &types.RuntimeEventSelection{}}
		if types.BindRuntimeMeasurementReceipt(ref, view.RuntimeMeasurementContract) {
			return true
		}
	}
	return false
}

func runtimeEventSelectorSurface(kind types.AnswerBlockKind, ref *types.AnswerRuntimeMeasurementReceipt) bool {
	return ref != nil && ref.EventSelection != nil && (kind == types.BlockSummary || kind == types.BlockTable || kind == types.BlockDiagram)
}

func runtimeEventSelectorRequiredSchema() map[string]any {
	return map[string]any{"required": []string{"runtime_measurement"}, "properties": map[string]any{
		"runtime_measurement": map[string]any{"required": []string{"event_selection"}},
	}}
}

// Every branch pins one producer pair. The selector contains identities only,
// never replacement values, formulas, source coordinates or causal labels.
func runtimeEventSelectorSchema(base map[string]any, choices []types.RuntimeMeasurementTable, contract *types.RuntimeMeasurementContract) map[string]any {
	var eventIDs []string
	var conditions []any
	for _, table := range choices {
		ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View, EventSelection: &types.RuntimeEventSelection{}}
		if !types.BindRuntimeMeasurementReceipt(ref, contract) || table.EventData == nil {
			continue
		}
		var rowIDs, fieldKeys []string
		for _, row := range table.EventData.Rows {
			rowIDs = append(rowIDs, row.ID)
		}
		for _, field := range table.EventData.Fields {
			fieldKeys = append(fieldKeys, field.Key)
		}
		selection := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
			"row_ids":    runtimeEventChoiceArray(rowIDs, types.TraceEventSearchInventoryRowLimit),
			"field_keys": runtimeEventChoiceArray(fieldKeys, 128),
			"group_by":   runtimeEventChoiceArray(fieldKeys, 4),
		}}
		selection["if"] = map[string]any{"required": []string{"group_by"}, "properties": map[string]any{"group_by": map[string]any{"minItems": 1}}}
		selection["then"] = map[string]any{"properties": map[string]any{"field_keys": map[string]any{"maxItems": 0}}}
		eventIDs = append(eventIDs, table.ObservationID)
		conditions = append(conditions, map[string]any{
			"if":   map[string]any{"properties": map[string]any{"observation_id": map[string]any{"const": table.ObservationID}}},
			"then": map[string]any{"properties": map[string]any{"event_selection": selection}},
		})
	}
	return map[string]any{
		"description": runtimeMeasurementTeaching + " " + runtimeEventSelectionTeaching,
		"type":        "object", "additionalProperties": false, "required": []string{"observation_id", "view"},
		"properties": map[string]any{"observation_id": map[string]any{"type": "string"}, "view": map[string]any{"type": "string"}, "event_selection": map[string]any{"type": "object"}},
		"if":         map[string]any{"required": []string{"event_selection"}},
		"then": map[string]any{"allOf": conditions, "properties": map[string]any{
			"observation_id": map[string]any{"enum": eventIDs}, "view": map[string]any{"const": "members"},
		}}, "else": base,
	}
}

func runtimeEventChoiceArray(choices []string, maximum int) map[string]any {
	if len(choices) == 0 {
		return map[string]any{"type": "array", "maxItems": 0}
	}
	return map[string]any{"type": "array", "maxItems": maximum, "uniqueItems": true, "items": map[string]any{"type": "string", "enum": choices}}
}
