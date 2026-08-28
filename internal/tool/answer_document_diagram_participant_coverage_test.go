package tool

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/stageauthority"
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

func TestDiagramParticipantCoverage_TypeRelationMemberSetDoesNotRequireCollectionBoundary(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisImplement,
		AnalyzerHints: types.AnalyzerHints{PrimaryEntities: []string{"LoopController"}},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: []types.DiagramParticipantHint{
			{Identity: "LoopController", Role: types.DiagramParticipantIncidentRequired},
		}},
	}
	view := &types.AnswerSemanticView{
		Family:       types.QFGeneric,
		RelationAxis: types.AxisImplement,
		DiagramPlan:  &types.DiagramFacetGraph{Kind: types.DiagramArchitecture, Required: true},
		DiagramParticipantObligations: append(
			[]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...,
		),
	}
	evidence := []types.EvidenceItem{{
		ID: "impl", Kind: types.EvidenceRelationship,
		Producer: types.EvidenceProducerRepoMapImplementerRelation,
		Subject:  "analyzerEvaluator", Predicate: "implements", Object: "LoopController",
		Source: "internal/agent/analyzer.go", LineStart: 49, Scope: types.ScopeLine,
		AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded,
	}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "types", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid",
			Body: "flowchart TD\n  A[\"analyzerEvaluator\"] -->|implements| L[\"LoopController\"]"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "L", FromIdentity: "analyzerEvaluator", ToIdentity: "LoopController",
			RelationKind: types.DiagramRelTypeRelation,
		}},
	}}}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence); len(got) != 0 {
		t.Fatalf("a concrete target incident to the exact typed member relation must not inherit an unresolved collection boundary: %+v", got)
	}
}

func TestPreCheckDiagramParticipantCoveragePreservesGroundedNoArrowGrouping(t *testing.T) {
	rm, view, doc, evidence := diagramParticipantCoverageFixture()
	doc.Blocks[0].Diagram.Body = "flowchart LR\n A[\"Analyzer\"] --> E[\"Explorer\"]\n subgraph B[\"BusContext\"]\n  M[\"MutableState\"]\n end"
	mut := types.NewMutableState("preserve no-arrow ownership grouping")
	mut.AppendEvidence(evidence)
	pctx := &preEmitCheckContext{ctx: &types.BusContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: rm},
		Mutable:    mut,
	}}
	bodyBefore := doc.Blocks[0].Diagram.Body
	hints := preCheckDiagramParticipantCoverage(doc, view, pctx)
	if len(hints) != 1 {
		t.Fatalf("expected one missing-boundary hint, got %+v", hints)
	}
	for _, want := range []string{
		`participant=MutableState issue=missing_unproven_boundary`,
		`identity_action:"ensure_exact_visible_participant_without_directed_incident_edge_and_preserve_any_existing_grounded_no_arrow_grouping"`,
		`boundary_action:"add_exactly_one_unproven_boundary"`,
	} {
		if !strings.Contains(hints[0].ExpectedShape, want) {
			t.Fatalf("no-arrow grouping repair hint missing %q:\n%s", want, hints[0].ExpectedShape)
		}
	}
	for _, forbidden := range []string{
		"add_exact_visible_disconnected_participant", "remove_no_arrow_grouping", "flatten_grouping",
		"JSON placement:", "participant_boundaries is block-level", "never nest it inside diagram",
	} {
		if strings.Contains(hints[0].ExpectedShape, forbidden) {
			t.Fatalf("typed participant repair must not emit unrelated or false guidance; found %q in %s", forbidden, hints[0].ExpectedShape)
		}
	}
	if !strings.HasPrefix(hints[0].ExpectedShape, "Typed participant coverage mismatch: participant=MutableState issue=missing_unproven_boundary") {
		t.Fatalf("repair must lead with the actual typed mismatch, got: %s", hints[0].ExpectedShape)
	}
	if doc.Blocks[0].Diagram.Body != bodyBefore {
		t.Fatalf("precheck guidance must not rewrite the model's existing grouping: %q", doc.Blocks[0].Diagram.Body)
	}
}

func TestDiagramParticipantCoverageUnprovenBoundaryMustBeActuallyDisconnected(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired,
	}}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n W[\"appendStageOutputEvidenceToMutable\"] --> M[\"Mutable\"]"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "W", ToNode: "M", FromIdentity: "appendStageOutputEvidenceToMutable",
		ToIdentity: "MutableState.AppendEvidence", RelationKind: types.DiagramRelCall,
	}}
	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
		Participant: "Mutable", Status: types.DiagramParticipantBoundaryUnproven,
	}}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall(
		"appendStageOutputEvidenceToMutable", "MutableState.AppendEvidence",
	)}
	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence)
	if len(got) != 1 || got[0].Issue != DiagramParticipantCoverageBoundaryConnected {
		t.Fatalf("an unproven participant must not visually impersonate a different typed endpoint: %+v", got)
	}

	doc.Blocks[0].Diagram.Body = "flowchart LR\n W[\"appendStageOutputEvidenceToMutable\"] --> OP[\"MutableState.AppendEvidence\"]\n Mutable[\"Mutable\"]"
	doc.Blocks[0].EdgeAnchors[0].ToNode = "OP"
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence); len(got) != 0 {
		t.Fatalf("the typed operation edge and exact disconnected business participant can coexist honestly: %+v", got)
	}

	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID: "second-view", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart LR\n W2[\"appendStageOutputEvidenceToMutable\"] --> M2[\"Mutable\"]"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "W2", ToNode: "M2", FromIdentity: "appendStageOutputEvidenceToMutable",
			ToIdentity: "MutableState.AppendEvidence", RelationKind: types.DiagramRelCall,
		}},
	})
	got = DiagramParticipantCoverageMismatches(doc, view, rm, evidence)
	if len(got) != 1 || got[0].Issue != DiagramParticipantCoverageBoundaryConnected {
		t.Fatalf("a document-level unproven boundary must not be contradicted by a sibling diagram: %+v", got)
	}
}

func TestPreCheckDiagramParticipantCoveragePublishesUniqueEndpointCollisionTuple(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired,
	}}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n W[\"appendStageOutputEvidenceToMutable\"] --> Mutable[\"Mutable\"]"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "W", ToNode: "Mutable", FromIdentity: "appendStageOutputEvidenceToMutable",
		ToIdentity: "MutableState.AppendEvidence", RelationKind: types.DiagramRelCall,
	}}
	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
		Participant: "Mutable", Status: types.DiagramParticipantBoundaryUnproven,
	}}
	evidence := diagramEvidenceTestCall("appendStageOutputEvidenceToMutable", "MutableState.AppendEvidence")
	mut := types.NewMutableState("unique endpoint collision")
	mut.AppendEvidence([]types.EvidenceItem{evidence})
	pctx := &preEmitCheckContext{ctx: &types.BusContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: mut,
	}}
	bodyBefore := doc.Blocks[0].Diagram.Body
	anchorBefore := doc.Blocks[0].EdgeAnchors[0]
	hints := preCheckDiagramParticipantCoverage(doc, view, pctx)
	if len(hints) != 1 {
		t.Fatalf("expected one endpoint-collision hint, got %+v", hints)
	}
	for _, want := range []string{
		`typed_endpoint_collision["Mutable"]`,
		`block_id:"flow"`,
		`body_edge:{from_node:"W",to_node:"Mutable"}`,
		`conflict_endpoint_side:"to"`,
		`conflict_visible_surface:"Mutable"`,
		`visible_label_collision:true`,
		`node_fields_to_change:"body.to_node+edge_anchor.to_node"`,
		`from_identity:"appendStageOutputEvidenceToMutable"`,
		`to_identity:"MutableState.AppendEvidence"`,
		`relation_kind:"call"`,
		"Choose one fresh non-participant Mermaid node ID",
		"relabel the same technical node with concise technical wording",
		"not creating or selecting a relation or choosing replacement wording",
		"not creating or selecting a relation",
	} {
		if !strings.Contains(hints[0].ExpectedShape, want) {
			t.Fatalf("unique endpoint-collision hint missing %q:\n%s", want, hints[0].ExpectedShape)
		}
	}
	for _, forbidden := range []string{"replacement_node_id:", "suggested_node_id:", "auto_created_edge:"} {
		if strings.Contains(hints[0].ExpectedShape, forbidden) {
			t.Fatalf("system must not choose the model's replacement node or edge; found %q in %s", forbidden, hints[0].ExpectedShape)
		}
	}
	if doc.Blocks[0].Diagram.Body != bodyBefore || doc.Blocks[0].EdgeAnchors[0] != anchorBefore {
		t.Fatalf("precheck guidance must not rewrite the model diagram: body=%q anchor=%+v", doc.Blocks[0].Diagram.Body, doc.Blocks[0].EdgeAnchors[0])
	}
}

func TestPreCheckDiagramParticipantCoveragePublishesCollisionForBusinessLabelOnShortNodeID(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired,
	}}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n W[\"append evidence\"] --> n6[\"Mutable\"]\n Mutable_boundary[\"Mutable\"]"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "W", ToNode: "n6", FromIdentity: "appendStageOutputEvidenceToMutable",
		ToIdentity: "MutableState.AppendEvidence", RelationKind: types.DiagramRelCall,
	}}
	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
		Participant: "Mutable", Status: types.DiagramParticipantBoundaryUnproven,
	}}
	evidence := diagramEvidenceTestCall("appendStageOutputEvidenceToMutable", "MutableState.AppendEvidence")
	mut := types.NewMutableState("short node id endpoint collision")
	mut.AppendEvidence([]types.EvidenceItem{evidence})
	pctx := &preEmitCheckContext{ctx: &types.BusContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: mut,
	}}
	hints := preCheckDiagramParticipantCoverage(doc, view, pctx)
	if len(hints) != 1 {
		t.Fatalf("expected one label-aware endpoint-collision hint, got %+v", hints)
	}
	for _, want := range []string{
		`typed_endpoint_collision["Mutable"]`,
		`body_edge:{from_node:"W",to_node:"n6"}`,
		`conflict_visible_surface:"Mutable"`,
		`visible_label_collision:true`,
		`node_fields_to_change:"body.to_node+edge_anchor.to_node"`,
		`to_identity:"MutableState.AppendEvidence"`,
	} {
		if !strings.Contains(hints[0].ExpectedShape, want) {
			t.Fatalf("label-aware endpoint-collision hint missing %q:\n%s", want, hints[0].ExpectedShape)
		}
	}
}

func TestDiagramParticipantEndpointCollisionGuidanceFailsOpenWhenAmbiguous(t *testing.T) {
	rm, _, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired,
	}}
	doc.Blocks[0].Diagram.Body = "flowchart LR\n W1 --> Mutable\n W2 --> Mutable"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{
		{FromNode: "W1", ToNode: "Mutable", FromIdentity: "firstWriter", ToIdentity: "MutableState.First", RelationKind: types.DiagramRelCall},
		{FromNode: "W2", ToNode: "Mutable", FromIdentity: "secondWriter", ToIdentity: "MutableState.Second", RelationKind: types.DiagramRelCall},
	}
	mismatches := []DiagramParticipantCoverageMismatch{{
		BlockID: "flow", Participant: "Mutable", Issue: DiagramParticipantCoverageBoundaryConnected,
	}}
	if got := diagramParticipantEndpointConflictGuidance(doc, rm, mismatches, nil); got != "" {
		t.Fatalf("ambiguous body edges must retain generic fail-open guidance, got %s", got)
	}

	doc.Blocks[0].Diagram.Body = "flowchart LR\n W1 --> Mutable"
	doc.Blocks[0].EdgeAnchors[1].FromNode = "W1"
	if got := diagramParticipantEndpointConflictGuidance(doc, rm, mismatches, nil); got != "" {
		t.Fatalf("ambiguous anchors for one body edge must retain generic fail-open guidance, got %s", got)
	}
}

func TestDiagramParticipantCoverageAllowsNoArrowOwnershipGroupBesideLocalFacts(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
	}}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n subgraph BusContext[\"BusContext\"]\n M[\"Mutable\"]\n end\n W[\"append evidence\"] --> OP[\"MutableState.AppendEvidence\"]"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "W", ToNode: "OP", FromIdentity: "appendStageOutputEvidenceToMutable",
		ToIdentity: "MutableState.AppendEvidence", RelationKind: types.DiagramRelCall,
	}}
	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
		Participant: "BusContext", Status: types.DiagramParticipantBoundaryUnproven,
	}}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall(
		"appendStageOutputEvidenceToMutable", "MutableState.AppendEvidence",
	)}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence); len(got) != 0 {
		t.Fatalf("a no-arrow owner group and an independently proved local operation may coexist with an unproved requested relation: %+v", got)
	}

	doc.Blocks[0].Diagram.Body += "\n BusContext --> OP"
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence); len(got) != 1 || got[0].Issue != DiagramParticipantCoverageBoundaryConnected {
		t.Fatalf("the owner boundary must still reject a fabricated directed bridge: %+v", got)
	}
}

func TestDiagramParticipantCoverageDoesNotPromoteLocalCarrierCallIntoRequestedStageFlow(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	rm.AnalyzerHints.Entities = []string{"Analyzer", "Explorer", "Mutable"}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n Analyzer --> Explorer\n W[\"appendStageOutputEvidenceToMutable\"] --> OP[\"MutableState.AppendEvidence\"]\n Mutable"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{
		{FromNode: "Analyzer", ToNode: "Explorer", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelPrecedence},
		{FromNode: "W", ToNode: "OP", FromIdentity: "appendStageOutputEvidenceToMutable", ToIdentity: "MutableState.AppendEvidence", RelationKind: types.DiagramRelCall},
	}
	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
		Participant: "Mutable", Status: types.DiagramParticipantBoundaryUnproven,
	}}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall(
		"appendStageOutputEvidenceToMutable", "MutableState.AppendEvidence",
	)}
	stagePrecedence := []stageauthority.PrecedenceRelation{{
		From: stageauthority.StageRow{StageIdent: "StageAnalyze", StageValue: "analyze", AgentIdent: "AgentAnalyzer", AgentValue: "analyzer"},
		To:   stageauthority.StageRow{StageIdent: "StageExplore", StageValue: "explore", AgentIdent: "AgentExplorer", AgentValue: "explorer"},
	}}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence, stagePrecedence...); len(got) != 0 {
		t.Fatalf("a local carrier call must coexist with an unproved complete requested-flow boundary: %+v", got)
	}

	doc.Blocks[0].ParticipantBoundaries = nil
	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence, stagePrecedence...)
	if len(got) != 1 || got[0].Participant != "Mutable" || got[0].Issue != DiagramParticipantCoverageMissingBoundary {
		t.Fatalf("local carrier incidence must not eliminate the missing requested-flow boundary: %+v", got)
	}

	doc.Blocks[0].Diagram.Body += "\n Explorer --> Mutable"
	doc.Blocks[0].EdgeAnchors = append(doc.Blocks[0].EdgeAnchors, types.DiagramEdgeAnchor{
		FromNode: "Explorer", ToNode: "Mutable", FromIdentity: "Explorer", ToIdentity: "Mutable", RelationKind: types.DiagramRelCall,
	})
	evidence = append(evidence, diagramEvidenceTestCall("Explorer", "Mutable"))
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence, stagePrecedence...); len(got) != 0 {
		t.Fatalf("a real typed graph connecting the full requested participant roster may close the boundary: %+v", got)
	}
}

