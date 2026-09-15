package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// These tests exercise the public emitter, not its private scalar normalizer.
// A candidate role is varied independently of already accepted whole-request
// shape. No parser, runtime-event, or LLM-end-to-end evidence is claimed here.
func b1691AnalysisPayload(t *testing.T) map[string]any {
	t.Helper()
	var p map[string]any
	if err := json.Unmarshal([]byte(withV4Required(`{"intent":"explain","scenario":"architecture_explain","complexity":"moderate","keywords":["worker","entry","flow"],"entities":["worker"],"question_kind":"mechanism","predicate_axis":"call","answer_subject":{"kind":"generic","confidence":0.7}}`)), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func b1691Dimensions(roles ...string) map[string]any {
	rows := make([]any, 0, len(roles))
	for i, role := range roles {
		label := fmt.Sprintf("dimension%d", i+1)
		rows = append(rows, map[string]any{"index": i + 1, "label": label, "role": role, "source_quote": label, "required": true})
	}
	return map[string]any{"is_dimensioned_answer": true, "confidence": 0.95, "dimensions": rows}
}

func b1691EmitAnalysis(t *testing.T, p map[string]any, objective string, runtime bool) *types.RequestModel {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), raw...)
	ctx := &types.BusContext{Mutable: types.NewMutableState(objective)}
	if runtime {
		ctx.AttachedHitrace = "inline trace carrier"
	}
	result, err := (&EmitAnalysis{}).Execute(ctx, raw)
	if err != nil || !result.Success || ctx.Mutable.RequestModel() == nil {
		t.Fatalf("public valid analysis rejected: err=%v result=%+v", err, result)
	}
	if !bytes.Equal(before, raw) {
		t.Fatal("public emitter mutated model input bytes")
	}
	return ctx.Mutable.RequestModel()
}

func b1691SetRole(p map[string]any, role string) {
	p["answer_role_profile"] = map[string]any{
		"is_role_binding_requested": true, "required_candidate_roles": []string{role},
		"source_quotes": []string{"worker"}, "confidence": 0.9,
	}
}

func b1691RuntimeProfiles(p map[string]any, scope string) {
	p["predicate_axis"] = ""
	p["runtime_artifact_scope_profile"] = map[string]any{
		"requested_scope": "explicit_time_window", "confidence": 1.0, "source_quote": "5..6 and 8..9",
		"time_windows": []any{
			map[string]any{"time_start": 5, "time_end": 6, "source_quote": "5..6"},
			map[string]any{"time_start": 8, "time_end": 9, "source_quote": "8..9"},
		},
	}
	p["runtime_target_profile"] = map[string]any{"declaration": "no_named_target", "confidence": 1.0}
	profile := map[string]any{"scope": scope, "confidence": 1.0, "source_quote": "worker", "runtime_work_relation_requested": false, "frame_causality_requested": false}
	if scope == "bounded_fact_set" {
		profile["fact_families"] = []string{"target_scheduler_state", "count_or_duration"}
	}
	if scope == "causal_diagnosis" {
		p["requested_answer_dimensions"] = b1691Dimensions("causal_contributor_set")
	}
	p["runtime_question_profile"] = profile
}

