package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const runtimeMeasurementTeaching = "Optional trusted measurement table: select one published observation_id/view pair on kind=table; do not copy values or supply text/items/columns. The system renders exact measured values, members, distributions or timeline rows with source/window and coverage notes. Put interpretation and runtime_work_relation in separate blocks. This selection proves measurements only, never a root cause."

func projectRuntimeMeasurementField(blockItems, blockProps map[string]any, view *types.AnswerSemanticView) {
	delete(blockProps, "requested_dimension_indices")
	choices := view.RuntimeMeasurementContract.Choices()
	allowed := allowedKindSet(view)
	events := runtimeEventChoicesAvailable(view)
	sampling := runtimeFixedFactPairs(choices)
	factSurfaces := events || len(sampling) > 0
	if len(choices) == 0 || (len(allowed) > 0 && !allowed[string(types.BlockTable)] && !(factSurfaces && (allowed[string(types.BlockSummary)] || allowed[string(types.BlockDiagram)]))) {
		delete(blockProps, "runtime_measurement")
		return
	}
	pairs := make([]any, 0, len(choices))
	for _, table := range choices {
		pairs = append(pairs, map[string]any{"observation_id": table.ObservationID, "view": string(table.View)})
	}
	// Object enum keeps the pair exact with the executable schema subset;
	// independent field enums would permit an unpublished id/view combination.
	blockProps["runtime_measurement"] = map[string]any{
		"description": runtimeMeasurementTeaching, "type": "object", "enum": pairs,
		"properties": map[string]any{"observation_id": map[string]any{"type": "string"}, "view": map[string]any{"type": "string"}},
		"required":   []string{"observation_id", "view"}, "additionalProperties": false,
	}
	if events {
		blockProps["runtime_measurement"] = runtimeEventSelectorSchema(blockProps["runtime_measurement"].(map[string]any), choices, view.RuntimeMeasurementContract)
	}
	if len(sampling) > 0 {
		property := blockProps["runtime_measurement"].(map[string]any)
		samplingTeaching, resourceTeaching := false, false
		for _, table := range choices {
			samplingTeaching = samplingTeaching || table.HasNativeSamplingFacts()
			resourceTeaching = resourceTeaching || table.HasNativeResourceFacts()
		}
		if samplingTeaching {
			property["description"] = property["description"].(string) + " " + runtimeSamplingFactTeaching
		}
		if resourceTeaching {
			property["description"] = property["description"].(string) + " " + runtimeResourceFactTeaching
		}
	}
	if indices := view.RuntimeMeasurementContract.NativeDisplayDimensionChoices(); len(indices) > 0 {
		blockProps["requested_dimension_indices"] = map[string]any{
			"type": "array", "minItems": 1, "uniqueItems": true,
			"items":       map[string]any{"type": "integer", "enum": indices},
			"description": "Optional display ownership: choose the requested member-set dimension indices this exact complete native table answers. One table may answer several dimensions. Select the published runtime_measurement pair and these indices; do not copy its rows. This binds display only, never source-code, causal or investigation-completion obligations.",
		}
	}
	conditions := schemaAllOfEntries(blockItems)
	if indices := view.RuntimeMeasurementContract.NativeDisplayDimensionChoices(); len(indices) > 0 {
		var eligible []any
		for _, table := range choices {
			candidate := types.AnswerBlock{Kind: types.BlockTable, RuntimeMeasurement: &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}, RequestedDimensionIndices: indices}
			if _, valid := types.BoundNativeDisplayDimensions(candidate, view.RuntimeMeasurementContract); valid {
				eligible = append(eligible, map[string]any{"observation_id": table.ObservationID, "view": string(table.View)})
			}
		}
		conditions = append(conditions, map[string]any{
			"if":   map[string]any{"required": []string{"requested_dimension_indices"}},
			"then": map[string]any{"required": []string{"runtime_measurement"}, "properties": map[string]any{"runtime_measurement": map[string]any{"enum": eligible}}},
		})
	}
	payload := map[string]any{"properties": map[string]any{
		"kind": map[string]any{"const": string(types.BlockTable)},
		"text": map[string]any{"maxLength": 0}, "columns": map[string]any{"maxItems": 0}, "items": map[string]any{"maxItems": 0},
		"diagram": map[string]any{"enum": []any{nil}}, "runtime_work_relation": map[string]any{"enum": []any{nil}},
	}}
	if factSurfaces {
		props := payload["properties"].(map[string]any)
		delete(props, "kind")
		payload["if"] = runtimeMeasurementFactRequiredSchema(events, sampling)
		payload["then"] = map[string]any{"properties": map[string]any{"kind": map[string]any{"enum": []string{"summary", "table", "diagram"}}}}
		payload["else"] = map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "table"}}}
		factConditions := []any{runtimeMeasurementFactRequiredSchema(events, sampling)}
		if events && len(sampling) > 0 {
			// The executable validator supports nested if/then/else, not anyOf.
			payload["else"] = map[string]any{"if": runtimeEventSelectorRequiredSchema(), "then": payload["then"], "else": payload["else"]}
			factConditions = append(factConditions, runtimeEventSelectorRequiredSchema())
		}
		for _, condition := range factConditions {
			conditions = append(conditions, map[string]any{"if": condition, "then": map[string]any{"properties": map[string]any{
				"edge_anchors": map[string]any{"maxItems": 0}, "participant_boundaries": map[string]any{"maxItems": 0},
				"relation_claims": map[string]any{"maxItems": 0}, "claim_uses": map[string]any{"maxItems": 0},
				"trace_causal_claim_caliber": map[string]any{"enum": []string{string(types.TraceCausalClaimNoConclusion)}}, "requested_relation_scope": map[string]any{"maxLength": 0},
			}}})
		}
	}
	conditions = append(conditions, map[string]any{
		"if":   map[string]any{"required": []string{"runtime_measurement"}},
		"then": payload,
	})
	blockItems["allOf"] = conditions
	// Correct the old generic table teaching only for dispatches offering this
	// alternative. No duplicated mandatory fields or global prompt burden.
	if kind, ok := blockProps["kind"].(map[string]any); ok {
		kind["description"] = "Choose an available block kind. A table may select runtime_measurement alone for verified measured rows; otherwise it must carry an ordinary complete table in text or visible structured items. Prose uses text; lists use items; diagram uses diagram."
	}
	if items, ok := blockProps["items"].(map[string]any); ok {
		items["description"] = "Visible list/table rows. Omit for a runtime_measurement table because its selected provider owns every row. Otherwise retain the normal structured item contract."
	}
	if events {
		if kind, ok := blockProps["kind"].(map[string]any); ok {
			kind["description"] = "Choose an available block kind. A native runtime_measurement with event_selection owns its summary/table/diagram payload; otherwise follow the ordinary kind payload mapping."
		}
		if diagram, ok := blockProps["diagram"].(map[string]any); ok {
			diagram["description"] = "Ordinary diagram payload. Omit when a published runtime_measurement.event_selection owns the fact diagram."
		}
	}
	if len(sampling) > 0 {
		if kind, ok := blockProps["kind"].(map[string]any); ok {
			kind["description"] = kind["description"].(string) + " Published native fixed facts also own summary/table/diagram via runtime_measurement alone."
		}
		if diagram, ok := blockProps["diagram"].(map[string]any); ok {
			diagram["description"] = "Ordinary diagram payload; omit when a published native fact selector supplies the fact diagram."
		}
	}
}

