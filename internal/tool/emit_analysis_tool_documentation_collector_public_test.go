package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitAnalysisToolDocumentationCollectsIndependentViolationsPublic(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
		want   []string
	}{
		{
			name: "independent_dimension_obligations",
			change: func(p map[string]any) {
				for _, row := range p["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any) {
					row.(map[string]any)["role"] = "source_location"
				}
			},
			want: []string{"index 1 carries an independent", "index 2 carries an independent"},
		},
		{
			name: "independent_missing_indices",
			change: func(p map[string]any) {
				p["tool_documentation_request"] = map[string]any{"scope": "mixed", "dimension_indices": []int{7, 9, 9}}
			},
			want: []string{"index 7 must refer", "index 9 must refer", "unique positive indices"},
		},
		{
			name: "independent_original_indices",
			change: func(p map[string]any) {
				for _, row := range p["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any) {
					delete(row.(map[string]any), "index")
				}
			},
			want: []string{"index 1 must name one explicitly", "index 2 must name one explicitly"},
		},
		{
			name: "typed_and_original_errors_coexist",
			change: func(p map[string]any) {
				rows := p["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)
				rows[0].(map[string]any)["role"] = "source_location"
				delete(rows[1].(map[string]any), "index")
			},
			want: []string{"index 1 carries an independent", "index 2 must name one explicitly"},
		},
		{
			name: "only_shape_and_obligation_coexist",
			change: func(p map[string]any) {
				p["tool_documentation_request"] = map[string]any{"scope": "only", "dimension_indices": []int{1}}
				p["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)[0].(map[string]any)["role"] = "source_location"
			},
			want: []string{"scope=only must omit", "scope=only conflicts with"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := documentationAnalysisPayload(t)
			payload["tool_documentation_request"] = map[string]any{"scope": "mixed", "dimension_indices": []int{1, 2}}
			tc.change(payload)
			result, model := executeDocumentationAnalysis(t, payload)
			if result.Success || model != nil {
				t.Fatalf("invalid declaration persisted: %s", result.Summary)
			}
			for _, want := range tc.want {
				if count := strings.Count(result.Summary, want); count != 1 {
					t.Errorf("wanted exactly one %q, found %d in %s", want, count, result.Summary)
				}
			}
			// Removing the invalid optional domain preserves the original source
			// obligations and the established dimension-normalization contract.
			delete(payload, "tool_documentation_request")
			accepted, model := executeDocumentationAnalysis(t, payload)
			if !accepted.Success || model == nil || len(model.RequestedAnswerDimensions.Dimensions) != 2 {
				t.Fatalf("valid complete repair lost dimensions: %s", accepted.Summary)
			}
			for i, row := range payload["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any) {
				if string(model.RequestedAnswerDimensions.Dimensions[i].Role) != row.(map[string]any)["role"] || !model.RequestedAnswerDimensions.Dimensions[i].Required {
					t.Fatalf("removing invalid documentation domain changed required role %d", i+1)
				}
			}
		})
	}
}

func TestEmitAnalysisToolDocumentationCollectorSharedContract(t *testing.T) {
	rm := &types.RequestModel{
		ToolDocumentationRequest: &types.ToolDocumentationRequest{Scope: types.ToolDocumentationRequestMixed, DimensionIndices: []int{0, 1, 1, 2, 7, 9, 9}},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{
			{Index: 1, Role: types.RequestedAnswerDimensionSourceLocation, Required: true},
			{Index: 2, Role: types.RequestedAnswerDimensionSourceLocation, Required: true},
		}},
	}
	before, _ := json.Marshal(rm)
	violations := types.CollectToolDocumentationRequestViolations(rm)
	joined := types.ValidateToolDocumentationRequest(rm)
	if len(violations) != 5 || joined == nil || len(strings.Split(joined.Error(), "\n")) != len(violations) {
		t.Fatalf("independent violations were dropped or duplicate selections cascaded: %v / %v", violations, joined)
	}
	for _, violation := range violations {
		if strings.Count(joined.Error(), violation.Error()) != 1 {
			t.Fatalf("error API did not retain each collector result once: %v", joined)
		}
	}
	after, _ := json.Marshal(rm)
	if string(before) != string(after) || types.ToolDocumentationDimensionRequested(rm, 1) {
		t.Fatal("invalid declaration mutated the request or waived a source dimension")
	}
	// Correcting shape/missing references alone must not waive a source role.
	rm.ToolDocumentationRequest.DimensionIndices = []int{2}
	if types.ValidateToolDocumentationRequest(rm) == nil || types.ToolDocumentationDimensionRequested(rm, 2) {
		t.Fatal("partial repair accepted the remaining source obligation")
	}
	rm.RequestedAnswerDimensions.Dimensions[1].Role = types.RequestedAnswerDimensionFunctionOrPurpose
	if types.ValidateToolDocumentationRequest(rm) != nil || !types.ToolDocumentationDimensionRequested(rm, 2) || types.ToolDocumentationDimensionRequested(rm, 1) || types.ToolDocumentationOnlyRequested(rm) {
		t.Fatal("valid mixed repair altered the unselected source dimension")
	}
	for _, legacy := range []*types.RequestModel{nil, {}} {
		if types.ValidateToolDocumentationRequest(legacy) != nil || len(types.CollectToolDocumentationRequestViolations(legacy)) != 0 || types.ToolDocumentationOnlyRequested(legacy) || types.ToolDocumentationDimensionRequested(legacy, 1) {
			t.Fatal("nil/omitted domain no longer preserves the legacy contract")
		}
	}
}
