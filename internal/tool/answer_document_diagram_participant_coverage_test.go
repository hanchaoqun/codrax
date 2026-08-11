package tool

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func diagramParticipantCoverageFixture() (types.RequestModel, *types.AnswerSemanticView, *types.AnswerDocumentV2, []types.EvidenceItem) {
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{
			{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "MutableState", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantContextOnly},
		}},
	}
	view := &types.AnswerSemanticView{
		Family:                        types.QFGeneric,
		RelationAxis:                  types.AxisFlow,
		DiagramPlan:                   &types.DiagramFacetGraph{Kind: types.DiagramFlow, Required: true},
		DiagramParticipantObligations: append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...),
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n A[\"Analyzer\"] --> E[\"Explorer\"]\n M[\"MutableState\"]\n B[\"BusContext\"]"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "E", RelationKind: types.DiagramRelCall}},
	}}}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall("Analyzer", "Explorer")}
	return rm, view, doc, evidence
}

func TestDiagramParticipantCoverageRequiresTypedBoundaryWithoutInventingEdge(t *testing.T) {
	rm, view, doc, evidence := diagramParticipantCoverageFixture()
	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence)
	if len(got) != 1 || got[0].Participant != "MutableState" || got[0].Issue != DiagramParticipantCoverageMissingBoundary {
		t.Fatalf("uncovered participant mismatch=%+v, want MutableState missing boundary", got)
	}

	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
		Participant: "MutableState", Status: types.DiagramParticipantBoundaryUnproven,
	}}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence); len(got) != 0 {
		t.Fatalf("visible disconnected participant with typed boundary should pass: %+v", got)
	}
	if len(doc.Blocks[0].EdgeAnchors) != 1 {
		t.Fatalf("boundary validation must not create an edge: %+v", doc.Blocks[0].EdgeAnchors)
	}
}

func TestDiagramParticipantCoverageUsesTypedEndpointPairBehindBusinessLabels(t *testing.T) {
	rm, view, doc, evidence := diagramParticipantCoverageFixture()
	doc.Blocks[0].Diagram.Body = "flowchart LR\n A[\"理解请求\"] --> E[\"收集证据\"]\n M[\"MutableState\"]"
	doc.Blocks[0].EdgeAnchors[0].FromIdentity = "Analyzer"
	doc.Blocks[0].EdgeAnchors[0].ToIdentity = "Explorer"
	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence)
	if len(got) != 1 || got[0].Participant != "MutableState" || got[0].Issue != DiagramParticipantCoverageMissingBoundary {
		t.Fatalf("typed endpoint pair must cover business-labelled incident participants only: %+v", got)
	}
}

func TestDiagramParticipantCoverageUsesExactOperationOwnerForBusinessParticipant(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "MutableState", Role: types.DiagramParticipantIncidentRequired,
	}}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n A[\"准备分析\"] --> M[\"MutableState\"]"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "A", ToNode: "M", FromIdentity: "analyzerEvaluator.BuildInitialInstruction",
		ToIdentity: "ctx.MutableState.ResetPrescanSummary", RelationKind: types.DiagramRelCall,
	}}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall(
		"analyzerEvaluator.BuildInitialInstruction", "ctx.MutableState.ResetPrescanSummary",
	)}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence); len(got) != 0 {
		t.Fatalf("business participant should be incident through its exact typed receiver operation: %+v", got)
	}
	if doc.Blocks[0].EdgeAnchors[0].ToIdentity != "ctx.MutableState.ResetPrescanSummary" {
		t.Fatalf("participant coverage must not rewrite technical endpoint identity: %+v", doc.Blocks[0].EdgeAnchors[0])
	}
}

func TestDiagramParticipantCoverageUsesExactStaticBindingWithoutChangingEdge(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
	}}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n O[\"产生证据\"] --> B[\"BusContext\"]"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "O", ToNode: "B", FromIdentity: "output.EvidenceItems",
		ToIdentity: "o.busCtx.EvidenceItems", RelationKind: types.DiagramRelDataFlow,
	}}
	operation := types.EvidenceItem{
		ID: "bus-write", Kind: types.EvidenceRelationship, Subject: "o.busCtx.EvidenceItems",
		Predicate: "assigns", Object: "output.EvidenceItems", Source: "src/pipeline.go", LineStart: 20,
		AnchorKind: types.AnchorAssignment, AnchorSymbol: "o.busCtx.EvidenceItems", Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded, Snippet: "o.busCtx.EvidenceItems = output.EvidenceItems",
		OwnerIdentity: "Orchestrator.applyStageOutput",
		DeclaredIdentityBindings: []types.EvidenceDeclaredIdentityBinding{{
			Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
		}},
	}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, []types.EvidenceItem{operation}); len(got) != 0 {
		t.Fatalf("business participant should align through its exact typed binding: %+v", got)
	}
	anchor := doc.Blocks[0].EdgeAnchors[0]
	if anchor.FromIdentity != "output.EvidenceItems" || anchor.ToIdentity != "o.busCtx.EvidenceItems" || anchor.RelationKind != types.DiagramRelDataFlow {
		t.Fatalf("identity bridge must not rewrite or mint edge authority: %+v", anchor)
	}
}

