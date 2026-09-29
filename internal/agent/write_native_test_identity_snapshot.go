package agent

import "github.com/hanchaoqun/codrax/internal/types"

// No historical context-pack fallback. Registration has a separately labelled
// source view bound to the private grant; other consumers retain current-plan
// and post-apply-channel matching. Neither view grants verification authority.
func buildWriteNativeTestIdentitySnapshot(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil || !ctx.Mode.IsWrite() {
		return ""
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		if ctx.Stage == types.StagePlan {
			return ctx.Mutable.NativeTestRegistrationIdentitySnapshot(ctx.RepoRoot)
		}
		return ""
	}
	return types.RenderCurrentNativeTestIdentitySnapshot(plan.ID, ctx.Mutable.ChangeReport())
}
