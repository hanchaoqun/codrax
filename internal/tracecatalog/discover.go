package tracecatalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

const (
	DefaultMaxArtifacts = 4096
	DefaultMaxEntries   = 100000
	DefaultMaxDepth     = 64
	MaxDiscoveryIssues  = 128
)

type DiscoverOptions struct {
	NonRecursive bool
	MaxArtifacts int
	MaxEntries   int
	MaxDepth     int
	// Filter is an optional candidate-navigation filter, not format admission.
	// Nil includes every regular file, including extensionless binary inputs.
	Filter      func(relativePath string) bool
	FilterLabel string
}

// Discover must be called only after the caller has authorized this exact
// root. It does not infer authority from a filename, repo root, prior catalog,
// model prose, or a query result. Root may be a directory or one explicit file.
// On cancellation it returns the partial catalog AND the cancellation error.
func Discover(ctx context.Context, authorizedRoot string, opts DiscoverOptions) (*Catalog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if authorizedRoot == "" {
		return nil, fmt.Errorf("an explicitly authorized root is required")
	}
	if opts.Filter != nil && opts.FilterLabel == "" {
		return nil, fmt.Errorf("candidate filters require a display label")
	}
	if opts.MaxArtifacts < 0 || opts.MaxEntries < 0 || opts.MaxDepth < 0 {
		return nil, fmt.Errorf("discovery limits must be nonnegative")
	}
	if opts.MaxArtifacts == 0 {
		opts.MaxArtifacts = DefaultMaxArtifacts
	}
	if opts.MaxEntries == 0 {
		opts.MaxEntries = DefaultMaxEntries
	}
	if opts.MaxDepth == 0 {
		opts.MaxDepth = DefaultMaxDepth
	}
	if opts.MaxArtifacts > DefaultMaxArtifacts || opts.MaxEntries > DefaultMaxEntries || opts.MaxDepth > DefaultMaxDepth {
		return nil, fmt.Errorf("discovery limits exceed supported bounds")
	}
	abs, err := filepath.Abs(authorizedRoot)
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("authorized root must be a regular file or directory")
	}
	c := &Catalog{run: &runIdentity{}, root: root,
		discovery: Discovery{Recursive: !opts.NonRecursive, FilterLabel: opts.FilterLabel, Complete: true},
		artifacts: map[string]Artifact{}, generations: map[string]filegeneration.Identity{},
		bindings: map[string]sourceBinding{}, queries: map[string]QueryRecord{}}
	w := discoveryWalker{ctx: ctx, catalog: c, opts: opts}
	if info.IsDir() {
		r, openErr := os.OpenRoot(root)
		if openErr != nil {
			return nil, openErr
		}
		w.walk(r, ".", 0)
		if closeErr := r.Close(); closeErr != nil {
			w.issue("directory_close_failed", root, closeErr)
		}
	} else {
		c.discovery.EntriesVisited++
		w.file(root, filepath.Base(root), info)
	}
	if ctx.Err() != nil {
		c.discovery.Canceled = true
		c.discovery.Complete = false
	}
	// The key depends on the frozen candidate revisions, not completion order or
	// a wall-clock timestamp. Same basename/stem never collapses two entries.
	s := c.snapshotLocked()
	c.id = digest("catalog_", struct {
		Root      string
		Discovery Discovery
		Artifacts []Artifact
	}{root, s.Discovery, s.Artifacts})
	if ctx.Err() != nil {
		return c, ctx.Err()
	}
	return c, nil
}

type discoveryWalker struct {
	ctx        context.Context
	catalog    *Catalog
	opts       DiscoverOptions
	entryLimit bool
}

func (w *discoveryWalker) issue(code, path string, err error) {
	d := &w.catalog.discovery
	d.Complete = false
	d.IssueCount++
	if len(d.Issues) < MaxDiscoveryIssues {
		message := ""
		if err != nil {
			message = boundedMessage(err.Error())
		}
		d.Issues = append(d.Issues, Issue{Code: code, Path: path, Message: message})
	}
}

func (w *discoveryWalker) limit(code, path string) {
	w.catalog.discovery.Truncated = true
	w.issue(code, path, nil)
}

