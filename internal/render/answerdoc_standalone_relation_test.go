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

func TestRenderAnswerDocumentResolvesExactDiagramAliasesToReaderLabels(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "relations", Kind: types.BlockBulletList, SurfaceRole: types.SurfacePrincipal,
			Items:     []types.AnswerBlockItem{{Label: "platform calls", Text: "details"}},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "n1", ToNode: "n2",
				FromIdentity: "cmd_sleep", ToIdentity: "monotonic_now_ns",
				RelationKind: types.DiagramRelCall, VisibleLabel: "调用",
			}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid", Body: strings.Join([]string{
				"flowchart TD",
				`  n1["cmd_sleep"]`,
				`  n2["monotonic_now_ns"]`,
				"  n1 -->|call| n2",
			}, "\n")},
		},
	}}
	got := RenderAnswerDocument(doc, "zh")
	if !strings.Contains(got, "**cmd_sleep → monotonic_now_ns** — 调用") {
		t.Fatalf("exact diagram aliases did not resolve to reader labels:\n%s", got)
	}
	if strings.Contains(got, "**n1 → n2**") {
		t.Fatalf("internal diagram aliases leaked to the reader:\n%s", got)
	}
}

func TestRenderAnswerDocumentPreservesAmbiguousOrUnrelatedEndpointLabels(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "relations", Kind: types.BlockBulletList, SurfaceRole: types.SurfacePrincipal,
			Items:     []types.AnswerBlockItem{{Label: "path", Text: "details"}},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "reader source", ToNode: "reader target",
				FromIdentity: "source", ToIdentity: "target",
				RelationKind: types.DiagramRelCall, VisibleLabel: "calls",
			}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid", Body: strings.Join([]string{
				"flowchart TD",
				`  n1["other source"]`,
				`  n2["other target"]`,
				"  n1 --> n2",
			}, "\n")},
		},
	}}
	got := RenderAnswerDocument(doc, "en")
	if !strings.Contains(got, "**reader source → reader target** — calls") {
		t.Fatalf("unrelated diagram rewrote reader-authored endpoints:\n%s", got)
	}
}