func TestB1691PublicSingleRolePreservesWholeRequestShape(t *testing.T) {
	previous := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(previous) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	cases := []struct {
		name    string
		runtime bool
		build   func(map[string]any, string)
	}{
		{"call_chain_without_dimensions", false, func(p map[string]any, entry string) {
			p["intent"], p["question_kind"] = "trace", "call_chain"
			p["call_chain_endpoints"] = map[string]any{"source": entry, "sink": "", "sink_mode": "discover_path"}
			p["predicates"].(map[string]any)["is_cross_component"] = true
		}},
		{"call_chain_relation_and_purpose", false, func(p map[string]any, entry string) {
			p["intent"], p["question_kind"] = "trace", "call_chain"
			p["call_chain_endpoints"] = map[string]any{"source": entry, "sink": "", "sink_mode": "discover_path"}
			p["requested_answer_dimensions"] = b1691Dimensions("relation_path", "function_or_purpose")
		}},
		{"mechanism_relation_dimension", false, func(p map[string]any, _ string) {
			p["requested_answer_dimensions"] = b1691Dimensions("relation_path")
		}},
		{"mechanism_workflow_dimension", false, func(p map[string]any, _ string) {
			p["predicate_axis"] = "flow"
			p["requested_answer_dimensions"] = b1691Dimensions("stage_or_workflow")
		}},
		{"independent_purpose_and_branch", false, func(p map[string]any, _ string) {
			p["requested_answer_dimensions"] = b1691Dimensions("function_or_purpose", "branch_behavior")
			p["sub_topics"] = []any{map[string]any{"summary": "dimension1", "entities": []string{"worker"}}}
		}},
		{"single_purpose_explanation", false, func(p map[string]any, _ string) {
			p["requested_answer_dimensions"] = b1691Dimensions("function_or_purpose")
		}},
		{"two_principal_values", false, func(p map[string]any, _ string) {
			p["requested_answer_dimensions"] = b1691Dimensions("observed_value", "observed_value")
		}},
		{"explicit_current_mechanism", false, func(p map[string]any, _ string) {
			p["current_source_explanation_profile"] = map[string]any{
				"is_current_source_explanation_requested": true, "modes": []string{"explain_current_mechanism"},
				"source_quotes": []string{"current mechanism"}, "confidence": 0.95,
			}
		}},
		{"runtime_windowed_facts", true, func(p map[string]any, _ string) { b1691RuntimeProfiles(p, "bounded_fact_set") }},
		{"runtime_windowed_causes", true, func(p map[string]any, _ string) { b1691RuntimeProfiles(p, "causal_diagnosis") }},
	}
	for _, tc := range cases {
		for _, entry := range []string{"Engine.Begin", "Coordinator.open"} {
			for _, role := range []string{"agent", "function"} {
				t.Run(tc.name+"/"+entry+"/"+role, func(t *testing.T) {
					objective := "Follow " + entry + " through worker; explain dimension1, dimension2, and current mechanism in 5..6 and 8..9."
					p := b1691AnalysisPayload(t)
					p["entities"] = []string{entry, "worker"}
					tc.build(p, entry)
					base := b1691EmitAnalysis(t, p, objective, tc.runtime)
					if base.Predicates.IsScalarAnswer || base.Predicates.IsRoleLocateLookup || types.ResolveQuestionFamily(*base) == types.QFRoleLookup {
						t.Fatalf("baseline is not the intended non-scalar request: %+v", base.Predicates)
					}
					if tc.runtime && (base.RuntimeArtifactScopeProfile == nil || !base.RuntimeArtifactScopeProfile.Active() || base.RuntimeQuestionProfile == nil) {
						t.Fatal("runtime baseline must actually retain both scope and question declarations")
					}
					if _, present := p["requested_answer_dimensions"].(map[string]any)["dimensions"]; present && (base.RequestedAnswerDimensions == nil || !base.RequestedAnswerDimensions.Active()) {
						t.Fatal("requested dimensions did not survive public provenance validation")
					}
					b1691SetRole(p, role)
					got := b1691EmitAnalysis(t, p, objective, tc.runtime)
					if got.AnswerRoleProfile == nil || !got.AnswerRoleProfile.Active() || len(got.AnswerRoleProfile.RequiredCandidateRoles) != 1 {
						t.Fatal("single role did not actually reach normalization")
					}
					if got.Predicates != base.Predicates || !reflect.DeepEqual(got.AnswerSubject, base.AnswerSubject) {
						t.Errorf("single candidate role overwrote accepted whole-request shape: predicates got=%+v want=%+v; subject got=%+v want=%+v", got.Predicates, base.Predicates, got.AnswerSubject, base.AnswerSubject)
					}
					if got.AnalyzerHints.Kind != base.AnalyzerHints.Kind || got.PredicateAxis != base.PredicateAxis || !reflect.DeepEqual(got.SubTopics, base.SubTopics) || types.ResolveQuestionFamily(*got) != types.ResolveQuestionFamily(*base) {
						t.Error("candidate metadata changed request family, relation axis, kind, or topic shape")
					}
					for _, pair := range [][2]any{
						{got.RequestedAnswerDimensions, base.RequestedAnswerDimensions},
						{got.CurrentSourceExplanationProfile, base.CurrentSourceExplanationProfile},
						{got.CallChainEndpointProfile, base.CallChainEndpointProfile},
						{got.RuntimeArtifactScopeProfile, base.RuntimeArtifactScopeProfile},
						{got.RuntimeQuestionProfile, base.RuntimeQuestionProfile},
					} {
						if !reflect.DeepEqual(pair[0], pair[1]) {
							t.Errorf("independent typed request carrier changed: got=%+v want=%+v", pair[0], pair[1])
						}
					}
					_, gotProjection := types.RuntimeTraceReportShapeAuthority(got)
					_, wantProjection := types.RuntimeTraceReportShapeAuthority(base)
					if gotProjection != wantProjection || (tc.name == "runtime_windowed_causes" && !gotProjection) {
						t.Error("role metadata changed Trace causal projection authority")
					}
				})
			}
		}
	}
}