func TestDiagramParticipantCoverageKeepsRequestedBoundaryOrthogonalToExactLocalEndpoint(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		AnalyzerHints: types.AnalyzerHints{Entities: []string{"Analyzer", "Mutable"}},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true,
			Participants: []types.DiagramParticipantHint{
				{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
			}},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFGeneric, RelationAxis: types.AxisFlow,
		DiagramPlan:                   &types.DiagramFacetGraph{Kind: types.DiagramFlow, Required: true},
		DiagramParticipantObligations: append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...),
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart LR\n Analyzer[\"Analyzer\"]\n writer[\"local writer\"] --> Mutable[\"Mutable\"]"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "writer", ToNode: "Mutable", FromIdentity: "appendStageOutputEvidenceToMutable",
			ToIdentity: "MutableState.AppendEvidence", RelationKind: types.DiagramRelCall,
		}},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{
			{Participant: "Analyzer", Status: types.DiagramParticipantBoundaryUnproven},
			{Participant: "Mutable", Status: types.DiagramParticipantBoundaryUnproven},
		},
	}}}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall(
		"appendStageOutputEvidenceToMutable", "MutableState.AppendEvidence",
	)}

	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence); len(got) != 0 {
		t.Fatalf("an exact local technical endpoint must remain orthogonal to the unproved requested relation: %+v", got)
	}

	doc.Blocks[0].ParticipantBoundaries = doc.Blocks[0].ParticipantBoundaries[:1]
	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence)
	if len(got) != 1 || got[0].Participant != "Mutable" || got[0].Issue != DiagramParticipantCoverageMissingBoundary {
		t.Fatalf("local technical incidence must not discharge requested-relation coverage: %+v", got)
	}
}

func TestDiagramParticipantCoverageDoesNotPromotePerParticipantLocalFactsIntoRequestedRelation(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		AnalyzerHints: types.AnalyzerHints{Entities: []string{"ToolA", "ToolB"}},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true,
			Participants: []types.DiagramParticipantHint{
				{Identity: "ToolA", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "ToolB", Role: types.DiagramParticipantIncidentRequired},
			}},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFGeneric, RelationAxis: types.AxisFlow,
		DiagramPlan:                   &types.DiagramFacetGraph{Kind: types.DiagramFlow, Required: true},
		DiagramParticipantObligations: append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...),
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart LR\n A[\"ToolA\"]\n B[\"ToolB\"]\n AN[\"ToolA.Name\"] --> AL[\"tool-a\"]\n BN[\"ToolB.Name\"] --> BL[\"tool-b\"]"},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "AN", ToNode: "AL", FromIdentity: "ToolA.Name", ToIdentity: "tool-a", RelationKind: types.DiagramRelReturn},
			{FromNode: "BN", ToNode: "BL", FromIdentity: "ToolB.Name", ToIdentity: "tool-b", RelationKind: types.DiagramRelReturn},
		},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{
			{Participant: "ToolA", Status: types.DiagramParticipantBoundaryUnproven},
			{Participant: "ToolB", Status: types.DiagramParticipantBoundaryUnproven},
		},
	}}}
	toolAReturn := diagramEvidenceTestCall("ToolA.Name", "tool-a")
	toolAReturn.AnchorKind, toolAReturn.Predicate = types.AnchorReturn, "returns"
	toolBReturn := diagramEvidenceTestCall("ToolB.Name", "tool-b")
	toolBReturn.AnchorKind, toolBReturn.Predicate = types.AnchorReturn, "returns"
	evidence := []types.EvidenceItem{toolAReturn, toolBReturn}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence); len(got) != 0 {
		t.Fatalf("separate local facts must coexist with honest unproven requested-relation boundaries: %+v", got)
	}
	localCandidates := diagramParticipantTypedIncidentCandidates(rm, rm.DiagramHint.Participants[0], evidence, nil, 3)
	if len(localCandidates) != 1 ||
		!strings.Contains(localCandidates[0], `candidate_scope:"local_operation_only"`) ||
		!strings.Contains(localCandidates[0], `requested_relation_closure:"unproven"`) ||
		!strings.Contains(localCandidates[0], `retain_participant_boundary:true`) {
		t.Fatalf("a disconnected local fact must remain available only with its requested-relation boundary: %v", localCandidates)
	}

	doc.Blocks[0].ParticipantBoundaries = nil
	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence)
	if len(got) != 2 || got[0].Issue != DiagramParticipantCoverageMissingBoundary || got[1].Issue != DiagramParticipantCoverageMissingBoundary {
		t.Fatalf("both disconnected requested participants need explicit unproven boundaries: %+v", got)
	}
}

func TestDiagramParticipantTypedIncidentCandidatesRankByTypedAxisBeforeBound(t *testing.T) {
	participant := types.DiagramParticipantHint{
		Identity: "AgentContext.Mutable", Role: types.DiagramParticipantIncidentRequired,
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true,
			Participants: []types.DiagramParticipantHint{participant}},
	}
	firstCall := diagramEvidenceTestCall("AgentContext.Mutable.Load", "consumer.Apply")
	firstCall.Source, firstCall.LineStart = "internal/context/read.go", 20
	secondCall := diagramEvidenceTestCall("producer.Build", "AgentContext.Mutable.Store")
	secondCall.Source, secondCall.LineStart = "internal/context/write.go", 30
	initializer := diagramEvidenceTestCall("AgentContext.Mutable", "bus.Mutable")
	initializer.ID = "ev-agent-context-mutable-initializer"
	initializer.Predicate = "assigns"
	initializer.AnchorKind = types.AnchorInitializer
	initializer.AnchorSymbol = "Mutable"
	initializer.InitializerContainer = "AgentContext"
	initializer.Snippet = "Mutable: bus.Mutable,"
	initializer.Source, initializer.LineStart = "internal/context/builder.go", 59

	ordered := []types.EvidenceItem{firstCall, secondCall, initializer}
	reversed := []types.EvidenceItem{initializer, secondCall, firstCall}
	flowRows := diagramParticipantTypedIncidentCandidates(rm, participant, ordered, nil, 2)
	flowRowsReversed := diagramParticipantTypedIncidentCandidates(rm, participant, reversed, nil, 2)
	if len(flowRows) != 2 || strings.Join(flowRows, "\n") != strings.Join(flowRowsReversed, "\n") {
		t.Fatalf("bounded typed candidates must be stable across evidence arrival order:\nordered=%v\nreversed=%v", flowRows, flowRowsReversed)
	}
	if !strings.Contains(flowRows[0], `relation_kind:"data_flow"`) ||
		!strings.Contains(flowRows[0], `from_identity:"bus.Mutable"`) ||
		!strings.Contains(flowRows[0], `to_identity:"AgentContext.Mutable"`) {
		t.Fatalf("flow axis must retain the exact typed initializer/data-flow before weaker incident calls: %v", flowRows)
	}

	rm.PredicateAxis = types.AxisCall
	callRows := diagramParticipantTypedIncidentCandidates(rm, participant, ordered, nil, 1)
	if len(callRows) != 1 || !strings.Contains(callRows[0], `relation_kind:"call"`) {
		t.Fatalf("call axis must prefer an already-typed call without changing the candidate pool: %v", callRows)
	}
}

func TestDiagramParticipantTypedCandidatesResolveContainerFieldOverlapAcrossEdge(t *testing.T) {
	participants := []types.DiagramParticipantHint{
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: participants},
	}
	operation := diagramEvidenceTestCall("Mutable", "bus.Mutable")
	operation.ID = "ev-bus-mutable-copy"
	operation.Predicate = "assigns"
	operation.AnchorKind = types.AnchorInitializer
	operation.AnchorSymbol = "Mutable"
	operation.InitializerContainer = "AgentContext"
	operation.Snippet = "Mutable: bus.Mutable,"
	operation.Source, operation.LineStart = "internal/context/builder.go", 59
	operation.OwnerIdentity = "context.BuildAgentContext"
	operation.DeclaredIdentityBindings = []types.EvidenceDeclaredIdentityBinding{{
		Binding: "context.BuildAgentContext.bus", Type: "*types.BusContext", Owner: "context.BuildAgentContext",
	}}
	surfaces := [][]string{{"BusContext"}, {"Mutable"}}
	scope := buildFlowParticipantRelationScope(rm, participants, surfaces, []types.EvidenceItem{operation}, nil)
	if !scope.participantCovered[0] || !scope.participantCovered[1] || !scope.operationRelevant[0] {
		t.Fatalf("the unique BusContext-owned field -> Mutable receiver pair must stay request-scoped: %+v", scope)
	}
	for i, wantSide := range []string{"from", "to"} {
		rows := diagramParticipantTypedIncidentCandidateValuesWithScope(
			rm, participants[i], []types.EvidenceItem{operation}, nil, 2,
			participants, surfaces, i, scope,
		)
		if len(rows) != 1 || rows[0].from != "bus.Mutable" || rows[0].to != "AgentContext.Mutable" ||
			rows[0].participantEndpointSide != wantSide || !rows[0].directParticipantPair {
			t.Fatalf("participant %s lost the same direct typed copy on side %s: %+v", participants[i].Identity, wantSide, rows)
		}
	}
}

func TestDiagramParticipantRepairDeltaRetargetsExistingTypedJoinWithoutDuplicate(t *testing.T) {
	participants := []types.DiagramParticipantHint{
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: participants},
	}
	initializer := diagramEvidenceTestCall("Mutable", "bus.Mutable")
	initializer.ID = "ev-bus-mutable-copy"
	initializer.Predicate = "assigns"
	initializer.AnchorKind = types.AnchorInitializer
	initializer.AnchorSymbol = "Mutable"
	initializer.InitializerContainer = "AgentContext"
	initializer.Snippet = "Mutable: bus.Mutable,"
	initializer.Source, initializer.LineStart = "internal/context/builder.go", 59
	initializer.OwnerIdentity = "context.BuildAgentContext"
	initializer.DeclaredIdentityBindings = []types.EvidenceDeclaredIdentityBinding{{
		Binding: "context.BuildAgentContext.bus", Type: "*types.BusContext", Owner: "context.BuildAgentContext",
	}}
	local := diagramEvidenceTestCall("AgentContext.Mutable.Load", "consumer.Apply")
	local.Source, local.LineStart = "internal/context/read.go", 20
	evidence := []types.EvidenceItem{initializer, local}

	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid", Body: strings.Join([]string{
			"flowchart LR",
			" BusContext --> AgentContext",
			" Mutable --> LocalConsumer",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "BusContext", ToNode: "AgentContext", FromIdentity: "bus.Mutable", ToIdentity: "AgentContext.Mutable", RelationKind: types.DiagramRelDataFlow},
			{FromNode: "Mutable", ToNode: "LocalConsumer", FromIdentity: "AgentContext.Mutable.Load", ToIdentity: "consumer.Apply", RelationKind: types.DiagramRelCall},
		},
	}}}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	raw := diagramParticipantRepairAdditionDeltaJSON(doc, ctx, []DiagramParticipantCoverageMismatch{{
		BlockID: "flow", Participant: "Mutable", Issue: DiagramParticipantCoverageComponentSplit,
	}}, evidence, nil)
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(raw), &delta); err != nil {
		t.Fatalf("anchored component repair delta must parse: raw=%q err=%v", raw, err)
	}
	if len(delta.AllowedAdditions) != 0 || len(delta.Failures) != 1 {
		t.Fatalf("existing typed tuple must publish one replacement carrier and no duplicate addition: %+v", delta)
	}
	failure := delta.Failures[0]
	if failure.Issue != diagramParticipantComponentJoinEndpointMappingIssue ||
		failure.FromNode != "BusContext" || failure.ToNode != "AgentContext" ||
		failure.FromIdentity != "bus.Mutable" || failure.ToIdentity != "AgentContext.Mutable" ||
		failure.BodyOccurrence != 1 ||
		failure.TargetCarrier != types.AnswerDiagramRelationRepairCarrierPriorAnchor ||
		len(failure.AllowedActions) != 1 || failure.AllowedActions[0] != types.AnswerDiagramRelationRepairActionReplace ||
		strings.TrimSpace(failure.FailureRef) == "" {
		t.Fatalf("anchored join must be uniquely occurrence-bound and replace-only: %+v", failure)
	}
}

func TestDiagramParticipantTypedIncidentCandidatesRetainStageAndOperationDiversityWithinCap(t *testing.T) {
	participants := []types.DiagramParticipantHint{
		{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: participants},
	}
	rows := []stageauthority.StageRow{
		{StageIdent: "StageExplore", StageValue: "explore", AgentIdent: "AgentExplorer", AgentValue: "explorer"},
		{StageIdent: "StageExtract", StageValue: "extract", AgentIdent: "AgentExtractor", AgentValue: "extractor"},
		{StageIdent: "StageFinalize", StageValue: "finalize", AgentIdent: "AgentFinalizer", AgentValue: "finalizer"},
	}
	precedence := []stageauthority.PrecedenceRelation{
		{From: rows[0], To: rows[1], SourceFile: "internal/orchestrator/topology.go", LineStart: 10, LineEnd: 11},
		{From: rows[1], To: rows[2], SourceFile: "internal/orchestrator/topology.go", LineStart: 11, LineEnd: 12},
	}
	operation := diagramEvidenceTestCall("Extractor.BuildInitialInstruction", "Mutable.TurnAArtifacts")
	operation.Source, operation.LineStart = "internal/agent/extractor.go", 262
	evidence := []types.EvidenceItem{operation}
	obligations, surfaces := diagramParticipantCandidateObligations(rm)
	scope := buildFlowParticipantRelationScope(rm, obligations, surfaces, evidence, precedence)

	extractor := diagramParticipantTypedIncidentCandidateValuesWithScope(
		rm, obligations[0], evidence, precedence, 2, obligations, surfaces, 0, scope,
	)
	if len(extractor) != 2 || !extractor[0].stageAuthority || extractor[1].stageAuthority {
		t.Fatalf("bounded Extractor roster must retain one stage and one operation candidate: %+v", extractor)
	}
	if extractor[1].from != "Extractor.BuildInitialInstruction" || extractor[1].to != "Mutable.TurnAArtifacts" ||
		extractor[1].participantEndpointSide != "from" {
		t.Fatalf("operation candidate lost exact Extractor endpoint authority: %+v", extractor[1])
	}

	mutable := diagramParticipantTypedIncidentCandidateValuesWithScope(
		rm, obligations[1], evidence, precedence, 2, obligations, surfaces, 1, scope,
	)
	if len(mutable) != 1 || mutable[0].from != extractor[1].from || mutable[0].to != extractor[1].to ||
		mutable[0].relation != extractor[1].relation || mutable[0].participantEndpointSide != "to" {
		t.Fatalf("the opposite participant must receive the same exact typed edge on its own side: %+v", mutable)
	}
}

