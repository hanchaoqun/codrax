package agent

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

func nativeDisplayDimensionBindingInstruction(ctx *types.AgentContext, index int, zh bool) string {
	if ctx == nil {
		return ""
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if view == nil {
		return ""
	}
	for _, candidate := range view.RuntimeMeasurementContract.NativeDisplayDimensionChoices() {
		if candidate != index {
			continue
		}
		if zh {
			return fmt.Sprintf("  - 若已公布的完整原生表回答此维度，在 kind=table 上选择精确 runtime_measurement，并加 requested_dimension_indices:[%d]；同一表可绑定多个实际回答的维度。自动展示表用 add_blocks 添加这份仅含选择器与索引的模型块，系统去掉重复默认表；已有模型选择器用 replace_blocks 只改索引，保留其它字段。不要重写原始行、补重复 facet 或把数据完整当因果/源码证明。\n", index)
		}
		return fmt.Sprintf("  - If a published complete native table answers this dimension, use its exact runtime_measurement selector on kind=table with requested_dimension_indices:[%d]; one table may bind several dimensions it actually answers. For an automatic table use add_blocks with only the selector and indices; its duplicate default is removed. For an existing model selector use replace_blocks changing only indices and preserving its other fields. Do not retype rows, add duplicate facets, or turn data completeness into causal/source proof.\n", index)
	}
	return ""
}

func answerDocumentNativeDisplayDimensionCovered(ctx *types.AgentContext, doc *types.AnswerDocumentV2, index int) bool {
	if ctx == nil || doc == nil || index <= 0 {
		return false
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if view == nil {
		return false
	}
	for _, block := range doc.Blocks {
		indices, valid := types.BoundNativeDisplayDimensions(block, view.RuntimeMeasurementContract)
		if !valid {
			continue
		}
		for _, selected := range indices {
			if selected == index {
				return true
			}
		}
	}
	return false
}

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
