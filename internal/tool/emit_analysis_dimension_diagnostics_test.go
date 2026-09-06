package tool

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// B1567: exercise the real emit boundary, where a provenance warning used to
// disappear behind the later missing-causal-dimension consistency error.
func TestEmitAnalysisReturnsDimensionNormalizationDiagnostics(t *testing.T) {
	const objective = "分析工作线程卡顿原因与可消除量，并说明提升空间之间的关系"
	for _, tc := range []struct {
		name             string
		label            string
		quote            string
		role             string
		scope            string
		families         []string
		unanchoredExtra  bool
		wantSuccess      bool
		wantDiagnostic   bool
		wantExistingGate string
	}{
		{name: "submitted_causal_role_unanchored_quote", label: "分方向根因", quote: "分析...卡顿原因", role: "causal_contributor_set", scope: "causal_diagnosis", wantDiagnostic: true, wantExistingGate: "causal_diagnosis requires"},
		{name: "single_cause_unanchored_quote", label: "单项机制", quote: "分析...卡顿原因", role: "causal_attribution", scope: "causal_diagnosis", wantDiagnostic: true, wantExistingGate: "causal_diagnosis requires"},
		{name: "correct_quote_preserves_causal_breadth", label: "分方向根因", quote: "卡顿原因", role: "causal_contributor_set", scope: "causal_diagnosis", wantSuccess: true},
		{name: "anchored_label_remains_valid_fallback", label: "卡顿原因", quote: "分析...卡顿原因", role: "causal_contributor_set", scope: "causal_diagnosis", wantSuccess: true},
		{name: "irrelevant_unanchored_row_remains_soft", label: "分方向根因", quote: "卡顿原因", role: "causal_contributor_set", scope: "causal_diagnosis", unanchoredExtra: true, wantSuccess: true},
		{name: "genuinely_missing_causal_role", label: "测量结果", quote: "可消除量", role: "observed_value", scope: "causal_diagnosis", wantExistingGate: "causal_diagnosis requires"},
		{name: "runtime_profile_reject_also_carries_normalization", label: "分方向根因", quote: "分析...卡顿原因", role: "causal_contributor_set", scope: "causal_diagnosis", families: []string{"target_scheduler_state"}, wantDiagnostic: true, wantExistingGate: "fact_families conflicts"},
		{name: "valid_dimension_does_not_weaken_fact_family_gate", label: "分方向根因", quote: "卡顿原因", role: "causal_contributor_set", scope: "causal_diagnosis", families: []string{"target_scheduler_state"}, wantExistingGate: "fact_families conflicts"},
		{name: "finite_effect_uses_same_diagnostic_path", label: "有限影响判断", quote: "是否...造成延迟", role: "target_effect_verdict", scope: "bounded_effect_verdict", families: []string{"target_scheduler_state"}, wantDiagnostic: true, wantExistingGate: "requires a required requested_answer_dimensions role=target_effect_verdict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var payload map[string]any
			if err := json.Unmarshal([]byte(`{
				"intent":"trace","scenario":"generic","complexity":"simple",
				"intent_confidence":0.95,"complexity_confidence":0.95,"kind_confidence":0.95,
				"keywords":["sched_switch","sched_wakeup"],"entities":["worker-20","binder:100_1-101"],
				"question_kind":"mechanism",
				"predicates":{"is_scalar_answer":false,"is_role_locate_lookup":false,"is_count_question":false,"is_cross_component":false,"is_relational_lookup":false,"is_category_enumeration":false,"is_history_lookup":false,"is_diagnostic_question":false,"has_per_member_table":false},
				"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.95},
				"answer_role_profile":{"is_role_binding_requested":false,"confidence":0.95},
				"error_granularity_profile":{"is_granularity_question":false,"confidence":0.95},
				"runtime_target_profile":{"declaration":"no_named_target","confidence":0.95},
				"history_selection_profile":{"mode":"not_applicable","item_kind":"not_applicable","confidence":0.95},
				"completeness_obligation":{"required":false,"source_quote":""}
			}`), &payload); err != nil {
				t.Fatal(err)
			}
			payload["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": "full_artifact", "source_quote": objective, "confidence": 0.95}
			profile := map[string]any{"scope": tc.scope, "runtime_work_relation_requested": true, "frame_causality_requested": false, "source_quote": "卡顿原因", "confidence": 0.95}
			if tc.families != nil {
				profile["fact_families"] = tc.families
			}
			payload["runtime_question_profile"] = profile
			payload["requested_answer_dimensions"] = map[string]any{
				"is_dimensioned_answer": true, "confidence": 0.95,
				"dimensions": []map[string]any{
					{"index": 1, "label": tc.label, "role": tc.role, "source_quote": tc.quote, "required": true},
					{"index": 2, "label": "可消除量", "role": "impact", "source_quote": "可消除量", "required": true},
					{"index": 3, "label": "提升空间之间的关系", "role": "comparison_axis", "source_quote": "提升空间之间的关系", "required": true},
				},
			}
			if tc.unanchoredExtra {
				dimensions := payload["requested_answer_dimensions"].(map[string]any)
				dimensions["dimensions"] = append(dimensions["dimensions"].([]map[string]any), map[string]any{
					"index": 4, "label": "可选展示形式", "role": "boundary", "source_quote": "并非原始请求", "required": false,
				})
			}
			params, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			original := bytes.Clone(params)
			ctx := &types.BusContext{Mutable: types.NewMutableState(objective)}
			ctx.Mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: "attached.systrace", Signals: []string{"sched_switch"}}})
			result, err := (&EmitAnalysis{}).Execute(ctx, params)
			if err != nil {
				t.Fatal(err)
			}
			if result.Success != tc.wantSuccess || !bytes.Equal(params, original) {
				t.Fatalf("unexpected admission or rewritten model input: success=%t summary=%s", result.Success, result.Summary)
			}
			if tc.wantExistingGate != "" && !strings.Contains(result.Summary, tc.wantExistingGate) {
				t.Fatalf("existing structural contract changed: %s", result.Summary)
			}
			if tc.wantDiagnostic {
				for _, part := range []string{"requested_answer_dimensions ignored unanchored dimension " + tc.label, "source_quote", "role", "required", "Other requested dimensions may coexist"} {
					if !strings.Contains(result.Summary, part) {
						t.Errorf("real emit reply hid normalization cause or repair guidance %q: %s", part, result.Summary)
					}
				}
			} else if !tc.unanchoredExtra && strings.Contains(result.Summary, "ignored unanchored dimension") {
				t.Fatalf("invented provenance failure for valid or genuinely absent causal role: %s", result.Summary)
			}
			if tc.wantSuccess {
				model := ctx.Mutable.RequestModel()
				if model == nil || model.RuntimeQuestionProfile == nil || !model.RuntimeQuestionProfile.RequiresFullReport() ||
					!model.RuntimeQuestionProfile.RuntimeWorkRelationRequested || model.RuntimeQuestionProfile.FrameCausalityRequested ||
					model.RequestedAnswerDimensions == nil || len(model.RequestedAnswerDimensions.Dimensions) != 3 {
					t.Fatalf("valid causal contract or sibling dimensions changed: %+v", model)
				}
			}
		})
	}
}
