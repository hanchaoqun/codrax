package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func samplingAggregatePreviewContextHMC245(t *testing.T, withInventory bool) *types.AgentContext {
	t.Helper()
	ctx := samplingSelectionAnalysisContext(t)
	// This fixture asks for aggregate hotspots, not an independent business
	// workflow or explanation. Use the public analyzer again rather than
	// relabeling the request's required dimensions after compilation.
	ctx.Objective = "请统计附件1到2秒文档工作线程（tid 11）的采样热点和权重占比。"
	perf := ctx.Mutable.PerfTrace()
	ctx.Mutable = types.NewMutableState(ctx.Objective)
	ctx.Mutable.SetPerfTrace(perf)
	ctx.AgentName, ctx.Stage = types.AgentAnalyzer, types.StageAnalyze
	analysis := json.RawMessage(`{
		"intent":"explain","scenario":"generic","complexity":"moderate","language":"zh",
		"keywords":["perf_sample"],"entities":["tid 11","document-worker"],
		"question_kind":"enumeration","predicate_axis":"call",
		"intent_confidence":0.9,"complexity_confidence":0.9,"kind_confidence":0.9,
		"predicates":{"is_scalar_answer":false,"is_role_locate_lookup":false,"is_count_question":false,"is_cross_component":false,"is_relational_lookup":false,"is_category_enumeration":true,"is_history_lookup":false,"is_diagnostic_question":false,"has_per_member_table":true},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.9},
		"answer_role_profile":{"is_role_binding_requested":false,"confidence":0.9},
		"error_granularity_profile":{"is_granularity_question":false,"confidence":0.9},
		"runtime_artifact_scope_profile":{"requested_scope":"explicit_time_window","time_start":1,"time_end":2,"source_quote":"1到2秒","confidence":0.9},
		"runtime_question_profile":{"scope":"bounded_fact_set","fact_families":["count_or_duration"],"runtime_work_relation_requested":false,"frame_causality_requested":false,"confidence":0.9},
		"runtime_target_profile":{"declaration":"named_target","source_quote":"tid 11","confidence":0.95},
		"runtime_targets":[{"kind":"thread","pid":11,"thread":"document-worker","source":"user_explicit","confidence":0.95}],
		"history_selection_profile":{"mode":"not_applicable","item_kind":"not_applicable","confidence":0.9},
		"completeness_obligation":{"required":false,"source_quote":""},
		"call_chain_endpoints":{"source":"","sink":"","sink_mode":"exact","runtime_selection_required":false,"runtime_selection_source_quote":""},
		"runtime_selection_profile":{"is_selection_question":false,"source_quote":"","confidence":0.9},
		"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.9,"dimensions":[
			{"index":1,"label":"采样热点","role":"member_set","source_quote":"采样热点","required":true},
			{"index":2,"label":"权重占比","role":"observed_value","source_quote":"权重占比","required":true}
		]}
	}`)
	emitted, err := (&tool.EmitAnalysis{}).Execute(types.ToolBusContext(ctx, types.AgentAnalyzer), analysis)
	if err != nil || !emitted.Success {
		t.Fatalf("aggregate analysis: %v %s", err, emitted.Summary)
	}
	ctx.AnalysisIR, err = buildAnalysisIR(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if dimensions := ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions; dimensions == nil || len(dimensions.Dimensions) != 2 {
		t.Fatalf("public aggregate dimensions lost: %+v", dimensions)
	}
	ctx.AgentName, ctx.Stage = types.AgentFinalizer, types.StageFinalize
	ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
	line := strings.Split(ctx.AttachedHitrace, "\n")[1]
	ctx.AttachedHitrace = strings.Replace(ctx.AttachedHitrace, line+"\n", strings.Repeat(line+"\n", 45), 1)
	var results []types.ToolResult
	if withInventory {
		r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"source":"attached_trace","view":"event_search","pid":11,"time_start":1,"time_end":2,"limit":100}`))
		if err != nil || !r.Success {
			t.Fatalf("inventory: %v %s", err, r.Summary)
		}
		results = append(results, r)
	}
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"source":"attached_trace","view":"perf_stats","pid":11,"time_start":1,"time_end":2}`))
	if err != nil || !r.Success {
		t.Fatalf("sampling: %v %s", err, r.Summary)
	}
	results = append(results, r)
	const synthesis = "CURRENT_SYNTHESIS_MUST_NOT_RESTATE_FULL_NATIVE_AGGREGATES_245"
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, InvestigationNotes: []string{synthesis, "INDEPENDENT_NOTE_RETAIN_245"}})
	ctx.Mutable.SetInvestigationComplete(synthesis)
	ctx.Mutable.SetInvestigationResultKind("resolved")
	return ctx
}

