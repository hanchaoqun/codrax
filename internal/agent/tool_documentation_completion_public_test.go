package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/analysis/hdp"
	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Only model choices are scripted. Tool execution, dispatch publication,
// completion acceptance, Turn A, extraction and answer emission are real.
// This is a three-agent public path, not an orchestrator.Run contract test.
type documentationCompletionPublicLLM struct {
	traceTeachingCaptureLLM
	actions []llm.ToolCall
	text    string
	prompts []string
}

func (l *documentationCompletionPublicLLM) Chat(_ context.Context, messages []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	var prompt strings.Builder
	for _, message := range messages {
		prompt.WriteString(message.Content)
		prompt.WriteByte('\n')
	}
	l.prompts = append(l.prompts, prompt.String())
	i := l.calls
	l.calls++
	if i < len(l.actions) {
		return llm.Response{ToolCalls: []llm.ToolCall{l.actions[i]}, StopReason: "tool_use"}, nil
	}
	if i == len(l.actions) && l.text != "" {
		return llm.Response{Content: l.text, StopReason: "end_turn"}, nil
	}
	return llm.Response{}, fmt.Errorf("script exhausted after %d real adapter calls", l.calls)
}

func documentationCompletionPublicBus(t *testing.T, language string) *types.BusContext {
	t.Helper()
	rm := types.RequestModel{
		Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain,
		Complexity: types.ComplexitySimple, Language: language,
		ToolDocumentationRequest: &types.ToolDocumentationRequest{Scope: types.ToolDocumentationRequestOnly},
	}
	compiled := compiler.Compile(rm, budget.BudgetSignals{})
	ir := &types.AnalysisIR{RequestModel: rm, TaskGraph: compiled.TaskGraph,
		EvidencePlan: compiled.EvidencePlan, AnswerContract: compiled.AnswerContract, HypothesisSet: hdp.Plan(rm)}
	root := t.TempDir()
	mu := types.NewMutableState("Explain the documented Trace units and input limitations, without claiming any capture measurement.")
	mu.SetRequestModel(rm)
	return &types.BusContext{RepoRoot: root, WorkDir: root, Language: language, Mutable: mu, AnalysisIR: ir}
}

func documentationCompletionPublicCloseCall() llm.ToolCall {
	return llm.ToolCall{ID: "close", Name: "emit_investigation_complete", Params: json.RawMessage(`{"reason":"The selected documentation supplies static units and input limitations; no capture or source behavior was measured.","confidence":"high","result_kind":"resolved"}`)}
}

func documentationCompletionPublicRegistry() *toolpkg.Registry {
	registry := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(registry)
	registry.Register(&toolpkg.EmitInvestigationComplete{})
	registry.Register(&toolpkg.EmitAnswerDocument{})
	registry.Register(&toolpkg.EmitAnswerDocumentPatch{})
	registry.Register(&toolpkg.EmitAnswerSymbol{})
	registry.Register(&toolpkg.EmitHypothesisVerdict{})
	return registry
}

