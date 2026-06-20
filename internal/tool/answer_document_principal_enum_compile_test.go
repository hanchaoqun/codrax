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
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Label: "说明",
					Role:  types.RequestedAnswerDimensionFunctionOrPurpose,
					Index: 1,
				}},
			},
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
	if len(doc.Blocks) != 2 {
		t.Fatalf("normalizer should preserve the authored table without appending a competing system table: %+v", doc.Blocks)
	}
	if doc.Blocks[0].Text != "公开函数（func）共 3 个：Eval、EvalAll、SetExternalArtifactFloor。" {
		t.Fatalf("model-authored summary should be preserved: %q", doc.Blocks[0].Text)
	}
	table := doc.Blocks[1]
	if table.Text == "" || table.Kind != types.BlockTable || len(table.Items) != 0 {
		t.Fatalf("authored markdown table should be preserved, got: %+v", table)
	}
	if joined := answerDocumentTestVisibleSurface(doc); strings.Contains(joined, "系统按已验证证据补充") {
		t.Fatalf("authored enum carrier must suppress system补表 even when rows are incomplete:\n%s", joined)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_ParentheticalCategoryCarrierSuppressesSupplement(t *testing.T) {
	mu := types.NewMutableState("read-mode pipeline stages")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("fallback_reset", "FallbackResetTarget", "internal/types/context.go", 2709, "FallbackResetTarget is a reset-depth enum used by fallback handling."),
		enumEvidence("stage_analyze", "StageAnalyze", "internal/types/enums.go", 33, "StageAnalyze is the first read-mode main stage."),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:        types.AnswerAggregateMemberSet,
			Label:       "条件性前置 stage（ADVISORY）",
			Value:       "1",
			Role:        types.AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"FallbackResetTarget"},
			SupportRefs: []string{"FallbackResetTarget @ internal/types/context.go:2709"},
		},
		{
			Kind:        types.AnswerAggregateMemberSet,
			Label:       "无条件主管道",
			Value:       "1",
			Role:        types.AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"StageAnalyze"},
			SupportRefs: []string{"StageAnalyze @ internal/types/enums.go:33"},
		},
	})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioArchitectureExplain,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:    "pre",
			Kind:  types.BlockSection,
			Title: "条件性前置 stage（按需执行）",
			Text:  "这一层已经说明 StageLogTriage 和 StagePerfTriage 会在 analyze 前按需运行。",
		},
		{
			ID:    "main",
			Kind:  types.BlockSection,
			Title: "无条件主管道",
			Text:  "主管道从 StageAnalyze 进入，随后继续 explore/extract/finalize。",
		},
	}}

	_ = normalizePrincipalEnumerationRowBlocks(doc, ctx)
	visible := answerDocumentTestVisibleSurface(doc)
	if strings.Contains(visible, "系统按已验证证据补充") ||
		strings.Contains(visible, "FallbackResetTarget") {
		t.Fatalf("parenthetical category carrier should suppress a competing supplement:\n%s", visible)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_SectionItemsSuppressSystemSupplement(t *testing.T) {
	mu := types.NewMutableState("列出 ArkTS 页面入口和 Builder 片段")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("index", "Index", "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", 7, "Index 是被 @Entry 标记的页面入口。"),
		enumEvidence("list", "ListPage", "internal/thirdparty/tree-sitter-arkts/corpus/sources/05_foreach_lazyforeach.ets", 32, "ListPage 是被 @Entry 标记的页面入口。"),
		enumEvidence("header", "defaultHeader", "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets", 8, "defaultHeader 是 @Builder 方法复用片段。"),
		enumEvidence("card", "GlobalCard", "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets", 26, "GlobalCard 是 @Builder 函数复用片段。"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:        types.AnswerAggregateMemberSet,
			Label:       "@Entry 标记的 ArkTS 页面入口",
			Value:       "2",
			Role:        types.AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"Index", "ListPage"},
			SupportRefs: []string{"Index @ internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets:7", "ListPage @ internal/thirdparty/tree-sitter-arkts/corpus/sources/05_foreach_lazyforeach.ets:32"},
		},
		{
			Kind:        types.AnswerAggregateMemberSet,
			Label:       "@Builder 复用片段",
			Value:       "2",
			Role:        types.AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"defaultHeader", "GlobalCard"},
			SupportRefs: []string{"defaultHeader @ internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets:8", "GlobalCard @ internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets:26"},
		},
	})
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
		Blocks: []types.AnswerBlock{
			{
				ID:   "summary",
				Kind: types.BlockSummary,
				Text: "仓库中包含 2 个 @Entry 页面入口和 2 个 @Builder 复用片段。",
			},
			{
				ID:    "entry",
				Kind:  types.BlockSection,
				Title: "@Entry 标记的 ArkTS 页面入口",
				Items: []types.AnswerBlockItem{
					{ID: "index", Label: "Index", Text: "页面入口组件。", CitationRef: 0},
					{ID: "list", Label: "ListPage", Text: "列表页面入口组件。", CitationRef: 1},
				},
			},
			{
				ID:    "builder",
				Kind:  types.BlockSection,
				Title: "@Builder 复用片段",
				Items: []types.AnswerBlockItem{
					{ID: "header", Label: "defaultHeader", Text: "方法级复用片段。", CitationRef: 2},
					{ID: "card", Label: "GlobalCard", Text: "函数级复用片段。", CitationRef: 3},
				},
			},
		},
		Citations: []types.Citation{
			{File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", Line: 7},
			{File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/05_foreach_lazyforeach.ets", Line: 32},
			{File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets", Line: 8},
			{File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets", Line: 26},
		},
	}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected deterministic annotation/citation normalization")
	}
	visible := answerDocumentTestVisibleSurface(doc)
	if strings.Contains(visible, "系统按已验证证据补充成员") {
		t.Fatalf("section item enum carrier should suppress duplicate system supplement:\n%s", visible)
	}
	for _, blockID := range []string{"entry", "builder"} {
		var found bool
		for _, block := range doc.Blocks {
			if block.ID != blockID {
				continue
			}
			found = true
			if block.SurfaceRole != types.SurfacePrincipal ||
				!testStringSliceContains(block.FacetIDs, string(types.FacetEnumerationItem)) {
				t.Fatalf("section carrier %s should be annotated as principal enumeration: %+v", blockID, block)
			}
		}
		if !found {
			t.Fatalf("section carrier %s missing after normalization: %+v", blockID, doc.Blocks)
		}
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_RedlineDoesNotSynthesizeSummaryForAuthoredCarriers(t *testing.T) {
	mu := types.NewMutableState("列出公开函数")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("eval", "Eval", "internal/analysis/criterion/eval.go", 15, "Eval 对单个 Criterion 求值并返回 Result。"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "公开函数",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"Eval"},
		SupportRefs: []string{"Eval @ internal/analysis/criterion/eval.go:15"},
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
	cases := []struct {
		name   string
		blocks []types.AnswerBlock
	}{
		{
			name: "markdown_table",
			blocks: []types.AnswerBlock{{
				ID:    "model_table",
				Kind:  types.BlockTable,
				Title: "模型给出的公开函数表",
				Text:  "| 函数 | 说明 |\n|---|---|\n| Eval | 对单个 Criterion 求值。 |",
			}},
		},
		{
			name: "structured_list",
			blocks: []types.AnswerBlock{{
				ID:    "model_list",
				Kind:  types.BlockOrderedList,
				Title: "模型给出的公开函数列表",
				Items: []types.AnswerBlockItem{{
					ID:    "eval",
					Label: "Eval",
					Text:  "对单个 Criterion 求值并返回 Result。",
				}},
			}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: append([]types.AnswerBlock(nil), tc.blocks...)}
			normalizePrincipalEnumerationRowBlocks(doc, ctx)
			if len(doc.Blocks) != 1 {
				t.Fatalf("model-authored carrier must remain the only visible block; got %+v", doc.Blocks)
			}
			visible := answerDocumentTestVisibleSurface(doc)
			if strings.Contains(visible, "本轮按已验收的结构化调查清单列出") ||
				strings.Contains(visible, "系统按已验证证据补充") {
				t.Fatalf("system structural preference leaked into authored answer surface:\n%s", visible)
			}
			if !strings.Contains(visible, "Eval") || !strings.Contains(visible, "Criterion") {
				t.Fatalf("model-authored content should be preserved:\n%s", visible)
			}
		})
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_AppendsOnlyMissingRowsForCorruptMarkdownSourceInventoryTable(t *testing.T) {
	mu := types.NewMutableState("列出公开字符串枚举类型")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "公开字符串枚举类型",
		Value:   "3",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Intent", "QuestionFamily", "AnswerRequestedOutput"},
		SupportRefs: []string{
			"Intent @ internal/types/analysis_ir.go:847",
			"QuestionFamily @ internal/types/facet_plan.go:48",
			"AnswerRequestedOutput @ internal/types/answer_intent_contract.go:61",
		},
		MemberNotes: []string{
			"Intent classifies the user's request intent.",
			"QuestionFamily names the broad answer family.",
			"AnswerRequestedOutput names the visible answer surface the user requested.",
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
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
				TypeUnderlying:    types.SourceInventoryTypeUnderlyingString,
				RequiresConstSet:  true,
				RequestedFields:   []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation},
				Confidence:        0.95,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "internal/types 包中共有 3 个公开字符串枚举类型。",
		},
		{
			ID:    "enum_table",
			Kind:  types.BlockTable,
			Title: "公开字符串枚举类型",
			Text: strings.Join([]string{
				"| 类型名 | 文件位置 | 说明 |",
				"|---|---|---|",
				"| Intent | internal/types/analysis_ir.go:847 | 用户意图分类 |",
				"| Intent | internal/types/analysis_ir.go:847 | 重复行，应被结构化清理 |",
				"| AnswerAnswerRequestedOutput | internal/types/answer_intent_contract.go:61 | 错误标签 |",
			}, "\n"),
		},
	}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected source-inventory missing-row supplement")
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("near-complete source inventory table should preserve model output without competing system rows: %+v", doc.Blocks)
	}
	modelTable := answerDocumentTestBlockByID(t, doc, "enum_table")
	if strings.TrimSpace(modelTable.Text) == "" || len(modelTable.Items) != 0 {
		t.Fatalf("model-authored markdown table should remain separate, got %+v", modelTable)
	}
	modelVisible := types.AnswerBlockVisibleSurface(modelTable)
	for _, want := range []string{"Intent", "AnswerAnswerRequestedOutput", "重复行，应被结构化清理"} {
		if !strings.Contains(modelVisible, want) {
			t.Fatalf("model table should be preserved with authored content %q:\n%s", want, modelVisible)
		}
	}
	if visible := answerDocumentTestVisibleSurface(doc); strings.Contains(visible, "系统按已验证证据补充") {
		t.Fatalf("model-authored source-inventory table must suppress system补表:\n%s", visible)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_AppendsSupplementForIncompatibleStructuredTable(t *testing.T) {
	mu := types.NewMutableState("列出公开函数")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("eval", "Eval", "internal/analysis/criterion/eval.go", 15, "Eval 对单个 Criterion 求值并返回 Result。"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "公开函数",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Unit:        "函数",
		Members:     []string{"Eval"},
		SupportRefs: []string{"Eval @ internal/analysis/criterion/eval.go:15"},
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
		ID:      "model_structured_table",
		Kind:    types.BlockTable,
		Title:   "模型给出的公开函数表",
		Columns: []string{"类别", "符号名称", "定义位置", "说明"},
		Items: []types.AnswerBlockItem{{
			ID:    "eval",
			Label: "Eval",
		}},
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected separated deterministic supplement for incompatible structured table")
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("authored structured table should not receive a synthetic system summary or competing table; got %+v", doc.Blocks)
	}
	model := answerDocumentTestBlockByID(t, doc, "model_structured_table")
	if len(model.Items) != 1 || len(model.Items[0].Cells) != 0 ||
		len(model.Columns) != 4 || model.Columns[0] != "类别" {
		t.Fatalf("model-authored structured table must remain untouched: %+v", model)
	}
	if visible := answerDocumentTestVisibleSurface(doc); strings.Contains(visible, "系统按已验证证据补充") {
		t.Fatalf("incompatible model table must not trigger a competing system table:\n%s", visible)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_UsesEnglishFieldSupplementForIncompatibleStructuredTable(t *testing.T) {
	mu := types.NewMutableState("list exported functions")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("eval", "Eval", "internal/analysis/criterion/eval.go", 15, "Eval evaluates one Criterion and returns Result."),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "exported functions",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"Eval"},
		SupportRefs: []string{"Eval @ internal/analysis/criterion/eval.go:15"},
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
		ID:      "model_structured_table",
		Kind:    types.BlockTable,
		Title:   "Model-authored exported function table",
		Columns: []string{"Category", "Name", "Location", "Description"},
		Items: []types.AnswerBlockItem{{
			ID:    "eval",
			Label: "Eval",
		}},
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected separated deterministic field supplement")
	}
	model := answerDocumentTestBlockByID(t, doc, "model_structured_table")
	if len(model.Columns) != 4 || model.Columns[0] != "Category" ||
		len(model.Items) != 1 || model.Items[0].Label != "Eval" ||
		len(model.Items[0].Cells) != 0 {
		t.Fatalf("model-authored structured table must remain untouched: %+v", model)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("authored structured table should not receive a synthetic system summary or competing table; got %+v", doc.Blocks)
	}
	if visible := answerDocumentTestVisibleSurface(doc); strings.Contains(visible, "System-verified") {
		t.Fatalf("English model table must suppress system supplement:\n%s", visible)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_AppendsMissingRowsForCorruptCompleteAttempt(t *testing.T) {
	mu := types.NewMutableState("列出公开字符串枚举类型")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("alpha", "AlphaKind", "internal/types/a.go", 10, "AlphaKind 表示第一类枚举。"),
		enumEvidence("beta", "BetaKind", "internal/types/b.go", 20, "BetaKind 表示第二类枚举。"),
		enumEvidence("gamma", "GammaKind", "internal/types/c.go", 30, "GammaKind 表示第三类枚举。"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "公开字符串枚举类型",
		Value:   "3",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"AlphaKind", "BetaKind", "GammaKind"},
		SupportRefs: []string{
			"AlphaKind @ internal/types/a.go:10",
			"BetaKind @ internal/types/b.go:20",
			"GammaKind @ internal/types/c.go:30",
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Label: "说明",
					Role:  types.RequestedAnswerDimensionFunctionOrPurpose,
					Index: 1,
				}},
			},
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:    "model_table",
		Kind:  types.BlockTable,
		Title: "模型整理的枚举类型表",
		Text: strings.Join([]string{
			"| 类型名 | 文件位置 |",
			"|---|---|",
			"| AlphaKind | internal/types/a.go:10 |",
			"| AlphaKind | internal/types/a.go:10 |",
			"| GammaKindTypo | internal/types/c.go:30 |",
		}, "\n"),
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected deterministic missing-row supplement for corrupt complete table")
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("authored markdown table should not receive a synthetic system summary or competing table; got %+v", doc.Blocks)
	}
	if !strings.Contains(types.AnswerBlockVisibleSurface(doc.Blocks[0]), "GammaKindTypo") {
		t.Fatalf("model-authored table should remain visibly separate, got %+v", doc.Blocks[0])
	}
	if visible := answerDocumentTestVisibleSurface(doc); strings.Contains(visible, "系统按已验证证据补充") {
		t.Fatalf("corrupt but authored table must not be overwritten by system补表:\n%s", visible)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_DoesNotPromoteMechanismMemberSet(t *testing.T) {
	mu := types.NewMutableState("analyze retry mechanism")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "analyze 阶段重试退出路径",
		Value: "4",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			`Error=="" 且 AnalysisIR!=nil → 成功退出`,
			`autoCorrectAnalyzerStageOutput 成功 → 清除 Error 并成功退出`,
			`retry storm（同 fingerprint ≥ ceil(max/2) 次）→ 提前退出降级路径`,
			`重试预算耗尽 → 降级路径（安装最小非零 AnalysisIR）`,
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentExplain,
			Scenario:      types.ScenarioArchitectureExplain,
			Complexity:    types.ComplexityComplex,
			Language:      "zh",
			PredicateAxis: types.AxisCondition,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			SubTopics: []types.SubTopic{
				{Summary: "retry budget", Entities: []string{"MaxRetriesPerStage"}},
				{Summary: "stage output", Entities: []string{"StageOutput"}},
				{Summary: "fallback", Entities: []string{"AnalysisIR"}},
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:    "summary",
		Kind:  types.BlockSummary,
		Title: "analyze 阶段重试机制",
		Text:  `重试循环在 Error=="" 且 AnalysisIR!=nil 时退出；失败后可能经过 autoCorrect、retry storm 或降级路径。`,
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed != 0 {
		t.Fatalf("single-topic mechanism member_set should stay support-only, fixed=%d doc=%+v", fixed, doc.Blocks)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("normalizer must not append mechanism member-set补表, got %+v", doc.Blocks)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_SuppressesGroupedCountComparisonSupplement(t *testing.T) {
	mu := types.NewMutableState("对比两个子系统的节点类型数量和职责差异")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateGroupedCount,
		Label:   "TaskGraph 节点类型数量（单 SubTopic）",
		Value:   "1",
		Unit:    "node_types",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"final"},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:     types.IntentExplain,
			Scenario:   types.ScenarioArchitectureExplain,
			Complexity: types.ComplexityComplex,
			Language:   "zh",
			Predicates: types.SemanticPredicates{
				IsCrossComponent: true,
			},
			Buckets: []types.QuestionBucket{{Label: "子系统 A"}, {Label: "子系统 B"}},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "两个子系统都只有一个 final 节点类型，但职责边界不同，不能把这个计数分区渲染成主成员清单。",
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed != 0 {
		t.Fatalf("comparison grouped_count members are support partitions, not supplement rows; fixed=%d doc=%+v", fixed, doc.Blocks)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("normalizer must not append one-row count member supplement, got %+v", doc.Blocks)
	}
	visible := types.AnswerBlockVisibleSurface(doc.Blocks[0])
	if !strings.Contains(visible, "职责边界不同") {
		t.Fatalf("model-authored comparison prose should remain dominant:\n%s", visible)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_SuppressesCountMetadataBehindRicherMemberSet(t *testing.T) {
	mu := types.NewMutableState("internal/analysis/criterion 对外导出的全部 API（函数、类型、常量、变量）共有哪些？分类列出。")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:    types.AnswerAggregateGroupedCount,
			Label:   "Kind 值按阶段分组",
			Value:   "3",
			Unit:    "groups",
			Role:    types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{"读模式 Kind (18个)", "写模式 Kind (5个)", "兼容性保留 Kind (1个)"},
		},
		{
			Kind:  types.AnswerAggregateMemberSet,
			Label: "Kind 常量成员",
			Value: "4",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{
				"KindSymbolPresent @ internal/analysis/criterion/grammar.go:29",
				"KindNoCallSites @ internal/analysis/criterion/grammar.go:30",
				"KindAnswerSetBounded @ internal/analysis/criterion/grammar.go:31",
				"KindExternalArtifactDecoded @ internal/analysis/criterion/grammar.go:65",
			},
		},
	})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "全部公开 API"},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleConstant},
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "Kind 常量成员已经在表格中列出。",
		},
		{
			ID:          "kind_members",
			Kind:        types.BlockTable,
			SurfaceRole: types.SurfacePrincipal,
			Title:       "Kind 常量成员",
			Columns:     []string{"符号名称", "定义位置"},
			Items: []types.AnswerBlockItem{
				{ID: "k1", Label: "KindSymbolPresent", Text: "internal/analysis/criterion/grammar.go:29"},
				{ID: "k2", Label: "KindNoCallSites", Text: "internal/analysis/criterion/grammar.go:30"},
				{ID: "k3", Label: "KindAnswerSetBounded", Text: "internal/analysis/criterion/grammar.go:31"},
				{ID: "k4", Label: "KindExternalArtifactDecoded", Text: "internal/analysis/criterion/grammar.go:65"},
			},
		},
	}}

	_ = normalizePrincipalEnumerationRowBlocks(doc, ctx)
	var surfaces []string
	for _, block := range doc.Blocks {
		surfaces = append(surfaces, block.Title, types.AnswerBlockVisibleSurface(block))
	}
	visible := strings.Join(surfaces, "\n")
	if strings.Contains(visible, "Kind 值按阶段分组") ||
		strings.Contains(visible, "读模式 Kind (18个)") ||
		strings.Contains(visible, "系统按已验证证据补充成员：Kind 值按阶段分组") {
		t.Fatalf("count/group metadata must not materialize over richer member rows:\n%s", visible)
	}
	if got := strings.Count(visible, "KindSymbolPresent"); got != 1 {
		t.Fatalf("model-authored member table should remain the visible carrier, occurrences=%d:\n%s", got, visible)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_DecoratedMemberCoveredByCitedBareLabel(t *testing.T) {
	mu := types.NewMutableState("列出 internal/analysis/criterion 对外导出的变量")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "导出包级变量",
		Value:   "1",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"ErrUnknownKind (grammar.go:118, error)"},
		SupportRefs: []string{
			"ErrUnknownKind: internal/analysis/criterion/grammar.go:118",
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
			CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "导出的变量"},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:          "vars",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				Items: []types.AnswerBlockItem{{
					ID:          "v1",
					Label:       "ErrUnknownKind",
					Text:        "导出包级变量，类型为 error。",
					CitationRef: 0,
				}},
			},
		},
		Citations: []types.Citation{{File: "internal/analysis/criterion/grammar.go", Line: 118}},
	}

	sets := types.CompileEnumerationDisplaySets(&ctx.AnalysisIR.RequestModel, answerSurfacePlan(ctx))
	if len(sets) != 1 || len(sets[0].Rows) != 1 {
		t.Fatalf("expected one compiled decorated row, got %+v", sets)
	}
	if !principalEnumerationStructuredItemCoversRow(doc.Blocks[0].Items[0], doc, sets[0].Rows[0]) {
		t.Fatalf("decorated row should be covered by cited bare label item: row=%+v item=%+v citations=%+v", sets[0].Rows[0], doc.Blocks[0].Items[0], doc.Citations)
	}
	_ = normalizePrincipalEnumerationRowBlocks(doc, ctx)
	var surfaces []string
	for _, block := range doc.Blocks {
		surfaces = append(surfaces, block.Title, types.AnswerBlockVisibleSurface(block))
	}
	visible := strings.Join(surfaces, "\n")
	if strings.Contains(visible, "系统按已验证证据补充成员：导出包级变量") {
		t.Fatalf("cited bare-label item already covers decorated member; no duplicate supplement expected:\n%s", visible)
	}
	if got := strings.Count(visible, "ErrUnknownKind"); got != 1 {
		t.Fatalf("expected one visible ErrUnknownKind row, got %d:\n%s", got, visible)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_CommaLineMemberCoveredByVisibleCaller(t *testing.T) {
	mu := types.NewMutableState("哪些生产代码函数调用了 appendTypedRelationKinds？")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "appendTypedRelationKinds 的生产代码调用者",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"BuildTypedRelationQueryWithResolvedSources (internal/types/typed_relation_hint.go:182, 183)",
			"TypedRelationKindsForRequest (internal/types/typed_relation_hint.go:209)",
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
				IsRelationalLookup:    true,
			},
			PredicateAxis: types.AxisCall,
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:          "callers",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				Items: []types.AnswerBlockItem{
					{ID: "c1", Label: "BuildTypedRelationQueryWithResolvedSources", Text: "在第 182 和 183 行两次调用 appendTypedRelationKinds。", CitationRef: 0},
					{ID: "c2", Label: "TypedRelationKindsForRequest", Text: "在第 209 行调用 appendTypedRelationKinds。", CitationRef: 1},
				},
			},
		},
		Citations: []types.Citation{
			{File: "internal/types/typed_relation_hint.go", Line: 182},
			{File: "internal/types/typed_relation_hint.go", Line: 209},
		},
	}

	_ = normalizePrincipalEnumerationRowBlocks(doc, ctx)
	var surfaces []string
	for _, block := range doc.Blocks {
		surfaces = append(surfaces, block.Title, types.AnswerBlockVisibleSurface(block))
	}
	visible := strings.Join(surfaces, "\n")
	if strings.Contains(visible, "系统按已验证证据补充缺失成员") {
		t.Fatalf("visible caller list already covers comma-line aggregate members; no duplicate supplement expected:\n%s", visible)
	}
	if got := strings.Count(visible, "BuildTypedRelationQueryWithResolvedSources"); got != 1 {
		t.Fatalf("expected one visible BuildTypedRelationQueryWithResolvedSources row, got %d:\n%s", got, visible)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_DoesNotDuplicateVCSCommitRowsAlreadyVisible(t *testing.T) {
	mu := types.NewMutableState("最近 10 次提交都做了哪些事情")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "最近10次提交",
		Value: "3",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"ae1dd6b256fab219104c09447b6ffe3697239b7a: evidence: unify vcs provenance lanes (34 files, +1521/-76)",
			"3ae8465b6afe3fb16902d511d51482fefd09a103: orchestrator: route retries through claim bindings (8 files, +356/-7)",
			"125687ab6f1ff7cd1187183fc459efe65be10fb3: orchestrator: suppress generic caveats after semantic pass (5 files, +170/-8)",
		},
		Dimensions: []types.AnswerAggregateDimension{{Name: "evidence_origin", Value: "vcs_metadata"}},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "以下是仓库最近 3 次提交。",
		},
		{
			ID:          "commits",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{
				{ID: "c1", Label: "ae1dd6b256fab219104c09447b6ffe3697239b7a", Text: "统一 VCS 证据通道。"},
				{ID: "c2", Label: "3ae8465b6afe3fb16902d511d51482fefd09a103", Text: "将重试路由绑定到 claim 机制。"},
				{ID: "c3", Label: "125687ab6f1ff7cd1187183fc459efe65be10fb3", Text: "语义审查通过后抑制通用 caveats。"},
			},
		},
	}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected normalization to annotate the existing principal blocks")
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("visible commit rows already cover the member_set; no duplicate supplement expected: %+v", doc.Blocks)
	}
	for _, block := range doc.Blocks {
		if strings.Contains(block.Title, "系统按已验证证据补充成员") {
			t.Fatalf("must not append duplicate VCS member supplement when labels already cover commit hashes: %+v", block)
		}
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_DoesNotDuplicateShortVCSHashes(t *testing.T) {
	mu := types.NewMutableState("最近 10 次提交都做了哪些事情")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "最近提交",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"ae1dd6b256fab219104c09447b6ffe3697239b7a",
			"3ae8465b6afe3fb16902d511d51482fefd09a103",
		},
		Dimensions: []types.AnswerAggregateDimension{{Name: "evidence_origin", Value: "vcs_metadata"}},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Language: "zh",
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Label: "特性",
					Role:  types.RequestedAnswerDimensionFunctionOrPurpose,
					Index: 1,
				}},
			},
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:          "summary",
		Kind:        types.BlockSummary,
		SurfaceRole: types.SurfacePrincipal,
		Text:        "1. ae1dd6b2 — 统一 VCS 证据通道。\n2. 3ae8465b — 将重试路由绑定到 claim 机制。",
	}}}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	if len(doc.Blocks) != 1 {
		t.Fatalf("must not append dry hash table when short hashes are visible: %+v", doc.Blocks)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_VCSSupplementUsesCommitColumn(t *testing.T) {
	mu := types.NewMutableState("最近 3 次提交都做了哪些事情")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "最近提交",
		Value: "3",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"ae1dd6b256fab219104c09447b6ffe3697239b7a: evidence: unify vcs provenance lanes",
			"3ae8465b6afe3fb16902d511d51482fefd09a103: orchestrator: route retries through claim bindings",
			"125687ab6f1ff7cd1187183fc459efe65be10fb3: orchestrator: suppress generic caveats after semantic pass",
		},
		Dimensions: []types.AnswerAggregateDimension{{Name: "evidence_origin", Value: "vcs_metadata"}},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:          "summary",
		Kind:        types.BlockSummary,
		SurfaceRole: types.SurfacePrincipal,
		Text:        "已说明 ae1dd6b2 和 3ae8465b 两个提交，还缺第三个提交。",
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected missing VCS row supplement")
	}
	var supplement *types.AnswerBlock
	for i := range doc.Blocks {
		if strings.Contains(doc.Blocks[i].Title, "系统按已验证证据补充缺失成员：最近提交") {
			supplement = &doc.Blocks[i]
			break
		}
	}
	if supplement == nil {
		t.Fatalf("expected VCS supplement, got %+v", doc.Blocks)
	}
	if got, want := supplement.Columns, []string{"提交"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("VCS supplement should use origin-aware column label, got %#v", got)
	}
	visible := types.AnswerBlockVisibleSurface(*supplement)
	for _, want := range []string{"125687ab6f1ff7cd1187183fc459efe65be10fb3", "generic caveats"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("supplement missing VCS row detail %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "符号名称") {
		t.Fatalf("VCS supplement must not render generic code-symbol heading:\n%s", visible)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_VCSChangedPathSupplementUsesFileColumn(t *testing.T) {
	mu := types.NewMutableState("对比最近两次提交的代码 diff，并结合当前源码分析影响链路")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "commit c9d1fe22 涉及文件",
		Value: "3",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"docs/design/local_model_eval_compat_20260523.md",
			"internal/agent/explorer.go",
			"internal/agent/explorer_test.go",
		},
		Dimensions: []types.AnswerAggregateDimension{{Name: "evidence_origin", Value: "vcs_metadata"}},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:          "summary",
		Kind:        types.BlockSummary,
		SurfaceRole: types.SurfacePrincipal,
		Text:        "当前回答已经说明了 internal/agent/explorer.go 的 FilterToolSchemas 机制，还需要补充其它变更文件。",
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected missing changed-path supplement")
	}
	var supplement *types.AnswerBlock
	for i := range doc.Blocks {
		if strings.Contains(doc.Blocks[i].Title, "系统按已验证证据补充缺失成员：commit c9d1fe22 涉及文件") {
			supplement = &doc.Blocks[i]
			break
		}
	}
	if supplement == nil {
		t.Fatalf("expected changed-path supplement, got %+v", doc.Blocks)
	}
	if got, want := supplement.Columns, []string{"文件"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("changed-path VCS supplement should use file column label, got %#v", got)
	}
	visible := types.AnswerBlockVisibleSurface(*supplement)
	if strings.Contains(visible, "| 提交 |") || strings.Contains(visible, "| 变更 |") {
		t.Fatalf("changed-path supplement must not render commit/change column:\n%s", visible)
	}
	for _, want := range []string{"docs/design/local_model_eval_compat_20260523.md", "internal/agent/explorer_test.go"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("supplement missing changed path %q:\n%s", want, visible)
		}
	}
}

