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
		`edge_anchor_identity_fields:{from_identity:"output.EvidenceItems",to_identity:"o.busCtx.EvidenceItems",relation_kind:"data_flow"}`,
		`edge_action:"select_one_existing_typed_candidate_and_render_its_exact_direction"`,
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
		`repair_action["BusContext"]={issue:"available_typed_incident_edge_not_rendered",edge_action:"select_one_existing_typed_candidate_and_render_its_exact_direction"`,
		`repair_action["Mutable"]={issue:"required_participant_identity_not_visible",edge_action:"retain_an_already_rendered_valid_candidate_or_select_one_existing_typed_candidate",identity_action:"add_only_the_missing_visible_participant_label_or_group_without_retargeting_canonical_endpoints",boundary_action:"omit_unproven_boundary"}`,
		`repair_action["analyzer"]={issue:"stale_boundary_for_connected_participant",edge_action:"retain_existing_typed_incident_edge",identity_action:"retain_existing_visible_participant_identity",boundary_action:"remove_stale_boundary"}`,
		`repair_action["UnprovenWorker"]={issue:"missing_unproven_boundary",edge_action:"none_no_typed_candidate_exists",identity_action:"add_exact_visible_disconnected_participant",boundary_action:"add_exactly_one_unproven_boundary"}`,
		`repair_action["DetachedStore"]={issue:"unproven_boundary_has_visible_incident_edge",edge_action:"move_existing_typed_edge_to_its_exact_technical_endpoint_and_keep_participant_disconnected",identity_action:"retain_exact_visible_disconnected_participant_separately",boundary_action:"retain_exactly_one_unproven_boundary"}`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed participant repair actions missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"add_edge_automatically", "retarget_to_participant", "infer_relation"} {
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
