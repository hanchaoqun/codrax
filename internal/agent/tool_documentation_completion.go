package agent

import "github.com/hanchaoqun/codrax/internal/types"

func acceptedToolDocumentationOnly(ctx *types.AgentContext) bool {
	return ctx != nil && ctx.Mutable != nil && ctx.AnalysisIR != nil &&
		types.ToolDocumentationOnlyRequested(&ctx.AnalysisIR.RequestModel) &&
		ctx.Mutable.HasAcceptedToolDocumentationCompletion(&ctx.AnalysisIR.RequestModel)
}
