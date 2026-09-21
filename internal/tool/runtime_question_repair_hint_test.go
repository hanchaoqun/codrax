package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Existing grammar tests exercise the profile in isolation. The public
// executor always supplies the parsed classifiers to the production parser.
func parseRuntimeQuestionProfile(raw string, runtimeArtifactCarrier bool, p *emitRuntimeQuestionProfileParam, dimensions *types.RequestedAnswerDimensionProfile) (*types.RuntimeQuestionProfile, string, []string) {
	return parseRuntimeQuestionProfileWithClassifiers(raw, runtimeArtifactCarrier, p, dimensions, "", "")
}

func runtimeRepairConflictPayload(t *testing.T, intent, scenario string, work, frame, families bool) (string, map[string]any) {
	t.Helper()
	raw, payload := mixedRuntimePayload(t, "causal_attribution", false, frame)
	payload["intent"], payload["scenario"] = intent, scenario
	if intent == "root_cause" || scenario == "root_cause" {
		payload["predicates"].(map[string]any)["is_diagnostic_question"] = true
		payload["diagnostic_profile"].(map[string]any)["is_diagnostic"] = true
	}
	rows := payload["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)
	// A topology declaration is not a discovered-cause declaration, regardless
	// of its label or source quote. The other requested verdict remains intact.
	rows[0].(map[string]any)["role"] = "relation_path"
	profile := payload["runtime_question_profile"].(map[string]any)
	profile["runtime_work_relation_requested"] = work
	profile["frame_causality_requested"] = frame
	if families {
		profile["fact_families"] = []string{"count_or_duration", "io_latency"}
	}
	return raw, payload
}

func TestRuntimeQuestionRepairConflictingRootClassifierDoesNotSelectFinite(t *testing.T) {
	for _, classifiers := range []struct{ intent, scenario string }{
		{"root_cause", "generic"}, {"explain", "root_cause"}, {"root_cause", "root_cause"},
	} {
		for _, work := range []bool{false, true} {
			for _, frame := range []bool{false, true} {
				for _, families := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/work=%t/frame=%t/families=%t", classifiers.intent, classifiers.scenario, work, frame, families), func(t *testing.T) {
						raw, payload := runtimeRepairConflictPayload(t, classifiers.intent, classifiers.scenario, work, frame, families)
						before, _ := json.Marshal(payload)
						ok, summary, rm := executeMixedRuntime(t, raw, payload)
						if ok {
							t.Fatalf("a root classifier must not grant the missing required causal role: %s", summary)
						}
						for _, want := range []string{
							"conflicting root-cause classifier",
							"no unique finite repair target",
							"Full causal request:", "Finite-only request:",
							"add the independently requested required causal_attribution or causal_contributor_set",
							"preserve the independent required target_effect_verdict",
							"non-root-cause intent and scenario",
							"all diagnostic predicate/profile flags false",
							fmt.Sprintf("runtime_work_relation_requested=%t", work),
							fmt.Sprintf("frame_causality_requested=%t", frame),
							"next COMPLETE model-owned object",
						} {
							if !strings.Contains(summary, want) {
								t.Errorf("missing conflict repair guidance %q: %s", want, summary)
							}
						}
						for _, forbidden := range []string{"bounded_effect_verdict_canonical_field_target=", "causal_diagnosis_canonical_field_target=", "uniquely selects"} {
							if strings.Contains(summary, forbidden) {
								t.Errorf("conflicting declarations received a unique breadth target %q: %s", forbidden, summary)
							}
						}
						if rm != nil && rm.RuntimeQuestionProfile != nil {
							t.Fatalf("rejected declarations acquired runtime authority: %+v", rm.RuntimeQuestionProfile)
						}
						after, _ := json.Marshal(payload)
						if string(before) != string(after) {
							t.Fatal("repair guidance mutated model-owned declarations")
						}
					})
				}
			}
		}
	}
}

func TestRuntimeQuestionRepairFiniteUniqueTargetRemainsFinite(t *testing.T) {
	for _, work := range []bool{false, true} {
		for _, frame := range []bool{false, true} {
			t.Run(fmt.Sprintf("work=%t/frame=%t", work, frame), func(t *testing.T) {
				raw, payload := runtimeRepairConflictPayload(t, "explain", "performance_bottleneck", work, frame, true)
				ok, summary, _ := executeMixedRuntime(t, raw, payload)
				if ok || !strings.Contains(summary, "already-typed required target_effect_verdict uniquely selects") ||
					!strings.Contains(summary, `bounded_effect_verdict_canonical_field_target={"fact_families":["count_or_duration","io_latency"],"scope":"bounded_effect_verdict"}`) {
					t.Fatalf("finite request lost its unique repair target: ok=%t %s", ok, summary)
				}
				payload["runtime_question_profile"].(map[string]any)["scope"] = "bounded_effect_verdict"
				ok, summary, rm := executeMixedRuntime(t, raw, payload)
				if !ok {
					t.Fatalf("finite target did not converge: %s", summary)
				}
				if decided, allowed := types.RuntimeTraceReportShapeAuthority(rm); !decided || allowed {
					t.Fatal("work, frame, relation_path, or performance classifier granted full report authority")
				}
			})
		}
	}
}

