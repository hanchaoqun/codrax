package agent

import "github.com/hanchaoqun/codrax/internal/types"

// The ir* helpers read analyzer-sourced hints from the AnalysisIR
// populated by the analyze stage. They return zero values when the IR
// is nil (analyze failed, or unit tests that skip the analyze stage).
//
// Before batch B5b-β these helpers fell back to legacy CurrentTask*
// fields on AgentContext; those fields were deleted along with their
// TaskItem siblings.

func irKeywords(ctx *types.AgentContext) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return ctx.AnalysisIR.RequestModel.AnalyzerHints.Keywords
}

func irEntities(ctx *types.AgentContext) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities
}

// irPrimaryEntities returns the pre-merge top-level entity snapshot
// captured by the analyzer before sub-topic entities were unioned into
// Entities for breadth ranking. Returns nil when no sub-topic merge
// ran (single-topic questions) or when IR is missing — callers should
// fall back to irEntities for the merged list.
func irPrimaryEntities(ctx *types.AgentContext) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return ctx.AnalysisIR.RequestModel.AnalyzerHints.PrimaryEntities
}

func irMentionedEntities(ctx *types.AgentContext) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return ctx.AnalysisIR.RequestModel.AnalyzerHints.MentionedEntities
}

// irDomainHints returns the unique non-empty Domain tags attached to
// every TermSymbol in the analyzer's TermGraph. Populated by the
// normalizer's SymbolResolver when a term is repo-grounded; empty when
// the resolver was unavailable or the TermGraph has no symbol terms.
// Consumed by keyword_search to boost files whose FileInfo.Package
// matches any hint — i.e., siblings of the answer symbol in the same
// package — without naming the symbol directly.
func irDomainHints(ctx *types.AgentContext) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, c := range ctx.AnalysisIR.RequestModel.TermGraph.Canonical {
		if c.Kind != types.TermSymbol || c.Domain == "" || seen[c.Domain] {
			continue
		}
		seen[c.Domain] = true
		out = append(out, c.Domain)
	}
	return out
}

func irQuestionKind(ctx *types.AgentContext) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	return ctx.AnalysisIR.RequestModel.AnalyzerHints.Kind
}

func irAnswerShape(ctx *types.AgentContext) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	return ctx.AnalysisIR.RequestModel.AnalyzerHints.Shape
}

// irComplexity returns the analyzer-classified complexity for the
// current request, or the empty string when the IR is missing (unit
// tests, or an analyze-phase failure that still let the pipeline
// continue). Callers that branch on this value should treat "" the
// same as ComplexityModerate so the pre-complexity-aware behavior
// is the safe default.
//
// Complexity flows from the analyzer's emit_analysis call (field
// `complexity`) through analyzer_complexity.reconcileComplexity
// (which can override the LLM choice on structural signals) into
// RequestModel.Complexity. The explorer reads it via this helper
// to scale its own runtime budgets: keyword-search top-N cap
// (T1b), ERM requirement-satisfaction thresholds (T1c), and
// potential future per-complexity behaviors.
func irComplexity(ctx *types.AgentContext) types.Complexity {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	return ctx.AnalysisIR.RequestModel.Complexity
}

// analyzerRequiredFilesFromIR returns the analyzer's EvidencePlan
// RequiredFiles list (T3a), or nil when the IR is unavailable. This
// is a deliberate thin wrapper so the explorer's consumer loop
// reads from a single, documented accessor instead of walking the
// nested IR → EvidencePlan → RequiredFiles path inline.
func analyzerRequiredFilesFromIR(ctx *types.AgentContext) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return ctx.AnalysisIR.EvidencePlan.RequiredFiles
}
