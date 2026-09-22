package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func artifactValueApplicabilityPayload(t *testing.T, scalar, causal bool) (string, map[string]any) {
	t.Helper()
	raw, payload := mixedRuntimePayload(t, "causal_attribution", false, true)
	if !causal {
		profile := payload["runtime_question_profile"].(map[string]any)
		profile["scope"] = "bounded_fact_set"
		profile["fact_families"] = []string{"count_or_duration"}
		dims := payload["requested_answer_dimensions"].(map[string]any)
		row := dims["dimensions"].([]any)[2].(map[string]any)
		row["index"] = 1
		dims["dimensions"] = []any{row}
	}
	if scalar {
		payload["predicates"].(map[string]any)["is_scalar_answer"] = true
		payload["intent"], payload["question_kind"] = "return_value", "return_value"
		payload["answer_subject"] = map[string]any{"kind": "numeric", "confidence": 0.9}
	}
	return raw, payload
}

func executeArtifactValueApplicability(t *testing.T, raw string, payload map[string]any) (bool, string, *types.RequestModel) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	before := string(data)
	mu := types.NewMutableState(raw)
	mu.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "attached.systrace"},
		Observations: []types.PerfObservation{{
			Authority: types.PerfObservationAuthorityPreTriageModelExtraction,
			Kind:      "span", Subject: "operation", DurationMs: 17, LineStart: 4, LineEnd: 5,
		}},
	})
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, data)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != before || mu.PerfTrace().Observations[0].DurationMs != 17 {
		t.Fatal("optional profile handling changed original input or observation")
	}
	return res.Success, res.Summary, mu.RequestModel()
}

func TestEmitAnalysisArtifactValueApplicabilityBeforeSemanticValidation(t *testing.T) {
	for _, causal := range []bool{false, true} {
		for _, scalar := range []bool{false, true} {
			if causal && scalar {
				continue // mixed causal/scalar is covered by the existing public test.
			}
			for _, tc := range []struct {
				name, profile string
				valid         bool
			}{
				{"complete", `{"is_artifact_value_lookup":true,"value":"17","target":"duration","literal_kind":"number","confidence":0.9}`, true},
				{"missing value", `{"is_artifact_value_lookup":true,"target":"duration","literal_kind":"number","confidence":0.9}`, false},
				{"empty object", `{}`, false},
				{"bad kind", `{"is_artifact_value_lookup":true,"value":"17","target":"duration","literal_kind":"invented","confidence":0.9}`, false},
				{"bad confidence", `{"is_artifact_value_lookup":true,"value":"17","target":"duration","literal_kind":"number","confidence":2}`, false},
				{"disabled malformed", `{"is_artifact_value_lookup":false,"confidence":2}`, false},
			} {
				t.Run(fmt.Sprintf("causal=%t/scalar=%t/%s", causal, scalar, tc.name), func(t *testing.T) {
					raw, payload := artifactValueApplicabilityPayload(t, scalar, causal)
					ok, summary, baseline := executeArtifactValueApplicability(t, raw, payload)
					if !ok {
						t.Fatalf("baseline fixture invalid: %s", summary)
					}
					payload["artifact_value_profile"] = json.RawMessage(tc.profile)
					ok, summary, got := executeArtifactValueApplicability(t, raw, payload)
					if scalar && !tc.valid {
						if ok || !strings.Contains(summary, "artifact_value_profile") {
							t.Fatalf("applicable invalid profile must still reject: ok=%t %s", ok, summary)
						}
						return
					}
					if !ok {
						t.Fatalf("inapplicable profile forced retry: %s", summary)
					}
					if scalar {
						if got.RuntimeArtifactValueProfile == nil || got.RuntimeArtifactValueProfile.Value != "17" {
							t.Fatal("valid scalar value lost")
						}
					} else if got.RuntimeArtifactValueProfile != nil || !strings.Contains(summary, "dropped artifact_value_profile outside predicates.is_scalar_answer=true") {
						t.Fatalf("inapplicable profile retained or silently dropped: %+v %s", got.RuntimeArtifactValueProfile, summary)
					}
					if !reflect.DeepEqual(baseline.RuntimeArtifactScopeProfile, got.RuntimeArtifactScopeProfile) ||
						!reflect.DeepEqual(baseline.RuntimeQuestionProfile, got.RuntimeQuestionProfile) ||
						!reflect.DeepEqual(baseline.RequestedAnswerDimensions, got.RequestedAnswerDimensions) ||
						!reflect.DeepEqual(baseline.RuntimeTargetProfile, got.RuntimeTargetProfile) {
						t.Fatal("optional value handling changed scope, dimensions or target authority")
					}
				})
			}
		}
	}
}

