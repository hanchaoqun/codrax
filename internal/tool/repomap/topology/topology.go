// Package topology models a parent directory's set of git sub-repos
// and answers "which sub-repo owns this relative path?" lookups for
// the multi-repo runtime (see docs/design/multi_repo_discovery_and_lazy_load.md).
//
// Discovery (Discover) is performed once per process — the result is
// a *RepoTopology snapshot consumed by the multigraph routing layer
// and the REPL `/repos` command. The snapshot is also persisted under
// <runtime-anchor>/cache/topology/<parent-slug>.json so subsequent
// runs in the same parent directory can short-circuit the BFS walk
// when the cached sub-repo roots still exist and their recognised
// manifest fingerprints have not changed.
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
		best       *SubRepo
		bestLen    = -1
		parentSelf *SubRepo
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

// SubRepoByRootRel returns the SubRepo with matching RootRel, or nil.
// Accepts either slash- or backslash-separated input (Windows-paste
// compat) and tolerates a leading "./" or trailing "/" — both round-
// trip to the same canonical RootRel that Discover persists.
func (t *RepoTopology) SubRepoByRootRel(rootRel string) *SubRepo {
	if t == nil {
		return nil
	}
	want := strings.TrimSuffix(strings.TrimPrefix(strings.ReplaceAll(rootRel, "\\", "/"), "./"), "/")
	if want == "" {
		// Empty after trim is meaningless; the parent-itself entry
		// canonicalises to "." (handled by direct equality below).
		return nil
	}
	for i := range t.Repos {
		if t.Repos[i].RootRel == want {
			return &t.Repos[i]
		}
	}
	return nil
}

// Resolve looks up a SubRepo by Slug first, then by RootRel — used by
// surfaces (REPL `/repos focus`) where the user can paste either
// identifier. Returns nil only when neither lookup matches.
func (t *RepoTopology) Resolve(token string) *SubRepo {
	if t == nil {
		return nil
	}
	if sr := t.SubRepoBySlug(token); sr != nil {
		return sr
	}
	return t.SubRepoByRootRel(token)
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
	pruneVanishedTopologyCaches(filepath.Dir(path), filepath.Base(path))
	return nil
}

// topologyCachePruneGrace mirrors the fileinfo chunk-dir grace window:
// entries younger than this are never pruned, so a topology cached for
// a root that is mid-recreation (or a racing process that just wrote
// it) is left alone. The prune signal itself is precise — the cached
// parent_root no longer exists on disk.
const topologyCachePruneGrace = 10 * time.Minute

// pruneVanishedTopologyCaches removes sibling cache entries whose
// recorded parent_root has ceased to exist (ephemeral eval scratches,
// deleted worktrees, removed checkouts). Without this the topology
// cache directory grows by one JSON per distinct parent path forever.
// Runs after a successful Save — at most once per discovery, so the
// O(entries) sibling scan is paid on a cold path. Unreadable entries
// past the grace window are treated as vanished (corrupt files have
// no other reclamation lane).
func pruneVanishedTopologyCaches(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-topologyCachePruneGrace)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == keep || !strings.HasSuffix(name, ".json") {
			continue
		}
		info, ierr := entry.Info()
		if ierr != nil || info.ModTime().After(cutoff) {
			continue
		}
		full := filepath.Join(dir, name)
		data, rerr := os.ReadFile(full)
		if rerr != nil {
			continue
		}
		var probe struct {
			ParentRoot string `json:"parent_root"`
		}
		vanished := false
		if jerr := json.Unmarshal(data, &probe); jerr != nil || strings.TrimSpace(probe.ParentRoot) == "" {
			vanished = true // corrupt or rootless entry past grace
		} else if _, serr := os.Stat(probe.ParentRoot); os.IsNotExist(serr) {
			vanished = true
		}
		if vanished {
			_ = os.Remove(full)
		}
	}
}

// LoadOrDiscover prefers a cached RepoTopology if present and fresh,
// otherwise runs Discover and persists the result. Cache freshness
// (design §3.3.3):
//   - parentAbs still exists
//   - every cached SubRepo.RootAbs still exists
//   - every cached SubRepo manifest fingerprint still matches
//
// It intentionally does NOT use parentAbs directory mtime. In the
// common <cwd>/.codrax layout, starting codrax writes logs / blob /
// cache artifacts under the same parent, which bumps the parent mtime
// without changing the sub-repo topology. Treating that as stale made
// warm REPL starts pay the cold BFS + git-ls-files probe repeatedly.
//
// On cache miss / staleness the fresh topology is saved best-effort
// (save failure is non-fatal — the in-memory result is returned).
func LoadOrDiscover(parentAbs, runtimeAnchor string, opts Options, parentSlug string) (*RepoTopology, error) {
	if cached, err := Load(runtimeAnchor, parentSlug); err == nil && cached != nil {
		if isCacheFresh(cached, parentAbs) {
			return cached, nil
		}
	}
	// When the runtime anchor lives under parentAbs (the normal
	// <cwd>/.codrax case), creating the cache directory after discovery
	// would bump parentAbs mtime past DiscoveredAt and make the just-saved
	// cache look stale on the very next call. Prepare it first; save
	// remains best-effort below.
	if runtimeAnchor != "" {
		_ = os.MkdirAll(filepath.Join(runtimeAnchor, "cache", "topology"), 0o755)
	}
	fresh, err := Discover(parentAbs, opts)
	if err != nil {
		return nil, err
	}
	_ = fresh.Save(runtimeAnchor) // best-effort
	return fresh, nil
}

// isCacheFresh implements the §3.3.3 freshness predicate.
func isCacheFresh(t *RepoTopology, parentAbs string) bool {
	if t == nil || t.ParentRoot != parentAbs {
		return false
	}
	if _, err := os.Stat(parentAbs); err != nil {
		return false
	}
	if len(t.Repos) == 0 {
		return false
	}
	for _, sr := range t.Repos {
		if sr.RootAbs == "" {
			return false
		}
		if _, err := os.Stat(sr.RootAbs); err != nil {
			return false
		}
		if got := manifestFingerprint(sr.RootAbs); got != sr.ManifestFingerprint {
			return false
		}
	}
	return true
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
