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
		repoMapPolicyStartsWithSourceInventory(policy) ||
		repoMapFirstHopSuppressedForRuntimeArtifact(ctx) {
		return ""
	}
	first, ok := policy.FirstHopStep()
	if !ok {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Typed Repo Map First Hop\n\n")
	b.WriteString("For this typed source-code navigation shape, make the first discovery tool batch include `repo_map(view=\"")
	b.WriteString(string(first.Route))
	b.WriteString("\")` before broad `grep` or exploratory `read_file`, unless exact required files already identify the files you will inspect. This is soft guidance, not a read obligation: exact read_file evidence still wins when the current source line is already pinned.\n")
	if policy.HasRoute(types.RepoMapNavigationRouteRelationMap) {
		b.WriteString("For relation / call-flow shapes, after task_map/file_map narrows candidate files or symbols, prefer `repo_map(view=\"relation_map\")` around those chosen `sources` / `scope` / `scopes` before falling back to broad grep expansion.\n")
	}
	if policy.HasRoute(types.RepoMapNavigationRouteSemanticGraph) {
		b.WriteString("For architecture or topology questions, use `repo_map(view=\"semantic_subgraph\")` as the early map of hubs, bridges, and chains before choosing files to read.\n")
	}
	if policy.HasRoute(types.RepoMapNavigationRouteEditImpact) {
		b.WriteString("For impact questions, use `repo_map(view=\"edit_impact\")` as the early affected-surface map before reading individual call sites.\n")
	}
	if policy.HasPurpose(types.RepoMapNavigationPurposeEntrypoint) {
		b.WriteString("For entry/startup/handler role-location questions, do not assume the answer is literally named `main`; use file_map/task_map plus route, manifest, registration, lifecycle callback, command-root, and generated-adjacent entry surfaces, then verify the selected source lines.\n")
	}
	if terms := policy.QueryTermList(8); len(terms) > 0 {
		b.WriteString("- Suggested exact `query` surfaces: `")
		b.WriteString(strings.Join(terms, "`, `"))
		b.WriteString("`. Avoid full sentences or broad glue words when these surfaces are available.\n")
	}
	b.WriteString("- Keep multi-repo or multi-topic scopes partitioned: choose the active sub-repo as `path`, then keep `sources` and `scope` relative to that selected path.\n\n")
	return b.String()
}

func renderRepoMapSourceInventoryNarrowingGuidance() string {
	return "When the task needs a source inventory or member/count checklist, call `repo_map` with `view=\"source_inventory\"`, the narrowest typed/requested `scope` or `scopes` available, model-chosen `roles`, a compact `query` when exact surfaces are known, and `include_attributes=false` by default. If no typed scope exists yet, use at most one bounded root summary as orientation with low `top_n` and no row-local attributes, then narrow by `scope`/`roles` before paging; do not make broad root inventory the default first hop, and do not page a broad root lens merely to satisfy non-inventory architecture/relation questions. Add `attribute_roles` only after choosing a narrow scope/member that needs row-local details."
}

func repoMapPolicyStartsWithSourceInventory(policy types.RepoMapNavigationPolicy) bool {
	return len(policy.Steps) > 0 &&
		policy.Steps[0].Route == types.RepoMapNavigationRouteSourceInventory &&
		policy.Steps[0].Purpose == types.RepoMapNavigationPurposeInventory
}

func repoMapFirstHopSuppressedForRuntimeArtifact(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	phase := runtimeSourceNavigationPhaseForExplorer(ctx, true)
	if phase.RuntimeProbePreferredBeforeSource {
		return true
	}
	if phase.TraceQueryAttempted || phase.RuntimeObservationAvailable {
		return false
	}
	return phase.RuntimeTraceCarrier && !phase.SourceOwnerRequired
}
