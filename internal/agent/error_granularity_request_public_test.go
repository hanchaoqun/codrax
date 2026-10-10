package agent

import (
	"encoding/json"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Model classification is scripted; parsing, request compilation, semantic
// contract construction and finalizer teaching are the production entry points.
// The negative reproduces the accepted tuple from the §218 live log failure.
func TestErrorGranularityPublicRuntimeQuestionContract(t *testing.T) {
	const events = "启动失败附近实际记录了哪些事件"
	const cause = "日志能否确认这些事件之间的先后和因果关系"
	const scope = "一个条目失败会不会导致整个批次失败"
	for _, tc := range []struct {
		name, request, quote, intent, kind string
		want                               bool
	}{
		{"event_question_is_not_failure_scope", events + "，分别由哪个进程和线程记录；" + cause + "？", events, "explain", "mechanism", false},
		{"mixed_cause_and_independent_scope", cause + "；" + scope + "？", scope, "explain", "mechanism", true},
		{"explicit_single_proposition", scope + "？", scope, "return_value", "return_value", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus, registry := granularityPublicAnalysis(t, tc.request, tc.quote, tc.intent, tc.kind, events, cause)
			rm := bus.Mutable.RequestModel()
			if rm == nil || rm.ErrorGranularityProfile == nil || !rm.ErrorGranularityProfile.Active() {
				t.Fatal("original classifier profile must remain available for audit")
			}
			view := types.BuildAnswerSemanticViewForBusContext(bus)
			if got := view.ErrorGranularityProfile != nil && view.ErrorGranularityProfile.Active(); got != tc.want {
				t.Errorf("failure-scope hard contract=%t, want %t", got, tc.want)
			}
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if got := strings.Contains(prompt, "## Typed Error Granularity Contract"); got != tc.want {
				t.Errorf("actual finalizer forced failure-scope=%t, want %t", got, tc.want)
			}
			if tc.intent != "return_value" && (rm.RuntimeQuestionProfile == nil || rm.RuntimeQuestionProfile.Scope != types.RuntimeQuestionScopeCausalDiagnosis) {
				t.Fatal("failure-scope handling changed the independent causal request")
			}
			if tc.intent != "return_value" && types.ResolveQuestionFamily(*rm) != types.QFRootCauseTrace {
				t.Fatal("failure-scope duty replaced independent runtime causal investigation with a decision-only family")
			}
			if tc.want {
				if !strings.Contains(prompt, `"whole_batch_failure"`) || !strings.Contains(prompt, `"per_item_rejection"`) {
					t.Fatal("single yes/no proposition must not close alternatives to only its affirmative enum")
				}
				// This checks structural expressibility, not proof sufficiency:
				// a whole-batch proposition can be answered with per-item rejection.
				res, err := registry.Execute(bus, "emit_answer_document", json.RawMessage(`{"blocks":[{"id":"summary","kind":"summary","text":"The scope question and the causal question remain separate."},{"id":"scope","kind":"decision","surface_role":"principal","text":"No: the operation rejects the bad record while continuing valid siblings, rather than rejecting the whole batch.","error_granularity_verdict":"per_item_rejection"}]}`))
				if err != nil || !res.Success {
					t.Fatalf("opposite answer was rejected by public emit: %v %s", err, res.Summary)
				}
			} else {
				res, err := registry.Execute(bus, "emit_answer_document", json.RawMessage(`{"blocks":[{"id":"events","kind":"summary","text":"The question asks for recorded events, recording actors and the evidence boundary on their relationships. It does not request a batch-failure verdict."}]}`))
				if err != nil || !res.Success {
					t.Fatalf("event answer without failure verdict rejected: %v %s", err, res.Summary)
				}
			}
		})
	}
}

