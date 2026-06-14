package orchestrator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ValidateChangePlanScope is the design §4.5.5 cross-sub-repo
// fail-loud gate for write-mode ChangePlan emission. Returns nil when
// every TargetPath belongs to a single sub-repo (or topology is
// single-repo / unset). Returns a Violation when the plan touches
// MORE THAN ONE sub-repo — multi-repo write is contractually banned;
// the operator MUST split the work into one ChangePlan per sub-repo.
//
// The check uses BusContext.SubRepos (Phase 4.1 snapshot) so we don't
// need to take a *MultiGraph reference here. Single-repo posture
// (IsSingleRepoSnapshot) is treated identically to "no topology" —
// every path resolves to the same sub-repo by construction.
//
// Stage = "plan". Layer = "write_contract". Producer is wired into
// stage_hooks plan post-hook so the violation lands in the closure
// before apply ever runs.
func ValidateChangePlanScope(busCtx *types.BusContext, plan *types.ChangePlan) *types.Violation {
	if busCtx == nil || plan == nil {
		return nil
	}
	if len(busCtx.SubRepos) == 0 || types.IsSingleRepoSnapshot(busCtx.SubRepos) {
		return nil
	}
	if len(plan.TargetPaths) == 0 {
		return nil
	}
	if busCtx.ActiveSubRepo != nil {
		if prefixed := workspacePrefixedTargetPaths(busCtx.SubRepos, plan.TargetPaths); len(prefixed) > 0 {
			sort.Strings(prefixed)
			detail := fmt.Sprintf("ChangePlan target paths use multi-repo workspace prefixes while write mode is scoped to sub-repo %q: %s", busCtx.ActiveSubRepo.RootRel, strings.Join(prefixed, ", "))
			return &types.Violation{
				Kind:       types.ViolWriteCrossSubRepoForbidden,
				Detail:     detail,
				Repair:     "re-emit the ChangePlan with paths relative to the active sub-repo root; do not include the multi-repo workspace prefix in target paths",
				Stage:      "plan",
				ClusterKey: "write_scoped_sub_repo_prefixed_target",
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "change_plan.target_paths",
					Reason:     "workspace-prefixed target paths in repo-scoped write batch",
					Confidence: 1.0,
				},
			}
		}
	}
	hits := make(map[string][]string) // sub-repo RootRel → TargetPaths owned
	for _, p := range plan.TargetPaths {
		sr := types.SubRepoSnapshotByRelPath(busCtx.SubRepos, p)
		if sr == nil {
			continue
		}
		hits[sr.RootRel] = append(hits[sr.RootRel], p)
	}
	if len(hits) <= 1 {
		return nil
	}
	// Cross-sub-repo plan — fail-loud.
	subRepoNames := make([]string, 0, len(hits))
	for rr := range hits {
		subRepoNames = append(subRepoNames, rr)
	}
	sort.Strings(subRepoNames)
	detail := fmt.Sprintf("ChangePlan touches %d sub-repos: %s — multi-repo write banned by design §4.5.5", len(subRepoNames), strings.Join(subRepoNames, ", "))
	repair := fmt.Sprintf("split into %d separate runs, one per sub-repo (cd into each sub-repo and re-issue the request, OR `/repos focus <slug>` then re-run with multi_repo_max_active=1)", len(subRepoNames))

	return &types.Violation{
		Kind:       types.ViolWriteCrossSubRepoForbidden,
		Detail:     detail,
		Repair:     repair,
		Stage:      "plan",
		ClusterKey: "write_cross_sub_repo",
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "change_plan.sub_repo_scope",
			Reason:     fmt.Sprintf("%d sub-repos in TargetPaths", len(subRepoNames)),
			Confidence: 1.0,
		},
	}
}

func workspacePrefixedTargetPaths(subs []types.SubRepoSnapshot, paths []string) []string {
	if len(subs) == 0 || len(paths) == 0 {
		return nil
	}
	var out []string
	for _, p := range paths {
		clean := strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
		for strings.HasPrefix(clean, "./") {
			clean = strings.TrimPrefix(clean, "./")
		}
		for _, sr := range subs {
			rr := strings.Trim(strings.ReplaceAll(strings.TrimSpace(sr.RootRel), "\\", "/"), "/")
			if rr == "" || rr == "." {
				continue
			}
			if clean == rr || strings.HasPrefix(clean, rr+"/") {
				out = append(out, p)
				break
			}
		}
	}
	return out
}
