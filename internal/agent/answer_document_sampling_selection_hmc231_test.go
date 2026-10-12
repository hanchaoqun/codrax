package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Use the public analyzer and IR compiler, including the finite named-target
// profile from the failed natural run. A profile-free renderer test misses the
// projection that previously discarded this correctly queried aggregate.
func samplingSelectionAnalysisContext(t *testing.T) *types.AgentContext {
	t.Helper()
	const objective = "请看附件中1到2秒文档工作线程（tid 11）的采样调用路径，哪些分支权重比较大？给出层次和占比，并说明不同采样事件显示的差异。"
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ctx := &types.AgentContext{Objective: objective, RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentAnalyzer, Stage: types.StageAnalyze,
		AttachedHitrace: string(data), AttachedHitraceSource: path, Mutable: types.NewMutableState(objective)}
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Observations: []types.PerfObservation{{
		Subject: "model sampling navigation", Summary: "old merged cycles=42 and split sys root work", LineStart: 2, LineEnd: 6,
	}}})
	raw := json.RawMessage(`{
		"intent":"explain","scenario":"generic","complexity":"moderate","language":"zh",
		"keywords":["perf_sample","sample_weight"],"entities":["tid 11","document-worker"],
		"question_kind":"mechanism","predicate_axis":"flow",
		"intent_confidence":0.9,"complexity_confidence":0.9,"kind_confidence":0.9,
		"predicates":{"is_scalar_answer":false,"is_role_locate_lookup":false,"is_count_question":false,"is_cross_component":false,"is_relational_lookup":false,"is_category_enumeration":false,"is_history_lookup":false,"is_diagnostic_question":false,"has_per_member_table":true},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.9},
		"answer_role_profile":{"is_role_binding_requested":false,"confidence":0.9},
		"error_granularity_profile":{"is_granularity_question":false,"confidence":0.9},
		"runtime_artifact_scope_profile":{"requested_scope":"explicit_time_window","time_start":1,"time_end":2,"source_quote":"1到2秒","confidence":0.9},
		"runtime_question_profile":{"scope":"bounded_fact_set","fact_families":["count_or_duration"],"runtime_work_relation_requested":false,"frame_causality_requested":false,"confidence":0.9},
		"runtime_target_profile":{"declaration":"named_target","source_quote":"tid 11","confidence":0.95},
		"runtime_targets":[{"kind":"thread","thread":"document-worker","source":"user_explicit","confidence":0.95}],
		"history_selection_profile":{"mode":"not_applicable","item_kind":"not_applicable","confidence":0.9},
		"completeness_obligation":{"required":false,"source_quote":""},
		"call_chain_endpoints":{"source":"","sink":"","sink_mode":"exact","runtime_selection_required":false,"runtime_selection_source_quote":""},
		"runtime_selection_profile":{"is_selection_question":false,"source_quote":"","confidence":0.9},
		"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.9,"dimensions":[
			{"index":1,"label":"采样层次","role":"stage_or_workflow","source_quote":"采样调用路径","required":true},
			{"index":2,"label":"采样占比","role":"observed_value","source_quote":"层次和占比","required":true}
		]}
	}`)
	before := append([]byte(nil), raw...)
	emitted, err := (&tool.EmitAnalysis{}).Execute(types.ToolBusContext(ctx, types.AgentAnalyzer), raw)
	if err != nil || !emitted.Success {
		t.Fatalf("public analyzer: %v %+v", err, emitted)
	}
	if !bytes.Equal(before, raw) {
		t.Fatal("analyzer mutated submitted classification")
	}
	ctx.AnalysisIR, err = buildAnalysisIR(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rm := &ctx.AnalysisIR.RequestModel
	if !rm.RuntimeQuestionProfile.BoundedFactSet() || !rm.RuntimeQuestionProfile.RequestsFactFamily(types.RuntimeQuestionFactCountOrDuration) ||
		len(rm.RuntimeTargets) != 1 || rm.RuntimeTargets[0].Thread != "document-worker" || rm.RuntimeTargets[0].Source != "user_explicit" {
		t.Fatalf("actual finite target profile was lost: %+v", rm)
	}
	ctx.AgentName, ctx.Stage = types.AgentFinalizer, types.StageFinalize
	return ctx
}

