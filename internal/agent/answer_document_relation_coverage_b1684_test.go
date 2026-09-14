package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The graph deliberately contains configuration arguments and a shared-state
// initializer, not stage-output delivery. These are valid local relations;
// participant connectivity must not make them an artifact-transfer receipt.
func b1684PublicRelationFixture(t *testing.T, carrier, delivery bool) (*types.BusContext, *types.AgentContext) {
	t.Helper()
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	source := strings.Join([]string{
		"package fixture",
		"type Mutable struct{}",
		"type BusContext struct { Mutable *Mutable }",
		"type AgentContext struct { Mutable *Mutable }",
		"type Orchestrator struct { busCtx *BusContext }",
		"const AgentExtractor = \"extractor\"",
		"func BuildAgentContext(bus *BusContext, stage string) AgentContext {",
		" return AgentContext{",
		"  Mutable: bus.Mutable,",
		" }",
		"}",
		"func (o *Orchestrator) Preflight() {",
		" BuildAgentContext(o.busCtx, AgentExtractor)",
		"}",
		"func Produce() string { return \"result\" }",
		"func Consume(value string) {}",
		"func Deliver() {",
		" payload := Produce()",
		" Consume(payload)",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(repo, "flow.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	entries, err := repomap.ScanFiles(repo)
	if err != nil {
		t.Fatal(err)
	}
	graph := repomap.BuildGraph(repo, repomap.ParseFiles(entries, repo))
	if len(graph.SymbolDefs) == 0 {
		t.Fatal("actual parser produced no source symbols")
	}
	mut := types.NewMutableState("Explain the requested participants and their data flow")
	mut.SetSearchGraph(graph)
	participants := make([]types.DiagramParticipantHint, 0, 6)
	for _, name := range []string{"Analyzer", "Explorer", "Extractor", "Finalizer", "BusContext", "Mutable"} {
		participants = append(participants, types.DiagramParticipantHint{Identity: name, Role: types.DiagramParticipantIncidentRequired})
	}
	ir := &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain, PredicateAxis: types.AxisFlow,
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism), EntityProvenance: []types.EntityProvenance{{
			Surface: "Extractor", ResolvedAs: "AgentExtractor", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true,
		}}},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: participants},
	}}
	bus := &types.BusContext{RepoRoot: repo, WorkDir: t.TempDir(), Mutable: mut, AnalysisIR: ir}
	read, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(`{"path":"flow.go","line_offset":0,"limit":50}`))
	if err != nil || !read.Success || read.ReadCoverage == nil {
		t.Fatalf("actual source read: %v %+v", err, read)
	}
	bus.ToolResults = append(bus.ToolResults, read)
	mut.AppendDispatchToolResult(read)
	item := func(subject, predicate, object, anchor string, line int) map[string]any {
		return map[string]any{"scope": "line", "evidence_kind": "relationship", "subject": subject, "predicate": predicate, "object": object, "source": "flow.go", "line_start": line, "anchor_kind": anchor, "anchor_symbol": subject}
	}
	items := []map[string]any{
		item("AgentExtractor", "passes_to", "BuildAgentContext", "argument", 13),
		item("o.busCtx", "passes_to", "BuildAgentContext", "argument", 13),
	}
	if carrier {
		items = append(items, item("Mutable", "initialized_from", "bus.Mutable", "initializer", 9))
	}
	if delivery {
		items = append(items,
			item("payload", "assigned_from", "Produce", "assignment", 18),
			item("payload", "passes_to", "Consume", "argument", 19))
	}
	params, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		t.Fatal(err)
	}
	emitted, err := (&tool.EmitEvidence{}).Execute(bus, params)
	if err != nil || !emitted.Success || len(mut.EmittedEvidence()) != len(items) {
		t.Fatalf("actual evidence submission: %v %+v\n%+v", err, emitted, mut.EmittedEvidence())
	}
	for _, ev := range mut.EmittedEvidence() {
		if !ev.IsCitable() || ev.Producer != tool.EmitEvidenceProducer || types.EvidenceIsDerivationCandidate(ev) {
			t.Fatalf("fixture did not establish parser-grounded evidence: %+v", ev)
		}
	}
	bus.ToolResults = append(bus.ToolResults, emitted)
	mut.AppendDispatchToolResult(emitted)
	ctx := &types.AgentContext{Mode: types.ModeRead, Stage: types.StageFinalize, RepoRoot: repo, WorkDir: bus.WorkDir,
		Mutable: mut, AnalysisIR: ir, EvidenceItems: mut.EmittedEvidence()}
	return bus, ctx
}

