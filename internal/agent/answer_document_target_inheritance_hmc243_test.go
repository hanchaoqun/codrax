package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Reproduce the live order: a first numeric cursor, followed by a statistical
// query omitting both target and window. The actual IR compiler owns the name.
func TestHMC243TargetInheritanceActualSamplingFinalizer(t *testing.T) {
	const question = "请看附件中1到2秒 document-worker-11（tid 11）的采样热点，按采样事件列出命中次数、权重和占比。"
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{Objective: question, RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentAnalyzer, Stage: types.StageAnalyze,
		AttachedHitrace: string(data), AttachedHitraceSource: path, Mutable: types.NewMutableState(question)}
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: path, Signals: []string{"perf_sample"}}})
	raw := json.RawMessage(`{
		"intent":"enumerate","scenario":"generic","complexity":"simple","language":"zh",
		"keywords":["sampling"],"entities":["document-worker-11","tid 11"],"question_kind":"mechanism","predicate_axis":"call",
		"intent_confidence":1,"complexity_confidence":1,"kind_confidence":1,
		"predicates":{"is_scalar_answer":false,"is_role_locate_lookup":false,"is_count_question":false,"is_cross_component":false,"is_relational_lookup":false,"is_category_enumeration":false,"is_history_lookup":false,"is_diagnostic_question":false,"has_per_member_table":false},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":1},
		"answer_role_profile":{"is_role_binding_requested":false,"confidence":1},"error_granularity_profile":{"is_granularity_question":false,"confidence":1},
		"runtime_artifact_scope_profile":{"requested_scope":"explicit_time_window","time_start":1,"time_end":2,"source_quote":"1到2秒","confidence":1},
		"runtime_question_profile":{"scope":"bounded_fact_set","runtime_work_relation_requested":false,"frame_causality_requested":false,"fact_families":["count_or_duration"],"confidence":1},
		"runtime_target_profile":{"declaration":"named_target","source_quote":"tid 11","confidence":1},
		"runtime_targets":[{"kind":"thread","pid":11,"thread":"document-worker-11","source":"user_explicit","confidence":1}],
		"runtime_selection_profile":{"is_selection_question":false,"source_quote":"","confidence":1},
		"completeness_obligation":{"required":false,"source_quote":""}
	}`)
	res, err := (&tool.EmitAnalysis{}).Execute(types.ToolBusContext(ctx, types.AgentAnalyzer), raw)
	if err != nil || !res.Success {
		t.Fatalf("actual analysis: %v %s", err, res.Summary)
	}
	ctx.AnalysisIR, err = buildAnalysisIR(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ctx.AgentName, ctx.Stage = types.AgentFinalizer, types.StageFinalize
	call := func(params map[string]any) types.ToolResult {
		t.Helper()
		params["source"], params["path"] = "attached_trace", path
		input, _ := json.Marshal(params)
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), input)
		if err != nil || !result.Success {
			t.Fatalf("native query: %v %s", err, result.Summary)
		}
		return result
	}
	first := call(map[string]any{"view": "event_search", "pid": 11, "pattern": "work", "time_start": 1, "time_end": 2})
	// The real IR compiler canonicalizes the accepted document-worker-11
	// spelling; it still carries a name whereas the numeric cursor does not.
	if targets := ctx.AnalysisIR.RequestModel.RuntimeTargets; len(targets) != 2 || targets[0].Thread != "document-worker" || targets[1].Thread != "" || !types.RuntimeTargetIsExplorationCursorSource(targets[1].Source) {
		t.Fatalf("did not reproduce user target plus nameless cursor: %+v", targets)
	}
	stats := call(map[string]any{"view": "perf_stats"})
	if !strings.Contains(stats.Summary, "trace_query_target_inherited=true") {
		t.Error("same TID cursor disabled request-target inheritance")
	}
	trees, facts := 0, 0
	for _, r := range stats.Observations {
		if r.SourceRef.QueryWindowKnown && (r.SourceRef.QueryWindowStartTs != 1 || r.SourceRef.QueryWindowEndTs != 2 || r.SourceRef.QueryTargetPID != 11 || r.SourceRef.QueryTargetScope != "thread") {
			t.Errorf("actual typed query lost request coordinates: %+v", r.SourceRef)
		}
		if r.Predicate == types.RuntimeSamplingMeasurementPredicate {
			facts++
		}
		if r.Predicate != tool.TracePerfSamplingTreePredicate {
			continue
		}
		trees++
		p, ok := tool.DecodeTracePerfSamplingTree(r)
		if !ok || p.TargetPID != 11 || p.TargetThread != "document-worker" || p.SampleCount != 5 || p.CohortCount != 3 {
			t.Errorf("typed tree scope/population: %+v ok=%t", p, ok)
		}
		text := tool.TracePerfSamplingTreeText(p)
		for _, want := range []string{"total_weight=35 stack_weight=30 unavailable_weight=5", "total_weight=7", "total_weight=100", `symbol="sys;root"`, `dso="/app/A.so" ip="0x2" inclusive=10`, `dso="/app/B.so" ip="0x3" inclusive=20`} {
			if !strings.Contains(text, want) {
				t.Errorf("tree missing %q", want)
			}
		}
	}
	if trees != 1 || facts == 0 {
		t.Errorf("native publications trees=%d sampling facts=%d", trees, facts)
	}
	const synthesis = "STALE_HMC243_CURRENT_SYNTHESIS_combines_cycles_32"
	const independent = "INDEPENDENT_HMC243_NOTE_preserve_unverified_sampling_configuration"
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{first, stats}, InvestigationNotes: []string{synthesis, independent}, AcceptedResultKind: "resolved", AcceptedClosureReason: synthesis})
	ctx.Mutable.SetInvestigationComplete(synthesis)
	ctx.Mutable.SetInvestigationResultKind("resolved")
	prompt := dependencyObservationMessages(t, ctx)
	if strings.Contains(prompt, synthesis) {
		t.Error("fully published native facts replayed the stale current synthesis")
		var publication answerDocNativePresentationReceipt
		_ = renderAnswerDocObservationLedgerWithPublication(ctx, &publication)
		for _, record := range answerDocObservationLedger(ctx).Records {
			if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact || !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
				continue
			}
			key := nativePresentationKey(record)
			if key == "" {
				key = publication.eventMirrorKey(record)
			}
			if key == "" || !publication.queries[key] {
				t.Logf("unpublished predicate=%q id=%q key_known=%t", record.Predicate, record.ID, key != "")
			}
		}
		t.Logf("publication_complete=%t stable_kind=%q", publication.completeFor(ctx), ctx.Mutable.StableInvestigationResultKind())
	}
	if !strings.Contains(prompt, independent) || ctx.Mutable.StableInvestigationCompleteReason() != synthesis {
		t.Error("native publication removed an independent note or mutated the audit lifecycle")
	}
	for _, want := range []string{"Native sampling fact choices", "15/35", "7/7", "100/100", "Sampling prefix tree:", `symbol="sys;root"`, "unavailable_weight=5"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual finalizer lost %q", want)
		}
	}
	if strings.Contains(prompt, "inclusive=900") {
		t.Error("window-excluded sample entered prompt")
	}
	if decided, allowed := types.RuntimeTraceReportShapeAuthority(&ctx.AnalysisIR.RequestModel); !decided || allowed {
		t.Error("target inheritance promoted bounded facts to causal authority")
	}
	for _, boundary := range []string{"source", "relation"} {
		t.Run(boundary, func(t *testing.T) {
			prior := ctx.AnalysisIR.RequestModel
			defer func() { ctx.AnalysisIR.RequestModel = prior }()
			if boundary == "source" {
				ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true, Modes: []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism}, SourceQuotes: []string{"source mechanism"}, Confidence: 1}
			} else {
				profile := *ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile
				profile.RuntimeWorkRelationRequested = true
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &profile
			}
			if !strings.Contains(dependencyObservationMessages(t, ctx), synthesis) {
				t.Error("independent source/relation duty lost its current synthesis")
			}
		})
	}
}
