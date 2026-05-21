package index

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestApplyScanGOMAXPROCS(t *testing.T) {
	defer func(prev int) { scanReserveCPUs = prev }(scanReserveCPUs)

	// Reserve 0 → no-op: GOMAXPROCS untouched.
	scanReserveCPUs = 0
	before := runtime.GOMAXPROCS(0)
	ApplyScanGOMAXPROCS()()
	if got := runtime.GOMAXPROCS(0); got != before {
		t.Fatalf("reserve=0 must not change GOMAXPROCS: %d → %d", before, got)
	}

	// Reserve 1 → GOMAXPROCS capped to the budget for the scan, then
	// restored. On a single-core base the budget clamps to 1 and the
	// call is a no-op, which is still correct.
	scanReserveCPUs = 1
	before = runtime.GOMAXPROCS(0)
	restore := ApplyScanGOMAXPROCS()
	if during, want := runtime.GOMAXPROCS(0), scanCPUBudget(); during != want {
		t.Fatalf("during scan GOMAXPROCS = %d, want budget %d", during, want)
	}
	restore()
	if after := runtime.GOMAXPROCS(0); after != before {
		t.Fatalf("GOMAXPROCS not restored: %d → %d", before, after)
	}
}

// resumeTestLanguages is every language the repomap cache versions —
// resume must be language-agnostic, so the reuse path is exercised
// against all of them, not just one.
func resumeTestLanguages() []string {
	langs := make([]string, 0, len(extractorVersions))
	for l := range extractorVersions {
		langs = append(langs, l)
	}
	sort.Strings(langs)
	return langs
}

// mkRepoFile writes a real file under root and returns the matching
// FileEntry. The on-disk bytes are what hash verification re-reads.
func mkRepoFile(t *testing.T, root, rel, content, lang string) FileEntry {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return FileEntry{RelPath: rel, AbsPath: abs, Language: lang, Size: int64(len(content))}
}

// cachedFileInfo builds a FileInfo whose Hash matches content, as a
// parser would have produced when this file was last scanned.
func cachedFileInfo(rel, content, lang string) *types.FileInfo {
	return &types.FileInfo{
		RelPath:  rel,
		Language: lang,
		Size:     int64(len(content)),
		Hash:     contentHash([]byte(content)),
	}
}