func TestPrincipalEnumerationPrimaryColumnLabel_OriginSpecific(t *testing.T) {
	cases := []struct {
		name   string
		origin types.AnswerEvidenceOrigin
		zh     string
		en     string
	}{
		{"current", types.AnswerEvidenceOriginCurrentSource, "符号名称", "Name"},
		{"vcs", types.AnswerEvidenceOriginVCSMetadata, "提交", "Commit"},
		{"diff", types.AnswerEvidenceOriginVCSDiff, "变更", "Change"},
		{"runtime", types.AnswerEvidenceOriginRuntimeArtifact, "观察项", "Observation"},
		{"command", types.AnswerEvidenceOriginCommandMeasurement, "命令结果", "Command result"},
		{"repo_negative", types.AnswerEvidenceOriginRepoNegativeSearch, "负向查询", "Negative check"},
		{"cross_repo", types.AnswerEvidenceOriginCrossRepoIndex, "仓库项", "Repository item"},
		{"external_doc", types.AnswerEvidenceOriginExternalDocument, "资源项", "Resource"},
		{"web", types.AnswerEvidenceOriginWebPage, "资源项", "Resource"},
		{"mcp", types.AnswerEvidenceOriginMCPResource, "资源项", "Resource"},
		{"connector", types.AnswerEvidenceOriginConnectorResource, "资源项", "Resource"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := []types.EnumerationDisplayRow{{EvidenceOrigins: []types.AnswerEvidenceOrigin{tc.origin}}}
			if got := principalEnumerationPrimaryColumnLabel(true, rows); got != tc.zh {
				t.Fatalf("zh label = %q, want %q", got, tc.zh)
			}
			if got := principalEnumerationPrimaryColumnLabel(false, rows); got != tc.en {
				t.Fatalf("en label = %q, want %q", got, tc.en)
			}
		})
	}
	mixed := []types.EnumerationDisplayRow{
		{EvidenceOrigins: []types.AnswerEvidenceOrigin{types.AnswerEvidenceOriginCurrentSource}},
		{EvidenceOrigins: []types.AnswerEvidenceOrigin{types.AnswerEvidenceOriginVCSMetadata}},
	}
	if got := principalEnumerationPrimaryColumnLabel(true, mixed); got != "项目" {
		t.Fatalf("mixed-origin zh label = %q, want 项目", got)
	}
	if got := principalEnumerationPrimaryColumnLabel(false, mixed); got != "Item" {
		t.Fatalf("mixed-origin en label = %q, want Item", got)
	}
	pathRows := []types.EnumerationDisplayRow{
		{Member: "docs/design/local_model_eval_compat_20260523.md", EvidenceOrigins: []types.AnswerEvidenceOrigin{types.AnswerEvidenceOriginVCSMetadata}},
		{Member: "internal/agent/explorer.go", EvidenceOrigins: []types.AnswerEvidenceOrigin{types.AnswerEvidenceOriginVCSMetadata}},
	}
	if got := principalEnumerationPrimaryColumnLabel(true, pathRows); got != "文件" {
		t.Fatalf("path-primary zh label = %q, want 文件", got)
	}
	if got := principalEnumerationPrimaryColumnLabel(false, pathRows); got != "File" {
		t.Fatalf("path-primary en label = %q, want File", got)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_ExternalResourceSupplementIsMarkedAppendOnly(t *testing.T) {
	mu := types.NewMutableState("列出 MCP 返回的打开事故")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "打开事故",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"INC-17", "INC-42"},
		MemberNotes: []string{
			"INC-17 由 runtime 组件负责，来自 MCP incidents 资源。",
			"INC-42 由 storage 组件负责，来自 MCP incidents 资源。",
		},
		Dimensions: []types.AnswerAggregateDimension{{
			Name:  "origin",
			Value: string(types.AnswerEvidenceOriginMCPResource),
		}},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Language: "zh",
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "当前 MCP incidents 资源里有打开事故，但正文尚未展开每一项。",
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected external resource supplement")
	}
	if len(doc.Blocks) != 2 || doc.Blocks[0].Text != "当前 MCP incidents 资源里有打开事故，但正文尚未展开每一项。" {
		t.Fatalf("system supplement must append only and preserve authored summary: %+v", doc.Blocks)
	}
	supplement := doc.Blocks[1]
	if !strings.Contains(supplement.Title, "系统按已验证证据补充缺失成员：打开事故") {
		t.Fatalf("external resource supplement should be clearly marked, got %q", supplement.Title)
	}
	if got, want := supplement.Columns, []string{"资源项", "说明"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("external resource supplement should use resource/note columns, got %#v", got)
	}
	visible := types.AnswerBlockVisibleSurface(supplement)
	for _, want := range []string{"INC-17", "INC-42", "runtime 组件负责", "storage 组件负责"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("external supplement lost rich note %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "符号名称") || len(doc.Citations) != 0 {
		t.Fatalf("external resource supplement must not look like a source-symbol table or invent citations: visible=%s citations=%+v", visible, doc.Citations)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_VisibleArchitectureCoveragePreventsSystemSupplements(t *testing.T) {
	mu := types.NewMutableState("codrax 的 read-mode pipeline 由哪几个 stage 组成？")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:  types.AnswerAggregateMemberSet,
			Label: "read-mode pipeline 全部阶段",
			Value: "6",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{
				"StageLogTriage (条件预阶段, AttachedLog 非空触发)",
				"StagePerfTriage (条件预阶段, AttachedHitrace 非空触发)",
				"StageAnalyze (主阶段, analyzer agent)",
				"StageExplore (主阶段, explorer agent, Turn A)",
				"StageExtract (主阶段, extractor agent, Turn B)",
				"StageFinalize (主阶段, finalizer agent, Terminal=true)",
			},
		},
		{
			Kind:       types.AnswerAggregateMemberSet,
			Label:      "主阶段四链路",
			Value:      "4",
			Role:       types.AnswerAggregateRolePrincipalAnswer,
			Provenance: "AllMainStages()",
			Members:    []string{"StageAnalyze", "StageExplore", "StageExtract", "StageFinalize"},
		},
		{
			Kind:  types.AnswerAggregateBucketCount,
			Label: "阶段类型分布",
			Value: "2",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{
				"条件预阶段 (advisory): 2 个 (log_triage, perf_triage)",
				"无条件主阶段: 4 个 (analyze, explore, extract, finalize)",
			},
		},
	})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioArchitectureExplain,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "read-mode pipeline 由条件预阶段和无条件主阶段组成：AttachedLog / AttachedHitrace 非空时触发条件预阶段；四个主阶段按 analyze → explore → extract → finalize 执行。",
		},
		{
			ID:    "main",
			Kind:  types.BlockSection,
			Title: "无条件主阶段",
			Text:  "四个无条件主阶段是 analyze、explore、extract、finalize，分别对应 analyzer agent、explorer agent、extractor agent、finalizer agent。",
		},
		{
			ID:    "stages",
			Kind:  types.BlockOrderedList,
			Title: "read-mode pipeline 全部阶段",
			Items: []types.AnswerBlockItem{
				{ID: "s1", Label: "StageLogTriage (条件预阶段)", Text: "AttachedLog 非空时触发，由 log_triager agent 执行日志整理。"},
				{ID: "s2", Label: "StagePerfTriage (条件预阶段)", Text: "AttachedHitrace 非空时触发，由 perf_triager agent 执行性能 trace 整理。"},
				{ID: "s3", Label: "StageAnalyze (主阶段)", Text: "analyzer agent 执行初步分析。"},
				{ID: "s4", Label: "StageExplore (主阶段, Turn A)", Text: "explorer agent 执行源码探索。"},
				{ID: "s5", Label: "StageExtract (主阶段, Turn B)", Text: "extractor agent 消费探索产物并抽取结构化信息。"},
				{ID: "s6", Label: "StageFinalize (主阶段, Terminal=true)", Text: "finalizer agent 生成最终答案。"},
			},
		},
	}}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)

	for _, block := range doc.Blocks {
		if strings.Contains(block.Title, "系统按已验证证据补充成员") {
			t.Fatalf("visible architecture prose/list already covers aggregate rows; duplicate supplement should not render: %+v", doc.Blocks)
		}
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_SkipsRuntimeArtifactCoordinateOnlySupplement(t *testing.T) {
	mu := types.NewMutableState("哪些 goroutine 同时出错？它们的共同问题是什么？")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "运行时帧占位成员",
		Value:   "1",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"<native>@runtime:0"},
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "evidence_origin", Value: "runtime_artifact"},
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsDiagnosticQuestion: true,
			},
			LogTriage: &types.LogBundle{Meta: types.LogMeta{Summary: "goroutine dump"}},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "goroutine 15、87、120 都表现为 concurrent map writes。",
	}}}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)

	var visible []string
	for _, block := range doc.Blocks {
		visible = append(visible, block.Title, types.AnswerBlockVisibleSurface(block))
	}
	joined := strings.Join(visible, "\n")
	if strings.Contains(joined, "<native>@runtime:0") {
		t.Fatalf("runtime artifact coordinate-only placeholder must not render as a system supplement:\n%s", joined)
	}
	if strings.Contains(joined, "系统按已验证证据补充成员：运行时帧占位成员") {
		t.Fatalf("coordinate-only runtime artifact member supplement should be skipped:\n%s", joined)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_RuntimeArtifactProseCoveragePreventsSupplement(t *testing.T) {
	mu := types.NewMutableState("哪些 goroutine 同时出错？它们的共同问题是什么？")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "触发错误的函数",
		Value:   "1",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"main.writeSession"},
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "evidence_origin", Value: "runtime_artifact"},
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsDiagnosticQuestion: true,
			},
			LogTriage: &types.LogBundle{Meta: types.LogMeta{Summary: "goroutine dump"}},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "三个 goroutine 的共同入口均为 main.writeSession，根因是 concurrent map writes。",
	}}}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)

	var visible []string
	for _, block := range doc.Blocks {
		visible = append(visible, block.Title, types.AnswerBlockVisibleSurface(block))
	}
	joined := strings.Join(visible, "\n")
	if strings.Contains(joined, "系统按已验证证据补充成员：触发错误的函数") {
		t.Fatalf("runtime artifact prose already covers member; duplicate supplement should not render:\n%s", joined)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_ObservationOnlyRuntimeDoesNotAppendSystemMemberTable(t *testing.T) {
	logBundle := &types.LogBundle{
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{
				{Func: "Cart.itemAt", Raw: "at demo.cart.Cart.itemAt(src/cart/Cart.cj:78)"},
				{Func: "Cart.checkout", Raw: "at demo.cart.Cart.checkout(src/cart/Cart.cj:42)"},
				{Func: "entry", Raw: "at demo.app.entry(src/main.cj:21)"},
			},
		}},
	}
	mu := types.NewMutableState("这个仓颉应用 panic 日志显示什么问题？追溯根因。")
	mu.SetLogTriage(logBundle)
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "调用链",
		Value: "3",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"demo.app.entry",
			"demo.cart.Cart.checkout",
			"demo.cart.Cart.itemAt",
		},
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "evidence_origin", Value: "runtime_artifact"},
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:    types.IntentRootCause,
			Scenario:  types.ScenarioRootCause,
			Language:  "zh",
			LogTriage: logBundle,
			Predicates: types.SemanticPredicates{
				IsDiagnosticQuestion: true,
			},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
				SourceQuotes:      []string{"只分析日志"},
				Confidence:        0.9,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetObservedArtifactFact)},
			ClaimUses: []types.RenderedClaimUse{{
				ClaimForm: types.ClaimExternalObservation,
				FacetID:   string(types.FacetObservedArtifactFact),
			}},
			Text: "调用链为 demo.app.entry → Cart.checkout → Cart.itemAt，最终在 Cart.itemAt 触发 index out of bounds: index=5, size=3。",
		},
		{
			ID:          "chain",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetObservedArtifactFact)},
			ClaimUses: []types.RenderedClaimUse{{
				ClaimForm: types.ClaimExternalObservation,
				FacetID:   string(types.FacetObservedArtifactFact),
			}},
			Items: []types.AnswerBlockItem{
				{ID: "entry", Label: "demo.app.entry", Text: "入口函数。", CitationRef: -1},
				{ID: "checkout", Label: "Cart.checkout", Text: "结算流程调用 itemAt。", CitationRef: -1},
				{ID: "itemAt", Label: "Cart.itemAt", Text: "越界点。", CitationRef: -1},
			},
		},
	}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed != 0 {
		t.Fatalf("observation-only runtime artifacts must not get system member-table normalization, fixed=%d doc=%+v", fixed, doc.Blocks)
	}
	if fixed := compileEnumerationDisplayTableRows(doc, ctx); fixed != 0 {
		t.Fatalf("observation-only runtime artifacts must not get deterministic enum table compilation, fixed=%d doc=%+v", fixed, doc.Blocks)
	}
	if fixed := normalizeAggregateMemberSetCarriers(doc, ctx); fixed != 0 {
		t.Fatalf("observation-only runtime artifacts must not materialize aggregate member carriers, fixed=%d doc=%+v", fixed, doc.Blocks)
	}
	if hints := preCheckAggregateMemberSetCoverage(doc, ctx); len(hints) != 0 {
		t.Fatalf("observation-only runtime aggregate members should be soft artifact context, not a visible-member hard/advisory gate: %+v", hints)
	}
	if hints := preCheckAggregateCardinalityConsistency(doc, ctx); len(hints) != 0 {
		t.Fatalf("observation-only runtime aggregate counts should not gate visible artifact wording: %+v", hints)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("normalizers should preserve the model-authored artifact answer shape, got %+v", doc.Blocks)
	}
	for _, block := range doc.Blocks {
		if strings.Contains(block.Title, "系统按已验证证据补充成员") ||
			testStringSliceContains(block.FacetIDs, string(types.FacetEnumerationItem)) {
			t.Fatalf("observation-only runtime answer polluted by enumeration supplement/facet: %+v", block)
		}
	}
}

func testStringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
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
	if len(doc.Blocks) != 1 {
		t.Fatalf("authored English table should not receive a synthetic system summary or supplement, got %+v", doc.Blocks)
	}
	if visible := answerDocumentTestVisibleSurface(doc); strings.Contains(visible, "System-verified") {
		t.Fatalf("authored English table must suppress system supplement:\n%s", visible)
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
	if !strings.Contains(supplement.Title, "系统按已验证证据补充缺失成员") {
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
	if strings.Contains(visible, "系统按已验证证据补充说明") {
		t.Fatalf("authored row descriptions should not receive a system note supplement:\n%s", visible)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_AppendsVerifiedNoteSupplementForDryRows(t *testing.T) {
	mu := types.NewMutableState("列出 Kind 常量")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("kind_symbol_present", "KindSymbolPresent", "internal/analysis/criterion/grammar.go", 29, "符号存在性判定，用于确认目标符号是否出现在证据或答案集合中。"),
		enumEvidence("kind_no_call_sites", "KindNoCallSites", "internal/analysis/criterion/grammar.go", 30, "调用点缺失判定，用于确认目标符号没有被调用。"),
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
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Label: "说明",
					Role:  types.RequestedAnswerDimensionFunctionOrPurpose,
					Index: 1,
				}},
			},
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	originalTable := strings.Join([]string{
		"| 符号名称 | 定义位置 |",
		"|---|---|",
		"| KindSymbolPresent | grammar.go:29 |",
		"| KindNoCallSites | grammar.go:30 |",
	}, "\n")
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:    "dry_kinds",
		Kind:  types.BlockTable,
		Title: "Kind 常量",
		Text:  originalTable,
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatal("expected append-only verified note supplement for dry rows")
	}
	authored := answerDocumentTestBlockByID(t, doc, "dry_kinds")
	if authored.Text != originalTable {
		t.Fatalf("model-authored markdown table must not be rewritten:\n%s", authored.Text)
	}
	if visible := answerDocumentTestVisibleSurface(doc); strings.Contains(visible, "系统按已验证证据补充说明：Kind 常量") {
		t.Fatalf("authored dry table must suppress note补表; notes remain available in evidence context:\n%s", visible)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_SkipsVerifiedNoteSupplementWhenSummaryNotRequested(t *testing.T) {
	mu := types.NewMutableState("列出 Kind 常量")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("kind_symbol_present", "KindSymbolPresent", "internal/analysis/criterion/grammar.go", 29, "符号存在性判定，用于确认目标符号是否出现在证据或答案集合中。"),
		enumEvidence("kind_no_call_sites", "KindNoCallSites", "internal/analysis/criterion/grammar.go", 30, "调用点缺失判定，用于确认目标符号没有被调用。"),
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
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:    "dry_kinds",
		Kind:  types.BlockTable,
		Title: "Kind 常量",
		Text: strings.Join([]string{
			"| 符号名称 | 定义位置 |",
			"|---|---|",
			"| KindSymbolPresent | grammar.go:29 |",
			"| KindNoCallSites | grammar.go:30 |",
		}, "\n"),
	}}}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	joined := answerDocumentTestVisibleSurface(doc)
	if strings.Contains(joined, "系统按已验证证据补充说明") {
		t.Fatalf("system note supplement must not appear when typed request only asks for names/locations:\n%s", joined)
	}
	for _, want := range []string{"KindSymbolPresent", "KindNoCallSites"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("model-authored dry row %q should be preserved:\n%s", want, joined)
		}
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_SkipsStaleSourceInventoryNoteSupplementForTypedRelation(t *testing.T) {
	mu := types.NewMutableState("列出 internal/analysis 下所有子包目录名和单一入口函数")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("aggregator_aggregate", "Aggregate", "internal/analysis/aggregator/aggregator.go", 132, "入口函数：接收 EvidenceClosure 并返回排序后的 FieldHeat。"),
		enumEvidence("amplifier_amplify", "Amplify", "internal/analysis/amplifier/amplifier.go", 100, "入口函数：按注册顺序执行 preCompileRules。"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "internal/analysis 子包入口函数",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"aggregator → Aggregate", "amplifier → Amplify"},
		SupportRefs: []string{
			"Aggregate @ internal/analysis/aggregator/aggregator.go:132",
			"Amplify @ internal/analysis/amplifier/amplifier.go:100",
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	mu.SetRequestModel(types.RequestModel{
		Intent:        types.IntentEnumerate,
		Language:      "zh",
		PredicateAxis: types.AxisReturn,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsRelationalLookup:    true,
		},
	})
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentEnumerate,
			Language:      "zh",
			PredicateAxis: types.AxisReturn,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
				IsRelationalLookup:    true,
			},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles: []types.AnswerCandidateRole{
					types.AnswerCandidateRoleFunction,
					types.AnswerCandidateRolePackage,
				},
				RequestedFields: []types.SourceInventoryRequestedField{
					types.SourceInventoryFieldName,
					types.SourceInventoryFieldLocation,
					types.SourceInventoryFieldSummary,
				},
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:    "entrypoints",
		Kind:  types.BlockTable,
		Title: "internal/analysis 子包入口函数",
		Text: strings.Join([]string{
			"| 子包 | 入口函数 | 文件位置 |",
			"|---|---|---|",
			"| aggregator | Aggregate | internal/analysis/aggregator/aggregator.go:132 |",
			"| amplifier | Amplify | internal/analysis/amplifier/amplifier.go:100 |",
		}, "\n"),
	}}}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	joined := answerDocumentTestVisibleSurface(doc)
	if strings.Contains(joined, "系统按已验证证据补充说明") {
		t.Fatalf("stale source_inventory_profile ignored for a typed relation must not re-enable system note supplements:\n%s", joined)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_CrossColumnMemberCoveragePreventsMissingSupplement(t *testing.T) {
	mu := types.NewMutableState("列出 internal/analysis 子包和入口函数")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("counterfactual_expand", "Expand", "internal/analysis/counterfactual/expander.go", 59, "Expand 是 counterfactual 子包入口函数。"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "子包目录名与单一入口函数",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"counterfactual → Expand"},
		SupportRefs: []string{"counterfactual → Expand @ internal/analysis/counterfactual/expander.go:59"},
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
		ID:    "entry_table",
		Kind:  types.BlockTable,
		Title: "子包入口",
		Text: strings.Join([]string{
			"| 子包目录 | 入口函数 | 文件位置 |",
			"|---|---|---|",
			"| counterfactual | Expand | internal/analysis/counterfactual/expander.go:61 |",
		}, "\n"),
	}}}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	joined := answerDocumentTestVisibleSurface(doc)
	if strings.Contains(joined, "系统按已验证证据补充缺失成员") {
		t.Fatalf("member identity split across table columns should count as visible coverage, not trigger a system table:\n%s", joined)
	}
	if !strings.Contains(joined, "counterfactual") || !strings.Contains(joined, "Expand") {
		t.Fatalf("model-authored cross-column row should remain visible:\n%s", joined)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_CrossColumnMemberRequiresSameFile(t *testing.T) {
	mu := types.NewMutableState("列出 internal/analysis 子包和入口函数")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("counterfactual_expand", "Expand", "internal/analysis/counterfactual/expander.go", 59, "Expand 是 counterfactual 子包入口函数。"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "子包目录名与单一入口函数",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"counterfactual → Expand"},
		SupportRefs: []string{"counterfactual → Expand @ internal/analysis/counterfactual/expander.go:59"},
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
		ID:    "entry_table",
		Kind:  types.BlockTable,
		Title: "子包入口",
		Text: strings.Join([]string{
			"| 子包目录 | 入口函数 | 文件位置 |",
			"|---|---|---|",
			"| counterfactual | Expand | internal/analysis/other/expander.go:59 |",
		}, "\n"),
	}}}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	joined := answerDocumentTestVisibleSurface(doc)
	if strings.Contains(joined, "系统按已验证证据补充缺失成员") {
		t.Fatalf("different-file mismatch must not trigger a competing system补表 once model supplied a carrier:\n%s", joined)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_StructuredItemRelationFieldsPreventMissingSupplement(t *testing.T) {
	mu := types.NewMutableState("列出 internal/analysis 子包和入口函数")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("aggregator_aggregate", "Aggregate", "internal/analysis/aggregator/aggregator.go", 129, "Aggregate walks the ledger on closure and returns FieldHeat entries sorted by WeightedScore descending."),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "internal/analysis 子包入口函数全集",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"aggregator.Aggregate"},
		SupportRefs: []string{"aggregator.Aggregate @ internal/analysis/aggregator/aggregator.go:129"},
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
		ID:          "entry_list",
		Kind:        types.BlockOrderedList,
		Title:       "internal/analysis 子包入口函数",
		SurfaceRole: types.SurfacePrincipal,
		Items: []types.AnswerBlockItem{{
			ID:    "aggregator",
			Label: "aggregator",
			Text:  "主入口函数 Aggregate，对 EvidenceClosure 执行聚合并返回按 WeightedScore 降序排列的 FieldHeat 列表。 (`internal/analysis/aggregator/aggregator.go:132`)",
		}},
	}}}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	joined := answerDocumentTestVisibleSurface(doc)
	if strings.Contains(joined, "系统按已验证证据补充缺失成员") {
		t.Fatalf("relation identity split across structured label/text should count as visible coverage:\n%s", joined)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_DoesNotDuplicateVisibleVerifiedNotes(t *testing.T) {
	mu := types.NewMutableState("列出 Kind 常量")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("kind_symbol_present", "KindSymbolPresent", "internal/analysis/criterion/grammar.go", 29, "符号存在性判定，用于确认目标符号是否出现在证据或答案集合中。"),
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
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "KindSymbolPresent 是符号存在性判定，用于确认目标符号是否出现在证据或答案集合中。",
		},
		{
			ID:    "dry_kinds",
			Kind:  types.BlockTable,
			Title: "Kind 常量",
			Text: strings.Join([]string{
				"| 符号名称 | 定义位置 |",
				"|---|---|",
				"| KindSymbolPresent | grammar.go:29 |",
			}, "\n"),
		},
	}}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	joined := answerDocumentTestVisibleSurface(doc)
	if strings.Contains(joined, "系统按已验证证据补充说明") {
		t.Fatalf("already visible verified note should not be duplicated:\n%s", joined)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_ProseDescriptionsPreventVerifiedNoteSupplement(t *testing.T) {
	mu := types.NewMutableState("对比两个子仓的核心导出标识符和功能用途")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("go_user_service", "UserService", "repo-greet-go/service/service.go", 8, "UserService declares the greeter interface contract."),
		enumEvidence("go_impl", "GreetServiceImpl", "repo-greet-go/service/impl.go", 6, "GreetServiceImpl implements the greeter service."),
		enumEvidence("py_process", "process_request", "repo-tools-py/tools/processor.py", 4, "process_request normalizes request text."),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   "repo-greet-go 核心导出标识符",
			Value:   "2",
			Role:    types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{"UserService", "GreetServiceImpl"},
			SupportRefs: []string{
				"UserService @ repo-greet-go/service/service.go:8",
				"GreetServiceImpl @ repo-greet-go/service/impl.go:6",
			},
		},
		{
			Kind:        types.AnswerAggregateMemberSet,
			Label:       "repo-tools-py 核心导出标识符",
			Value:       "1",
			Role:        types.AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"process_request"},
			SupportRefs: []string{"process_request @ repo-tools-py/tools/processor.py:4"},
		},
	})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Language: "zh",
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Label: "功能用途",
					Role:  types.RequestedAnswerDimensionFunctionOrPurpose,
					Index: 1,
				}},
			},
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "本回答按子仓分别说明核心导出标识符和功能用途。",
		},
		{
			ID:    "go_bucket",
			Kind:  types.BlockSection,
			Title: "repo-greet-go",
			Text: strings.Join([]string{
				"UserService（repo-greet-go/service/service.go:8）是 greeter 的契约接口，定义服务调用方依赖的方法集合。",
				"GreetServiceImpl（repo-greet-go/service/impl.go:6）是问候服务实现，负责提供具体问候和告别行为。",
			}, "\n"),
		},
		{
			ID:    "py_bucket",
			Kind:  types.BlockSection,
			Title: "repo-tools-py",
			Text:  "process_request（repo-tools-py/tools/processor.py:4）是请求文本规范化入口，负责清理空白并统一大小写。",
		},
	}}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	joined := answerDocumentTestVisibleSurface(doc)
	if strings.Contains(joined, "系统按已验证证据补充说明") {
		t.Fatalf("model-authored prose descriptions should not receive duplicate note supplements:\n%s", joined)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_NoteSupplementSupportsExternalOrigins(t *testing.T) {
	mu := types.NewMutableState("最近一次合入是什么特性")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "最近提交",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"bfc2054", "677035e"},
		MemberNotes: []string{
			"bfc2054 引入 typed relation selector，影响 caller/import relation 的提示与覆盖检查。",
			"677035e 接入 called-by relation provider，稳定调用者枚举答案。",
		},
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: string(types.AnswerEvidenceOriginVCSMetadata)},
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Language: "zh",
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Label: "特性",
					Role:  types.RequestedAnswerDimensionFunctionOrPurpose,
					Index: 1,
				}},
			},
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:    "commits",
		Kind:  types.BlockTable,
		Title: "最近提交",
		Text: strings.Join([]string{
			"| 提交 |",
			"|---|",
			"| bfc2054 |",
			"| 677035e |",
		}, "\n"),
	}}}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	joined := answerDocumentTestVisibleSurface(doc)
	if strings.Contains(joined, "系统按已验证证据补充说明：最近提交") ||
		strings.Contains(joined, "typed relation selector") {
		t.Fatalf("authored external-origin table must suppress note补表:\n%s", joined)
	}
	if strings.Contains(joined, "internal/") {
		t.Fatalf("VCS-origin note supplement must not invent current-source citations:\n%s", joined)
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
	visible := types.AnswerBlockVisibleSurface(answerDocumentTestBlockByID(t, doc, "all_symbols"))
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

func TestNormalizePrincipalEnumerationRowBlocks_RuntimeArtifactGoroutineShorthandPreventsSupplement(t *testing.T) {
	mu := types.NewMutableState("哪些 goroutine 同时出错？")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:       types.AnswerAggregateMemberSet,
		Label:      "同时出错的 goroutine",
		Value:      "3",
		Role:       types.AnswerAggregateRolePrincipalAnswer,
		Provenance: string(types.AnswerEvidenceOriginRuntimeArtifact),
		Dimensions: []types.AnswerAggregateDimension{{
			Name:  "origin",
			Value: string(types.AnswerEvidenceOriginRuntimeArtifact),
		}},
		Members: []string{"goroutine 15", "goroutine 87", "goroutine 120"},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Language: "zh",
			Intent:   types.IntentRootCause,
			Predicates: types.SemanticPredicates{
				IsDiagnosticQuestion: true,
			},
			DiagnosticProfile: types.DiagnosticIntentProfile{IsDiagnostic: true},
			LogTriage:         &types.LogBundle{Errors: []types.LogError{{Type: "fatal error"}}},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "运行时日志显示三个 goroutine（15、87、120）同时触发 fatal error: concurrent map writes。",
	}}}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	for _, block := range doc.Blocks {
		if strings.Contains(block.Title, "系统按已验证证据补充成员") {
			t.Fatalf("compact goroutine shorthand should count as visible coverage without a supplement block: %+v", doc.Blocks)
		}
	}
}

