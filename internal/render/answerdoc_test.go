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
	out := RenderAnswerDocument(doc, "en")
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
	out := RenderAnswerDocument(doc, "en")
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
	out := RenderAnswerDocument(doc, "en")
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
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "- **Item A**") {
		t.Errorf("bullet list rendering wrong; got %q", out)
	}
}

// TestRenderV2_SkipsEmptyItems pins the 2026-05-05 fix: items
// without Label or Text (e.g. carriers of typed claim_use only)
// MUST NOT render as bare "- " bullets. Section / BulletList /
// OrderedList all share the same chokepoint.
func TestRenderV2_SkipsEmptyItems(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID: "s1", Kind: types.BlockSection, Title: "S", Text: "body",
				Items: []types.AnswerBlockItem{
					{CitationRef: -1}, // empty (claim_use-only emit shape)
					{CitationRef: -1},
				},
			},
			{
				ID: "b1", Kind: types.BlockBulletList,
				Items: []types.AnswerBlockItem{
					{Label: "Real", CitationRef: -1},
					{CitationRef: -1}, // empty — should be skipped
				},
			},
			{
				ID: "o1", Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{CitationRef: -1}, // empty — should be skipped
					{Label: "Two", CitationRef: -1},
				},
			},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	if strings.Contains(out, "- \n") {
		t.Errorf("rendered empty bullet '- '; output:\n%s", out)
	}
	if strings.Contains(out, "1. \n") || strings.Contains(out, "2. \n") {
		t.Errorf("rendered empty ordered item; output:\n%s", out)
	}
	if !strings.Contains(out, "- **Real**") {
		t.Errorf("non-empty bullet item missing; output:\n%s", out)
	}
	// OrderedList numbering must be re-indexed after skipping empty:
	// only "Two" remains, so it should be "1. **Two**", not "2. ...".
	if !strings.Contains(out, "1. **Two**") {
		t.Errorf("ordered-list re-numbering after skip wrong; output:\n%s", out)
	}
}

func TestRenderV2_BlockScalar(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "v1", Kind: types.BlockScalar, Text: "42"},
		},
	}
	out := RenderAnswerDocument(doc, "en")
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
	out := RenderAnswerDocument(doc, "zh")
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
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "Decision:") || !strings.Contains(out, "Yes") {
		t.Errorf("decision rendering wrong; got %q", out)
	}
}

func TestRenderV2_BlockDecisionRendersErrorGranularityVerdict(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:                      "d1",
				Kind:                    types.BlockDecision,
				Text:                    "The cited path rejects the bad record while continuing siblings.",
				ErrorGranularityVerdict: types.ErrorGranularityPerItemRejection,
			},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "`per_item_rejection`") ||
		!strings.Contains(out, "The cited path rejects") {
		t.Errorf("decision verdict rendering wrong; got %q", out)
	}
}

func TestRenderV2_BlockDecisionRendersCurrentStatusVerdict(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:                   "d1",
				Kind:                 types.BlockDecision,
				Text:                 "The current checkout no longer has the observed nil path.",
				CurrentStatusVerdict: types.CurrentStatusFixed,
			},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "`fixed`") ||
		!strings.Contains(out, "The current checkout") {
		t.Errorf("current-status verdict rendering wrong; got %q", out)
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
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "| L1 | first |") || !strings.Contains(out, "| L2 | second |") {
		t.Errorf("table rendering wrong; got %q", out)
	}
}

func TestRenderV2_BlockTableTextOnly(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:    "t1",
				Kind:  types.BlockTable,
				Title: "Layers",
				Text:  "| Layer | Detail |\n|---|---|\n| code default | DefaultExploreHeuristics() |",
			},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	for _, want := range []string{
		"**Layers**",
		"| code default | DefaultExploreHeuristics() |",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("text-only table rendering missing %q:\n%s", want, out)
		}
	}
}