func (w *discoveryWalker) walk(root *os.Root, relative string, depth int) {
	if w.ctx.Err() != nil || w.entryLimit {
		return
	}
	// OpenRoot confines directory traversal. Opening "." through its held
	// directory handle cannot block on a path swapped to a FIFO or follow an
	// external symlink. Files below are only generation-captured, never decoded.
	dir, err := root.Open(".")
	if err != nil {
		w.issue("directory_read_failed", relative, err)
		return
	}
	before, err := filegeneration.FromFile(dir)
	if err != nil {
		_ = dir.Close()
		w.issue("directory_identity_failed", relative, err)
		return
	}
	defer func() {
		after, statErr := filegeneration.FromFile(dir)
		if statErr != nil || !before.SameVersion(after) {
			w.issue("directory_changed_during_discovery", relative, statErr)
		}
		pathGeneration, pathErr := filegeneration.FromPath(filepath.Join(w.catalog.root, relative))
		if pathErr != nil || !before.SameVersion(pathGeneration) {
			w.issue("directory_path_changed_during_discovery", relative, pathErr)
		}
		if closeErr := dir.Close(); closeErr != nil {
			w.issue("directory_close_failed", relative, closeErr)
		}
	}()
	for {
		if w.ctx.Err() != nil || w.entryLimit {
			return
		}
		entries, readErr := dir.ReadDir(128)
		// A bounded page avoids materializing a huge directory in memory. Stable
		// final ordering and IDs do not depend on directory enumeration order.
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if w.ctx.Err() != nil {
				return
			}
			if w.catalog.discovery.EntriesVisited >= w.opts.MaxEntries {
				w.entryLimit = true
				w.limit("entry_limit", relative)
				return
			}
			w.catalog.discovery.EntriesVisited++
			rel := filepath.Join(relative, entry.Name())
			if entry.Type()&os.ModeSymlink != 0 {
				w.catalog.discovery.SymlinksSkipped++
				continue
			}
			info, infoErr := root.Lstat(entry.Name())
			if infoErr != nil {
				w.issue("entry_stat_failed", rel, infoErr)
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 {
				w.catalog.discovery.SymlinksSkipped++
				continue
			}
			if info.IsDir() {
				if w.opts.NonRecursive {
					w.catalog.discovery.SubdirectoriesSkipped++
					continue
				}
				if depth >= w.opts.MaxDepth {
					w.catalog.discovery.SubdirectoriesSkipped++
					w.limit("depth_limit", rel)
					continue
				}
				child, childErr := root.OpenRoot(entry.Name())
				if childErr != nil {
					w.issue("directory_open_failed", rel, childErr)
					continue
				}
				w.walk(child, rel, depth+1)
				if closeErr := child.Close(); closeErr != nil {
					w.issue("directory_close_failed", rel, closeErr)
				}
				continue
			}
			if !info.Mode().IsRegular() {
				w.catalog.discovery.SpecialFilesSkipped++
				continue
			}
			w.file(filepath.Join(w.catalog.root, rel), rel, info)
		}
		if errors.Is(readErr, io.EOF) {
			return
		}
		if readErr != nil {
			w.issue("directory_read_failed", relative, readErr)
			return
		}
	}
}

func (w *discoveryWalker) file(path, relative string, info os.FileInfo) {
	d := &w.catalog.discovery
	d.RegularFilesSeen++
	if w.opts.Filter != nil && !w.opts.Filter(filepath.ToSlash(relative)) {
		d.FilteredFiles++
		return
	}
	if len(w.catalog.artifacts) >= w.opts.MaxArtifacts {
		if !d.Truncated {
			w.limit("artifact_limit", relative)
		}
		d.Complete = false
		d.Truncated = true
		return
	}
	id := digest("artifact_", path)
	a := Artifact{ID: id, Path: path, RelativePath: filepath.ToSlash(relative), Bytes: info.Size(), Status: "candidate"}
	canonical, err := filepath.EvalSymlinks(path)
	if err == nil && canonical != path {
		err = fmt.Errorf("candidate path changed through a symbolic link")
	}
	var generation filegeneration.Identity
	if err == nil {
		generation, err = filegeneration.FromPath(path)
		initial := filegeneration.FromInfo(info)
		if err == nil && (!generation.Mode().IsRegular() || generation.Size() != info.Size() || generation.ModUnixNano() != info.ModTime().UnixNano() || (initial.Strong() && !generation.SameVersion(initial))) {
			err = fmt.Errorf("candidate changed during discovery")
		}
	}
	if err != nil {
		a.Status = "unavailable"
		a.Issues = []Issue{{Code: "candidate_identity_failed", Path: path, Message: boundedMessage(err.Error())}}
		w.issue("candidate_identity_failed", relative, err)
	} else {
		a.Bytes = generation.Size()
		a.RevisionID = digest("revision_", generation.CacheToken())
		w.catalog.generations[id] = generation
	}
	w.catalog.artifacts[id] = a
}
