package index

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// Orphaned-cache reclamation for the repomap cache tree
// (<cache_dir>/repomap/<slug>/).
//
// Each cache directory is a derived artifact of exactly one source
// repo root: CacheDir slugs the absolute, symlink-resolved root path.
// Once that root is gone the directory is dead weight — scans and
// loads only ever target roots that exist on disk, so nothing reads
// or writes a cache dir whose source root is absent. The dominant
// producer is write mode: every write-phase Run swaps RepoRoot to a
// per-Run git worktree under <CWD>/.codrax/worktrees/, repo_map scans
// it into a fresh cache dir, and the L5 red line then unconditionally
// discards the worktree — stranding one cache dir per Run.
//
// Reclamation contract (architecture.md §14.7):
//
//   - PRECISE liveness signal only. A directory is removed when its
//     RECORDED source root (manifest repo_root, falling back to the
//     chunkdir.meta.json sentinel that has carried the root since the
//     resume machinery landed) is an absolute path whose absence is
//     POSITIVELY confirmed (fs.ErrNotExist — an EACCES/EIO/ESTALE
//     probe is "unknowable", never "gone") while the root's PARENT
//     directory still exists. The parent check separates "repo
//     deleted on a live filesystem" from "the whole tree vanished"
//     (unmounted volume, relocated home — keep; it may come back).
//     Directories with no recoverable provenance are never touched.
//
//   - Two age tiers. A dead root that is ephemeral by construction —
//     under codrax's own worktree base (…/.codrax/worktrees: a
//     discarded write-mode worktree) or under the OS temp tree
//     (t.TempDir test repos, /tmp scratch checkouts; the OS reaps
//     those trees wholesale, so the parent-exists requirement is
//     waived for them) — is reclaimed after the short grace window.
//     Any other dead root could also be a repo sitting directly under
//     a persistent mountpoint (umount leaves the empty mountpoint dir
//     behind, so the parent survives while the root "disappears") —
//     those wait out a long TTL of cache inactivity first, bounding
//     the damage of a transient unmount to one full rescan in the
//     worst case while still reclaiming genuinely deleted repos.
//
//   - Multi-instance safe. The age gates read the directory's newest
//     top-level mtime, which active scans keep refreshing (chunk and
//     manifest writes — same precedent as chunkDirPruneGraceWindow),
//     and the root is re-checked immediately before removal to narrow
//     the race against a root recreated mid-sweep. A process that
//     already loaded the cache into memory is unaffected either way:
//     the next save recreates the directory via MkdirAll.
//
// Known bounded imprecision: caches written before the manifest
// repo_root field existed are existence-checked on the sentinel's
// raw (uncanonicalised) root. If that recorded form dies while the
// resolved form lives (a removed symlink alias), the cache is reaped
// even though its true source survives — the cost is one rescan, and
// the durable-tier TTL makes the window small in practice.

// cacheDirPruneGraceWindow shields a cache directory from the orphan
// sweep for a while after its last write, mirroring the chunk-dir
// grace window: a directory being written right now has a fresh mtime
// somewhere in its top level; a genuine orphan stops changing and
// ages past the window.
const cacheDirPruneGraceWindow = chunkDirPruneGraceWindow

// cacheDirOrphanTTL is the inactivity age a dead-root cache dir must
// reach before the sweep reclaims it when the root is NOT a discarded
// codrax worktree. Mirrors the preserved-worktree keep TTL (7 days):
// long enough that a transiently unmounted volume almost always comes
// back first, short enough that genuinely deleted repos do not pin
// cache space forever.
const cacheDirOrphanTTL = 168 * time.Hour

// codraxWorktreeBaseSuffix identifies codrax's own per-Run write-mode
// worktree base: cmd/root.go anchors it at <CWD>/.codrax/worktrees
// (fixed layout, architecture.md §14.6). A dead root directly under
// it is a discarded worktree by construction — the one producer this
// sweep exists for.
var codraxWorktreeBaseSuffix = string(filepath.Separator) + ".codrax" + string(filepath.Separator) + "worktrees"

// systemTempPrefixes are path prefixes identifying the OS temp tree.
// Roots under them are ephemeral by OS contract — nothing durable
// lives there, the OS reaps the trees wholesale (so a missing parent
// is the normal end state, not an unmount), and no volume is ever
// mounted there. Beyond the current process's TempDir (raw and
// symlink-resolved — macOS $TMPDIR is per-user per-boot, so old
// caches may record other boots' paths), the well-known fixed Unix
// locations are listed verbatim; they simply never match on Windows.
// Package var so tests can neutralise the classification when they
// need a t.TempDir root treated as durable.
var systemTempPrefixes = defaultSystemTempPrefixes()

