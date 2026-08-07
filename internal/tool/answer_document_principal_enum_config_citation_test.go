package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizePrincipalEnumerationRowBlocks_ConfigPrecedenceKeepsTypedRowCitations(t *testing.T) {
	mu := types.NewMutableState("config precedence")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("default", "defaultMaxSteps", "cmd/root.go", 88, "code default"),
		enumEvidence("yaml", "pipeline_max_steps", "codrax.yaml.example", 485, "yaml setting"),
		enumEvidence("cli", "flagMaxSteps", "cmd/root.go", 649, "CLI flag"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "pipeline_max_steps 配置优先级（后者覆盖前者）",
		Value:   "3",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"代码默认值", "codrax.yaml", "CLI --pipeline-max-steps"},
		SupportRefs: []string{
			"cmd/root.go:88",
			"codrax.yaml.example:485",
			"cmd/root.go:649",
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind: string(types.ReqConfigMapping),
			},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:    "summary",
				Kind:  types.BlockSummary,
				Text:  "pipeline_max_steps uses layered precedence.",
				Items: []types.AnswerBlockItem{{ID: "default", CitationRef: 0}},
			},
			{
				ID:          "precedence",
				Kind:        types.BlockTable,
				Title:       "pipeline_max_steps 配置优先级（后者覆盖前者）",
				SurfaceRole: types.SurfacePrincipal,
				Text: "| 层级 | 来源文件:行 |\n" +
					"|---|---|\n" +
					"| 代码默认值 | cmd/root.go:88 |\n" +
					"| codrax.yaml | codrax.yaml.example:485 |\n" +
					"| CLI --pipeline-max-steps | cmd/root.go:649 |",
			},
		},
		Citations: []types.Citation{
			{File: "cmd/root.go", Line: 88},
			{File: "codrax.yaml.example", Line: 485},
			{File: "cmd/root.go", Line: 649},
		},
	}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	if got := len(doc.Blocks[1].Items); got != 0 {
		t.Fatalf("model-authored Markdown table items = %d, want 0: %+v", got, doc.Blocks[1].Items)
	}
	if fixed := normalizeUnusedCitationPoolEntries(doc, ctx); fixed != 0 {
		t.Fatalf("typed Markdown row citations were pruned/remapped: fixed=%d citations=%+v", fixed, doc.Citations)
	}
	if got := len(doc.Citations); got != 3 {
		t.Fatalf("typed precedence row citation count = %d, want 3: %+v", got, doc.Citations)
	}
	if fixed := normalizeUnusedCitationPoolEntries(doc, ctx); fixed != 0 {
		t.Fatalf("typed Markdown citation retention is not idempotent: fixed=%d citations=%+v", fixed, doc.Citations)
	}
}

func TestNormalizePrincipalEnumerationRowBlocks_MarkdownTableDoesNotInventCitations(t *testing.T) {
	mu := types.NewMutableState("runtime modes")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "runtime modes",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"read", "write"},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:    "modes",
		Kind:  types.BlockTable,
		Title: "runtime modes",
		Text:  "| mode |\n|---|\n| read |\n| write |",
	}}}

	normalizePrincipalEnumerationRowBlocks(doc, ctx)
	normalizeUnusedCitationPoolEntries(doc, ctx)
	if len(doc.Citations) != 0 || len(doc.Blocks[0].Items) != 0 {
		t.Fatalf("non-file rows invented citation authority: blocks=%+v citations=%+v", doc.Blocks, doc.Citations)
	}
}