func TestB1691PublicSingleLiteralRoleCompletionStillWorks(t *testing.T) {
	for _, tc := range []struct {
		role, axis string
		subject    types.AnswerSubjectKind
	}{
		{"function", "call", types.SubjectFunctionName},
		{"config_key", "configure", types.SubjectConfigKey},
		{"type", "define", types.SubjectTypeName},
		{"agent", "call", types.SubjectTypeName},
	} {
		for _, alreadyScalar := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/already_scalar=%t", tc.role, alreadyScalar), func(t *testing.T) {
				p := b1691AnalysisPayload(t)
				p["predicate_axis"] = tc.axis
				predicates := p["predicates"].(map[string]any)
				predicates["is_scalar_answer"] = alreadyScalar
				predicates["is_relational_lookup"] = !alreadyScalar
				b1691SetRole(p, tc.role)
				rm := b1691EmitAnalysis(t, p, "Which single name is responsible for worker?", false)
				if !rm.Predicates.IsScalarAnswer || !rm.Predicates.IsRoleLocateLookup || rm.Predicates.IsRelationalLookup || rm.AnswerSubject.Kind != tc.subject || types.ResolveQuestionFamily(*rm) != types.QFRoleLookup {
					t.Fatalf("legitimate one-name role completion regressed: %+v subject=%+v", rm.Predicates, rm.AnswerSubject)
				}
			})
		}
	}
}

func TestB1691PublicScalarAttributesDoNotBecomeIndependentAnswers(t *testing.T) {
	for _, tc := range []string{"location_and_proof", "optional_explanation", "unanchored_explanation", "locate_current_code"} {
		t.Run(tc, func(t *testing.T) {
			p := b1691AnalysisPayload(t)
			b1691SetRole(p, "function")
			objective := "Which single function handles worker? Give dimension1, dimension2, dimension3, dimension4; locate current code."
			switch tc {
			case "location_and_proof":
				p["requested_answer_dimensions"] = b1691Dimensions("observed_value", "source_location", "evidence_source", "boundary")
			case "optional_explanation":
				p["requested_answer_dimensions"] = b1691Dimensions("function_or_purpose")
				p["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)[0].(map[string]any)["required"] = false
			case "unanchored_explanation":
				p["requested_answer_dimensions"] = b1691Dimensions("relation_path")
				p["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)[0].(map[string]any)["source_quote"] = "not in this request"
				p["requested_answer_dimensions"].(map[string]any)["dimensions"].([]any)[0].(map[string]any)["label"] = "also absent from this request"
			case "locate_current_code":
				p["current_source_explanation_profile"] = map[string]any{
					"is_current_source_explanation_requested": true, "modes": []string{"locate_current_code"},
					"source_quotes": []string{"locate current code"}, "confidence": 0.95,
				}
			}
			rm := b1691EmitAnalysis(t, p, objective, false)
			if !rm.Predicates.IsScalarAnswer || !rm.Predicates.IsRoleLocateLookup || rm.AnswerSubject.Kind != types.SubjectFunctionName {
				t.Fatalf("one scalar with contextual attributes lost completion: %+v", rm.Predicates)
			}
			if tc == "location_and_proof" && (rm.RequestedAnswerDimensions == nil || len(rm.RequestedAnswerDimensions.Dimensions) != 4) {
				t.Fatal("positive scalar-attribute matrix did not retain its actual dimensions")
			}
			if tc == "locate_current_code" && (rm.CurrentSourceExplanationProfile == nil || !rm.CurrentSourceExplanationProfile.Active()) {
				t.Fatal("positive current-code locator was not active")
			}
			if tc == "unanchored_explanation" && rm.RequestedAnswerDimensions.Active() {
				t.Fatal("unanchored negative control must actually drop the dimension")
			}
		})
	}
}

func TestB1691PublicExplicitRuntimeScalarRemainsScalar(t *testing.T) {
	p := b1691AnalysisPayload(t)
	b1691RuntimeProfiles(p, "bounded_fact_set")
	p["question_kind"] = "return_value"
	p["predicates"].(map[string]any)["is_scalar_answer"] = true
	p["runtime_artifact_scope_profile"].(map[string]any)["time_windows"] = []any{map[string]any{"time_start": 5, "time_end": 6, "source_quote": "5..6"}}
	p["runtime_artifact_scope_profile"].(map[string]any)["source_quote"] = "5..6"
	p["runtime_question_profile"].(map[string]any)["fact_families"] = []string{"direct_waker"}
	b1691SetRole(p, "literal_value")
	rm := b1691EmitAnalysis(t, p, "Identify the one direct waker of worker in 5..6.", true)
	if !rm.Predicates.IsScalarAnswer || !rm.Predicates.IsRoleLocateLookup || rm.AnswerSubject.Kind != types.SubjectStringLiteral || !rm.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() {
		t.Fatalf("an explicitly scalar runtime request lost its scalar role completion or scope: %+v", rm)
	}
	if _, projection := types.RuntimeTraceReportShapeAuthority(rm); projection {
		t.Fatal("one observed identity was widened into causal projection")
	}
}
