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
	for i, path := range paths {
		abs, err := filepath.Abs(path)
		if err != nil || path == "" {
			return nil, fmt.Errorf("comparison path is not explicit: %q", path)
		}
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
	s := c.snapshotLocked()
	c.id = digest("catalog_", struct {
		Paths     [2]string
		Artifacts []Artifact
	}{paths, s.Artifacts})
	return c, nil
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
