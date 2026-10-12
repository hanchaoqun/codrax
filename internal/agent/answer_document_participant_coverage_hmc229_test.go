package agent

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC229ParticipantActualEmitParseOutputDoesNotSignIncompleteDiagram(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 1.065
	rm := types.RequestModel{Language: "en", Intent: types.IntentTrace, PerfTrace: &types.PerfBundle{},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit"}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1 to 1.065"},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true, RelationScopeQuote: "Main, Worker, and Storage", Participants: []types.DiagramParticipantHint{
			{Identity: "Main", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "Main"},
			{Identity: "Worker", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "Worker"},
			{Identity: "Storage", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "Storage"},
		}},
	}
	dir := t.TempDir()
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "en", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("Show Main, Worker, and Storage"), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}}
	ctx.Mutable.SetRequestModel(rm)
	ctx.AnalysisIR.AnswerContract.Diagram = &types.DiagramContract{Required: true, Minimum: 1, RequiredKind: types.DiagramSequence, PreferredKinds: []types.DiagramKind{types.DiagramSequence}}
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "wakeup_chain", "pid": 100, "time_start": start, "time_end": end, "min_duration_ms": 0.1})
	query, err := (&tool.TraceQuery{}).Execute(bus, args)
	if err != nil || !query.Success {
		t.Fatalf("actual query: %v %s", err, query.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{query}})
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	rows := tool.RuntimeDiagramRelations(ledger, &rm)
	if len(rows) != 2 {
		t.Fatalf("expected two actual wakeup tuples: %+v", rows)
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, TraceCausalClaimCaliber: types.TraceCausalClaimNoConclusion, Text: "The measured events do not by themselves attribute the response delay."},
		{ID: "facts", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{Text: "A wakeup is independently recorded.", CitationRef: -1}}},
		{ID: "sequence", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n participant M as Main\n participant W as Worker\n participant S as Storage\n W->>M: observed wakeup"}, EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "W", ToNode: "M", FromIdentity: rows[0].FromIdentity, ToIdentity: rows[0].ToIdentity, RelationKind: rows[0].Kind, VisibleLabel: "observed wakeup"}}},
	}}
	doc.Blocks[2].FacetIDs = []string{string(types.FacetDiagramSpine), string(types.FacetObservedArtifactFact)}
	raw, _ := json.Marshal(doc)
	result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || result.Success || result.Repair == nil || result.Repair.Metadata[types.ToolRepairMetaDiagramParticipantRepairDeltaJSON] == "" {
		t.Errorf("actual emit must reject incomplete declared coverage: %v %+v", err, result)
	}
	out, err := (&answerDocumentEvaluator{mu: ctx.Mutable, language: "en"}).ParseOutput(ctx, []llm.Message{{Role: "assistant", Content: "No completed repair."}}, nil, nil)
	if err != nil || out == nil || !out.AnswerDegraded || !out.SkipAnswerChecks || ctx.Mutable.AnswerDocumentV2() != nil {
		t.Fatalf("display recovery must not turn participant incompleteness into accepted coverage: %v %+v", err, out)
	}
}
