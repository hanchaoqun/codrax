package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const runtimeMeasurementTeaching = "Optional trusted measurement table: select one published observation_id/view pair on kind=table; do not copy values or supply text/items/columns. The system renders exact measured values, members or timeline segments with source/window and coverage notes. Put interpretation and runtime_work_relation in separate blocks. This selection proves measurements only, never a root cause."

func projectRuntimeMeasurementField(blockItems, blockProps map[string]any, view *types.AnswerSemanticView) {
	choices := view.RuntimeMeasurementContract.Choices()
	allowed := allowedKindSet(view)
	if len(choices) == 0 || (len(allowed) > 0 && !allowed[string(types.BlockTable)]) {
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
	conditions := schemaAllOfEntries(blockItems)
	conditions = append(conditions, map[string]any{
		"if": map[string]any{"required": []string{"runtime_measurement"}},
		"then": map[string]any{"properties": map[string]any{
			"kind":                  map[string]any{"const": string(types.BlockTable)},
			"text":                  map[string]any{"maxLength": 0},
			"columns":               map[string]any{"maxItems": 0},
			"items":                 map[string]any{"maxItems": 0},
			"diagram":               map[string]any{"enum": []any{nil}},
			"runtime_work_relation": map[string]any{"enum": []any{nil}},
		}},
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
}

func validateEmitRuntimeMeasurementBlock(raw emitAnswerBlockV2, path string) error {
	receipt := raw.RuntimeMeasurement
	if receipt == nil {
		return nil
	}
	if strings.TrimSpace(receipt.ObservationID) == "" || !receipt.View.IsValid() {
		return fmt.Errorf("%s.runtime_measurement requires one published observation_id and view (summary, members, or timeline)", path)
	}
	if raw.Kind != string(types.BlockTable) || raw.Text != "" || len(raw.Items) > 0 || len(raw.Columns) > 0 || raw.Diagram != nil || raw.RuntimeWorkRelation != nil {
		return fmt.Errorf("%s.runtime_measurement is only valid on kind=table without text/items/columns/diagram/runtime_work_relation; put explanations and relation judgments in separate blocks", path)
	}
	return nil
}

func runtimeMeasurementBindingValid(block types.AnswerBlock, view *types.AnswerSemanticView) bool {
	if block.RuntimeMeasurement == nil {
		return true
	}
	if block.Kind != types.BlockTable || block.Text != "" || len(block.Items) > 0 || len(block.Columns) > 0 || block.Diagram != nil || block.RuntimeWorkRelation != nil {
		return false
	}
	var contract *types.RuntimeMeasurementContract
	if view != nil {
		contract = view.RuntimeMeasurementContract
	}
	copy := *block.RuntimeMeasurement
	return types.BindRuntimeMeasurementReceipt(&copy, contract)
}

func preCheckRuntimeMeasurementBindings(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	var hints []emitFixHint
	for i, block := range doc.Blocks {
		if runtimeMeasurementBindingValid(block, view) {
			continue
		}
		hints = append(hints, emitFixHint{
			Field:         fmt.Sprintf("blocks[%d].runtime_measurement", i),
			ExpectedShape: "Select one observation_id/view object published by the current schema on kind=table without text/items/columns/diagram/runtime_work_relation. If no choices are published, omit the optional selector. For a patch, use a complete replace_blocks entry; preserve unrelated blocks.",
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