func validateEmitRuntimeMeasurementBlock(raw emitAnswerBlockV2, path string, nativeSampling ...bool) error {
	receipt := raw.RuntimeMeasurement
	if receipt == nil {
		return nil
	}
	if strings.TrimSpace(receipt.ObservationID) == "" || !receipt.View.IsValid() {
		return fmt.Errorf("%s.runtime_measurement requires one published observation_id/view pair from the current choices", path)
	}
	sampling := len(nativeSampling) == 1 && nativeSampling[0]
	if (raw.Kind != string(types.BlockTable) && !runtimeEventSelectorSurface(types.AnswerBlockKind(raw.Kind), receipt) && !(sampling && runtimeMeasurementFactSurfaceCandidate(types.AnswerBlockKind(raw.Kind), receipt))) || raw.Text != "" || len(raw.Items) > 0 || len(raw.Columns) > 0 || raw.Diagram != nil || raw.RuntimeWorkRelation != nil {
		return fmt.Errorf("%s.runtime_measurement requires kind=table, or a published native fixed fact/event_selection on summary/table/diagram, without text/items/columns/diagram/runtime_work_relation; put explanations in separate blocks", path)
	}
	if (receipt.EventSelection != nil || raw.Kind != string(types.BlockTable)) && (len(raw.EdgeAnchors) > 0 || len(raw.ParticipantBoundaries) > 0 || len(raw.RelationClaims) > 0 || len(raw.ClaimUses) > 0 || !runtimeEventCaliberCompatible(types.AnswerBlockKind(raw.Kind), raw.TraceCausalClaimCaliber) || raw.RequestedRelationScope != "") {
		return fmt.Errorf("%s.runtime_measurement displays native facts only; omit causal claims and relation metadata", path)
	}
	return nil
}