func TestRenderV2_BlockTableItemsSuppressMarkdownText(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "t1",
				Kind: types.BlockTable,
				Text: "| Layer | Source |\n|---|---|\n| override | stale.go:1 |",
				Items: []types.AnswerBlockItem{
					{Label: "override", Text: "cmd/root.go:1430"},
					{Label: "default", Text: "cmd/root.go:71"},
				},
			},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	if strings.Contains(out, "stale.go:1") {
		t.Fatalf("structured table items must be canonical when both items[] and markdown text are present:\n%s", out)
	}
	for _, want := range []string{"| override | cmd/root.go:1430 |", "| default | cmd/root.go:71 |"} {
		if !strings.Contains(out, want) {
			t.Fatalf("structured table output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderV2_BlockTableTextSurvivesCitationOnlyItems(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "t1",
				Kind: types.BlockTable,
				Text: "| Layer | Source | Value |\n|---|---|---|\n| default | internal/types/config.go:943 | absent |",
				Items: []types.AnswerBlockItem{
					{ID: "r1", CitationRef: 0},
					{ID: "r2", CitationRef: 1},
				},
			},
		},
		Citations: []types.Citation{
			{File: "internal/types/config.go", Line: 943},
			{File: "cmd/root.go", Line: 2429},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "| default | internal/types/config.go:943 | absent |") {
		t.Fatalf("markdown table text should render when items are citation-only anchors:\n%s", out)
	}
	if strings.Contains(out, "|  |  |") {
		t.Fatalf("citation-only table items must not render empty rows:\n%s", out)
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
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "```mermaid") || !strings.Contains(out, "flowchart LR") {
		t.Errorf("diagram rendering wrong; got %q", out)
	}
}

func TestRenderV2_BlockDiagram_StripsNestedMermaidFence(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "d1",
				Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramFlow,
					Language: "mermaid",
					Body:     "```mermaid\nflowchart TD\n  A --> B\n```",
				},
			},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	if strings.Count(out, "```mermaid") != 1 {
		t.Fatalf("nested mermaid fence should be normalized to a single outer fence:\n%s", out)
	}
	if !strings.Contains(out, "flowchart TD") {
		t.Fatalf("normalized diagram body missing payload:\n%s", out)
	}
}

func TestRenderV2_DedupesExactStructuredDiagramFenceInProse(t *testing.T) {
	body := "sequenceDiagram\n    User->>Agent: ask\n    Agent-->>User: answer"
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "s1",
				Kind: types.BlockSummary,
				Text: "这里是解释。\n\n```mermaid\n" + body + "\n```\n\n这里是补充说明。",
			},
			{
				ID:    "d1",
				Kind:  types.BlockDiagram,
				Title: "调用时序",
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramSequence,
					Language: "mermaid",
					Body:     body,
				},
			},
		},
	}
	out := RenderAnswerDocument(doc, "zh")
	if strings.Count(out, "sequenceDiagram") != 1 {
		t.Fatalf("exact duplicate diagram fence should render once:\n%s", out)
	}
	if !strings.Contains(out, "这里是解释。") || !strings.Contains(out, "这里是补充说明。") {
		t.Fatalf("surrounding prose must be preserved:\n%s", out)
	}
}

func TestRenderV2_KeepsNonIdenticalDiagramFenceInProse(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "s1",
				Kind: types.BlockSummary,
				Text: "```mermaid\nsequenceDiagram\n    A->>B: draft\n```",
			},
			{
				ID:   "d1",
				Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramSequence,
					Language: "mermaid",
					Body:     "sequenceDiagram\n    A->>B: final",
				},
			},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	if strings.Count(out, "sequenceDiagram") != 2 {
		t.Fatalf("non-identical diagram fences must both render to avoid data loss:\n%s", out)
	}
}

