package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1682PublicSourceFixture(t *testing.T, consumer bool) (*types.BusContext, *types.AgentContext, *explorerEvaluator, []types.ToolResult) {
	t.Helper()
	repo := t.TempDir()
	source := "package fixture\ntype State struct{}\nfunc Build(s State) State { return s }\nfunc Consume(s State) {}\nfunc Pipeline(s State) {\n value := Build(s)\n Consume(value)\n}\n"
	if err := os.WriteFile(filepath.Join(repo, "flow.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	entries, err := repomap.ScanFiles(repo)
	if err != nil {
		t.Fatal(err)
	}
	graph := repomap.BuildGraph(repo, repomap.ParseFiles(entries, repo))
	if len(graph.SymbolDefs) == 0 {
		t.Fatal("real parser produced no symbol definitions")
	}
	mut := types.NewMutableState("explain the requested relationships")
	mut.SetSearchGraph(graph)
	ir := &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain, PredicateAxis: types.AxisFlow,
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{
			{Identity: "Pipeline", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Build", Role: types.DiagramParticipantIncidentRequired},
		}},
	}, AnswerContract: types.AnswerContract{CitationReq: types.CitationReq{Required: false}}}
	if !consumer {
		ir.RequestModel.DiagramHint.Participants = append(ir.RequestModel.DiagramHint.Participants, types.DiagramParticipantHint{Identity: "Consume", Role: types.DiagramParticipantIncidentRequired})
	}
	bus := &types.BusContext{RepoRoot: repo, WorkDir: t.TempDir(), Mutable: mut, AnalysisIR: ir}
	read, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(`{"path":"flow.go","line_offset":0,"limit":30}`))
	if err != nil || !read.Success || read.ReadCoverage == nil {
		t.Fatalf("actual source read: %v %+v", err, read)
	}
	bus.ToolResults = []types.ToolResult{read}
	mut.AppendDispatchToolResult(read)
	items := []map[string]any{
		{"scope": "line", "evidence_kind": "mechanism", "subject": "fixture.Pipeline", "predicate": "calls", "object": "Build", "source": "flow.go", "line_start": 6, "anchor_kind": "call", "anchor_symbol": "Build"},
		{"scope": "line", "evidence_kind": "mechanism", "subject": "State", "predicate": "defined_in", "object": "flow.go", "source": "flow.go", "line_start": 2, "anchor_kind": "definition", "anchor_symbol": "State"},
		{"scope": "line", "evidence_kind": "mechanism", "subject": "Build", "predicate": "defined_in", "object": "flow.go", "source": "flow.go", "line_start": 3, "anchor_kind": "definition", "anchor_symbol": "Build"},
		{"scope": "line", "evidence_kind": "mechanism", "subject": "Consume", "predicate": "defined_in", "object": "flow.go", "source": "flow.go", "line_start": 4, "anchor_kind": "definition", "anchor_symbol": "Consume"},
	}
	items = append(items, map[string]any{"scope": "line", "evidence_kind": "relationship", "subject": "value", "predicate": "assigns", "object": "Build", "source": "flow.go", "line_start": 6, "anchor_kind": "assignment", "anchor_symbol": "value"})
	params, _ := json.Marshal(map[string]any{"items": items})
	emit, err := (&tool.EmitEvidence{}).Execute(bus, params)
	if err != nil || !emit.Success || len(mut.EmittedEvidence()) != len(items) {
		t.Fatalf("actual source evidence: %v %+v evidence=%+v", err, emit, mut.EmittedEvidence())
	}
	for _, ev := range mut.EmittedEvidence() {
		if !ev.IsCitable() || ev.Producer != tool.EmitEvidenceProducer || types.EvidenceIsDerivationCandidate(ev) {
			t.Fatalf("fixture lacks actual citable producer evidence: %+v", ev)
		}
	}
	mut.AppendDispatchToolResult(emit)
	reg := tool.NewRegistry()
	reg.Register(&tool.ReadFile{})
	reg.Register(&tool.EmitEvidence{})
	reg.Register(&tool.EmitInvestigationComplete{})
	ctx := &types.AgentContext{Stage: types.StageExplore, RepoRoot: repo, WorkDir: bus.WorkDir, Mutable: mut, AnalysisIR: ir}
	eval := &explorerEvaluator{phase: 1, mutable: mut, analysisIR: ir, repoRoot: repo, tools: reg, searchResult: &keywordSearchResult{Graph: graph}, heuristics: types.ExploreHeuristics{MidLoopMinIteration: 2}, midLoopPostPrimaryInjected: true, ermRequirements: []EvidenceRequirement{{Kind: types.ReqMechanism}}}
	return bus, ctx, eval, []types.ToolResult{read, emit}
}

