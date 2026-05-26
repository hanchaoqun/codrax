package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAnswerDocumentRedlineMatrix_FullPreEmitPreservesModelAuthoredTable(t *testing.T) {
	mu := types.NewMutableState("列出 Kind 常量并说明用途")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("kind_symbol_present", "KindSymbolPresent", "internal/analysis/criterion/grammar.go", 29, "KindSymbolPresent常量定义"),
		enumEvidence("kind_no_call_sites", "KindNoCallSites", "internal/analysis/criterion/grammar.go", 30, "KindNoCallSites常量定义"),
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
					Label: "中文说明",
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
		"| Kind常量 | 定义位置 | 中文说明 |",
		"|---|---|---|",
		"| KindSymbolPresent | internal/analysis/criterion/grammar.go:29 | 符号存在性判定 |",
		"| KindNoCallSites | internal/analysis/criterion/grammar.go:30 | 调用点缺失判定 |",
	}, "\n")
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:          "model_table",
		Kind:        types.BlockTable,
		Title:       "模型给出的 Kind 常量表",
		SurfaceRole: types.SurfacePrincipal,
		FacetIDs:    []string{string(types.FacetEnumerationItem)},
		Text:        originalTable,
	}}}
	view := &types.AnswerSemanticView{}
	normalizeAnswerDocumentForPreEmit("redline_matrix", doc, view, ctx, newPreEmitCheckContext(ctx))

	if len(doc.Blocks) != 1 {
		t.Fatalf("full pre-emit normalization must not append a competing system block when the model authored the carrier: %+v", doc.Blocks)
	}
	if doc.Blocks[0].Text != originalTable {
		t.Fatalf("model-authored markdown table must stay byte-identical:\n%s", doc.Blocks[0].Text)
	}
	visible := answerDocumentTestVisibleSurface(doc)
	for _, want := range []string{"符号存在性判定", "调用点缺失判定"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("model-authored rich description lost %q:\n%s", want, visible)
		}
	}
	for _, banned := range []string{"系统按已验证证据补充", "KindSymbolPresent常量定义", "KindNoCallSites常量定义"} {
		if strings.Contains(visible, banned) {
			t.Fatalf("system structural preference or weak evidence summary leaked into model-authored answer surface (%q):\n%s", banned, visible)
		}
	}
	if hints := runPreEmitChecks(doc, view, nil, ctx); len(hints) != 0 {
		t.Fatalf("model-authored covered table should pass pre-emit without forcing a rewrite: %+v", hints)
	}
}

func TestAnswerDocumentRedlineMatrix_FullPreEmitPreservesModelStructuredTable(t *testing.T) {
	mu := types.NewMutableState("列出系统 Agent 及职责")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("analyzer", "analyzer", "internal/agent/analyzer.go", 34, "读模式核心分析器。"),
		enumEvidence("explorer", "explorer", "internal/agent/explorer.go", 68, "读模式代码探索器。"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "Agent 清单",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"analyzer", "explorer"},
		SupportRefs: []string{
			"analyzer @ internal/agent/analyzer.go:34",
			"explorer @ internal/agent/explorer.go:68",
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
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "系统 Agent 按读写模式分工。",
		}, {
			ID:          "model_agent_table",
			Kind:        types.BlockTable,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Columns:     []string{"Agent 标识", "作用/职责 (function_or_purpose)", "关系/协作 (relationship)"},
			Items: []types.AnswerBlockItem{{
				ID:          "analyzer",
				Label:       "analyzer",
				Text:        "读模式核心分析器。驱动 StageAnalyze 阶段，通过 emit_analysis 工具生成 AnalysisIR。",
				CitationRef: 0,
			}, {
				ID:          "explorer",
				Label:       "explorer",
				Text:        "读模式代码探索器。驱动 StageExplore 阶段，维护搜索缓存与多图句柄。",
				CitationRef: 1,
			}},
		}},
		Citations: []types.Citation{
			{File: "internal/agent/analyzer.go", Line: 34},
			{File: "internal/agent/explorer.go", Line: 68},
		},
	}
	view := &types.AnswerSemanticView{}
	normalizeAnswerDocumentForPreEmit("redline_matrix", doc, view, ctx, newPreEmitCheckContext(ctx))

	table := answerDocumentTestBlockByID(t, doc, "model_agent_table")
	wantColumns := []string{"Agent 标识", "作用/职责 (function_or_purpose)", "关系/协作 (relationship)"}
	if len(table.Columns) != len(wantColumns) {
		t.Fatalf("structured table columns must stay model-authored: %+v", table.Columns)
	}
	for i := range wantColumns {
		if table.Columns[i] != wantColumns[i] {
			t.Fatalf("structured table columns must stay model-authored: %+v", table.Columns)
		}
	}
	if len(table.Items) != 2 {
		t.Fatalf("model-authored table rows must not be replaced: %+v", table.Items)
	}
	for _, item := range table.Items {
		if len(item.Cells) != 0 {
			t.Fatalf("system must not fill model-authored structured table cells: %+v", item)
		}
	}
	if table.Items[0].Text != "读模式核心分析器。驱动 StageAnalyze 阶段，通过 emit_analysis 工具生成 AnalysisIR。" ||
		table.Items[1].Text != "读模式代码探索器。驱动 StageExplore 阶段，维护搜索缓存与多图句柄。" {
		t.Fatalf("model-authored rich row text was changed: %+v", table.Items)
	}
	visible := answerDocumentTestVisibleSurface(doc)
	for _, banned := range []string{"系统按已验证证据补充", "[illustrative]", "定义位置 | 说明"} {
		if strings.Contains(visible, banned) {
			t.Fatalf("system supplement or rewritten table shape leaked into model-authored table (%q):\n%s", banned, visible)
		}
	}
}