func TestSamplingSelectionPublicFiniteFinalizerHMC231(t *testing.T) {
	for _, view := range []string{"perf_sampling_tree", "perf_stats", "window_stats"} {
		t.Run(view, func(t *testing.T) {
			ctx := samplingSelectionAnalysisContext(t)
			params, _ := json.Marshal(map[string]any{"source": "attached_trace", "view": view, "pid": 11, "time_start": 1, "time_end": 2})
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
			if err != nil || !result.Success {
				t.Fatalf("public query: %v %+v", err, result)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			ledger := answerDocObservationLedger(ctx)
			before, _ := json.Marshal(ledger)
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			for _, want := range []string{"### 采样前缀树", "sys;root", "/system/lib.so", "/app/A.so", "/app/B.so", "total_weight=35", "stack_weight=30", "unavailable_weight=5", "total_weight=7", "total_weight=100", "不是实际调用"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("actual finite finalizer lost %q", want)
				}
			}
			if strings.Contains(prompt, types.TraceNoteKeyPerfSamplingTree+"={") {
				t.Error("raw transport JSON duplicated the native handoff")
			}
			projected, superseded := answerDocFinalizerObservationRecords(ctx, ledger.Records)
			if superseded != 1 {
				t.Errorf("same-capture pre-triage narration was not superseded: %d", superseded)
			}
			for _, record := range projected {
				if record.ID == "perf:observation:0" {
					t.Error("old merged sampling narration survived native query")
				}
			}
			after, _ := json.Marshal(ledger)
			if !bytes.Equal(before, after) {
				t.Fatal("prompt retention changed original audit ledger")
			}
			if decided, allowed := types.RuntimeTraceReportShapeAuthority(&ctx.AnalysisIR.RequestModel); !decided || allowed {
				t.Fatal("aggregate display enabled a causal report")
			}
			for _, record := range ledger.Records {
				if _, ok := tool.DecodeTracePerfSamplingTree(record); ok && (record.Origin != types.AnswerEvidenceOriginRuntimeArtifact || record.Role != types.AnswerAggregateRoleSupportingCoverage) {
					t.Fatal("sampling display acquired source or principal causal authority")
				}
			}
		})
	}
}

func TestSamplingSelectionPublicFinalizerBoundariesHMC231(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*types.AgentContext, *types.ToolResult)
	}{
		{"foreign_source_binding", func(_ *types.AgentContext, r *types.ToolResult) { r.Observations[0].SourceRef.Path += ".other" }},
		{"query_window_binding", func(_ *types.AgentContext, r *types.ToolResult) { r.Observations[0].SourceRef.QueryWindowStartTs = 0 }},
		{"query_target_binding", func(_ *types.AgentContext, r *types.ToolResult) { r.Observations[0].SourceRef.QueryTargetPID = 12 }},
		{"principal_role", func(_ *types.AgentContext, r *types.ToolResult) {
			r.Observations[0].Role = types.AnswerAggregateRolePrincipalAnswer
		}},
		{"other_requested_tid", func(ctx *types.AgentContext, _ *types.ToolResult) {
			ctx.AnalysisIR.RequestModel.RuntimeTargets[0].PID = 12
		}},
		{"other_requested_name", func(ctx *types.AgentContext, _ *types.ToolResult) {
			ctx.AnalysisIR.RequestModel.RuntimeTargets[0].Thread = "other-worker"
		}},
		{"narrower_requested_window", func(ctx *types.AgentContext, _ *types.ToolResult) {
			start, end := 1.15, 1.4
			ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeStart = &start
			ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &end
		}},
		{"outside_requested_window", func(ctx *types.AgentContext, _ *types.ToolResult) {
			start, end := 3., 4.
			ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeStart = &start
			ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &end
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := samplingSelectionAnalysisContext(t)
			params := json.RawMessage(`{"source":"attached_trace","view":"perf_sampling_tree","pid":11,"time_start":1,"time_end":2}`)
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
			if err != nil || !result.Success || len(result.Observations) != 1 {
				t.Fatalf("public native query: %v %+v", err, result)
			}
			tc.edit(ctx, &result)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if strings.Contains(prompt, "### 采样前缀树") {
				t.Fatal("unqualified or other-window aggregate borrowed requested display scope")
			}
		})
	}
	// Query without a target in a separate, unclassified tool context. The
	// finalizer must not relabel that actual global query as its named thread,
	// even though this small fixture happens to contain only that thread.
	ctx := samplingSelectionAnalysisContext(t)
	unscoped := &types.BusContext{RepoRoot: ctx.RepoRoot, WorkDir: ctx.WorkDir, Mutable: types.NewMutableState("query artifact")}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": ctx.AttachedHitraceSource, "view": "perf_sampling_tree", "time_start": 1, "time_end": 2})
	result, err := (&tool.TraceQuery{}).Execute(unscoped, params)
	if err != nil || !result.Success {
		t.Fatalf("public global query: %v %+v", err, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	if strings.Contains((&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil), "### 采样前缀树") {
		t.Fatal("global query was silently retargeted to a named thread")
	}
}