func TestNormalizeAggregateMemberSetCarriers_VisibleProseCoveragePreventsDuplicateCarrier(t *testing.T) {
	mu := types.NewMutableState("列出完整公开函数")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "公开函数",
		Value:   "3",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Eval", "EvalAll", "RegisteredKinds"},
		SupportRefs: []string{
			"Eval @ internal/analysis/criterion/eval.go:15",
			"EvalAll @ internal/analysis/criterion/eval.go:36",
			"RegisteredKinds @ internal/analysis/criterion/grammar.go:118",
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
			CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "完整公开函数"},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "answer",
		Kind: types.BlockSummary,
		Text: "公开函数完整包含 Eval @ internal/analysis/criterion/eval.go:15、EvalAll @ internal/analysis/criterion/eval.go:36 和 RegisteredKinds @ internal/analysis/criterion/grammar.go:118；模型已在主回答中解释三者职责。",
	}}}

	if fixed := normalizeAggregateMemberSetCarriers(doc, ctx); fixed != 0 {
		t.Fatalf("model-authored visible prose already covers every member; system carrier must not duplicate it, fixed=%d doc=%+v", fixed, doc.Blocks)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("system duplicate carrier appended despite visible coverage: %+v", doc.Blocks)
	}
}

func TestNormalizeAggregateMemberSetCarriers_CrossColumnModelTablePreventsDuplicateCarrier(t *testing.T) {
	mu := types.NewMutableState("列出 internal/analysis 子包入口")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "internal/analysis 子包入口函数",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"logtriage: ValidateBundle", "normalizer: Normalize"},
		SupportRefs: []string{
			"ValidateBundle @ internal/analysis/logtriage/validate.go:149",
			"Normalize @ internal/analysis/normalizer/normalizer.go:72",
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
			CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "所有子包"},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:          "model_table",
		Kind:        types.BlockTable,
		Title:       "子包入口函数",
		SurfaceRole: types.SurfacePrincipal,
		FacetIDs:    []string{string(types.FacetEnumerationItem)},
		Text: strings.Join([]string{
			"| 子包目录 | 入口函数 | 文件位置 |",
			"|---|---|---|",
			"| logtriage | ValidateBundle | internal/analysis/logtriage/validate.go:149 |",
			"| normalizer | Normalize | internal/analysis/normalizer/normalizer.go:72 |",
		}, "\n"),
	}}}

	if fixed := normalizeAggregateMemberSetCarriers(doc, ctx); fixed != 0 {
		t.Fatalf("model table already covers aggregate rows across columns; system carrier must not duplicate it, fixed=%d doc=%+v", fixed, doc.Blocks)
	}
	joined := answerDocumentTestVisibleSurface(doc)
	if strings.Contains(joined, "系统按已验证证据补充成员") {
		t.Fatalf("duplicate aggregate carrier leaked after cross-column coverage:\n%s", joined)
	}
}

