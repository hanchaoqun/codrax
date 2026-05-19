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

func TestRenderV2_PreservesAuthoredTextAcrossBlockKinds(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "summary-authored-text"},
			{ID: "section", Kind: types.BlockSection, Text: "section-authored-text"},
			{ID: "ordered", Kind: types.BlockOrderedList, Text: "ordered-authored-text", Items: []types.AnswerBlockItem{{Label: "first"}}},
			{ID: "bullets", Kind: types.BlockBulletList, Text: "bullet-authored-text", Items: []types.AnswerBlockItem{{Label: "one"}}},
			{ID: "scalar", Kind: types.BlockScalar, Text: "scalar-authored-text"},
			{ID: "decision", Kind: types.BlockDecision, Text: "decision-authored-text"},
			{ID: "table", Kind: types.BlockTable, Text: "| A | B |\n|---|---|\n| table-authored-text | value |"},
			{ID: "diagram", Kind: types.BlockDiagram, Text: "diagram-authored-text", Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramFlow,
				Body: "flowchart LR\n  A --> B",
			}},
			{ID: "caveat", Kind: types.BlockCaveat, Text: "caveat-authored-text"},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	for _, want := range []string{
		"summary-authored-text",
		"section-authored-text",
		"ordered-authored-text",
		"bullet-authored-text",
		"scalar-authored-text",
		"decision-authored-text",
		"table-authored-text",
		"diagram-authored-text",
		"caveat-authored-text",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("renderer dropped authored block text %q:\n%s", want, out)
		}
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

func TestRenderV2_BlockSectionItemsKeepCitations(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:    "s1",
				Kind:  types.BlockSection,
				Title: "Layer A",
				Items: []types.AnswerBlockItem{
					{Label: "Fact", Text: "grounded detail", CitationRef: 0},
				},
			},
		},
		Citations: []types.Citation{{File: "internal/foo.go", Line: 42}},
	}
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "- **Fact** — grounded detail (`internal/foo.go:42`)") {
		t.Fatalf("section item citation was hidden:\n%s", out)
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
			{ID: "d1", Kind: types.BlockDecision, Title: "Availability", Text: "Yes"},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "**Availability**") ||
		!strings.Contains(out, "Decision:") ||
		!strings.Contains(out, "Yes") {
		t.Errorf("decision rendering wrong; got %q", out)
	}
}

func TestRenderV2_ExactResolutionVisibleLead(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{
			Status:      types.AnswerExactResolutionAbsent,
			ContextMode: types.AnswerExactResolutionContextGroundedOnly,
		},
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "附近上下文仍然有效。"},
		},
	}
	out := RenderAnswerDocument(doc, "zh")
	for _, want := range []string{
		"当前已验证范围内未找到完全一致的精确目标。",
		"仅作为已落地上下文",
		"附近上下文仍然有效。",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("exact_resolution lead missing %q:\n%s", want, out)
		}
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
	if !strings.Contains(out, "| Item | Details |") {
		t.Errorf("english fallback table header should be polished; got %q", out)
	}
}

func TestRenderV2_BlockTableFallbackHeaderZH(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:    "t1",
				Kind:  types.BlockTable,
				Title: "配置来源",
				Items: []types.AnswerBlockItem{
					{Label: "codrax.yaml", Text: "运行时配置"},
				},
			},
		},
	}
	out := RenderAnswerDocument(doc, "zh")
	if !strings.Contains(out, "| 项目 | 说明 |") || strings.Contains(out, "| Item | Detail |") {
		t.Fatalf("zh fallback table header should be localized and not leak Item/Detail:\n%s", out)
	}
	if !strings.Contains(out, "**配置来源**") || !strings.Contains(out, "| codrax.yaml | 运行时配置 |") {
		t.Fatalf("fallback table lost title or row:\n%s", out)
	}
}