func b1684Coverage(t *testing.T, ctx *types.AgentContext) (tool.FlowParticipantRelationCoverage, []stageauthority.PrecedenceRelation, string) {
	t.Helper()
	rm := ctx.AnalysisIR.RequestModel
	precedence := answerDocVerifiedReadModeStagePrecedenceForRequest(ctx)
	participants := rm.DiagramHint.Participants
	surfaces := make([][]string, len(participants))
	for i, participant := range participants {
		surfaces[i] = append([]string{participant.Identity}, types.DiagramParticipantIdentitySurfaces(rm, participant)...)
	}
	coverage := tool.ResolveFlowParticipantRelationCoverage(rm, participants, surfaces, ctx.EvidenceItems, precedence)
	candidates := tool.FlowParticipantTypedIncidentCandidateGuidance(rm, ctx.EvidenceItems, precedence, 8)
	return coverage, precedence, candidates
}

func b1684PromptPreservesAuthority(t *testing.T, ctx *types.AgentContext) string {
	t.Helper()
	eval := &answerDocumentEvaluator{}
	schemas := []llm.ToolSchema{{Name: "emit_answer_document"}, {Name: "emit_answer_document_patch"}, {Name: "read_file"}}
	permissions := eval.FilterToolSchemas(ctx, schemas)
	before, _ := json.Marshal(struct {
		IR       *types.AnalysisIR
		Evidence []types.EvidenceItem
		Emitted  []types.EvidenceItem
	}{ctx.AnalysisIR, ctx.EvidenceItems, ctx.Mutable.EmittedEvidence()})
	coverage, precedence, candidates := b1684Coverage(t, ctx)
	complete := ctx.Mutable.IsInvestigationComplete()
	prompt := eval.BuildInitialInstruction(ctx, nil)
	after, _ := json.Marshal(struct {
		IR       *types.AnalysisIR
		Evidence []types.EvidenceItem
		Emitted  []types.EvidenceItem
	}{ctx.AnalysisIR, ctx.EvidenceItems, ctx.Mutable.EmittedEvidence()})
	nextCoverage, nextPrecedence, nextCandidates := b1684Coverage(t, ctx)
	if string(before) != string(after) || !reflect.DeepEqual(coverage, nextCoverage) || !reflect.DeepEqual(precedence, nextPrecedence) || candidates != nextCandidates || complete != ctx.Mutable.IsInvestigationComplete() || !reflect.DeepEqual(permissions, eval.FilterToolSchemas(ctx, schemas)) {
		t.Fatal("prompt generation changed inputs, evidence, participant authority, candidates, completion, or tool permissions")
	}
	return prompt
}