func TestToolDocumentationCompletionPublicThreeAgents(t *testing.T) {
	for _, language := range []string{"zh", "en"} {
		t.Run(language, func(t *testing.T) {
			bus := documentationCompletionPublicBus(t, language)
			registry := documentationCompletionPublicRegistry()
			explore := &documentationCompletionPublicLLM{actions: []llm.ToolCall{
				{ID: "catalog", Name: "trace_capabilities", Params: json.RawMessage(`{"view":"window_stats","detail":true}`)},
				documentationCompletionPublicCloseCall(),
			}}
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
			// Match cmd/root.go's unconditional structured-tool bootstrap.
			exploreSkill := traceTeachingSkill(t, "explore-skill")
			exploreSkill.ToolSuggestions = append(exploreSkill.ToolSuggestions, "emit_evidence", "emit_investigation_complete")
			out, err := NewExplorerAgent(&Dependencies{LLM: explore, Tools: registry, MaxIterations: 3}).Execute(ctx, exploreSkill)
			if err != nil || out == nil || out.Error != "" || explore.calls != 2 {
				t.Errorf("documentation investigation did not close directly: calls=%d err=%v results=%s", explore.calls, err, documentationCompletionPublicResults(bus.Mutable.DispatchToolResults()))
			}
			if !bus.Mutable.IsInvestigationComplete() || !bus.Mutable.HasAcceptedToolDocumentationCompletion(&bus.AnalysisIR.RequestModel) {
				t.Error("real current producer and accepted completion did not yield a documentation receipt")
			}
			ta := bus.Mutable.TurnAArtifacts()
			if ta == nil {
				t.Fatal("actual explorer failed to publish Turn A")
			}
			if !types.AcceptedToolDocumentationCompletion(ta) || InvestigationStructurallyEmpty(ta, nil) {
				t.Error("accepted documentation-only Turn A is incorrectly structurally empty")
			}
			var content string
			for _, result := range ta.ToolResults {
				if result.ToolName == "trace_capabilities" && result.Success && result.Handoff != nil && result.Handoff.Documentation != nil {
					content = string(result.Handoff.Documentation.Content)
				}
			}
			if content == "" || !json.Valid([]byte(content)) {
				t.Fatal("real producer did not publish complete typed JSON")
			}
			documentationCompletionPublicNoEvidence(t, bus, ta)
			extract := &documentationCompletionPublicLLM{text: "The accepted documentation is ready for a source-free static explanation."}
			extractCtx := ctxbuilder.BuildAgentContext(bus, types.AgentExtractor, types.StageExtract)
			extracted, err := NewExtractorAgent(&Dependencies{LLM: extract, Tools: registry, MaxIterations: 2}).Execute(extractCtx, traceTeachingSkill(t, "extract-skill"))
			if err != nil || extracted == nil || extracted.Error != "" || extract.calls != 1 {
				t.Errorf("accepted documentation incorrectly needs repository extraction: calls=%d out=%+v err=%v", extract.calls, extracted, err)
			}
			if len(extract.prompts) == 0 || strings.Count(extract.prompts[0], content) != 1 {
				t.Error("extractor did not receive the exact complete selected document once")
			}
			if extracted != nil && len(extracted.AnswerSymbols) != 0 {
				t.Error("documentation invented source answer symbols")
			}
			if report := renderExtractorStageReport(extractCtx); strings.Contains(report, "source_lane: current_source_or_mixed") ||
				!strings.Contains(report, "documented tool capabilities only") {
				t.Errorf("extraction report contradicts the accepted documentation domain: %s", report)
			}
			answer := "`window_stats` documents trace-time seconds and per-field measurement units. Static availability does not establish capture support, measured values, or a cause."
			if language == "zh" {
				answer = "`window_stats` 的查询时间使用秒，测量字段各有单位。静态能力不等于当前采集支持、实测值或因果结论。"
			}
			params, _ := json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "documentation", "kind": "summary", "text": answer}}})
			finalize := &documentationCompletionPublicLLM{actions: []llm.ToolCall{{ID: "answer", Name: "emit_answer_document", Params: params}}}
			finalCtx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
			final, err := NewFinalizerAgent(&Dependencies{LLM: finalize, Tools: registry, MaxIterations: 2}).Execute(finalCtx, traceTeachingSkill(t, "answer-document-skill"))
			if err != nil || final == nil || final.Error != "" || !strings.Contains(final.FinalAnswer, answer) {
				t.Errorf("real finalizer did not deliver the source-free explanation: err=%v results=%s", err, documentationCompletionPublicResults(bus.Mutable.DispatchToolResults()))
			}
			if len(finalize.prompts) == 0 || strings.Count(finalize.prompts[0], content) != 1 {
				t.Error("finalizer did not receive the exact complete selected document once")
			}
			doc := bus.Mutable.AnswerDocumentV2()
			if doc == nil || len(doc.Citations) != 0 || len(doc.Blocks) != 1 || doc.Blocks[0].Text != answer {
				t.Errorf("documentation must remain ordinary model-owned uncited text: %+v", doc)
			}
			if !bus.Mutable.HasAcceptedToolDocumentationCompletion(&bus.AnalysisIR.RequestModel) {
				t.Error("ordinary extract/finalize dispatch reset invalidated the accepted receipt")
			}
			documentationCompletionPublicNoEvidence(t, bus, bus.Mutable.TurnAArtifacts())
		})
	}
}

