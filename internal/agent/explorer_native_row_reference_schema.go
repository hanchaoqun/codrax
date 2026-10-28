package agent

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Reuse the completion roster, rather than inventing a second source catalog.
// Each whole branch identifies one exact pair and its original row range. This
// schema-only bound does not reserve or expand any prompt-publication budget.
func projectExplorerNativeRowReferenceSchema(ctx *types.AgentContext, fields map[string]json.RawMessage, choices []types.AnswerRuntimeMeasurementReceipt) {
	var units, item, properties, refs, refItem map[string]json.RawMessage
	if json.Unmarshal(fields["interpretation_units"], &units) != nil || json.Unmarshal(units["items"], &item) != nil || json.Unmarshal(item["properties"], &properties) != nil || json.Unmarshal(properties["native_refs"], &refs) != nil || json.Unmarshal(refs["items"], &refItem) != nil {
		return
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	var branches []json.RawMessage
	var encoded json.RawMessage
	if view != nil {
		for _, receipt := range choices {
			if !types.BindRuntimeMeasurementReceipt(&receipt, view.RuntimeMeasurementContract) || !receipt.IsBound() || len(receipt.BoundTable.Rows) == 0 {
				continue
			}
			if _, current := receipt.BoundTable.NativeSourceTitleBasis(); !current {
				continue
			}
			branch, _ := json.Marshal(map[string]any{
				"properties": map[string]any{
					"observation_id": map[string]any{"enum": []string{receipt.ObservationID}},
					"view":           map[string]any{"enum": []types.RuntimeMeasurementView{receipt.View}},
					"row_indices":    map[string]any{"items": map[string]any{"maximum": len(receipt.BoundTable.Rows) - 1}},
				},
			})
			// Share the base shape; reserve the complete serialized item schema,
			// including its type/required/range declarations and separators.
			refItem["oneOf"], _ = json.Marshal(append(branches, branch))
			candidate, _ := json.Marshal(refItem)
			if len(candidate) > nativePresentationNavigationByteLimit {
				continue
			}
			branches = append(branches, branch)
			encoded = candidate
		}
	}
	if len(branches) == 0 {
		delete(properties, "native_refs")
	} else {
		refs["items"] = encoded
		properties["native_refs"], _ = json.Marshal(refs)
	}
	item["properties"], _ = json.Marshal(properties)
	units["items"], _ = json.Marshal(item)
	fields["interpretation_units"], _ = json.Marshal(units)
}
