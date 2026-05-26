package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCompileCitationBackedTableRows_PreservesEmptyMultiColumnRows(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:      "kind_consts",
			Kind:    types.BlockTable,
			Title:   "Kind 常量",
			Columns: []string{"符号名称", "定义位置", "说明"},
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

	if fixed := compileCitationBackedTableRows(doc); fixed != 0 {
		t.Fatalf("fixed=%d, want 0", fixed)
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
	if got := table.Items[0].Cells; len(got) != 0 {
		t.Fatalf("system must not fill model-authored table cells: %#v", got)
	}
}

func TestCompileCitationBackedTableRows_PreservesCompatibleAuthoredTextRows(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:      "agents",
			Kind:    types.BlockTable,
			Title:   "Agent 清单",
			Columns: []string{"Agent 标识", "定义位置", "说明"},
			Items: []types.AnswerBlockItem{{
				ID:          "analyzer",
				Label:       "analyzer",
				Text:        "读模式核心分析器，驱动 StageAnalyze 阶段。",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{
			File: "internal/agent/analyzer.go",
			Line: 34,
		}},
	}

	if fixed := compileCitationBackedTableRows(doc); fixed != 0 {
		t.Fatalf("fixed=%d, want 0", fixed)
	}
	table := doc.Blocks[0]
	if got, want := table.Columns, []string{"Agent 标识", "定义位置", "说明"}; len(got) != len(want) {
		t.Fatalf("columns=%v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("columns=%v, want %v", got, want)
			}
		}
	}
	item := table.Items[0]
	if item.Text != "读模式核心分析器，驱动 StageAnalyze 阶段。" {
		t.Fatalf("model-authored text must remain authoritative: %#v", item)
	}
	if got := item.Cells; len(got) != 0 {
		t.Fatalf("system must not fill compatible-looking model-authored table cells: %#v", got)
	}
}

func TestCompileCitationBackedTableRows_RefusesToRewriteModelColumnShapeWithAuthoredText(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:      "agents",
			Kind:    types.BlockTable,
			Title:   "Agent 清单",
			Columns: []string{"Agent 标识", "作用/职责 (function_or_purpose)", "关系/协作 (relationship)"},
			Items: []types.AnswerBlockItem{{
				ID:          "analyzer",
				Label:       "analyzer",
				Text:        "读模式核心分析器。驱动 StageAnalyze 阶段，通过 emit_analysis 工具生成 AnalysisIR。",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{
			File: "internal/agent/analyzer.go",
			Line: 34,
		}},
	}

	if fixed := compileCitationBackedTableRows(doc); fixed != 0 {
		t.Fatalf("incompatible model-authored column shape must not be rewritten; fixed=%d doc=%+v", fixed, doc.Blocks[0])
	}
	table := doc.Blocks[0]
	if got, want := table.Columns, []string{"Agent 标识", "作用/职责 (function_or_purpose)", "关系/协作 (relationship)"}; len(got) != len(want) {
		t.Fatalf("columns should be preserved: %v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("columns should be preserved: %v, want %v", got, want)
			}
		}
	}
	item := table.Items[0]
	if len(item.Cells) != 0 ||
		item.Label != "analyzer" ||
		item.Text != "读模式核心分析器。驱动 StageAnalyze 阶段，通过 emit_analysis 工具生成 AnalysisIR。" ||
		item.CitationRef != 0 {
		t.Fatalf("model row surface must remain untouched: %+v", item)
	}
}

