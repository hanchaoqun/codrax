package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func documentationRequestModel() RequestModel {
	return RequestModel{ToolDocumentationRequest: &ToolDocumentationRequest{Scope: ToolDocumentationRequestOnly},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{
			{Index: 1, Label: "units", Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true},
			{Index: 2, Label: "dispatch", Role: RequestedAnswerDimensionBranchBehavior, Required: true},
		}}}
}

func TestToolDocumentationRequestDomain(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*RequestModel)
	}{
		{"nil", func(r *RequestModel) { r.ToolDocumentationRequest = nil }},
		{"unknown", func(r *RequestModel) { r.ToolDocumentationRequest.Scope = "source_free" }},
		{"only_indices", func(r *RequestModel) { r.ToolDocumentationRequest.DimensionIndices = []int{1} }},
		{"current_code", func(r *RequestModel) {
			r.RequestedAnswerDimensions.Dimensions[1].Role = RequestedAnswerDimensionCurrentKeyCode
		}},
		{"source_location", func(r *RequestModel) {
			r.RequestedAnswerDimensions.Dimensions[1].Role = RequestedAnswerDimensionSourceLocation
		}},
		{"current_profile", func(r *RequestModel) {
			r.CurrentSourceExplanationProfile = &CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true, SourceQuotes: []string{"current source"}}
		}},
		{"source_scope", func(r *RequestModel) {
			r.SourceScopeProfile = &SourceScopeProfile{RequestedScope: SourceScopeDocumentation, SourceQuotes: []string{"README"}}
		}},
		{"source_signal", func(r *RequestModel) {
			r.CurrentSourceObligationSignals = []CurrentSourceObligationSignal{{Kind: CurrentSourceObligationSignalRouteBackedHistoryExplanation}}
		}},
		{"source_file", func(r *RequestModel) {
			r.AnalyzerHints.RequiredFileHints = []RequiredFileHint{{Path: "Makefile", RequestedDimensionIndices: []int{1}}}
		}},
		{"source_target", func(r *RequestModel) { r.AnalyzerHints.ExactTargets = []string{"src/main.go:12"} }},
		{"pinned_file", func(r *RequestModel) { r.UserPinnedFiles = []string{"README"} }},
		{"runtime_window", func(r *RequestModel) {
			a, b := 0.0, 1.0
			r.RuntimeArtifactScopeProfile = &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: &a, TimeEnd: &b, SourceQuote: "0..1"}
		}},
		{"runtime_families", func(r *RequestModel) {
			r.RuntimeQuestionProfile = &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeBoundedFactSet, FactFamilies: []RuntimeQuestionFactFamily{RuntimeQuestionFactCountOrDuration}}
		}},
		{"causal_role", func(r *RequestModel) {
			r.RequestedAnswerDimensions.Dimensions[1].Role = RequestedAnswerDimensionCausalAttribution
		}},
		{"root_cause", func(r *RequestModel) { r.Intent = IntentRootCause }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := documentationRequestModel()
			tc.change(&r)
			if ToolDocumentationOnlyRequested(&r) {
				t.Fatal("independent obligation/invalid domain became pure documentation")
			}
			if ToolDocumentationDimensionRequested(&r, 1) {
				t.Fatal("invalid pure domain waived a dimension")
			}
		})
	}
	r := documentationRequestModel()
	r.RawRequest = "trace_capabilities runtime source code src/main.go all keywords are non-authoritative"
	r.AnalyzerHints.RequiredFileHints = []RequiredFileHint{{Path: "navigation-only.go", Confidence: 0.99}}
	if !ToolDocumentationOnlyRequested(&r) {
		t.Fatal("unstructured request text or confidence-only navigation changed the typed classification")
	}
	b, _ := json.Marshal(r)
	var copy RequestModel
	if err := json.Unmarshal(b, &copy); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(copy.ToolDocumentationRequest, r.ToolDocumentationRequest) {
		t.Fatal("request domain lost in IR JSON")
	}
	clone := CloneToolDocumentationRequest(&ToolDocumentationRequest{Scope: ToolDocumentationRequestMixed, DimensionIndices: []int{1}})
	source := clone
	clone = CloneToolDocumentationRequest(source)
	clone.DimensionIndices[0] = 2
	if source.DimensionIndices[0] != 1 {
		t.Fatal("profile clone aliases dimension indices")
	}
}

func TestToolDocumentationMixedDimensionApplicability(t *testing.T) {
	r := documentationRequestModel()
	r.ToolDocumentationRequest = &ToolDocumentationRequest{Scope: ToolDocumentationRequestMixed, DimensionIndices: []int{1}}
	r.AnalyzerHints.RequiredFileHints = []RequiredFileHint{{Path: "src/engine.go", Confidence: 0.9, RequestedDimensionIndices: []int{2}}}
	r.RuntimeQuestionProfile = &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeCausalDiagnosis, FrameCausalityRequested: true}
	before, _ := json.Marshal(r)
	if err := ValidateToolDocumentationRequest(&r); err != nil {
		t.Fatal(err)
	}
	needs := RequestedExplanationOperationNeedsForAuthority(&r, RuntimeSourceAnswerAuthoritySnapshot{})
	if len(needs) != 1 || needs[0].Dimension.Index != 2 || needs[0].Source != "src/engine.go" {
		t.Fatalf("mixed source seat changed: %+v", needs)
	}
	if marker := CompileDimensionOwnerUnresolvedForRequest(&r); marker != nil {
		t.Fatalf("documentation dimension created false source-owner guidance: %+v", marker)
	}
	after, _ := json.Marshal(r)
	if string(before) != string(after) {
		t.Fatal("domain projection mutated runtime/source/dimension data")
	}
	for _, tc := range []struct {
		name   string
		change func(*RequestModel)
	}{
		{"missing_indices", func(r *RequestModel) { r.ToolDocumentationRequest.DimensionIndices = nil }},
		{"unknown_index", func(r *RequestModel) { r.ToolDocumentationRequest.DimensionIndices = []int{3} }},
		{"duplicate_refs", func(r *RequestModel) { r.ToolDocumentationRequest.DimensionIndices = []int{1, 1} }},
		{"zero_ref", func(r *RequestModel) { r.ToolDocumentationRequest.DimensionIndices = []int{0} }},
		{"ambiguous_index", func(r *RequestModel) { r.RequestedAnswerDimensions.Dimensions[1].Index = 1 }},
		{"optional_dimension", func(r *RequestModel) { r.RequestedAnswerDimensions.Dimensions[0].Required = false }},
		{"bound_source_dimension", func(r *RequestModel) { r.ToolDocumentationRequest.DimensionIndices = []int{2} }},
		{"source_role", func(r *RequestModel) {
			r.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionSourceLocation
		}},
		{"runtime_effect_role", func(r *RequestModel) {
			r.RequestedAnswerDimensions.Dimensions[0].Role = RequestedAnswerDimensionTargetEffectVerdict
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var changed RequestModel
			if err := json.Unmarshal(before, &changed); err != nil {
				t.Fatal(err)
			}
			tc.change(&changed)
			if ValidateToolDocumentationRequest(&changed) == nil || ToolDocumentationDimensionRequested(&changed, 1) {
				t.Fatal("invalid mixed reference weakened a source/runtime seat")
			}
		})
	}
}
