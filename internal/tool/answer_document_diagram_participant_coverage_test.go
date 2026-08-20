package tool

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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
		!scope.effectiveParticipantCovered(0, false) || !scope.effectiveParticipantCovered(1, false) ||
		scope.effectiveParticipantCovered(2, false) || scope.effectiveParticipantCovered(3, false) {
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

func TestDiagramParticipantCoverageCompactRepairRetainsLocalCandidateAndRequestedBoundary(t *testing.T) {
	participants := []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	rm := types.RequestModel{Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
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
		"typed_candidate[Mutable][1]",
		`candidate_scope:"local_operation_only"`,
		`requested_relation_closure:"unproven"`,
		`retain_participant_boundary:true`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compact repair must retain the failed requested candidate and independently grounded local relation; missing %q:\n%s", want, got)
		}
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
	if !scope.effectiveParticipantCovered(0, false) || !scope.effectiveParticipantCovered(1, false) ||
		scope.effectiveParticipantCovered(2, false) {
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
		!strings.Contains(delta.Candidates, `from_identity:"output.EvidenceItems"`) {
		t.Fatalf("patch delta must carry one bounded executable candidate: %+v", delta)
	}
	if strings.Contains(raw, "For every typed incident_required participant") || len(raw) >= len(hints[0].ExpectedShape) {
		t.Fatalf("patch delta must not repeat the full participant handbook: delta=%d full=%d raw=%s",
			len(raw), len(hints[0].ExpectedShape), raw)
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