func TestHMC245NativeAggregateFactsIndependentOfAuditReferences(t *testing.T) {
	ctx := samplingAggregatePreviewContextHMC245(t, false)
	ledger := answerDocObservationLedger(ctx)
	refsOmitted := false
	for _, r := range ledger.Records {
		if p, ok := tool.DecodeTracePerfSamplingTree(r); ok {
			if p.OmittedCohorts != 0 || p.DisplayRoot != "" {
				t.Fatalf("not a complete aggregate: %+v", p)
			}
			for _, c := range p.Cohorts {
				if c.OmittedNodes != 0 {
					t.Fatal("aggregate nodes missing")
				}
				for _, n := range c.Nodes {
					if n.OmittedChildren != 0 {
						t.Fatal("aggregate children missing")
					}
					refsOmitted = refsOmitted || n.OmittedReferences > 0
				}
			}
		}
	}
	if !refsOmitted {
		t.Fatal("fixture failed to exercise reference budget")
	}
	before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"total_weight=475", "total_weight=7", "total_weight=100", "INDEPENDENT_NOTE_RETAIN_245", "omitted_references="} {
		if !strings.Contains(prompt, want) {
			t.Errorf("lost exact facts/disclosure %q", want)
		}
	}
	if strings.Contains(prompt, ctx.Mutable.StableInvestigationCompleteReason()) {
		t.Error("complete answer-grade aggregates are blocked by an audit-reference preview omission")
	}
	after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	if !bytes.Equal(before, after) {
		t.Fatal("presentation changed audit artifacts")
	}
	var publication answerDocNativePresentationReceipt
	renderAnswerDocPerfSamplingTrees(ctx, ledger, &publication)
	if publication.answerFactsCompleteFor(ctx) {
		t.Fatal("answer-grade tree cannot publish independent flat/callchain/table families")
	}
	renderAnswerDocPerfMemberFacts(ledger, &publication)
	if publication.answerFactsCompleteFor(ctx) {
		t.Fatal("tree and member envelopes cannot publish quality/fact tables")
	}
	renderAnswerDocRuntimeMeasurementChoices(ctx, &publication)
	if publication.completeFor(ctx) || !publication.answerFactsCompleteFor(ctx) {
		t.Fatal("audit reference completeness must remain separate from fully published answer facts")
	}
	if len(publication.eventMembers) != 0 {
		t.Fatal("aggregate receipt minted event member authority")
	}
}

func TestHMC245NativeAggregateDoesNotCoverEventPreview(t *testing.T) {
	ctx := samplingAggregatePreviewContextHMC245(t, true)
	ledger := answerDocObservationLedger(ctx)
	found := false
	for _, r := range ledger.Records {
		if r.EventSearchInventory == nil {
			continue
		}
		i := r.EventSearchInventory
		if !types.IsValidTraceEventSearchInventoryRecord(r) || len(i.Rows) != 40 || i.Coverage.MatchedTotal <= 40 || i.RowsComplete || i.HandoffRowsOmitted <= 0 {
			t.Fatalf("fixture must retain a complete raw event result but a partial inventory: %+v", i)
		}
		found = true
	}
	if !found {
		t.Fatal("event-search inventory missing")
	}
	before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, ctx.Mutable.StableInvestigationCompleteReason()) || !strings.Contains(prompt, "INDEPENDENT_NOTE_RETAIN_245") {
		t.Fatal("same source/window/target aggregate must not cover another query's incomplete event inventory")
	}
	after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	if !bytes.Equal(before, after) {
		t.Fatal("presentation changed original query records")
	}
}