func TestNormalizeAggregateMemberSetCarriers_SystemRowSupplementPreventsDuplicateCarrier(t *testing.T) {
	mu := types.NewMutableState("列出完整公开函数")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "公开函数",
		Value:   "3",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Eval", "EvalAll", "RegisteredKinds"},
		SupportRefs: []string{
			"Eval @ internal/analysis/criterion/eval.go:15",
			"EvalAll @ internal/analysis/criterion/eval.go:36",
			"RegisteredKinds @ internal/analysis/criterion/grammar.go:118",
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
			CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "完整公开函数"},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "这里先解释整体语义，模型没有逐条列出成员。",
	}}}

	if fixed := normalizePrincipalEnumerationRowBlocks(doc, ctx); fixed == 0 {
		t.Fatalf("expected row compiler to append the missing-member supplement")
	}
	before := len(doc.Blocks)
	if fixed := normalizeAggregateMemberSetCarriers(doc, ctx); fixed != 0 {
		t.Fatalf("aggregate carrier must not duplicate rows already covered by the system row supplement, fixed=%d doc=%+v", fixed, doc.Blocks)
	}
	if len(doc.Blocks) != before {
		t.Fatalf("aggregate carrier appended a duplicate block; before=%d after=%d doc=%+v", before, len(doc.Blocks), doc.Blocks)
	}
	var missingSupplements, legacyCarriers int
	for _, block := range doc.Blocks {
		if strings.Contains(block.Title, "系统按已验证证据补充缺失成员") {
			missingSupplements++
		}
		if strings.Contains(block.Title, "系统按已验证证据补充成员") {
			legacyCarriers++
		}
	}
	if missingSupplements != 1 || legacyCarriers != 0 {
		t.Fatalf("expected exactly one missing-member supplement and no legacy carrier; missing=%d carriers=%d doc=%+v", missingSupplements, legacyCarriers, doc.Blocks)
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

func answerDocumentTestVisibleSurface(doc *types.AnswerDocumentV2) string {
	if doc == nil {
		return ""
	}
	parts := make([]string, 0, len(doc.Blocks))
	for _, block := range doc.Blocks {
		parts = append(parts, types.AnswerBlockVisibleSurface(block))
	}
	return strings.Join(parts, "\n")
}