func TestEmitAnalysisArtifactValueLegacyConversionCannotBypassApplicability(t *testing.T) {
	for _, scalar := range []bool{false, true} {
		t.Run(fmt.Sprintf("scalar=%t", scalar), func(t *testing.T) {
			raw, payload := artifactValueApplicabilityPayload(t, scalar, false)
			raw += "; only the trace"
			payload["external_observation_policy"] = map[string]any{
				"current_source_mode": "exclude", "exclusion_kind": "explicit_user_exclusion",
				"current_source_exclusion_quote": "only the trace", "confidence": 1,
			}
			payload["field_value_profile"] = map[string]any{
				"is_field_value_lookup": true, "literal": "17", "literal_kind": "number",
				"source_quote": "measured duration", "confidence": 0.9,
			}
			ok, summary, got := executeArtifactValueApplicability(t, raw, payload)
			if !ok {
				t.Fatalf("artifact-only compatibility request failed: %s", summary)
			}
			if got.FieldValueProfile != nil {
				t.Fatal("artifact value acquired current-source authority")
			}
			if scalar {
				if got.RuntimeArtifactValueProfile == nil || got.RuntimeArtifactValueProfile.Value != "17" {
					t.Fatal("legacy scalar conversion lost")
				}
			} else if got.RuntimeArtifactValueProfile != nil || strings.Contains(summary, "converted runtime-artifact") {
				t.Fatalf("legacy path resurrected non-scalar profile: %+v %s", got.RuntimeArtifactValueProfile, summary)
			}
		})
	}
}

func TestEmitAnalysisArtifactValueScopeStillRejectsMalformedJSONTypes(t *testing.T) {
	raw, payload := artifactValueApplicabilityPayload(t, false, false)
	payload["artifact_value_profile"] = "not an object"
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{Mutable: types.NewMutableState(raw), AttachedHitrace: "attached.systrace"}
	res, err := (&EmitAnalysis{}).Execute(ctx, data)
	if err == nil && res.Success {
		t.Fatalf("scope cleanup must not bypass JSON decoding: %s", res.Summary)
	}
}

func TestEmitAnalysisArtifactValueScopePreservesIndependentSourceField(t *testing.T) {
	for _, grounded := range []bool{false, true} {
		t.Run(fmt.Sprintf("grounded=%t", grounded), func(t *testing.T) {
			raw, payload := artifactValueApplicabilityPayload(t, false, true)
			raw += "; Config.Enabled = false"
			quote := "Config.Enabled = true"
			if grounded {
				quote = "Config.Enabled = false"
			}
			payload["field_value_profile"] = map[string]any{
				"is_field_value_lookup": true, "target": "Config.Enabled", "literal": "false",
				"literal_kind": "bool", "source_quote": quote, "confidence": 0.9,
			}
			ok, summary, got := executeArtifactValueApplicability(t, raw, payload)
			if !grounded {
				if ok || !strings.Contains(summary, "field_value_profile.source_quote") {
					t.Fatalf("runtime conversion erased independent source failure: ok=%t %s", ok, summary)
				}
				return
			}
			if !ok || got.FieldValueProfile == nil || got.FieldValueProfile.Literal != "false" || got.RuntimeArtifactValueProfile != nil {
				t.Fatalf("independent source field not preserved: ok=%t rm=%+v %s", ok, got, summary)
			}
		})
	}
}
