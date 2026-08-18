package tool

import (
	"path/filepath"
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
	if got := diagramParticipantEndpointConflictGuidance(doc, rm, mismatches); got != "" {
		t.Fatalf("ambiguous body edges must retain generic fail-open guidance, got %s", got)
	}

	doc.Blocks[0].Diagram.Body = "flowchart LR\n W1 --> Mutable"
	doc.Blocks[0].EdgeAnchors[1].FromNode = "W1"
	if got := diagramParticipantEndpointConflictGuidance(doc, rm, mismatches); got != "" {
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
	if got := diagramParticipantTypedIncidentCandidates(rm, rm.DiagramHint.Participants[0], evidence, nil, 3); len(got) != 0 {
		t.Fatalf("a disconnected local fact must not be offered as a requested-relation repair candidate: %v", got)
	}

	doc.Blocks[0].ParticipantBoundaries = nil
	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence)
	if len(got) != 2 || got[0].Issue != DiagramParticipantCoverageMissingBoundary || got[1].Issue != DiagramParticipantCoverageMissingBoundary {
		t.Fatalf("both disconnected requested participants need explicit unproven boundaries: %+v", got)
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
	surfaces := make([][]string, len(participants))
	for i := range participants {
		surfaces[i] = []string{participants[i].Identity}
	}
	argument := func(subject string) types.EvidenceItem {
		return types.EvidenceItem{ID: "arg-" + subject, Kind: types.EvidenceRelationship,
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
	scope := buildFlowParticipantRelationScope(rm, participants, surfaces, evidence, precedence)
	for i, covered := range scope.participantCovered {
		if !covered {
			t.Fatalf("participant %s should join a verified relation component: %+v", participants[i].Identity, scope)
		}
		if !scope.participantRequestScopedCovered[i] {
			t.Fatalf("participant %s should inherit request-scoped authority through the exact carrier bridge: %+v", participants[i].Identity, scope)
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
	if got := diagramParticipantTypedIncidentCandidates(rm, participants[2], evidence, precedence, 3); len(got) != 0 {
		t.Fatalf("a disconnected local carrier edge must not be offered as request-scoped repair authority: %v", got)
	}

	doc.Blocks[0].ParticipantBoundaries = nil
	got := DiagramParticipantCoverageMismatches(doc, view, rm, evidence, precedence...)
	if len(got) != 2 || got[0].Issue != DiagramParticipantCoverageMissingBoundary ||
		got[1].Issue != DiagramParticipantCoverageMissingBoundary {
		t.Fatalf("both local-only participants need explicit requested-relation boundaries: %+v", got)
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
		ID: "bus-write", Kind: types.EvidenceRelationship, Subject: "o.busCtx.EvidenceItems",
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
		`boundary_action:"omit_unproven_boundary"`,
		"never replace those exact identities with the broader participant name",
		"not a requirement to render every candidate",
		"does not create an edge",
	} {
		if !strings.Contains(hints[0].ExpectedShape, want) {
			t.Fatalf("candidate-map hint missing %q: %s", want, hints[0].ExpectedShape)
		}
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
	}
	got := diagramParticipantCoverageRepairActions(mismatches)
	for _, want := range []string{
		`repair_action["BusContext"]={issue:"available_typed_incident_edge_not_rendered",edge_action:"reuse_one_existing_typed_candidate_as_one_edge_and_map_only_its_declared_participant_endpoint_side_to_the_exact_participant_node_id_without_adding_a_bridge_edge"`,
		`repair_action["Mutable"]={issue:"required_participant_identity_not_visible",edge_action:"retain_an_already_rendered_valid_candidate_or_select_one_existing_typed_candidate",identity_action:"add_only_the_missing_visible_participant_label_or_group_without_retargeting_canonical_endpoints",boundary_action:"omit_unproven_boundary"}`,
		`repair_action["analyzer"]={issue:"stale_boundary_for_connected_participant",edge_action:"retain_existing_typed_incident_edge",identity_action:"retain_existing_visible_participant_identity",boundary_action:"remove_stale_boundary"}`,
		`repair_action["UnprovenWorker"]={issue:"missing_unproven_boundary",edge_action:"none_for_missing_requested_relation_keep_independent_typed_local_facts_if_any",identity_action:"ensure_exact_visible_participant_without_directed_incident_edge_and_preserve_any_existing_grounded_no_arrow_grouping",boundary_action:"add_exactly_one_unproven_boundary"}`,
		`repair_action["DetachedStore"]={issue:"unproven_boundary_has_visible_incident_edge",edge_action:"move_existing_typed_edge_to_its_exact_technical_endpoint_and_keep_participant_out_of_that_directed_edge",identity_action:"retain_exact_visible_participant_and_preserve_any_existing_grounded_no_arrow_grouping",boundary_action:"retain_exactly_one_unproven_boundary"}`,
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
