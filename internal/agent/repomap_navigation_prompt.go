package agent

import "github.com/hanchaoqun/codrax/internal/types"

func repoMapNavigationPolicyForContext(ctx *types.AgentContext) types.RepoMapNavigationPolicy {
	if ctx == nil || ctx.AnalysisIR == nil {
		return types.RepoMapNavigationPolicy{}
	}
	return types.CompileRepoMapNavigationPolicy(
		ctx.AnalysisIR.RequestModel,
		&ctx.AnalysisIR.AnswerContract,
		ctx.ExploreLanePlan,
	)
}

func renderRepoMapTypedNavigationPolicy(ctx *types.AgentContext) string {
	policy := repoMapNavigationPolicyForContext(ctx)
	return policy.RenderMarkdownHint("", "")
}