func TestRuntimeQuestionRepairConflictingFiniteTupleStillNeedsModelChoice(t *testing.T) {
	for _, classifiers := range []struct{ intent, scenario string }{
		{"root_cause", "generic"}, {"explain", "root_cause"}, {"root_cause", "root_cause"},
	} {
		for _, optional := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s/optional_causal=%t", classifiers.intent, classifiers.scenario, optional), func(t *testing.T) {
				raw, payload := runtimeRepairConflictPayload(t, classifiers.intent, classifiers.scenario, true, false, true)
				payload["runtime_question_profile"].(map[string]any)["scope"] = "bounded_effect_verdict"
				if optional {
					dimensions := payload["requested_answer_dimensions"].(map[string]any)
					rows := dimensions["dimensions"].([]any)
					dimensions["dimensions"] = append(rows, map[string]any{
						"index": 4, "label": "optional cause", "role": "causal_attribution", "required": false,
						"source_quote": rows[0].(map[string]any)["source_quote"],
					})
				}
				ok, summary, rm := executeMixedRuntime(t, raw, payload)
				if ok || !strings.Contains(summary, "Full causal request:") || !strings.Contains(summary, "Finite-only request:") ||
					!strings.Contains(summary, "preserve the independent required target_effect_verdict") || strings.Contains(summary, "canonical_field_target=") {
					t.Fatalf("root classifier conflict must not select either unique breadth: ok=%t %s", ok, summary)
				}
				if rm != nil && rm.RuntimeQuestionProfile != nil {
					t.Fatal("root classifier or optional causal dimension authorized the rejected tuple")
				}
			})
		}
	}
}

func TestRuntimeQuestionRepairModelOwnedAlternativesConverge(t *testing.T) {
	for _, causal := range []bool{false, true} {
		for _, work := range []bool{false, true} {
			for _, frame := range []bool{false, true} {
				t.Run(fmt.Sprintf("causal=%t/work=%t/frame=%t", causal, work, frame), func(t *testing.T) {
					raw, payload := runtimeRepairConflictPayload(t, "root_cause", "root_cause", work, frame, true)
					profile := payload["runtime_question_profile"].(map[string]any)
					dimensions := payload["requested_answer_dimensions"].(map[string]any)
					rows := dimensions["dimensions"].([]any)
					if causal {
						delete(profile, "fact_families")
						dimensions["dimensions"] = append(rows, map[string]any{
							"index": 4, "label": "independent cause discovery", "role": "causal_attribution", "required": true,
							"source_quote": rows[0].(map[string]any)["source_quote"],
						})
					} else {
						profile["scope"] = "bounded_effect_verdict"
						payload["intent"], payload["scenario"] = "explain", "generic"
						payload["predicates"].(map[string]any)["is_diagnostic_question"] = false
						payload["diagnostic_profile"].(map[string]any)["is_diagnostic"] = false
					}
					ok, summary, rm := executeMixedRuntime(t, raw, payload)
					if !ok {
						t.Fatalf("model-owned coherent choice failed: %s", summary)
					}
					if decided, allowed := types.RuntimeTraceReportShapeAuthority(rm); !decided || allowed != causal {
						t.Fatalf("wrong report authority: decided=%t allowed=%t causal=%t", decided, allowed, causal)
					}
					if rm.RuntimeQuestionProfile.RuntimeWorkRelationRequested != work || rm.RuntimeQuestionProfile.FrameCausalityRequested != frame || rm.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() != frame {
						t.Fatal("independent decisions or exact artifact window changed")
					}
					got := rm.RequestedAnswerDimensions.Dimensions
					wantCount := 3
					if causal {
						wantCount++
					}
					if len(got) != wantCount || got[0].Role != types.RequestedAnswerDimensionRelationPath || got[1].Role != types.RequestedAnswerDimensionTargetEffectVerdict || got[2].Role != types.RequestedAnswerDimensionObservedValue {
						t.Fatalf("lost independent topology/verdict/measurement: %+v", got)
					}
				})
			}
		}
	}
}
