package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the public tool with an actual runtime carrier, not only the
// consistency helper. Raw wording and artifact range never grant breadth.
func mixedRuntimePayload(t *testing.T, role string, reverse, window bool) (string, map[string]any) {
	t.Helper()
	causal, verdict, measured := "discover the delaying cause", "whether the quota affected this operation", "measured duration"
	if role == "causal_contributor_set" {
		causal, verdict, measured = "哪条依赖真正拖住了响应", "能否把耗时较长的 IO 直接当成这次操作的根因", "毫秒耗时"
	}
	phrases := []string{causal, verdict, measured}
	roles := []string{role, "target_effect_verdict", "observed_value"}
	if reverse {
		phrases[0], phrases[1] = phrases[1], phrases[0]
		roles[0], roles[1] = roles[1], roles[0]
	}
	raw := strings.Join(phrases, "; ")
	var payload map[string]any
	if err := json.Unmarshal([]byte(withV4Required(`{"intent":"explain","scenario":"generic","complexity":"simple","question_kind":"behavior","keywords":["operation"],"entities":[]}`)), &payload); err != nil {
		t.Fatal(err)
	}
	dimensions := make([]any, 0, len(roles))
	for i, r := range roles {
		dimensions = append(dimensions, map[string]any{"index": i + 1, "label": fmt.Sprintf("visible dimension %d", i+1), "role": r, "source_quote": phrases[i], "required": true})
	}
	payload["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": true, "confidence": 0.95, "dimensions": dimensions}
	payload["runtime_target_profile"] = map[string]any{"declaration": "no_named_target", "confidence": 0.95}
	payload["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": "full_artifact", "source_quote": raw, "confidence": 0.95}
	if window {
		raw = "2..2.020; " + raw
		payload["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": "explicit_time_window", "time_start": 2.0, "time_end": 2.020, "source_quote": "2..2.020", "confidence": 0.95}
	}
	payload["runtime_question_profile"] = map[string]any{"scope": "causal_diagnosis", "runtime_work_relation_requested": reverse, "frame_causality_requested": window, "source_quote": raw, "confidence": 0.95}
	return raw, payload
}

func executeMixedRuntime(t *testing.T, raw string, payload map[string]any) (bool, string, *types.RequestModel) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	before := string(data)
	ctx := &types.BusContext{Mutable: types.NewMutableState(raw)}
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: "attached.systrace", Signals: []string{"sched_switch"}}})
	result, err := (&EmitAnalysis{}).Execute(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != before {
		t.Fatal("Execute mutated the submitted JSON")
	}
	return result.Success, result.Summary, ctx.Mutable.RequestModel()
}

func TestEmitAnalysisMixedRuntimeRequiredDimensions(t *testing.T) {
	for _, role := range []string{"causal_attribution", "causal_contributor_set"} {
		for _, reverse := range []bool{false, true} {
			for _, window := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/reverse=%t/window=%t", role, reverse, window), func(t *testing.T) {
					raw, payload := mixedRuntimePayload(t, role, reverse, window)
					ok, summary, rm := executeMixedRuntime(t, raw, payload)
					if !ok {
						t.Fatalf("independently required causal and finite dimensions must coexist: %s", summary)
					}
					profile := rm.RuntimeQuestionProfile
					if profile.Scope != types.RuntimeQuestionScopeCausalDiagnosis || len(profile.FactFamilies) != 0 || profile.RuntimeWorkRelationRequested != reverse || profile.FrameCausalityRequested != window {
						t.Fatalf("mixed request breadth/independent decisions changed: %+v", profile)
					}
					got := rm.RequestedAnswerDimensions.Dimensions
					want := payload["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)
					if len(got) != len(want) {
						t.Fatalf("lost requested dimensions: %+v", got)
					}
					for i, d := range got {
						w := want[i].(map[string]any)
						if string(d.Role) != w["role"] || d.SourceQuote != w["source_quote"] || d.Label != w["label"] || !d.Required || d.Index != i+1 {
							t.Fatalf("dimension %d rewritten: %+v, want %+v", i, d, w)
						}
					}
					if decided, allowed := types.RuntimeTraceReportShapeAuthority(rm); !decided || !allowed {
						t.Fatal("a finite sub-verdict suppressed independently required causal report")
					}
					if rm.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() != window {
						t.Fatal("artifact range was lost or invented")
					}
				})
			}
		}
	}
}

