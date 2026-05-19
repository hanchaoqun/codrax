package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCompileCitationBackedTableRows_FillsEmptyMultiColumnRows(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:      "kind_consts",
			Kind:    types.BlockTable,
			Title:   "Kind 常量",
			Columns: []string{"类别", "符号名称", "定义位置", "说明"},
			Items: []types.AnswerBlockItem{
				{ID: "r1", Label: "KindSymbolPresent", CitationRef: 0},
				{ID: "r2", Label: "KindNoCallSites", CitationRef: 1},
			},
		}},
		Citations: []types.Citation{
			{File: "internal/analysis/criterion/grammar.go", Line: 29, Quote: "KindSymbolPresent Kind = Kind(types.CritSymbolPresent)"},
			{File: "internal/analysis/criterion/grammar.go", Line: 30, Quote: "KindNoCallSites Kind = Kind(types.CritNoCallSites)"},
		},
	}

	if fixed := compileCitationBackedTableRows(doc); fixed != 2 {
		t.Fatalf("fixed=%d, want 2", fixed)
	}
	table := doc.Blocks[0]
	if got, want := table.Columns, []string{"符号名称", "定义位置", "说明"}; len(got) != len(want) {
		t.Fatalf("columns=%v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("columns=%v, want %v", got, want)
			}
		}
	}
	if got := table.Items[0].Label; got != "KindSymbolPresent" {
		t.Fatalf("label changed: %q", got)
	}
	if got := table.Items[0].Cells; len(got) != 2 ||
		got[0] != "internal/analysis/criterion/grammar.go:29" ||
		got[1] != "KindSymbolPresent Kind = Kind(types.CritSymbolPresent)" {
		t.Fatalf("cells not compiled from citation: %#v", got)
	}
}

func TestCompileCitationBackedTableRows_PreservesExplicitCells(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:      "comparison",
			Kind:    types.BlockTable,
			Columns: []string{"维度", "codrax", "opencode"},
			Items: []types.AnswerBlockItem{{
				ID:    "r1",
				Label: "证据追踪",
				Cells: []string{"citations[]", "none"},
			}},
		}},
	}

	if fixed := compileCitationBackedTableRows(doc); fixed != 0 {
		t.Fatalf("fixed=%d, want 0", fixed)
	}
	if got := doc.Blocks[0].Columns; len(got) != 3 || got[0] != "维度" {
		t.Fatalf("columns should be preserved: %#v", got)
	}
}

func TestCompileCitationBackedTableRows_PreservesMarkdownTableText(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:      "markdown",
			Kind:    types.BlockTable,
			Columns: []string{"A", "B", "C"},
			Text:    "| A | B | C |\n|---|---|---|\n| x | y | z |",
			Items: []types.AnswerBlockItem{{
				ID:          "r1",
				Label:       "x",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{File: "a.go", Line: 1, Quote: "x"}},
	}

	if fixed := compileCitationBackedTableRows(doc); fixed != 0 {
		t.Fatalf("fixed=%d, want 0", fixed)
	}
	if len(doc.Blocks[0].Items[0].Cells) != 0 {
		t.Fatalf("markdown table carrier should not be rewritten: %#v", doc.Blocks[0].Items[0].Cells)
	}
}

func TestCompileCitationBackedTableRows_RequiresEveryVisibleRowCited(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:      "partial",
			Kind:    types.BlockTable,
			Columns: []string{"Name", "Location", "Notes"},
			Items: []types.AnswerBlockItem{
				{ID: "r1", Label: "Known", CitationRef: 0},
				{ID: "r2", Label: "Unknown", CitationRef: -1},
			},
		}},
		Citations: []types.Citation{{File: "a.go", Line: 1, Quote: "Known"}},
	}

	if fixed := compileCitationBackedTableRows(doc); fixed != 0 {
		t.Fatalf("fixed=%d, want 0", fixed)
	}
	if len(doc.Blocks[0].Items[0].Cells) != 0 || len(doc.Blocks[0].Items[1].Cells) != 0 {
		t.Fatalf("partial citation coverage should not synthesize table cells: %+v", doc.Blocks[0].Items)
	}
}
