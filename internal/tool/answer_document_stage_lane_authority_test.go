package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDiagramVerifiedReadModeStagePrecedenceWiresPostValidation(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{
			{Identity: "analyzer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "explorer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "extractor", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "finalizer", Role: types.DiagramParticipantIncidentRequired},
		}},
	}
	ctx := &types.BusContext{RepoRoot: repoRoot, Mode: types.ModeRead, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	if got := diagramVerifiedReadModeStagePrecedence(ctx, view); len(got) != 3 {
		t.Fatalf("expected three checkout-verified adjacent relations, got %+v", got)
	}
	doc := func(toLabel string) *types.AnswerDocumentV2 {
		return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "read-lane", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid",
				Body: "flowchart TD\n  A[Analyzer] --> E[" + toLabel + "]\n"},
			EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "E", RelationKind: types.DiagramRelPrecedence}},
		}}}
	}
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, doc("Explorer"), view, nil); len(got) != 2 || !got[0].IsRequestedStagePrecedenceSpineIncomplete() {
		t.Fatalf("a required full workflow diagram with only one adjacent edge must report the missing principal spine: %+v", got)
	}
	preEmitFound := false
	for _, hint := range runPreEmitChecks(doc("Explorer"), view, nil, ctx) {
		if hint.Kind == types.ViolDiagramCallEdgeUnproven &&
			hint.HardSignal == preEmitHardSignalTypedCallEdgeEvidence &&
			hint.ExpectedShape != "" {
			preEmitFound = true
			break
		}
	}
	if !preEmitFound {
		t.Fatal("the pre-emit production chokepoint must reject a partial requested stage spine")
	}
	full := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "read-lane", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart TD\n  A[Analyzer] --> E[Explorer]\n  E --> X[Extractor]\n  X --> F[Finalizer]\n"},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "E", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "E", ToNode: "X", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "X", ToNode: "F", RelationKind: types.DiagramRelPrecedence},
		},
	}}}
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, full, view, nil); len(got) != 0 {
		t.Fatalf("one connected diagram with the complete requested spine must pass: %+v", got)
	}
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, doc("Extractor"), view, nil); len(got) == 0 {
		t.Fatal("non-adjacent stage edge must remain unproved")
	}

	ctx.Mode = types.ModeApply
	if got := diagramVerifiedReadModeStagePrecedence(ctx, view); len(got) != 0 {
		t.Fatalf("write mode must not borrow read-lane authority: %+v", got)
	}
	ctx.Mode = types.ModeRead
	ctx.AnalysisIR.RequestModel.DiagramHint.Participants = ctx.AnalysisIR.RequestModel.DiagramHint.Participants[:3]
	if got := diagramVerifiedReadModeStagePrecedence(ctx, view); len(got) != 0 {
		t.Fatalf("a partial typed stage slate must not activate checkout authority: %+v", got)
	}
	ctx.AnalysisIR.RequestModel = rm
	traceView := &types.AnswerSemanticView{Family: types.QFRootCauseTrace, RelationAxis: types.AxisFlow}
	if got := diagramVerifiedReadModeStagePrecedence(ctx, traceView); len(got) != 0 {
		t.Fatalf("Trace must retain its independent relation authority: %+v", got)
	}
}

