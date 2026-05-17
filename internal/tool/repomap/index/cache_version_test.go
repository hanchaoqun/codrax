package index

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestCacheVersionRoundTrip writes a graph to a temp cache dir and
// reads it back through the versioned payload wrapper, verifying
// that a fresh save-then-load reproduces the types.FileInfo slice.
func TestCacheVersionRoundTrip(t *testing.T) {
	dir := t.TempDir()

	files := []*types.FileInfo{
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
	if err := saveFileInfos(dir, "", files); err != nil {
		t.Fatalf("saveFileInfos: %v", err)
	}
	got := LoadFileInfos(dir)
	if len(got) != 1 {
		t.Fatalf("LoadFileInfos: got %d files, want 1", len(got))
	}
	if got[0].RelPath != "a.go" || got[0].Symbols[0].Name != "Foo" {
		t.Errorf("round-trip corrupted: %+v", got[0])
	}
}

// TestCacheVersionMismatchInvalidates writes a cache with the
// current schema then tampers with the SchemaVersion field and
// verifies LoadFileInfos returns nil (forcing a rescan).
func TestCacheVersionMismatchInvalidates(t *testing.T) {
	dir := t.TempDir()
	files := []*types.FileInfo{{RelPath: "a.go", Language: types.LangGo}}
	if err := saveFileInfos(dir, "", files); err != nil {
		t.Fatal(err)
	}

	manifest := readFileInfosManifestForTest(t, dir)
	manifest.SchemaVersion = cacheSchemaVersion + 99
	writeFileInfosManifestForTest(t, dir, manifest)

	if got := LoadFileInfos(dir); got != nil {
		t.Errorf("expected nil on schema mismatch, got %d files", len(got))
	}
}

// TestCacheExtractorVersionMismatchInvalidates verifies that a
// per-language extractor version bump invalidates caches
// previously written against the older version, but caches that
// match on every key are still accepted.
func TestCacheExtractorVersionMismatchInvalidates(t *testing.T) {
	dir := t.TempDir()
	files := []*types.FileInfo{{RelPath: "a.go", Language: types.LangGo}}
	if err := saveFileInfos(dir, "", files); err != nil {
		t.Fatal(err)
	}

	manifest := readFileInfosManifestForTest(t, dir)
	manifest.ExtractorVersions[types.LangGo] = 99
	writeFileInfosManifestForTest(t, dir, manifest)

	if got := LoadFileInfos(dir); got != nil {
		t.Errorf("expected nil on extractor version mismatch, got %d files", len(got))
	}
}

// TestCacheChecksumMismatchInvalidates tampers with the Files
// slice after the checksum has been computed and verifies the
// loader rejects the corrupted cache.
func TestCacheChecksumMismatchInvalidates(t *testing.T) {
	dir := t.TempDir()
	files := []*types.FileInfo{{RelPath: "a.go", Language: types.LangGo}}
	if err := saveFileInfos(dir, "", files); err != nil {
		t.Fatal(err)
	}

	manifest := readFileInfosManifestForTest(t, dir)
	if len(manifest.Chunks) == 0 {
		t.Fatal("expected at least one fileinfos chunk")
	}
	chunkPath := filepath.Join(dir, manifest.ChunkDir, manifest.Chunks[0].File)
	raw, err := os.ReadFile(chunkPath)
	if err != nil {
		t.Fatal(err)
	}
	var chunkFiles []*types.FileInfo
	if err := json.Unmarshal(raw, &chunkFiles); err != nil {
		t.Fatal(err)
	}
	chunkFiles[0].RelPath = "TAMPERED.go"
	out, _ := json.Marshal(chunkFiles)
	if err := os.WriteFile(chunkPath, out, 0o644); err != nil {
		t.Fatal(err)
	}

	if got := LoadFileInfos(dir); got != nil {
		t.Errorf("expected nil on checksum mismatch, got %+v", got)
	}
}

func TestLegacyCachePayloadStillLoads(t *testing.T) {
	dir := t.TempDir()
	files := []*types.FileInfo{{RelPath: "legacy.go", Language: types.LangGo}}
	writeLegacyCachePayloadForTest(t, dir, files)
	got := LoadFileInfos(dir)
	if len(got) != 1 || got[0].RelPath != "legacy.go" {
		t.Fatalf("legacy cache payload should still load, got %+v", got)
	}
}

func readFileInfosManifestForTest(t *testing.T, dir string) cacheFileInfosManifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, cacheFileInfosManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var manifest cacheFileInfosManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func writeFileInfosManifestForTest(t *testing.T, dir string, manifest cacheFileInfosManifest) {
	t.Helper()
	out, err := json.Marshal(&manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, cacheFileInfosManifestFile), out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeLegacyCachePayloadForTest(t *testing.T, dir string, files []*types.FileInfo) {
	t.Helper()
	filesData, err := json.Marshal(files)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(filesData)
	payload := cachePayload{
		SchemaVersion:     cacheSchemaVersion,
		ExtractorVersions: cloneExtractorVersions(),
		Checksum:          hex.EncodeToString(sum[:8]),
		Files:             files,
	}
	out, err := json.Marshal(&payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, cacheFileInfosFile), out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestChangedFilesUsesHashesForCleanCommittedRepo(t *testing.T) {
	repo := t.TempDir()
	if err := runGitForCacheTest(repo, "init"); err != nil {
		t.Skipf("git unavailable for cache invalidation regression: %v", err)
	}
	if err := runGitForCacheTest(repo, "config", "user.email", "cache-test@example.com"); err != nil {
		t.Skipf("git config failed: %v", err)
	}
	if err := runGitForCacheTest(repo, "config", "user.name", "cache-test"); err != nil {
		t.Skipf("git config failed: %v", err)
	}
	const rel = "a.go"
	current := []byte("package main\n\nfunc Current() {}\n")
	abs := filepath.Join(repo, rel)
	if err := os.WriteFile(abs, current, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runGitForCacheTest(repo, "add", rel); err != nil {
		t.Fatal(err)
	}
	if err := runGitForCacheTest(repo, "commit", "-m", "current"); err != nil {
		t.Fatal(err)
	}

	cacheDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cacheDir, cacheMetaFile), []byte("# meta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	staleHashes := "# File Hashes\n\n" + rel + "\tdeadbeef\tgo\t1\n"
	if err := os.WriteFile(filepath.Join(cacheDir, cacheHashesFile), []byte(staleHashes), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ChangedFiles(repo, cacheDir, []FileEntry{{
		RelPath:  rel,
		AbsPath:  abs,
		Language: types.LangGo,
		Size:     int64(len(current)),
	}})
	if len(got) != 1 || got[0] != rel {
		t.Fatalf("clean committed repo with stale cached hash should reparse %s, got %v", rel, got)
	}
}

func TestBasicFileInfoPersistsHashForUnsupportedFiles(t *testing.T) {
	repo := t.TempDir()
	const rel = "notes.txt"
	body := []byte("plain text still participates in cache invalidation\n")
	abs := filepath.Join(repo, rel)
	if err := os.WriteFile(abs, body, 0o644); err != nil {
		t.Fatal(err)
	}

	info := BasicFileInfo(FileEntry{RelPath: rel, AbsPath: abs, Size: int64(len(body))})
	if info.Hash == "" {
		t.Fatal("unsupported-file cache entry must carry a content hash")
	}

	cacheDir := t.TempDir()
	graph := BuildGraph(repo, []*types.FileInfo{info})
	if err := SaveCache(cacheDir, graph); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}
	changed := ChangedFiles(repo, cacheDir, []FileEntry{{
		RelPath: rel,
		AbsPath: abs,
		Size:    int64(len(body)),
	}})
	if len(changed) != 0 {
		t.Fatalf("fresh unsupported file should be a cache hit, changed=%v", changed)
	}
}

func runGitForCacheTest(repo string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &gitCacheTestError{args: args, out: strings.TrimSpace(string(out)), err: err}
	}
	return nil
}

type gitCacheTestError struct {
	args []string
	out  string
	err  error
}

func (e *gitCacheTestError) Error() string {
	if e == nil {
		return ""
	}
	if e.out == "" {
		return e.err.Error()
	}
	return e.err.Error() + ": " + e.out
}
