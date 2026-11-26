package agent

import "github.com/hanchaoqun/codrax/internal/types"

const explorerFrameFollowupTeaching = "The first frame bundle is already available. Use its published values, source lines and evidence IDs directly for covered facts. Identify only evidence gaps that can change the answer. Send all independent trace_query calls whose selectors are already known together in this tool-call batch (up to eight); this includes distinct business-span references already returned by the bundle. Wait for a result only when it supplies a selector or a decision needed by the next query. When no material gap remains, submit the completion with current evidence."

func explorerFrameBundleFollowups(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.Stage != types.StageExplore || ctx.ExploreDispatchKind == types.NodeValidate ||
		ctx.Mutable == nil || !ctx.AttachedTraceMaterial.SinglePhysicalSource() {
		return false
	}
	if _, _, _, ok := types.KnownSingleFrameTraceTarget(ctx.Mutable.RequestModel()); !ok {
		return false
	}
	for _, result := range ctx.Mutable.DispatchToolResults() {
		if len(result.TraceFrameBundleCoveredViews) > 0 {
			return true
		}
	}
	return false
}
