package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceFieldOriginParam(target string) map[string]any {
	return map[string]any{
		"is_field_value_lookup": true, "target": target, "literal": "false",
		"literal_kind": "bool", "source_quote": target + " = false", "confidence": 0.9,
	}
}

// A runtime attachment cannot change the authority of a separately declared
// source field when its own validation fails. Exercise Execute, not just the
// compatibility helper, and leave every existing source validation in place.
func TestEmitAnalysisArtifactLegacyCannotReclassifyQualifiedSourceField(t *testing.T) {
	for _, target := range []string{"Config.Enabled", "server.port", "Namespace::Option", "Worker->enabled", "Owner#member"} {
		for _, tc := range []struct {
			name, wantError string
			change          func(map[string]any)
		}{
			{"wrong quote", "source_quote must be copied verbatim", func(p map[string]any) { p["source_quote"] = target + " = true" }},
			{"missing quote", "source_quote is required", func(p map[string]any) { delete(p, "source_quote") }},
			{"quote without target", "source_quote must include both", func(p map[string]any) { p["source_quote"] = "unrelated = false" }},
			{"quote without literal", "source_quote must include both", func(p map[string]any) { p["source_quote"] = target + " context" }},
			{"invalid kind", "literal_kind", func(p map[string]any) { p["literal_kind"] = "invented" }},
			{"missing confidence", "missing required field(s): confidence", func(p map[string]any) { delete(p, "confidence") }},
			{"invalid confidence", "confidence 2.00 out of", func(p map[string]any) { p["confidence"] = 2 }},
			{"missing literal", "literal is required", func(p map[string]any) { delete(p, "literal") }},
		} {
			t.Run(target+"/"+tc.name, func(t *testing.T) {
				raw, payload := artifactValueApplicabilityPayload(t, true, false)
				raw += "; " + target + " = false; unrelated = false; " + target + " context"
				field := sourceFieldOriginParam(target)
				payload["field_value_profile"] = field
				ok, summary, valid := executeArtifactValueApplicability(t, raw, payload)
				if !ok || valid.FieldValueProfile == nil || valid.RuntimeArtifactValueProfile != nil {
					t.Fatalf("valid source control failed: ok=%t rm=%+v %s", ok, valid, summary)
				}
				tc.change(field)
				ok, summary, got := executeArtifactValueApplicability(t, raw, payload)
				if ok || !strings.Contains(summary, "field_value_profile."+tc.wantError) &&
					!strings.Contains(summary, "field_value_profile "+tc.wantError) {
					t.Fatalf("source validation was bypassed: ok=%t profile=%+v %s", ok, got, summary)
				}
				if got != nil && (got.FieldValueProfile != nil || got.RuntimeArtifactValueProfile != nil) {
					t.Fatalf("failed source declaration published a replacement profile: %+v", got)
				}
			})
		}
	}
}

