package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

func toolDocumentationRequestSchema() map[string]any {
	return map[string]any{
		"type": "object", "description": types.ToolDocumentationRequestTeaching,
		"properties": map[string]any{
			"scope": map[string]any{"type": "string", "enum": []string{"only", "mixed"}},
			"dimension_indices": map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": 1}, "uniqueItems": true,
				"description": "For mixed only: explicit unique indices of retained required documentation-only requested_answer_dimensions. Omit for only. Do not rely on inferred default indices."},
		},
		"required": []string{"scope"}, "additionalProperties": false,
	}
}

func validateEmitToolDocumentationRequest(rm *types.RequestModel, raw *emitRequestedAnswerDimensionsParam) error {
	if err := types.ValidateToolDocumentationRequest(rm); err != nil {
		return err
	}
	if rm.ToolDocumentationRequest == nil || rm.ToolDocumentationRequest.Scope != types.ToolDocumentationRequestMixed {
		return nil
	}
	// Presentation normalization may assign missing indices or discard rows.
	// A cross-field reference must name an explicit unique original index;
	// it must never silently retarget a surviving/default-indexed row.
	for _, index := range rm.ToolDocumentationRequest.DimensionIndices {
		count := 0
		if raw != nil {
			for _, dim := range raw.Dimensions {
				if dim.Index == index {
					count++
				}
			}
		}
		if count != 1 {
			return fmt.Errorf("tool_documentation_request index %d must name one explicitly indexed original requested_answer_dimensions row, not an inferred or duplicate index", index)
		}
	}
	return nil
}