func TestRenderV2_BlockTableStructuredCellsPreserveMultipleColumns(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:      "comparison",
				Kind:    types.BlockTable,
				Title:   "读模式防幻觉对比",
				Columns: []string{"维度", "codrax", "opencode"},
				Items: []types.AnswerBlockItem{
					{ID: "r1", Cells: []string{"证据追踪", "EvidenceItem + citation_ref", "无结构化追踪"}},
					{ID: "r2", Cells: []string{"交验策略", "ContractCheck + PreEmitCheck", "主要依赖模型自检"}},
				},
			},
		},
	}
	out := RenderAnswerDocument(doc, "zh")
	for _, want := range []string{
		"| 维度 | codrax | opencode |",
		"| 证据追踪 | EvidenceItem + citation_ref | 无结构化追踪 |",
		"| 交验策略 | ContractCheck + PreEmitCheck | 主要依赖模型自检 |",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("structured table should preserve multi-column cell %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "| 项目 | 说明 |") || strings.Contains(out, "| Item | Details |") {
		t.Fatalf("structured multi-column table must not collapse to two-column fallback:\n%s", out)
	}
}

func TestRenderV2_BlockTableStructuredCellsWithRowLabels(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:      "comparison",
				Kind:    types.BlockTable,
				Columns: []string{"codrax", "opencode"},
				Items: []types.AnswerBlockItem{
					{ID: "r1", Label: "证据追踪", Cells: []string{"citations[] 池", "无等价结构"}},
				},
			},
		},
	}
	out := RenderAnswerDocument(doc, "zh")
	if !strings.Contains(out, "| 项目 | codrax | opencode |") ||
		!strings.Contains(out, "| 证据追踪 | citations[] 池 | 无等价结构 |") {
		t.Fatalf("row label should become the first column without losing cells:\n%s", out)
	}
}

func TestRenderV2_BlockTableStructuredCellsPreserveItemTextOverflow(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:      "kind_rows",
				Kind:    types.BlockTable,
				Columns: []string{"符号名称", "定义位置", "说明"},
				Items: []types.AnswerBlockItem{
					{
						ID:    "k_external",
						Label: "KindExternalArtifactDecoded",
						Cells: []string{
							"internal/analysis/criterion/grammar.go:65",
							"Kind常量",
						},
						Text: "Kind = Kind(types.CritExternalArtifactDecoded)，为兼容性保留，已废弃",
					},
				},
			},
		},
	}
	out := RenderAnswerDocument(doc, "zh")
	for _, want := range []string{
		"| 项目 | 符号名称 | 定义位置 | 说明 |",
		"KindExternalArtifactDecoded",
		"为兼容性保留，已废弃",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("structured table renderer dropped item text surface %q:\n%s", want, out)
		}
	}
}

func TestRenderV2_BlockTableIntroTextDoesNotHideStructuredRows(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:      "symbols",
				Kind:    types.BlockTable,
				Text:    "下面按公开符号类别列出。",
				Columns: []string{"符号名称", "说明"},
				Items: []types.AnswerBlockItem{
					{Label: "Eval", Cells: []string{"对单个 Criterion 求值"}},
				},
			},
		},
	}
	out := RenderAnswerDocument(doc, "zh")
	for _, want := range []string{
		"下面按公开符号类别列出。",
		"| 符号名称 | 说明 |",
		"| Eval | 对单个 Criterion 求值 |",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("table intro text should not hide structured rows %q:\n%s", want, out)
		}
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

func TestRenderV2_BlockTableMarkdownTextWinsOverRowItems(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "t1",
				Kind: types.BlockTable,
				Text: "| Layer | Source | Mode |\n|---|---|---|\n| override | stale.go:1 | read |",
				Items: []types.AnswerBlockItem{
					{Label: "override", Text: "cmd/root.go:1430"},
					{Label: "default", Text: "cmd/root.go:71"},
				},
			},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	for _, want := range []string{
		"| Layer | Source | Mode |",
		"| override | stale.go:1 | read |",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("markdown table text should be authoritative for multi-column tables; missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "| Item | Detail |") || strings.Contains(out, "| default | cmd/root.go:71 |") {
		t.Fatalf("table renderer must not synthesize an alternate two-column table when block text is present:\n%s", out)
	}
}

