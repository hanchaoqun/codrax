package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitAnalysisComposedRuntimeRepairPublic(t *testing.T) {
	for _, role := range []string{"causal_attribution", "causal_contributor_set"} {
		for _, work := range []bool{false, true} {
			for _, frame := range []bool{false, true} {
				for _, field := range []string{"source_quote", "confidence"} {
					t.Run(fmt.Sprintf("%s/work=%t/frame=%t/%s", role, work, frame, field), func(t *testing.T) {
						raw, payload := mixedRuntimePayload(t, role, work, frame)
						raw = "worker; " + raw
						payload["runtime_targets"] = []any{map[string]any{
							"kind": "thread", "thread": "worker", "source": "user_explicit", "confidence": 0.95,
						}}
						target := map[string]any{"declaration": "named_target", "source_quote": "worker", "confidence": 0.95}
						payload["runtime_target_profile"] = target
						profile := payload["runtime_question_profile"].(map[string]any)
						valid, invalid, targetError := any("worker"), any("not in the current request"), "runtime_target_profile named_target requires source_quote"
						if field == "confidence" {
							valid, invalid, targetError = 0.95, 2.0, "runtime_target_profile.confidence 2.00 out of [0,1]"
						}
						target[field] = invalid
						profile["fact_families"] = []string{"count_or_duration"}
						ok, summary, rejected := executeMixedRuntime(t, raw, payload)
						if ok || rejected != nil {
							t.Fatal("both invalid independent declarations must remain rejected")
						}
						for _, want := range []string{targetError, "fact_families conflicts with causal_diagnosis",
							"Repair every independently listed error", "Each canonical field target applies only to its own diagnostic",
							"For this diagnostic, omit runtime_question_profile.fact_families"} {
							if !strings.Contains(summary, want) {
								t.Errorf("combined public repair lacks %q: %s", want, summary)
							}
						}
						if strings.Contains(summary, "repair only runtime_question_profile.fact_families") {
							t.Error("local repair still forbids correcting the independent target error")
						}

						delete(profile, "fact_families")
						ok, summary, rejected = executeMixedRuntime(t, raw, payload)
						if ok || rejected != nil || !strings.Contains(summary, targetError) || strings.Contains(summary, "fact_families conflicts") {
							t.Fatalf("family-only repair weakened the independent target gate: %s", summary)
						}
						target[field] = valid
						profile["fact_families"] = []string{"count_or_duration"}
						ok, summary, rejected = executeMixedRuntime(t, raw, payload)
						if ok || rejected != nil || !strings.Contains(summary, "fact_families conflicts with causal_diagnosis") || strings.Contains(summary, targetError) {
							t.Fatalf("target-only repair weakened the independent family gate: %s", summary)
						}
						delete(profile, "fact_families")
						ok, summary, accepted := executeMixedRuntime(t, raw, payload)
						if !ok || accepted == nil || accepted.RuntimeQuestionProfile == nil ||
							accepted.RuntimeQuestionProfile.Scope != types.RuntimeQuestionScopeCausalDiagnosis ||
							accepted.RuntimeQuestionProfile.RuntimeWorkRelationRequested != work || accepted.RuntimeQuestionProfile.FrameCausalityRequested != frame ||
							accepted.RuntimeTargetProfile == nil || accepted.RuntimeTargetProfile.SourceQuote != "worker" {
							t.Fatalf("complete repair did not preserve independent model decisions: %s %+v", summary, accepted)
						}
						var dimensions types.RequestedAnswerDimensionProfile
						encoded, _ := json.Marshal(payload["requested_answer_dimensions"])
						if err := json.Unmarshal(encoded, &dimensions); err != nil {
							t.Fatal(err)
						}
						if !reflect.DeepEqual(accepted.RequestedAnswerDimensions, &dimensions) {
							t.Fatalf("required causal, target-effect or observed-value dimension changed: %+v", accepted.RequestedAnswerDimensions)
						}
						var artifactScope types.RuntimeArtifactScopeProfile
						encoded, _ = json.Marshal(payload["runtime_artifact_scope_profile"])
						if err := json.Unmarshal(encoded, &artifactScope); err != nil {
							t.Fatal(err)
						}
						if !reflect.DeepEqual(accepted.RuntimeArtifactScopeProfile, &artifactScope) {
							t.Fatalf("full-artifact or explicit-time-window scope changed: %+v", accepted.RuntimeArtifactScopeProfile)
						}
					})
				}
			}
		}
	}
}

