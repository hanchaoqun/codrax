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

// b1626MultiWindowAnalysisParams supplies native public emit arguments only;
// callers may add their own typed target fields before the real Execute call.
func b1626MultiWindowAnalysisParams(t *testing.T, windowsJSON, questionScope, sourceQuote string) json.RawMessage {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(withV4Required(`{"intent":"explain","scenario":"generic","complexity":"moderate","keywords":["trace","window","state"],"entities":["trace"],"question_kind":"mechanism"}`)), &payload); err != nil {
		t.Fatal(err)
	}
	var windows any
	if err := json.Unmarshal([]byte(windowsJSON), &windows); err != nil {
		t.Fatal(err)
	}
	payload["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": "explicit_time_window", "time_windows": windows, "source_quote": sourceQuote, "confidence": 1.0}
	payload["runtime_target_profile"] = map[string]any{"declaration": "no_named_target", "confidence": 1.0}
	profile := map[string]any{"scope": questionScope, "source_quote": sourceQuote, "runtime_work_relation_requested": false, "frame_causality_requested": false, "confidence": 1.0}
	if questionScope == "bounded_fact_set" {
		profile["fact_families"] = []string{"target_scheduler_state", "count_or_duration"}
	}
	if questionScope == "causal_diagnosis" {
		payload["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": true, "confidence": 1.0, "dimensions": []any{map[string]any{"index": 1, "label": "cause", "role": "causal_attribution", "source_quote": sourceQuote, "required": true}}}
	}
	payload["runtime_question_profile"] = profile
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestB1626PublicEmitAnalysisPreservesOrderedWindowMembers(t *testing.T) {
	const request = "Compare this trace in 0..0.003 seconds and 1..1.030 seconds."
	const members = `[{"time_start":0,"time_end":0.003,"source_quote":"0..0.003"},{"time_start":1,"time_end":1.030,"source_quote":"1..1.030"}]`
	for _, scope := range []string{"bounded_fact_set", "causal_diagnosis"} {
		t.Run(scope, func(t *testing.T) {
			ctx := &types.BusContext{Mutable: types.NewMutableState(request), AttachedHitrace: "inline trace"}
			raw := b1626MultiWindowAnalysisParams(t, members, scope, request)
			before := append([]byte(nil), raw...)
			result, err := (&EmitAnalysis{}).Execute(ctx, raw)
			if err != nil || !result.Success {
				t.Fatalf("valid native multi-window request rejected: %+v err=%v", result, err)
			}
			rm := ctx.Mutable.RequestModel()
			if rm == nil || rm.RuntimeArtifactScopeProfile == nil {
				t.Fatal("public emitter did not store scope")
			}
			encoded, err := json.Marshal(rm.RuntimeArtifactScopeProfile)
			if err != nil {
				t.Fatal(err)
			}
			var saved map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &saved); err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(saved["time_windows"], &got); err != nil {
				t.Fatalf("actual persisted scope dropped native request members: %s (%v)", encoded, err)
			}
			if err := json.Unmarshal([]byte(members), &want); err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("ordered member facts changed: got=%s want=%s", saved["time_windows"], members)
			}
			if _, _, ok := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindow(); ok || !rm.RuntimeArtifactScopeProfile.Active() {
				t.Fatal("multiple members must be active, never a single/envelope window")
			}
			if _, allowed := types.RuntimeTraceReportShapeAuthority(rm); allowed != (scope == "causal_diagnosis") {
				t.Fatalf("member collection changed bounded/full report qualification: allowed=%t", allowed)
			}
			if !bytes.Equal(raw, before) {
				t.Fatal("model input bytes changed")
			}
		})
	}
}

func TestB1626PublicAnalysisSchemaCarriesMemberCoordinates(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	profile := schema["properties"].(map[string]any)["runtime_artifact_scope_profile"].(map[string]any)
	properties := profile["properties"].(map[string]any)
	if _, ok := properties["time_windows"]; !ok {
		t.Fatal("actual analyzer schema cannot express separate requested window members")
	}
	if !strings.Contains(profile["description"].(string), skill.AnalysisRuntimeWindowMembersTeaching) {
		t.Fatal("actual analyzer schema lost shared member teaching")
	}
}

