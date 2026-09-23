package tool

import (
	"errors"
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
	return errors.Join(collectEmitToolDocumentationRequestViolations(rm, raw)...)
}

func collectEmitToolDocumentationRequestViolations(rm *types.RequestModel, raw *emitRequestedAnswerDimensionsParam) []error {
	violations := types.CollectToolDocumentationRequestViolations(rm)
	if rm == nil || rm.ToolDocumentationRequest == nil || rm.ToolDocumentationRequest.Scope != types.ToolDocumentationRequestMixed {
		return violations
	}
	// Presentation normalization may assign missing indices or discard rows.
	// A cross-field reference must name an explicit unique original index;
	// it must never silently retarget a surviving/default-indexed row.
	seen := map[int]bool{}
	for _, index := range rm.ToolDocumentationRequest.DimensionIndices {
		if seen[index] {
			continue
		}
		seen[index] = true
		// Check provenance for each independently valid selection, even when
		// another selection is invalid. Do not cascade a missing/ambiguous
		// retained index into a second error about its original row.
		selection := *rm
		selection.ToolDocumentationRequest = &types.ToolDocumentationRequest{
			Scope: types.ToolDocumentationRequestMixed, DimensionIndices: []int{index},
		}
		if types.ValidateToolDocumentationRequest(&selection) != nil {
			continue
		}
		count := 0
		if raw != nil {
			for _, dim := range raw.Dimensions {
				if dim.Index == index {
					count++
				}
			}
		}
		if count != 1 {
			violations = append(violations, fmt.Errorf("tool_documentation_request index %d must name one explicitly indexed original requested_answer_dimensions row, not an inferred or duplicate index", index))
		}
	}
	return violations
}