// writeOrphanChunkDir streams the given FileInfo records into a fresh
// chunk directory and flushes them WITHOUT installing a manifest —
// exactly the on-disk state an OOM-killed scan leaves behind.
func writeOrphanChunkDir(t *testing.T, cacheDir, repoRoot string, infos []*types.FileInfo) *FileInfoCacheWriter {
	t.Helper()
	w, err := NewFileInfoCacheWriter(cacheDir, repoRoot)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	for _, fi := range infos {
		if err := w.Append(fi); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := w.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	return w
}

func TestResumableFileInfos_ReusesAcrossAllLanguages(t *testing.T) {
	defer func(prev bool) { resumeInterruptedScan = prev }(resumeInterruptedScan)
	resumeInterruptedScan = true

	repo := t.TempDir()
	cache := t.TempDir()

	var entries []FileEntry
	var infos []*types.FileInfo
	for i, lang := range resumeTestLanguages() {
		rel := fmt.Sprintf("src/file_%d_%s.src", i, lang)
		content := fmt.Sprintf("// %s source %d\nsymbol_%d\n", lang, i, i)
		entries = append(entries, mkRepoFile(t, repo, rel, content, lang))
		infos = append(infos, cachedFileInfo(rel, content, lang))
	}
	writeOrphanChunkDir(t, cache, repo, infos)

	got := ResumableFileInfos(cache, entries)
	if len(got) != len(entries) {
		t.Fatalf("reused %d of %d files; resume must cover every language", len(got), len(entries))
	}
	for _, e := range entries {
		if got[e.RelPath] == nil {
			t.Errorf("language %s file %s was not resumed", e.Language, e.RelPath)
		}
	}
}

func TestResumableFileInfos_RejectsChangedContent(t *testing.T) {
	defer func(prev bool) { resumeInterruptedScan = prev }(resumeInterruptedScan)
	resumeInterruptedScan = true

	repo := t.TempDir()
	cache := t.TempDir()

	stable := mkRepoFile(t, repo, "a.go", "package a\n", types.LangGo)
	mutated := mkRepoFile(t, repo, "b.py", "print(1)\n", types.LangPython)
	infos := []*types.FileInfo{
		cachedFileInfo("a.go", "package a\n", types.LangGo),
		cachedFileInfo("b.py", "print(1)\n", types.LangPython),
	}
	writeOrphanChunkDir(t, cache, repo, infos)

	// b.py's bytes change after the interrupted scan recorded its hash.
	if err := os.WriteFile(mutated.AbsPath, []byte("print(2)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := ResumableFileInfos(cache, []FileEntry{stable, mutated})
	if got[stable.RelPath] == nil {
		t.Error("unchanged file should be resumed")
	}
	if got[mutated.RelPath] != nil {
		t.Error("content-changed file must NOT be resumed")
	}
}

func TestResumableFileInfos_RejectsLanguageChange(t *testing.T) {
	defer func(prev bool) { resumeInterruptedScan = prev }(resumeInterruptedScan)
	resumeInterruptedScan = true

	repo := t.TempDir()
	cache := t.TempDir()

	content := "shared bytes\n"
	entry := mkRepoFile(t, repo, "x.src", content, types.LangRust)
	// The cached record classified the same path as a different
	// language — reuse across a language change is unsafe.
	writeOrphanChunkDir(t, cache, repo, []*types.FileInfo{
		cachedFileInfo("x.src", content, types.LangCpp),
	})

	if got := ResumableFileInfos(cache, []FileEntry{entry}); len(got) != 0 {
		t.Fatalf("reuse must be rejected when the detected language changed; got %d", len(got))
	}
}

func TestResumableFileInfos_Disabled(t *testing.T) {
	defer func(prev bool) { resumeInterruptedScan = prev }(resumeInterruptedScan)
	resumeInterruptedScan = false

	repo := t.TempDir()
	cache := t.TempDir()
	entry := mkRepoFile(t, repo, "a.go", "package a\n", types.LangGo)
	writeOrphanChunkDir(t, cache, repo, []*types.FileInfo{
		cachedFileInfo("a.go", "package a\n", types.LangGo),
	})

	if got := ResumableFileInfos(cache, []FileEntry{entry}); got != nil {
		t.Fatalf("resume disabled must return nil, got %d entries", len(got))
	}
}

func TestResumableFileInfos_ExcludesLiveManifestDir(t *testing.T) {
	defer func(prev bool) { resumeInterruptedScan = prev }(resumeInterruptedScan)
	resumeInterruptedScan = true

	repo := t.TempDir()
	cache := t.TempDir()
	entry := mkRepoFile(t, repo, "a.go", "package a\n", types.LangGo)

	// A *completed* scan: Close installs the manifest, so this chunk
	// directory is the live cache — not an interrupted orphan.
	w := writeOrphanChunkDir(t, cache, repo, []*types.FileInfo{
		cachedFileInfo("a.go", "package a\n", types.LangGo),
	})
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if got := ResumableFileInfos(cache, []FileEntry{entry}); got != nil {
		t.Fatalf("the live manifest's chunk dir must not be treated as resumable; got %d", len(got))
	}
}

func TestResumableFileInfos_RejectsIncompatibleSentinel(t *testing.T) {
	defer func(prev bool) { resumeInterruptedScan = prev }(resumeInterruptedScan)
	resumeInterruptedScan = true

	repo := t.TempDir()
	cache := t.TempDir()
	entry := mkRepoFile(t, repo, "a.go", "package a\n", types.LangGo)
	w := writeOrphanChunkDir(t, cache, repo, []*types.FileInfo{
		cachedFileInfo("a.go", "package a\n", types.LangGo),
	})

	// Rewrite the sentinel with an incompatible schema version, as a
	// chunk dir from an older binary would carry.
	bad, _ := json.Marshal(&chunkDirMeta{
		SchemaVersion:     cacheSchemaVersion + 99,
		ExtractorVersions: cloneExtractorVersions(),
	})
	if err := os.WriteFile(filepath.Join(w.chunkDir, cacheChunkDirMetaFile), bad, 0o644); err != nil {
		t.Fatal(err)
	}

	if got := ResumableFileInfos(cache, []FileEntry{entry}); got != nil {
		t.Fatalf("chunks with an incompatible sentinel must be ignored; got %d", len(got))
	}
}

func TestResumableFileInfos_SkipsCorruptChunk(t *testing.T) {
	defer func(prev bool) { resumeInterruptedScan = prev }(resumeInterruptedScan)
	resumeInterruptedScan = true

	repo := t.TempDir()
	cache := t.TempDir()

	good := []*types.FileInfo{
		cachedFileInfo("a.go", "package a\n", types.LangGo),
		cachedFileInfo("b.go", "package b\n", types.LangGo),
	}
	entries := []FileEntry{
		mkRepoFile(t, repo, "a.go", "package a\n", types.LangGo),
		mkRepoFile(t, repo, "b.go", "package b\n", types.LangGo),
		mkRepoFile(t, repo, "c.go", "package c\n", types.LangGo),
	}

	w, err := NewFileInfoCacheWriter(cache, repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, fi := range good {
		if err := w.Append(fi); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.flush(); err != nil { // chunk-00000: a.go, b.go
		t.Fatal(err)
	}
	if err := w.Append(cachedFileInfo("c.go", "package c\n", types.LangGo)); err != nil {
		t.Fatal(err)
	}
	if err := w.flush(); err != nil { // chunk-00001: c.go
		t.Fatal(err)
	}
	// Corrupt the second chunk — a partial write from a kill mid-flush.
	if err := os.WriteFile(filepath.Join(w.chunkDir, "chunk-00001.json"), []byte("{ truncated"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := ResumableFileInfos(cache, entries)
	if len(got) != 2 {
		t.Fatalf("a corrupt chunk must be skipped while intact chunks resume; got %d, want 2", len(got))
	}
	if got["c.go"] != nil {
		t.Error("c.go came from the corrupt chunk and must not be resumed")
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atomic.json")
	if err := writeFileAtomic(path, []byte("hello")); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "hello" {
		t.Fatalf("read back = %q, %v", got, err)
	}
	// No .tmp- residue should survive a successful write.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected only the final file, got %d entries", len(entries))
	}
}