func TestRenderV2_BlockTableMarkdownItemTextWinsOverFallback(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:    "t1",
				Kind:  types.BlockTable,
				Title: "对比结果",
				Items: []types.AnswerBlockItem{
					{
						ID: "r1",
						Text: strings.Join([]string{
							"| 维度 | codrax | opencode |",
							"|---|---|---|",
							"| 引用校验 | citations[] + citation_ref | 无等价结构 |",
						}, "\n"),
					},
					{ID: "carrier", Label: "引用校验", Text: "citation-only row carrier"},
				},
			},
		},
	}
	out := RenderAnswerDocument(doc, "zh")
	for _, want := range []string{
		"**对比结果**",
		"| 维度 | codrax | opencode |",
		"| 引用校验 | citations[] + citation_ref | 无等价结构 |",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("markdown table inside item text should be preserved; missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "| 项目 | 说明 |") || strings.Contains(out, "citation-only row carrier") {
		t.Fatalf("markdown table item text must not collapse to fallback rows:\n%s", out)
	}
}

func TestRenderV2_BlockTablePreservesComparisonColumnsWithRowCitationItems(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:    "comparison_table",
				Kind:  types.BlockTable,
				Title: "防幻觉机制对比",
				Text: strings.Join([]string{
					"| 对比维度 | codrax (读模式) | opencode (读模式) |",
					"|---|---|---|",
					"| 证据追踪 | EvidenceClosure 追踪 readSet/readRanges/fileTotalLines | 无任何追踪机制 |",
					"| 引用验证 | ContractCheck 校验 citation_ref | 无引用验证 |",
				}, "\n"),
				Items: []types.AnswerBlockItem{
					{ID: "r1", Label: "证据追踪", CitationRef: 0},
					{ID: "r2", Label: "引用验证", CitationRef: 1},
				},
			},
		},
		Citations: []types.Citation{
			{File: "internal/agent/evidence_closure.go", Line: 10},
			{File: "internal/orchestrator/contract_check.go", Line: 784},
		},
	}
	out := RenderAnswerDocument(doc, "zh")
	for _, want := range []string{
		"**防幻觉机制对比**",
		"| 对比维度 | codrax (读模式) | opencode (读模式) |",
		"| 证据追踪 | EvidenceClosure 追踪 readSet/readRanges/fileTotalLines | 无任何追踪机制 |",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("comparison table lost markdown column surface %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "| Item | Detail |") || strings.Contains(out, "| 证据追踪 |  |") {
		t.Fatalf("row citation carriers must not collapse a multi-column table to Item/Detail:\n%s", out)
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

func TestRenderV2_BlockDiagramKeepsAuthoredText(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:    "d1",
				Kind:  types.BlockDiagram,
				Title: "Flow",
				Text:  "The diagram shows the accepted control path.",
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramFlow,
					Language: "mermaid",
					Body:     "flowchart LR\n  A --> B",
				},
			},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	for _, want := range []string{"**Flow**", "The diagram shows the accepted control path.", "```mermaid"} {
		if !strings.Contains(out, want) {
			t.Fatalf("diagram block lost authored text %q:\n%s", want, out)
		}
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
	for _, want := range []string{
		"主体答案。\n\n---\n\n> **系统保留内容**",
		"> 下面内容来自模型已生成但未能完整进入结构化答案的输出",
		"#### 保留的图",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("recovered diagram attachment missing %q:\n%s", want, out)
		}
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

func TestRenderV2_WithRecoveredTextAttachmentUsesDivider(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "s1",
			Kind: types.BlockSummary,
			Text: "主体答案。",
		}},
	}
	out := RenderAnswerDocumentWithAttachments(doc, []types.AnswerDisplayAttachment{{
		Kind: types.AnswerDisplayAttachmentText,
		Body: "模型最后一轮保留下来的原文。",
	}}, "zh")
	for _, want := range []string{
		"主体答案。",
		"---\n\n> **系统保留内容**",
		"> 下面内容来自模型已生成但未能完整进入结构化答案的输出",
		"#### 保留的原文",
		"模型最后一轮保留下来的原文。",
		"\n---\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("recovered text attachment missing %q:\n%s", want, out)
		}
	}
}