func TestB1684PublicParticipantJoinDoesNotCertifyArtifactHandoff(t *testing.T) {
	_, ctx := b1684PublicRelationFixture(t, true, false)
	coverage, precedence, candidates := b1684Coverage(t, ctx)
	if len(precedence) != 3 || !coverage.RequestScopedRelationComplete || coverage.RequestScopedSubsetIncomplete {
		t.Fatalf("valid identity-based participant join was not established: precedence=%+v coverage=%+v evidence=%+v", precedence, coverage, ctx.EvidenceItems)
	}
	for _, want := range []string{`from_identity:"AgentExtractor"`, `from_identity:"o.busCtx"`, `to_identity:"BuildAgentContext"`, `from_identity:"bus.Mutable"`, `technical_endpoint_identity_stays_in_edge_anchor:true`} {
		if !strings.Contains(candidates, want) {
			t.Fatalf("actual candidate premise missing %q: %s", want, candidates)
		}
	}
	prompt := b1684PromptPreservesAuthority(t, ctx)
	for _, want := range []string{"proved_through_typed_participant_carrier_join", "cross_component_value_handoff_status=`unproven`", "do not emit requested_relation_scope=partial_unproven"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("bounded authority or existing scope contract disappeared: %q", want)
		}
	}
	// Assert only system-authored teaching, never scan model prose for a gate.
	for _, overclaim := range []string{"one complete requested-participant relation", "proves a bounded model-authorable join through the listed rows"} {
		if strings.Contains(prompt, overclaim) {
			t.Errorf("participant connectivity teaching overstates relation semantics: %q", overclaim)
		}
	}
	for _, scope := range []string{"participant-identity coverage", "does not certify complete artifact", "compact exact-endpoint projection"} {
		if !strings.Contains(prompt, scope) {
			t.Errorf("system teaching omitted the authority scope %q", scope)
		}
	}
	if strings.Contains(prompt, "Do not retarget the relation to an abstract component node, and do not delete") {
		t.Error("unqualified component-retarget prohibition contradicts candidate-authorized participant endpoint projection")
	}
}

func TestB1684PublicRealValueDeliveryRemainsAvailable(t *testing.T) {
	_, ctx := b1684PublicRelationFixture(t, true, true)
	coverage, _, _ := b1684Coverage(t, ctx)
	if !coverage.RequestScopedRelationComplete {
		t.Fatal("real delivery must not erase the legal participant join")
	}
	prompt := b1684PromptPreservesAuthority(t, ctx)
	for _, want := range []string{`from_identity":"Produce"`, `to_identity":"payload"`, `from_identity":"payload"`, `to_identity":"Consume"`} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual producer/value/consumer edge lost from authoring input: %q", want)
		}
	}
}

func TestB1684PublicDisconnectedParticipantRemainsPartial(t *testing.T) {
	_, ctx := b1684PublicRelationFixture(t, false, false)
	coverage, precedence, _ := b1684Coverage(t, ctx)
	if len(precedence) != 3 || coverage.RequestScopedRelationComplete || !coverage.RequestScopedSubsetIncomplete {
		t.Fatalf("missing carrier must remain unproved: %+v", coverage)
	}
	prompt := b1684PromptPreservesAuthority(t, ctx)
	if !strings.Contains(prompt, "requested_relation_spine_status=`unproven`") || !strings.Contains(prompt, "requested_relation_scope_recipe={requested_relation_scope:`partial_unproven`") || strings.Contains(prompt, "proved_through_typed_participant_carrier_join") {
		t.Fatal("bounded partial disclosure or no-invented-bridge contract changed")
	}
}

func TestB1684TraceDoesNotAcquireSourceParticipantJoin(t *testing.T) {
	_, ctx := b1684PublicRelationFixture(t, true, true)
	ctx.AnalysisIR.RequestModel.Intent = types.IntentRootCause
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Kind = ""
	ctx.AnalysisIR.RequestModel.Scenario = types.ScenarioRootCause
	start, end := 5.0, 5.01
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end,
		SourceQuote: "5.000 through 5.010 seconds", Confidence: 0.99,
	}
	if family := types.ResolveQuestionFamily(ctx.AnalysisIR.RequestModel); family != types.QFRootCauseTrace {
		t.Fatalf("Trace negative-control premise resolved to %s", family)
	}
	coverage, precedence, _ := b1684Coverage(t, ctx)
	if len(precedence) != 0 || coverage.RequestScopedRelationComplete {
		t.Fatalf("Trace acquired source workflow authority: %+v %+v", precedence, coverage)
	}
	prompt := b1684PromptPreservesAuthority(t, ctx)
	if strings.Contains(prompt, "proved_through_typed_participant_carrier_join") || strings.Contains(prompt, "## Current-Source Mechanism Relation Authority") {
		t.Fatal("source connectivity teaching entered the independent Trace lane")
	}
}