func TestDiagramParticipantCoverageDoesNotLetHiddenTypedOperationReplaceBusinessParticipant(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
	}}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n O[\"output.EvidenceItems\"] --> F[\"ActiveAgent\"]"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "O", ToNode: "F", FromIdentity: "output.EvidenceItems",
		ToIdentity: "o.busCtx.EvidenceItems", RelationKind: types.DiagramRelDataFlow,
	}}
	operation := types.EvidenceItem{
		ID: "bus-write", Kind: types.EvidenceRelationship, Subject: "o.busCtx.EvidenceItems",
		Predicate: "assigns", Object: "output.EvidenceItems", Source: "src/pipeline.go", LineStart: 20,
		AnchorKind: types.AnchorAssignment, AnchorSymbol: "o.busCtx.EvidenceItems", Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded, Snippet: "o.busCtx.EvidenceItems = output.EvidenceItems",
		OwnerIdentity: "Orchestrator.applyStageOutput",
		DeclaredIdentityBindings: []types.EvidenceDeclaredIdentityBinding{{
			Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
		}},
	}
	got := DiagramParticipantCoverageMismatches(doc, view, rm, []types.EvidenceItem{operation})
	if len(got) != 1 || got[0].Issue != DiagramParticipantCoverageIdentityMissing {
		t.Fatalf("a hidden technical endpoint must not replace the requested BusContext participant: %+v", got)
	}

	doc.Blocks[0].Diagram.Body = "flowchart LR\n O[\"output.EvidenceItems\"] --> F[\"BusContext\"]"
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, []types.EvidenceItem{operation}); len(got) != 0 {
		t.Fatalf("a visible business label plus exact edge identities should pass without rewriting the edge: %+v", got)
	}
}

func TestPreCheckDiagramParticipantCoverageMapsBoundedTypedCandidatesPerParticipant(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
	}}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n B[\"BusContext\"]"
	doc.Blocks[0].EdgeAnchors = nil
	operation := types.EvidenceItem{
		ID: "bus-write", Kind: types.EvidenceRelationship, Subject: "o.busCtx.EvidenceItems",
		Predicate: "assigns", Object: "output.EvidenceItems", Source: "src/pipeline.go", LineStart: 20,
		AnchorKind: types.AnchorAssignment, AnchorSymbol: "o.busCtx.EvidenceItems", Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded, Snippet: "o.busCtx.EvidenceItems = output.EvidenceItems",
		OwnerIdentity: "Orchestrator.applyStageOutput",
		DeclaredIdentityBindings: []types.EvidenceDeclaredIdentityBinding{{
			Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
		}},
	}
	mut := types.NewMutableState("typed participant candidate map")
	mut.AppendEvidence([]types.EvidenceItem{operation})
	pctx := &preEmitCheckContext{ctx: &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: mut}}
	hints := preCheckDiagramParticipantCoverage(doc, view, pctx)
	if len(hints) != 1 {
		t.Fatalf("expected one participant candidate-map hint, got %+v", hints)
	}
	for _, want := range []string{
		"typed_candidate[BusContext][1]",
		`relation_kind:"data_flow"`,
		`from_identity:"output.EvidenceItems"`,
		`to_identity:"o.busCtx.EvidenceItems"`,
		"not a requirement to render every candidate",
		"does not create an edge",
	} {
		if !strings.Contains(hints[0].ExpectedShape, want) {
			t.Fatalf("candidate-map hint missing %q: %s", want, hints[0].ExpectedShape)
		}
	}
}

func TestDiagramParticipantCoverageRejectsUnprovenBoundaryWhenTypedOperationIsAvailable(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
	}}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n B[\"BusContext\"]"
	doc.Blocks[0].EdgeAnchors = nil
	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
		Participant: "BusContext", Status: types.DiagramParticipantBoundaryUnproven,
	}}
	operation := types.EvidenceItem{
		ID: "bus-write", Producer: types.EvidenceProducerExplorerEmitEvidence,
		Kind: types.EvidenceRelationship, Subject: "o.busCtx.EvidenceItems", Predicate: "assigns", Object: "output.EvidenceItems",
		Source: "src/pipeline.go", LineStart: 20, AnchorKind: types.AnchorAssignment,
		AnchorSymbol: "o.busCtx.EvidenceItems", Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded, Snippet: "o.busCtx.EvidenceItems = output.EvidenceItems",
		OwnerIdentity: "Orchestrator.applyStageOutput",
		DeclaredIdentityBindings: []types.EvidenceDeclaredIdentityBinding{{
			Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
		}},
	}
	got := DiagramParticipantCoverageMismatches(doc, view, rm, []types.EvidenceItem{operation})
	if len(got) != 1 || got[0].Issue != DiagramParticipantCoverageTypedEdgeMissing {
		t.Fatalf("available typed operation must require a model-authored visible incident edge, got %+v", got)
	}
}