func TestToolDocumentationCompletionPublicDoesNotBorrowAuthority(t *testing.T) {
	for _, kind := range []string{"missing_call", "failed_call", "serialized_display_only", "mixed_source", "mixed_trace"} {
		t.Run(kind, func(t *testing.T) {
			bus := documentationCompletionPublicBus(t, "en")
			actions := []llm.ToolCall{}
			catalog := llm.ToolCall{ID: "catalog", Name: "trace_capabilities", Params: json.RawMessage(`{}`)}
			switch kind {
			case "failed_call":
				catalog.Params = json.RawMessage(`{"view":"nonexistent_view"}`)
				actions = append(actions, catalog)
			case "serialized_display_only":
				other := documentationCompletionPublicBus(t, "en")
				result, err := (&toolpkg.TraceCapabilities{}).Execute(other, json.RawMessage(`{}`))
				if err != nil || !result.Success {
					t.Fatalf("replay fixture producer failed: %v", err)
				}
				wire, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				var replay types.ToolResult
				if err := json.Unmarshal(wire, &replay); err != nil {
					t.Fatal(err)
				}
				bus.ToolResults = []types.ToolResult{replay}
				bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{replay}, HandoffCarriers: []types.ToolHandoffCarrier{*replay.Handoff}})
			case "mixed_source", "mixed_trace":
				rm := &bus.AnalysisIR.RequestModel
				if kind == "mixed_trace" {
					runtime := traceCapabilitiesDiscoveryBus(t, true)
					*rm = runtime.AnalysisIR.RequestModel
					bus.AttachedHitrace = runtime.AttachedHitrace
					start, end := 1.0, 1.02
					rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
						RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.0..1.02", Confidence: 1,
					}
				}
				rm.ToolDocumentationRequest = &types.ToolDocumentationRequest{Scope: types.ToolDocumentationRequestMixed, DimensionIndices: []int{1}}
				rm.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{
					{Index: 1, Label: "Documented units", SourceQuote: "Documented units", Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
				}}
				if kind == "mixed_source" {
					if err := os.WriteFile(filepath.Join(bus.RepoRoot, "implementation.go"), []byte("package sample\nfunc Implementation() {}\n"), 0600); err != nil {
						t.Fatal(err)
					}
					rm.RequestedAnswerDimensions.Dimensions = append(rm.RequestedAnswerDimensions.Dimensions, types.RequestedAnswerDimension{
						Index: 2, Label: "Implementation operation", SourceQuote: "Implementation operation", Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true,
					})
					rm.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{Path: "implementation.go", Confidence: 1, RequestedDimensionIndices: []int{2}}}
				}
				if err := types.ValidateToolDocumentationRequest(rm); err != nil {
					t.Fatalf("mixed fixture domain is invalid: %v", err)
				}
				compiled := compiler.Compile(*rm, budget.BudgetSignals{})
				bus.AnalysisIR.TaskGraph, bus.AnalysisIR.EvidencePlan, bus.AnalysisIR.AnswerContract = compiled.TaskGraph, compiled.EvidencePlan, compiled.AnswerContract
				bus.AnalysisIR.HypothesisSet = hdp.Plan(*rm)
				bus.Mutable.SetRequestModel(*rm)
				actions = append(actions, catalog)
			}
			actions = append(actions, documentationCompletionPublicCloseCall())
			script := &documentationCompletionPublicLLM{actions: actions}
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
			exploreSkill := traceTeachingSkill(t, "explore-skill")
			exploreSkill.ToolSuggestions = append(exploreSkill.ToolSuggestions, "emit_evidence", "emit_investigation_complete")
			_, _ = NewExplorerAgent(&Dependencies{LLM: script, Tools: documentationCompletionPublicRegistry(), MaxIterations: len(actions) + 1}).Execute(ctx, exploreSkill)
			if script.calls < len(actions) {
				t.Fatalf("fixture did not attempt completion: calls=%d", script.calls)
			}
			if bus.Mutable.IsInvestigationComplete() || bus.Mutable.HasAcceptedToolDocumentationCompletion(&bus.AnalysisIR.RequestModel) || types.AcceptedToolDocumentationCompletion(bus.Mutable.TurnAArtifacts()) {
				t.Errorf("%s borrowed documentation/source/runtime completion authority: %s", kind, documentationCompletionPublicResults(bus.Mutable.DispatchToolResults()))
			}
			documentationCompletionPublicNoEvidence(t, bus, bus.Mutable.TurnAArtifacts())
		})
	}
}