func TestAnswerDocumentRedlineMatrix_FullPreEmitKeepsOriginSpecificAbsenceExternal(t *testing.T) {
	mu := types.NewMutableState("最近提交里是否还有 Backport Foo")
	mu.SetInvestigationResultKind("absence")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateNegativeObservation,
		Label: "Backport Foo absent in recent git history",
		Value: "0",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: string(types.AnswerEvidenceOriginVCSMetadata)},
			{Name: "target", Value: "Backport Foo"},
			{Name: "scope", Value: "HEAD~50..HEAD"},
			{Name: "result_count", Value: "0"},
			{Name: "tool_result", Value: "git log --grep Backport Foo"},
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Language: "zh",
		}},
	}
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
		Blocks: []types.AnswerBlock{{
			ID:          "answer",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "Backport Foo 在 HEAD~50..HEAD 范围内没有相关提交；该结论来自 git 历史检索，result_count=0。",
		}},
	}
	view := &types.AnswerSemanticView{}
	normalizeAnswerDocumentForPreEmit("redline_matrix", doc, view, ctx, newPreEmitCheckContext(ctx))

	visible := answerDocumentTestVisibleSurface(doc)
	for _, banned := range []string{"系统按已验证证据补充关键源码锚点", "系统按已验证证据补充未命中范围", "file:line"} {
		if strings.Contains(visible, banned) {
			t.Fatalf("origin-specific absence must not be re-anchored as current-source or duplicated by supplement (%q):\n%s", banned, visible)
		}
	}
	if len(doc.Citations) != 0 {
		t.Fatalf("VCS negative observation must not invent current-source citations: %+v", doc.Citations)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("structured absent answer already covers target/scope; no duplicate supplement expected: %+v", doc.Blocks)
	}
}

func TestAnswerDocumentRedlineMatrix_MissingMemberSupplementIsSingleMarkedAppendOnly(t *testing.T) {
	mu := types.NewMutableState("列出完整公开函数")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "公开函数",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Eval", "EvalAll"},
		SupportRefs: []string{
			"Eval @ internal/analysis/criterion/eval.go:15",
			"EvalAll @ internal/analysis/criterion/eval.go:36",
		},
		MemberNotes: []string{
			"Eval 对单个 Criterion 求值。",
			"EvalAll 批量求值所有 Criterion。",
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
		Text: "模型先给出了整体说明，但没有列出完整成员。",
	}}}
	view := &types.AnswerSemanticView{}
	normalizeAnswerDocumentForPreEmit("redline_matrix", doc, view, ctx, newPreEmitCheckContext(ctx))

	if doc.Blocks[0].Text != "模型先给出了整体说明，但没有列出完整成员。" {
		t.Fatalf("append-only supplement must not rewrite model-authored summary: %q", doc.Blocks[0].Text)
	}
	var markedSupplements int
	for _, block := range doc.Blocks {
		if strings.Contains(block.Title, "系统按已验证证据补充缺失成员") {
			markedSupplements++
			if !preEmitSystemEnumerationRowSupplementBlock(block) {
				t.Fatalf("missing-member supplement must be recognized by the shared system-supplement predicate: %+v", block)
			}
		}
		if strings.Contains(block.Title, "系统按已验证证据补充成员：") {
			t.Fatalf("legacy aggregate carrier must not duplicate the missing-member supplement: %+v", doc.Blocks)
		}
	}
	if markedSupplements != 1 {
		t.Fatalf("expected exactly one clearly marked missing-member supplement, got %d: %+v", markedSupplements, doc.Blocks)
	}
}

func TestAnswerDocumentRedlineMatrix_CurrentSourceVisibleLocationHitIsLanguageAgnostic(t *testing.T) {
	cases := []struct {
		name    string
		ev      types.EvidenceItem
		visible string
	}{
		{
			name: "arkts_basename_with_anchor",
			ev: types.EvidenceItem{
				AnchorSymbol:    "IndexPage",
				Source:          "entry/src/main/ets/pages/Index.ets",
				LineStart:       20,
				Scope:           types.ScopeLine,
				GroundingStatus: types.GroundingGrounded,
			},
			visible: "入口页面 IndexPage 位于 Index.ets:20。",
		},
		{
			name: "cpp_full_path",
			ev: types.EvidenceItem{
				AnchorSymbol:    "ResSchedService::Init",
				Source:          "services/resschedservice/src/res_sched_service.cpp",
				LineStart:       188,
				Scope:           types.ScopeLine,
				GroundingStatus: types.GroundingGrounded,
			},
			visible: "当前源码锚点是 services/resschedservice/src/res_sched_service.cpp:188，对应 ResSchedService::Init。",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !currentSourceCitationSupplementVisibleLocationHit(tc.ev, tc.visible) {
				t.Fatalf("visible source location should suppress duplicate current-source supplement across languages:\nev=%+v\nvisible=%s", tc.ev, tc.visible)
			}
		})
	}
}