func granularityPublicAnalysis(t *testing.T, request, quote, intent, kind, events, cause string) (*types.BusContext, *toolpkg.Registry) {
	t.Helper()
	var params map[string]any
	if err := json.Unmarshal([]byte(`{
		"intent":"explain","scenario":"generic","complexity":"moderate","keywords":[],"entities":[],"question_kind":"mechanism",
		"intent_confidence":0.9,"complexity_confidence":0.9,"kind_confidence":0.9,
		"predicates":{"is_scalar_answer":false,"is_role_locate_lookup":false,"is_count_question":false,"is_cross_component":false,"is_relational_lookup":false,"is_category_enumeration":false,"is_history_lookup":false,"is_diagnostic_question":false,"has_per_member_table":false},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.9},
		"answer_role_profile":{"is_role_binding_requested":false,"confidence":0.9},
		"error_granularity_profile":{"is_granularity_question":true,"requested_verdict_options":["per_item_rejection"],"confidence":0.88},
		"runtime_artifact_scope_profile":{"requested_scope":"not_applicable","confidence":0.9},
		"history_selection_profile":{"mode":"not_applicable","item_kind":"not_applicable","confidence":0.9},
		"completeness_obligation":{"required":false,"source_quote":""},"predicate_axis":"",
		"call_chain_endpoints":{"source":"","sink":"","sink_mode":"exact","runtime_selection_required":false,"runtime_selection_source_quote":""},
		"runtime_selection_profile":{"is_selection_question":false,"source_quote":"","confidence":0.9},
		"requested_answer_dimensions":{"is_dimensioned_answer":false,"confidence":0.9},
		"runtime_target_profile":{"declaration":"unspecified","confidence":0.9},
		"runtime_question_profile":{"scope":"unspecified","runtime_work_relation_requested":false,"frame_causality_requested":false,"confidence":0.9}
	}`), &params); err != nil {
		t.Fatal(err)
	}
	params["intent"], params["question_kind"] = intent, kind
	params["error_granularity_profile"].(map[string]any)["source_quotes"] = []string{quote}
	if quote == events {
		// Replay the independently discarded non-contiguous second quote;
		// it must not rescue the misclassification's sole surviving anchor.
		params["error_granularity_profile"].(map[string]any)["source_quotes"] = []string{quote, "能否确认这些事件之间的因果关系"}
	} else {
		params["error_granularity_profile"].(map[string]any)["requested_verdict_options"] = []string{"whole_batch_failure"}
	}
	if intent != "return_value" {
		params["runtime_question_profile"].(map[string]any)["scope"] = "causal_diagnosis"
		var dims []any
		for _, dimension := range []struct{ text, role string }{{events, "member_set"}, {cause, "causal_attribution"}} {
			if strings.Contains(request, dimension.text) {
				dims = append(dims, map[string]any{"label": dimension.text, "role": dimension.role, "source_quote": dimension.text, "required": true, "index": len(dims) + 1})
			}
		}
		params["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": true, "confidence": 0.9, "dimensions": dims}
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	bus := &types.BusContext{RepoRoot: root, WorkDir: root, Mutable: types.NewMutableState(request)}
	registry := toolpkg.NewRegistry()
	registry.Register(&toolpkg.EmitAnalysis{})
	registry.Register(&toolpkg.EmitAnswerDocument{})
	script := &documentationCompletionPublicLLM{actions: []llm.ToolCall{{ID: "classify", Name: "emit_analysis", Params: raw}}}
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentAnalyzer, types.StageAnalyze)
	out, err := NewAnalyzerAgent(&Dependencies{LLM: script, Tools: registry, MaxIterations: 2}).Execute(ctx, traceTeachingSkill(t, "analysis-skill"))
	if err != nil || out == nil || out.Error != "" || out.AnalysisIR == nil || script.calls != 1 {
		t.Fatalf("actual classifier/IR failed: %v %+v results=%s", err, out, documentationCompletionPublicResults(bus.Mutable.DispatchToolResults()))
	}
	bus.AnalysisIR = out.AnalysisIR
	return bus, registry
}
