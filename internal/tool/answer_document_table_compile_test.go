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

func TestCompileEnumerationDisplayTableRows_FillsRowsFromPrincipalEvidence(t *testing.T) {
	mut := types.NewMutableState("list public functions")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "公开函数",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Unit:        "函数",
		Members:     []string{"Eval"},
		SupportRefs: []string{"Eval @ internal/analysis/criterion/eval.go:15"},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "ev-eval",
			Kind:            types.EvidenceDirect,
			Subject:         "Eval",
			AnchorSymbol:    "Eval",
			AnchorKind:      types.AnchorDefinition,
			Source:          "internal/analysis/criterion/eval.go",
			LineStart:       15,
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
			Summary:         "Eval 对单个 Criterion 进行求值并返回 Result。",
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:      "public_functions",
			Kind:    types.BlockTable,
			Title:   "公开函数",
			Columns: []string{"符号名称", "定义位置", "说明"},
			Items: []types.AnswerBlockItem{{
				ID:          "eval",
				Label:       "Eval",
				CitationRef: -1,
			}},
		}},
	}

	if fixed := compileEnumerationDisplayTableRows(doc, ctx); fixed != 1 {
		t.Fatalf("fixed=%d, want 1", fixed)
	}
	table := doc.Blocks[0]
	if got, want := table.Columns, []string{"符号名称", "类别", "定义位置", "说明"}; len(got) != len(want) {
		t.Fatalf("columns=%v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("columns=%v, want %v", got, want)
			}
		}
	}
	item := table.Items[0]
	if item.Label != "Eval" {
		t.Fatalf("label changed: %#v", item)
	}
	if got := item.Cells; len(got) != 3 ||
		got[0] != "函数" ||
		got[1] != "internal/analysis/criterion/eval.go:15" ||
		got[2] != "Eval 对单个 Criterion 进行求值并返回 Result。" {
		t.Fatalf("cells not compiled from deterministic row: %#v", got)
	}
	if item.Text != "Eval 对单个 Criterion 进行求值并返回 Result。" {
		t.Fatalf("item text should preserve rich note for validators/rendering: %#v", item)
	}
	if item.CitationRef != 0 || len(doc.Citations) != 1 ||
		doc.Citations[0].File != "internal/analysis/criterion/eval.go" ||
		doc.Citations[0].Line != 15 {
		t.Fatalf("citation was not appended/reused: item=%#v citations=%#v", item, doc.Citations)
	}
}

func TestCompileEnumerationDisplayTableRows_PreservesModelItemTextOverDryRowNote(t *testing.T) {
	mut := types.NewMutableState("列出 Kind 常量")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "Kind 常量",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Unit:        "常量",
		Members:     []string{"KindExternalArtifactDecoded"},
		SupportRefs: []string{"KindExternalArtifactDecoded @ internal/analysis/criterion/grammar.go:65"},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentEnumerate,
				Language: "zh",
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "ev-external-artifact",
			Kind:            types.EvidenceDirect,
			Subject:         "KindExternalArtifactDecoded",
			AnchorSymbol:    "KindExternalArtifactDecoded",
			AnchorKind:      types.AnchorDefinition,
			Source:          "internal/analysis/criterion/grammar.go",
			LineStart:       65,
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
			Summary:         "Kind常量",
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:      "kind_constants",
			Kind:    types.BlockTable,
			Title:   "Kind 常量",
			Columns: []string{"符号名称", "定义位置", "说明"},
			Items: []types.AnswerBlockItem{{
				ID:          "external",
				Label:       "KindExternalArtifactDecoded",
				Text:        "Kind = Kind(types.CritExternalArtifactDecoded)，为兼容性保留，已废弃",
				CitationRef: -1,
			}},
		}},
	}

	if fixed := compileEnumerationDisplayTableRows(doc, ctx); fixed != 1 {
		t.Fatalf("fixed=%d, want 1", fixed)
	}
	item := doc.Blocks[0].Items[0]
	if item.Text != "Kind = Kind(types.CritExternalArtifactDecoded)，为兼容性保留，已废弃" {
		t.Fatalf("model-authored item text must remain authoritative: %#v", item)
	}
	if got := item.Cells; len(got) != 3 ||
		got[0] != "常量" ||
		got[1] != "internal/analysis/criterion/grammar.go:65" ||
		got[2] != "Kind = Kind(types.CritExternalArtifactDecoded)，为兼容性保留，已废弃" {
		t.Fatalf("compiled cells must carry model-authored note, got %#v", got)
	}
}

