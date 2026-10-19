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

// Reproduce the accepted HMC262 analyzer shape through public emit_analysis,
// then obtain addressable facts through public trace_query. The user need not
// add a source-code exclusion or know any answer ownership marker.
func runtimePathContextHMC263(t *testing.T, query bool) *types.AgentContext {
	t.Helper()
	const objective = "请看附件在1到2秒内document-worker线程（tid 11）的采样热点和调用栈，说明主要集中在哪里。"
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
		AttachedHitrace: string(data), AttachedHitraceSource: path, Mutable: types.NewMutableState(objective),
		TurnRouteHint: types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}}
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{})
	raw := json.RawMessage(`{
		"intent":"explain","scenario":"generic","complexity":"moderate","language":"zh",
		"keywords":["sampling"],"entities":["document-worker","11"],
		"question_kind":"mechanism","predicate_axis":"flow",
		"intent_confidence":0.9,"complexity_confidence":0.9,"kind_confidence":0.9,
		"predicates":{"is_scalar_answer":false,"is_role_locate_lookup":false,"is_count_question":false,"is_cross_component":false,"is_relational_lookup":false,"is_category_enumeration":false,"is_history_lookup":false,"is_diagnostic_question":false,"has_per_member_table":false},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.9},
		"answer_role_profile":{"is_role_binding_requested":false,"confidence":0.9},
		"error_granularity_profile":{"is_granularity_question":false,"confidence":0.9},
		"runtime_artifact_scope_profile":{"requested_scope":"explicit_time_window","time_start":1,"time_end":2,"source_quote":"1到2秒","confidence":0.9},
		"runtime_question_profile":{"scope":"bounded_fact_set","fact_families":["count_or_duration","other_observed_value"],"runtime_work_relation_requested":false,"frame_causality_requested":false,"confidence":0.9},
		"runtime_target_profile":{"declaration":"named_target","source_quote":"document-worker线程（tid 11）","confidence":0.95},
		"runtime_targets":[{"kind":"thread","pid":11,"thread":"document-worker","source":"user_explicit","confidence":0.95}],
		"history_selection_profile":{"mode":"not_applicable","item_kind":"not_applicable","confidence":0.9},
		"completeness_obligation":{"required":false,"source_quote":""},
		"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.9,"dimensions":[
			{"index":1,"label":"采样热点","role":"member_set","source_quote":"采样热点","required":true},
			{"index":2,"label":"调用栈","role":"relation_path","source_quote":"调用栈","required":true}
		]}
	}`)
	emitted, err := (&tool.EmitAnalysis{}).Execute(types.ToolBusContext(ctx, types.AgentAnalyzer), raw)
	if err != nil || !emitted.Success {
		t.Fatalf("public analyzer: %v %+v", err, emitted)
	}
	ctx.AnalysisIR, err = buildAnalysisIR(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ctx.AgentName, ctx.Stage = types.AgentFinalizer, types.StageFinalize
	if query {
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"source":"attached_trace","view":"perf_stats","pid":11,"time_start":1,"time_end":2}`))
		if err != nil || !result.Success {
			t.Fatalf("public query: %v %+v", err, result)
		}
		ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	}
	return ctx
}

func runtimePathDocumentHMC263() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "recorded-path", Kind: types.BlockSection, SurfaceRole: types.SurfacePrincipal,
		Text:      "Recorded stack frames retain their observed order; this does not prove a source call or execution interval.",
		FacetIDs:  []string{string(types.RequestedAnswerDimensionRelationPath), string(types.FacetObservedArtifactFact)},
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation, FacetID: string(types.FacetObservedArtifactFact)}},
	}}}
}

func TestRequestedRuntimePathPublicHMC263(t *testing.T) {
	ctx := runtimePathContextHMC263(t, true)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if view == nil || view.Family != types.QFGeneric {
		t.Fatalf("fixture must retain the real noncausal generic shape: %+v", view)
	}
	rm := &ctx.AnalysisIR.RequestModel
	if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		t.Fatal("fixture must not rely on a user source exclusion")
	}
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	if !answerDocRequestedRelationPathUsesRuntimeObservation(ctx) {
		t.Error("accepted bounded runtime path was promoted to current-source relation ownership")
	}
	dim := rm.RequestedAnswerDimensions.Dimensions[1]
	if !requestedDimensionCoveredByTypedDocumentShape(ctx, dim, runtimePathDocumentHMC263()) {
		t.Error("runtime path ownership requires fake source-call metadata")
	}
	for _, lang := range []string{"zh", "en"} {
		ctx.Language = lang
		for name, prompt := range map[string]string{
			"initial": renderAnswerDocRequestedAnswerDimensions(ctx),
			"repair":  requestedAnswerDimensionCoverageHint(ctx, []types.RequestedAnswerDimension{dim}, lang),
		} {
			if !strings.Contains(prompt, `facet_ids:["relation_path","observed_artifact_fact"]`) || !strings.Contains(prompt, `"claim_form":"external_observation"`) {
				t.Errorf("%s %s lost runtime ownership: %s", lang, name, prompt)
			}
			for _, bad := range []string{"这条源码关系路径", "principal source-relation block", "typed 唤醒/依赖方向", "typed wakeup/dependency direction"} {
				if strings.Contains(prompt, bad) {
					t.Errorf("%s %s forced unrelated semantics %q", lang, name, bad)
				}
			}
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("presentation-domain selection changed the original ledger")
	}
	if decided, causal := types.RuntimeTraceReportShapeAuthority(rm); !decided || causal {
		t.Fatal("runtime path presentation gained root-cause permission")
	}
}

func TestRequestedRuntimePathPreservesSourceAndEvidenceBoundariesHMC263(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query bool
		edit  func(*types.AgentContext)
	}{
		{name: "no native query"},
		{name: "independent source outcome", query: true, edit: func(ctx *types.AgentContext) {
			ctx.TurnRouteHint.RequiredOutcomes |= types.TurnOutcomeSourceExplanation
		}},
		{name: "explicit source explanation", query: true, edit: func(ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true, Modes: []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism}, SourceQuotes: []string{"explain the current implementation"}}
		}},
		{name: "precise source target", query: true, edit: func(ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{"worker.go:9"}
		}},
		{name: "source dimension binding", query: true, edit: func(ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{Path: "Makefile", Confidence: 1, RequestedDimensionIndices: []int{2}}}
		}},
		{name: "source location requested", query: true, edit: func(ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions.Dimensions = append(ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions.Dimensions, types.RequestedAnswerDimension{Index: 3, Role: types.RequestedAnswerDimensionSourceLocation, Label: "source file", Required: true})
		}},
		{name: "no accepted runtime question", query: true, edit: func(ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = nil
		}},
		{name: "model aggregate cannot borrow query producer", query: true, edit: func(ctx *types.AgentContext) {
			artifacts := ctx.Mutable.TurnAArtifacts()
			for i := range artifacts.ToolResults {
				for j := range artifacts.ToolResults[i].Observations {
					artifacts.ToolResults[i].Observations[j].ClaimAuthority = types.ObservationClaimAuthorityModelInference
				}
			}
			ctx.Mutable.SetTurnAArtifacts(*artifacts)
		}},
		{name: "model producer cannot supply native path", query: true, edit: func(ctx *types.AgentContext) {
			artifacts := ctx.Mutable.TurnAArtifacts()
			for i := range artifacts.ToolResults {
				for j := range artifacts.ToolResults[i].Observations {
					artifacts.ToolResults[i].Observations[j].Producer = "emit_answer_aggregate"
				}
			}
			ctx.Mutable.SetTurnAArtifacts(*artifacts)
		}},
		{name: "native query without addressable source", query: true, edit: func(ctx *types.AgentContext) {
			artifacts := ctx.Mutable.TurnAArtifacts()
			for i := range artifacts.ToolResults {
				for j := range artifacts.ToolResults[i].Observations {
					r := &artifacts.ToolResults[i].Observations[j]
					r.SourceRef = types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact}
					r.Span, r.SupportRefs = types.ObservationSpan{}, nil
				}
			}
			ctx.Mutable.SetTurnAArtifacts(*artifacts)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := runtimePathContextHMC263(t, tc.query)
			if tc.edit != nil {
				tc.edit(ctx)
			}
			if answerDocRequestedRelationPathUsesRuntimeObservation(ctx) {
				t.Fatal("runtime presentation waived independent source or missing runtime authority")
			}
			dim := ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions.Dimensions[1]
			if requestedDimensionCoveredByTypedDocumentShape(ctx, dim, runtimePathDocumentHMC263()) {
				t.Fatal("runtime carrier discharged a retained source relation dimension")
			}
		})
	}
}
