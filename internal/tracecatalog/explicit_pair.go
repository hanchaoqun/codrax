package tracecatalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// DiscoverIndependentPair freezes exactly two paths after the caller has
// validated an independent comparison operation. It never walks their parent
// directories. Missing/unreadable members remain explicit failed candidates.
func DiscoverIndependentPair(ctx context.Context, paths [2]string) (*Catalog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var infos [2]os.FileInfo
	var origins [2]string
	for i, path := range paths {
		abs, err := filepath.Abs(path)
		if err != nil || path == "" {
			return nil, fmt.Errorf("comparison path is not explicit: %q", path)
		}
		origins[i] = filepath.Clean(abs)
		if canonical, err := filepath.EvalSymlinks(abs); err == nil {
			abs = canonical
		}
		paths[i] = filepath.Clean(abs)
		infos[i], _ = os.Stat(paths[i])
		if infos[i] != nil && !infos[i].Mode().IsRegular() {
			return nil, fmt.Errorf("comparison input is not a regular file: %q", path)
		}
	}
	if paths[0] == paths[1] || infos[0] != nil && infos[1] != nil && os.SameFile(infos[0], infos[1]) {
		return nil, fmt.Errorf("comparison roles resolve to the same physical input; independent preparation is not established")
	}
	root := filepath.Dir(paths[0])
	for {
		rel, err := filepath.Rel(root, paths[1])
		if err == nil && rel != ".." && len(rel) > 0 && !(len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator)) {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			return nil, fmt.Errorf("comparison paths have no shared filesystem root")
		}
		root = parent
	}
	c := &Catalog{run: &runIdentity{}, root: root, discovery: Discovery{Complete: true, FilterLabel: "explicit independent comparison inputs"}, artifacts: map[string]Artifact{}, generations: map[string]filegeneration.Identity{}, bindings: map[string]sourceBinding{}, queries: map[string]QueryRecord{}, independentInputs: true}
	c.independentOrigins = map[string]string{}
	w := discoveryWalker{ctx: ctx, catalog: c, opts: DiscoverOptions{MaxArtifacts: 2}}
	for i, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rel, _ := filepath.Rel(root, path)
		c.discovery.EntriesVisited++
		if infos[i] == nil {
			id := digest("artifact_", path)
			c.artifacts[id] = Artifact{ID: id, Path: path, RelativePath: filepath.ToSlash(rel), Status: "unavailable", Issues: []Issue{{Code: "candidate_unavailable", Path: path, Message: "The explicitly named input is unavailable; preparation has not succeeded."}}}
			continue
		}
		w.file(path, rel, infos[i])
	}
	for i, path := range paths {
		if artifact, ok := c.ArtifactForPath(path); ok {
			c.independentOrigins[origins[i]] = artifact.ID
		}
	}
	s := c.snapshotLocked()
	c.id = digest("catalog_", struct {
		Paths     [2]string
		Origins   [2]string
		Artifacts []Artifact
	}{paths, origins, s.Artifacts})
	return c, nil
}

// IndependentArtifactForOriginal resolves only the original input spelling
// captured by this live catalog. It is navigation, not a query/read receipt.
// Retargeting an alias cannot move an old role to a different physical source.
func (c *Catalog) IndependentArtifactForOriginal(ctx context.Context, path string) (Artifact, bool, error) {
	if c == nil {
		return Artifact{}, false, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Artifact{}, false, err
	}
	c.mu.Lock()
	id, bound := c.independentOrigins[filepath.Clean(abs)]
	a := c.artifacts[id]
	bound = bound && c.independentInputs && len(c.artifacts) == 2
	c.mu.Unlock()
	if !bound {
		return Artifact{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return a, true, err
	}
	actual, aliasErr := filepath.EvalSymlinks(abs)
	if aliasErr != nil && a.Status != "unavailable" || aliasErr == nil && filepath.Clean(actual) != a.Path {
		c.markStale(id, ErrStale)
		a.Status = "stale"
		return a, true, ErrStale
	}
	a, err = c.Resolve(ctx, id)
	return a, true, err
}

func (c *Catalog) IndependentInput(artifactID string) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, exists := c.artifacts[artifactID]
	return c.independentInputs && exists && len(c.artifacts) == 2
}

// RecordPreparationFailure is navigation, not a fictitious query attempt or
// measurement. Source generation validation remains required on every read.
func (c *Catalog) RecordPreparationFailure(path string, cause error) {
	defer c.trackDeliveryBranch()
	if c == nil || cause == nil {
		return
	}
	if canonical, err := filepath.EvalSymlinks(path); err == nil {
		path = canonical
	}
	a, ok := c.ArtifactForPath(path)
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	a = c.artifacts[a.ID]
	if a.Status == "candidate" {
		a.Status = "preparation_failed"
	}
	a.Issues = append(a.Issues, Issue{Code: "preparation_failed", Path: path, Message: boundedMessage(cause.Error())})
	c.artifacts[a.ID] = a
}
