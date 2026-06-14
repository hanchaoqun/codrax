package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/types"
)

// configureWriteScopedMultiRepo turns a multi-repo parent run into a
// single-sub-repo write run once the typed active set has converged to exactly
// one sub-repo. Write-mode worktrees require a real git root, while the
// multi-repo parent is intentionally only a container. The controller therefore
// applies one repo-scoped batch at a time and leaves cross-repo fan-out to the
// durable workflow layer.
func (o *Orchestrator) configureWriteScopedMultiRepo(originalRoot string) error {
	if o == nil || o.busCtx == nil || !o.busCtx.Mode.IsWrite() {
		return nil
	}
	mg, _ := o.busCtx.MultiGraph.(*multigraph.MultiGraph)
	if mg == nil || mg.IsSingle() || len(o.busCtx.SubRepos) < 2 {
		return nil
	}

	active := activeSubRepoSnapshots(o.busCtx.SubRepos, o.busCtx.PendingSubRepos)
	if len(active) != 1 {
		names := make([]string, 0, len(active))
		for _, sr := range active {
			names = append(names, sr.RootRel)
		}
		if len(names) == 0 {
			names = append(names, "none")
		}
		msg := fmt.Sprintf("write mode on a multi-repo workspace requires exactly one active sub-repo; active=%s. Pin one sub-repo with --focus or /repos focus, then rerun the write task.", strings.Join(names, ", "))
		o.busCtx.Mutable.SetResultPlain(msg)
		o.busCtx.TaskState.LastError = msg
		return fmt.Errorf("%s", msg)
	}

	sr := active[0]
	if strings.TrimSpace(sr.RootAbs) == "" {
		msg := fmt.Sprintf("write mode selected sub-repo %q but its absolute root is missing from the topology snapshot", sr.RootRel)
		o.busCtx.Mutable.SetResultPlain(msg)
		o.busCtx.TaskState.LastError = msg
		return fmt.Errorf("%s", msg)
	}

	o.busCtx.ActiveSubRepo = cloneSubRepoSnapshot(sr)
	o.busCtx.RepoRoot = sr.RootAbs
	o.busCtx.MainRepoRoot = sr.RootAbs
	o.busCtx.MultiGraph = nil
	o.busCtx.PendingSubRepos = nil
	if o.busCtx.Mutable != nil {
		o.busCtx.Mutable.SetRepoRoot(sr.RootAbs)
	}
	logging.Info("[orchestrator] write mode scoped multi-repo parent %s to sub-repo %s at %s", originalRoot, sr.RootRel, sr.RootAbs)
	return nil
}

func activeSubRepoSnapshots(subs []types.SubRepoSnapshot, pending []string) []types.SubRepoSnapshot {
	if len(subs) == 0 {
		return nil
	}
	pendingSet := make(map[string]bool, len(pending))
	for _, p := range pending {
		pendingSet[strings.TrimSpace(p)] = true
	}
	out := make([]types.SubRepoSnapshot, 0, len(subs))
	for _, sr := range subs {
		if sr.RootRel == "" {
			continue
		}
		if !pendingSet[sr.RootRel] {
			out = append(out, sr)
		}
	}
	return out
}

func cloneSubRepoSnapshot(sr types.SubRepoSnapshot) *types.SubRepoSnapshot {
	cp := sr
	cp.PrimaryLangs = append([]string(nil), sr.PrimaryLangs...)
	return &cp
}
