package tracecatalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

const MaxHistoricalRuns = 8

// Resume re-captures only the historical roster under an independently
// authorized, identical canonical root. The caller must revalidate Parameters
// using its current tool schema and rebuild the parallel plan metadata. Neither
// historical successes nor declared-window/source permissions are restored.
func Resume(ctx context.Context, authorizedRoot string, old Snapshot, plans []Plan) (*Catalog, error) {
	if err := validateSnapshot(old); err != nil {
		return nil, err
	}
	if len(old.History) >= MaxHistoricalRuns {
		return nil, fmt.Errorf("catalog history has %d runs; resume would exceed the limit without retaining complete history", MaxHistoricalRuns)
	}
	if len(plans) != len(old.Queries) {
		return nil, fmt.Errorf("every historical query must be revalidated")
	}
	root, err := filepath.EvalSymlinks(authorizedRoot)
	if err != nil || !filepath.IsAbs(root) || root != old.Root {
		return nil, fmt.Errorf("resume requires the same independently authorized canonical root")
	}
	c := &Catalog{run: &runIdentity{}, root: root, discovery: Discovery{Complete: true, FilterLabel: "historical roster only"}, artifacts: map[string]Artifact{}, generations: map[string]filegeneration.Identity{}, bindings: map[string]sourceBinding{}, queries: map[string]QueryRecord{}}
	w := discoveryWalker{ctx: ctx, catalog: c, opts: DiscoverOptions{MaxArtifacts: DefaultMaxArtifacts}}
	for _, previous := range old.Artifacts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.discovery.EntriesVisited++
		info, statErr := os.Lstat(previous.Path)
		canonical, canonicalErr := filepath.EvalSymlinks(previous.Path)
		if statErr != nil || canonicalErr != nil || canonical != previous.Path || !info.Mode().IsRegular() {
			a := Artifact{ID: previous.ID, Path: previous.Path, RelativePath: previous.RelativePath, Status: "unavailable", Issues: []Issue{{Code: "resume_member_unavailable", Path: previous.Path, Message: "Historical member is missing, non-regular, or resolves through a changed symbolic link; no current input identity was admitted."}}}
			c.artifacts[a.ID] = a
			w.issue("resume_member_unavailable", previous.RelativePath, nil)
			continue
		}
		w.file(previous.Path, previous.RelativePath, info)
		if a := c.artifacts[previous.ID]; a.Status == "candidate" && a.RevisionID != previous.RevisionID {
			a.Issues = append(a.Issues, Issue{Code: "input_generation_changed", Path: a.Path, Message: "Current input generation differs; historical results are not reused."})
			c.artifacts[a.ID] = a
		}
	}
	for i, plan := range plans {
		if plan.ArtifactID != old.Queries[i].ArtifactID {
			return nil, fmt.Errorf("revalidated plan changed its artifact")
		}
		q, err := c.RegisterPlannedQuery(ctx, plan)
		if err != nil {
			return nil, err
		}
		if c.resumedPlans == nil {
			c.resumedPlans = map[string]bool{}
		}
		c.resumedPlans[q.ID] = true
	}
	c.history = cloneHistory(old.History)
	c.history = append(c.history, cloneHistory([]HistoricalRun{{ID: old.ID, Root: old.Root, Discovery: old.Discovery, Artifacts: old.Artifacts, Queries: old.Queries}})...)
	c.id = digest("catalog_", c.snapshotLocked())
	return c, nil
}

// IsResumedQuery distinguishes this freshly re-admitted roster from ordinary
// query repetition. It is not input/read/window/causal authority; tools must
// independently recheck host authorization, schema and generation on use.
func (c *Catalog) IsResumedQuery(id string) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resumedPlans[id]
}

func cloneHistory(in []HistoricalRun) []HistoricalRun {
	if len(in) == 0 {
		return nil
	}
	out := make([]HistoricalRun, len(in))
	for i, h := range in {
		out[i] = h
		out[i].Discovery.Issues = append([]Issue(nil), h.Discovery.Issues...)
		out[i].Artifacts = append([]Artifact(nil), h.Artifacts...)
		for j := range out[i].Artifacts {
			out[i].Artifacts[j].Issues = append([]Issue(nil), h.Artifacts[j].Issues...)
		}
		out[i].Queries = make([]QueryRecord, len(h.Queries))
		for j, q := range h.Queries {
			out[i].Queries[j] = cloneQuery(q)
		}
	}
	return out
}