func TestRenderV2_DedupesRecoveredTextAttachmentMatchingRenderedAnswer(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "s1",
			Kind: types.BlockSummary,
			Text: "主体答案。",
		}},
	}
	out := RenderAnswerDocumentWithAttachments(doc, []types.AnswerDisplayAttachment{{
		Kind: types.AnswerDisplayAttachmentText,
		Body: RenderAnswerDocument(doc, "zh"),
	}}, "zh")
	if strings.Contains(out, "系统保留内容") || strings.Contains(out, "保留的原文") {
		t.Fatalf("attachment identical to the final rendered answer must be suppressed:\n%s", out)
	}
	if strings.Count(out, "主体答案。") != 1 {
		t.Fatalf("final answer should render exactly once:\n%s", out)
	}
}

func TestRenderV2_RecoveredAttachmentCapDisclosesOmittedItems(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "主体答案。"}},
	}
	attachments := []types.AnswerDisplayAttachment{
		{Kind: types.AnswerDisplayAttachmentText, Body: "保留内容 1"},
		{Kind: types.AnswerDisplayAttachmentText, Body: "保留内容 2"},
		{Kind: types.AnswerDisplayAttachmentText, Body: "保留内容 3"},
		{Kind: types.AnswerDisplayAttachmentText, Body: "保留内容 4"},
		{Kind: types.AnswerDisplayAttachmentText, Body: "保留内容 5"},
	}
	out := RenderAnswerDocumentWithAttachments(doc, attachments, "zh")
	if !strings.Contains(out, "保留内容 4") || strings.Contains(out, "保留内容 5") {
		t.Fatalf("attachment cap should show first four unique attachments only:\n%s", out)
	}
	if !strings.Contains(out, "另有 1 条保留内容未显示") {
		t.Fatalf("attachment cap should disclose omitted preserved content:\n%s", out)
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

func TestRenderV2_ScopeDisclosureVisibleOnce(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "没有找到目标。", ScopeDisclosure: types.ScopeDisclosureOutOfActiveScope},
			{ID: "c1", Kind: types.BlockCaveat, Text: "只检查了当前活跃仓。", ScopeDisclosure: types.ScopeDisclosureOutOfActiveScope},
		},
	}
	out := RenderAnswerDocument(doc, "zh")
	if count := strings.Count(out, "目标不在当前活跃仓范围内"); count != 1 {
		t.Fatalf("scope_disclosure should render once, got %d:\n%s", count, out)
	}
	if !strings.Contains(out, "没有找到目标。") || !strings.Contains(out, "只检查了当前活跃仓。") {
		t.Fatalf("scope_disclosure rendering dropped block prose:\n%s", out)
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

func TestRenderV2_SkipsSectionItemsDuplicatedByPrincipalList(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:    "types_section",
				Kind:  types.BlockSection,
				Title: "Types",
				Text:  "Types are listed below.",
				Items: []types.AnswerBlockItem{
					{ID: "t1", Label: "Kind", Text: "type alias", CitationRef: 0},
					{ID: "t2", Label: "Env", Text: "runtime data", CitationRef: 1},
				},
			},
			{
				ID:   "types_list",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "t1", Label: "Kind", Text: "type alias", CitationRef: 0},
					{ID: "t2", Label: "Env", Text: "runtime data", CitationRef: 1},
				},
			},
		},
		Citations: []types.Citation{
			{File: "internal/analysis/criterion/grammar.go", Line: 26},
			{File: "internal/analysis/criterion/grammar.go", Line: 124},
		},
	}

	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "Types are listed below.") {
		t.Fatalf("section prose should remain visible:\n%s", out)
	}
	if strings.Count(out, "**Kind**") != 1 || strings.Count(out, "**Env**") != 1 {
		t.Fatalf("duplicated section items should render only through the principal list:\n%s", out)
	}
}

func TestRenderV2_KeepsDistinctSectionItems(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:    "context",
				Kind:  types.BlockSection,
				Title: "Context",
				Items: []types.AnswerBlockItem{
					{ID: "note", Label: "Kind", Text: "extra section-only context", CitationRef: 0},
				},
			},
			{
				ID:   "types_list",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "t1", Label: "Kind", Text: "type alias", CitationRef: 0},
				},
			},
		},
		Citations: []types.Citation{{File: "internal/analysis/criterion/grammar.go", Line: 26}},
	}

	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "extra section-only context") || !strings.Contains(out, "type alias") {
		t.Fatalf("distinct section items must remain visible:\n%s", out)
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
