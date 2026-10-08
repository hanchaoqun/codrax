package agent

import "github.com/hanchaoqun/codrax/internal/types"

func answerDocumentMeasurementMemberPayload(ctx *types.AgentContext, block types.AnswerBlock) bool {
	if ctx == nil || ctx.AnalysisIR == nil || block.RuntimeMeasurement == nil || !block.RuntimeMeasurement.IsBound() {
		return false
	}
	rm := &ctx.AnalysisIR.RequestModel
	if !types.RuntimeMeasurementMemberSetDomain(rm, types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, types.ObservationLedger{})) {
		return false
	}
	// Rebind a copy: an accepted draft can outlive a query/window change.
	receipt := *block.RuntimeMeasurement
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	return types.BindRuntimeMeasurementReceipt(&receipt, contract) && receipt.BoundTable.CoversMemberSet(rm)
}

func answerDocumentMemberPayloadForContext(ctx *types.AgentContext, block types.AnswerBlock) bool {
	if block.RuntimeMeasurement != nil {
		return answerDocumentMeasurementMemberPayload(ctx, block)
	}
	return answerDocumentBlockHasVisibleMemberPayload(block)
}

func answerDocumentHasMarkedMeasurementMembers(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	doc := ctx.Mutable.AnswerDocumentV2()
	if doc == nil {
		return false
	}
	for _, block := range doc.Blocks {
		if block.RuntimeMeasurement != nil && answerBlockHasFacet(block, string(types.RequestedAnswerDimensionMemberSet)) {
			return true
		}
	}
	return false
}
