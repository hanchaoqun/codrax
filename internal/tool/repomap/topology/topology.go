// Package topology models a parent directory's set of git sub-repos
// and answers "which sub-repo owns this relative path?" lookups for
// the multi-repo runtime (see docs/design/multi_repo_discovery_and_lazy_load.md).
//
// Discovery (Discover) is performed once per process — the result is
// a *RepoTopology snapshot consumed by the multigraph routing layer
// and the REPL `/repos` command. The snapshot is also persisted under
// <runtime-anchor>/cache/topology/<parent-slug>.json so subsequent
// runs in the same parent directory can short-circuit the BFS walk
// when the parent's special-files fingerprint hasn't changed.
//
// SubRepo.Slug is byte-identical to the slug index.CacheDir mints for
// the same RootAbs (single canonical "slug source", design §9 #7).
package topology

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SubRepo describes one git-rooted sub-tree under the parent root.
type SubRepo struct {
	// Slug = index.CacheDirSlug(RootAbs). Stable across runs.
	Slug string `json:"slug"`

	// RootAbs is the absolute, EvalSymlinks-normalised path of the
	// sub-repo root.
	RootAbs string `json:"root_abs"`

	// RootRel is the slash-separated path of the sub-repo root
	// relative to the parent root. The degenerate single-repo case
	// (parent itself is a git repo) uses RootRel = ".".
	RootRel string `json:"root_rel"`

	// GitMode reflects the form of `.git` at RootAbs:
	//   "dir"  → independent repo (.git is a directory)
	//   "file" → submodule / git-worktree pointer file
	//   ""     → not present (legacy / synthetic case)
	GitMode string `json:"git_mode"`

	// PrimaryLangs is the top-3 languages by file count. Tier 1 is
	// derived from `git ls-files` extension counts plus
	// special-file overrides; Tier 2 is calibrated from the live
	// graph metadata on first EnsureLoaded.
	PrimaryLangs     []string `json:"primary_langs"`
	PrimaryLangsTier int      `json:"primary_langs_tier"`

	// ManifestFingerprint hashes the size+mtime of well-known
	// manifest files (go.mod, package.json, Cargo.toml, ...). When
	// the fingerprint changes the topology cache is invalidated.
	ManifestFingerprint string `json:"manifest_fp,omitempty"`

	// FileCount is the number of files reported by `git ls-files`
	// at discovery time. Used by the active-set "biggest first"
	// fallback (routing channel E) and the min-files filter.
	FileCount int `json:"file_count,omitempty"`
}

// RepoTopology is the snapshot Discover produces and persists.
type RepoTopology struct {
	ParentRoot   string    `json:"parent_root"`
	ParentSlug   string    `json:"parent_slug"`
	Repos        []SubRepo `json:"repos"`
	DiscoveredAt time.Time `json:"discovered_at"`
}

// IsSingle reports whether this topology represents the degenerate
// single-repo case (parent itself is a git repo, exactly one SubRepo
// whose RootRel == "."). Used by callers that fast-path single-repo
// behaviour to byte-identical pre-multi-repo semantics.
func (t *RepoTopology) IsSingle() bool {
	return t != nil && len(t.Repos) == 1 && t.Repos[0].RootRel == "."
}

// Lookup returns the SubRepo that owns relPathFromParent, or nil if
// the path falls outside every discovered sub-repo. Selection is by
// longest matching RootRel prefix, with the parent-root catch-all
// (RootRel == ".") used only when no other entry matches.
//
// relPathFromParent must be slash-form ("a/b/c.go") and must NOT have
// a leading "/". An empty path resolves to the parent-root SubRepo
// when present.
func (t *RepoTopology) Lookup(relPathFromParent string) *SubRepo {
	if t == nil {
		return nil
	}
	rel := filepath.ToSlash(strings.TrimPrefix(relPathFromParent, "./"))
	rel = strings.TrimPrefix(rel, "/")

	if rel == "" || rel == "." {
		for i := range t.Repos {
			if t.Repos[i].RootRel == "." {
				return &t.Repos[i]
			}
		}
		return nil
	}

	var (
		best        *SubRepo
		bestLen     = -1
		parentSelf  *SubRepo
	)
	for i := range t.Repos {
		rr := t.Repos[i].RootRel
		if rr == "." {
			parentSelf = &t.Repos[i]
			continue
		}
		if rel == rr || strings.HasPrefix(rel, rr+"/") {
			if len(rr) > bestLen {
				best = &t.Repos[i]
				bestLen = len(rr)
			}
		}
	}
	if best != nil {
		return best
	}
	return parentSelf
}

// SubRepoBySlug returns the SubRepo with matching Slug, or nil.
func (t *RepoTopology) SubRepoBySlug(slug string) *SubRepo {
	if t == nil {
		return nil
	}
	for i := range t.Repos {
		if t.Repos[i].Slug == slug {
			return &t.Repos[i]
		}
	}
	return nil
}

// CacheFilePath returns where this topology snapshot is persisted on
// disk under the given runtime anchor. Returns "" if runtimeAnchor or
// ParentSlug are empty.
func (t *RepoTopology) CacheFilePath(runtimeAnchor string) string {
	if runtimeAnchor == "" || t == nil || t.ParentSlug == "" {
		return ""
	}
	return filepath.Join(runtimeAnchor, "cache", "topology", t.ParentSlug+".json")
}

// Save atomically persists t under <runtimeAnchor>/cache/topology/<parent-slug>.json.
// The parent directory tree is created on demand.
func (t *RepoTopology) Save(runtimeAnchor string) error {
	if t == nil {
		return fmt.Errorf("topology.Save: nil topology")
	}
	path := t.CacheFilePath(runtimeAnchor)
	if path == "" {
		return fmt.Errorf("topology.Save: empty runtime anchor or parent slug")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("topology.Save: mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("topology.Save: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("topology.Save: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("topology.Save: rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// Load reads a previously persisted RepoTopology. Returns (nil, nil)
// when the cache file does not exist (cold start).
func Load(runtimeAnchor, parentSlug string) (*RepoTopology, error) {
	if runtimeAnchor == "" || parentSlug == "" {
		return nil, nil
	}
	path := filepath.Join(runtimeAnchor, "cache", "topology", parentSlug+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("topology.Load: read %s: %w", path, err)
	}
	var t RepoTopology
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("topology.Load: unmarshal %s: %w", path, err)
	}
	// Defensive: keep Repos sorted on load so callers may rely on
	// stable iteration order even after concurrent re-saves.
	sort.SliceStable(t.Repos, func(i, j int) bool {
		return t.Repos[i].RootRel < t.Repos[j].RootRel
	})
	return &t, nil
}