func TestCompileEnumerationDisplayTableRows_OmitsLocationColumnForNonFileRows(t *testing.T) {
	mut := types.NewMutableState("list runtime modes")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "运行时模式",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Unit:    "模式",
		Members: []string{"read", "write"},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:      "modes",
			Kind:    types.BlockTable,
			Columns: []string{"模式", "说明"},
			Items: []types.AnswerBlockItem{
				{ID: "read", Label: "read"},
				{ID: "write", Label: "write"},
			},
		}},
	}

	if fixed := compileEnumerationDisplayTableRows(doc, ctx); fixed != 2 {
		t.Fatalf("fixed=%d, want 2", fixed)
	}
	if got, want := doc.Blocks[0].Columns, []string{"符号名称", "类别"}; len(got) != len(want) {
		t.Fatalf("columns=%v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("columns=%v, want %v", got, want)
			}
		}
	}
	for _, item := range doc.Blocks[0].Items {
		if len(item.Cells) != 1 || item.Cells[0] != "模式" {
			t.Fatalf("non-file row should not get an empty location cell: %#v", item)
		}
		if item.CitationRef >= 0 || len(doc.Citations) != 0 {
			t.Fatalf("non-file row must not synthesize citation: item=%#v citations=%#v", item, doc.Citations)
		}
	}
}

func TestCompileEnumerationDisplayTableRows_PreservesMarkdownTableText(t *testing.T) {
	mut := types.NewMutableState("list public functions")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "公开函数",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"Eval"},
		SupportRefs: []string{"Eval @ internal/analysis/criterion/eval.go:15"},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentEnumerate},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:      "markdown",
			Kind:    types.BlockTable,
			Columns: []string{"A", "B", "C"},
			Text:    "| A | B | C |\n|---|---|---|\n| Eval | x | y |",
			Items: []types.AnswerBlockItem{{
				ID:    "eval",
				Label: "Eval",
			}},
		}},
	}

	if fixed := compileEnumerationDisplayTableRows(doc, ctx); fixed != 0 {
		t.Fatalf("fixed=%d, want 0", fixed)
	}
	if len(doc.Blocks[0].Items[0].Cells) != 0 {
		t.Fatalf("markdown table carrier should not be rewritten: %#v", doc.Blocks[0].Items[0].Cells)
	}
}

func TestCompileEnumerationDisplayTableRows_RequiresUniqueExactRowMatch(t *testing.T) {
	mut := types.NewMutableState("list public functions")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "公开函数",
		Value:       "2",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"Eval", "eval"},
		SupportRefs: []string{"Eval @ internal/a.go:1", "eval @ internal/b.go:2"},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentEnumerate},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:      "ambiguous",
			Kind:    types.BlockTable,
			Columns: []string{"Name", "Location", "Notes"},
			Items: []types.AnswerBlockItem{{
				ID:    "eval",
				Label: "EVAL",
			}},
		}},
	}

	if fixed := compileEnumerationDisplayTableRows(doc, ctx); fixed != 0 {
		t.Fatalf("fixed=%d, want 0", fixed)
	}
	if len(doc.Blocks[0].Items[0].Cells) != 0 {
		t.Fatalf("ambiguous/non-exact label must not be rewritten: %#v", doc.Blocks[0].Items[0].Cells)
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
