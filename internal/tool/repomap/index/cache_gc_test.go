package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// gcTestFileInfos is a minimal one-file payload for building a real
// cache dir through the production writer.
func gcTestFileInfos() []*types.FileInfo {
	return []*types.FileInfo{
		{
			RelPath:  "a.go",
			Language: types.LangGo,
			Package:  "main",
			Hash:     "deadbeef",
			Size:     42,
			Symbols: []types.Symbol{
				{Name: "Foo", Kind: "type", File: "a.go", Line: 1, EndLine: 1},
			},
		},
	}
}

// writeGCCache builds a cache dir under base for repoRoot via the
// production chunked writer (manifest + chunk dir + sentinel).
func writeGCCache(t *testing.T, base, repoRoot string) string {
	t.Helper()
	dir := filepath.Join(base, CacheDirSlug(repoRoot))
	if err := saveFileInfos(dir, repoRoot, gcTestFileInfos()); err != nil {
		t.Fatalf("saveFileInfos: %v", err)
	}
	return dir
}

// asDurableRoots neutralises the system-temp-tree classification for
// the duration of one test. Test roots live under t.TempDir — which IS
// the system temp tree — so exercising the durable tier and the
// worktree-suffix branch requires switching the temp-tree shortcut
// off.
func asDurableRoots(t *testing.T) {
	t.Helper()
	old := systemTempPrefixes
	systemTempPrefixes = nil
	t.Cleanup(func() { systemTempPrefixes = old })
}

// gcWorktreeRoot creates a root directory shaped like a codrax
// write-mode worktree: <tmp>/.codrax/worktrees/<name>. Dead roots
// under this parent take the short ephemeral-tier grace window.
func gcWorktreeRoot(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".codrax", "worktrees", name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// backdate pushes the mtime of a cache dir and every immediate child
// back by age.
func backdate(t *testing.T, dir string, age time.Duration) {
	t.Helper()
	old := time.Now().Add(-age)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	for _, e := range entries {
		if err := os.Chtimes(filepath.Join(dir, e.Name()), old, old); err != nil {
			t.Fatalf("Chtimes: %v", err)
		}
	}
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatalf("Chtimes(dir): %v", err)
	}
}

func TestPruneOrphanedCacheDirs_RemovesDeadWorktreeRoot(t *testing.T) {
	asDurableRoots(t)
	base := t.TempDir()
	repoRoot := gcWorktreeRoot(t, "trace-1718169600123456789-9999")
	dir := writeGCCache(t, base, repoRoot)

	// Worktree discarded: root gone, the worktrees base remains.
	if err := os.RemoveAll(repoRoot); err != nil {
		t.Fatal(err)
	}
	backdate(t, dir, cacheDirPruneGraceWindow+time.Hour)

	if got := pruneOrphanedCacheDirsIn(base); got != 1 {
		t.Fatalf("pruned = %d, want 1", got)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cache dir still exists after prune: %v", err)
	}
}

