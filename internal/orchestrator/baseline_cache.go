package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// BaselineCache reuses a previously-captured pre-apply test report
// across Runs that observe the same main-repo HEAD SHA. Capturing a
// baseline doubles the test-suite wall-time, so for repos where the
// user runs many plan/apply cycles between commits the cache turns
// the second-and-later captures into a fast disk read.
//
// Cache layout:
//
//   <cacheDir>/baselines/<sha-prefix>.json
//
// where <sha-prefix> is the first 16 hex chars of the main-repo HEAD
// SHA. (Why 16 not 8: GitHub-shape collision for 8 hex is plausible
// in a long-lived repo across hundreds of commits; 16 hex is
// effectively collision-free for any single user.)
//
// Persistence is JSON (existing types.WriteChangeReportToFile +
// LoadChangeReportFromFile equivalent — the report shape is already
// JSON-stable). LRU eviction caps disk usage; entry count above
// MaxEntries keeps the most-recent N by mtime.
//
// The cache is intentionally per-runtime-anchor (lives at the same
// place log_dir / memory_dir live), so multi-repo use stays
// isolated. SHA collision across repos is impossible because each
// repo writes to its own anchor.
//
// Concurrency: the cache is not multi-process safe today (two codrax
// instances running on the same anchor against the same SHA could
// race the LRU rebuild). The same is true for log retention and
// memory compaction — the multi-instance lock at MEMORY.md provides
// the umbrella; cache rebuild collisions are tolerable because they
// regenerate the same baseline either way.
type BaselineCache struct {
	dir        string // <runtimeAnchor>/baselines
	maxEntries int
}

// NewBaselineCache constructs the cache rooted at runtimeAnchor.
// maxEntries=0 returns a no-op cache (Lookup always misses, Store is
// a no-op, OnHit/OnMiss are bypassed).
func NewBaselineCache(runtimeAnchor string, maxEntries int) *BaselineCache {
	if runtimeAnchor == "" || maxEntries == 0 {
		return &BaselineCache{maxEntries: 0}
	}
	return &BaselineCache{
		dir:        filepath.Join(runtimeAnchor, "baselines"),
		maxEntries: maxEntries,
	}
}

// Enabled reports whether the cache will actually store / lookup.
func (c *BaselineCache) Enabled() bool {
	return c != nil && c.maxEntries > 0 && c.dir != ""
}

// Lookup returns the cached report for sha, or nil when the cache
// is disabled, the SHA is empty, or no entry exists.
func (c *BaselineCache) Lookup(sha string) *types.ChangeReport {
	if !c.Enabled() {
		return nil
	}
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return nil
	}
	path := c.path(sha)
	data, err := os.ReadFile(path)
	if err != nil {
		// ENOENT is the common path; only log on other errors.
		if !os.IsNotExist(err) {
			logging.Warning("[baseline-cache] read %s: %v", path, err)
		}
		return nil
	}
	var report types.ChangeReport
	if err := json.Unmarshal(data, &report); err != nil {
		logging.Warning("[baseline-cache] unmarshal %s: %v (cache entry invalidated)", path, err)
		_ = os.Remove(path)
		return nil
	}
	// Touch mtime so the LRU eviction sees recency. Best-effort.
	_ = os.Chtimes(path, time.Now(), time.Now())
	logging.Info("[baseline-cache] hit sha=%s tests=%d", sha[:8], len(report.TestResults))
	return &report
}