func TestRenderV2_WithRecoveredDiagramAttachment(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "s1",
			Kind: types.BlockSummary,
			Text: "主体答案。",
		}},
	}
	out := RenderAnswerDocumentWithAttachments(doc, []types.AnswerDisplayAttachment{{
		Kind:     types.AnswerDisplayAttachmentDiagram,
		Language: "mermaid",
		Body:     "sequenceDiagram\n    User->>Agent: ask",
	}}, "zh")
	if !strings.Contains(out, "主体答案。") {
		t.Fatalf("summary lost:\n%s", out)
	}
	if !strings.Contains(out, "未能完整进入结构化答案") {
		t.Fatalf("fallback lead missing:\n%s", out)
	}
	if !strings.Contains(out, "```mermaid\nsequenceDiagram\n    User->>Agent: ask\n```") {
		t.Fatalf("recovered diagram not rendered:\n%s", out)
	}
}

func TestRenderV2_WithRecoveredDiagramAttachmentDedupesStructuredDiagram(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "answer"},
			{ID: "d1", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
				Kind:     types.DiagramSequence,
				Language: "mermaid",
				Body:     "sequenceDiagram\n    A->>B: ok",
			}},
		},
	}
	out := RenderAnswerDocumentWithAttachments(doc, []types.AnswerDisplayAttachment{{
		Kind:     types.AnswerDisplayAttachmentDiagram,
		Language: "mermaid",
		Body:     "sequenceDiagram\n    A->>B: ok",
	}}, "en")
	if strings.Count(out, "sequenceDiagram") != 1 {
		t.Fatalf("duplicate recovered diagram rendered:\n%s", out)
	}
}

func TestRenderV2_BlockCaveat(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "c1", Kind: types.BlockCaveat, Text: "scope is bounded"},
		},
	}
	out := RenderAnswerDocument(doc, "en")
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
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "Caveats:") || !strings.Contains(out, "out of scope") {
		t.Errorf("doc-level caveat missing; got %q", out)
	}
	if !strings.Contains(out, "internal/foo.go:42") {
		t.Errorf("citation pool missing; got %q", out)
	}
}

func TestRenderV2_MissingRequestedRoles(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "b1", Kind: types.BlockSummary, Text: "exact key is absent"},
		},
		MissingRequestedRoles: []types.AnswerMissingRequestedRole{
			{Role: types.EvidenceDiagramRoleConfig, Label: "codrax.yaml"},
			{Role: types.EvidenceDiagramRoleOverride, Label: "CLI"},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	for _, want := range []string{
		"Missing requested layers:",
		"No codrax.yaml entry binds this exact target.",
		"No CLI flag or command-line override binding exists for this exact target.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderV2_StripsAuthorityMarkersFromPrincipalBlocks(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "[hedged] current code suggests the failure starts here."},
			{
				ID:   "o1",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{Label: "[historical] ParseOutput", Text: "[illustrative] old stack frame", CitationRef: -1},
				},
			},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	for _, forbidden := range []string{"[hedged]", "[historical]", "[illustrative]"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("authority marker %q leaked into rendered output:\n%s", forbidden, out)
		}
	}
	for _, want := range []string{"current code suggests the failure starts here.", "ParseOutput", "old stack frame"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered output missing %q after stripping markers:\n%s", want, out)
		}
	}
}

func TestRenderV2_HidesAuthorityCaveatFromUserSurface(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "_authority_caveat",
				Kind: types.BlockCaveat,
				Text: AuthorityCaveatPrefix + AuthorityCaveatTag() + "Answer rests on mixed-authority evidence.",
			},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	if strings.Contains(out, AuthorityCaveatTag()) {
		t.Fatalf("rendered authority caveat leaked private tag to user surface:\n%s", out)
	}
	if strings.Contains(out, "Authority:") || strings.Contains(out, "Answer rests on mixed-authority evidence.") {
		t.Fatalf("rendered authority caveat should stay internal, got:\n%s", out)
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
	out := RenderAnswerDocument(doc, "en")
	if out == "" {
		t.Error("expected non-empty output even for empty blocks")
	}
}

func TestRenderV2_NilSafe(t *testing.T) {
	if got := RenderAnswerDocument(nil, "en"); got != "" {
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
	out := RenderAnswerDocument(doc, "en")
	if out == "" {
		t.Fatal("expected non-empty render for full-coverage doc")
	}
}