func TestEmitAnalysisComposedRepairWithDimensionDiagnosticPublic(t *testing.T) {
	raw, payload := mixedRuntimePayload(t, "causal_attribution", true, false)
	payload["runtime_target_profile"] = map[string]any{"declaration": "no_named_target", "confidence": 2.0}
	payload["runtime_question_profile"].(map[string]any)["fact_families"] = []string{"count_or_duration"}
	dimensions := payload["requested_answer_dimensions"].(map[string]any)
	dimensions["dimensions"] = append(dimensions["dimensions"].([]any), map[string]any{
		"index": 4, "label": "optional display", "role": "boundary", "source_quote": "not in the request", "required": false,
	})
	ok, summary, model := executeMixedRuntime(t, raw, payload)
	if ok || model != nil || !strings.Contains(summary, "runtime_target_profile.confidence") ||
		!strings.Contains(summary, "fact_families conflicts") || !strings.Contains(summary, "ignored unanchored dimension optional display") {
		t.Fatalf("independent errors and normalization warning not retained together: %s", summary)
	}
	if !strings.Contains(summary, "Repair every independently listed error") || !strings.Contains(summary, "also correct the other independently listed errors") ||
		strings.Contains(summary, "existing scope/fact_families contract intact") {
		t.Errorf("normalization guidance contradicts the sibling canonical repair: %s", summary)
	}
	payload["runtime_target_profile"].(map[string]any)["confidence"] = 0.95
	delete(payload["runtime_question_profile"].(map[string]any), "fact_families")
	rows := dimensions["dimensions"].([]any)
	rows[3].(map[string]any)["source_quote"] = "measured duration"
	ok, summary, model = executeMixedRuntime(t, raw, payload)
	if !ok || model == nil || len(model.RequestedAnswerDimensions.Dimensions) != 4 ||
		model.RequestedAnswerDimensions.Dimensions[3].Role != types.RequestedAnswerDimensionBoundary ||
		model.RequestedAnswerDimensions.Dimensions[3].Required || !model.RuntimeQuestionProfile.RuntimeWorkRelationRequested {
		t.Fatalf("complete repair must preserve the optional and required dimensions and independent flag: %s %+v", summary, model)
	}
}

func TestEmitAnalysisComposedBoundedEffectRepairPublic(t *testing.T) {
	raw, payload := mixedRuntimePayload(t, "causal_attribution", false, true)
	dimensions := payload["requested_answer_dimensions"].(map[string]any)
	dimensions["dimensions"] = dimensions["dimensions"].([]any)[1:]
	target := payload["runtime_target_profile"].(map[string]any)
	target["confidence"] = 2.0
	profile := payload["runtime_question_profile"].(map[string]any)
	profile["fact_families"] = []string{"count_or_duration"}
	ok, summary, model := executeMixedRuntime(t, raw, payload)
	if ok || model != nil || !strings.Contains(summary, "runtime_target_profile.confidence") ||
		!strings.Contains(summary, "bounded_effect_verdict_canonical_field_target=") {
		t.Fatalf("independent target error and finite verdict conflict must remain rejected: %s", summary)
	}
	if !strings.Contains(summary, "Repair every independently listed error") ||
		!strings.Contains(summary, "For this diagnostic, apply these runtime_question_profile fields") ||
		strings.Contains(summary, "apply only these runtime_question_profile fields") {
		t.Errorf("finite verdict field repair forbids correcting independent errors: %s", summary)
	}
	profile["scope"] = "bounded_effect_verdict"
	ok, summary, model = executeMixedRuntime(t, raw, payload)
	if ok || model != nil || !strings.Contains(summary, "runtime_target_profile.confidence") || strings.Contains(summary, "canonical_field_target=") {
		t.Fatalf("scope-only repair must retain the independent target gate: %s", summary)
	}
	target["confidence"] = 0.95
	profile["scope"] = "causal_diagnosis"
	ok, summary, model = executeMixedRuntime(t, raw, payload)
	if ok || model != nil || !strings.Contains(summary, "bounded_effect_verdict_canonical_field_target=") || strings.Contains(summary, "runtime_target_profile.confidence") {
		t.Fatalf("target-only repair must retain the finite verdict gate: %s", summary)
	}
	profile["scope"] = "bounded_effect_verdict"
	ok, summary, model = executeMixedRuntime(t, raw, payload)
	if !ok || model == nil || !model.RuntimeQuestionProfile.BoundedEffectVerdict() ||
		model.RuntimeQuestionProfile.RequiresFullReport() || !model.RuntimeQuestionProfile.FrameCausalityRequested ||
		model.RuntimeQuestionProfile.RuntimeWorkRelationRequested || len(model.RuntimeQuestionProfile.FactFamilies) != 1 ||
		len(model.RequestedAnswerDimensions.Dimensions) != 2 {
		t.Fatalf("complete repair must preserve finite breadth, families, dimensions and flags: %s %+v", summary, model)
	}
}
