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