func TestCompileEnumerationDisplayTableRows_PreservesRowsDespitePrincipalEvidence(t *testing.T) {
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

	if fixed := compileEnumerationDisplayTableRows(doc, ctx); fixed != 0 {
		t.Fatalf("fixed=%d, want 0", fixed)
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
	item := table.Items[0]
	if item.Label != "Eval" {
		t.Fatalf("label changed: %#v", item)
	}
	if got := item.Cells; len(got) != 0 {
		t.Fatalf("system must not write deterministic row cells into the model table: %#v", got)
	}
	if item.Text != "" {
		t.Fatalf("system must not write deterministic row text into the model table: %#v", item)
	}
	if item.CitationRef != -1 || len(doc.Citations) != 0 {
		t.Fatalf("system must not rewrite row citations while preserving table content: item=%#v citations=%#v", item, doc.Citations)
	}
}

func TestCompileEnumerationDisplayTableRows_RefusesToRewriteModelColumnShape(t *testing.T) {
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
			Columns: []string{"类别", "符号名称", "定义位置", "说明"},
			Items: []types.AnswerBlockItem{{
				ID:          "eval",
				Label:       "Eval",
				CitationRef: -1,
			}},
		}},
	}

	if fixed := compileEnumerationDisplayTableRows(doc, ctx); fixed != 0 {
		t.Fatalf("incompatible model-authored column shape must not be rewritten inline; fixed=%d doc=%+v", fixed, doc.Blocks[0])
	}
	table := doc.Blocks[0]
	if got, want := table.Columns, []string{"类别", "符号名称", "定义位置", "说明"}; len(got) != len(want) {
		t.Fatalf("columns should be preserved: %v, want %v", got, want)
	}
	if len(table.Items[0].Cells) != 0 || table.Items[0].Label != "Eval" {
		t.Fatalf("model row surface must remain untouched: %+v", table.Items[0])
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

	if fixed := compileEnumerationDisplayTableRows(doc, ctx); fixed != 0 {
		t.Fatalf("fixed=%d, want 0", fixed)
	}
	item := doc.Blocks[0].Items[0]
	if item.Text != "Kind = Kind(types.CritExternalArtifactDecoded)，为兼容性保留，已废弃" {
		t.Fatalf("model-authored item text must remain authoritative: %#v", item)
	}
	if got := item.Cells; len(got) != 0 {
		t.Fatalf("system must not turn model-authored text into table cells: %#v", got)
	}
	if item.CitationRef != -1 || len(doc.Citations) != 0 {
		t.Fatalf("system must not rewrite row citations while preserving table content: item=%#v citations=%#v", item, doc.Citations)
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
				{ID: "read", Label: "read", CitationRef: -1},
				{ID: "write", Label: "write", CitationRef: -1},
			},
		}},
	}

	if fixed := compileEnumerationDisplayTableRows(doc, ctx); fixed != 0 {
		t.Fatalf("label-only rows without location/note should be left as model-authored surface; fixed=%d", fixed)
	}
	if got, want := doc.Blocks[0].Columns, []string{"模式", "说明"}; len(got) != len(want) {
		t.Fatalf("columns should be preserved: %v, want %v", got, want)
	}
	for _, item := range doc.Blocks[0].Items {
		if len(item.Cells) != 0 {
			t.Fatalf("non-file row must not be rewritten into synthetic cells: %#v", item)
		}
		if item.CitationRef >= 0 || len(doc.Citations) != 0 {
			t.Fatalf("non-file row must not synthesize citation: item=%#v citations=%#v", item, doc.Citations)
		}
	}
}

func TestCompileEnumerationDisplayTableRows_RefusesMixedRowsThatWouldCreateBlankCells(t *testing.T) {
	mut := types.NewMutableState("list public functions")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "ev-eval",
		Kind:            types.EvidenceDirect,
		Subject:         "Eval",
		AnchorSymbol:    "Eval",
		AnchorKind:      types.AnchorDefinition,
		Source:          "internal/analysis/criterion/eval.go",
		LineStart:       15,
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
		Summary:         "Eval 对单个 Criterion 求值。",
	}})
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "公开函数",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Unit:    "函数",
		Members: []string{"Eval", "coverage-only"},
		SupportRefs: []string{
			"Eval @ internal/analysis/criterion/eval.go:15",
		},
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
			Summary:         "Eval 对单个 Criterion 求值。",
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:      "public_functions",
			Kind:    types.BlockTable,
			Title:   "公开函数",
			Columns: []string{"符号名称", "定义位置", "说明"},
			Items: []types.AnswerBlockItem{
				{ID: "eval", Label: "Eval"},
				{ID: "coverage", Label: "coverage-only"},
			},
		}},
	}

	if fixed := compileEnumerationDisplayTableRows(doc, ctx); fixed != 0 {
		t.Fatalf("mixed complete/incomplete rows must not be rewritten into a table with blank cells; fixed=%d doc=%+v", fixed, doc.Blocks[0])
	}
	if len(doc.Blocks[0].Items[0].Cells) != 0 || len(doc.Blocks[0].Items[1].Cells) != 0 {
		t.Fatalf("table should remain untouched when safe repair is impossible: %+v", doc.Blocks[0].Items)
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
