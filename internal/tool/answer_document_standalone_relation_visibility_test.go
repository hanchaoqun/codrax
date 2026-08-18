package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func standaloneRelationVisibilityFixture(label string) (*types.AnswerDocumentV2, *types.AnswerSemanticView) {
	return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "handoff", Kind: types.BlockBulletList, SurfaceRole: types.SurfacePrincipal,
		Items:     []types.AnswerBlockItem{{ID: "native", Label: "native tokenizer", Text: "exports the optimized implementation"}},
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimRegistrationEdge}},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "native tokenizer", ToNode: "Python tokenizer",
			FromIdentity: "_fastlex.tokenize_bytes", ToIdentity: "py.tokenize_bytes",
			RelationKind: types.DiagramRelRegister, VisibleLabel: label,
		}},
	}}}, &types.AnswerSemanticView{Family: types.QFCallChain}
}

func TestStandaloneTypedRelationVisibilityRequiresModelLabel(t *testing.T) {
	doc, _ := standaloneRelationVisibilityFixture("")
	hints := preCheckStandaloneTypedRelationVisibility(doc)
	if len(hints) != 1 ||
		!strings.Contains(hints[0].Field, "edge_anchors[].visible_label") ||
		!strings.Contains(hints[0].ExpectedShape, "_fastlex.tokenize_bytes -> py.tokenize_bytes") ||
		strings.Contains(hints[0].ExpectedShape, "注册可用") {
		t.Fatalf("visibility hints=%+v, want structure-only missing-label repair", hints)
	}

	doc.Blocks[0].EdgeAnchors[0].VisibleLabel = "注册可用的跨语言回退入口"
	if hints := preCheckStandaloneTypedRelationVisibility(doc); len(hints) != 0 {
		t.Fatalf("model-authored visible relation label must close visibility debt: %+v", hints)
	}
}

func TestStandaloneTypedRelationVisibilityPreEmitWirePin(t *testing.T) {
	doc, view := standaloneRelationVisibilityFixture("")
	hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, view, nil)
	if len(hints) != 1 ||
		len(hints[0].DiagramRelationFailureIssues) != 1 ||
		hints[0].DiagramRelationFailureIssues[0] != diagramStandaloneRelationMissingVisibleLabel {
		t.Fatalf("pre-emit wire lost standalone visibility gate: %+v", hints)
	}
}

func TestStandaloneRelationHintsDoNotSerializeIndependentDiagramRepairs(t *testing.T) {
	doc, view := standaloneRelationVisibilityFixture("")
	// The standalone carrier independently lacks exact endpoint identities.
	doc.Blocks[0].EdgeAnchors[0].FromIdentity = ""
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID: "requested-sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n" +
				"  participant A as Alpha.Run\n" +
				"  participant B as Beta.Run\n" +
				"  participant C as Gamma.Run\n" +
				"  A->>B: first\n" +
				"  B->>C: second\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall,
		}},
	})
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("Alpha.Run", "Beta.Run"),
		diagramEvidenceTestCall("Beta.Run", "Gamma.Run"),
	}
	evidence[1].ID = "ev-beta-gamma"
	evidence[1].LineStart = 11
	mut := types.NewMutableState("show the selected relations")
	mut.AppendEvidence(evidence)
	pctx := newPreEmitCheckContext(&types.BusContext{Mutable: mut, EvidenceItems: evidence})

	hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, view, pctx)
	var issues []string
	for _, hint := range hints {
		issues = append(issues, hint.DiagramRelationFailureIssues...)
	}
	for _, want := range []string{
		diagramStandaloneRelationMissingVisibleLabel,
		diagramStandaloneRelationIdentityMissing,
		types.DiagramRelationFailureMissingGroundedCallAnchor,
	} {
		if !containsString(issues, want) {
			t.Fatalf("one draft must report every independent precise relation repair; missing %q in hints=%+v", want, hints)
		}
	}
}

func TestStandaloneTypedRelationVisibilityDoesNotApplyToDiagram(t *testing.T) {
	doc, _ := standaloneRelationVisibilityFixture("")
	doc.Blocks[0].Kind = types.BlockDiagram
	doc.Blocks[0].Diagram = &types.AnswerDiagramBlock{
		Kind: types.DiagramSequence, Language: "mermaid",
		Body: "sequenceDiagram\n  participant A as native tokenizer\n  participant B as Python tokenizer\n  A->>B: register",
	}
	if hints := preCheckStandaloneTypedRelationVisibility(doc); len(hints) != 0 {
		t.Fatalf("diagram body already owns visible relation surface: %+v", hints)
	}
}

func TestStandaloneTypedRelationVisibilityAcceptsSiblingDiagramEdge(t *testing.T) {
	doc, _ := standaloneRelationVisibilityFixture("")
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID: "diagram", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n  participant N as native tokenizer\n  participant P as Python tokenizer\n  N->>P: registers fallback",
		},
	})
	if hints := preCheckStandaloneTypedRelationVisibility(doc); len(hints) != 0 {
		t.Fatalf("sibling diagram already renders the exact anchor pair: %+v", hints)
	}
}