func b1682Observe(t *testing.T, eval *explorerEvaluator, ctx *types.AgentContext, results []types.ToolResult, iteration int) LoopSignal {
	t.Helper()
	return eval.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, Iteration: iteration, AllToolResults: results, CurrentToolResults: results[len(results)-1:], LastToolResult: &results[len(results)-1]})
}

func b1682Completion(t *testing.T, bus *types.BusContext) types.ToolResult {
	t.Helper()
	result, err := (&tool.EmitInvestigationComplete{}).Execute(bus, json.RawMessage(`{"reason":"Only the currently grounded source operations are established; unconnected participants remain unproven.","confidence":"high","result_kind":"resolved"}`))
	if err != nil {
		t.Fatalf("public completion failed outside the intended contract: %v %+v", err, result)
	}
	return result
}

func b1682CheckSurveyHint(t *testing.T, sig LoopSignal) {
	t.Helper()
	if !sig.HintRequested || sig.HintKey != "explorer.mid-loop.completion-ready" || sig.StopRequested {
		t.Fatalf("expected original advisory timing, got %+v", sig)
	}
	for _, overclaim := range []string{"all current evidence requirements are satisfied", "answer-ready faces", "the current structured evidence appears close-ready"} {
		if strings.Contains(sig.Hint, overclaim) {
			t.Errorf("survey guidance overstates unverified completion contracts: %q\n%s", overclaim, sig.Hint)
		}
	}
	if !strings.Contains(sig.Hint, "emit_investigation_complete") {
		t.Error("survey guidance lost the structured completion path")
	}
}

