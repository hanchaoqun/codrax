package orchestrator

import writeimpact "github.com/hanchaoqun/codrax/internal/writeflow/impact"

// These roots are controller-owned checkout identities, not model path aliases.
// A pre-apply main-checkout graph and a current worktree graph share repository
// relative coordinates only through those explicitly installed roots.
func (o *Orchestrator) writeImpactGraphProvider() writeimpact.GraphProvider {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return nil
	}
	return writeimpact.GraphProviderFromSearchGraphForRoots(o.busCtx.Mutable.SearchGraph(),
		o.busCtx.RepoRoot, o.busCtx.MainRepoRoot, o.busCtx.WorktreePath)
}
