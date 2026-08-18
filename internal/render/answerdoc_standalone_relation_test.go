package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderAnswerDocumentRendersModelAuthoredStandaloneRelation(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "handoff", Kind: types.BlockBulletList, Title: "回退链", SurfaceRole: types.SurfacePrincipal,
		Items:     []types.AnswerBlockItem{{Label: "native tokenizer", Text: "优化实现"}},
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimRegistrationEdge}},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "native tokenizer", ToNode: "Python tokenizer",
			FromIdentity: "_fastlex.tokenize_bytes", ToIdentity: "py.tokenize_bytes",
			RelationKind: types.DiagramRelRegister,
			VisibleLabel: "注册可用的跨语言回退入口",
		}},
	}}}
	got := RenderAnswerDocument(doc, "zh")
	if !strings.Contains(got, "**native tokenizer → Python tokenizer** — 注册可用的跨语言回退入口") {
		t.Fatalf("standalone model relation not rendered:\n%s", got)
	}
	if strings.Contains(got, "relation_kind") || strings.Contains(got, "register") {
		t.Fatalf("renderer leaked internal relation metadata:\n%s", got)
	}
}

func TestRenderAnswerDocumentDoesNotDuplicateDiagramAnchorLabel(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "diagram", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence, Language: "mermaid",
			Body: "sequenceDiagram\n  participant A\n  participant B\n  A->>B: calls",
		},
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall,
			VisibleLabel: "must not render twice",
		}},
	}}}
	got := RenderAnswerDocument(doc, "en")
	if strings.Contains(got, "must not render twice") {
		t.Fatalf("diagram relation label duplicated outside diagram body:\n%s", got)
	}
}
