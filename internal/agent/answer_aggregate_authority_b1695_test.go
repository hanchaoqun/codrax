package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	promptcontext "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1695RelationRequest(lang string) types.RequestModel {
	return types.RequestModel{
		Language: lang, Intent: types.IntentEnumerate, PredicateAxis: types.AxisRegister,
		AnalyzerHints:          types.AnalyzerHints{Kind: string(types.ReqRegistration), PrimaryEntities: []string{"RegisterDefaults"}},
		Predicates:             types.SemanticPredicates{IsCategoryEnumeration: true, IsRelationalLookup: true},
		CompletenessObligation: &types.CompletenessObligation{Required: true},
		CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
			SourceQuotes:                        []string{"current checkout"}, Confidence: 1,
		},
	}
}

// The definition witness is produced by the actual source reader and emitter.
// The aggregate is a normalized, retained model-input fixture, not a fabricated
// ObservationRecord or a claim that the completion preflight accepted it.
func b1695DefinitionBus(t *testing.T, lang string) *types.BusContext {
	t.Helper()
	const source = "package sample\ntype Worker struct{}\nfunc (w *Worker) Name() string { return \"worker\" }\n"
	graph, repo := b1627ParsedGraph(t, map[string]string{"worker.go": source})
	bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Language: lang,
		Mutable:    types.NewMutableState("List the default registered worker names"),
		AnalysisIR: &types.AnalysisIR{RequestModel: b1695RelationRequest(lang)},
	}
	bus.Mutable.SetSearchGraph(graph)
	read, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(`{"path":"worker.go","line_offset":0,"limit":20}`))
	if err != nil || !read.Success || read.ReadCoverage == nil {
		t.Fatalf("actual source read failed: %v %+v", err, read)
	}
	bus.Mutable.AppendDispatchToolResult(read)
	emit, err := (&tool.EmitEvidence{}).Execute(bus, json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"direct","subject":"Worker.Name","predicate":"defined","source":"worker.go","line_start":3,"anchor_kind":"definition","anchor_symbol":"Name","summary":"The source defines the Name method."}]}`))
	if err != nil || !emit.Success {
		t.Fatalf("actual definition emission failed: %v %+v", err, emit)
	}
	bus.Mutable.AppendDispatchToolResult(emit)
	bus.EvidenceItems = bus.Mutable.EmittedEvidence()
	if len(bus.EvidenceItems) != 1 || bus.EvidenceItems[0].GroundingStatus != types.GroundingGrounded || bus.EvidenceItems[0].AnchorKind != types.AnchorDefinition {
		t.Fatalf("definition-only prerequisite not met: %+v", bus.EvidenceItems)
	}
	current, err := os.ReadFile(filepath.Join(repo, "worker.go"))
	if err != nil || string(current) != source {
		t.Fatal("read/emitter changed source")
	}
	return bus
}

func b1695RetainFacts(t *testing.T, bus *types.BusContext, facts []types.AnswerAggregateFact) {
	t.Helper()
	normalized, err := types.NormalizeAnswerAggregateFacts(facts)
	if err != nil {
		t.Fatalf("aggregate fixture failed real structural normalization: %v", err)
	}
	bus.Mutable.SetInvestigationAggregateFacts(normalized)
	bus.Mutable.RetainInvestigationAggregateFacts()
}

func b1695EnrichReadSource(t *testing.T, bus *types.BusContext) {
	t.Helper()
	ctx := promptcontext.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	explorer := &explorerEvaluator{}
	explorer.BuildInitialInstruction(ctx, nil)
	explorer.searchResult = &keywordSearchResult{Graph: bus.Mutable.SearchGraph().(*repomap.Graph)}
	out, err := explorer.ParseOutput(ctx, nil, bus.Mutable.DispatchToolResults(), nil)
	if err != nil || out == nil {
		t.Fatalf("actual Explorer handoff failed: %v %+v", err, out)
	}
	bus.EvidenceItems = out.EvidenceItems
}

func b1695LedgerPrompt(t *testing.T, bus *types.BusContext) (types.ObservationLedger, string) {
	t.Helper()
	before, _ := json.Marshal(struct {
		Facts    []types.AnswerAggregateFact
		Evidence []types.EvidenceItem
	}{bus.Mutable.StableInvestigationAggregateFacts(), bus.EvidenceItems})
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0))
	ctx := promptcontext.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	after, _ := json.Marshal(struct {
		Facts    []types.AnswerAggregateFact
		Evidence []types.EvidenceItem
	}{bus.Mutable.StableInvestigationAggregateFacts(), bus.EvidenceItems})
	if !bytes.Equal(before, after) {
		t.Fatal("ledger or finalizer changed model facts or source evidence")
	}
	return ledger, prompt
}

func b1695AggregateRecord(t *testing.T, ledger types.ObservationLedger) types.ObservationRecord {
	t.Helper()
	for _, record := range ledger.Records {
		if strings.HasPrefix(record.ID, "aggregate:0#") {
			return record
		}
	}
	t.Fatalf("aggregate observation was discarded: %+v", ledger.Records)
	return types.ObservationRecord{}
}

func TestB1695DefinitionDoesNotProveRelationAggregateInFinalizer(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, support := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/support=%t", lang, support), func(t *testing.T) {
				bus := b1695DefinitionBus(t, lang)
				fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet, Label: "default registered worker names", Value: "1",
					Role: types.AnswerAggregateRolePrincipalAnswer, Members: []string{"worker"},
					MemberNotes: []string{"Name returns worker; default registration membership is a separate model assertion."},
				}
				if support {
					fact.SupportRefs = []string{"worker.go:3"}
				}
				b1695RetainFacts(t, bus, []types.AnswerAggregateFact{fact})
				if types.AnswerAggregateFactAuthorizesPrincipalContract(bus.Mutable.StableInvestigationAggregateFacts()[0], &bus.AnalysisIR.RequestModel) {
					t.Fatal("fixture unexpectedly already has relation authority")
				}
				ledger, prompt := b1695LedgerPrompt(t, bus)
				record := b1695AggregateRecord(t, ledger)
				if record.ClaimAuthority == types.ObservationClaimAuthorityIndependentlyProven {
					t.Errorf("definition coordinate certified the complete relation aggregate: %+v", record)
				}
				if support && (record.Origin != types.AnswerEvidenceOriginCurrentSource || record.SourceRef.Path != "worker.go" || record.Span.LineStart != 3 || record.GroundingStatus != types.GroundingGrounded || record.Role != types.AnswerAggregateRolePrincipalAnswer) {
					t.Errorf("narrow claim qualification lost valid source/role/grounding metadata: %+v", record)
				}
				var ledgerLine string
				for _, line := range strings.Split(prompt, "\n") {
					if strings.HasPrefix(line, "- `"+record.ID+"`:") {
						ledgerLine = line
					}
				}
				if ledgerLine == "" || strings.Contains(ledgerLine, "claim_authority=`independently_proven`") {
					t.Errorf("actual finalizer ledger contradicts the relation contract: %s", ledgerLine)
				}
				for _, want := range []string{"principal_contract=`not_authorized`", "default registered worker names", "worker", "default registration membership is a separate model assertion"} {
					if !strings.Contains(prompt, want) {
						t.Errorf("advisory prompt lost exact model/source handoff %q", want)
					}
				}
			})
		}
	}
}

func TestB1695PreciseAggregateLanesRemainIndependent(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang+"/source_scalar", func(t *testing.T) {
			bus := b1695DefinitionBus(t, lang)
			bus.AnalysisIR.RequestModel.Intent = types.IntentExplain
			bus.AnalysisIR.RequestModel.PredicateAxis = types.AxisDefine
			bus.AnalysisIR.RequestModel.Predicates = types.SemanticPredicates{IsScalarAnswer: true}
			bus.AnalysisIR.RequestModel.AnalyzerHints = types.AnalyzerHints{Kind: string(types.ReqMechanism)}
			bus.AnalysisIR.RequestModel.CompletenessObligation = nil
			fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateScalar, Role: types.AnswerAggregateRolePrincipalAnswer,
				Label: "name method", Value: "Worker.Name", SupportRefs: []string{"worker.go:3"}}
			b1695RetainFacts(t, bus, []types.AnswerAggregateFact{fact})
			ledger, prompt := b1695LedgerPrompt(t, bus)
			record := b1695AggregateRecord(t, ledger)
			if record.ClaimAuthority != types.ObservationClaimAuthorityIndependentlyProven || !types.AnswerAggregateFactAuthorizesPrincipalContract(fact, &bus.AnalysisIR.RequestModel) || !strings.Contains(prompt, "Worker.Name") {
				t.Fatalf("valid simple source scalar was over-demoted: %+v", record)
			}
		})
		t.Run(lang+"/typed_relation", func(t *testing.T) {
			graph, repo := b1627ParsedGraph(t, map[string]string{
				"Handler.java": "interface Handler { String name(); }\n",
				"Worker.java":  "class Worker implements Handler {\n public String name() { return \"worker\"; }\n}\n",
			})
			rm := b1695RelationRequest(lang)
			rm.PredicateAxis = types.AxisImplement
			rm.AnalyzerHints = types.AnalyzerHints{PrimaryEntities: []string{"Handler"}}
			bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Language: lang, Mutable: types.NewMutableState("List Handler implementations"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
			bus.Mutable.SetSearchGraph(graph)
			read, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(`{"path":"Worker.java","line_offset":0,"limit":20}`))
			if err != nil || !read.Success || read.ReadCoverage == nil {
				t.Fatalf("registration source must be read before enrichment: %v %+v", err, read)
			}
			bus.Mutable.AppendDispatchToolResult(read)
			// The actual parser resolves the implements edge. No graph node,
			// relation candidate, or system provenance marker is hand-built here.
			fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet, Label: "Handler implementations", Value: "1",
				Role: types.AnswerAggregateRolePrincipalAnswer, Members: []string{"Worker"}, SupportRefs: []string{"Worker.java:1"}}
			b1695RetainFacts(t, bus, []types.AnswerAggregateFact{fact})
			bus.Mutable.SetInvestigationComplete("retained model member set awaits exact relation qualification")
			if !tool.RefreshExactTypedRelationPrincipalMemberSets(bus) {
				t.Fatalf("actual exact relation matcher did not authorize the producer's matching member: %+v", bus.EvidenceItems)
			}
			fact = bus.Mutable.StableInvestigationAggregateFacts()[0]
			if !types.AnswerAggregateFactHasTypedRelationPrincipalAuthority(fact) {
				t.Fatalf("system relation marker missing after exact matching: %+v", fact)
			}
			ledger, prompt := b1695LedgerPrompt(t, bus)
			if record := b1695AggregateRecord(t, ledger); record.ClaimAuthority != types.ObservationClaimAuthorityIndependentlyProven || !strings.Contains(prompt, "Worker") {
				t.Fatalf("actual typed relation lost its aggregate authority: %+v", record)
			}
		})
		t.Run(lang+"/source_inventory", func(t *testing.T) {
			bus := b1695DefinitionBus(t, lang)
			bus.AnalysisIR.RequestModel.PredicateAxis = types.AxisDefine
			bus.AnalysisIR.RequestModel.Predicates = types.SemanticPredicates{IsCategoryEnumeration: true}
			bus.AnalysisIR.RequestModel.AnalyzerHints = types.AnalyzerHints{Kind: string(types.ReqEnumeration)}
			bus.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = nil
			bus.AnalysisIR.RequestModel.CompletenessObligation = &types.CompletenessObligation{Required: true, SourceQuote: "List all methods"}
			bus.AnalysisIR.RequestModel.SourceInventoryProfile = &types.SourceInventoryProfile{IsSourceInventory: true, Confidence: 1, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleMethod}}
			result, err := (&repomap.RepoMapV2{}).Execute(bus, json.RawMessage(`{"path":".","view":"source_inventory","scopes":["."],"roles":["method"],"include_counts":true,"top_n":20}`))
			if err != nil || !result.Success {
				t.Fatalf("real source inventory producer failed: %v %+v", err, result)
			}
			bus.Mutable.AppendDispatchToolResult(result)
			observation := types.SourceInventoryObservationFromMutable(bus.Mutable)
			facts := types.ProjectSourceInventoryPrincipalRowSetAggregateFacts(nil, observation, bus.AnalysisIR.RequestModel)
			if len(facts) != 1 || facts[0].Provenance != types.SourceInventoryPrincipalRowSetAggregateProvenance {
				t.Fatalf("actual inventory projection prerequisite failed: facts=%+v observation=%+v", facts, observation)
			}
			b1695RetainFacts(t, bus, facts)
			ledger, prompt := b1695LedgerPrompt(t, bus)
			if record := b1695AggregateRecord(t, ledger); record.ClaimAuthority != types.ObservationClaimAuthorityIndependentlyProven || !strings.Contains(prompt, "Name") {
				t.Fatalf("actual source inventory lost its aggregate authority: %+v", record)
			}
		})
	}
}

func TestB1695RealReturnEvidenceSurvivesUnprovenRelationAggregate(t *testing.T) {
	const source = "package sample\nfunc Value() string {\n return \"worker\"\n}\n"
	graph, repo := b1627ParsedGraph(t, map[string]string{"value.go": source})
	bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Language: "en",
		Mutable:    types.NewMutableState("List registered members"),
		AnalysisIR: &types.AnalysisIR{RequestModel: b1695RelationRequest("en")}}
	bus.Mutable.SetSearchGraph(graph)
	read, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(`{"path":"value.go","line_offset":0,"limit":20}`))
	if err != nil || !read.Success || read.ReadCoverage == nil {
		t.Fatalf("return source read failed: %v %+v", err, read)
	}
	bus.Mutable.AppendDispatchToolResult(read)
	emit, err := (&tool.EmitEvidence{}).Execute(bus, json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"direct","subject":"Value","predicate":"defined","source":"value.go","line_start":2,"anchor_kind":"definition","anchor_symbol":"Value","summary":"Value is defined here."}]}`))
	if err != nil || !emit.Success {
		t.Fatalf("actual return emission failed: %v %+v", err, emit)
	}
	bus.Mutable.AppendDispatchToolResult(emit)
	b1695EnrichReadSource(t, bus)
	var returnID string
	for _, item := range bus.EvidenceItems {
		if item.AnchorKind == types.AnchorReturn && item.Predicate == "returns" && item.Object == `"worker"` && item.LineStart == 3 && item.IsCitable() && !types.EvidenceIsDerivationCandidate(item) {
			returnID = item.ID
		}
	}
	if returnID == "" {
		t.Fatalf("actual grounded return prerequisite missing: %+v", bus.EvidenceItems)
	}
	fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet, Label: "registered members", Value: "1",
		Role: types.AnswerAggregateRolePrincipalAnswer, Members: []string{"worker"}, SupportRefs: []string{"value.go:3"}}
	b1695RetainFacts(t, bus, []types.AnswerAggregateFact{fact})
	ledger, prompt := b1695LedgerPrompt(t, bus)
	if b1695AggregateRecord(t, ledger).ClaimAuthority != types.ObservationClaimAuthorityModelInference {
		t.Fatal("one return literal cannot prove the proposed registration membership")
	}
	for _, record := range ledger.Records {
		if record.ID == "evidence:"+returnID {
			if record.ClaimAuthority != types.ObservationClaimAuthorityIndependentlyProven || record.Object != `"worker"` || record.Span.LineStart != 3 || !strings.Contains(prompt, "Value") {
				t.Fatalf("the independently grounded source return was lost or demoted: %+v", record)
			}
			return
		}
	}
	t.Fatal("return source observation disappeared")
}

