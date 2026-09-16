package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1705CompiledDiagramView(kind types.DiagramKind, axis types.PredicateAxis, required bool) *types.AnswerSemanticView {
	return types.BuildAnswerSemanticView(&types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioGeneric, PredicateAxis: axis,
	}}, &types.AnswerSurfacePlan{Diagram: &types.DiagramContract{
		Required: required, PreferredKinds: []types.DiagramKind{kind},
	}})
}

func b1705DiagramDocument(kind types.DiagramKind, relation types.DiagramRelationKind, edge bool) *types.AnswerDocumentV2 {
	body := "flowchart LR\n  A[\"Origin\"]\n  B[\"Destination\"]"
	if kind == types.DiagramSequence {
		body = "sequenceDiagram\n  participant A as Origin\n  participant B as Destination"
	}
	if edge {
		if kind == types.DiagramSequence {
			body += "\n  A->>B: 下一环节"
		} else {
			body += "\n  A --> B"
		}
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "diagram", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: kind, Language: "mermaid", Body: body},
	}}}
	if edge {
		doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: relation}}
	}
	return doc
}

func TestB1705CompiledDiagramContractAndReviewerDoNotInventRelations(t *testing.T) {
	for _, tc := range []struct {
		name     string
		kind     types.DiagramKind
		relation types.DiagramRelationKind
		axis     types.PredicateAxis
	}{
		{"flow precedence", types.DiagramFlow, types.DiagramRelPrecedence, types.AxisFlow},
		{"sequence callback", types.DiagramSequence, types.DiagramRelCallback, types.AxisCall},
		{"sequence registration", types.DiagramSequence, types.DiagramRelRegister, types.AxisRegister},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := b1705CompiledDiagramView(tc.kind, tc.axis, true)
			doc := b1705DiagramDocument(tc.kind, tc.relation, true)
			before, _ := json.Marshal(doc)
			if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
				t.Errorf("grounded display must not acquire an unrelated semantic minimum: %+v", vs)
			}
			input := BuildSemanticQualityInput("question", "summary", "body", doc, view, nil)
			if input.DiagramContract == nil || len(input.DiagramContract.Edges) != 0 {
				t.Errorf("reviewer received invented semantic requirements: %+v", input.DiagramContract)
			}
			prompt := renderSemanticQualityUserMessage(input)
			if !strings.Contains(prompt, "Only listed relation rows prescribe semantic minimums.") || strings.Contains(prompt, "min_expected=") {
				t.Errorf("reviewer teaching restored an implicit minimum:\n%s", prompt)
			}
			evidence := types.EvidenceItem{
				Kind: types.EvidenceRelationship, Subject: "Origin", Object: "Destination",
				Source: "pipeline.go", LineStart: 7, LineEnd: 8, Scope: types.ScopeLineRange,
				GroundingStatus: types.GroundingGrounded,
			}
			switch tc.relation {
			case types.DiagramRelPrecedence:
				evidence.AnchorKind = types.AnchorPrecedence
			case types.DiagramRelCallback:
				evidence.AnchorKind = types.AnchorCallback
			case types.DiagramRelRegister:
				evidence.Kind, evidence.AnchorKind = types.EvidenceRegistration, types.AnchorDefinition
			}
			if got := tool.DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{evidence}); len(got) != 0 {
				t.Errorf("exact evidence must still authorize its own non-call relation: %+v", got)
			}
			reversed := evidence
			reversed.Subject, reversed.Object = reversed.Object, reversed.Subject
			if got := tool.DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{reversed}); len(got) == 0 {
				t.Error("reversed evidence authorized the authored direction")
			}
			// Removing an invented minimum must not authorize the actual edge.
			if got := tool.DiagramCallEdgeEvidenceMismatches(doc, view, nil); len(got) == 0 {
				t.Error("unsupported visible edge escaped the independent typed evidence gate")
			}
			after, _ := json.Marshal(doc)
			if string(before) != string(after) {
				t.Fatal("validation or reviewer projection changed the model document")
			}
		})
	}
}