func TestHMC245NativeAggregateAnswerObligations(t *testing.T) {
	for _, name := range []string{"occurrence_time", "other_value", "recorded_reason", "missing_family", "exhaustive", "unknown_dimension", "stage_workflow", "function_purpose", "comparison", "relation", "causal", "source"} {
		t.Run(name, func(t *testing.T) {
			ctx := samplingAggregatePreviewContextHMC245(t, false)
			rm := &ctx.AnalysisIR.RequestModel
			switch name {
			case "occurrence_time":
				rm.RuntimeQuestionProfile.FactFamilies = append(rm.RuntimeQuestionProfile.FactFamilies, types.RuntimeQuestionFactOccurrenceTime)
			case "other_value":
				rm.RuntimeQuestionProfile.FactFamilies = append(rm.RuntimeQuestionProfile.FactFamilies, types.RuntimeQuestionFactOtherObservedValue)
			case "recorded_reason":
				rm.RuntimeQuestionProfile.FactFamilies = append(rm.RuntimeQuestionProfile.FactFamilies, types.RuntimeQuestionFactRecordedReason)
			case "missing_family":
				rm.RuntimeQuestionProfile.FactFamilies = nil
			case "exhaustive":
				rm.CompletenessObligation = &types.CompletenessObligation{Required: true, SourceQuote: "all"}
			case "unknown_dimension":
				rm.RequestedAnswerDimensions.Dimensions = append(rm.RequestedAnswerDimensions.Dimensions, types.RequestedAnswerDimension{Role: types.RequestedAnswerDimensionOther, Required: true})
			case "stage_workflow", "function_purpose", "comparison":
				role := map[string]types.RequestedAnswerDimensionRole{"stage_workflow": types.RequestedAnswerDimensionStageWorkflow, "function_purpose": types.RequestedAnswerDimensionFunctionOrPurpose, "comparison": types.RequestedAnswerDimensionComparisonAxis}[name]
				rm.RequestedAnswerDimensions.Dimensions = append(rm.RequestedAnswerDimensions.Dimensions, types.RequestedAnswerDimension{Role: role, Required: true})
			case "relation":
				rm.RuntimeQuestionProfile.RuntimeWorkRelationRequested = true
			case "causal":
				rm.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
			case "source":
				rm.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true, Modes: []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism}, SourceQuotes: []string{"runtime.go"}, Confidence: 1}
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if !strings.Contains(prompt, ctx.Mutable.StableInvestigationCompleteReason()) || !strings.Contains(prompt, "INDEPENDENT_NOTE_RETAIN_245") {
				t.Fatal("reference-preview exception erased an uncovered answer obligation")
			}
		})
	}
}

func TestHMC245NativeAggregateStillRequiresWholeTree(t *testing.T) {
	for _, name := range []string{"depth", "limit", "root", "target", "window", "filtered"} {
		t.Run(name, func(t *testing.T) {
			ctx := samplingAggregatePreviewContextHMC245(t, false)
			args := map[string]any{"source": "attached_trace", "view": "perf_sampling_tree", "pid": 11, "time_start": 1, "time_end": 2}
			switch name {
			case "depth":
				args["sampling_depth"] = 1
			case "limit":
				args["limit"] = 1
			case "root":
				for _, r := range answerDocObservationLedger(ctx).Records {
					if p, ok := tool.DecodeTracePerfSamplingTree(r); ok && len(p.Cohorts) > 0 && len(p.Cohorts[0].Nodes) > 0 {
						args["sampling_root"] = p.Cohorts[0].Nodes[0].ID
						break
					}
				}
				if args["sampling_root"] == nil {
					t.Fatal("root fixture missing")
				}
			case "target":
				args["pid"] = 99
			case "window":
				end := 1.5
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &end
			case "filtered":
				args["view"], args["pattern"] = "perf_stats", "dso=/app/A.so"
			}
			raw, _ := json.Marshal(args)
			r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || !r.Success {
				t.Fatalf("query: %v %s", err, r.Summary)
			}
			artifacts := *ctx.Mutable.TurnAArtifacts()
			artifacts.ToolResults = []types.ToolResult{r}
			ctx.Mutable.SetTurnAArtifacts(artifacts)
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if !strings.Contains(prompt, ctx.Mutable.StableInvestigationCompleteReason()) {
				t.Fatal("partial, unavailable or unselected native tree replaced the current synthesis")
			}
		})
	}
}