func TestToolDocumentationCompletionPublicFamilyKeepsPresentation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		intent   types.Intent
		scenario types.Scenario
	}{
		{"architecture", types.IntentExplain, types.ScenarioArchitectureExplain},
		{"configuration", types.IntentExplain, types.ScenarioConfigTrace},
		{"sequence", types.IntentTrace, types.ScenarioGeneric},
		{"enumeration", types.IntentEnumerate, types.ScenarioGeneric},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := documentationCompletionPublicBus(t, "en")
			rm := &bus.AnalysisIR.RequestModel
			rm.Intent, rm.Scenario = tc.intent, tc.scenario
			compiled := compiler.Compile(*rm, budget.BudgetSignals{})
			bus.AnalysisIR.AnswerContract = compiled.AnswerContract
			// The original display request must survive unchanged. With no
			// relation support the existing effective-diagram gate still drops
			// a hard diagram obligation; documentation must not mint that support.
			bus.AnalysisIR.AnswerContract.Diagram = &types.DiagramContract{Required: true, Minimum: 1, RequiredKind: types.DiagramFlow}
			bus.Mutable.SetRequestModel(*rm)
			before, _ := json.Marshal(bus.AnalysisIR)
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
			view := types.BuildAnswerSemanticViewForAgentContext(ctx)
			if view == nil || view.Family != types.QFGeneric || !view.Presentation.AllowsBlock(types.BlockTable) || view.Presentation.DiagramRequired {
				t.Fatalf("pure documentation inherited a source family, lost table support, or minted diagram support: %+v", view)
			}
			capture := &traceTeachingCaptureLLM{stop: errors.New("captured real documentation display contract")}
			_, err := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: documentationCompletionPublicRegistry(), MaxIterations: 1}).Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
			if !errors.Is(err, capture.stop) || capture.calls != 1 {
				t.Fatalf("did not reach the actual finalizer adapter: calls=%d err=%v", capture.calls, err)
			}
			after, _ := json.Marshal(bus.AnalysisIR)
			if string(before) != string(after) {
				t.Fatal("presentation assembly changed request/contract authority")
			}
			if bus.Mutable.IsInvestigationComplete() || bus.Mutable.HasAcceptedToolDocumentationCompletion(rm) {
				t.Fatal("family/presentation classification alone minted completion")
			}
		})
	}
}