// TestPruneOrphanedCacheDirs_DurableTierWaitsOutTTL pins the two-tier
// age policy: a dead root that is NOT a codrax worktree (it could be a
// repo directly under a persistent mountpoint whose volume is merely
// unmounted) must sit cache-inactive past cacheDirOrphanTTL before it
// is reclaimed — the short grace window alone is not enough.
func TestPruneOrphanedCacheDirs_DurableTierWaitsOutTTL(t *testing.T) {
	asDurableRoots(t)
	base := t.TempDir()
	rootParent := t.TempDir()
	repoRoot := filepath.Join(rootParent, "myrepo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := writeGCCache(t, base, repoRoot)
	if err := os.RemoveAll(repoRoot); err != nil {
		t.Fatal(err)
	}

	// Older than the grace window but younger than the TTL: kept.
	backdate(t, dir, cacheDirPruneGraceWindow+time.Hour)
	if got := pruneOrphanedCacheDirsIn(base); got != 0 {
		t.Fatalf("pruned = %d, want 0 (durable tier inside TTL)", got)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("durable-tier cache dir removed inside TTL: %v", err)
	}

	// Older than the TTL: reclaimed.
	backdate(t, dir, cacheDirOrphanTTL+time.Hour)
	if got := pruneOrphanedCacheDirsIn(base); got != 1 {
		t.Fatalf("pruned = %d, want 1 (durable tier past TTL)", got)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cache dir still exists after TTL prune: %v", err)
	}
}

func TestPruneOrphanedCacheDirs_KeepsLiveRoot(t *testing.T) {
	base := t.TempDir()
	repoRoot := t.TempDir()
	dir := writeGCCache(t, base, repoRoot)
	backdate(t, dir, cacheDirOrphanTTL+time.Hour)

	if got := pruneOrphanedCacheDirsIn(base); got != 0 {
		t.Fatalf("pruned = %d, want 0", got)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("live-root cache dir was removed: %v", err)
	}
}

func TestPruneOrphanedCacheDirs_GraceWindowShieldsRecentWrites(t *testing.T) {
	asDurableRoots(t)
	base := t.TempDir()
	repoRoot := gcWorktreeRoot(t, "trace-1718169600123456789-8888")
	dir := writeGCCache(t, base, repoRoot)
	if err := os.RemoveAll(repoRoot); err != nil {
		t.Fatal(err)
	}
	// No backdate: mtimes are fresh, as they would be for a cache a
	// concurrent process is writing right now.
	if got := pruneOrphanedCacheDirsIn(base); got != 0 {
		t.Fatalf("pruned = %d, want 0 (grace window)", got)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("recently-written cache dir was removed: %v", err)
	}
}

func TestPruneOrphanedCacheDirs_KeepsWhenParentMissing(t *testing.T) {
	asDurableRoots(t)
	base := t.TempDir()
	rootParent := filepath.Join(t.TempDir(), "parent")
	repoRoot := filepath.Join(rootParent, "repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := writeGCCache(t, base, repoRoot)

	// Whole tree vanishes — the unmounted-volume shape where the
	// mountpoint dir disappears with the mount (macOS /Volumes).
	if err := os.RemoveAll(rootParent); err != nil {
		t.Fatal(err)
	}
	backdate(t, dir, cacheDirOrphanTTL+time.Hour)

	if got := pruneOrphanedCacheDirsIn(base); got != 0 {
		t.Fatalf("pruned = %d, want 0 (parent missing)", got)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("cache dir removed despite missing parent tree: %v", err)
	}
}

// TestPruneOrphanedCacheDirs_SkipsOnUnknowableProbe pins the error
// taxonomy: an Lstat failure that is NOT fs.ErrNotExist (here EACCES
// from a traversal-denied parent) says nothing about the root's
// existence and must never feed the hard delete.
func TestPruneOrphanedCacheDirs_SkipsOnUnknowableProbe(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits do not deny traversal")
	}
	base := t.TempDir()
	shared := filepath.Join(t.TempDir(), "shared")
	repoRoot := filepath.Join(shared, "proj")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := writeGCCache(t, base, repoRoot)
	backdate(t, dir, cacheDirOrphanTTL+time.Hour)

	// Revoke traversal on the parent: Lstat(repoRoot) now fails with
	// EACCES while the root still exists.
	if err := os.Chmod(shared, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(shared, 0o755) })

	if got := pruneOrphanedCacheDirsIn(base); got != 0 {
		t.Fatalf("pruned = %d, want 0 (EACCES probe is unknowable, not absent)", got)
	}
	if err := os.Chmod(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("cache dir of a live-but-unreadable root was removed: %v", err)
	}
}

