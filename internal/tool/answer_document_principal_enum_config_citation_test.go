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