func defaultSystemTempPrefixes() []string {
	seen := make(map[string]bool)
	var out []string
	add := func(p string) {
		p = strings.TrimRight(p, string(filepath.Separator))
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	add(os.TempDir())
	add(canonicalCacheRepoRoot(os.TempDir()))
	add("/tmp")
	add("/private/tmp")
	add("/var/folders")
	add("/private/var/folders")
	return out
}

func underSystemTempTree(root string) bool {
	for _, prefix := range systemTempPrefixes {
		if strings.HasPrefix(root, prefix+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// PruneOrphanedCacheDirs sweeps the repomap cache tree and removes
// directories whose recorded source repo root no longer exists.
// Best-effort: every failure or unknowable probe skips the directory
// rather than erroring. Called once at startup (cmd/root.go), after
// SetCacheDir has applied the configured cache_dir so the sweep sees
// the same tree CacheDir writes to. Returns the number of directories
// removed.
func PruneOrphanedCacheDirs() int {
	return pruneOrphanedCacheDirsIn(cacheRepomapRoot())
}

func pruneOrphanedCacheDirsIn(base string) int {
	entries, err := os.ReadDir(base)
	if err != nil {
		return 0 // missing base = nothing to prune
	}
	removed := 0
	now := time.Now()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(base, e.Name())
		root := recordedCacheSourceRoot(dir)
		if root == "" || !filepath.IsAbs(root) {
			// Unknown or untrustworthy provenance (pre-provenance
			// cache, foreign directory, relative root from an old
			// writer) — never touch.
			continue
		}
		gone, knowable := cachePathAbsent(root)
		if !knowable || !gone {
			continue // live source, or probe error — keep either way
		}
		// Tier classification: ephemeral-by-construction roots reclaim
		// after the short grace window; anything else must also sit
		// cache-inactive for the long TTL, so a repo directly under a
		// persistent mountpoint survives a transient unmount.
		ephemeral := false
		requireParent := true
		switch {
		case underSystemTempTree(root):
			// The OS reaps temp trees wholesale: a missing parent is
			// the normal end state there, so the parent check would
			// wrongly shield these forever.
			ephemeral = true
			requireParent = false
		case strings.HasSuffix(filepath.Dir(root), codraxWorktreeBaseSuffix):
			ephemeral = true
		}
		if requireParent {
			parentGone, parentKnowable := cachePathAbsent(filepath.Dir(root))
			if !parentKnowable || parentGone {
				// The parent tree is gone too (unmounted volume,
				// relocated home — not a deleted repo) or unprobeable.
				// Keep — pruning would throw away a cache that becomes
				// reachable again when the tree returns.
				continue
			}
		}
		minAge := cacheDirOrphanTTL
		if ephemeral {
			minAge = cacheDirPruneGraceWindow
		}
		if newestCacheDirMTime(dir).After(now.Add(-minAge)) {
			continue // too recent — a concurrent process may own it
		}
		// Re-check the root immediately before removal to narrow the
		// window against a root recreated since the check above (a
		// fresh scan into a recreated root writes into this same slug).
		if gone, knowable := cachePathAbsent(root); !knowable || !gone {
			continue
		}
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			logging.Warning("repomap: prune orphaned cache %s: %v", dir, rmErr)
			continue
		}
		logging.Debug("repomap: pruned orphaned cache %s (source root %s no longer exists)", dir, root)
		removed++
	}
	if removed > 0 {
		logging.Info("repomap: pruned %d orphaned cache dir(s) under %s", removed, base)
	}
	return removed
}

// recordedCacheSourceRoot recovers the source repo root a cache
// directory was built from. Preference order: the installed
// manifest's repo_root; the chunkdir.meta.json sentinel of the
// manifest's live chunk dir (manifests written before repo_root
// existed); the newest orphan chunk dir's sentinel (scan interrupted
// before a manifest was installed). Returns "" when no provenance is
// recoverable.
func recordedCacheSourceRoot(dir string) string {
	if raw, err := os.ReadFile(filepath.Join(dir, cacheFileInfosManifestFile)); err == nil {
		var manifest cacheFileInfosManifest
		if json.Unmarshal(raw, &manifest) == nil {
			if manifest.RepoRoot != "" {
				return manifest.RepoRoot
			}
			if manifest.ChunkDir != "" {
				if root := chunkDirSentinelRoot(filepath.Join(dir, manifest.ChunkDir)); root != "" {
					return root
				}
			}
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var newestRoot string
	var newestMod time.Time
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "fileinfos.") || !strings.HasSuffix(e.Name(), ".d") {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		if newestRoot != "" && !info.ModTime().After(newestMod) {
			continue
		}
		if root := chunkDirSentinelRoot(filepath.Join(dir, e.Name())); root != "" {
			newestRoot = root
			newestMod = info.ModTime()
		}
	}
	return newestRoot
}

func chunkDirSentinelRoot(chunkDir string) string {
	raw, err := os.ReadFile(filepath.Join(chunkDir, cacheChunkDirMetaFile))
	if err != nil {
		return ""
	}
	var meta chunkDirMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return ""
	}
	return meta.RepoRoot
}

// newestCacheDirMTime is the freshest mtime across the cache dir
// itself and its immediate children. Every write path refreshes
// something at this level: manifest installs and sidecar renames bump
// the dir, streaming chunk writes bump their chunk dir entry.
func newestCacheDirMTime(dir string) time.Time {
	var newest time.Time
	if info, err := os.Stat(dir); err == nil {
		newest = info.ModTime()
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return newest
	}
	for _, e := range entries {
		if info, ierr := e.Info(); ierr == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest
}

// cachePathAbsent probes whether a path entry is positively absent.
// gone is meaningful only when knowable is true: absence must be
// confirmed by fs.ErrNotExist — an EACCES, EIO or ESTALE probe says
// nothing about the path's existence and must never feed a hard
// delete (precise-signals red line). Lstat (not Stat) so a dangling
// symlink still counts as present — the conservative reading for a
// liveness signal.
func cachePathAbsent(p string) (gone, knowable bool) {
	_, err := os.Lstat(p)
	switch {
	case err == nil:
		return false, true
	case errors.Is(err, fs.ErrNotExist):
		return true, true
	default:
		return false, false
	}
}