func TestFlowParticipantRelationScopeDoesNotJoinSharedReturnSinkAcrossOperations(t *testing.T) {
	participants := []types.DiagramParticipantHint{
		{Identity: "ToolA", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "ToolB", Role: types.DiagramParticipantIncidentRequired},
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true, Participants: participants},
	}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("ToolA.Check", "nil"),
		diagramEvidenceTestCall("ToolB.Check", "nil"),
	}
	for i := range evidence {
		evidence[i].AnchorKind = types.AnchorReturn
		evidence[i].Predicate = "returns"
	}
	scope := buildFlowParticipantRelationScope(rm, participants, [][]string{{"ToolA"}, {"ToolB"}}, evidence, nil)
	if scope.participantCovered[0] || scope.participantCovered[1] ||
		scope.operationRelevant[0] || scope.operationRelevant[1] {
		t.Fatalf("equal terminal return values must not join unrelated returning operations: %+v", scope)
	}
	for _, participant := range participants {
		if got := diagramParticipantTypedIncidentCandidates(rm, participant, evidence, nil, 3); len(got) != 0 {
			t.Fatalf("shared terminal return sink must not become a requested-relation repair candidate for %s: %v", participant.Identity, got)
		}
	}
}

func TestDiagramParticipantLocalOnlyScalarReturnExcludesValuesButKeepsCodeIdentity(t *testing.T) {
	for _, tc := range []struct {
		object string
		want   bool
	}{
		{object: "nil", want: true},
		{object: "true", want: true},
		{object: "42", want: true},
		{object: `"tool-a"`, want: true},
		{object: "resolved.Payload", want: false},
	} {
		operation := diagramEvidenceTestCall("ToolA.Resolve", tc.object)
		operation.AnchorKind = types.AnchorReturn
		operation.Predicate = "returns"
		if got := diagramParticipantLocalOnlyScalarReturn(operation); got != tc.want {
			t.Fatalf("local-only scalar return classification for %q=%v, want %v", tc.object, got, tc.want)
		}
	}
	call := diagramEvidenceTestCall("ToolA.Resolve", "dependency.Load")
	if diagramParticipantLocalOnlyScalarReturn(call) {
		t.Fatal("a non-return operation must never be removed by the scalar-return filter")
	}
}

func TestFlowParticipantRelationScopeKeepsDirectRequestedReturnEdge(t *testing.T) {
	participants := []types.DiagramParticipantHint{
		{Identity: "ToolA", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Payload", Role: types.DiagramParticipantIncidentRequired},
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true, Participants: participants},
	}
	returned := diagramEvidenceTestCall("ToolA.Load", "Payload")
	returned.AnchorKind = types.AnchorReturn
	returned.Predicate = "returns"
	scope := buildFlowParticipantRelationScope(rm, participants, [][]string{{"ToolA"}, {"Payload"}}, []types.EvidenceItem{returned}, nil)
	if !scope.participantCovered[0] || !scope.participantCovered[1] || !scope.operationRelevant[0] {
		t.Fatalf("one direct typed return must still cover its two requested endpoints: %+v", scope)
	}
}

func TestDiagramParticipantCoverageAcceptsMultiHopTypedRequestedRelation(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		AnalyzerHints: types.AnalyzerHints{Entities: []string{"ToolA", "ToolB"}},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true,
			Participants: []types.DiagramParticipantHint{
				{Identity: "ToolA", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "ToolB", Role: types.DiagramParticipantIncidentRequired},
			}},
	}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("ToolA.Execute", "shared.Dispatch"),
		diagramEvidenceTestCall("shared.Dispatch", "ToolB.Execute"),
	}
	for _, participant := range rm.DiagramHint.Participants {
		if got := diagramParticipantTypedIncidentCandidates(rm, participant, evidence, nil, 3); len(got) != 1 {
			t.Fatalf("the incident edge from a complete typed multi-hop component should be available without requiring a direct edge for %s: %v", participant.Identity, got)
		}
	}
}

func TestFlowParticipantRelationScopeFailsClosedOnAmbiguousParticipantEndpoint(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		AnalyzerHints: types.AnalyzerHints{Entities: []string{"Foo", "pkg.Foo"}},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true,
			Participants: []types.DiagramParticipantHint{
				{Identity: "Foo", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "pkg.Foo", Role: types.DiagramParticipantIncidentRequired},
			}},
	}
	surfaces := [][]string{{"Foo"}, {"pkg.Foo"}}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall("pkg.Foo.Run", "Sink.Accept")}
	scope := buildFlowParticipantRelationScope(rm, rm.DiagramHint.Participants, surfaces, evidence, nil)
	if scope.participantCovered[0] || scope.participantCovered[1] || scope.operationRelevant[0] {
		t.Fatalf("one ambiguous endpoint must not prove a relation between short and qualified participant identities: %+v", scope)
	}
}

func TestFlowParticipantRelationScopeJoinsVerifiedStageAndCarrierArguments(t *testing.T) {
	rows := []stageauthority.StageRow{
		{StageIdent: "StageAnalyze", StageValue: "analyze", AgentIdent: "AgentAnalyzer", AgentValue: "analyzer"},
		{StageIdent: "StageExplore", StageValue: "explore", AgentIdent: "AgentExplorer", AgentValue: "explorer"},
		{StageIdent: "StageExtract", StageValue: "extract", AgentIdent: "AgentExtractor", AgentValue: "extractor"},
		{StageIdent: "StageFinalize", StageValue: "finalize", AgentIdent: "AgentFinalizer", AgentValue: "finalizer"},
	}
	precedence := []stageauthority.PrecedenceRelation{
		{From: rows[0], To: rows[1]}, {From: rows[1], To: rows[2]}, {From: rows[2], To: rows[3]},
	}
	participants := []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	rm := types.RequestModel{Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: participants}}
	rm.AnalyzerHints.EntityProvenance = []types.EntityProvenance{{
		Surface: "Extractor", ResolvedAs: "types.AgentExtractor",
		Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true,
	}}
	surfaces := make([][]string, len(participants))
	for i := range participants {
		surfaces[i] = []string{participants[i].Identity}
	}
	argument := func(subject string) types.EvidenceItem {
		return types.EvidenceItem{ID: "arg-" + subject, Producer: types.EvidenceProducerExplorerEmitEvidence, Kind: types.EvidenceRelationship,
			Subject: subject, Predicate: "passes argument", Object: "BuildAgentContext",
			Source: "internal/orchestrator/extract_work.go", LineStart: 15,
			AnchorKind: types.AnchorArgument, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
			OwnerIdentity: "Orchestrator.extractStageHasRequiredWork"}
	}
	carrier := argument("o.busCtx")
	carrier.DeclaredIdentityBindings = []types.EvidenceDeclaredIdentityBinding{{
		Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
	}}
	evidence := []types.EvidenceItem{
		carrier,
		argument("types.AgentExtractor"),
		diagramEvidenceTestCall("BuildAgentContext", "bus.Mutable.Objective"),
	}
	splitScope := buildFlowParticipantRelationScope(rm, participants, surfaces, []types.EvidenceItem{
		carrier,
		diagramEvidenceTestCall("BuildAgentContext", "bus.Mutable.Objective"),
	}, precedence)
	for i := 0; i < 4; i++ {
		if !splitScope.participantCovered[i] || !splitScope.completionParticipantConnectedCovered(i) {
			t.Fatalf("verified stage participant %s should remain in the principal completion component: %+v", participants[i].Identity, splitScope)
		}
	}
	for i := 4; i < len(participants); i++ {
		if !splitScope.participantCovered[i] || splitScope.completionParticipantConnectedCovered(i) {
			t.Fatalf("disconnected carrier participant %s must stay open for a typed bridge pass: %+v", participants[i].Identity, splitScope)
		}
	}
	scope := buildFlowParticipantRelationScope(rm, participants, surfaces, evidence, precedence)
	for i, covered := range scope.participantCovered {
		if !covered {
			t.Fatalf("participant %s should join a verified relation component: %+v", participants[i].Identity, scope)
		}
		if !scope.participantRequestScopedCovered[i] {
			t.Fatalf("participant %s should inherit request-scoped authority through the exact carrier bridge: %+v", participants[i].Identity, scope)
		}
		if !scope.completionParticipantConnectedCovered(i) {
			t.Fatalf("participant %s should join the one connected completion component: %+v", participants[i].Identity, scope)
		}
	}
	if scope.requestScopedSubsetIncomplete {
		t.Fatalf("one exact component joining the provider and all requested carriers should be complete: %+v", scope)
	}
	for i, relevant := range scope.operationRelevant {
		if !relevant {
			t.Fatalf("operation %d should contribute to a requested relation component: %+v", i, scope)
		}
	}

	view := &types.AnswerSemanticView{
		Family: types.QFGeneric, RelationAxis: types.AxisFlow,
		DiagramPlan:                   &types.DiagramFacetGraph{Kind: types.DiagramArchitecture, Required: true},
		DiagramParticipantObligations: append([]types.DiagramParticipantHint(nil), participants...),
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid", Body: strings.Join([]string{
			"flowchart LR",
			" Analyzer[\"Analyzer\"] --> Explorer[\"Explorer\"] --> Extractor[\"Extractor\"] --> Finalizer[\"Finalizer\"]",
			" BusContext[\"BusContext\"] --> BuildAgentContext[\"构造阶段上下文\"] --> Mutable[\"Mutable\"]",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "Analyzer", ToNode: "Explorer", FromIdentity: "analyzer", ToIdentity: "explorer", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "Explorer", ToNode: "Extractor", FromIdentity: "explorer", ToIdentity: "extractor", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "Extractor", ToNode: "Finalizer", FromIdentity: "extractor", ToIdentity: "finalizer", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "BusContext", ToNode: "BuildAgentContext", FromIdentity: "o.busCtx", ToIdentity: "BuildAgentContext", RelationKind: types.DiagramRelArgumentFlow},
			{FromNode: "BuildAgentContext", ToNode: "Mutable", FromIdentity: "BuildAgentContext", ToIdentity: "bus.Mutable.Objective", RelationKind: types.DiagramRelCall},
		},
	}}}
	noBridgeEvidence := []types.EvidenceItem{
		carrier,
		diagramEvidenceTestCall("BuildAgentContext", "bus.Mutable.Objective"),
	}
	if got := diagramParticipantTypedJoinCandidates(doc, rm, noBridgeEvidence, precedence, 4); len(got) != 0 {
		t.Fatalf("local incident candidates must not be published as a cross-component repair frontier: %+v", got)
	}
	for _, mismatch := range DiagramParticipantCoverageMismatches(doc, view, rm, noBridgeEvidence, precedence...) {
		if mismatch.Issue == DiagramParticipantCoverageComponentSplit {
			t.Fatalf("component hard gate must not fire without an executable typed join candidate: %+v", mismatch)
		}
	}
	mismatches := DiagramParticipantCoverageMismatches(doc, view, rm, evidence, precedence...)
	if len(mismatches) != 2 || mismatches[0].Issue != DiagramParticipantCoverageComponentSplit ||
		mismatches[1].Issue != DiagramParticipantCoverageComponentSplit ||
		!((mismatches[0].Participant == "BusContext" && mismatches[1].Participant == "Mutable") ||
			(mismatches[0].Participant == "Mutable" && mismatches[1].Participant == "BusContext")) {
		t.Fatalf("typed-complete evidence must not allow the final diagram to re-split into participant islands: %+v", mismatches)
	}
	guidance := diagramParticipantCoverageCandidateGuidance(doc, rm, mismatches, evidence, precedence)
	for _, want := range []string{"typed_join_candidate[1]", `participant_node_id:"Extractor"`, `from_identity:"types.AgentExtractor"`, `to_identity:"BuildAgentContext"`} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("component-join repair must publish the already-proved principal-side bridge %q:\n%s", want, guidance)
		}
	}
	if strings.Contains(guidance, "typed_candidate[") {
		t.Fatalf("component repair must publish the crossing frontier without lower-value local incident noise:\n%s", guidance)
	}

	// A missing reader-facing identity can be reported before the complete
	// graph check. It must not suppress the same executable crossing frontier.
	// The component repair is higher value; after the model authors that join,
	// a later check may surface any still-missing local identity separately.
	identityGapDoc := *doc
	identityGapDoc.Blocks = append([]types.AnswerBlock(nil), doc.Blocks...)
	identityGapBlock := identityGapDoc.Blocks[0]
	identityGapBlock.Diagram = &types.AnswerDiagramBlock{
		Kind: doc.Blocks[0].Diagram.Kind, Language: doc.Blocks[0].Diagram.Language,
		Body: strings.ReplaceAll(doc.Blocks[0].Diagram.Body, `BusContext["BusContext"]`, `B["上下文载体"]`),
	}
	identityGapBlock.EdgeAnchors = append([]types.DiagramEdgeAnchor(nil), doc.Blocks[0].EdgeAnchors...)
	for i := range identityGapBlock.EdgeAnchors {
		if identityGapBlock.EdgeAnchors[i].FromNode == "BusContext" {
			identityGapBlock.EdgeAnchors[i].FromNode = "B"
		}
	}
	identityGapDoc.Blocks[0] = identityGapBlock
	identityGapMismatches := DiagramParticipantCoverageMismatches(&identityGapDoc, view, rm, evidence, precedence...)
	componentSeen := false
	for _, mismatch := range identityGapMismatches {
		if mismatch.Participant == "BusContext" && mismatch.Issue == DiagramParticipantCoverageIdentityMissing {
			t.Fatalf("a local identity symptom must not hide an executable component join: %+v", identityGapMismatches)
		}
		componentSeen = componentSeen || mismatch.Issue == DiagramParticipantCoverageComponentSplit
	}
	if !componentSeen {
		t.Fatalf("typed-complete split graph must retain the component repair frontier: %+v", identityGapMismatches)
	}
	identityGapGuidance := diagramParticipantCoverageCandidateGuidance(&identityGapDoc, rm, identityGapMismatches, evidence, precedence)
	if !strings.HasPrefix(identityGapGuidance, "typed_join_candidate[1]") || strings.Contains(identityGapGuidance, "typed_candidate[") {
		t.Fatalf("crossing candidates must lead and remain undiluted when a local mismatch was found first:\n%s", identityGapGuidance)
	}

	// Rendering the already-proved Extractor argument edge through the shared
	// technical handoff node joins both islands. No system-authored edge or
	// participant retarget is needed.
	doc.Blocks[0].Diagram.Body += "\n Extractor --> BuildAgentContext"
	doc.Blocks[0].EdgeAnchors = append(doc.Blocks[0].EdgeAnchors, types.DiagramEdgeAnchor{
		FromNode: "Extractor", ToNode: "BuildAgentContext",
		FromIdentity: "types.AgentExtractor", ToIdentity: "BuildAgentContext",
		RelationKind: types.DiagramRelArgumentFlow,
	})
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence, precedence...); len(got) != 0 {
		t.Fatalf("one model-authored typed bridge through a shared technical node should close the visible graph: %+v", got)
	}
}