func TestB1682PublicSurveyThenRelationRepairDoesNotClaimAllContractsReady(t *testing.T) {
	for _, consumer := range []bool{false, true} {
		name, wantCode := "participant", "flow_participant_operation_evidence"
		if consumer {
			name, wantCode = "consumer", "flow_value_consumer_evidence"
		}
		t.Run(name, func(t *testing.T) {
			bus, ctx, eval, results := b1682PublicSourceFixture(t, consumer)
			first := b1682Observe(t, eval, ctx, results, 2)
			b1682CheckSurveyHint(t, first)
			completion := b1682Completion(t, bus)
			if !completion.Success || completion.Repair == nil || completion.Repair.Code != wantCode || bus.Mutable.IsInvestigationComplete() {
				t.Fatalf("real completion did not produce expected typed downgrade %s: %+v", wantCode, completion)
			}
			results = append(results, completion)
			bus.Mutable.AppendDispatchToolResult(completion)
			_ = b1682Observe(t, eval, ctx, results, 3)
			read, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(`{"path":"flow.go","line_offset":0,"limit":30}`))
			if err != nil || !read.Success {
				t.Fatalf("actual bounded follow-up read: %v %+v", err, read)
			}
			results = append(results, read)
			schemas := []llm.ToolSchema{{Name: "read_file"}, {Name: "grep"}, {Name: "emit_evidence"}, {Name: "emit_investigation_complete"}}
			before := eval.FilterToolSchemas(ctx, schemas)
			downgrades := bus.Mutable.EvidenceClosure().Stats().PreCompleteDowngrades
			evidenceBefore, _ := json.Marshal(bus.Mutable.EmittedEvidence())
			post := b1682Observe(t, eval, ctx, results, 4)
			if !post.HintRequested || post.StopRequested {
				t.Fatalf("expected existing post-ready advisory, got %+v", post)
			}
			for _, overclaim := range []string{"marked the structured state as close-ready", "state was already marked close-ready", "call `emit_investigation_complete(reason, confidence, result_kind)` now"} {
				if strings.Contains(post.Hint, overclaim) {
					t.Errorf("typed %s repair was overridden by a stale all-ready claim: %s", wantCode, post.Hint)
				}
			}
			if !strings.Contains(strings.ToLower(post.Hint), "repair") {
				t.Errorf("post-downgrade guidance omitted the existing repair obligation: %s", post.Hint)
			}
			evidenceAfter, _ := json.Marshal(bus.Mutable.EmittedEvidence())
			if bus.Mutable.IsInvestigationComplete() || downgrades != bus.Mutable.EvidenceClosure().Stats().PreCompleteDowngrades || string(evidenceBefore) != string(evidenceAfter) || !reflect.DeepEqual(before, eval.FilterToolSchemas(ctx, schemas)) {
				t.Fatal("guidance changed completion, proof, downgrade accounting, or permissions")
			}
			// Repair the actual typed operations, without changing the old
			// completion result. A repair emission permits another attempt;
			// it is not itself an accepted completion receipt.
			additional := json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"mechanism","subject":"fixture.Pipeline","predicate":"calls","object":"Consume","source":"flow.go","line_start":7,"anchor_kind":"call","anchor_symbol":"Consume"},{"scope":"line","evidence_kind":"relationship","subject":"value","predicate":"passes_to","object":"Consume","source":"flow.go","line_start":7,"anchor_kind":"argument","anchor_symbol":"value"}]}`)
			emit, err := (&tool.EmitEvidence{}).Execute(bus, additional)
			if err != nil || !emit.Success {
				t.Fatalf("actual repair evidence: %v %+v", err, emit)
			}
			results = append(results, emit)
			if bus.Mutable.IsInvestigationComplete() {
				t.Fatal("evidence repair silently completed investigation")
			}
			retryHint := eval.scopeCollectionReadinessHint(LoopObservation{AllToolResults: results, LastToolResult: &emit}, "Collection progress.")
			if !strings.Contains(retryHint, "already supplied that repair evidence, retry for validation") {
				t.Errorf("actual supplied repair must permit validation retry, not declare permanent failure: %s", retryHint)
			}
			accepted := b1682Completion(t, bus)
			if !accepted.Success || accepted.Repair != nil || !bus.Mutable.IsInvestigationComplete() {
				t.Fatalf("actual complete relation and consumer could not close: %+v", accepted)
			}
			results = append(results, accepted)
			completedSignal := b1682Observe(t, eval, ctx, results, 6)
			if !completedSignal.StopRequested {
				t.Errorf("actual accepted completion must stop the existing investigation loop: %+v", completedSignal)
			}
			if strings.Contains(completedSignal.Hint, "The most recent completion attempt was not accepted") {
				t.Errorf("accepted result inherited stale failure instruction: %+v", completedSignal)
			}
		})
	}
}

func TestB1682LatestActualCompletionOwnsGuidance(t *testing.T) {
	for _, name := range []string{"latest-failed-invocation", "latest-failed-not-in-history", "prose-only", "unrelated-tool-summary", "actual-accepted"} {
		t.Run(name, func(t *testing.T) {
			bus, _, eval, _ := b1682PublicSourceFixture(t, true)
			obs := LoopObservation{}
			wantFailure := false
			switch name {
			case "prose-only":
				obs.Response = llm.Response{Content: "emit_investigation_complete DOWNGRADED: flow_value_consumer_evidence; the completion was not accepted"}
			case "unrelated-tool-summary":
				fake := types.ToolResult{ToolName: "read_file", Success: true, Summary: "emit_investigation_complete DOWNGRADED: flow_value_consumer_evidence"}
				obs.AllToolResults, obs.LastToolResult = []types.ToolResult{fake}, &fake
			default:
				downgraded := b1682Completion(t, bus)
				if !downgraded.Success || downgraded.Repair == nil || downgraded.Repair.Code != "flow_value_consumer_evidence" || bus.Mutable.IsInvestigationComplete() {
					t.Fatalf("actual prior downgrade premise: %+v", downgraded)
				}
				obs.AllToolResults = []types.ToolResult{downgraded}
				if name == "actual-accepted" {
					emit, err := (&tool.EmitEvidence{}).Execute(bus, json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":"value","predicate":"passes_to","object":"Consume","source":"flow.go","line_start":7,"anchor_kind":"argument","anchor_symbol":"value"}]}`))
					if err != nil || !emit.Success {
						t.Fatalf("actual consumer repair: %v %+v", err, emit)
					}
					accepted := b1682Completion(t, bus)
					if !accepted.Success || accepted.Repair != nil || !bus.Mutable.IsInvestigationComplete() {
						t.Fatalf("actual accepted result: %+v", accepted)
					}
					obs.AllToolResults = append(obs.AllToolResults, emit, accepted)
					obs.LastToolResult = &accepted
				} else {
					failed, err := (&tool.EmitInvestigationComplete{}).Execute(&types.BusContext{}, json.RawMessage(`{"reason":"attempted invocation","confidence":"high","result_kind":"resolved"}`))
					if err != nil || failed.Success || failed.Repair != nil {
						t.Fatalf("actual failed invocation premise: %v %+v", err, failed)
					}
					if name == "latest-failed-invocation" {
						obs.AllToolResults = append(obs.AllToolResults, failed)
					}
					obs.LastToolResult = &failed
					wantFailure = true
				}
			}
			before, _ := json.Marshal(obs)
			completeBefore := bus.Mutable.IsInvestigationComplete()
			got := eval.scopeCollectionReadinessHint(obs, "Collection progress.")
			if strings.Contains(got, "The most recent completion call failed.") != wantFailure || strings.Contains(got, "The most recent completion attempt was not accepted.") {
				t.Errorf("latest actual result or non-tool prose misclassified: %s", got)
			}
			if wantFailure && !strings.Contains(got, "reported invocation problem") {
				t.Errorf("latest failed invocation lost its own recovery guidance: %s", got)
			}
			after, _ := json.Marshal(obs)
			if string(before) != string(after) || completeBefore != bus.Mutable.IsInvestigationComplete() {
				t.Fatal("guidance mutated original results or completion state")
			}
		})
	}
}

