package tool

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func boundedScopeAuthorityPublicParams(t *testing.T, scope map[string]any, request string) json.RawMessage {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(withV4Required(`{"intent":"explain","scenario":"generic","complexity":"moderate","keywords":["trace","operation","wait"],"entities":["trace"],"question_kind":"mechanism"}`)), &payload); err != nil {
		t.Fatal(err)
	}
	payload["runtime_artifact_scope_profile"] = scope
	payload["runtime_target_profile"] = map[string]any{"declaration": "no_named_target", "confidence": 1.0}
	payload["runtime_question_profile"] = map[string]any{
		"scope": "causal_diagnosis", "source_quote": request, "confidence": 1.0,
		"runtime_work_relation_requested": false, "frame_causality_requested": false,
	}
	payload["requested_answer_dimensions"] = map[string]any{
		"is_dimensioned_answer": true, "confidence": 1.0,
		"dimensions": []any{map[string]any{"index": 1, "label": "cause", "role": "causal_attribution", "source_quote": request, "required": true}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestEmitAnalysisPublicBoundedScopeDoesNotMintRequestedTimeWindow(t *testing.T) {
	const request = "Explain the complete response interval of RefreshPanel in this trace."
	const quote = "complete response interval of RefreshPanel"
	for _, tc := range []struct {
		name        string
		coordinates map[string]any
	}{
		{"without_coordinates", nil},
		{"scalar", map[string]any{"time_start": 0.999, "time_end": 1.051}},
		{"invalid_scalar", map[string]any{"time_start": 2.0, "time_end": 1.0}},
		{"single_list", map[string]any{"time_windows": []any{map[string]any{"time_start": 0.999, "time_end": 1.051, "source_quote": quote}}}},
		{"multiple_list", map[string]any{"time_windows": []any{
			map[string]any{"time_start": 0.999, "time_end": 1.051, "source_quote": quote},
			map[string]any{"time_start": 2.0, "time_end": 2.05, "source_quote": quote},
		}}},
		{"empty_list", map[string]any{"time_windows": []any{}}},
		{"incomplete_list_member", map[string]any{"time_windows": []any{map[string]any{"time_end": 1.051}}}},
		{"mixed_coordinates", map[string]any{"time_start": 0.999, "time_end": 1.051, "time_windows": []any{map[string]any{"time_start": 2.0, "time_end": 2.05, "source_quote": quote}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope := map[string]any{"requested_scope": "bounded_selector", "source_quote": quote, "confidence": 0.95}
			for key, value := range tc.coordinates {
				scope[key] = value
			}
			raw := boundedScopeAuthorityPublicParams(t, scope, request)
			before := append([]byte(nil), raw...)
			ctx := &types.BusContext{Mutable: types.NewMutableState(request), AttachedHitrace: "inline trace"}
			result, err := (&EmitAnalysis{}).Execute(ctx, raw)
			if err != nil || !result.Success {
				t.Fatalf("bounded intent should normalize without a retry: result=%+v err=%v", result, err)
			}
			rm := ctx.Mutable.RequestModel()
			if rm == nil || rm.RuntimeArtifactScopeProfile == nil {
				t.Fatal("public emitter did not persist the scope")
			}
			profile := rm.RuntimeArtifactScopeProfile
			if profile.RequestedScope != types.RuntimeArtifactScopeBoundedSelector || profile.SourceQuote != quote || !profile.Active() {
				t.Errorf("bounded intent/quote changed: %+v", profile)
			}
			if profile.TimeStart != nil || profile.TimeEnd != nil || profile.TimeWindows != nil || profile.HasExplicitTimeWindows() {
				t.Errorf("selector coordinates became requested-window authority: %+v", profile)
			}
			windowScope := types.ResolveTraceQueryWindowScope(profile, 0.999, 1.051)
			if windowScope.RequestedWindowKnown || windowScope.Role != types.TraceQueryWindowScopeElectedQueryWindow ||
				windowScope.RequestedWindowCount != 0 || windowScope.RequestedWindowOrdinal != 0 {
				t.Errorf("public scope laundered exploratory coordinates into a user window: %+v", windowScope)
			}
			if got := windowScope.Format("zh"); !strings.Contains(got, "未绑定明确的用户时间窗") || strings.Contains(got, "用户指定") {
				t.Errorf("reader scope claims user-authored coordinates: %s", got)
			}
			if decided, allowed := types.RuntimeTraceReportShapeAuthority(rm); !decided || !allowed {
				t.Errorf("causal report authority must survive independently of explicit time: %+v", rm.RuntimeQuestionProfile)
			}
			if dimensions := rm.RequestedAnswerDimensions; dimensions == nil || !dimensions.Active() || len(dimensions.Dimensions) != 1 ||
				dimensions.Dimensions[0].Label != "cause" || dimensions.Dimensions[0].Role != types.RequestedAnswerDimensionCausalAttribution ||
				!dimensions.Dimensions[0].Required || dimensions.Dimensions[0].SourceQuote != request {
				t.Errorf("coordinate normalization changed the required answer dimensions: %+v", dimensions)
			}
			if !bytes.Equal(raw, before) {
				t.Fatal("normalization mutated the submitted JSON")
			}
			if len(tc.coordinates) > 0 && !strings.Contains(result.Summary, "selector coordinates are not explicit user-window authority") {
				t.Errorf("coordinate normalization has no audit warning: %s", result.Summary)
			}
		})
	}
}

func TestEmitAnalysisPublicBoundedScopeUnanchoredQuoteDoesNotBorrowMemberQuotes(t *testing.T) {
	const request = "Explain the complete response interval of RefreshPanel in this trace."
	for _, quote := range []string{"", "a selector from a tool result"} {
		scope := map[string]any{
			"requested_scope": "bounded_selector", "source_quote": quote, "confidence": 0.9,
			"time_windows": []any{map[string]any{"time_start": 0.999, "time_end": 1.051, "source_quote": "complete response interval of RefreshPanel"}},
		}
		ctx := &types.BusContext{Mutable: types.NewMutableState(request), AttachedHitrace: "inline trace"}
		result, err := (&EmitAnalysis{}).Execute(ctx, boundedScopeAuthorityPublicParams(t, scope, request))
		if err != nil || !result.Success {
			t.Fatalf("unanchored selector should soften: result=%+v err=%v", result, err)
		}
		profile := ctx.Mutable.RequestModel().RuntimeArtifactScopeProfile
		if profile.RequestedScope != types.RuntimeArtifactScopeUnspecified || profile.Active() || profile.SourceQuote != "" ||
			profile.TimeStart != nil || profile.TimeEnd != nil || profile.TimeWindows != nil {
			t.Fatalf("member quote supplied missing selector/request authority: %+v", profile)
		}
	}
}

func TestEmitAnalysisPublicBoundedScopePreservesOtherScopeLanes(t *testing.T) {
	const request = "Explain this trace in 0.999..1.051 seconds."
	for _, tc := range []struct {
		name     string
		scope    map[string]any
		want     types.RuntimeArtifactRequestedScope
		explicit bool
	}{
		{"explicit_scalar", map[string]any{"requested_scope": "explicit_time_window", "time_start": 0.999, "time_end": 1.051, "source_quote": "0.999..1.051", "confidence": 1.0}, types.RuntimeArtifactScopeExplicitWindow, true},
		{"explicit_single_list", map[string]any{"requested_scope": "explicit_time_window", "time_windows": []any{map[string]any{"time_start": 0.999, "time_end": 1.051, "source_quote": "0.999..1.051"}}, "confidence": 1.0}, types.RuntimeArtifactScopeExplicitWindow, true},
		{"full_artifact", map[string]any{"requested_scope": "full_artifact", "source_quote": "this trace", "time_start": 0.999, "time_end": 1.051, "confidence": 1.0}, types.RuntimeArtifactScopeFullArtifact, false},
		{"unspecified", map[string]any{"requested_scope": "unspecified", "time_start": 0.999, "time_end": 1.051, "confidence": 1.0}, types.RuntimeArtifactScopeUnspecified, false},
		{"not_applicable_with_artifact", map[string]any{"requested_scope": "not_applicable", "confidence": 1.0}, types.RuntimeArtifactScopeUnspecified, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &types.BusContext{Mutable: types.NewMutableState(request), AttachedHitrace: "inline trace"}
			result, err := (&EmitAnalysis{}).Execute(ctx, boundedScopeAuthorityPublicParams(t, tc.scope, request))
			if err != nil || !result.Success {
				t.Fatalf("unchanged scope lane rejected: result=%+v err=%v", result, err)
			}
			profile := ctx.Mutable.RequestModel().RuntimeArtifactScopeProfile
			if profile.RequestedScope != tc.want || profile.HasExplicitTimeWindows() != tc.explicit {
				t.Fatalf("unrelated scope lane changed: %+v", profile)
			}
			got := types.ResolveTraceQueryWindowScope(profile, 0.999, 1.051)
			if got.RequestedWindowKnown != tc.explicit || (got.Role == types.TraceQueryWindowScopeRequestedPrincipal) != tc.explicit {
				t.Fatalf("request/query authority changed: %+v", got)
			}
			if !tc.explicit && (profile.TimeStart != nil || profile.TimeEnd != nil || profile.TimeWindows != nil) {
				t.Fatalf("non-explicit scope retained coordinates: %+v", profile)
			}
		})
	}
	t.Run("missing_profile_still_rejected", func(t *testing.T) {
		var payload map[string]any
		if err := json.Unmarshal(boundedScopeAuthorityPublicParams(t, nil, request), &payload); err != nil {
			t.Fatal(err)
		}
		delete(payload, "runtime_artifact_scope_profile")
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		ctx := &types.BusContext{Mutable: types.NewMutableState(request), AttachedHitrace: "inline trace"}
		result, err := (&EmitAnalysis{}).Execute(ctx, raw)
		if err != nil || result.Success || !strings.Contains(result.Summary, "runtime_artifact_scope_profile") || ctx.Mutable.RequestModel() != nil {
			t.Fatalf("missing profile bypassed the existing contract: result=%+v err=%v", result, err)
		}
	})
	t.Run("no_artifact_carrier_still_not_applicable", func(t *testing.T) {
		scope := map[string]any{"requested_scope": "bounded_selector", "source_quote": "this trace", "time_start": 0.999, "time_end": 1.051, "confidence": 1.0}
		ctx := &types.BusContext{Mutable: types.NewMutableState(request)}
		result, err := (&EmitAnalysis{}).Execute(ctx, boundedScopeAuthorityPublicParams(t, scope, request))
		if err != nil || !result.Success {
			t.Fatalf("missing carrier should retain existing normalization: result=%+v err=%v", result, err)
		}
		profile := ctx.Mutable.RequestModel().RuntimeArtifactScopeProfile
		if profile.RequestedScope != types.RuntimeArtifactScopeNotApplicable || profile.Active() || profile.TimeStart != nil || profile.TimeEnd != nil || profile.TimeWindows != nil {
			t.Fatalf("selector created runtime authority without an artifact carrier: %+v", profile)
		}
	})
}

func TestEmitAnalysisPublicBoundedScopeKeepsNativeBusinessFocusSupplement(t *testing.T) {
	const request = "Explain why OpenDocument was slow in this trace."
	for _, coordinates := range []map[string]any{
		{"time_start": 0.999, "time_end": 1.051},
		{"time_windows": []any{map[string]any{"time_start": 0.999, "time_end": 1.051, "source_quote": "OpenDocument"}}},
	} {
		scope := map[string]any{"requested_scope": "bounded_selector", "source_quote": "OpenDocument", "confidence": 1.0}
		for key, value := range coordinates {
			scope[key] = value
		}
		analyzerCtx := &types.BusContext{Mutable: types.NewMutableState(request), AttachedHitrace: "inline trace"}
		result, err := (&EmitAnalysis{}).Execute(analyzerCtx, boundedScopeAuthorityPublicParams(t, scope, request))
		if err != nil || !result.Success {
			t.Fatalf("public analysis rejected: result=%+v err=%v", result, err)
		}
		ctx, ref := supplementBusinessFocusFixture(t, "OpenDocument")
		ctx.AnalysisIR.RequestModel = *analyzerCtx.Mutable.RequestModel()
		ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
		supplementAcceptBusinessFocus(t, ctx, &ref)
		out := RunTraceQuerySystemSupplement(ctx)
		if len(out.Executed) == 0 {
			t.Fatalf("selector normalization suppressed native business focus: %+v", out)
		}
		supplementAssertBusinessWindow(t, ctx, ref)
		meta := ctx.Mutable.SystemTraceSupplementMeta()
		if meta.WindowStart != 1.0 || meta.WindowEnd != 1.05 {
			t.Fatalf("exploratory coordinates displaced the measured 50ms business interval: %+v", meta)
		}
		windowScope := types.ResolveTraceQueryWindowScope(ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile, meta.WindowStart, meta.WindowEnd)
		if windowScope.RequestedWindowKnown || windowScope.Role != types.TraceQueryWindowScopeElectedQueryWindow {
			t.Fatalf("resolved business interval was recast as user coordinates: %+v", windowScope)
		}
	}
}

func TestEmitAnalysisPublicBoundedScopeSchemaTeachesNoPromotion(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	profile := schema["properties"].(map[string]any)["runtime_artifact_scope_profile"].(map[string]any)
	if description := profile["description"].(string); !strings.Contains(description, "For bounded_selector, omit time_start, time_end and time_windows") ||
		!strings.Contains(description, "never promote the selector to an explicit time window") {
		t.Fatalf("public schema lost selector-coordinate boundary: %s", description)
	}
}

func TestEmitAnalysisPublicBoundedScopeKeepsQuestionBreadthIndependent(t *testing.T) {
	for _, tc := range []struct {
		question, request, role string
		families                []string
		fullReport              bool
	}{
		{"bounded_fact_set", "Report the states of RefreshPanel in this trace.", "observed_value", []string{"target_scheduler_state"}, false},
		{"bounded_effect_verdict", "Did the frequency limit affect RefreshPanel in this trace?", "target_effect_verdict", []string{"frequency_residency"}, false},
		{"relation_analysis", "Explain the dependency path of RefreshPanel in this trace.", "relation_path", nil, true},
		{"system_overview", "Give an overview of RefreshPanel in this trace.", "observed_value", nil, true},
	} {
		for _, form := range []string{"scalar", "list"} {
			t.Run(tc.question+"/"+form, func(t *testing.T) {
				scope := map[string]any{"requested_scope": "bounded_selector", "source_quote": "RefreshPanel", "confidence": 1.0}
				if form == "scalar" {
					scope["time_start"], scope["time_end"] = 0.999, 1.051
				} else {
					scope["time_windows"] = []any{map[string]any{"time_start": 0.999, "time_end": 1.051, "source_quote": "RefreshPanel"}}
				}
				var payload map[string]any
				if err := json.Unmarshal(boundedScopeAuthorityPublicParams(t, scope, tc.request), &payload); err != nil {
					t.Fatal(err)
				}
				question := payload["runtime_question_profile"].(map[string]any)
				question["scope"] = tc.question
				if len(tc.families) > 0 {
					question["fact_families"] = tc.families
				}
				wantDimensions := types.RequestedAnswerDimensionProfile{
					IsDimensionedAnswer: true, Confidence: 1.0,
					Dimensions: []types.RequestedAnswerDimension{
						{Index: 1, Label: "requested result", Role: types.RequestedAnswerDimensionRole(tc.role), SourceQuote: tc.request, Required: true},
						{Index: 2, Label: "supporting evidence", Role: types.RequestedAnswerDimensionEvidenceSource, SourceQuote: tc.request, Required: true},
					},
				}
				payload["requested_answer_dimensions"] = wantDimensions
				raw, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				ctx := &types.BusContext{Mutable: types.NewMutableState(tc.request), AttachedHitrace: "inline trace"}
				result, err := (&EmitAnalysis{}).Execute(ctx, raw)
				if err != nil || !result.Success {
					t.Fatalf("coherent question breadth rejected: result=%+v err=%v", result, err)
				}
				rm := ctx.Mutable.RequestModel()
				if rm.RuntimeArtifactScopeProfile.RequestedScope != types.RuntimeArtifactScopeBoundedSelector || rm.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() {
					t.Fatalf("question breadth changed selector authority: %+v", rm.RuntimeArtifactScopeProfile)
				}
				if rm.RuntimeQuestionProfile.Scope != types.RuntimeQuestionScope(tc.question) {
					t.Fatalf("coordinate cleanup rewrote question breadth: %+v", rm.RuntimeQuestionProfile)
				}
				if decided, allowed := types.RuntimeTraceReportShapeAuthority(rm); !decided || allowed != tc.fullReport {
					t.Errorf("report breadth must come from the question, not stray coordinates: decided=%t allowed=%t want=%t", decided, allowed, tc.fullReport)
				}
				if !reflect.DeepEqual(rm.RequestedAnswerDimensions, &wantDimensions) {
					t.Errorf("required result/evidence dimensions changed: got=%+v want=%+v", rm.RequestedAnswerDimensions, wantDimensions)
				}
			})
		}
	}
}