func TestDiagramParticipantCoveragePublishesMultiEdgeJoinThroughNewTechnicalNode(t *testing.T) {
	rows := []stageauthority.StageRow{
		{StageIdent: "StageAnalyze", StageValue: "analyze", AgentIdent: "AgentAnalyzer", AgentValue: "analyzer"},
		{StageIdent: "StageExplore", StageValue: "explore", AgentIdent: "AgentExplorer", AgentValue: "explorer"},
	}
	precedence := []stageauthority.PrecedenceRelation{{From: rows[0], To: rows[1]}}
	participants := []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	rm := types.RequestModel{Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: participants}}
	rm.AnalyzerHints.EntityProvenance = []types.EntityProvenance{{
		Surface: "Explorer", ResolvedAs: "types.AgentExplorer",
		Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true,
	}}
	argument := func(id, subject string) types.EvidenceItem {
		return types.EvidenceItem{ID: id, Producer: types.EvidenceProducerExplorerEmitEvidence,
			Kind: types.EvidenceRelationship, Subject: subject, Predicate: "passes argument", Object: "BuildAgentContext",
			Source: "internal/orchestrator/explore.go", LineStart: 15,
			AnchorKind: types.AnchorArgument, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
			OwnerIdentity: "Orchestrator.exploreStage"}
	}
	busArgument := argument("arg-bus", "o.busCtx")
	busArgument.DeclaredIdentityBindings = []types.EvidenceDeclaredIdentityBinding{{
		Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
	}}
	evidence := []types.EvidenceItem{
		argument("arg-explorer", "types.AgentExplorer"),
		busArgument,
		diagramEvidenceTestCall("BusContext.SetMutable", "Mutable.Load"),
	}
	view := &types.AnswerSemanticView{
		Family: types.QFGeneric, RelationAxis: types.AxisFlow,
		DiagramPlan:                   &types.DiagramFacetGraph{Kind: types.DiagramArchitecture, Required: true},
		DiagramParticipantObligations: append([]types.DiagramParticipantHint(nil), participants...),
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid", Body: strings.Join([]string{
			"flowchart LR",
			" Analyzer[\"Analyzer\"] --> Explorer[\"Explorer\"]",
			" BusContext[\"BusContext\"] --> Mutable[\"Mutable\"]",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "Analyzer", ToNode: "Explorer", FromIdentity: "analyzer", ToIdentity: "explorer", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "BusContext", ToNode: "Mutable", FromIdentity: "BusContext.SetMutable", ToIdentity: "Mutable.Load", RelationKind: types.DiagramRelCall},
		},
	}}}

	join := diagramParticipantTypedJoinCandidates(doc, rm, evidence, precedence, 4)
	if len(join) != 2 {
		t.Fatalf("two exact argument rows sharing one new technical node should be one executable join path: %+v; candidates=%s", join,
			flowParticipantTypedIncidentCandidateGuidance(rm, evidence, precedence, nil, 8))
	}
	wantPairs := map[string]bool{
		"types.AgentExplorer\x00BuildAgentContext": false,
		"o.busCtx\x00BuildAgentContext":            false,
	}
	for _, candidate := range join {
		key := candidate.from + "\x00" + candidate.to
		if _, ok := wantPairs[key]; ok {
			wantPairs[key] = true
		}
	}
	for pair, seen := range wantPairs {
		if !seen {
			t.Fatalf("multi-edge join path omitted typed row %q: %+v", pair, join)
		}
	}
	mismatches := DiagramParticipantCoverageMismatches(doc, view, rm, evidence, precedence...)
	componentSplit := false
	for _, mismatch := range mismatches {
		componentSplit = componentSplit || mismatch.Issue == DiagramParticipantCoverageComponentSplit
	}
	if !componentSplit {
		t.Fatalf("typed-complete two-edge join must keep a split visible graph open for repair: %+v", mismatches)
	}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	deltaJSON := diagramParticipantRepairAdditionDeltaJSON(doc, ctx, mismatches, evidence, precedence)
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(deltaJSON), &delta); err != nil {
		t.Fatalf("multi-edge join repair must publish one executable typed delta: err=%v raw=%s", err, deltaJSON)
	}
	if len(delta.AllowedAdditions) != 2 {
		t.Fatalf("multi-edge join repair must carry the complete shortest path in one patch generation: %+v", delta)
	}

	doc.Blocks[0].Diagram.Body += "\n Explorer --> BuildAgentContext\n BusContext --> BuildAgentContext"
	doc.Blocks[0].EdgeAnchors = append(doc.Blocks[0].EdgeAnchors,
		types.DiagramEdgeAnchor{FromNode: "Explorer", ToNode: "BuildAgentContext", FromIdentity: "types.AgentExplorer", ToIdentity: "BuildAgentContext", RelationKind: types.DiagramRelArgumentFlow},
		types.DiagramEdgeAnchor{FromNode: "BusContext", ToNode: "BuildAgentContext", FromIdentity: "o.busCtx", ToIdentity: "BuildAgentContext", RelationKind: types.DiagramRelArgumentFlow},
	)
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence, precedence...); len(got) != 0 {
		t.Fatalf("the model-authored complete two-edge join should close the visible requested graph: %+v", got)
	}
}

func TestFlowParticipantRelationScopeTreatsDisconnectedFullRequestRosterAsPartial(t *testing.T) {
	participants := []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
	}
	rm := types.RequestModel{Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: participants}}
	precedence := []stageauthority.PrecedenceRelation{
		{From: stageauthority.StageRow{StageIdent: "StageAnalyze", StageValue: "analyze", AgentIdent: "AgentAnalyzer", AgentValue: "analyzer"},
			To: stageauthority.StageRow{StageIdent: "StageExplore", StageValue: "explore", AgentIdent: "AgentExplorer", AgentValue: "explorer"}},
		{From: stageauthority.StageRow{StageIdent: "StageExtract", StageValue: "extract", AgentIdent: "AgentExtractor", AgentValue: "extractor"},
			To: stageauthority.StageRow{StageIdent: "StageFinalize", StageValue: "finalize", AgentIdent: "AgentFinalizer", AgentValue: "finalizer"}},
	}
	scope := buildFlowParticipantRelationScope(
		rm, participants,
		[][]string{{"Analyzer"}, {"Explorer"}, {"Extractor"}, {"Finalizer"}},
		nil, precedence,
	)
	for i, covered := range scope.participantRequestScopedCovered {
		if !covered {
			t.Fatalf("both provider-owned islands should retain typed request scope for participant %d: %+v", i, scope)
		}
	}
	if scope.requestScopedRelationComplete || !scope.requestScopedSubsetIncomplete {
		t.Fatalf("a full name roster split across two typed components remains partial: %+v", scope)
	}
	exported := ResolveFlowParticipantRelationCoverage(
		rm, participants,
		[][]string{{"Analyzer"}, {"Explorer"}, {"Extractor"}, {"Finalizer"}},
		nil, precedence,
	)
	if exported.RequestScopedRelationComplete || !exported.RequestScopedSubsetIncomplete {
		t.Fatalf("prompt and hard-gate consumers must receive the same disconnected-scope verdict: %+v", exported)
	}
}

func TestDiagramParticipantCoverageKeepsDisconnectedLocalPairOutsideRequestScopedProvider(t *testing.T) {
	participants := []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	rm := types.RequestModel{Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: participants}}
	view := &types.AnswerSemanticView{
		Family: types.QFGeneric, RelationAxis: types.AxisFlow,
		DiagramPlan:                   &types.DiagramFacetGraph{Kind: types.DiagramArchitecture, Required: true},
		DiagramParticipantObligations: append([]types.DiagramParticipantHint(nil), participants...),
	}
	precedence := []stageauthority.PrecedenceRelation{{
		From: stageauthority.StageRow{StageIdent: "StageAnalyze", StageValue: "analyze", AgentIdent: "AgentAnalyzer", AgentValue: "analyzer"},
		To:   stageauthority.StageRow{StageIdent: "StageExplore", StageValue: "explore", AgentIdent: "AgentExplorer", AgentValue: "explorer"},
	}}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall("BusContext.SetMutable", "Mutable.Load")}
	surfaces := [][]string{{"Analyzer"}, {"Explorer"}, {"BusContext"}, {"Mutable"}}
	scope := buildFlowParticipantRelationScope(rm, participants, surfaces, evidence, precedence)
	for i, covered := range scope.participantCovered {
		if !covered {
			t.Fatalf("both independently proved pairs remain valid typed components; participant %s was lost: %+v", participants[i].Identity, scope)
		}
	}
	if !scope.requestScopedSubsetIncomplete ||
		!scope.effectiveParticipantCovered(0) || !scope.effectiveParticipantCovered(1) ||
		scope.effectiveParticipantCovered(2) || scope.effectiveParticipantCovered(3) {
		t.Fatalf("only the provider-connected stage pair should close request-scoped coverage before a full authored graph exists: %+v", scope)
	}

	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid",
			Body: "flowchart LR\n Analyzer[\"Analyzer\"] --> Explorer[\"Explorer\"]\n BusContext[\"BusContext\"] --> Mutable[\"Mutable\"]"},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "Analyzer", ToNode: "Explorer", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "BusContext", ToNode: "Mutable", FromIdentity: "BusContext.SetMutable", ToIdentity: "Mutable.Load", RelationKind: types.DiagramRelCall},
		},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{
			{Participant: "BusContext", Status: types.DiagramParticipantBoundaryUnproven},
			{Participant: "Mutable", Status: types.DiagramParticipantBoundaryUnproven},
		},
	}}}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence, precedence...); len(got) != 0 {
		t.Fatalf("a truthful provider subset plus disconnected local pair should retain local facts and requested-relation boundaries: %+v", got)
	}
	scopeMismatches := DiagramRequestedRelationScopeMismatches(doc, view, rm, evidence, precedence...)
	if len(scopeMismatches) != 1 || scopeMismatches[0].Issue != DiagramRequestedRelationScopeMissing {
		t.Fatalf("disconnected local islands need one whole-diagram partial scope disclosure: %+v", scopeMismatches)
	}
	doc.Blocks[0].RequestedRelationScope = types.DiagramRelationScopePartialUnproven
	if got := DiagramRequestedRelationScopeMismatches(doc, view, rm, evidence, precedence...); len(got) != 0 {
		t.Fatalf("one model-authored whole-diagram scope disclosure should satisfy the partial typed spine: %+v", got)
	}
	if len(doc.Blocks[0].EdgeAnchors) != 2 {
		t.Fatalf("scope disclosure must not create, delete, or reconnect model-authored edges: %+v", doc.Blocks[0].EdgeAnchors)
	}
	duplicate := doc.Blocks[0]
	duplicate.ID = "flow-support"
	doc.Blocks = append(doc.Blocks, duplicate)
	if got := DiagramRequestedRelationScopeMismatches(doc, view, rm, evidence, precedence...); len(got) != 1 ||
		got[0].Issue != DiagramRequestedRelationScopeDuplicate || got[0].BlockID != "flow-support" {
		t.Fatalf("whole-relation scope must have exactly one model-authored carrier: %+v", got)
	}
	doc.Blocks = doc.Blocks[:1]
	if got := DiagramRequestedRelationScopeMismatches(doc, view, rm, evidence); len(got) != 1 ||
		got[0].Issue != DiagramRequestedRelationScopeStale {
		t.Fatalf("partial scope declaration must be removed without a typed partial-spine provider: %+v", got)
	}
	localCandidates := diagramParticipantTypedIncidentCandidates(rm, participants[2], evidence, precedence, 3)
	if len(localCandidates) != 1 ||
		!strings.Contains(localCandidates[0], `candidate_scope:"local_operation_only"`) ||
		!strings.Contains(localCandidates[0], `requested_relation_closure:"unproven"`) ||
		!strings.Contains(localCandidates[0], `retain_participant_boundary:true`) {
		t.Fatalf("a disconnected local carrier edge must remain available only as local authoring input with its requested boundary: %v", localCandidates)
	}

	doc.Blocks[0].ParticipantBoundaries = nil
	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence, precedence...)
	if len(got) != 2 || got[0].Issue != DiagramParticipantCoverageMissingBoundary ||
		got[1].Issue != DiagramParticipantCoverageMissingBoundary {
		t.Fatalf("both local-only participants need explicit requested-relation boundaries: %+v", got)
	}
}

func TestFlowParticipantRelationScopeStrictProviderCannotBeWidenedByAuthoredGraph(t *testing.T) {
	scope := flowParticipantRelationScope{
		participantCovered:              []bool{true, true, true},
		participantRequestScopedCovered: []bool{true, true, false},
		requestScopedSubsetIncomplete:   true,
	}
	if !scope.effectiveParticipantCovered(0) || !scope.effectiveParticipantCovered(1) {
		t.Fatal("the parser-owned request-scoped provider must retain its covered participants")
	}
	if scope.effectiveParticipantCovered(2) {
		t.Fatal("a model-authored connected graph must not promote a local-only participant into parser-owned request scope")
	}
}

func TestDiagramParticipantCoverageCompactRepairRetainsLocalCandidateAndRequestedBoundary(t *testing.T) {
	participants := []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	rm := types.RequestModel{Intent: types.IntentExplain, PredicateAxis: types.AxisFlow, Language: "zh-CN",
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: participants}}
	precedence := []stageauthority.PrecedenceRelation{{
		From:       stageauthority.StageRow{StageIdent: "StageAnalyze", StageValue: "analyze", AgentIdent: "AgentAnalyzer", AgentValue: "analyzer"},
		To:         stageauthority.StageRow{StageIdent: "StageExplore", StageValue: "explore", AgentIdent: "AgentExplorer", AgentValue: "explorer"},
		SourceFile: "internal/orchestrator/topology.go", LineStart: 21, LineEnd: 22,
	}}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall("BuildAgentContext", "bus.Mutable.Objective")}
	mismatches := []DiagramParticipantCoverageMismatch{{
		BlockID: "flow", Participant: "Analyzer", Issue: DiagramParticipantCoverageTypedEdgeMissing,
	}}

	got := diagramParticipantCoverageCompactCandidateGuidance(
		&types.AnswerDocumentV2{DocumentModel: "v2"}, rm, mismatches, evidence, precedence,
	)
	for _, want := range []string{
		"typed_candidate[Analyzer][1]",
		`visible_arrow_label:"确定分析范围后收集证据"`,
		"typed_candidate[Mutable][1]",
		`candidate_scope:"local_operation_only"`,
		`requested_relation_closure:"unproven"`,
		`retain_participant_boundary:true`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compact repair must retain the failed requested candidate and independently grounded local relation; missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `typed_candidate[Analyzer][1]={relation_kind:"precedence",visible_arrow_label:"随后进入"`) {
		t.Fatalf("stage participant candidate must not diverge from the canonical stage-recipe wording: %s", got)
	}
	if strings.Contains(got, "typed_candidate[Explorer]") {
		t.Fatalf("compact repair must not widen the request-scoped repair roster beyond the failed participant: %s", got)
	}
}