func TestB1705CompiledDiagramKeepsStructuralAndUnprovenContracts(t *testing.T) {
	for _, kind := range []types.DiagramKind{types.DiagramFlow, types.DiagramSequence, types.DiagramCallDAG} {
		t.Run(string(kind), func(t *testing.T) {
			view := b1705CompiledDiagramView(kind, types.AxisFlow, true)
			doc := b1705DiagramDocument(kind, types.DiagramRelUnknown, false)
			if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 1 || vs[0].Kind != types.ViolRequiredDiagramEdgeAbsent {
				t.Fatalf("required empty graph lost its structural obligation: %+v", vs)
			}
			mut := types.NewMutableState("question")
			mut.EvidenceClosure().AppendCompletionCaveat(types.CompletionCaveat{Lane: types.DowngradeLaneFlowOperationCarrier})
			bus := &types.BusContext{Mutable: mut}
			if vs := validateDiagramEdgeSupportWithRuntimeContext(doc, view, bus); len(vs) != 0 {
				t.Fatalf("typed unproven exit now demands invented edges: %+v", vs)
			}
			if vs := validateDiagramEdgeSupportWithRuntimeContext(&types.AnswerDocumentV2{}, view, bus); len(vs) != 1 || vs[0].Kind != types.ViolDiagramEdgeUnsupported {
				t.Fatalf("typed unproven exit incorrectly waived the diagram itself: %+v", vs)
			}
			optional := b1705CompiledDiagramView(kind, types.AxisFlow, false)
			if optional.DiagramPlan.RequireStructuralEdge {
				t.Fatal("optional compiler manufactured a structural obligation")
			}
			if vs := validateDiagramEdgeSupport(doc, optional); len(vs) != 0 {
				t.Fatalf("optional graph gained a hard edge requirement: %+v", vs)
			}
		})
	}
}

func TestB1705CompiledDiagramExactParticipantBoundaryStillOwnsUnprovenExit(t *testing.T) {
	view := b1705CompiledDiagramView(types.DiagramFlow, types.AxisFlow, true)
	view.DiagramParticipantObligations = []types.DiagramParticipantHint{
		{Identity: "Origin", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Destination", Role: types.DiagramParticipantIncidentRequired},
	}
	doc := b1705DiagramDocument(types.DiagramFlow, types.DiagramRelUnknown, false)
	doc.Blocks[0].ParticipantBoundaries = []types.DiagramParticipantBoundary{
		{Participant: "Origin", Status: types.DiagramParticipantBoundaryUnproven},
		{Participant: "Destination", Status: types.DiagramParticipantBoundaryUnproven},
	}
	bus := &types.BusContext{Mutable: types.NewMutableState("question")}
	if vs := validateDiagramEdgeSupportWithRuntimeContext(doc, view, bus); len(vs) != 0 {
		t.Fatalf("exact model-authored boundaries lost the typed exit: %+v", vs)
	}
	doc.Blocks[0].ParticipantBoundaries = doc.Blocks[0].ParticipantBoundaries[:1]
	if vs := validateDiagramEdgeSupportWithRuntimeContext(doc, view, bus); len(vs) != 1 || vs[0].Kind != types.ViolRequiredDiagramEdgeAbsent {
		t.Fatalf("incomplete boundaries must not waive the structural obligation: %+v", vs)
	}
}

func TestB1705CompiledNodeOnlyArchitectureAndTraceRemainSeparate(t *testing.T) {
	for _, trace := range []bool{false, true} {
		ir := &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain}}
		kind := types.DiagramArchitecture
		if trace {
			ir.RequestModel.Intent, ir.RequestModel.LogTriage = types.IntentRootCause, &types.LogBundle{}
			kind = types.DiagramSequence
		}
		view := types.BuildAnswerSemanticView(ir, &types.AnswerSurfacePlan{Diagram: &types.DiagramContract{Required: true, PreferredKinds: []types.DiagramKind{kind}}})
		if view.DiagramPlan.RequireStructuralEdge {
			t.Fatalf("source nonempty-edge requirement leaked into independent shape: %+v", view.DiagramPlan)
		}
		if vs := validateDiagramEdgeSupport(b1705DiagramDocument(kind, types.DiagramRelUnknown, false), view); len(vs) != 0 {
			t.Fatalf("node-only ownership grouping/runtime diagram acquired a source relation requirement: %+v", vs)
		}
	}
}