func TestEmitAnalysisMixedRuntimeRequiredRoleBoundary(t *testing.T) {
	for _, role := range []string{"causal_attribution", "causal_contributor_set"} {
		for _, tc := range []struct {
			name, scope            string
			optional, omit, wantOK bool
		}{
			{"optional causal cannot grant full breadth", "causal_diagnosis", true, false, false},
			{"verdict alone cannot grant full breadth", "causal_diagnosis", false, true, false},
			{"required causal cannot enter bounded effect", "bounded_effect_verdict", false, false, false},
			{"required causal cannot enter bounded facts", "bounded_fact_set", false, false, false},
			{"optional causal does not widen finite verdict", "bounded_effect_verdict", true, false, true},
			{"pure finite verdict stays finite", "bounded_effect_verdict", false, true, true},
		} {
			t.Run(role+"/"+tc.name, func(t *testing.T) {
				raw, payload := mixedRuntimePayload(t, role, false, true)
				profile := payload["runtime_question_profile"].(map[string]any)
				profile["scope"] = tc.scope
				if tc.scope != "causal_diagnosis" {
					profile["fact_families"] = []string{"count_or_duration"}
				}
				dims := payload["requested_answer_dimensions"].(map[string]any)
				rows := dims["dimensions"].([]any)
				if tc.optional {
					rows[0].(map[string]any)["required"] = false
				}
				if tc.omit {
					dims["dimensions"] = rows[1:]
				}
				ok, summary, rm := executeMixedRuntime(t, raw, payload)
				if ok != tc.wantOK {
					t.Fatalf("success=%t, want %t: %s", ok, tc.wantOK, summary)
				}
				if ok {
					if decided, allowed := types.RuntimeTraceReportShapeAuthority(rm); !decided || allowed {
						t.Fatal("optional causal role or explicit window widened a finite verdict")
					}
				} else if !strings.Contains(summary, "runtime_question_profile.scope=") {
					t.Fatalf("rejected before the intended structural boundary: %s", summary)
				}
				if !ok && !tc.optional && !tc.omit && tc.scope != "causal_diagnosis" {
					for _, want := range []string{"causal_diagnosis_canonical_field_target=", "Preserve every requested dimension, including any independent target_effect_verdict", "keep runtime_work_relation_requested and frame_causality_requested unchanged"} {
						if !strings.Contains(summary, want) {
							t.Fatalf("bounded mixed retry must preserve independent obligations (%q): %s", want, summary)
						}
					}
					profile["scope"] = "causal_diagnosis"
					delete(profile, "fact_families")
					ok, summary, rm = executeMixedRuntime(t, raw, payload)
					if !ok || len(rm.RequestedAnswerDimensions.Dimensions) != 3 || !rm.RuntimeQuestionProfile.FrameCausalityRequested {
						t.Fatalf("bounded mixed retry did not converge without losing obligations: %s", summary)
					}
				}
			})
		}
	}
}

func TestEmitAnalysisMixedRuntimeFactFamilyRepairPreservesDimensions(t *testing.T) {
	for _, role := range []string{"causal_attribution", "causal_contributor_set"} {
		for _, work := range []bool{false, true} {
			for _, frame := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/work=%t/frame=%t", role, work, frame), func(t *testing.T) {
					raw, payload := mixedRuntimePayload(t, role, work, frame)
					profile := payload["runtime_question_profile"].(map[string]any)
					profile["fact_families"] = []string{"count_or_duration"}
					before, _ := json.Marshal(payload["requested_answer_dimensions"])
					ok, summary, _ := executeMixedRuntime(t, raw, payload)
					for _, want := range []string{
						`causal_diagnosis_canonical_field_target={"scope":"causal_diagnosis"}`,
						"For this diagnostic, omit runtime_question_profile.fact_families",
						fmt.Sprintf("runtime_work_relation_requested is an independent model decision and must remain exactly %t", work),
						fmt.Sprintf("frame_causality_requested is likewise an independent model decision and must remain exactly %t", frame),
						"preserve every requested dimension, including any independently requested target_effect_verdict",
					} {
						if ok || !strings.Contains(summary, want) {
							t.Fatalf("mixed repair must retain the complete answer contract (%q): %s", want, summary)
						}
					}
					delete(profile, "fact_families")
					ok, summary, rm := executeMixedRuntime(t, raw, payload)
					if !ok {
						t.Fatalf("one-field repair did not converge: %s", summary)
					}
					after, _ := json.Marshal(payload["requested_answer_dimensions"])
					if !reflect.DeepEqual(before, after) || len(rm.RequestedAnswerDimensions.Dimensions) != 3 || rm.RuntimeQuestionProfile.RuntimeWorkRelationRequested != work || rm.RuntimeQuestionProfile.FrameCausalityRequested != frame {
						t.Fatal("structural retry erased a dimension or independent decision")
					}
				})
			}
		}
	}
}