func TestDiagramParticipantCoverageDoesNotJoinRepoWideExpansionIntoRequestedStageFlow(t *testing.T) {
	participants := []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
	}
	rm := types.RequestModel{Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: participants}}
	view := &types.AnswerSemanticView{
		Family: types.QFGeneric, RelationAxis: types.AxisFlow,
		DiagramPlan:                   &types.DiagramFacetGraph{Kind: types.DiagramArchitecture, Required: true},
		DiagramParticipantObligations: append([]types.DiagramParticipantHint(nil), participants...),
	}
	precedence := []stageauthority.PrecedenceRelation{{
		From: stageauthority.StageRow{StageIdent: "StageAnalyze", StageValue: "analyze", AgentIdent: "AgentAnalyzer", AgentValue: "analyzer"},
		To:   stageauthority.StageRow{StageIdent: "StageExplore", StageValue: "explore", AgentIdent: "AgentExplorer", AgentValue: "explorer"},
	}}
	autoExplorer := diagramEvidenceTestCall("internal/agent/explorer.go:renderExplorerToolBudgetPlan", "append")
	autoExplorer.Producer = types.EvidenceProducerRepoMapCooperativeCall
	autoExplorer.Source = "internal/agent/explorer.go"
	autoBus := diagramEvidenceTestCall("append", "BusContext.Flush")
	autoBus.Producer = types.EvidenceProducerRepoMapCooperativeCall
	autoBus.Source = "internal/orchestrator/orchestrator.go"
	evidence := []types.EvidenceItem{autoExplorer, autoBus}

	surfaces := [][]string{{"Analyzer"}, {"Explorer"}, {"BusContext"}}
	scope := buildFlowParticipantRelationScope(rm, participants, surfaces, evidence, precedence)
	if !scope.effectiveParticipantCovered(0) || !scope.effectiveParticipantCovered(1) ||
		scope.effectiveParticipantCovered(2) {
		t.Fatalf("repo-wide expansion must not extend the requested stage provider to BusContext: %+v", scope)
	}

	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid",
			Body: "flowchart LR\n Analyzer[\"Analyzer\"] --> Explorer[\"Explorer\"]\n Explorer --> append\n append --> BusContext[\"BusContext\"]"},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "Analyzer", ToNode: "Explorer", FromIdentity: "analyzer", ToIdentity: "explorer", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "Explorer", ToNode: "append", FromIdentity: autoExplorer.Subject, ToIdentity: autoExplorer.Object, RelationKind: types.DiagramRelCall},
			{FromNode: "append", ToNode: "BusContext", FromIdentity: autoBus.Subject, ToIdentity: autoBus.Object, RelationKind: types.DiagramRelCall},
		},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{{
			Participant: "BusContext", Status: types.DiagramParticipantBoundaryUnproven,
		}},
	}}}
	if got := diagramParticipantTypedJoinCandidates(doc, rm, evidence, precedence, 2); len(got) != 0 {
		t.Fatalf("repo-wide expansion must not be published as a requested join candidate: %+v", got)
	}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence, precedence...); len(got) != 0 {
		t.Fatalf("truthful local background edges plus an unproven requested boundary should pass: %+v", got)
	}
	doc.Blocks[0].ParticipantBoundaries = nil
	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence, precedence...)
	if len(got) != 1 || got[0].Participant != "BusContext" || got[0].Issue != DiagramParticipantCoverageMissingBoundary {
		t.Fatalf("background-only BusContext incidence must not close requested relation coverage: %+v", got)
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

func TestDiagramParticipantCoverageExactEndpointNodeIDMayCarryBusinessLabel(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired,
	}}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n A[\"准备分析\"] --> Mutable[\"共享可变状态\"]"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "A", ToNode: "Mutable", FromIdentity: "analyzerEvaluator.BuildInitialInstruction",
		ToIdentity: "ctx.Mutable.SetSearchGraph", RelationKind: types.DiagramRelCall,
	}}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall(
		"analyzerEvaluator.BuildInitialInstruction", "ctx.Mutable.SetSearchGraph",
	)}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence); len(got) != 0 {
		t.Fatalf("exact participant endpoint node id plus business label should preserve typed incidence: %+v", got)
	}

	doc.Blocks[0].Diagram.Body = "flowchart LR\n A[\"准备分析\"] --> Other[\"共享可变状态\"]"
	doc.Blocks[0].EdgeAnchors[0].ToNode = "Other"
	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence)
	if len(got) != 1 || got[0].Issue != DiagramParticipantCoverageIdentityMissing {
		t.Fatalf("an unrelated endpoint node id must not borrow participant identity from a generic label: %+v", got)
	}
}

func TestDiagramParticipantCoverageRejectsRequestedParticipantOnNonincidentCandidateSide(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
	}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n BusContext[\"BusContext\"] --> Analyzer[\"Analyzer\"]"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "BusContext", ToNode: "Analyzer", FromIdentity: "o.busCtx",
		ToIdentity: "ctxbuilder.BuildAgentContext", RelationKind: types.DiagramRelArgumentFlow,
	}}
	evidence := []types.EvidenceItem{{
		ID: "bus-argument", Producer: types.EvidenceProducerExplorerEmitEvidence,
		Kind: types.EvidenceRelationship, Subject: "o.busCtx", Predicate: "passes",
		Object: "ctxbuilder.BuildAgentContext", Source: "internal/orchestrator/orchestrator.go", LineStart: 8029,
		AnchorKind: types.AnchorArgument, AnchorSymbol: "o.busCtx", Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
		OwnerIdentity:   "Orchestrator.dispatchStage",
		DeclaredIdentityBindings: []types.EvidenceDeclaredIdentityBinding{{
			Binding: "o.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
		}},
	}}

	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence)
	found := false
	for _, mismatch := range got {
		if mismatch.BlockID == "flow" && mismatch.Participant == "Analyzer" &&
			mismatch.Issue == DiagramParticipantCoverageEndpointRetargeted {
			found = true
		}
	}
	if !found {
		t.Fatalf("candidate's opposite endpoint borrowed Analyzer without typed incidence: %+v", got)
	}
	mut := types.NewMutableState("participant endpoint-side retarget")
	mut.AppendEvidence(evidence)
	hints := preCheckDiagramParticipantCoverage(doc, view, &preEmitCheckContext{ctx: &types.BusContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: mut,
	}})
	if len(hints) != 1 || !strings.Contains(hints[0].ExpectedShape, "participant=Analyzer issue=participant_visible_on_nonincident_endpoint") ||
		!strings.Contains(hints[0].ExpectedShape, "opposite typed endpoint is independently incident to it") ||
		!strings.Contains(hints[0].ExpectedShape, "remove_the_requested_participant_identity_from_the_nonincident_endpoint") ||
		!strings.Contains(hints[0].ExpectedShape, `typed_endpoint_collision["Analyzer"]`) ||
		!strings.Contains(hints[0].ExpectedShape, `body_edge:{from_node:"BusContext",to_node:"Analyzer"}`) ||
		!strings.Contains(hints[0].ExpectedShape, `conflict_endpoint_side:"to"`) ||
		!strings.Contains(hints[0].ExpectedShape, `node_fields_to_change:"body.to_node+edge_anchor.to_node"`) {
		t.Fatalf("retarget repair lost its side-specific typed contract: %+v", hints)
	}

	// Reader-friendly wording remains legal when it does not impersonate a
	// different requested participant. The exact operation identity stays in
	// the anchor and continues to own relation authority.
	doc.Blocks[0].Diagram.Body = "flowchart LR\n BusContext[\"BusContext\"] --> Build[\"构建 AgentContext\"]"
	doc.Blocks[0].EdgeAnchors[0].ToNode = "Build"
	for _, mismatch := range DiagramParticipantCoverageMismatches(doc, view, rm, evidence) {
		if mismatch.Issue == DiagramParticipantCoverageEndpointRetargeted {
			t.Fatalf("neutral business wording was incorrectly treated as a participant retarget: %+v", mismatch)
		}
	}
}

func TestDiagramParticipantCoverageRejectsBoundedParticipantRetargetedFromSiblingIdentity(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n subgraph BusContext[\"BusContext\"]\n  mutable[\"Mutable\"]\n end\n mutable -->|作为参数传递| ctxbuilder[\"构建阶段上下文\"]"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "mutable", ToNode: "ctxbuilder", FromIdentity: "o.busCtx",
		ToIdentity: "ctxbuilder.BuildAgentContext", RelationKind: types.DiagramRelArgumentFlow,
		VisibleLabel: "作为参数传递",
	}}
	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
		Participant: "Mutable", Status: types.DiagramParticipantBoundaryUnproven,
	}}
	evidence := []types.EvidenceItem{{
		ID: "bus-argument", Producer: types.EvidenceProducerExplorerEmitEvidence,
		Kind: types.EvidenceRelationship, Subject: "o.busCtx", Predicate: "passes",
		Object: "ctxbuilder.BuildAgentContext", Source: "internal/orchestrator/extract_work.go", LineStart: 15,
		AnchorKind: types.AnchorArgument, AnchorSymbol: "o.busCtx", Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded, OwnerIdentity: "Orchestrator.extractStageHasRequiredWork",
		DeclaredIdentityBindings: []types.EvidenceDeclaredIdentityBinding{{
			Binding: "o.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
		}},
	}}

	found := false
	for _, mismatch := range DiagramParticipantCoverageMismatches(doc, view, rm, evidence) {
		if mismatch.BlockID == "flow" && mismatch.Participant == "Mutable" &&
			mismatch.Issue == DiagramParticipantCoverageEndpointRetargeted {
			found = true
		}
	}
	if !found {
		t.Fatal("a Mutable boundary must not allow the BusContext argument endpoint to be relabelled as Mutable")
	}
	mut := types.NewMutableState("bounded sibling endpoint retarget")
	mut.AppendEvidence(evidence)
	hints := preCheckDiagramParticipantCoverage(doc, view, &preEmitCheckContext{ctx: &types.BusContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: mut,
	}})
	if len(hints) != 1 ||
		!strings.Contains(hints[0].ExpectedShape, `typed_endpoint_collision["Mutable"]`) ||
		!strings.Contains(hints[0].ExpectedShape, `body_edge:{from_node:"mutable",to_node:"ctxbuilder"}`) ||
		!strings.Contains(hints[0].ExpectedShape, `conflict_endpoint_side:"from"`) ||
		!strings.Contains(hints[0].ExpectedShape, `from_identity:"o.busCtx"`) {
		t.Fatalf("bounded same-side retarget must receive an exact non-authoring repair tuple: %+v", hints)
	}
}

func TestDiagramParticipantCoverageRejectsRepeatedPairRetargetPerOccurrence(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{
		{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n Extractor --> Mutable\n Extractor --> Mutable"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{
		{FromNode: "Extractor", ToNode: "Mutable", FromIdentity: "Orchestrator.hasReusableTurnBSlateForFinalize", ToIdentity: "o.busCtx.Mutable.EmittedAnswerSymbols", RelationKind: types.DiagramRelCall},
		{FromNode: "Extractor", ToNode: "Mutable", FromIdentity: "Orchestrator.hasReusableTurnBSlateForFinalize", ToIdentity: "o.busCtx.Mutable.EmittedHypothesisVerdicts", RelationKind: types.DiagramRelCall},
	}
	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
		Participant: "Mutable", Status: types.DiagramParticipantBoundaryUnproven,
	}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("Orchestrator.hasReusableTurnBSlateForFinalize", "o.busCtx.Mutable.EmittedAnswerSymbols"),
		diagramEvidenceTestCall("Orchestrator.hasReusableTurnBSlateForFinalize", "o.busCtx.Mutable.EmittedHypothesisVerdicts"),
	}

	found := false
	for _, mismatch := range DiagramParticipantCoverageMismatches(doc, view, rm, evidence) {
		if mismatch.BlockID == "flow" && mismatch.Participant == "Extractor" &&
			mismatch.Issue == DiagramParticipantCoverageEndpointRetargeted {
			found = true
		}
	}
	if !found {
		t.Fatalf("distinct typed operations collapsed onto one repeated business pair must not bypass endpoint incidence")
	}
}