func TestHMC245NativeAggregateReceiptsRemainPostBudget(t *testing.T) {
	for _, name := range []string{"query_budget", "byte_budget"} {
		t.Run(name, func(t *testing.T) {
			ctx := samplingAggregatePreviewContextHMC245(t, false)
			queries := 5
			if name == "byte_budget" {
				queries = 2
				// Complete native nodes with large but legal labels force the
				// combined 63KiB presentation budget. Each query remains below
				// the producer's independent 48KiB receipt bound.
				line := strings.Split(ctx.AttachedHitrace, "\n")[1]
				var trace strings.Builder
				trace.WriteString("# tracer: nop\n")
				for i := 0; i < 30; i++ {
					label := fmt.Sprintf("work%03d_%s", i, strings.Repeat("x", 500))
					row := strings.ReplaceAll(line, "symbol=work ", "symbol="+label+" ")
					row = strings.ReplaceAll(row, `\"symbol\":\"work\"`, `\"symbol\":\"`+label+`\"`)
					row = strings.ReplaceAll(row, "/app/A.so", "/app/"+strings.Repeat("d", 480)+".so")
					trace.WriteString(row)
					trace.WriteByte('\n')
				}
				ctx.AttachedHitrace = trace.String()
			}
			var results []types.ToolResult
			for i := 0; i < queries; i++ {
				params, _ := json.Marshal(map[string]any{"source": "attached_trace", "view": "perf_sampling_tree", "pid": 11, "time_start": 1 + float64(i)/1000, "time_end": 2, "limit": 256})
				r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
				if err != nil || !r.Success {
					t.Fatalf("query: %v %s", err, r.Summary)
				}
				results = append(results, r)
			}
			artifacts := *ctx.Mutable.TurnAArtifacts()
			artifacts.ToolResults = results
			ctx.Mutable.SetTurnAArtifacts(artifacts)
			ledger := answerDocObservationLedger(ctx)
			for _, r := range ledger.Records {
				if p, ok := tool.DecodeTracePerfSamplingTree(r); ok {
					if p.Status != "available" || p.OmittedCohorts != 0 {
						t.Fatalf("fixture has producer omissions: %+v", p)
					}
					for _, c := range p.Cohorts {
						if c.OmittedNodes != 0 {
							t.Fatalf("fixture hit producer node budget instead of presentation budget: nodes=%d retained=%d", c.NodeCount, len(c.Nodes))
						}
					}
				}
			}
			var publication answerDocNativePresentationReceipt
			text := renderAnswerDocPerfSamplingTrees(ctx, ledger, &publication)
			if !strings.Contains(text, "另省略采样树查询=1") || publication.answerFactsCompleteFor(ctx) {
				t.Fatalf("fixture must exercise actual display omission without an aggregate receipt: rendered_bytes=%d complete=%t text=%s", len(text), publication.answerFactsCompleteFor(ctx), text)
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if !strings.Contains(prompt, ctx.Mutable.StableInvestigationCompleteReason()) {
				t.Fatal("actual finalizer discarded synopsis despite omitted native aggregates")
			}
		})
	}
}