// Store writes the report to the cache for sha. Failure is logged
// and swallowed — the in-memory baseline still works for the rest
// of the Run.
//
// Cross-process concurrency (commit 11 #2): Store re-checks the
// cache file just before writing, so when proc A and proc B both
// captured baselines for the same SHA in parallel, the second
// writer skips the rename instead of clobbering. Doesn't avoid
// the duplicate test-suite cost (both procs already paid), but
// avoids the ABBA failure where two stores interleave a partial
// .tmp file. atomic rename + idempotent os.Remove handle the
// remaining races (eviction concurrency is self-correcting:
// double-delete returns ENOENT, no corruption).
//
// We deliberately don't take a full flock around the cache dir.
// The hot path is "Lookup misses, runs tests for ~minutes, then
// Stores" — holding a lock for that duration would serialise
// otherwise-independent Runs across multiple terminals. The
// mtime guard below covers the realistic collision (rare,
// minutes-scale) without that cost.
func (c *BaselineCache) Store(sha string, report *types.ChangeReport) {
	if !c.Enabled() || report == nil {
		return
	}
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return
	}
	path := c.path(sha)
	// Skip the write when another process just stored (mtime
	// within the last 60 s). 60 s is a fudge that covers test
	// suites of ~that length running in parallel; smaller
	// windows risk false-positives on systems with imprecise
	// fs mtime, larger windows risk holding stale data when
	// tests change quickly.
	if existing, err := os.Stat(path); err == nil {
		if time.Since(existing.ModTime()) < 60*time.Second {
			logging.Info("[baseline-cache] skipping store for sha=%s (peer wrote %.0fs ago)",
				sha[:8], time.Since(existing.ModTime()).Seconds())
			return
		}
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		logging.Warning("[baseline-cache] mkdir %s: %v", c.dir, err)
		return
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		logging.Warning("[baseline-cache] marshal: %v", err)
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		logging.Warning("[baseline-cache] write %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		logging.Warning("[baseline-cache] rename %s: %v", tmp, err)
		_ = os.Remove(tmp)
		return
	}
	logging.Info("[baseline-cache] stored sha=%s tests=%d path=%s", sha[:8], len(report.TestResults), path)
	c.evictIfOverCap()
}

// path returns the on-disk file path for sha.
func (c *BaselineCache) path(sha string) string {
	prefix := sha
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}
	if len(prefix) < 8 {
		// Highly improbable input shape — pad via hash so the
		// filename remains plausible. Defensive only.
		h := sha256.Sum256([]byte(sha))
		prefix = hex.EncodeToString(h[:8])
	}
	return filepath.Join(c.dir, prefix+".json")
}

// evictIfOverCap walks the cache dir, sorts by mtime, and removes
// the oldest entries until the survivor count is at or below
// maxEntries. Best-effort — failure to walk leaves the cache as-is.
func (c *BaselineCache) evictIfOverCap() {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type fileWithMtime struct {
		path  string
		mtime time.Time
	}
	files := make([]fileWithMtime, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileWithMtime{
			path:  filepath.Join(c.dir, e.Name()),
			mtime: info.ModTime(),
		})
	}
	if len(files) <= c.maxEntries {
		return
	}
	// Oldest first.
	sort.Slice(files, func(i, j int) bool {
		return files[i].mtime.Before(files[j].mtime)
	})
	dropCount := len(files) - c.maxEntries
	for i := 0; i < dropCount; i++ {
		if err := os.Remove(files[i].path); err != nil {
			logging.Warning("[baseline-cache] evict %s: %v", files[i].path, err)
			continue
		}
		logging.Debug("[baseline-cache] evicted %s (over cap %d)", filepath.Base(files[i].path), c.maxEntries)
	}
}

// gitHeadSHA reads the main-repo HEAD SHA via `git rev-parse HEAD`.
// Returns ("", err) on any failure (not a git repo, no HEAD, git
// binary missing). Callers degrade gracefully when err is non-nil:
// no SHA means no cache lookup, capture proceeds as before.
//
// IMPORTANT: this reads the MAIN repo HEAD, not the worktree HEAD.
// The worktree may be ahead of main if a previous apply already
// committed. Cache key invariant: same main-repo HEAD = same
// pre-apply baseline test outcome.
func gitHeadSHA(mainRepoRoot string) (string, error) {
	if mainRepoRoot == "" {
		return "", fmt.Errorf("empty repo root")
	}
	cmd := exec.Command("git", "-C", mainRepoRoot, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "", fmt.Errorf("empty git rev-parse output")
	}
	return sha, nil
}