func TestDiagramParticipantEndpointRetargetGuidanceNamesOnlyConflictingLocalEdge(t *testing.T) {
	rm, _, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	doc.Blocks[0].Diagram.Body = "flowchart LR\n Analyzer --> Explorer\n Analyzer --> Mutable"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{
		{FromNode: "Analyzer", ToNode: "Explorer", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelPrecedence},
		{FromNode: "Analyzer", ToNode: "Mutable", FromIdentity: "analyzerEvaluator.BuildInitialInstruction", ToIdentity: "ctx.Mutable.ResetPrescanSummary", RelationKind: types.DiagramRelCall},
	}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall(
		"analyzerEvaluator.BuildInitialInstruction", "ctx.Mutable.ResetPrescanSummary",
	)}
	mismatches := []DiagramParticipantCoverageMismatch{{
		BlockID: "flow", Participant: "Analyzer", Issue: DiagramParticipantCoverageEndpointRetargeted,
	}}
	got := diagramParticipantEndpointConflictGuidance(doc, rm, mismatches, evidence)
	for _, want := range []string{
		`typed_endpoint_collision["Analyzer"]`,
		`body_edge:{from_node:"Analyzer",to_node:"Mutable"}`,
		`conflict_endpoint_side:"from"`,
		`from_identity:"analyzerEvaluator.BuildInitialInstruction"`,
		`to_identity:"ctx.Mutable.ResetPrescanSummary"`,
		`node_fields_to_change:"body.from_node+edge_anchor.from_node"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("endpoint-retarget guidance missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `body_edge:{from_node:"Analyzer",to_node:"Explorer"}`) {
		t.Fatalf("valid stage edge must not be named as the retarget conflict: %s", got)
	}
}

func TestDiagramParticipantCoverageAllowsParticipantWhenSameTypedEndpointIsIncident(t *testing.T) {
	rm, view, doc, evidence := diagramParticipantCoverageFixture()
	doc.Blocks[0].Diagram.Body = "flowchart LR\n Analyzer[\"Analyzer\"] --> Explorer[\"Explorer\"]\n MutableState[\"MutableState\"]"
	doc.Blocks[0].EdgeAnchors[0] = types.DiagramEdgeAnchor{
		FromNode: "Analyzer", ToNode: "Explorer", FromIdentity: "Analyzer",
		ToIdentity: "Explorer", RelationKind: types.DiagramRelCall,
	}
	for _, mismatch := range DiagramParticipantCoverageMismatches(doc, view, rm, evidence) {
		if mismatch.Issue == DiagramParticipantCoverageEndpointRetargeted {
			t.Fatalf("a participant may label the exact endpoint incident to itself: %+v", mismatch)
		}
	}
}

func TestDiagramParticipantCandidateEndpointSideIsDeterministic(t *testing.T) {
	for _, tc := range []struct {
		name         string
		fromIncident bool
		toIncident   bool
		want         string
	}{
		{name: "from", fromIncident: true, want: "from"},
		{name: "to", toIncident: true, want: "to"},
		{name: "both", fromIncident: true, toIncident: true, want: "from_or_to"},
		{name: "neither", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := diagramParticipantCandidateEndpointSide(tc.fromIncident, tc.toIncident); got != tc.want {
				t.Fatalf("participant endpoint side drift: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestDiagramParticipantRenderedRequestScopedCandidateRequiresExactVisibleCarrier(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart LR\n O[\"产生证据\"] --> B[\"BusContext\"]"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "O", ToNode: "B", FromIdentity: "output.EvidenceItems",
			ToIdentity: "o.busCtx.EvidenceItems", RelationKind: types.DiagramRelDataFlow,
		}},
	}}}
	candidate := diagramParticipantTypedIncidentCandidate{
		participant: "BusContext", relation: types.DiagramRelDataFlow,
		from: "output.EvidenceItems", to: "o.busCtx.EvidenceItems",
		participantEndpointSide: "to",
	}
	if !diagramParticipantHasRenderedRequestScopedCandidate(doc, []string{"BusContext"}, []diagramParticipantTypedIncidentCandidate{candidate}) {
		t.Fatal("an exact canonical candidate with its declared visible participant endpoint must be recognized")
	}

	localOnly := candidate
	localOnly.localOnly = true
	if diagramParticipantHasRenderedRequestScopedCandidate(doc, []string{"BusContext"}, []diagramParticipantTypedIncidentCandidate{localOnly}) {
		t.Fatal("an independent local operation must not close request-scoped participant incidence")
	}
	wrongSide := candidate
	wrongSide.participantEndpointSide = "from"
	if diagramParticipantHasRenderedRequestScopedCandidate(doc, []string{"BusContext"}, []diagramParticipantTypedIncidentCandidate{wrongSide}) {
		t.Fatal("the opposite visible endpoint must not carry the requested participant")
	}

	anchorOnly := *doc
	anchorOnly.Blocks = append([]types.AnswerBlock(nil), doc.Blocks...)
	anchorOnly.Blocks[0].Diagram = &types.AnswerDiagramBlock{
		Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n B[\"BusContext\"]",
	}
	if diagramParticipantHasRenderedRequestScopedCandidate(&anchorOnly, []string{"BusContext"}, []diagramParticipantTypedIncidentCandidate{candidate}) {
		t.Fatal("a hidden structured anchor without its visible body edge is not rendered incidence")
	}
}

func TestParticipantOnlyAdditionCapabilityOmitsAlreadyAnchoredTuple(t *testing.T) {
	rm, _, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
	}}
	doc.Blocks[0].Diagram.Body = "flowchart LR\n O[\"产生证据\"] --> B[\"BusContext\"]"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "O", ToNode: "B", FromIdentity: "output.EvidenceItems",
		ToIdentity: "o.busCtx.EvidenceItems", RelationKind: types.DiagramRelDataFlow,
	}}
	operation := types.EvidenceItem{
		ID: "bus-write", Producer: types.EvidenceProducerExplorerEmitEvidence, Kind: types.EvidenceRelationship,
		Subject: "o.busCtx.EvidenceItems", Predicate: "assigns", Object: "output.EvidenceItems",
		Source: "src/pipeline.go", LineStart: 20, AnchorKind: types.AnchorAssignment,
		GroundingStatus: types.GroundingGrounded, Snippet: "o.busCtx.EvidenceItems = output.EvidenceItems",
		OwnerIdentity: "Orchestrator.applyStageOutput",
		DeclaredIdentityBindings: []types.EvidenceDeclaredIdentityBinding{{
			Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
		}},
	}
	mut := types.NewMutableState("existing participant candidate")
	mut.AppendEvidence([]types.EvidenceItem{operation})
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: mut}
	raw := diagramParticipantRepairAdditionDeltaJSON(doc, ctx, []DiagramParticipantCoverageMismatch{{
		BlockID: "flow", Participant: "BusContext", Issue: DiagramParticipantCoverageTypedEdgeMissing,
	}}, []types.EvidenceItem{operation}, nil)
	if raw != "" {
		t.Fatalf("an existing canonical tuple must never be re-advertised as action=add: %s", raw)
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
		ID: "bus-write", Producer: types.EvidenceProducerExplorerEmitEvidence, Kind: types.EvidenceRelationship, Subject: "o.busCtx.EvidenceItems",
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

	// The same selected assignment also authorizes its exact binding view in
	// LHS -> RHS direction. Requested-scope filtering must preserve both typed
	// renderings without treating either one as an automatically invented edge.
	doc.Blocks[0].Diagram.Body = "flowchart LR\n B[\"BusContext\"] --> O[\"产生证据\"]"
	doc.Blocks[0].EdgeAnchors[0] = types.DiagramEdgeAnchor{
		FromNode: "B", ToNode: "O", FromIdentity: "o.busCtx.EvidenceItems",
		ToIdentity: "output.EvidenceItems", RelationKind: types.DiagramRelAssignment,
	}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, []types.EvidenceItem{operation}); len(got) != 0 {
		t.Fatalf("selected assignment binding view should retain requested relation authority: %+v", got)
	}
}

func TestDiagramParticipantCoverageCandidateShapePassesRelationAndIdentityGatesTogether(t *testing.T) {
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
		ID: "bus-write", Producer: types.EvidenceProducerExplorerEmitEvidence, Kind: types.EvidenceRelationship, Subject: "o.busCtx.EvidenceItems",
		Predicate: "assigns", Object: "output.EvidenceItems", Source: "src/pipeline.go", LineStart: 20,
		AnchorKind: types.AnchorAssignment, AnchorSymbol: "o.busCtx.EvidenceItems", Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded, Snippet: "o.busCtx.EvidenceItems = output.EvidenceItems",
		OwnerIdentity: "Orchestrator.applyStageOutput",
		DeclaredIdentityBindings: []types.EvidenceDeclaredIdentityBinding{{
			Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
		}},
	}
	evidence := []types.EvidenceItem{operation}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 0 {
		t.Fatalf("candidate's canonical anchor identities must pass relation authority: %+v", got)
	}
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence); len(got) != 0 {
		t.Fatalf("business endpoint label plus canonical anchor identities must pass participant coverage: %+v", got)
	}
	anchor := doc.Blocks[0].EdgeAnchors[0]
	if anchor.FromIdentity != "output.EvidenceItems" || anchor.ToIdentity != "o.busCtx.EvidenceItems" {
		t.Fatalf("cross-gate validation must not retarget canonical identities: %+v", anchor)
	}
}

func TestDiagramParticipantCandidateCarrierAuthorityKeepsCanonicalEndpointIdentity(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
	}}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n B[\"BusContext\"] --> C[\"BuildAgentContext\"]"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "B", ToNode: "C", FromIdentity: "o.busCtx",
		ToIdentity: "ctxbuilder.BuildAgentContext", RelationKind: types.DiagramRelArgumentFlow,
	}}
	argumentFlow := types.EvidenceItem{
		ID: "ctx-arg", Producer: types.EvidenceProducerExplorerEmitEvidence,
		Kind: types.EvidenceRelationship, Subject: "o.busCtx", Predicate: "passes argument",
		Object: "ctxbuilder.BuildAgentContext", Source: "internal/orchestrator/pipeline.go", LineStart: 88,
		AnchorKind: types.AnchorArgument, AnchorSymbol: "ctxbuilder.BuildAgentContext",
		Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
		OwnerIdentity: "Orchestrator.extractStageHasRequiredWork",
		DeclaredIdentityBindings: []types.EvidenceDeclaredIdentityBinding{{
			Binding: "o.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
		}},
	}
	busIdentity := types.EvidenceItem{
		ID: "bus-type", Kind: types.EvidenceMechanism, Subject: "BusContext", AnchorSymbol: "BusContext",
		Source: "internal/types/context.go", LineStart: 20, Scope: types.ScopeLine,
		AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded,
	}
	calleeIdentity := types.EvidenceItem{
		ID: "callee", Kind: types.EvidenceMechanism, Subject: "ctxbuilder.BuildAgentContext",
		AnchorSymbol: "ctxbuilder.BuildAgentContext", Source: "internal/ctxbuilder/context.go", LineStart: 40,
		Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded,
	}
	evidence := []types.EvidenceItem{argumentFlow, busIdentity, calleeIdentity}

	// The context-free validator intentionally retains B1237's strict default:
	// a visible exact identity cannot sign a different technical endpoint.
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 1 ||
		got[0].Issue != diagramEdgeAnchorNodeIdentityConflict {
		t.Fatalf("without the request-scoped candidate the exact display/anchor difference must fail closed: %+v", got)
	}

	ctx := &types.BusContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: rm},
		Mutable:    types.NewMutableState("show BusContext argument flow"),
	}
	ctx.Mutable.AppendEvidence(evidence)
	if got := diagramCallEdgeEvidenceMismatchesWithRequestModel(doc, view, evidence, nil, &rm); len(got) != 0 {
		t.Fatalf("the same typed candidate must authorize only its declared participant display carrier: %+v", got)
	}
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, doc, view, evidence); len(got) != 0 {
		t.Fatalf("post-finalizer validation must consume the same precise carrier authority: %+v", got)
	}
	if hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, view, newPreEmitCheckContext(ctx)); len(hints) != 0 {
		t.Fatalf("pre-emit validation must not forbid the candidate shape it prescribed: %+v", hints)
	}

	wrongParticipant := rm
	wrongParticipant.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{{Identity: "MutableState", Role: types.DiagramParticipantIncidentRequired}},
	}
	if got := diagramCallEdgeEvidenceMismatchesWithRequestModel(doc, view, evidence, nil, &wrongParticipant); len(got) != 1 ||
		got[0].Issue != diagramEdgeAnchorNodeIdentityConflict {
		t.Fatalf("an unrelated requested participant must not widen the display carrier: %+v", got)
	}

	doc.Blocks[0].Diagram.Body = "flowchart LR\n B[\"BusContext\"] --> C[\"BuildAgentContext\"]"
	doc.Blocks[0].EdgeAnchors[0].FromIdentity = "ctxbuilder.BuildAgentContext"
	doc.Blocks[0].EdgeAnchors[0].ToIdentity = "o.busCtx"
	if got := diagramCallEdgeEvidenceMismatchesWithRequestModel(doc, view, evidence, nil, &rm); len(got) == 0 ||
		got[0].Issue != diagramEdgeAnchorNodeIdentityConflict {
		t.Fatalf("a reversed canonical tuple must retain the strict node-identity rejection: %+v", got)
	}
}

func TestDiagramParticipantCandidateCarrierAuthorityAllowsExactParticipantBesideCitableTypeLabel(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired,
	}}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n B[\"BuildAgentContext\"] --> Mutable[\"Mutable<br/>MutableState\"]"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "B", ToNode: "Mutable", FromIdentity: "BuildAgentContext",
		ToIdentity: "bus.Mutable.Objective", RelationKind: types.DiagramRelCall,
	}}
	call := diagramEvidenceTestCall("BuildAgentContext", "bus.Mutable.Objective")
	call.DeclaredIdentityBindings = []types.EvidenceDeclaredIdentityBinding{{
		Binding: "bus.Mutable", Type: "*MutableState", Owner: "BusContext",
	}}
	evidence := []types.EvidenceItem{
		call,
		{
			ID: "builder", Kind: types.EvidenceMechanism, Subject: "BuildAgentContext",
			AnchorSymbol: "BuildAgentContext", Source: "internal/context/builder.go", LineStart: 17,
			Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded,
		},
		{
			ID: "mutable-type", Kind: types.EvidenceMechanism, Subject: "MutableState",
			AnchorSymbol: "MutableState", Source: "internal/types/context.go", LineStart: 113,
			Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded,
		},
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, evidence); len(got) != 1 ||
		got[0].Issue != diagramEdgeAnchorNodeIdentityConflict {
		t.Fatalf("context-free validation must retain the strict exact-type conflict: %+v", got)
	}
	ctx := &types.BusContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: rm},
		Mutable:    types.NewMutableState("show Mutable's typed incident call"),
	}
	ctx.Mutable.AppendEvidence(evidence)
	if got := diagramCallEdgeEvidenceMismatchesWithRequestModel(doc, view, evidence, nil, &rm); len(got) != 0 {
		t.Fatalf("the exact candidate must authorize its participant node while retaining the technical anchor: %+v", got)
	}
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, doc, view, evidence); len(got) != 0 {
		t.Fatalf("post-finalizer relation validation rejected the prescribed participant carrier: %+v", got)
	}
	if got := DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, doc, view, evidence); len(got) != 0 {
		t.Fatalf("participant and relation gates must accept the same typed candidate shape: %+v", got)
	}
	if hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, view, newPreEmitCheckContext(ctx)); len(hints) != 0 {
		t.Fatalf("pre-emit validation rejected the same candidate shape it prescribes: %+v", hints)
	}

	// An arbitrary alias cannot borrow the request-scoped participant carrier.
	doc.Blocks[0].Diagram.Body = "flowchart LR\n B[\"BuildAgentContext\"] --> M[\"MutableState\"]"
	doc.Blocks[0].EdgeAnchors[0].ToNode = "M"
	if got := diagramCallEdgeEvidenceMismatchesWithRequestModel(doc, view, evidence, nil, &rm); len(got) != 1 ||
		got[0].Issue != diagramEdgeAnchorNodeIdentityConflict {
		t.Fatalf("a type-only node without the exact participant identity must remain rejected: %+v", got)
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
		ID: "bus-write", Producer: types.EvidenceProducerExplorerEmitEvidence, Kind: types.EvidenceRelationship, Subject: "o.busCtx.EvidenceItems",
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

func TestDiagramParticipantCoverageDoesNotJoinDisconnectedBusinessNodeToTechnicalEdge(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
	}}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n O[\"output.EvidenceItems\"] --> F[\"ActiveAgent\"]\n B[\"BusContext\"]"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "O", ToNode: "F", FromIdentity: "output.EvidenceItems",
		ToIdentity: "o.busCtx.EvidenceItems", RelationKind: types.DiagramRelDataFlow,
	}}
	operation := types.EvidenceItem{
		ID: "bus-write", Producer: types.EvidenceProducerExplorerEmitEvidence, Kind: types.EvidenceRelationship, Subject: "o.busCtx.EvidenceItems",
		Predicate: "assigns", Object: "output.EvidenceItems", Source: "src/pipeline.go", LineStart: 20,
		AnchorKind: types.AnchorAssignment, AnchorSymbol: "o.busCtx.EvidenceItems", Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded, Snippet: "o.busCtx.EvidenceItems = output.EvidenceItems",
		OwnerIdentity: "Orchestrator.applyStageOutput",
		DeclaredIdentityBindings: []types.EvidenceDeclaredIdentityBinding{{
			Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
		}},
	}
	got := DiagramParticipantCoverageMismatches(doc, view, rm, []types.EvidenceItem{operation})
	if len(got) != 1 || got[0].Issue != DiagramParticipantCoverageTypedEdgeMissing {
		t.Fatalf("a disconnected business node must not borrow incidence from a technical edge elsewhere: %+v", got)
	}

	doc.Blocks[0].Diagram.Body = "flowchart LR\n subgraph B [\"BusContext\"]\n  O[\"output.EvidenceItems\"] --> F[\"ActiveAgent\"]\n end"
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, []types.EvidenceItem{operation}); len(got) != 0 {
		t.Fatalf("an exact technical endpoint inside the visible participant group should pass: %+v", got)
	}
}

func TestPreCheckDiagramParticipantCoverageMapsBoundedTypedCandidatesPerParticipant(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.Language = "zh-CN"
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
	}}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n B[\"BusContext\"]"
	doc.Blocks[0].EdgeAnchors = nil
	operation := types.EvidenceItem{
		ID: "bus-write", Producer: types.EvidenceProducerExplorerEmitEvidence, Kind: types.EvidenceRelationship, Subject: "o.busCtx.EvidenceItems",
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
		`visible_arrow_label:"数据写入"`,
		`from_identity:"output.EvidenceItems"`,
		`to_identity:"o.busCtx.EvidenceItems"`,
		`participant_endpoint_side:"to"`,
		`participant_node_id:"BusContext"`,
		`participant_node_side:"to"`,
		`existing_visible_participant_endpoint_ids["BusContext"]=["B"]`,
		"reuse one of those already-authored node/subgraph IDs",
		`technical_endpoint_identity_stays_in_edge_anchor:true`,
		`edge_anchor_identity_fields:{from_identity:"output.EvidenceItems",to_identity:"o.busCtx.EvidenceItems",relation_kind:"data_flow"}`,
		`edge_action:"reuse_one_existing_typed_candidate_as_one_edge_and_map_only_its_declared_participant_endpoint_side_to_the_exact_participant_node_id_without_adding_a_bridge_edge"`,
		`identity_action:"use_the_exact_participant_as_that_edge_endpoint_node_id_with_a_business_label_or_group_the_exact_technical_endpoint_inside_it"`,
		`boundary_action:"recompute_from_the_complete_requested_relation_authority_omit_only_when_complete_otherwise_retain_exactly_one_unproven_requested_relation_boundary"`,
		"never replace those exact identities with the broader participant name",
		"not a requirement to render every candidate",
		"does not create an edge",
	} {
		if !strings.Contains(hints[0].ExpectedShape, want) {
			t.Fatalf("candidate-map hint missing %q: %s", want, hints[0].ExpectedShape)
		}
	}
	repair := emitFixHintsRepair(hints)
	if repair == nil || repair.Metadata == nil {
		t.Fatal("participant mismatch must publish compact retry metadata")
	}
	raw := repair.Metadata[types.ToolRepairMetaDiagramParticipantRepairDeltaJSON]
	var delta diagramParticipantRepairDelta
	if err := json.Unmarshal([]byte(raw), &delta); err != nil {
		t.Fatalf("compact participant delta must be valid JSON: %v raw=%s", err, raw)
	}
	if delta.Version != 1 || len(delta.Mismatches) != 1 ||
		delta.Mismatches[0].Participant != "BusContext" ||
		delta.Mismatches[0].Issue != DiagramParticipantCoverageTypedEdgeMissing {
		t.Fatalf("compact delta lost exact mismatch identity: %+v", delta)
	}
	if strings.Count(delta.Candidates, "typed_candidate[BusContext]") != 1 ||
		!strings.Contains(delta.Candidates, `from_identity:"output.EvidenceItems"`) ||
		!strings.Contains(delta.Candidates, `existing_visible_participant_endpoint_ids["BusContext"]=["B"]`) {
		t.Fatalf("patch delta must carry one bounded executable candidate: %+v", delta)
	}
	if strings.Contains(raw, "For every typed incident_required participant") || len(raw) >= len(hints[0].ExpectedShape) {
		t.Fatalf("patch delta must not repeat the full participant handbook: delta=%d full=%d raw=%s",
			len(raw), len(hints[0].ExpectedShape), raw)
	}
	relationRaw := repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]
	var relationDelta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(relationRaw), &relationDelta); err != nil {
		t.Fatalf("participant-only retry must publish a machine-readable addition capability: %v raw=%s", err, relationRaw)
	}
	if len(relationDelta.Failures) != 0 || !relationDelta.PreserveUnlistedEdges || len(relationDelta.AllowedAdditions) != 1 {
		t.Fatalf("participant-only retry must publish exactly its typed candidate without inventing a failed edge: %+v", relationDelta)
	}
	addition := relationDelta.AllowedAdditions[0]
	if addition.AdditionRef != "" || addition.BlockID != "flow" || addition.RelationKind != types.DiagramRelDataFlow ||
		addition.FromIdentity != "output.EvidenceItems" || addition.ToIdentity != "o.busCtx.EvidenceItems" || addition.Source != "src/pipeline.go:20" {
		t.Fatalf("participant addition capability drifted from the same typed candidate provider: %+v", addition)
	}
}

func TestDiagramParticipantExistingVisibleEndpointGuidancePreservesModelAuthoredAlias(t *testing.T) {
	rm, _, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
	}}
	doc.Blocks[0].Diagram.Body = strings.Join([]string{
		"flowchart TD",
		`  subgraph BC["BusContext"]`,
		`    MS["MutableState"]`,
		"  end",
	}, "\n")
	got := diagramParticipantExistingVisibleEndpointGuidance(doc, rm, []DiagramParticipantCoverageMismatch{{
		BlockID: doc.Blocks[0].ID, Participant: "BusContext", Issue: DiagramParticipantCoverageTypedEdgeMissing,
	}})
	if got != `existing_visible_participant_endpoint_ids["BusContext"]=["BC"]` {
		t.Fatalf("repair guidance must preserve the exact already-authored participant alias, got %q", got)
	}
	if strings.Contains(got, "BuildAgentContext") || strings.Contains(got, "argument_flow") {
		t.Fatalf("visible endpoint reuse guidance must not create or select a relation: %q", got)
	}
}

func TestParticipantOnlyAdditionCapabilityFailsOpenWhenDiagramTargetIsAmbiguous(t *testing.T) {
	rm, view, doc, _ := diagramParticipantCoverageFixture()
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
	}}
	view.DiagramParticipantObligations = append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)
	doc.Blocks[0].Diagram.Body = "flowchart LR\n B[\"BusContext\"]"
	doc.Blocks[0].EdgeAnchors = nil
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID: "flow-2", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n X[Other]"},
	})
	operation := types.EvidenceItem{
		ID: "bus-write", Producer: types.EvidenceProducerExplorerEmitEvidence, Kind: types.EvidenceRelationship,
		Subject: "o.busCtx.EvidenceItems", Predicate: "assigns", Object: "output.EvidenceItems",
		Source: "src/pipeline.go", LineStart: 20, AnchorKind: types.AnchorAssignment,
		GroundingStatus: types.GroundingGrounded, Snippet: "o.busCtx.EvidenceItems = output.EvidenceItems",
		OwnerIdentity: "Orchestrator.applyStageOutput",
		DeclaredIdentityBindings: []types.EvidenceDeclaredIdentityBinding{{
			Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
		}},
	}
	mut := types.NewMutableState("ambiguous participant addition target")
	mut.AppendEvidence([]types.EvidenceItem{operation})
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: mut}
	raw := diagramParticipantRepairAdditionDeltaJSON(doc, ctx, []DiagramParticipantCoverageMismatch{{
		Participant: "BusContext", Issue: DiagramParticipantCoverageTypedEdgeMissing,
	}}, []types.EvidenceItem{operation}, nil)
	if raw != "" {
		t.Fatalf("ambiguous diagram target must not mint an executable addition ref: %s", raw)
	}
	oneDiagram := *doc
	oneDiagram.Blocks = append([]types.AnswerBlock(nil), doc.Blocks[:1]...)
	raw = diagramParticipantRepairAdditionDeltaJSON(&oneDiagram, ctx, []DiagramParticipantCoverageMismatch{{
		Participant: "BusContext", Issue: DiagramParticipantCoverageIdentityMissing,
	}}, []types.EvidenceItem{operation}, nil)
	if raw != "" {
		t.Fatalf("identity-only repair may need a node/group edit and must not be narrowed to add-only: %s", raw)
	}
}

func TestDiagramRelationRepairDeltaCarriesOnlyFailedEdgesAndBoundedLocalAlternatives(t *testing.T) {
	rm, _, doc, _ := diagramParticipantCoverageFixture()
	rm.Language = "zh-CN"
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
	}
	operation := types.EvidenceItem{
		ID: "bus-write", Producer: types.EvidenceProducerExplorerEmitEvidence,
		Kind: types.EvidenceRelationship, Subject: "o.busCtx.EvidenceItems",
		Predicate: "assigns", Object: "output.EvidenceItems",
		Source: "src/pipeline.go", LineStart: 20, AnchorKind: types.AnchorAssignment,
		AnchorSymbol: "o.busCtx.EvidenceItems", Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
		Snippet:         "o.busCtx.EvidenceItems = output.EvidenceItems",
		OwnerIdentity:   "Orchestrator.applyStageOutput",
		DeclaredIdentityBindings: []types.EvidenceDeclaredIdentityBinding{{
			Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
		}},
	}
	mismatches := []DiagramCallEdgeEvidenceMismatch{
		{BlockID: "flow", Issue: diagramDataFlowEdgeIssueNoEvidence,
			FromNode: "BC", ToNode: "A", FromSymbol: "BusContext", ToSymbol: "Analyzer",
			Relation: types.DiagramRelDataFlow},
		{BlockID: "flow", Issue: diagramDataFlowEdgeIssueNoEvidence,
			FromNode: "BC", ToNode: "X", FromSymbol: "BusContext", ToSymbol: "Extractor",
			Relation: types.DiagramRelDataFlow},
	}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	raw := diagramRelationRepairDeltaJSON(doc, ctx, mismatches, []types.EvidenceItem{operation}, nil)
	var delta diagramRelationRepairDelta
	if err := json.Unmarshal([]byte(raw), &delta); err != nil {
		t.Fatalf("relation repair delta must be valid JSON: %v raw=%s", err, raw)
	}
	if delta.Version != 1 || !delta.PreserveUnlistedEdges || len(delta.Failures) != 2 {
		t.Fatalf("relation delta lost exact local failure set: %+v", delta)
	}
	for _, failure := range delta.Failures {
		if failure.FailureRef == "" {
			t.Fatalf("every production failure row must carry a live stable selector: %+v", delta.Failures)
		}
	}
	if len(delta.AllowedAdditions) == 0 {
		t.Fatalf("relation delta must carry machine-readable candidates from the same typed provider: %+v", delta)
	}
	for _, candidate := range delta.AllowedAdditions {
		if candidate.BlockID != "flow" || candidate.FromIdentity == "" || candidate.ToIdentity == "" ||
			!candidate.RelationKind.IsValid() || candidate.Source == "" {
			t.Fatalf("allowed addition must be a complete typed tuple scoped to the failed block: %+v", candidate)
		}
	}
	for _, want := range []string{
		`"from_node":"BC"`, `"to_node":"A"`, `"to_node":"X"`,
		`"allowed_additions"`, `"block_id":"flow"`,
		`typed_candidate[BusContext][1]`, `candidate_scope:\"local_operation_only\"`,
		`retain_participant_boundary:true`, `from_identity:\"output.EvidenceItems\"`,
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("relation delta missing %q: %s", want, raw)
		}
	}
	for _, forbidden := range []string{
		"For every typed incident_required participant", "Copy-ready verified component fragments",
		"Verified component fragment",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("relation delta leaked full handbook section %q: %s", forbidden, raw)
		}
	}
	if len(raw) > 6000 {
		t.Fatalf("relation delta must stay bounded, got %d bytes", len(raw))
	}
}

