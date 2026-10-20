package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

func appendRuntimeMeasurementSourceTitleSchema(conditions []map[string]any, properties map[string]any, choices []types.RuntimeMeasurementTable) []map[string]any {
	owned := false
	for _, table := range choices {
		if _, _, ok := table.NativeSourcePresentation(""); !ok {
			continue
		}
		// Match id/view fields, not an entire object enum: a native event
		// selection can legitimately carry its additional exact subset.
		selector := map[string]any{
			"required": []string{"observation_id", "view"},
			"properties": map[string]any{
				"observation_id": map[string]any{"const": table.ObservationID},
				"view":           map[string]any{"const": string(table.View)},
			},
		}
		// The executable validator supports allOf/if, not anyOf. One exact
		// pair per condition also keeps event_selection's extra fields legal.
		conditions = append(conditions, map[string]any{
			"if": map[string]any{"required": []string{"runtime_measurement"},
				"properties": map[string]any{"runtime_measurement": selector}},
			"then": map[string]any{"properties": map[string]any{"title": map[string]any{"maxLength": 0}}},
		})
		owned = true
	}
	if !owned {
		return conditions
	}
	if title, ok := properties["title"].(map[string]any); ok {
		title["description"] = fmt.Sprint(title["description"]) + " " + types.RuntimeMeasurementSourceTitleTeaching
	}
	return conditions
}

func runtimeMeasurementSourceTitleHint(block types.AnswerBlock, view *types.AnswerSemanticView, index int) (emitFixHint, bool) {
	if block.RuntimeMeasurement == nil || block.Title == "" || view == nil {
		return emitFixHint{}, false
	}
	receipt := *block.RuntimeMeasurement
	if !types.BindRuntimeMeasurementReceipt(&receipt, view.RuntimeMeasurementContract) {
		return emitFixHint{}, false
	}
	if _, _, owned := receipt.BoundTable.NativeSourcePresentation(""); !owned {
		return emitFixHint{}, false
	}
	return emitFixHint{
		Field: fmt.Sprintf("blocks[%d].title", index),
		ExpectedShape: fmt.Sprintf("Omit title; keep runtime_measurement.observation_id=%q and view=%q (and its event_selection, if present). The current source binding supplies the heading. For a patch, replace only this complete block without title; preserve unrelated blocks.",
			receipt.ObservationID, receipt.View),
		Reason:    "A model title competes with this native selector's current source-owned heading; title text is not interpreted.",
		ForceHard: true, HardSignal: preEmitHardSignalExactReceiptBinding,
	}, true
}
