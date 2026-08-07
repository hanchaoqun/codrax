package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeDiagramEdgeAnchorMetadata_NormalizesOnlyTypedMetadata(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "d1",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind:     types.DiagramArchitecture,
			Language: "mermaid",
			Body: strings.Join([]string{
				"flowchart TD",
				"    A[\"Caller\"] -->|calls| B[\"Callee\"]",
				"    B -->|imports| C[\"Module\"]",
			}, "\n"),
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode:     "Caller",
			ToNode:       "Callee",
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimImportEdge,
		}},
	}}}
	originalBody := doc.Blocks[0].Diagram.Body

	fixed := normalizeDiagramEdgeAnchorMetadata(doc)
	if fixed != 3 {
		t.Fatalf("fixed=%d, want 3; anchors=%+v", fixed, doc.Blocks[0].EdgeAnchors)
	}
	if doc.Blocks[0].Diagram.Body != originalBody {
		t.Fatalf("typed metadata normalization must not rewrite model Mermaid body:\nbefore=%s\nafter=%s", originalBody, doc.Blocks[0].Diagram.Body)
	}
	anchors := doc.Blocks[0].EdgeAnchors
	if len(anchors) != 1 {
		t.Fatalf("len(edge_anchors)=%d, want 1: label text must not mint typed authority: %+v", len(anchors), anchors)
	}
	if anchors[0].FromNode != "A" || anchors[0].ToNode != "B" ||
		anchors[0].RelationKind != types.DiagramRelCall ||
		anchors[0].ClaimForm != types.ClaimCallEdge {
		t.Fatalf("existing anchor not normalized: %+v", anchors[0])
	}
}

func TestNormalizeOrphanDiagramEdgeAnchors_RemovedOptionalDiagramClearsSiblingMetadata(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "summary", Kind: types.BlockSummary,
			Text: "The optional diagram was removed; the grounded text remains.",
		},
		{
			ID: "path", Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{ID: "hop", Label: "run_pipeline"}},
			EdgeAnchors: []types.DiagramEdgeAnchor{
				{FromNode: "run_pipeline", ToNode: "resolve", RelationKind: types.DiagramRelCall},
				{FromNode: "loop", ToNode: "handle", RelationKind: types.DiagramRelCallback},
			},
		},
	}}
	if removed := normalizeOrphanDiagramEdgeAnchors(doc); removed != 2 {
		t.Fatalf("removed=%d, want 2: %+v", removed, doc.Blocks)
	}
	if len(doc.Blocks[1].EdgeAnchors) != 0 {
		t.Fatalf("orphan anchors survived after all typed diagrams were removed: %+v", doc.Blocks[1].EdgeAnchors)
	}
}

func TestNormalizeOrphanDiagramEdgeAnchors_PreservesSiblingCarrierForExistingDiagram(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList,
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall,
			}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramSequence, Language: "mermaid",
				Body: "sequenceDiagram\n  participant A as Caller\n  participant B as Callee\n  A->>B: call\n",
			},
		},
	}}
	if removed := normalizeOrphanDiagramEdgeAnchors(doc); removed != 0 {
		t.Fatalf("removed=%d, want 0 while a typed diagram can consume sibling anchors", removed)
	}
	if len(doc.Blocks[0].EdgeAnchors) != 1 {
		t.Fatalf("valid sibling diagram anchor was removed: %+v", doc.Blocks[0].EdgeAnchors)
	}
}

func TestNormalizeAnswerDocumentForPreEmit_RecordsOrphanAnchorRepair(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "path", Kind: types.BlockOrderedList,
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "loop", ToNode: "handle", RelationKind: types.DiagramRelCallback,
		}},
	}}}
	pctx := newPreEmitCheckContext()
	normalizeAnswerDocumentForPreEmit("test", doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil, pctx)
	if got := pctx.repairCounts["normalizeOrphanDiagramEdgeAnchors"]; got != 1 {
		t.Fatalf("repair count=%d, want 1: %+v", got, pctx.repairCounts)
	}
	if len(doc.Blocks[0].EdgeAnchors) != 0 {
		t.Fatalf("pre-emit normalization retained orphan metadata: %+v", doc.Blocks[0].EdgeAnchors)
	}
}