func TestB1695NativeTraceAuthorityIsSeparateFromModelAggregate(t *testing.T) {
	repo := t.TempDir()
	capture := filepath.Join(repo, "capture.systrace")
	const trace = "# tracer: nop\n app-100 (100) [000] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n app-100 (100) [000] .... 5.010000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120\n"
	if err := os.WriteFile(capture, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": capture, "view": "event_search", "time_start": 5.0, "time_end": 5.1, "limit": 100})
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: repo, WorkDir: repo, Mutable: types.NewMutableState("query")}, params)
	if err != nil || !result.Success || len(result.Observations) == 0 {
		t.Fatalf("actual TraceQuery failed: %v %+v", err, result)
	}
	queryOnly := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}})
	if !queryOnly.HasDeterministicRuntimeQueryObservation() {
		t.Fatal("actual query did not establish native observation authority")
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Language: lang, Mutable: types.NewMutableState("observed scheduling"),
				ToolResults: []types.ToolResult{result}, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: lang, Intent: types.IntentRootCause, Scenario: types.ScenarioPerformanceBottleneck}}}
			fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateScalar, Label: "model interpretation", Value: "999", Provenance: "trace_query", Role: types.AnswerAggregateRolePrincipalAnswer,
				Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: string(types.AnswerEvidenceOriginRuntimeArtifact)}}}
			b1695RetainFacts(t, bus, []types.AnswerAggregateFact{fact})
			if !types.AnswerAggregateFactAuthorizesPrincipalContract(fact, &bus.AnalysisIR.RequestModel) {
				t.Fatal("explicit runtime aggregate compatibility lane unexpectedly changed")
			}
			ledger, _ := b1695LedgerPrompt(t, bus)
			if record := b1695AggregateRecord(t, ledger); record.ClaimAuthority == types.ObservationClaimAuthorityIndependentlyProven {
				t.Fatalf("explicit producer spelling borrowed native proof: %+v", record)
			}
			if !ledger.HasDeterministicRuntimeQueryObservation() {
				t.Fatal("native observations disappeared")
			}
			for _, native := range queryOnly.Records {
				found := false
				for _, current := range ledger.Records {
					if current.ID == native.ID {
						a, _ := json.Marshal(native)
						b, _ := json.Marshal(current)
						if !bytes.Equal(a, b) {
							t.Fatalf("model aggregate changed native record %s", native.ID)
						}
						found = true
					}
				}
				if !found {
					t.Fatalf("native record %s disappeared", native.ID)
				}
			}
		})
	}
}
