package tool

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeIntentScopeValidationIndependentOfPreparation(t *testing.T) {
	const request = "Inspect captures in /capture-set during 10..11 and 12..13 seconds, or the selected launch span."
	confidence, start, end, secondStart, secondEnd := .9, 10.0, 11.0, 12.0, 13.0
	for _, tc := range []struct {
		name string
		p    emitRuntimeArtifactScopeProfileParam
	}{
		{"explicit", emitRuntimeArtifactScopeProfileParam{RequestedScope: "explicit_time_window", SourceQuote: "10..11", TimeStart: &start, TimeEnd: &end}},
		{"full", emitRuntimeArtifactScopeProfileParam{RequestedScope: "full_artifact", SourceQuote: "captures in /capture-set"}},
		{"selector", emitRuntimeArtifactScopeProfileParam{RequestedScope: "bounded_selector", SourceQuote: "selected launch span", TimeStart: &start, TimeEnd: &end}},
		{"unspecified", emitRuntimeArtifactScopeProfileParam{RequestedScope: "unspecified", TimeStart: &start, TimeEnd: &end}},
		{"unanchored", emitRuntimeArtifactScopeProfileParam{RequestedScope: "explicit_time_window", SourceQuote: "query chosen by explorer", TimeStart: &start, TimeEnd: &end}},
		{"invalid_coordinates", emitRuntimeArtifactScopeProfileParam{RequestedScope: "explicit_time_window", SourceQuote: "10..11", TimeStart: &end, TimeEnd: &start}},
		{"multiwindow", emitRuntimeArtifactScopeProfileParam{RequestedScope: "explicit_time_window", TimeWindows: []types.RuntimeArtifactTimeWindow{
			{TimeStart: &start, TimeEnd: &end, SourceQuote: "10..11"},
			{TimeStart: &secondStart, TimeEnd: &secondEnd, SourceQuote: "12..13"},
		}}},
		{"invalid_member", emitRuntimeArtifactScopeProfileParam{RequestedScope: "explicit_time_window", TimeWindows: []types.RuntimeArtifactTimeWindow{
			{TimeStart: &start, TimeEnd: &end, SourceQuote: "10..11"},
			{TimeStart: &secondStart, TimeEnd: &secondEnd, SourceQuote: "explorer window"},
		}}},
		{"mixed_window_forms", emitRuntimeArtifactScopeProfileParam{RequestedScope: "explicit_time_window", TimeStart: &start, TimeEnd: &end, TimeWindows: []types.RuntimeArtifactTimeWindow{
			{TimeStart: &secondStart, TimeEnd: &secondEnd, SourceQuote: "12..13"},
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.p.Confidence = &confidence
			want, wantErr, wantWarnings := parseRuntimeArtifactScopeProfile(request, true, &tc.p)
			got, gotErr, gotWarnings := parseRuntimeArtifactScopeProfile(request, false, &tc.p)
			if !reflect.DeepEqual(got, want) || gotErr != wantErr || !reflect.DeepEqual(gotWarnings, wantWarnings) {
				t.Fatalf("pending preparation changed request validation: got=%+v/%q/%v want=%+v/%q/%v", got, gotErr, gotWarnings, want, wantErr, wantWarnings)
			}
			if tc.name == "multiwindow" {
				if len(got.ExplicitTimeWindows()) != 2 {
					t.Fatal("request members were discarded")
				}
				if _, _, single := got.ExplicitTimeWindow(); single {
					t.Fatal("multiple request members became one elected window")
				}
			}
		})
	}
}

func TestRuntimeIntentQuestionValidationIndependentOfPreparation(t *testing.T) {
	confidence, yes, no := .9, true, false
	for _, scope := range []string{"bounded_fact_set", "bounded_effect_verdict", "causal_diagnosis", "relation_analysis", "system_overview", "unspecified"} {
		for _, work := range []*bool{&no, &yes} {
			for _, frame := range []*bool{&no, &yes} {
				p := emitRuntimeQuestionProfileParam{Scope: scope, RuntimeWorkRelationRequested: work, FrameCausalityRequested: frame, Confidence: &confidence, SourceQuote: "request facts"}
				if scope == "bounded_fact_set" || scope == "bounded_effect_verdict" {
					p.FactFamilies = []string{"count_or_duration", "other_observed_value"}
				}
				want, wantErr, wantWarnings := parseRuntimeQuestionProfile("request facts", true, &p, nil)
				got, gotErr, gotWarnings := parseRuntimeQuestionProfile("request facts", false, &p, nil)
				if !reflect.DeepEqual(got, want) || gotErr != wantErr || !reflect.DeepEqual(gotWarnings, wantWarnings) {
					t.Fatalf("pending preparation changed %s intent: got=%+v/%q/%v want=%+v/%q/%v", scope, got, gotErr, gotWarnings, want, wantErr, wantWarnings)
				}
			}
		}
	}
	for _, tc := range []struct {
		name string
		p    emitRuntimeQuestionProfileParam
		want string
	}{
		{"work_decision", emitRuntimeQuestionProfileParam{Scope: "bounded_fact_set", FrameCausalityRequested: &no, FactFamilies: []string{"count_or_duration"}}, "runtime_work_relation_requested"},
		{"frame_decision", emitRuntimeQuestionProfileParam{Scope: "causal_diagnosis", RuntimeWorkRelationRequested: &no}, "frame_causality_requested"},
		{"empty_families", emitRuntimeQuestionProfileParam{Scope: "bounded_fact_set", RuntimeWorkRelationRequested: &no, FrameCausalityRequested: &no}, "requires one or more fact_families"},
		{"invalid_family", emitRuntimeQuestionProfileParam{Scope: "bounded_fact_set", RuntimeWorkRelationRequested: &no, FrameCausalityRequested: &no, FactFamilies: []string{"untyped"}}, "invalid value"},
		{"nonbounded_families", emitRuntimeQuestionProfileParam{Scope: "relation_analysis", RuntimeWorkRelationRequested: &no, FrameCausalityRequested: &no, FactFamilies: []string{"count_or_duration"}}, "conflicts"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.p.Confidence = &confidence
			got, errText, _ := parseRuntimeQuestionProfile("request facts", false, &tc.p, nil)
			if got != nil || !strings.Contains(errText, tc.want) {
				t.Fatalf("pending preparation bypassed required declaration: got=%+v error=%q", got, errText)
			}
		})
	}
}

