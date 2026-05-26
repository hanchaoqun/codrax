package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

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

func renderExplorerRepoMapTypedFirstHop(ctx *types.AgentContext) string {
	policy := repoMapNavigationPolicyForContext(ctx)
	if policy.Empty() ||
		!policy.HasRoute(types.RepoMapNavigationRouteTaskMap) ||
		!policy.HasRoute(types.RepoMapNavigationRouteRelationMap) {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Typed Repo Map First Hop\n\n")
	b.WriteString("For this typed relation / call-flow shape, make the first broad structural navigation move `repo_map(view=\"task_map\")` with the typed query terms below, unless exact required files already identify the files you will inspect. After task_map/file_map narrows candidate files or symbols, prefer `repo_map(view=\"relation_map\")` around those chosen `sources` / `scope` / `scopes` before falling back to broad grep expansion. This is soft guidance, not a read obligation: exact read_file evidence still wins when the current source line is already pinned.\n")
	if terms := policy.QueryTermList(8); len(terms) > 0 {
		b.WriteString("- Suggested `query` terms: `")
		b.WriteString(strings.Join(terms, "`, `"))
		b.WriteString("`.\n")
	}
	b.WriteString("- Keep multi-repo or multi-topic scopes partitioned: choose the active sub-repo as `path`, then keep `sources` and `scope` relative to that selected path.\n\n")
	return b.String()
}