func TestDiagramRelationRepairAllowedAdditionsCarriesCompleteTypedStageSpine(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true},
	}
	stage := func(name, agent string) stageauthority.StageRow {
		return stageauthority.StageRow{StageValue: name, AgentValue: agent}
	}
	precedence := []stageauthority.PrecedenceRelation{
		{From: stage("analyze", "analyzer"), To: stage("explore", "explorer"), SourceFile: "internal/types/enums.go", LineStart: 120, LineEnd: 121},
		{From: stage("explore", "explorer"), To: stage("extract", "extractor"), SourceFile: "internal/types/enums.go", LineStart: 121, LineEnd: 122},
		{From: stage("extract", "extractor"), To: stage("finalize", "finalizer"), SourceFile: "internal/types/enums.go", LineStart: 122, LineEnd: 123},
	}
	got := diagramRelationRepairAllowedAdditions(nil, rm, nil, precedence, []string{"diagram-1"}, 8)
	if len(got) != 3 {
		t.Fatalf("complete checkout-verified stage spine must remain selectable in one local repair, got %+v", got)
	}
	for i, want := range []struct{ from, to string }{
		{"analyzer", "explorer"}, {"explorer", "extractor"}, {"extractor", "finalizer"},
	} {
		if got[i].BlockID != "diagram-1" || got[i].RelationKind != types.DiagramRelPrecedence ||
			got[i].FromIdentity != want.from || got[i].ToIdentity != want.to || got[i].Source == "" {
			t.Fatalf("stage candidate %d drifted: got=%+v want=%s->%s", i, got[i], want.from, want.to)
		}
	}
}

