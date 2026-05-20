package tool

import (
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizePrincipalEnumerationRowBlocks_AppendsOnlyMissingRowsForPartialMarkdownTable(t *testing.T) {
	mu := types.NewMutableState("公开函数")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("is_registered", "IsRegistered", "internal/analysis/criterion/grammar.go", 100, "IsRegistered 判断给定 Kind 是否在 registered 集合中。"),
		enumEvidence("registered_kinds", "RegisteredKinds", "internal/analysis/criterion/grammar.go", 106, "RegisteredKinds 返回所有合法 Kind；顺序不稳定，需要 deterministic 输出的调用方自行排序。"),
		enumEvidence("eval", "Eval", "internal/analysis/criterion/eval.go", 15, "Eval 对单个 Criterion 求值，未知 Kind 通过 Result.UnknownKind 报告。"),
		enumEvidence("eval_all", "EvalAll", "internal/analysis/criterion/eval.go", 36, "EvalAll 批量求值所有 Criterion，返回 allOK 和 failed 列表。"),
		enumEvidence("set_floor", "SetExternalArtifactFloor", "internal/analysis/criterion/eval.go", 982, "SetExternalArtifactFloor 是兼容 legacy codrax.yaml 的配置入口。"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "公开函数（func）",
		Value:   "5",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"IsRegistered", "RegisteredKinds", "Eval", "EvalAll", "SetExternalArtifactFloor"},
		SupportRefs: []string{
			"IsRegistered @ internal/analysis/criterion/grammar.go:100",
			"RegisteredKinds @ internal/analysis/criterion/grammar.go:106",
			"Eval @ internal/analysis/criterion/eval.go:15",
			"EvalAll @ internal/analysis/criterion/eval.go:36",
			"SetExternalArtifactFloor @ internal/analysis/criterion/eval.go:982",
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			AnswerVisibilityProfile: &types.AnswerVisibilityProfile{
				SymbolVisibility: types.AnswerSymbolVisibilityPublicExported,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "公开函数（func）共 3 个：Eval、EvalAll、SetExternalArtifactFloor。",
		},
		{
			ID:    "funcs_table",
			Kind:  types.BlockTable,
			Title: "公开函数（func）— 3 个",
			Text:  "| 成员名称 | 签名 | 定义位置 |\n|---|---|---|\n| Eval | func Eval(...) | eval.go:15 |\n| EvalAll | func EvalAll(...) | eval.go:36 |\n| SetExternalArtifactFloor | func SetExternalArtifactFloor(f float64) | eval.go:982 |",
		},
	}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected deterministic principal enumeration normalization")
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("normalizer should preserve the authored table and append only missing rows: %+v", doc.Blocks)
	}
	if doc.Blocks[0].Text != "公开函数（func）共 3 个：Eval、EvalAll、SetExternalArtifactFloor。" {
		t.Fatalf("model-authored summary should be preserved; missing members belong in a supplement block: %q", doc.Blocks[0].Text)
	}
	table := doc.Blocks[1]
	if table.Text == "" || table.Kind != types.BlockTable || len(table.Items) != 0 {
		t.Fatalf("authored markdown table should be preserved, got: %+v", table)
	}
	supplement := doc.Blocks[2]
	if supplement.Text != "" || supplement.Kind != types.BlockTable || len(supplement.Items) != 2 {
		t.Fatalf("expected missing rows to be appended as a supplemental table: %+v", supplement)
	}
	if !strings.Contains(supplement.Title, "补充") {
		t.Fatalf("supplemental table title should mark local repair, got %q", supplement.Title)
	}
	joined := types.AnswerBlockVisibleSurface(supplement)
	for _, want := range []string{"IsRegistered", "RegisteredKinds", "deterministic 输出"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("supplemental table missing rich member surface %q:\n%s", want, joined)
		}
	}
	if len(doc.Citations) != 2 {
		t.Fatalf("expected one citation per missing principal row, got %+v", doc.Citations)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_SuppressesExactAbsenceSupplement(t *testing.T) {
	mu := types.NewMutableState("缺失配置键的三层覆盖")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "三层配置层中的缺席确认",
		Value:   "3",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"代码默认值层", "配置文件层", "CLI flag 层"},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentConfigQuery,
			Scenario: types.ScenarioConfigTrace,
			Language: "zh",
		}},
	}
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
		Blocks: []types.AnswerBlock{
			{
				ID:   "summary",
				Kind: types.BlockSummary,
				Text: "`explore_xyz_phantom_unique_budget` 在默认值、配置文件、CLI 三层均不存在。",
			},
			{
				ID:   "layers",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "l1", Label: "代码默认值层", Text: "没有为该精确键提供默认绑定。", CitationRef: -1},
					{ID: "l2", Label: "配置文件层", Text: "没有为该精确键提供配置项。", CitationRef: -1},
					{ID: "l3", Label: "CLI flag 层", Text: "没有为该精确键提供 CLI 覆盖。", CitationRef: -1},
				},
			},
		},
	}
	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed != 0 {
		t.Fatalf("exact-absence non-enumeration answers must not receive system member supplement, fixed=%d doc=%+v", fixed, doc.Blocks)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("system supplement should not be appended: %+v", doc.Blocks)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_UsesEnglishSupplementTitle(t *testing.T) {
	mu := types.NewMutableState("list exported functions")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("eval", "Eval", "internal/analysis/criterion/eval.go", 15, "Eval evaluates one Criterion."),
		enumEvidence("eval_all", "EvalAll", "internal/analysis/criterion/eval.go", 36, "EvalAll evaluates every Criterion and returns failed results."),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "exported functions",
		Value:       "2",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"Eval", "EvalAll"},
		SupportRefs: []string{"Eval @ internal/analysis/criterion/eval.go:15", "EvalAll @ internal/analysis/criterion/eval.go:36"},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "en",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:    "funcs",
		Kind:  types.BlockTable,
		Title: "Exported functions",
		Text:  "| Name | Location | Note |\n|---|---|---|\n| Eval | internal/analysis/criterion/eval.go:15 | Evaluates one criterion. |",
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected deterministic principal enumeration normalization")
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("expected summary, authored table, and English supplement, got %+v", doc.Blocks)
	}
	if !strings.Contains(doc.Blocks[2].Title, "System-verified member supplement") {
		t.Fatalf("supplement title should follow answer language, got %q", doc.Blocks[2].Title)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_SystemSupplementOmitsEmptyLocationAndNoteColumns(t *testing.T) {
	mu := types.NewMutableState("列出被硬编码覆盖的 yaml 字段")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "被 sameErrorClassRetryCap 硬编码覆盖的 YAML 配置字段",
		Value:   "3",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"shape_violation", "citation_violation", "other"},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "YAML 配置字段已在正文说明。",
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected deterministic principal enumeration normalization")
	}
	var supplement *types.AnswerBlock
	for i := range doc.Blocks {
		if doc.Blocks[i].Kind == types.BlockTable &&
			strings.Contains(doc.Blocks[i].Title, "被 sameErrorClassRetryCap 硬编码覆盖的 YAML 配置字段") {
			supplement = &doc.Blocks[i]
			break
		}
	}
	if supplement == nil {
		t.Fatalf("expected system supplement table, got %+v", doc.Blocks)
	}
	if !strings.Contains(supplement.Title, "系统按已验证证据补充成员") {
		t.Fatalf("system-generated table should be explicitly marked, got %q", supplement.Title)
	}
	if got, want := supplement.Columns, []string{"符号名称"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("member-only supplement must not expose empty location/note columns: %#v", got)
	}
	for _, item := range supplement.Items {
		if len(item.Cells) != 0 || strings.TrimSpace(item.Text) != "" || item.CitationRef >= 0 {
			t.Fatalf("member-only row should not synthesize empty cells/text/citation: %#v", item)
		}
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_SystemSupplementSkipsRowsThatWouldCreateBlankCells(t *testing.T) {
	mu := types.NewMutableState("梳理代码架构")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("eval", "Eval", "internal/analysis/criterion/eval.go", 15, "Eval 对单个 Criterion 求值。"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "系统补充候选",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Eval", "coverage-only"},
		SupportRefs: []string{
			"Eval @ internal/analysis/criterion/eval.go:15",
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioArchitectureExplain,
			Language: "zh",
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "正文已经解释架构。",
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected deterministic supplement for the renderable row")
	}
	var supplement *types.AnswerBlock
	for i := range doc.Blocks {
		if doc.Blocks[i].Kind == types.BlockTable &&
			strings.Contains(doc.Blocks[i].Title, "系统补充候选") {
			supplement = &doc.Blocks[i]
			break
		}
	}
	if supplement == nil {
		t.Fatalf("expected system supplement table, got %+v", doc.Blocks)
	}
	if len(supplement.Items) != 1 || supplement.Items[0].Label != "Eval" {
		t.Fatalf("coverage-only row should not render because it would create blank generated cells: %+v", supplement.Items)
	}
	for _, item := range supplement.Items {
		for _, cell := range item.Cells {
			if strings.TrimSpace(cell) == "" {
				t.Fatalf("system-generated table must not contain blank cells: %+v", supplement.Items)
			}
		}
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_SkipsScalarHistoryCountSupportMembers(t *testing.T) {
	mu := types.NewMutableState("过去 20 个提交里有多少次改过 runTaskGraph")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "匹配提交",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"commit a", "commit b"},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentReturnValue,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsScalarAnswer:  true,
				IsCountQuestion: true,
				IsHistoryLookup: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "答案是 2 次。",
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed != 0 {
		t.Fatalf("scalar history count support members must not generate principal rows; fixed=%d doc=%+v", fixed, doc.Blocks)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("system must not append commit/member tables to scalar count answers: %+v", doc.Blocks)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_PreservesAuthoredRichRowNotes(t *testing.T) {
	mu := types.NewMutableState("列出 Kind 常量")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("kind_symbol_present", "KindSymbolPresent", "internal/analysis/criterion/grammar.go", 29, "KindSymbolPresent常量定义"),
		enumEvidence("kind_no_call_sites", "KindNoCallSites", "internal/analysis/criterion/grammar.go", 30, "KindNoCallSites常量定义"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "Kind常量完整成员列表",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"KindSymbolPresent", "KindNoCallSites"},
		SupportRefs: []string{
			"KindSymbolPresent @ internal/analysis/criterion/grammar.go:29",
			"KindNoCallSites @ internal/analysis/criterion/grammar.go:30",
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:    "kinds",
		Kind:  types.BlockTable,
		Title: "Kind常量完整成员列表",
		Text: strings.Join([]string{
			"| Kind常量 | 定义位置 | 说明 |",
			"|---|---|---|",
			"| KindSymbolPresent | internal/analysis/criterion/grammar.go:29 | 符号存在性判定 |",
			"| KindNoCallSites | internal/analysis/criterion/grammar.go:30 | 调用点缺失判定 |",
		}, "\n"),
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected deterministic principal enumeration normalization")
	}
	var surfaces []string
	for _, block := range doc.Blocks {
		surfaces = append(surfaces, types.AnswerBlockVisibleSurface(block))
	}
	visible := strings.Join(surfaces, "\n")
	if !strings.Contains(visible, "符号存在性判定") || !strings.Contains(visible, "调用点缺失判定") {
		t.Fatalf("authored rich row notes should be preserved:\n%s", visible)
	}
	if strings.Contains(visible, "KindSymbolPresent常量定义") || strings.Contains(visible, "KindNoCallSites常量定义") {
		t.Fatalf("weak evidence summaries should not overwrite richer same-row finalizer text:\n%s", visible)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_CorrectsCoarseStructuredItemCitationRef(t *testing.T) {
	mu := types.NewMutableState("列出 Kind 常量")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("kind_symbol_present", "KindSymbolPresent", "internal/analysis/criterion/grammar.go", 29, "读模式 Kind：符号存在性判定。"),
		enumEvidence("kind_no_call_sites", "KindNoCallSites", "internal/analysis/criterion/grammar.go", 30, "读模式 Kind：无调用点判定。"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "Kind 常量",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"KindSymbolPresent", "KindNoCallSites"},
		SupportRefs: []string{
			"KindSymbolPresent @ internal/analysis/criterion/grammar.go:29",
			"KindNoCallSites @ internal/analysis/criterion/grammar.go:30",
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:    "kinds",
			Kind:  types.BlockOrderedList,
			Title: "Kind 常量",
			Items: []types.AnswerBlockItem{
				{ID: "k1", Label: "KindSymbolPresent", Text: "符号存在性判定", CitationRef: 0},
				{ID: "k2", Label: "KindNoCallSites", Text: "无调用点判定", CitationRef: 0},
			},
		}},
		Citations: []types.Citation{{File: "internal/analysis/criterion/grammar.go", Line: 28, Quote: "const ("}},
	}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected citation-only normalization")
	}
	block := answerDocumentTestBlockByID(t, doc, "kinds")
	if block.Items[0].Text != "符号存在性判定" || block.Items[1].Text != "无调用点判定" {
		t.Fatalf("citation normalization must not rewrite rich model text: %+v", block.Items)
	}
	if got := doc.Citations[block.Items[0].CitationRef]; got.File != "internal/analysis/criterion/grammar.go" || got.Line != 29 {
		t.Fatalf("first item citation not corrected to exact row: item=%+v citations=%+v", block.Items[0], doc.Citations)
	}
	if got := doc.Citations[block.Items[1].CitationRef]; got.File != "internal/analysis/criterion/grammar.go" || got.Line != 30 {
		t.Fatalf("second item citation not corrected to exact row: item=%+v citations=%+v", block.Items[1], doc.Citations)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_DoesNotCorrectExplicitConflictingItemLocation(t *testing.T) {
	mu := types.NewMutableState("列出 Kind 常量")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("kind_symbol_present", "KindSymbolPresent", "internal/analysis/criterion/grammar.go", 29, "读模式 Kind：符号存在性判定。"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "Kind 常量",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"KindSymbolPresent"},
		SupportRefs: []string{"KindSymbolPresent @ internal/analysis/criterion/grammar.go:29"},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:    "kinds",
			Kind:  types.BlockOrderedList,
			Title: "Kind 常量",
			Items: []types.AnswerBlockItem{{
				ID:          "k1",
				Label:       "KindSymbolPresent",
				Text:        "符号存在性判定，位置按 const block 说明见 internal/analysis/criterion/grammar.go:28",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{File: "internal/analysis/criterion/grammar.go", Line: 28, Quote: "const ("}},
	}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	block := answerDocumentTestBlockByID(t, doc, "kinds")
	if block.Items[0].CitationRef != 0 || len(doc.Citations) != 1 {
		t.Fatalf("explicit conflicting location should remain model-authored, got item=%+v citations=%+v", block.Items[0], doc.Citations)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_PreservesAuthoredNotesFromCategorizedMarkdownTable(t *testing.T) {
	mu := types.NewMutableState("按类型、函数、Kind 常量列出公开符号")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("kind_symbol_present", "KindSymbolPresent", "internal/analysis/criterion/grammar.go", 29, "KindSymbolPresent = Kind(types.CritSymbolPresent)"),
		enumEvidence("kind_no_call_sites", "KindNoCallSites", "internal/analysis/criterion/grammar.go", 30, "KindNoCallSites = Kind(types.CritNoCallSites)"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "Kind常量完整成员集",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"KindSymbolPresent", "KindNoCallSites"},
		SupportRefs: []string{
			"KindSymbolPresent @ internal/analysis/criterion/grammar.go:29",
			"KindNoCallSites @ internal/analysis/criterion/grammar.go:30",
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:    "all_symbols",
		Kind:  types.BlockTable,
		Title: "公开符号总表",
		Text: strings.Join([]string{
			"| 类别 | 符号名称 | 位置 | 说明 |",
			"|---|---|---|---|",
			"| **Kind 常量** | KindSymbolPresent | grammar.go:29 | read-mode：检查符号是否存在于 evidence |",
			"| | KindNoCallSites | grammar.go:30 | read-mode：检查符号无调用点 |",
		}, "\n"),
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected deterministic principal enumeration normalization")
	}
	visible := types.AnswerBlockVisibleSurface(doc.Blocks[1])
	if !strings.Contains(visible, "read-mode：检查符号是否存在") || !strings.Contains(visible, "read-mode：检查符号无调用点") {
		t.Fatalf("categorized markdown row notes should be preserved:\n%s", visible)
	}
	if strings.Contains(visible, "KindSymbolPresent = Kind(types.CritSymbolPresent)") {
		t.Fatalf("dry evidence note should not replace richer categorized markdown note:\n%s", visible)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_RemovesRedundantSectionShells(t *testing.T) {
	mu := types.NewMutableState("列出类型成员")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("kind", "Kind", "internal/analysis/criterion/grammar.go", 26, "Kind 是 Criterion 的公开类型别名。"),
		enumEvidence("env", "Env", "internal/analysis/criterion/grammar.go", 124, "Env 聚合评估 Criterion 时需要的运行环境。"),
		enumEvidence("result", "Result", "internal/analysis/criterion/eval.go", 12, "Result 是 Criterion 求值结果。"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "类型成员",
		Value:       "3",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"Kind", "Env", "Result"},
		SupportRefs: []string{"Kind @ internal/analysis/criterion/grammar.go:26", "Env @ internal/analysis/criterion/grammar.go:124", "Result @ internal/analysis/criterion/eval.go:12"},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	sectionText := "类型成员共 3 项；完整成员、定义位置和说明见对应表格。"
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "sec1", Kind: types.BlockSection, Title: "类型成员（3）", Text: sectionText},
		{ID: "sec2", Kind: types.BlockSection, Title: "类型成员（3）", Text: sectionText},
		{ID: "sec3", Kind: types.BlockSection, Title: "类型成员（3）", Text: sectionText},
		{
			ID:    "table",
			Kind:  types.BlockTable,
			Title: "类型成员",
			Items: []types.AnswerBlockItem{{ID: "kind", Label: "Kind"}, {ID: "env", Label: "Env"}, {ID: "result", Label: "Result"}},
		},
	}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected deterministic principal enumeration normalization")
	}
	sectionCount := 0
	for _, block := range doc.Blocks {
		if block.Kind == types.BlockSection && strings.Contains(block.Title, "类型成员") {
			sectionCount++
		}
	}
	if sectionCount != 1 {
		t.Fatalf("redundant section shells should collapse to one, got %d blocks: %+v", sectionCount, doc.Blocks)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_DoesNotAppendExcludedVariableSet(t *testing.T) {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{
		"Kind":           {{Name: "Kind", Kind: "type", File: "internal/analysis/criterion/grammar.go", Line: 26, Exported: true}},
		"ErrUnknownKind": {{Name: "ErrUnknownKind", Kind: "var", File: "internal/analysis/criterion/grammar.go", Line: 118, Exported: true}},
	}}
	mu := types.NewMutableState("公开符号不要列变量")
	mu.SetSearchGraph(graph)
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("kind", "Kind", "internal/analysis/criterion/grammar.go", 26, "Kind 是 string 的类型别名。"),
		enumEvidence("err", "ErrUnknownKind", "internal/analysis/criterion/grammar.go", 118, "ErrUnknownKind 是导出变量。"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "公开导出类型",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"Kind"},
		SupportRefs: []string{"Kind @ internal/analysis/criterion/grammar.go:26"},
	}, {
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "公开导出变量",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"ErrUnknownKind"},
		SupportRefs: []string{"ErrUnknownKind @ internal/analysis/criterion/grammar.go:118"},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
				IsExclusionRequested: true,
				ExcludedCandidateRoles: []types.AnswerCandidateRole{
					types.AnswerCandidateRoleVariable,
				},
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:    "types",
		Kind:  types.BlockTable,
		Title: "公开导出类型",
		Items: []types.AnswerBlockItem{{ID: "kind", Label: "Kind"}},
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected deterministic principal enumeration normalization")
	}
	var surfaces []string
	for _, block := range doc.Blocks {
		surfaces = append(surfaces, block.Title, types.AnswerBlockVisibleSurface(block))
	}
	visible := strings.Join(surfaces, "\n")
	if strings.Contains(visible, "公开导出变量") || strings.Contains(visible, "ErrUnknownKind") {
		t.Fatalf("excluded variable set leaked through deterministic compiler:\n%s", visible)
	}
	if !strings.Contains(visible, "公开导出类型") || !strings.Contains(visible, "Kind") {
		t.Fatalf("allowed principal type set was not preserved:\n%s", visible)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_DoesNotAppendSingletonCountBasisMetadata(t *testing.T) {
	mu := types.NewMutableState("按类型、函数列出公开符号，并说明 const block 数量依据")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("kind", "Kind", "internal/analysis/criterion/grammar.go", 26, "Kind 是公开类型别名。"),
		enumEvidence("env", "Env", "internal/analysis/criterion/grammar.go", 124, "Env 是公开结构体。"),
		enumEvidence("eval", "Eval", "internal/analysis/criterion/eval.go", 15, "Eval 是公开函数。"),
		enumEvidence("eval_all", "EvalAll", "internal/analysis/criterion/eval.go", 36, "EvalAll 是公开函数。"),
		enumEvidence("const_block", "const () block", "internal/analysis/criterion/grammar.go", 28, "一个 const() 块承载数量依据。"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:        types.AnswerAggregateMemberSet,
			Label:       "类型成员",
			Value:       "2",
			Role:        types.AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"Kind", "Env"},
			SupportRefs: []string{"Kind @ internal/analysis/criterion/grammar.go:26", "Env @ internal/analysis/criterion/grammar.go:124"},
		},
		{
			Kind:        types.AnswerAggregateMemberSet,
			Label:       "函数成员",
			Value:       "2",
			Role:        types.AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"Eval", "EvalAll"},
			SupportRefs: []string{"Eval @ internal/analysis/criterion/eval.go:15", "EvalAll @ internal/analysis/criterion/eval.go:36"},
		},
		{
			Kind:        types.AnswerAggregateMemberSet,
			Label:       "Kind const block 数量",
			Value:       "1",
			Role:        types.AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"const () block @ grammar.go:28-66"},
			SupportRefs: []string{"const () block @ internal/analysis/criterion/grammar.go:28"},
		},
	})
	mu.RetainInvestigationAggregateFacts()
	mu.SetEmittedAnswerSymbols([]types.AnswerSymbol{
		{Name: "Kind", Kind: types.KindType},
		{Name: "Env", Kind: types.KindType},
		{Name: "Eval", Kind: types.KindFunction},
		{Name: "EvalAll", Kind: types.KindFunction},
	}, types.CompletenessComplete)
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			CompletenessObligation: &types.CompletenessObligation{
				Required:    true,
				SourceQuote: "每一类必须列出完整成员名称",
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "类型 2 个、函数 2 个；const block 数量依据在说明中保留。",
		},
		{
			ID:    "all_symbols",
			Kind:  types.BlockTable,
			Title: "公开符号总表",
			Text: strings.Join([]string{
				"| 类别 | 符号名称 | 定义位置 | 说明 |",
				"|---|---|---|---|",
				"| 类型 | Kind | grammar.go:26 | Kind 是公开类型别名 |",
				"| 类型 | Env | grammar.go:124 | Env 是公开结构体 |",
				"| 函数 | Eval | eval.go:15 | Eval 是公开函数 |",
				"| 函数 | EvalAll | eval.go:36 | EvalAll 是公开函数 |",
			}, "\n"),
		},
		{
			ID:    "scope",
			Kind:  types.BlockSection,
			Title: "Kind const block 数量说明",
			Text:  "数量依据只作为范围说明：grammar.go:28-66 是一个 const() 块。",
		},
	}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected deterministic principal enumeration normalization")
	}
	var visible []string
	for _, block := range doc.Blocks {
		visible = append(visible, block.Title, types.AnswerBlockVisibleSurface(block))
	}
	joined := strings.Join(visible, "\n")
	if strings.Contains(joined, "const () block @") ||
		strings.Contains(joined, "| const () block") ||
		strings.Contains(joined, "系统按已验证证据补充缺失成员：Kind const block") {
		t.Fatalf("count-basis singleton metadata must not be appended as a principal row:\n%s", joined)
	}
	if !strings.Contains(joined, "Kind const block 数量说明") ||
		!strings.Contains(joined, "数量依据只作为范围说明") {
		t.Fatalf("model-authored count-basis explanation should be preserved:\n%s", joined)
	}
}

func enumEvidence(id, symbol, source string, line int, summary string) types.EvidenceItem {
	return types.EvidenceItem{
		ID:              id,
		Kind:            types.EvidenceDirect,
		Subject:         symbol,
		AnchorSymbol:    symbol,
		AnchorKind:      types.AnchorDefinition,
		Source:          source,
		LineStart:       line,
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
		Summary:         summary,
	}
}

func answerDocumentTestBlockByID(t *testing.T, doc *types.AnswerDocumentV2, id string) types.AnswerBlock {
	t.Helper()
	for _, block := range doc.Blocks {
		if block.ID == id {
			return block
		}
	}
	t.Fatalf("block %q not found in %+v", id, doc.Blocks)
	return types.AnswerBlock{}
}