func runtimeIntentPublicPayload(t *testing.T) (string, map[string]any) {
	t.Helper()
	const request = "For captures in /capture-set, list allocation events and stacks for threads 101 and 202 during 10.000..10.050 seconds."
	scope := map[string]any{"requested_scope": "explicit_time_window", "time_start": 10.0, "time_end": 10.05, "source_quote": "10.000..10.050", "confidence": .95}
	var payload map[string]any
	if err := json.Unmarshal(boundedScopeAuthorityPublicParams(t, scope, request), &payload); err != nil {
		t.Fatal(err)
	}
	payload["runtime_target_profile"] = map[string]any{"declaration": "named_target", "source_quote": "threads 101 and 202", "confidence": .95}
	payload["runtime_targets"] = []any{
		map[string]any{"kind": "thread", "pid": 101, "source": "user_explicit", "confidence": .95},
		map[string]any{"kind": "thread", "pid": 202, "source": "user_explicit", "confidence": .95},
	}
	payload["runtime_question_profile"] = map[string]any{"scope": "bounded_fact_set", "runtime_work_relation_requested": false, "frame_causality_requested": false, "fact_families": []string{"other_observed_value", "count_or_duration"}, "confidence": .95}
	payload["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": false, "confidence": .95}
	return request, payload
}