func TestOptionalReadModeStageDiagramSharesPromptEdgeAuthorityWithoutCompletenessGate(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: false},
	}
	mu := types.NewMutableState("optional model-authored stage diagram")
	mu.AppendEvidence([]types.EvidenceItem{
		{Kind: types.EvidenceDirect, Source: types.ReadModePipelineStageBindingFile, LineStart: 142, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "ReadModeMainStageBindings", GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceDirect, Source: types.ReadModePipelineEnumsFile, LineStart: 34, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "StageAnalyze", Subject: "StageAnalyze", GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceDirect, Source: types.ReadModePipelineEnumsFile, LineStart: 35, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "StageExplore", Subject: "StageExplore", GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceDirect, Source: types.ReadModePipelineEnumsFile, LineStart: 131, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "AgentAnalyzer", Subject: "AgentAnalyzer", GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceDirect, Source: types.ReadModePipelineEnumsFile, LineStart: 132, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "AgentExplorer", Subject: "AgentExplorer", GroundingStatus: types.GroundingGrounded},
	})
	ctx := &types.BusContext{RepoRoot: repoRoot, Mode: types.ModeRead, AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: mu}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "read-lane", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid", Body: strings.Join([]string{
			"flowchart TD",
			`  StageAnalyze["analyze\nAgentAnalyzer"] --> StageExplore["explore\nAgentExplorer"]`,
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "StageAnalyze", ToNode: "StageExplore",
			FromIdentity: "StageAnalyze", ToIdentity: "StageExplore",
			RelationKind: types.DiagramRelPrecedence,
		}},
	}}}
	if got := diagramVerifiedReadModeStagePrecedence(ctx, view); len(got) != 0 {
		t.Fatalf("optional diagram must not activate requested completeness, got %+v", got)
	}
	if got := diagramVerifiedReadModeStageEdgeAuthority(ctx, view); len(got) != 1 {
		t.Fatalf("optional authored edge must see the same evidence-selected prompt recipe, got %+v", got)
	}
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, doc, view, mu.EmittedEvidence()); len(got) != 0 {
		t.Fatalf("one truthful optional stage edge must pass without forcing the full spine: %+v", got)
	}
	doc.Blocks[0].EdgeAnchors[0].FromIdentity = "StageExplore"
	doc.Blocks[0].EdgeAnchors[0].ToIdentity = "StageAnalyze"
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, doc, view, mu.EmittedEvidence()); len(got) == 0 {
		t.Fatal("reversed typed identities must remain rejected")
	}
}

func TestRequestedStagePrecedenceSpineRequiresOneConnectedDiagram(t *testing.T) {
	relations := diagramTestReadModePrecedence()
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	block := func(id, body string, anchors ...types.DiagramEdgeAnchor) types.AnswerBlock {
		return types.AnswerBlock{
			ID: id, Kind: types.BlockDiagram,
			Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: body},
			EdgeAnchors: anchors,
		}
	}

	supportOnly := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{block(
		"support", "flowchart TD\n  R[Orchestrator.Run] --> D[dispatchStage]\n",
		types.DiagramEdgeAnchor{FromNode: "R", ToNode: "D", RelationKind: types.DiagramRelCall},
	)}}
	got := DiagramRequestedStagePrecedenceSpineMismatches(supportOnly, view, relations)
	if len(got) != 3 {
		t.Fatalf("supporting implementation graph cannot replace the requested stage spine: %+v", got)
	}
	nodesOnly := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{block(
		"nodes", "flowchart TD\n  A[Analyzer]\n  E[Explorer]\n  X[Extractor]\n  F[Finalizer]\n",
	)}}
	if got := DiagramRequestedStagePrecedenceSpineMismatches(nodesOnly, view, relations); len(got) != 3 {
		t.Fatalf("a nodes-only required diagram must report the whole missing spine: %+v", got)
	}

	split := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		block("part-1", "flowchart TD\n  A[Analyzer] --> E[Explorer]\n",
			types.DiagramEdgeAnchor{FromNode: "A", ToNode: "E", RelationKind: types.DiagramRelPrecedence}),
		block("part-2", "flowchart TD\n  E[Explorer] --> X[Extractor]\n  X --> F[Finalizer]\n",
			types.DiagramEdgeAnchor{FromNode: "E", ToNode: "X", RelationKind: types.DiagramRelPrecedence},
			types.DiagramEdgeAnchor{FromNode: "X", ToNode: "F", RelationKind: types.DiagramRelPrecedence}),
	}}
	got = DiagramRequestedStagePrecedenceSpineMismatches(split, view, relations)
	if len(got) != 1 || got[0].BlockID != "part-2" {
		t.Fatalf("relations distributed across diagrams must fail against the best repair target: %+v", got)
	}

	disconnected := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{block(
		"disconnected", "flowchart TD\n  A[Analyzer] --> E1[Explorer]\n  E2[Explorer] --> X1[Extractor]\n  X2[Extractor] --> F[Finalizer]\n",
		types.DiagramEdgeAnchor{FromNode: "A", ToNode: "E1", RelationKind: types.DiagramRelPrecedence},
		types.DiagramEdgeAnchor{FromNode: "E2", ToNode: "X1", RelationKind: types.DiagramRelPrecedence},
		types.DiagramEdgeAnchor{FromNode: "X2", ToNode: "F", RelationKind: types.DiagramRelPrecedence},
	)}}
	got = DiagramRequestedStagePrecedenceSpineMismatches(disconnected, view, relations)
	if len(got) != 3 {
		t.Fatalf("individually valid but visibly disconnected relations must not count as one spine: %+v", got)
	}

	if got := DiagramRequestedStagePrecedenceSpineMismatches(supportOnly,
		&types.AnswerSemanticView{Family: types.QFRootCauseTrace, RelationAxis: types.AxisFlow}, relations); len(got) != 0 {
		t.Fatalf("Trace diagrams must remain outside read-mode stage spine authority: %+v", got)
	}
}