func TestB1626PublicEmitPreservesSingleRepeatedAndReorderedMembers(t *testing.T) {
	const request = "For this trace, compare 0..1, 0.25..0.75, and 3..4; keep the requested order."
	for _, tc := range []struct {
		name, members string
		count         int
	}{
		{"single", `[{"time_start":0,"time_end":1,"source_quote":"0..1"}]`, 1},
		{"reordered nested", `[{"time_start":3,"time_end":4,"source_quote":"3..4"},{"time_start":0,"time_end":1,"source_quote":"0..1"},{"time_start":0.25,"time_end":0.75,"source_quote":"0.25..0.75"}]`, 3},
		{"repeated", `[{"time_start":0,"time_end":1,"source_quote":"0..1"},{"time_start":0,"time_end":1,"source_quote":"0..1"}]`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var payload map[string]any
			if err := json.Unmarshal(b1626MultiWindowAnalysisParams(t, tc.members, "bounded_fact_set", request), &payload); err != nil {
				t.Fatal(err)
			}
			// Only member quotes are authoritative in the list form. The
			// producer must not invent a top-level phrase from those members.
			delete(payload["runtime_artifact_scope_profile"].(map[string]any), "source_quote")
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			ctx := &types.BusContext{Mutable: types.NewMutableState(request), AttachedHitrace: "inline trace"}
			result, err := (&EmitAnalysis{}).Execute(ctx, raw)
			if err != nil || !result.Success {
				t.Fatalf("valid member shape rejected: %+v err=%v", result, err)
			}
			profile := ctx.Mutable.RequestModel().RuntimeArtifactScopeProfile
			if !profile.Active() || profile.SourceQuote != "" || len(profile.ExplicitTimeWindows()) != tc.count {
				t.Fatalf("member shape changed: %+v", profile)
			}
			var want []types.RuntimeArtifactTimeWindow
			if err := json.Unmarshal([]byte(tc.members), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(profile.TimeWindows, want) {
				t.Fatal("public producer sorted/deduplicated/changed member bytes")
			}
			if _, _, ok := profile.ExplicitTimeWindow(); ok != (tc.count == 1) {
				t.Fatal("single-window compatibility guessed a member")
			}
			if tc.name == "repeated" {
				if i, ok := profile.MatchExplicitTimeWindow(0, 1); ok || i != -1 {
					t.Fatal("repeated public members gained first-wins identity")
				}
			}
		})
	}
}

func TestB1626PublicEmitRejectsWholeMalformedMemberListWithoutReplacingPrior(t *testing.T) {
	const request = "This trace has requested windows 0..1 and 3..4."
	for _, tc := range []struct {
		name, members string
		mutate        func(map[string]any)
	}{
		{"empty", `[]`, nil},
		{"missing start", `[{"time_end":1,"source_quote":"0..1"}]`, nil},
		{"missing end", `[{"time_start":0,"source_quote":"0..1"}]`, nil},
		{"negative", `[{"time_start":-1,"time_end":1,"source_quote":"0..1"}]`, nil},
		{"zero duration", `[{"time_start":1,"time_end":1,"source_quote":"0..1"}]`, nil},
		{"later missing quote", `[{"time_start":0,"time_end":1,"source_quote":"0..1"},{"time_start":3,"time_end":4}]`, nil},
		{"foreign quote", `[{"time_start":0,"time_end":1,"source_quote":"from a tool summary"}]`, nil},
		{"mixed scalar envelope", `[{"time_start":0,"time_end":1,"source_quote":"0..1"},{"time_start":3,"time_end":4,"source_quote":"3..4"}]`, func(p map[string]any) { p["time_start"], p["time_end"] = 0, 4 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mu := types.NewMutableState(request)
			mu.SetRequestModel(types.RequestModel{RawRequest: "previous accepted request"})
			before, _ := json.Marshal(mu.RequestModel())
			raw := b1626MultiWindowAnalysisParams(t, tc.members, "bounded_fact_set", request)
			if tc.mutate != nil {
				var payload map[string]any
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatal(err)
				}
				tc.mutate(payload["runtime_artifact_scope_profile"].(map[string]any))
				var err error
				raw, err = json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
			}
			result, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, AttachedHitrace: "inline trace"}, raw)
			if err != nil || result.Success || !strings.Contains(result.Summary, "runtime_artifact_scope_profile") {
				t.Fatalf("malformed full member list did not yield scoped repair: %+v err=%v", result, err)
			}
			after, _ := json.Marshal(mu.RequestModel())
			if !bytes.Equal(before, after) {
				t.Fatal("rejection replaced previous request with a valid-prefix guess")
			}
		})
	}
}

func TestB1626SingleWindowOnlyNormalizerRulesDoNotConsumeMemberCollections(t *testing.T) {
	const quote = "0..1 and 3..4"
	var profile types.RuntimeArtifactScopeProfile
	if err := json.Unmarshal([]byte(`{"requested_scope":"explicit_time_window","source_quote":"0..1 and 3..4","time_windows":[{"time_start":0,"time_end":1,"source_quote":"0..1 and 3..4"},{"time_start":3,"time_end":4,"source_quote":"0..1 and 3..4"}]}`), &profile); err != nil {
		t.Fatal(err)
	}
	rm := explicitWindowCausalRequestModelForSubtopicAuthority()
	rm.RuntimeArtifactScopeProfile = &profile
	before, _ := json.Marshal(rm.SubTopics)
	if warning := normalizeSingleTargetExplicitWindowCausalSubTopics(&rm); warning != "" {
		t.Fatal("multiple windows lost distinct planning topics")
	}
	after, _ := json.Marshal(rm.SubTopics)
	if !bytes.Equal(before, after) {
		t.Fatal("multiple windows' topics mutated")
	}
	boundary := &emitEnumerationBoundaryParam{DeclaredCount: 2, SourceQuote: quote}
	if enumerationBoundaryDuplicatesExplicitRuntimeWindow(boundary, &profile) {
		t.Fatal("multi-member request quote erased a genuine member enumeration")
	}
	profile.TimeWindows = profile.TimeWindows[:1]
	profile.SourceQuote = ""
	if !enumerationBoundaryDuplicatesExplicitRuntimeWindow(boundary, &profile) {
		t.Fatal("true-single list lost old exact-quote duplicate boundary handling")
	}
	if warning := normalizeSingleTargetExplicitWindowCausalSubTopics(&rm); warning == "" || len(rm.SubTopics) != 0 {
		t.Fatal("true-single list lost the old single-window rule")
	}
}
