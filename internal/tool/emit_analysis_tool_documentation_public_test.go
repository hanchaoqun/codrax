package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func documentationAnalysisPayload(t *testing.T) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(withV4Required(`{"intent":"explain","scenario":"architecture_explain","complexity":"simple","keywords":["capabilities"],"entities":[],"question_kind":"define"}`)), &payload); err != nil {
		t.Fatal(err)
	}
	payload["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": true, "confidence": 0.9, "dimensions": []any{
		map[string]any{"index": 1, "label": "documented units", "source_quote": "documented units", "role": "function_or_purpose", "required": true},
		map[string]any{"index": 2, "label": "implementation behavior", "source_quote": "implementation behavior", "role": "branch_behavior", "required": true},
	}}
	return payload
}

func TestEmitAnalysisToolDocumentationPublicInvalidDimensions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"mixed_without_indices", func(p map[string]any) { p["tool_documentation_request"] = map[string]any{"scope": "mixed"} }},
		{"only_with_indices", func(p map[string]any) {
			p["tool_documentation_request"] = map[string]any{"scope": "only", "dimension_indices": []int{1}}
		}},
		{"missing_index", func(p map[string]any) {
			p["tool_documentation_request"] = map[string]any{"scope": "mixed", "dimension_indices": []int{3}}
		}},
		{"duplicate_index", func(p map[string]any) {
			p["tool_documentation_request"] = map[string]any{"scope": "mixed", "dimension_indices": []int{1, 1}}
		}},
		{"inferred_index", func(p map[string]any) {
			delete(p["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)[0].(map[string]any), "index")
		}},
		{"ambiguous_original_index", func(p map[string]any) {
			p["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)[1].(map[string]any)["index"] = 1
		}},
		{"dropped_dimension", func(p map[string]any) {
			d := p["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)[0].(map[string]any)
			d["source_quote"] = "not in current request"
			d["label"] = "not anchored either"
		}},
		{"optional_dimension", func(p map[string]any) {
			p["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)[0].(map[string]any)["required"] = false
		}},
		{"source_location", func(p map[string]any) {
			p["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)[0].(map[string]any)["role"] = "source_location"
		}},
		{"only_current_key_code", func(p map[string]any) {
			p["tool_documentation_request"] = map[string]any{"scope": "only"}
			p["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)[1].(map[string]any)["role"] = "current_key_code"
		}},
		{"source_file_binding", func(p map[string]any) {
			p["required_files"] = []any{map[string]any{"path": "src/core.go", "confidence": 0.95, "requested_dimension_indices": []int{1}, "rationale": "actual implementation"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := documentationAnalysisPayload(t)
			p["tool_documentation_request"] = map[string]any{"scope": "mixed", "dimension_indices": []int{1}}
			tc.change(p)
			result, model := executeDocumentationAnalysis(t, p)
			if result.Success || model != nil || !strings.Contains(result.Summary, "tool_documentation_request") {
				t.Fatalf("invalid domain was accepted or lost diagnostic: %s", result.Summary)
			}
		})
	}
}

func TestEmitAnalysisToolDocumentationPublicRuntimeWindow(t *testing.T) {
	for _, scope := range []string{"only", "mixed"} {
		t.Run(scope, func(t *testing.T) {
			raw, payload := mixedRuntimePayload(t, "causal_attribution", true, true)
			raw += "; documented units"
			dimensions := payload["requested_answer_dimensions"].(map[string]any)
			dimensions["dimensions"] = append(dimensions["dimensions"].([]any), map[string]any{"index": 4, "label": "documented units", "source_quote": "documented units", "role": "function_or_purpose", "required": true})
			profile := map[string]any{"scope": scope}
			if scope == "mixed" {
				profile["dimension_indices"] = []int{4}
			}
			payload["tool_documentation_request"] = profile
			ok, summary, model := executeMixedRuntime(t, raw, payload)
			if scope == "only" {
				if ok || model != nil || !strings.Contains(summary, "tool_documentation_request") {
					t.Fatalf("pure docs swallowed explicit causal window: %s", summary)
				}
				return
			}
			if !ok || model == nil {
				t.Fatalf("mixed docs and real window rejected: %s", summary)
			}
			var wantScope types.RuntimeArtifactScopeProfile
			data, _ := json.Marshal(payload["runtime_artifact_scope_profile"])
			_ = json.Unmarshal(data, &wantScope)
			if !reflect.DeepEqual(model.RuntimeArtifactScopeProfile, &wantScope) || !model.RuntimeQuestionProfile.FrameCausalityRequested || !model.RuntimeQuestionProfile.RuntimeWorkRelationRequested || model.RuntimeQuestionProfile.Scope != types.RuntimeQuestionScopeCausalDiagnosis {
				t.Fatalf("mixed docs altered window or causal breadth: %+v", model)
			}
			if len(model.RequestedAnswerDimensions.Dimensions) != 4 {
				t.Fatal("mixed docs dropped user dimensions")
			}
		})
	}
}

func TestEmitAnalysisToolDocumentationPublicLegacyAndSchema(t *testing.T) {
	p := documentationAnalysisPayload(t)
	result, model := executeDocumentationAnalysis(t, p)
	if !result.Success || model == nil || model.ToolDocumentationRequest != nil {
		t.Fatalf("omission changed legacy request: %s", result.Summary)
	}
	schema := string((&EmitAnalysis{}).Parameters())
	if !strings.Contains(schema, types.ToolDocumentationRequestTeaching) {
		t.Fatal("actual tool schema lost single-source domain teaching")
	}
}

func executeDocumentationAnalysis(t *testing.T, payload map[string]any) (types.ToolResult, *types.RequestModel) {
	t.Helper()
	ctx := &types.BusContext{Mutable: types.NewMutableState("Explain documented units and implementation behavior")}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&EmitAnalysis{}).Execute(ctx, encoded)
	if err != nil {
		t.Fatal(err)
	}
	return result, ctx.Mutable.RequestModel()
}

func TestEmitAnalysisToolDocumentationPublic(t *testing.T) {
	for _, scope := range []string{"only", "mixed"} {
		t.Run(scope, func(t *testing.T) {
			payload := documentationAnalysisPayload(t)
			profile := map[string]any{"scope": scope}
			if scope == "mixed" {
				profile["dimension_indices"] = []int{1}
			}
			payload["tool_documentation_request"] = profile
			result, model := executeDocumentationAnalysis(t, payload)
			if !result.Success || model == nil {
				t.Fatalf("valid documentation domain rejected: %s", result.Summary)
			}
			encoded, _ := json.Marshal(model)
			var actual map[string]json.RawMessage
			_ = json.Unmarshal(encoded, &actual)
			if len(actual["tool_documentation_request"]) == 0 {
				t.Fatalf("accepted request lost documentation domain: %s", encoded)
			}
		})
	}
	t.Run("unknown_scope", func(t *testing.T) {
		payload := documentationAnalysisPayload(t)
		payload["tool_documentation_request"] = map[string]any{"scope": "arbitrary"}
		result, model := executeDocumentationAnalysis(t, payload)
		if result.Success || model != nil || !strings.Contains(result.Summary, "tool_documentation_request") {
			t.Fatalf("unknown domain accepted: %s", result.Summary)
		}
	})
}
