package agent

import "github.com/hanchaoqun/codrax/internal/types"

// No historical context-pack fallback: only the currently held report and
// concrete active plan can supply this identity snapshot. The types renderer
// also requires an explicit post-apply channel and byte-equal plan IDs.
func buildWriteNativeTestIdentitySnapshot(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil || !ctx.Mode.IsWrite() {
		return ""
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		return ""
	}
	return types.RenderCurrentNativeTestIdentitySnapshot(plan.ID, ctx.Mutable.ChangeReport())
}