func TestEmitAnalysisRuntimeIntentBeforePreparationPublic(t *testing.T) {
	request, payload := runtimeIntentPublicPayload(t)
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), raw...)
	ctx := &types.BusContext{Mutable: types.NewMutableState(request)}
	result, err := (&EmitAnalysis{}).Execute(ctx, raw)
	if err != nil || !result.Success {
		t.Fatalf("public request declaration rejected: %+v %v", result, err)
	}
	rm := ctx.Mutable.RequestModel()
	if rm == nil || !rm.RuntimeQuestionProfile.BoundedFactSet() || len(rm.RuntimeQuestionProfile.FactFamilies) != 2 ||
		!rm.RuntimeTargetProfile.NamedTarget() || len(rm.RuntimeTargets) != 2 || rm.RuntimeTargets[0].PID != 101 || rm.RuntimeTargets[1].PID != 202 {
		t.Fatalf("declared breadth/targets were lost before discovery: %+v", rm)
	}
	start, end, ok := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindow()
	if !ok || start != 10 || end != 10.05 {
		t.Fatalf("explicit ruler was lost before discovery: %+v", rm.RuntimeArtifactScopeProfile)
	}
	if decided, full := types.RuntimeTraceReportShapeAuthority(rm); !decided || full {
		t.Fatal("request window bypassed the finite-fact declaration and enabled a causal report")
	}
	if emitAnalysisHasRuntimeArtifactCarrier(ctx) || ctx.RuntimeArtifactPreflight.HasRuntimeArtifact() || rm.PerfTrace != nil || rm.LogTriage != nil || rm.RuntimeArtifactValueProfile.Active() {
		t.Fatal("declaration became material or observed-value authority")
	}
	if out := RunTraceQuerySystemSupplement(ctx); len(out.Executed) > 0 {
		t.Fatalf("request declaration selected an unadmitted source: %+v", out)
	}
	if !bytes.Equal(raw, before) {
		t.Fatal("request preservation mutated submitted JSON")
	}
}

func TestEmitAnalysisRuntimeIntentRequiresCompanionBreadthPublic(t *testing.T) {
	for _, field := range []string{"profile", "not_applicable", "runtime_work_relation_requested", "frame_causality_requested"} {
		t.Run(field, func(t *testing.T) {
			request, payload := runtimeIntentPublicPayload(t)
			switch field {
			case "profile":
				delete(payload, "runtime_question_profile")
			case "not_applicable":
				payload["runtime_question_profile"] = map[string]any{"scope": "not_applicable", "runtime_work_relation_requested": false, "frame_causality_requested": false, "confidence": .9}
			default:
				delete(payload["runtime_question_profile"].(map[string]any), field)
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			ctx := &types.BusContext{Mutable: types.NewMutableState(request)}
			result, err := (&EmitAnalysis{}).Execute(ctx, raw)
			if err != nil || result.Success || ctx.Mutable.RequestModel() != nil || !strings.Contains(result.Summary, "runtime_question_profile") {
				t.Fatalf("missing/conflicting runtime breadth was silently replaced: %+v %v", result, err)
			}
		})
	}
}

func TestEmitAnalysisRuntimeIntentLegacyRepositoryCompatibility(t *testing.T) {
	for _, question := range []json.RawMessage{nil, json.RawMessage(`{"scope":"not_applicable","confidence":0.9}`)} {
		ctx := &types.BusContext{Mutable: types.NewMutableState("Explain the repository architecture.")}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal([]byte(withV4Required(`{"intent":"explain","scenario":"generic","complexity":"moderate","keywords":["architecture"],"question_kind":"mechanism"}`)), &payload); err != nil {
			t.Fatal(err)
		}
		// The shared attached-trace fixture inserts unspecified plus both
		// booleans. Exercise genuine old repository callers without that shim.
		delete(payload, "runtime_question_profile")
		delete(payload, "runtime_target_profile")
		if question != nil {
			payload["runtime_question_profile"] = question
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		result, err := (&EmitAnalysis{}).Execute(ctx, raw)
		if err != nil || !result.Success {
			t.Fatalf("legacy repository call acquired runtime obligations: %+v %v", result, err)
		}
		rm := ctx.Mutable.RequestModel()
		if rm.RuntimeArtifactScopeProfile.RequestedScope != types.RuntimeArtifactScopeNotApplicable || rm.RuntimeQuestionProfile.Scope != types.RuntimeQuestionScopeNotApplicable || emitAnalysisHasRuntimeArtifactCarrier(ctx) {
			t.Fatalf("repository call acquired runtime intent/material: %+v", rm)
		}
	}
}

func TestEmitAnalysisRuntimeIntentSchemaTeaching(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	description := schema.Properties["runtime_artifact_scope_profile"].Description
	if !strings.Contains(description, skill.AnalysisRuntimeIntentAvailabilityTeaching) || strings.Contains(description, "Use not_applicable when no runtime artifact is attached or referenced") {
		t.Fatalf("schema contradicts pre-discovery request intent: %s", description)
	}
}