func TestRequestedStageSpineRepairReusesEquivalentTuplesInOneGeneration(t *testing.T) {
	relations := diagramTestReadModePrecedence()
	view := &types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "disconnected", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n  A[Analyzer] --> E1[Explorer]\n  E2[Explorer] --> X1[Extractor]\n  X2[Extractor] --> F[Finalizer]\n"},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "E1", FromIdentity: "analyzer", ToIdentity: "explorer", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "E2", ToNode: "X1", FromIdentity: "explorer", ToIdentity: "extractor", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "X2", ToNode: "F", FromIdentity: "extractor", ToIdentity: "finalizer", RelationKind: types.DiagramRelPrecedence},
		},
	}}}
	mismatches := DiagramRequestedStagePrecedenceSpineMismatches(doc, view, relations)
	if len(mismatches) != 3 {
		t.Fatalf("fixture must begin with one disconnected verified spine: %+v", mismatches)
	}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true},
	}}}
	raw := diagramRelationRepairDeltaJSON(doc, ctx, mismatches, nil, relations)
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(raw), &delta); err != nil {
		t.Fatalf("stage-spine repair delta must remain valid: %v raw=%s", err, raw)
	}
	lease := types.NewAnswerDiagramRelationRepairLease(doc, delta.Failures, delta.AllowedAdditions)
	if lease == nil || len(lease.Failures) != 3 || len(lease.AllowedAdditions) != 0 {
		t.Fatalf("existing equivalent tuples must become replacement targets, not duplicate additions: %+v", lease)
	}
	desired := map[string][2]string{
		"analyzer": {"A", "E1"}, "explorer": {"E1", "X1"}, "extractor": {"X1", "F"},
	}
	edits := make([]emitAnswerDiagramEdgeEdit, 0, len(lease.Failures))
	for _, failure := range lease.Failures {
		if !failure.AllowsAction("replace") {
			t.Fatalf("equivalent tuple carrier must expose exact replace: %+v", failure)
		}
		nodes, ok := desired[failure.FromIdentity]
		if !ok {
			t.Fatalf("unexpected stage tuple: %+v", failure)
		}
		edits = append(edits, emitAnswerDiagramEdgeEdit{
			FailureRef: failure.FailureRef, Action: "replace",
			Edge: &types.DiagramEdgeAnchor{FromNode: nodes[0], ToNode: nodes[1], VisibleLabel: "model-authored transition"},
		})
	}
	patch := &types.AnswerDocumentV2Patch{}
	if err := applyModelAuthoredDiagramAtomicEdits(doc, patch, edits, nil, lease, relations); err != nil {
		t.Fatalf("one atomic generation must reconnect the verified spine: %v", err)
	}
	merged, err := types.ApplyAnswerDocumentV2Patch(doc, patch)
	if err != nil {
		t.Fatalf("compiled stage patch must apply: %v", err)
	}
	if got := DiagramRequestedStagePrecedenceSpineMismatches(merged, view, relations); len(got) != 0 {
		t.Fatalf("same-generation repair left a disconnected spine: %+v\n%+v", got, merged.Blocks[0])
	}
}

func diagramTestReadModePrecedence() []stageauthority.PrecedenceRelation {
	rows := []stageauthority.StageRow{
		{StageIdent: "StageAnalyze", StageValue: "analyze", AgentIdent: "AgentAnalyzer", AgentValue: "analyzer"},
		{StageIdent: "StageExplore", StageValue: "explore", AgentIdent: "AgentExplorer", AgentValue: "explorer"},
		{StageIdent: "StageExtract", StageValue: "extract", AgentIdent: "AgentExtractor", AgentValue: "extractor"},
		{StageIdent: "StageFinalize", StageValue: "finalize", AgentIdent: "AgentFinalizer", AgentValue: "finalizer"},
	}
	return []stageauthority.PrecedenceRelation{
		{From: rows[0], To: rows[1]},
		{From: rows[1], To: rows[2]},
		{From: rows[2], To: rows[3]},
	}
}