func TestNormalizePrincipalAggregateItemCitationRefs_CallChainUsesExactSupportRows(t *testing.T) {
	mu := types.NewMutableState("call chain")
	mu.AppendEvidence([]types.EvidenceItem{
		enumEvidence("logger", "Logger::log", "src/logger.cpp", 29, "entry"),
		enumEvidence("sink", "sink_->write", "src/logger.cpp", 36, "virtual call"),
		enumEvidence("console", "ConsoleSink::write", "include/logx/console_sink.hpp", 10, "override"),
		enumEvidence("guard", "kind==\"console\" guard", "src/registry.cpp", 17, "selection"),
		enumEvidence("return", "ConsoleSink return", "src/registry.cpp", 18, "factory return"),
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind: types.AnswerAggregateMemberSet, Label: "完整调用路径", Value: "3",
			Role:    types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{"Logger::log", "sink_->write", "ConsoleSink::write"},
			SupportRefs: []string{
				"src/logger.cpp:29", "src/logger.cpp:36", "include/logx/console_sink.hpp:10",
			},
		},
		{
			Kind: types.AnswerAggregateMemberSet, Label: "运行时 sink 选择链路", Value: "2",
			Role:        types.AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"kind==\"console\" guard", "ConsoleSink return"},
			SupportRefs: []string{"src/registry.cpp:17", "src/registry.cpp:18"},
		},
	})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentTrace,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID: "path", Kind: types.BlockOrderedList, Title: "完整调用路径",
				Items: []types.AnswerBlockItem{
					{ID: "logger", Label: "Logger::log", CitationRef: 0},
					{ID: "sink", Label: "sink_->write", CitationRef: 1},
					{ID: "console", Label: "ConsoleSink::write", CitationRef: -1},
				},
			},
			{
				ID: "select", Kind: types.BlockOrderedList, Title: "运行时 sink 选择链路",
				Items: []types.AnswerBlockItem{
					{ID: "guard", Label: "kind==\"console\" guard", CitationRef: 0},
					{ID: "return", Label: "ConsoleSink return", CitationRef: 99},
				},
			},
		},
		Citations: []types.Citation{
			{File: "wrong.cpp", Line: 1},
			{File: "include/logx/console_sink.hpp", Line: 11},
		},
	}

	if fixed := normalizePrincipalAggregateItemCitationRefsWithContext(doc, ctx); fixed != 5 {
		t.Fatalf("fixed=%d, want every exact principal item rebound: blocks=%+v citations=%+v", fixed, doc.Blocks, doc.Citations)
	}
	want := []types.Citation{
		{File: "src/logger.cpp", Line: 29},
		{File: "src/logger.cpp", Line: 36},
		{File: "include/logx/console_sink.hpp", Line: 10},
		{File: "src/registry.cpp", Line: 17},
		{File: "src/registry.cpp", Line: 18},
	}
	items := append(append([]types.AnswerBlockItem{}, doc.Blocks[0].Items...), doc.Blocks[1].Items...)
	for i, item := range items {
		if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) || doc.Citations[item.CitationRef] != want[i] {
			t.Fatalf("item[%d]=%+v citation=%+v want=%+v", i, item, doc.Citations, want[i])
		}
	}
	if fixed := normalizePrincipalAggregateItemCitationRefsWithContext(doc, ctx); fixed != 0 {
		t.Fatalf("citation binding must be idempotent, fixed=%d", fixed)
	}

	// Production-wiring pin: the complete pre-emit chain must invoke the
	// citation-only aggregate binder after weaker generic label candidates.
	for bi := range doc.Blocks {
		for ii := range doc.Blocks[bi].Items {
			doc.Blocks[bi].Items[ii].CitationRef = 0
		}
	}
	normalizeAnswerDocumentForPreEmit("test", doc, &types.AnswerSemanticView{Family: types.QFCallChain}, ctx, newPreEmitCheckContext(ctx))
	items = append(append([]types.AnswerBlockItem{}, doc.Blocks[0].Items...), doc.Blocks[1].Items...)
	for i, item := range items {
		if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) || doc.Citations[item.CitationRef].File != want[i].File || doc.Citations[item.CitationRef].Line != want[i].Line {
			t.Fatalf("pre-emit wiring item[%d]=%+v citation=%+v want=%+v", i, item, doc.Citations, want[i])
		}
	}
}

func TestNormalizePrincipalAggregateItemCitationRefs_AmbiguousLabelStandsDown(t *testing.T) {
	mu := types.NewMutableState("ambiguous rows")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{Kind: types.AnswerAggregateMemberSet, Label: "left", Value: "2", Role: types.AnswerAggregateRolePrincipalAnswer, Members: []string{"Run", "Left"}, SupportRefs: []string{"a.go:10", "a.go:11"}},
		{Kind: types.AnswerAggregateMemberSet, Label: "right", Value: "2", Role: types.AnswerAggregateRolePrincipalAnswer, Members: []string{"Run", "Right"}, SupportRefs: []string{"b.go:20", "b.go:21"}},
	})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace}}}
	doc := &types.AnswerDocumentV2{
		Blocks:    []types.AnswerBlock{{ID: "generic", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{ID: "run", Label: "Run", CitationRef: 0}}}},
		Citations: []types.Citation{{File: "keep.go", Line: 1}},
	}
	if fixed := normalizePrincipalAggregateItemCitationRefsWithContext(doc, ctx); fixed != 0 || doc.Blocks[0].Items[0].CitationRef != 0 {
		t.Fatalf("ambiguous unscoped member must stand down: fixed=%d doc=%+v", fixed, doc)
	}
}