func TestDiagramParticipantCoverageAcceptsProductionBareDisconnectedNodes(t *testing.T) {
	rm, view, doc, evidence := diagramParticipantCoverageFixture()
	doc.Blocks[0].Diagram.Body = "flowchart TD\n A[\"Analyzer\"] --> E[\"Explorer\"]\n MutableState"
	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
		Participant: "MutableState", Status: types.DiagramParticipantBoundaryUnproven,
	}}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence); len(got) != 0 {
		t.Fatalf("valid Mermaid bare node plus typed unproven boundary must pass: %+v", got)
	}
	if len(doc.Blocks[0].EdgeAnchors) != 1 {
		t.Fatalf("bare-node visibility must not mint relation metadata: %+v", doc.Blocks[0].EdgeAnchors)
	}
}

func TestDiagramParticipantCoverageRejectsStaleUnknownAndInvisibleBoundaries(t *testing.T) {
	rm, view, doc, evidence := diagramParticipantCoverageFixture()
	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{
		{Participant: "Analyzer", Status: types.DiagramParticipantBoundaryUnproven},
		{Participant: "BusContext", Status: types.DiagramParticipantBoundaryUnproven},
		{Participant: "NotInTypedSlate", Status: types.DiagramParticipantBoundaryUnproven},
		{Participant: "MutableState", Status: types.DiagramParticipantBoundaryUnproven},
	}
	doc.Blocks[0].Diagram.Body = "flowchart LR\n A[\"Analyzer\"] --> E[\"Explorer\"]\n B[\"BusContext\"]"
	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence)
	want := map[DiagramParticipantCoverageIssue]bool{
		DiagramParticipantCoverageStaleBoundary:   true,
		DiagramParticipantCoverageUnknownBoundary: true,
		DiagramParticipantCoverageNodeMissing:     true,
	}
	for _, mismatch := range got {
		delete(want, mismatch.Issue)
	}
	if len(want) != 0 {
		t.Fatalf("missing mismatch classes %v; got %+v", want, got)
	}
}

func TestPreCheckDiagramParticipantCoverageTeachesPrimaryTypedIdentity(t *testing.T) {
	rm, view, doc, evidence := diagramParticipantCoverageFixture()
	doc.Blocks[0].Diagram.Body = "flowchart LR\n A[\"Analyzer\"] --> E[\"Explorer\"]\n MS[\"Mutable\\n(MutableState)\"]"
	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
		Participant: "MutableState", Status: types.DiagramParticipantBoundaryUnproven,
	}}
	pctx := &preEmitCheckContext{ctx: &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}}}
	pctx.ctx.EvidenceItems = evidence
	hints := preCheckDiagramParticipantCoverage(doc, view, pctx)
	if len(hints) != 1 {
		t.Fatalf("expected one exact participant repair hint, got %+v", hints)
	}
	want := "make the exact typed identity the Mermaid node id or the first visible node label"
	if !strings.Contains(hints[0].ExpectedShape, want) {
		t.Fatalf("participant repair hint did not reduce primary-identity ambiguity; want %q in %q", want, hints[0].ExpectedShape)
	}
}

func TestDiagramParticipantCoverageBoundaryMustBeUniqueAndVisibleInItsOwnBlock(t *testing.T) {
	rm, view, doc, evidence := diagramParticipantCoverageFixture()
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID: "second", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n X[\"Other\"]"},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{{
			Participant: "MutableState", Status: types.DiagramParticipantBoundaryUnproven,
		}},
	})
	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence)
	if len(got) != 1 || got[0].BlockID != "second" || got[0].Issue != DiagramParticipantCoverageNodeMissing {
		t.Fatalf("a boundary cannot borrow visibility from another diagram block: %+v", got)
	}

	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
		Participant: "MutableState", Status: types.DiagramParticipantBoundaryUnproven,
	}}
	doc.Blocks[1].Diagram.Body = "flowchart LR\n M[\"MutableState\"]"
	got = DiagramParticipantCoverageMismatches(doc, view, rm, evidence)
	if len(got) != 1 || got[0].Issue != DiagramParticipantCoverageDuplicate {
		t.Fatalf("one participant may carry exactly one unproven boundary across the document: %+v", got)
	}
}