func documentationCompletionPublicResults(results []types.ToolResult) string {
	var b strings.Builder
	for _, result := range results {
		fmt.Fprintf(&b, "%s success=%t", result.ToolName, result.Success)
		if !result.Success || result.ToolName == "emit_investigation_complete" {
			fmt.Fprintf(&b, " summary=%.1200s", result.Summary)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func documentationCompletionPublicNoEvidence(t *testing.T, bus *types.BusContext, ta *types.TurnAArtifacts) {
	t.Helper()
	if len(bus.EvidenceItems) != 0 || len(bus.Mutable.EmittedEvidence()) != 0 || ta == nil || len(ta.EvidenceItems) != 0 || len(ta.ReadFiles) != 0 || ta.TerminalEvidenceCount != 0 {
		t.Error("documentation was promoted to repository evidence or file reads")
	}
	if ta != nil && len(types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: ta.ToolResults}).Records) != 0 {
		t.Error("documentation was promoted to runtime observation authority")
	}
}

// Unlike the three-agent test's compiled fixture, this begins with a fresh
// analysis context and exercises the complete emit_analysis/buildAnalysisIR
// post-processing path before inspecting the resulting downstream obligations.
func TestToolDocumentationCompletionPublicActualAnalyzer(t *testing.T) {
	root := t.TempDir()
	bus := &types.BusContext{RepoRoot: root, WorkDir: root, Language: "en",
		Mutable: types.NewMutableState("Explain the documented Trace units and input limitations, without claiming any capture measurement.")}
	params := json.RawMessage(`{
		"intent":"explain","scenario":"architecture_explain","complexity":"simple",
		"keywords":["capabilities"],"entities":[],"question_kind":"define",
		"intent_confidence":0.7,"complexity_confidence":0.7,"kind_confidence":0.7,
		"predicates":{"is_scalar_answer":false,"is_role_locate_lookup":false,"is_count_question":false,
			"is_cross_component":false,"is_relational_lookup":false,"is_category_enumeration":false,
			"is_history_lookup":false,"is_diagnostic_question":false,"has_per_member_table":false},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.7},
		"answer_role_profile":{"is_role_binding_requested":false,"confidence":0.7},
		"error_granularity_profile":{"is_granularity_question":false,"confidence":0.7},
		"runtime_artifact_scope_profile":{"requested_scope":"not_applicable","confidence":0.7},
		"history_selection_profile":{"mode":"not_applicable","item_kind":"not_applicable","confidence":0.7},
		"completeness_obligation":{"required":false,"source_quote":""},"predicate_axis":"",
		"call_chain_endpoints":{"source":"","sink":"","sink_mode":"exact","runtime_selection_required":false,"runtime_selection_source_quote":""},
		"runtime_selection_profile":{"is_selection_question":false,"source_quote":"","confidence":0.7},
		"requested_answer_dimensions":{"is_dimensioned_answer":false,"confidence":0.7},
		"runtime_target_profile":{"declaration":"unspecified","confidence":0.7},
		"runtime_question_profile":{"scope":"unspecified","runtime_work_relation_requested":false,"frame_causality_requested":false,"confidence":0.7},
		"tool_documentation_request":{"scope":"only"}
	}`)
	registry := documentationCompletionPublicRegistry()
	registry.Register(&toolpkg.EmitAnalysis{})
	script := &documentationCompletionPublicLLM{actions: []llm.ToolCall{{ID: "classify", Name: "emit_analysis", Params: params}}}
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentAnalyzer, types.StageAnalyze)
	out, err := NewAnalyzerAgent(&Dependencies{LLM: script, Tools: registry, MaxIterations: 2}).Execute(ctx, traceTeachingSkill(t, "analysis-skill"))
	if err != nil || out == nil || out.Error != "" || out.AnalysisIR == nil || script.calls != 1 {
		t.Fatalf("actual classification did not complete: calls=%d err=%v out=%+v results=%s", script.calls, err, out, documentationCompletionPublicResults(bus.Mutable.DispatchToolResults()))
	}
	ir := out.AnalysisIR
	if !types.ToolDocumentationOnlyRequested(&ir.RequestModel) || types.ResolveQuestionFamily(ir.RequestModel) != types.QFGeneric {
		t.Fatalf("analyzer post-processing replaced the pure documentation domain: %+v", ir.RequestModel)
	}
	if ir.AnswerContract.CitationReq.Required || ir.AnswerContract.CitationReq.MinCitations != 0 || len(ir.EvidencePlan.RequiredFiles) != 0 {
		t.Fatalf("analyzer reintroduced source obligations: citation=%+v files=%v", ir.AnswerContract.CitationReq, ir.EvidencePlan.RequiredFiles)
	}
	readReady, finalReady := false, false
	for _, node := range ir.TaskGraph.Nodes {
		for _, c := range append(append([]types.Criterion(nil), node.EntryConditions...), node.SuccessCriteria...) {
			if c.Kind == types.CritCitationCountGE || c.Kind == types.CritEvidenceCount {
				t.Errorf("documentation node reacquired source evidence/citation floor: node=%s criterion=%+v", node.ID, c)
			}
			if c.Kind == types.CritToolDocumentationReady {
				readReady = readReady || node.Type == types.NodeEvidence
				finalReady = finalReady || node.Type == types.NodeFinalize
			}
		}
	}
	if !readReady || !finalReady {
		t.Errorf("analyzer lost current-document support criteria: read=%t final=%t", readReady, finalReady)
	}
	for _, c := range ir.AnswerContract.AcceptanceTests {
		if c.Kind == types.CritCitationCountGE || c.Kind == types.CritToolDocumentationReady {
			t.Errorf("answer text checker received an inapplicable source floor or runtime-only criterion: %+v", c)
		}
	}
	if bus.Mutable.ToolDocumentationReady() || bus.Mutable.IsInvestigationComplete() || len(bus.Mutable.EmittedEvidence()) != 0 {
		t.Fatal("successful classification itself granted documentation/evidence/completion authority")
	}
}
