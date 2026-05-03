package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ── B5-T8 V2 renderer per-BlockKind 单测 ───────────────────────────

func TestRenderV2_BlockSummary(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockSummary, Title: "Conclusion", Text: "the answer is X"},
		},
	}
	out := RenderAnswerDocumentV2(doc, "en")
	if !strings.Contains(out, "## Conclusion") {
		t.Errorf("missing summary heading; got %q", out)
	}
	if !strings.Contains(out, "the answer is X") {
		t.Errorf("missing summary body; got %q", out)
	}
}

func TestRenderV2_BlockSection(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSection, Title: "Layer A", Text: "responsibility"},
		},
	}
	out := RenderAnswerDocumentV2(doc, "en")
	if !strings.Contains(out, "### Layer A") {
		t.Errorf("missing section heading; got %q", out)
	}
}

func TestRenderV2_BlockOrderedList(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "l1",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{Label: "Step 1", Text: "do thing", CitationRef: -1},
					{Label: "Step 2", Text: "next thing", CitationRef: -1},
				},
			},
		},
	}
	out := RenderAnswerDocumentV2(doc, "en")
	if !strings.Contains(out, "1. **Step 1**") || !strings.Contains(out, "2. **Step 2**") {
		t.Errorf("ordered list rendering wrong; got %q", out)
	}
}

func TestRenderV2_BlockBulletList(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "b1",
				Kind: types.BlockBulletList,
				Items: []types.AnswerBlockItem{
					{Label: "Item A", CitationRef: -1},
					{Label: "Item B", CitationRef: -1},
				},
			},
		},
	}
	out := RenderAnswerDocumentV2(doc, "en")
	if !strings.Contains(out, "- **Item A**") {
		t.Errorf("bullet list rendering wrong; got %q", out)
	}
}

func TestRenderV2_BlockScalar(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "v1", Kind: types.BlockScalar, Text: "42"},
		},
	}
	out := RenderAnswerDocumentV2(doc, "en")
	if !strings.Contains(out, "Value:") || !strings.Contains(out, "`42`") {
		t.Errorf("scalar rendering wrong; got %q", out)
	}
}

func TestRenderV2_BlockScalarZH(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "v1", Kind: types.BlockScalar, Text: "42"},
		},
	}
	out := RenderAnswerDocumentV2(doc, "zh")
	if !strings.Contains(out, "值：") {
		t.Errorf("scalar zh rendering wrong; got %q", out)
	}
}

func TestRenderV2_BlockDecision(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "d1", Kind: types.BlockDecision, Text: "Yes"},
		},
	}
	out := RenderAnswerDocumentV2(doc, "en")
	if !strings.Contains(out, "Decision:") || !strings.Contains(out, "Yes") {
		t.Errorf("decision rendering wrong; got %q", out)
	}
}

func TestRenderV2_BlockTable(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:    "t1",
				Kind:  types.BlockTable,
				Title: "Layers",
				Items: []types.AnswerBlockItem{
					{Label: "L1", Text: "first"},
					{Label: "L2", Text: "second"},
				},
			},
		},
	}
	out := RenderAnswerDocumentV2(doc, "en")
	if !strings.Contains(out, "| L1 | first |") || !strings.Contains(out, "| L2 | second |") {
		t.Errorf("table rendering wrong; got %q", out)
	}
}

func TestRenderV2_BlockDiagram(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "d1",
				Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramFlow,
					Language: "mermaid",
					Body:     "flowchart LR\n  A --> B",
				},
			},
		},
	}
	out := RenderAnswerDocumentV2(doc, "en")
	if !strings.Contains(out, "```mermaid") || !strings.Contains(out, "flowchart LR") {
		t.Errorf("diagram rendering wrong; got %q", out)
	}
}

func TestRenderV2_BlockCaveat(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "c1", Kind: types.BlockCaveat, Text: "scope is bounded"},
		},
	}
	out := RenderAnswerDocumentV2(doc, "en")
	if !strings.Contains(out, "> scope is bounded") {
		t.Errorf("caveat rendering wrong; got %q", out)
	}
}

func TestRenderV2_DocumentLevelCaveatsAndCitations(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockSummary, Text: "x"},
		},
		Caveats: []string{"out of scope"},
		Citations: []types.Citation{
			{File: "internal/foo.go", Line: 42, Scope: types.ScopeLine},
		},
	}
	out := RenderAnswerDocumentV2(doc, "en")
	if !strings.Contains(out, "Caveats:") || !strings.Contains(out, "out of scope") {
		t.Errorf("doc-level caveat missing; got %q", out)
	}
	if !strings.Contains(out, "internal/foo.go:42") {
		t.Errorf("citation pool missing; got %q", out)
	}
}

func TestRenderV2_BoundaryEmptyBlocks(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockOrderedList}, // empty Items
			{ID: "b2", Kind: types.BlockSummary},     // empty Text
		},
	}
	// Should not crash; produces minimal output.
	out := RenderAnswerDocumentV2(doc, "en")
	if out == "" {
		t.Error("expected non-empty output even for empty blocks")
	}
}

func TestRenderV2_NilSafe(t *testing.T) {
	if got := RenderAnswerDocumentV2(nil, "en"); got != "" {
		t.Errorf("nil doc should return empty; got %q", got)
	}
}

// ── B5-T7 V1/V2 一致性 fixture (smoke-level: V2 渲染产用户 visible markdown) ──

func TestRenderV2_AllBlockKindsCoveredBySwitch(t *testing.T) {
	// 结构性测试: 每 BlockKind 都被 renderer dispatch 处理 (即使是 no-op).
	// 手段: 构造一个 doc 含每种 BlockKind, 渲染不 panic 且非空.
	blocks := []types.AnswerBlock{
		{ID: "1", Kind: types.BlockSummary, Text: "x"},
		{ID: "2", Kind: types.BlockSection, Title: "S", Text: "y"},
		{ID: "3", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{Label: "a"}}},
		{ID: "4", Kind: types.BlockBulletList, Items: []types.AnswerBlockItem{{Label: "a"}}},
		{ID: "5", Kind: types.BlockScalar, Text: "42"},
		{ID: "6", Kind: types.BlockDecision, Text: "Yes"},
		{ID: "7", Kind: types.BlockTable, Items: []types.AnswerBlockItem{{Label: "a", Text: "b"}}},
		{ID: "8", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Body: "x"}},
		{ID: "9", Kind: types.BlockCaveat, Text: "z"},
	}
	if len(blocks) != len(types.AllAnswerBlockKinds()) {
		t.Fatalf("test fixture covers %d kinds but AllAnswerBlockKinds has %d — update fixture",
			len(blocks), len(types.AllAnswerBlockKinds()))
	}
	doc := &types.AnswerDocumentV2{Blocks: blocks}
	out := RenderAnswerDocumentV2(doc, "en")
	if out == "" {
		t.Fatal("expected non-empty render for full-coverage doc")
	}
}