func TestB1682ActualBoundedTraceKeepsWindowAndRuntimeOnlyCompletion(t *testing.T) {
	ctx := blobEscapeObservationOnlyContext(t, types.NewMutableState("bounded trace facts"))
	capture := filepath.Join(ctx.WorkDir, "capture.systrace")
	text := "# tracer: nop\n app-100 (100) [000] .... 5.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120\n app-100 (100) [000] .... 5.005000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n app-100 (100) [000] .... 5.020000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120\n"
	if err := os.WriteFile(capture, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	ctx.AttachedHitrace = capture
	start, end := 5.0, 5.01
	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisFlow
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "5.000 through 5.010 seconds", Confidence: 0.99}
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}}
	bus := types.ToolBusContext(ctx, types.AgentExplorer)
	params, _ := json.Marshal(map[string]any{"source": "path", "path": capture, "view": "event_search", "time_start": start, "time_end": end, "limit": 100})
	query, err := (&tool.TraceQuery{}).Execute(bus, params)
	if err != nil || !query.Success || query.RawRef == "" || len(query.Observations) == 0 {
		t.Fatalf("actual bounded trace query: %v %+v", err, query)
	}
	ctx.Mutable.AppendDispatchToolResult(query)
	if ctx.Mutable.TraceQueryRuntimeObservationCount() == 0 {
		t.Fatal("actual trace query result did not enter the runtime observation ledger")
	}
	readParams, _ := json.Marshal(map[string]any{"path": query.RawRef, "line_offset": 0, "limit": 15})
	read, err := (&tool.ReadFile{}).Execute(bus, readParams)
	if err != nil || !read.Success || read.RuntimeArtifactRead == nil || read.ReadCoverage != nil {
		t.Fatalf("actual runtime-only blob read: %v %+v", err, read)
	}
	results := []types.ToolResult{query, read}
	eval := &explorerEvaluator{phase: 1, mutable: ctx.Mutable, analysisIR: ctx.AnalysisIR, repoRoot: ctx.RepoRoot, searchResult: &keywordSearchResult{Graph: &repomap.Graph{}}, heuristics: types.ExploreHeuristics{MidLoopMinIteration: 2}}
	schemas := []llm.ToolSchema{{Name: "trace_query"}, {Name: "read_file"}, {Name: "emit_evidence"}, {Name: "emit_investigation_complete"}}
	before := eval.FilterToolSchemas(ctx, schemas)
	profileBefore, _ := json.Marshal(ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile)
	count := ctx.Mutable.TraceQueryRuntimeObservationCount()
	sig := b1682Observe(t, eval, ctx, results, 3)
	if sig.StopRequested || strings.Contains(sig.Hint, "flow_participant_operation_evidence") || strings.Contains(sig.Hint, "required source-flow") {
		t.Errorf("runtime facts acquired a source relation obligation: %+v", sig)
	}
	profileAfter, _ := json.Marshal(ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile)
	if string(profileBefore) != string(profileAfter) || count != ctx.Mutable.TraceQueryRuntimeObservationCount() || !reflect.DeepEqual(before, eval.FilterToolSchemas(ctx, schemas)) {
		t.Fatal("guidance changed trace window, observations or permissions")
	}
	complete, err := (&tool.EmitInvestigationComplete{}).Execute(bus, json.RawMessage(`{"reason":"Only the recorded scheduler events within the requested window are reported; no source-code cause is asserted.","confidence":"high","result_kind":"resolved"}`))
	if err != nil || !complete.Success || complete.Repair != nil || !ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("bounded runtime facts lost the existing completion lane: %v %+v", err, complete)
	}
}

func TestB1682OrdinaryReadStillHasStructuredCompletionPath(t *testing.T) {
	bus, ctx, eval, results := b1682PublicSourceFixture(t, false)
	bus.AnalysisIR.RequestModel.DiagramHint = nil
	bus.AnalysisIR.RequestModel.PredicateAxis = types.AxisUnknown
	sig := b1682Observe(t, eval, ctx, results, 2)
	if !sig.HintRequested || sig.HintKey != "explorer.mid-loop.completion-ready" || sig.StopRequested || !strings.Contains(sig.Hint, "emit_investigation_complete") {
		t.Fatalf("ordinary read lost existing advisory close path: %+v", sig)
	}
	completion := b1682Completion(t, bus)
	if !completion.Success || completion.Repair != nil || !bus.Mutable.IsInvestigationComplete() {
		t.Fatalf("ordinary grounded read acquired source-flow obligations: %+v", completion)
	}
}