func runtimeMeasurementBindingValid(block types.AnswerBlock, view *types.AnswerSemanticView) bool {
	if block.RuntimeMeasurement == nil {
		return len(block.RequestedDimensionIndices) == 0
	}
	if !runtimeMeasurementFactSurfaceCandidate(block.Kind, block.RuntimeMeasurement) || block.Text != "" || len(block.Items) > 0 || len(block.Columns) > 0 || block.Diagram != nil || block.RuntimeWorkRelation != nil {
		return false
	}
	if block.RuntimeMeasurement.EventSelection != nil && (len(block.EdgeAnchors) > 0 || len(block.ParticipantBoundaries) > 0 || len(block.RelationClaims) > 0 || len(block.ClaimUses) > 0 || !runtimeEventCaliberCompatible(block.Kind, string(block.TraceCausalClaimCaliber)) || block.RequestedRelationScope != "") {
		return false
	}
	var contract *types.RuntimeMeasurementContract
	if view != nil {
		contract = view.RuntimeMeasurementContract
	}
	copy := *block.RuntimeMeasurement
	if !types.BindRuntimeMeasurementReceipt(&copy, contract) {
		return false
	}
	if block.Kind != types.BlockTable && copy.EventSelection == nil && !copy.BoundTable.HasNativeFixedFacts() {
		return false
	}
	if copy.BoundTable.HasNativeFixedFacts() && (len(block.EdgeAnchors) > 0 || len(block.ParticipantBoundaries) > 0 || len(block.RelationClaims) > 0 || len(block.ClaimUses) > 0 || !runtimeEventCaliberCompatible(block.Kind, string(block.TraceCausalClaimCaliber)) || block.RequestedRelationScope != "") {
		return false
	}
	if len(block.RequestedDimensionIndices) > 0 {
		_, valid := types.BoundNativeDisplayDimensions(block, contract)
		return valid
	}
	return true
}

func preCheckRuntimeMeasurementBindings(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	var hints []emitFixHint
	for i, block := range doc.Blocks {
		if runtimeMeasurementBindingValid(block, view) {
			continue
		}
		hints = append(hints, emitFixHint{
			Field:         fmt.Sprintf("blocks[%d].runtime_measurement", i),
			ExpectedShape: "Select one observation_id/view object published by the current schema on kind=table; published native fixed facts and event_selection also support available summary/diagram kinds. Fixed facts require no event_selection. Omit competing text/items/columns/diagram/runtime_work_relation. Optional requested_dimension_indices must select current published member-set indices on a complete native display table. Do not copy rows to repair display ownership. If no choices are published, omit the optional selector. For a patch, use a complete replace_blocks entry; preserve unrelated blocks.",
			Reason:        "This measurement selector is unavailable, stale, or has competing table content; no prose or causal conclusion is checked.",
			ForceHard:     true, HardSignal: preEmitHardSignalExactReceiptBinding,
		})
	}
	return hints
}

func bindRuntimeMeasurementReceipts(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) error {
	if doc == nil {
		return nil
	}
	var failures []string
	for i, block := range doc.Blocks {
		if !runtimeMeasurementBindingValid(block, view) {
			failures = append(failures, fmt.Sprintf("blocks[%d].runtime_measurement does not match an exact current measurement table/view or has competing table content", i))
		}
	}
	if err := mergedDocumentViolationsError(failures); err != nil {
		return err
	}
	for i := range doc.Blocks {
		if doc.Blocks[i].RuntimeMeasurement != nil {
			types.BindRuntimeMeasurementReceipt(doc.Blocks[i].RuntimeMeasurement, view.RuntimeMeasurementContract)
		}
	}
	return nil
}