func TestPruneOrphanedCacheDirs_SkipsUnknownProvenance(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "mystery-abcd1234")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hashes.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	backdate(t, dir, cacheDirOrphanTTL+time.Hour)

	if got := pruneOrphanedCacheDirsIn(base); got != 0 {
		t.Fatalf("pruned = %d, want 0 (no provenance)", got)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("provenance-less dir was removed: %v", err)
	}
}

func TestPruneOrphanedCacheDirs_SkipsRelativeRecordedRoot(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "relative-abcd1234")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := cacheFileInfosManifest{
		SchemaVersion:     cacheSchemaVersion,
		ExtractorVersions: cloneExtractorVersions(),
		RepoRoot:          "definitely/not/absolute",
	}
	raw, err := json.Marshal(&manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, cacheFileInfosManifestFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	backdate(t, dir, cacheDirOrphanTTL+time.Hour)

	if got := pruneOrphanedCacheDirsIn(base); got != 0 {
		t.Fatalf("pruned = %d, want 0 (relative root)", got)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("relative-root dir was removed: %v", err)
	}
}

// TestPruneOrphanedCacheDirs_RecoversRootFromChunkDirSentinel covers
// caches written by binaries that predate the manifest repo_root
// field: the chunkdir.meta.json sentinel has carried the root since
// the resume machinery landed, so existing leaked worktree caches are
// healable, not just future ones.
func TestPruneOrphanedCacheDirs_RecoversRootFromChunkDirSentinel(t *testing.T) {
	asDurableRoots(t)
	base := t.TempDir()
	repoRoot := gcWorktreeRoot(t, "trace-1718169600123456789-7777")
	dir := writeGCCache(t, base, repoRoot)

	// Rewrite the manifest WITHOUT repo_root, as an old binary would
	// have written it, keeping ChunkDir pointing at the live chunk
	// dir whose sentinel still records the root.
	raw, err := os.ReadFile(filepath.Join(dir, cacheFileInfosManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var manifest cacheFileInfosManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.RepoRoot == "" {
		t.Fatal("test setup: writer no longer records repo_root")
	}
	manifest.RepoRoot = ""
	raw, err = json.Marshal(&manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, cacheFileInfosManifestFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(repoRoot); err != nil {
		t.Fatal(err)
	}
	backdate(t, dir, cacheDirPruneGraceWindow+time.Hour)

	if got := pruneOrphanedCacheDirsIn(base); got != 1 {
		t.Fatalf("pruned = %d, want 1 (sentinel fallback)", got)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cache dir still exists after sentinel-based prune: %v", err)
	}
}

// TestPruneOrphanedCacheDirs_OrphanChunksWithoutManifest covers a scan
// interrupted before installing any manifest: chunk dirs plus
// sentinels are the only provenance.
func TestPruneOrphanedCacheDirs_OrphanChunksWithoutManifest(t *testing.T) {
	asDurableRoots(t)
	base := t.TempDir()
	repoRoot := gcWorktreeRoot(t, "trace-1718169600123456789-6666")
	dir := filepath.Join(base, CacheDirSlug(repoRoot))
	w, err := NewFileInfoCacheWriter(dir, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, fi := range gcTestFileInfos() {
		if err := w.Append(fi); err != nil {
			t.Fatal(err)
		}
	}
	// No Close: simulate the interruption. Chunk dir + sentinel exist,
	// manifest does not.
	if _, err := os.Stat(filepath.Join(dir, cacheFileInfosManifestFile)); !os.IsNotExist(err) {
		t.Fatal("test setup: manifest unexpectedly installed")
	}

	if err := os.RemoveAll(repoRoot); err != nil {
		t.Fatal(err)
	}
	backdate(t, dir, cacheDirPruneGraceWindow+time.Hour)

	if got := pruneOrphanedCacheDirsIn(base); got != 1 {
		t.Fatalf("pruned = %d, want 1 (orphan chunks)", got)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cache dir still exists after orphan-chunk prune: %v", err)
	}
}

// TestManifestRecordsCanonicalRepoRoot pins the provenance write: the
// manifest must record the absolute, symlink-resolved root — the same
// identity CacheDirSlug derives the dir name from — and must leave an
// empty root empty rather than fabricating the process CWD.
func TestManifestRecordsCanonicalRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()

	dir := t.TempDir()
	if err := saveFileInfos(dir, repoRoot, gcTestFileInfos()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, cacheFileInfosManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var manifest cacheFileInfosManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	want := canonicalCacheRepoRoot(repoRoot)
	if manifest.RepoRoot != want {
		t.Errorf("manifest repo_root = %q, want canonical %q", manifest.RepoRoot, want)
	}
	if !filepath.IsAbs(manifest.RepoRoot) {
		t.Errorf("manifest repo_root not absolute: %q", manifest.RepoRoot)
	}

	emptyDir := t.TempDir()
	if err := saveFileInfos(emptyDir, "", gcTestFileInfos()); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(filepath.Join(emptyDir, cacheFileInfosManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var emptyManifest cacheFileInfosManifest
	if err := json.Unmarshal(raw, &emptyManifest); err != nil {
		t.Fatal(err)
	}
	if emptyManifest.RepoRoot != "" {
		t.Errorf("empty repoRoot recorded as %q, want empty (no fabricated CWD provenance)", emptyManifest.RepoRoot)
	}
}

// TestSlugAndProvenanceShareCanonicaliser is the structural guard for
// the shared-canonicaliser rule: the slug minted from any path form
// (raw, relative, symlinked) must equal the slug minted from the
// recorded provenance form, so the pruner's existence check always
// speaks for the path identity the dir name was derived from.
func TestSlugAndProvenanceShareCanonicaliser(t *testing.T) {
	target := t.TempDir()
	linkParent := t.TempDir()
	link := filepath.Join(linkParent, "alias")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	for _, form := range []string{target, link} {
		canonical := canonicalCacheRepoRoot(form)
		if CacheDirSlug(form) != CacheDirSlug(canonical) {
			t.Errorf("slug(%q) != slug(canonical %q) — slug and provenance canonicalisation diverged", form, canonical)
		}
	}
	if CacheDirSlug(link) != CacheDirSlug(target) {
		t.Errorf("symlink form and target mint different slugs")
	}
	if got := canonicalCacheRepoRoot(link); got != canonicalCacheRepoRoot(target) {
		t.Errorf("recorded provenance differs across path forms: %q vs %q", got, canonicalCacheRepoRoot(target))
	}
}

// TestPruneOrphanedCacheDirs_TempTreeRootEphemeralDespiteMissingParent
// pins the system-temp-tree classification that heals the dominant
// real-world leak: test suites scanning t.TempDir repos through the
// production path. Those roots vanish parent-and-all when the test
// run's temp tree is cleaned, so they must be reclaimed on the short
// grace window without the parent-exists requirement.
func TestPruneOrphanedCacheDirs_TempTreeRootEphemeralDespiteMissingParent(t *testing.T) {
	base := t.TempDir()
	rootParent := filepath.Join(t.TempDir(), "TestSomething123456789")
	repoRoot := filepath.Join(rootParent, "001")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if !underSystemTempTree(canonicalCacheRepoRoot(repoRoot)) {
		t.Fatalf("test premise broken: %s not classified under the system temp tree", repoRoot)
	}
	dir := writeGCCache(t, base, repoRoot)

	// The whole test temp tree is cleaned: parent gone with the root.
	if err := os.RemoveAll(rootParent); err != nil {
		t.Fatal(err)
	}
	backdate(t, dir, cacheDirPruneGraceWindow+time.Hour)

	if got := pruneOrphanedCacheDirsIn(base); got != 1 {
		t.Fatalf("pruned = %d, want 1 (temp-tree root, parent-exists waived)", got)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("temp-tree cache dir still exists after prune: %v", err)
	}
}