func TestDiagramRelationRepairCandidateCarriesTypedParticipantEndpointPermissions(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "diagram-1", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n  BUS[BusContext]\n  MUT[Mutable]\n"},
	}}}
	rm := types.RequestModel{DiagramHint: &types.DiagramHint{Participants: []types.DiagramParticipantHint{
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}}}
	row := types.AnswerDiagramRelationRepairCandidate{
		BlockID: "diagram-1", RelationKind: types.DiagramRelDataFlow,
		FromIdentity: "bus.Mutable", ToIdentity: "AgentContext.Mutable", Source: "typed",
	}
	from := diagramParticipantTypedIncidentCandidate{
		participant: "BusContext", relation: types.DiagramRelDataFlow,
		from: row.FromIdentity, to: row.ToIdentity, participantEndpointSide: "from",
	}
	to := diagramParticipantTypedIncidentCandidate{
		participant: "Mutable", relation: types.DiagramRelDataFlow,
		from: row.FromIdentity, to: row.ToIdentity, participantEndpointSide: "to",
	}
	left, right := row, row
	bindDiagramRelationRepairCandidateParticipantNodes(&left, doc, rm, from)
	bindDiagramRelationRepairCandidateParticipantNodes(&right, doc, rm, to)
	mergeDiagramRelationRepairCandidateNodeIDs(&left, right)
	if !atomicDiagramNodeIDListed("BUS", left.FromNodeIDs) ||
		!atomicDiagramNodeIDListed("BusContext", left.FromNodeIDs) ||
		!atomicDiagramNodeIDListed("MUT", left.ToNodeIDs) ||
		!atomicDiagramNodeIDListed("Mutable", left.ToNodeIDs) {
		t.Fatalf("typed participant-side permissions did not survive tuple merge: %+v", left)
	}
	if atomicDiagramNodeIDListed("MUT", left.FromNodeIDs) || atomicDiagramNodeIDListed("BUS", left.ToNodeIDs) {
		t.Fatalf("participant permission crossed its typed endpoint side: %+v", left)
	}
}

func TestDiagramRelationRepairCandidatePublishesStableSafeTechnicalEndpointIDs(t *testing.T) {
	row := types.AnswerDiagramRelationRepairCandidate{
		BlockID: "diagram-1", RelationKind: types.DiagramRelArgumentFlow,
		FromIdentity: "o.busCtx", ToIdentity: "ctxbuilder.BuildAgentContext", Source: "typed",
	}
	bindDiagramRelationRepairCandidateTechnicalNodeIDs(&row)
	fromAlias := mermaidcompat.CanonicalFlowchartNodeID(row.FromIdentity)
	toAlias := mermaidcompat.CanonicalFlowchartNodeID(row.ToIdentity)
	if !atomicDiagramNodeIDListed(fromAlias, row.FromNodeIDs) ||
		!atomicDiagramNodeIDListed(toAlias, row.ToNodeIDs) {
		t.Fatalf("typed technical aliases were not published on their exact sides: %+v", row)
	}
	if atomicDiagramNodeIDListed(toAlias, row.FromNodeIDs) ||
		atomicDiagramNodeIDListed(fromAlias, row.ToNodeIDs) {
		t.Fatalf("typed technical aliases crossed endpoint sides: %+v", row)
	}
}

func TestDiagramRelationRepairDeltaPublishesAdditionsOnlyForDiagramCarriers(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "diagram-1", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n    A->>B: visible\n",
		}, EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", FromIdentity: "analyzer", ToIdentity: "explorer", RelationKind: types.DiagramRelPrecedence,
		}}},
		{ID: "ol1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{ID: "row", Label: "visible list row"}},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "A", ToNode: "B", FromIdentity: "analyzer", ToIdentity: "explorer", RelationKind: types.DiagramRelPrecedence,
			}}},
	}}
	stage := func(name, agent string) stageauthority.StageRow {
		return stageauthority.StageRow{StageValue: name, AgentValue: agent}
	}
	precedence := []stageauthority.PrecedenceRelation{{
		From: stage("analyze", "analyzer"), To: stage("explore", "explorer"),
		SourceFile: "internal/types/enums.go", LineStart: 120, LineEnd: 121,
	}}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true},
	}}}
	raw := diagramRelationRepairDeltaJSON(doc, ctx, []DiagramCallEdgeEvidenceMismatch{
		{BlockID: "diagram-1", Issue: diagramSemanticRelationIssueNoEvidence, FromNode: "A", ToNode: "B", FromSymbol: "analyzer", ToSymbol: "explorer", Relation: types.DiagramRelPrecedence},
		{BlockID: "ol1", Issue: diagramSemanticRelationIssueNoEvidence, FromNode: "A", ToNode: "B", FromSymbol: "analyzer", ToSymbol: "explorer", Relation: types.DiagramRelPrecedence},
	}, nil, precedence)
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(raw), &delta); err != nil {
		t.Fatalf("mixed carrier relation delta must remain valid: %v raw=%s", err, raw)
	}
	if len(delta.Failures) != 2 {
		t.Fatalf("non-diagram failure must remain visible for metadata cleanup: %+v", delta)
	}
	for _, failure := range delta.Failures {
		if failure.BlockID == "ol1" && (failure.TargetCarrier != types.AnswerDiagramRelationRepairCarrierPriorAnchorMetadata ||
			!failure.AllowsAction("remove") || failure.AllowsAction("replace")) {
			t.Fatalf("non-diagram failure gained a visible-body capability: %+v", failure)
		}
	}
	if len(delta.AllowedAdditions) == 0 {
		t.Fatalf("diagram carrier should retain typed addition permissions: %+v", delta)
	}
	for _, candidate := range delta.AllowedAdditions {
		if candidate.BlockID != "diagram-1" {
			t.Fatalf("non-diagram block must never receive an unexecutable addition_ref: %+v", delta.AllowedAdditions)
		}
	}
}

func TestDiagramRelationRepairAllowedAdditionsAcceptsEquivalentExactTypedReceiptDialect(t *testing.T) {
	allowed := []types.AnswerDiagramRelationRepairCandidate{{
		BlockID: "diagram-1", RelationKind: types.DiagramRelPrecedence,
		FromIdentity: "analyzer", ToIdentity: "explorer", Source: "internal/types/enums.go:120-121",
	}}
	receipts := []types.DiagramEdgeAnchor{{
		FromNode: "n1", ToNode: "n2", FromIdentity: "Analyzer", ToIdentity: "Explorer",
		RelationKind: types.DiagramRelPrecedence,
	}}
	got := diagramRelationRepairAllowedAdditionsWithTypedReceipts(allowed, receipts, 8)
	if len(got) != 2 || got[1].BlockID != "diagram-1" ||
		got[1].FromIdentity != "Analyzer" || got[1].ToIdentity != "Explorer" ||
		got[1].RelationKind != types.DiagramRelPrecedence || got[1].Source != allowed[0].Source {
		t.Fatalf("lease must admit the exact equivalent identity tuple that its typed-recipe normalizer can stamp: %+v", got)
	}

	nonEquivalent := []types.DiagramEdgeAnchor{{
		FromIdentity: "Unrelated.Run", ToIdentity: "Other.Run", RelationKind: types.DiagramRelPrecedence,
	}}
	if unchanged := diagramRelationRepairAllowedAdditionsWithTypedReceipts(allowed, nonEquivalent, 8); len(unchanged) != 1 {
		t.Fatalf("an unrelated receipt must not broaden the allowed relation set: %+v", unchanged)
	}
}

func TestDiagramParticipantReaderArrowLabelCoversEveryEdgeRelationWithoutRawEnums(t *testing.T) {
	for _, relation := range types.AllDiagramRelationKinds() {
		zh := diagramParticipantReaderArrowLabel(relation, "zh-CN")
		en := diagramParticipantReaderArrowLabel(relation, "en")
		if relation == types.DiagramRelContain {
			if zh != "" || en != "" {
				t.Fatalf("containment is a no-arrow grouping facet, got zh=%q en=%q", zh, en)
			}
			continue
		}
		if zh == "" || en == "" {
			t.Fatalf("edge relation %q lacks reader labels: zh=%q en=%q", relation, zh, en)
		}
		if zh == string(relation) || en == string(relation) {
			t.Fatalf("reader label for %q repeated the raw relation enum: zh=%q en=%q", relation, zh, en)
		}
		if strings.Contains(zh, "_") || strings.Contains(en, "_") {
			t.Fatalf("reader label for %q leaked a snake_case control token: zh=%q en=%q", relation, zh, en)
		}
	}
	if got := diagramParticipantReaderArrowLabel(types.DiagramRelUnknown, "zh-CN"); got != "" {
		t.Fatalf("unknown relation must not gain reader wording: %q", got)
	}
}

func TestDiagramParticipantCoverageRepairActionsKeepTypedLanesSeparate(t *testing.T) {
	mismatches := []DiagramParticipantCoverageMismatch{
		{Participant: "BusContext", Issue: DiagramParticipantCoverageTypedEdgeMissing},
		{Participant: "Mutable", Issue: DiagramParticipantCoverageIdentityMissing},
		{Participant: "analyzer", Issue: DiagramParticipantCoverageStaleBoundary},
		{Participant: "UnprovenWorker", Issue: DiagramParticipantCoverageMissingBoundary},
		{Participant: "DetachedStore", Issue: DiagramParticipantCoverageBoundaryConnected},
		{Participant: "Analyzer", Issue: DiagramParticipantCoverageEndpointRetargeted},
	}
	got := diagramParticipantCoverageRepairActions(mismatches)
	for _, want := range []string{
		`repair_action["BusContext"]={issue:"available_typed_incident_edge_not_rendered",edge_action:"reuse_one_existing_typed_candidate_as_one_edge_and_map_only_its_declared_participant_endpoint_side_to_the_exact_participant_node_id_without_adding_a_bridge_edge"`,
		`repair_action["Mutable"]={issue:"required_participant_identity_not_visible",edge_action:"retain_an_already_rendered_valid_candidate_or_select_one_existing_typed_candidate",identity_action:"add_only_the_missing_visible_participant_label_or_group_without_retargeting_canonical_endpoints",boundary_action:"recompute_from_the_complete_requested_relation_authority_omit_only_when_complete_otherwise_retain_exactly_one_unproven_requested_relation_boundary"}`,
		`repair_action["analyzer"]={issue:"stale_boundary_for_connected_participant",edge_action:"retain_existing_typed_incident_edge",identity_action:"retain_existing_visible_participant_identity",boundary_action:"remove_stale_boundary"}`,
		`repair_action["UnprovenWorker"]={issue:"missing_unproven_boundary",edge_action:"none_for_missing_requested_relation_keep_independent_typed_local_facts_if_any",identity_action:"ensure_exact_visible_participant_without_directed_incident_edge_and_preserve_any_existing_grounded_no_arrow_grouping",boundary_action:"add_exactly_one_unproven_boundary"}`,
		`repair_action["DetachedStore"]={issue:"unproven_boundary_has_visible_incident_edge",edge_action:"move_existing_typed_edge_to_its_exact_technical_endpoint_and_keep_participant_out_of_that_directed_edge",identity_action:"retain_exact_visible_participant_and_preserve_any_existing_grounded_no_arrow_grouping",boundary_action:"retain_exactly_one_unproven_boundary"}`,
		`repair_action["Analyzer"]={issue:"participant_visible_on_nonincident_endpoint",edge_action:"keep_the_existing_typed_edge_and_anchor_direction_but_remove_the_requested_participant_identity_from_the_nonincident_endpoint",identity_action:"map_each_requested_participant_only_to_an_endpoint_that_is_typed_incident_to_it_or_keep_it_in_an_honest_unproven_group",boundary_action:"recompute_from_the_corrected_visible_incidence_without_inventing_an_edge"}`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed participant repair actions missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"add_edge_automatically", "retarget_to_participant", "infer_relation", "disconnected_participant"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("repair action must preserve model edge authorship; found %q in %s", forbidden, got)
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

func TestDiagramParticipantCoverageDoesNotEnterRootCauseTrace(t *testing.T) {
	rm, view, doc, evidence := diagramParticipantCoverageFixture()
	rm.Intent = types.IntentRootCause
	rm.Scenario = types.ScenarioRootCause
	if got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence); len(got) != 0 {
		t.Fatalf("Trace diagrams keep independent causal authority: %+v", got)
	}
}

func TestDiagramParticipantCoverageEntersSourceCallChain(t *testing.T) {
	rm, view, doc, evidence := diagramParticipantCoverageFixture()
	rm.Intent = types.IntentTrace
	rm.PredicateAxis = types.AxisCall
	view.Family = types.QFCallChain
	view.RelationAxis = types.AxisCall
	view.DiagramPlan.Kind = types.DiagramSequence
	doc.Blocks[0].Diagram.Kind = types.DiagramSequence
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n participant Analyzer\n participant Explorer\n Analyzer->>Explorer: call"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "Analyzer", ToNode: "Explorer", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelCall,
	}}
	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence)
	if len(got) != 1 || got[0].Participant != "MutableState" || got[0].Issue != DiagramParticipantCoverageMissingBoundary {
		t.Fatalf("source call-chain diagrams must preserve every explicit typed participant or boundary: %+v", got)
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

func TestDiagramParticipantCoveragePreAndPostValidationUseSameLosslessEvidencePool(t *testing.T) {
	participants := []types.DiagramParticipantHint{
		{Identity: "Producer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Consumer", Role: types.DiagramParticipantIncidentRequired},
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: participants},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFGeneric, RelationAxis: types.AxisFlow,
		DiagramPlan:                   &types.DiagramFacetGraph{Kind: types.DiagramFlow, Required: true},
		DiagramParticipantObligations: append([]types.DiagramParticipantHint(nil), participants...),
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart LR\n Producer[\"Producer\"]\n Consumer[\"Consumer\"]"},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{
			{Participant: "Producer", Status: types.DiagramParticipantBoundaryUnproven},
			{Participant: "Consumer", Status: types.DiagramParticipantBoundaryUnproven},
		},
	}}}
	relation := diagramEvidenceTestCall("Producer", "Consumer")
	relation.ID = "lossless-bus-relation"
	mut := types.NewMutableState("participant coverage evidence parity")
	ctx := &types.BusContext{
		Mutable:       mut,
		AnalysisIR:    &types.AnalysisIR{RequestModel: rm},
		EvidenceItems: []types.EvidenceItem{relation},
	}
	pctx := newPreEmitCheckContext(ctx)
	prePool := pctx.evidenceItems()
	want := DiagramParticipantCoverageMismatches(doc, view, rm, prePool)
	if len(want) == 0 {
		t.Fatal("lossless typed relation should make disconnected unproven boundaries stale")
	}

	postPool := DiagramEvidenceForValidation(ctx, doc, view, mut.EmittedEvidence())
	got := DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, doc, view, postPool)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pre/post participant evidence authority diverged:\npre=%+v\npost=%+v\npool=%+v", want, got, postPool)
	}

	// M4 wiring pin: the actual first-pass precheck must report the same
	// contradiction instead of accepting a boundary that post-finalizer rejects.
	if hints := preCheckDiagramParticipantCoverage(doc, view, pctx); len(hints) != 1 {
		t.Fatalf("pre-emit participant gate did not consume the shared pool: %+v", hints)
	}
}
