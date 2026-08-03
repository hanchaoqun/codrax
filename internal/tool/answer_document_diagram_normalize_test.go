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
