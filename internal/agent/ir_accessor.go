package agent

import "github.com/hanchaoqun/codrax/internal/types"

// The ir* helpers read analyzer-sourced hints from the AnalysisIR
// populated by the analyze stage. When the IR is nil (pre-v3 call
// paths, fail-safe analyze output, unit tests that skip analyze),
// they fall back to the legacy AgentContext.CurrentTask* fields so
// behavior is unchanged for callers that have not migrated yet.
//
// Batch B5b-α routes explorer and finalizer reads through these
// helpers. B5b-β deletes the legacy CurrentTask* fields once every
// consumer has migrated.

func irKeywords(ctx *types.AgentContext) []string {
	if ctx == nil {
		return nil
	}
	if ctx.AnalysisIR != nil {
		if h := ctx.AnalysisIR.RequestModel.AnalyzerHints.Keywords; len(h) > 0 {
			return h
		}
	}
	return ctx.CurrentTaskKeywords
}

func irEntities(ctx *types.AgentContext) []string {
	if ctx == nil {
		return nil
	}
	if ctx.AnalysisIR != nil {
		if h := ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities; len(h) > 0 {
			return h
		}
	}
	return ctx.CurrentTaskEntities
}

func irQuestionKind(ctx *types.AgentContext) string {
	if ctx == nil {
		return ""
	}
	if ctx.AnalysisIR != nil {
		if k := ctx.AnalysisIR.RequestModel.AnalyzerHints.Kind; k != "" {
			return k
		}
	}
	return ctx.CurrentTaskQuestionKind
}

func irAnswerShape(ctx *types.AgentContext) string {
	if ctx == nil {
		return ""
	}
	if ctx.AnalysisIR != nil {
		if s := ctx.AnalysisIR.RequestModel.AnalyzerHints.Shape; s != "" {
			return s
		}
	}
	return ctx.CurrentTaskAnswerShape
}