func TestDiagramParticipantCoverageDoesNotEnterTrace(t *testing.T) {
	rm, view, doc, evidence := diagramParticipantCoverageFixture()
	rm.Intent = types.IntentTrace
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence); len(got) != 0 {
		t.Fatalf("Trace diagrams keep independent causal authority: %+v", got)
	}
}

func TestDiagramParticipantCoverageUsesUniqueTypedDisplayAliasWithoutMintingRelation(t *testing.T) {
	rm, view, doc, evidence := diagramParticipantCoverageFixture()
	rm.AnalyzerHints.Entities = []string{"Analyzer", "Explorer", "MutableState"}
	rm.DiagramHint.Participants[2].Identity = "MutableState (state carrier)"
	view.DiagramParticipantObligations[2].Identity = "MutableState (state carrier)"

	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence)
	if len(got) != 1 || got[0].Participant != "MutableState (state carrier)" || got[0].Issue != DiagramParticipantCoverageMissingBoundary {
		t.Fatalf("decorated typed participant must resolve to its visible node but cannot acquire a relation: %+v", got)
	}
	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
		Participant: "MutableState", Status: types.DiagramParticipantBoundaryUnproven,
	}}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence); len(got) != 0 {
		t.Fatalf("unique typed display alias should bind the non-authoritative boundary: %+v", got)
	}
}

func TestDiagramParticipantCoverageAcceptsExactTypedDisplayIdentitiesWithSpaces(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true, Participants: []types.DiagramParticipantHint{
			{Identity: "Analyzer Agent", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Finalizer Agent", Role: types.DiagramParticipantIncidentRequired},
		}},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFGeneric, RelationAxis: types.AxisFlow,
		DiagramPlan:                   &types.DiagramFacetGraph{Kind: types.DiagramSequence, Required: true},
		DiagramParticipantObligations: append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...),
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n participant Analyzer_Agent as Analyzer Agent\n participant Finalizer_Agent as Finalizer Agent"},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{
			{Participant: "Analyzer Agent", Status: types.DiagramParticipantBoundaryUnproven},
			{Participant: "Finalizer Agent", Status: types.DiagramParticipantBoundaryUnproven},
		},
	}}}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, nil); len(got) != 0 {
		t.Fatalf("exact schema-valid display identities must not be both unknown and missing: %+v", got)
	}
}

func TestDiagramParticipantCoverageExactQualifiedBoundaryWinsOverTailAlias(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{
			{Identity: "Foo", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "pkg.Foo", Role: types.DiagramParticipantIncidentRequired},
		}},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFGeneric, RelationAxis: types.AxisFlow,
		DiagramPlan:                   &types.DiagramFacetGraph{Kind: types.DiagramFlow, Required: true},
		DiagramParticipantObligations: append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...),
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n Foo\n pkg.Foo"},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{
			{Participant: "Foo", Status: types.DiagramParticipantBoundaryUnproven},
			{Participant: "pkg.Foo", Status: types.DiagramParticipantBoundaryUnproven},
		},
	}}}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, nil); len(got) != 0 {
		t.Fatalf("exact typed identities must outrank otherwise ambiguous tail aliases: %+v", got)
	}
}

func TestDiagramParticipantCoverageConsumesVerifiedStageAliasesInBothValidationPasses(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{
			{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
		}},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFGeneric, RelationAxis: types.AxisFlow,
		DiagramPlan:                   &types.DiagramFacetGraph{Kind: types.DiagramFlow, Required: true},
		DiagramParticipantObligations: append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...),
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "stages", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n A[\"StageAnalyze\\n(analyzer)\"] --> E[\"StageExplore\\n(explorer)\"]\n E --> X[\"StageExtract\\n(extractor)\"]\n X --> F[\"StageFinalize\\n(finalizer)\"]"},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "E", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "E", ToNode: "X", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "X", ToNode: "F", RelationKind: types.DiagramRelPrecedence},
		},
	}}}
	ctx := &types.BusContext{RepoRoot: repoRoot, Mode: types.ModeRead, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	if got := DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, doc, view, nil); len(got) != 0 {
		t.Fatalf("checkout-verified stage aliases must satisfy participant coverage without false boundaries: %+v", got)
	}
	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
		Participant: "Analyzer", Status: types.DiagramParticipantBoundaryUnproven,
	}}
	got := DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, doc, view, nil)
	if len(got) != 1 || got[0].Issue != DiagramParticipantCoverageStaleBoundary {
		t.Fatalf("a boundary on an authority-covered stage must be rejected as stale: %+v", got)
	}
}