func TestEmitAnalysisArtifactLegacyOriginCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, target                            string
		source, explicitArtifact, excludeSource bool
	}{
		{name: "missing legacy target"},
		{name: "unqualified legacy target", target: "measured duration"},
		{name: "valid source", target: "Config.Enabled", source: true},
		{name: "explicit artifact field", target: "frame.duration", explicitArtifact: true},
		{name: "two explicit origins", target: "Config.Enabled", source: true, explicitArtifact: true},
		{name: "observation only invalid source", target: "Config.Enabled", excludeSource: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, payload := artifactValueApplicabilityPayload(t, true, false)
			raw += "; Config.Enabled = false; only the trace"
			if tc.excludeSource {
				payload["external_observation_policy"] = map[string]any{
					"current_source_mode": "exclude", "exclusion_kind": "explicit_user_exclusion",
					"current_source_exclusion_quote": "only the trace", "confidence": 1,
				}
			}
			ok, summary, before := executeArtifactValueApplicability(t, raw, payload)
			if !ok {
				t.Fatalf("baseline failed: %s", summary)
			}
			if tc.source || tc.excludeSource {
				payload["field_value_profile"] = sourceFieldOriginParam(tc.target)
				if tc.excludeSource {
					payload["field_value_profile"].(map[string]any)["source_quote"] = "Config.Enabled = true"
				}
			} else if !tc.explicitArtifact {
				payload["field_value_profile"] = map[string]any{
					"is_field_value_lookup": true, "target": tc.target, "literal": "17",
					"literal_kind": "number", "source_quote": "measured duration", "confidence": 0.9,
				}
			}
			if tc.explicitArtifact {
				payload["artifact_value_profile"] = map[string]any{
					"is_artifact_value_lookup": true, "target": "frame.duration", "value": "17",
					"literal_kind": "number", "confidence": 0.9,
				}
			}
			ok, summary, got := executeArtifactValueApplicability(t, raw, payload)
			if !ok {
				t.Fatalf("legitimate compatibility route failed: %s", summary)
			}
			if !reflect.DeepEqual(before.RuntimeArtifactScopeProfile, got.RuntimeArtifactScopeProfile) ||
				!reflect.DeepEqual(before.RuntimeQuestionProfile, got.RuntimeQuestionProfile) ||
				!reflect.DeepEqual(before.RuntimeTargetProfile, got.RuntimeTargetProfile) ||
				!reflect.DeepEqual(before.RequestedAnswerDimensions, got.RequestedAnswerDimensions) {
				t.Fatal("value origin handling changed window, target or question authority")
			}
			if (got.FieldValueProfile != nil) != tc.source {
				t.Fatalf("source authority changed: %+v", got.FieldValueProfile)
			}
			wantArtifact := tc.explicitArtifact || (!tc.source && !tc.excludeSource)
			if (got.RuntimeArtifactValueProfile != nil) != wantArtifact {
				t.Fatalf("unexpected artifact conversion: %+v %s", got.RuntimeArtifactValueProfile, summary)
			}
			if wantArtifact && got.RuntimeArtifactValueProfile.Value != "17" {
				t.Fatalf("artifact literal changed: %+v", got.RuntimeArtifactValueProfile)
			}
			if tc.explicitArtifact && got.RuntimeArtifactValueProfile.Target != "frame.duration" {
				t.Fatal("explicit artifact fields must not be treated as legacy source fields")
			}
			if tc.excludeSource && !strings.Contains(summary, "dropped invalid optional field_value_profile for observation-only") {
				t.Fatalf("optional source failure must retain its warning: %s", summary)
			}
		})
	}
}

func TestEmitAnalysisArtifactLegacyRequiresRuntimeCarrier(t *testing.T) {
	for _, valid := range []bool{false, true} {
		t.Run(map[bool]string{false: "invalid source", true: "valid source"}[valid], func(t *testing.T) {
			var payload map[string]any
			if err := json.Unmarshal([]byte(withV4Required(`{"intent":"return_value","scenario":"generic","complexity":"simple","question_kind":"return_value","keywords":["Config"],"entities":[]}`)), &payload); err != nil {
				t.Fatal(err)
			}
			payload["predicates"].(map[string]any)["is_scalar_answer"] = true
			payload["predicates"].(map[string]any)["is_count_question"] = false
			payload["answer_subject"] = map[string]any{"kind": "numeric", "confidence": 0.9}
			field := sourceFieldOriginParam("Config.Enabled")
			if !valid {
				field["source_quote"] = "Config.Enabled = true"
			}
			payload["field_value_profile"] = field
			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			mu := types.NewMutableState("Config.Enabled = false")
			res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, data)
			if err != nil || res.Success != valid {
				t.Fatalf("no-carrier source boundary changed: ok=%t err=%v %s", res.Success, err, res.Summary)
			}
			if !valid && !strings.Contains(res.Summary, "field_value_profile.source_quote") {
				t.Fatalf("wrong rejection: %s", res.Summary)
			}
			if got := mu.RequestModel(); got != nil && got.RuntimeArtifactValueProfile != nil {
				t.Fatal("absent runtime carrier manufactured an artifact value")
			}
		})
	}
}

func TestEmitAnalysisExplicitArtifactDoesNotHideInvalidSourceField(t *testing.T) {
	raw, payload := artifactValueApplicabilityPayload(t, true, false)
	raw += "; Config.Enabled = false"
	field := sourceFieldOriginParam("Config.Enabled")
	field["source_quote"] = "Config.Enabled = true"
	payload["field_value_profile"] = field
	payload["artifact_value_profile"] = map[string]any{
		"is_artifact_value_lookup": true, "target": "frame.duration", "value": "17",
		"literal_kind": "number", "confidence": 0.9,
	}
	ok, summary, got := executeArtifactValueApplicability(t, raw, payload)
	if ok || !strings.Contains(summary, "field_value_profile.source_quote") {
		t.Fatalf("explicit artifact hid invalid source field: ok=%t %s", ok, summary)
	}
	if got != nil && (got.FieldValueProfile != nil || got.RuntimeArtifactValueProfile != nil) {
		t.Fatal("failed submission partially published value profiles")
	}
}
