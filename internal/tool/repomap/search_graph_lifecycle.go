package repomap

import "github.com/hanchaoqun/codrax/internal/types"

// A selected-directory lens uses graph-local coordinates while it runs. Do not
// leave that projection in the shared repository graph slot for later stages.
// Selection only inspects already resident graphs; it never widens a scan.
func sourceInventorySharedGraph(ctx *types.BusContext, selected *Graph) *Graph {
	if ctx == nil || ctx.RepoRoot == "" {
		return nil
	}
	match := func(value any, root string) *Graph {
		g, _ := value.(*Graph)
		if g != nil && g.Root != "" && root != "" && sameRepoMapRoot(g.Root, root) {
			return g
		}
		return nil
	}
	roots := []string{ctx.RepoRoot}
	// Apply changes the active physical root. Retain an already resident main
	// checkout graph as the same navigation baseline, not as a fresh worktree
	// index: its Root and all local coordinates remain unchanged.
	if ctx.WorktreePath != "" && sameRepoMapRoot(ctx.WorktreePath, ctx.RepoRoot) && ctx.MainRepoRoot != "" {
		roots = append(roots, ctx.MainRepoRoot)
	}
	for _, root := range roots {
		if g := match(selected, root); g != nil {
			return g
		}
		if ctx.Mutable != nil {
			if g := match(ctx.Mutable.SearchGraph(), root); g != nil {
				return g
			}
		}
		if g := match(ctx.SearchGraph, root); g != nil {
			return g
		}
		if mg := MultiGraphFromContext(ctx); mg != nil {
			for _, g := range mg.AllGraphs() {
				if found := match(g, root); found != nil {
					return found
				}
			}
		}
	}
	return nil
}

func restoreSourceInventorySharedGraph(ctx *types.BusContext, shared, selected *Graph) {
	if ctx == nil || ctx.Mutable == nil {
		return
	}
	if shared == nil {
		ctx.Mutable.SetSearchGraph(nil)
		return
	}
	ctx.Mutable.SetSearchGraph(shared)
	if shared != nil && selected != nil && !sameRepoMapRoot(shared.Root, selected.Root) {
		if key := scopedGraphProjectionCacheKey(shared.Root, selected.Root); key != "" {
			ctx.Mutable.SetScopedSearchGraph(key, selected)
		}
	}
}
