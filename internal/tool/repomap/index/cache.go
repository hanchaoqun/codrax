package index

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// cacheBaseDir is the root directory for repo map caches. When empty
// (the default), CacheDir falls back to ~/.codrax/cache/repomap.
// main.go can override it at startup via SetCacheDir so the operator
// can point all caches at a single tree.
var cacheBaseDir string

// SetCacheDir overrides the base directory for repo map caches.
// An empty value restores the default (~/.codrax/cache). Called once
// from main.go before any tool runs.
func SetCacheDir(dir string) {
	cacheBaseDir = dir
}

// resumeInterruptedScan controls whether a full scan reuses the
// already-parsed chunks left behind by an earlier scan that was
// interrupted before installing its manifest. Default enabled;
// SetResumeInterruptedScan honours the codrax.yaml knob at startup.
var resumeInterruptedScan = true

// SetResumeInterruptedScan toggles interrupted-scan resume. Called once
// from main.go before any tool runs.
func SetResumeInterruptedScan(enabled bool) {
	resumeInterruptedScan = enabled
}

// scanReserveCPUs is the number of CPU cores a repomap scan leaves
// free for interactive processes (notably sshd) so a long full scan of
// a huge repository does not freeze a remote session. It is enforced by
// ApplyScanGOMAXPROCS, which caps the whole Go runtime for the scan.
var scanReserveCPUs int

// SetScanReserveCPUs sets how many cores a scan leaves free. Called
// once from main.go before any tool runs.
func SetScanReserveCPUs(n int) {
	if n < 0 {
		n = 0
	}
	scanReserveCPUs = n
}

// baseGOMAXPROCS captures the process's intended parallelism once at
// package init, before any scan lowers runtime.GOMAXPROCS. Scan budgets
// derive from this stable base so repeated ApplyScanGOMAXPROCS calls do
// not compound the subtraction.
var baseGOMAXPROCS = runtime.GOMAXPROCS(0)

// scanCPUBudget is how many CPUs scan work may occupy: the process's
// base GOMAXPROCS minus the reserved cores, always at least 1.
func scanCPUBudget() int {
	n := baseGOMAXPROCS - scanReserveCPUs
	if n < 1 {
		n = 1
	}
	return n
}

// ApplyScanGOMAXPROCS caps runtime.GOMAXPROCS to the scan CPU budget for
// the duration of a scan and returns a function that restores the
// previous value.
//
// Per-thread niceness only tames the threads codrax explicitly creates
// and locks (the parse-worker pool). The Go runtime's own threads — GC
// workers (which GOMEMLIMIT keeps busy), the scheduler, and the
// goroutines that build and rank the graph — run at normal priority and
// can still saturate every core. GOMAXPROCS is the lever that bounds
// the *entire* runtime: capping it hard-reserves cores for interactive
// processes (sshd) regardless of what the Go side is doing. No-op when
// no cores are reserved.
func ApplyScanGOMAXPROCS() func() {
	if scanReserveCPUs <= 0 {
		return func() {}
	}
	budget := scanCPUBudget()
	prev := runtime.GOMAXPROCS(budget)
	if prev == budget {
		return func() {}
	}
	return func() { runtime.GOMAXPROCS(prev) }
}

// CacheDir returns the cache directory for a given repo root.
// Default: ~/.codrax/cache/repomap/<repo-slug>/
// Configured: <cache_dir>/repomap/<repo-slug>/
//
// All cross-project per-user codrax storage lives under
// ~/.codrax/ — the repomap cache, env_recommend cache, future
// per-user state. Pre-2026-04-30 the repomap cache used
// os.UserCacheDir() ("~/.cache/codrax" on Linux), splitting
// per-user storage across two roots; that drifted from the
// env-cache helper which already used ~/.codrax/cache/. Unified
// here so a fresh user sees one well-known location for all
// cross-project codrax state.
//
// Per-platform default lookup is via os.UserHomeDir():
//   - Linux:   ~/.codrax/cache/repomap/
//   - macOS:   ~/.codrax/cache/repomap/
//   - Windows: %USERPROFILE%\.codrax\cache\repomap\
//
// When os.UserHomeDir fails (rare; broken env), falls back to
// os.TempDir()/codrax-cache so the rest of the binary keeps
// running with a non-persistent cache.
func CacheDir(repoRoot string) string {
	slug := CacheDirSlug(repoRoot)

	base := cacheBaseDir
	if base == "" {
		home, herr := os.UserHomeDir()
		if herr != nil {
			base = filepath.Join(os.TempDir(), "codrax-cache")
		} else {
			base = filepath.Join(home, ".codrax", "cache")
		}
	}
	return filepath.Join(base, "repomap", slug)
}

// CacheDirSlug returns the canonical "<basename>-<8hex>" slug for a
// repo root path. Exposed so the topology / multigraph packages can
// mint per-sub-repo slugs that are byte-identical to the slugs
// CacheDir writes — keeping repomap on-disk cache, multi-graph LRU
// keys, and topology metadata in a single namespace ("slug 真同源",
// design §3.3.1 / §9 decision #7).
//
// The path is normalised via filepath.Abs + filepath.EvalSymlinks so
// two callers pointing at the same content produce the same slug
// regardless of relative-path or symlink form.
func CacheDirSlug(repoRoot string) string {
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		abs = repoRoot
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	h := sha256.Sum256([]byte(abs))
	return filepath.Base(abs) + "-" + hex.EncodeToString(h[:4])
}

const (
	cacheSymbolsFile           = "symbols.md"
	cacheRelationsFile         = "relations.md"
	cacheMetaFile              = "meta.md"
	cacheHashesFile            = "hashes.md"
	cacheFileInfosFile         = "fileinfos.json"
	cacheFileInfosManifestFile = "fileinfos.manifest.json"

	// cacheChunkDirMetaFile is a sentinel written into every chunk
	// directory at creation time. It lets a resuming scan validate
	// that orphan chunks (left by an interrupted scan with no
	// installed manifest) were written by a compatible binary before
	// trusting their contents.
	cacheChunkDirMetaFile = "chunkdir.meta.json"

	// cacheSchemaVersion tracks the cache-file layout. Bump this
	// whenever the fileinfos.json wrapper shape changes (fields
	// added or removed from cachePayload, not per-language
	// extractor updates — those go in extractorVersions). Loaders
	// reject any cache whose SchemaVersion doesn't match, forcing
	// a full rescan on the next BuildOrLoadGraph.
	cacheSchemaVersion = 3
)

const cacheFileInfosChunkSize = 1024

// extractorVersions tracks per-language extractor generations.
// Bump the relevant entry whenever an extract_*.go file changes
// its output semantics (e.g. the Phase 1 P1.0 grouped-import
// fix). Any mismatch between the cache and the current map
// invalidates the cache wholesale — individual-file invalidation
// would be cheaper but adds complexity we don't need until scan
// latency is a real bottleneck.
var extractorVersions = map[string]int{
	types.LangGo:         2, // P1.0 grouped-import fix; P1.2a call receiver capture
	types.LangJava:       1,
	types.LangPython:     1,
	types.LangJavaScript: 1,
	types.LangTypeScript: 1,
	types.LangArkTS:      2,
	types.LangCangjie:    2,
	types.LangKotlin:     2,
	types.LangRuby:       1,
	types.LangSwift:      2,
	types.LangLua:        1,
	types.LangProto:      1,
	types.LangRust:       1,
	types.LangC:          1,
	types.LangCpp:        1,
}

// cachePayload is the wrapper around fileinfos.json. Previous
// versions persisted the []*types.FileInfo slice bare; Phase 3 wraps it
// in a header recording schema version, per-language extractor
// versions, the repo HEAD SHA when the cache was written, and a
// truncated SHA-256 checksum over the Files slice for corruption
// detection.
type cachePayload struct {
	SchemaVersion     int               `json:"schema_version"`
	ExtractorVersions map[string]int    `json:"extractor_versions"`
	RepoHead          string            `json:"repo_head,omitempty"`
	WrittenAt         string            `json:"written_at,omitempty"`
	Checksum          string            `json:"checksum,omitempty"`
	Files             []*types.FileInfo `json:"files"`
}

type cacheFileInfosManifest struct {
	SchemaVersion     int                   `json:"schema_version"`
	ExtractorVersions map[string]int        `json:"extractor_versions"`
	RepoHead          string                `json:"repo_head,omitempty"`
	WrittenAt         string                `json:"written_at,omitempty"`
	TotalFiles        int                   `json:"total_files"`
	ChunkSize         int                   `json:"chunk_size"`
	ChunkDir          string                `json:"chunk_dir"`
	Checksum          string                `json:"checksum,omitempty"`
	Chunks            []cacheFileInfosChunk `json:"chunks"`
}

type cacheFileInfosChunk struct {
	File     string `json:"file"`
	Count    int    `json:"count"`
	Checksum string `json:"checksum"`
}

// chunkDirMeta is the chunkdir.meta.json sentinel. It carries enough
// version information for a resuming scan to reject chunks written by
// an incompatible binary, mirroring the validation cacheManifestVersionValid
// applies to a fully installed manifest.
type chunkDirMeta struct {
	SchemaVersion     int            `json:"schema_version"`
	ExtractorVersions map[string]int `json:"extractor_versions"`
	RepoRoot          string         `json:"repo_root,omitempty"`
	WrittenAt         string         `json:"written_at,omitempty"`
}

// FileInfoCacheWriter persists FileInfo records in bounded JSON chunks.
// It is intentionally append-only until Close installs the manifest:
// interrupted scans leave only an unreferenced temp directory, while
// readers keep using the previous manifest/cache.
type FileInfoCacheWriter struct {
	dir        string
	repoRoot   string
	generation string
	chunkDir   string

	buf     []*types.FileInfo
	chunks  []cacheFileInfosChunk
	total   int
	overall hash.Hash
	closed  bool
}

// SaveCache writes the graph index to markdown files and a JSON snapshot
// of types.FileInfo data (for incremental reload) in the cache directory.
func SaveCache(dir string, g *types.Graph) error {
	return saveCache(dir, g, true)
}

// SaveCacheWithoutFileInfos writes the derived cache sidecars while
// preserving an already-installed fileinfos manifest. Full scans use
// this after streaming FileInfo chunks during parsing, avoiding a second
// giant fileinfos write at the end of the scan.
func SaveCacheWithoutFileInfos(dir string, g *types.Graph) error {
	return saveCache(dir, g, false)
}

func saveCache(dir string, g *types.Graph, includeFileInfos bool) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Save file hashes for incremental update
	if err := saveHashes(dir, g); err != nil {
		return err
	}

	if includeFileInfos {
		// Save types.FileInfo JSON for incremental reload, wrapped in a
		// versioned payload so loaders can reject stale caches.
		if err := saveFileInfos(dir, g.Root, g.Files); err != nil {
			return err
		}
	}

	// Save symbols (markdown, for grep)
	if err := saveSymbols(dir, g); err != nil {
		return err
	}

	// Save relations (markdown, for grep)
	if err := saveRelations(dir, g); err != nil {
		return err
	}

	// Save metadata
	return saveMeta(dir, g)
}

func saveFileInfos(dir, repoRoot string, files []*types.FileInfo) error {
	w, err := NewFileInfoCacheWriter(dir, repoRoot)
	if err != nil {
		return err
	}
	for _, fi := range files {
		if err := w.Append(fi); err != nil {
			w.Abort()
			return err
		}
	}
	if err := w.Close(); err != nil {
		w.Abort()
		return err
	}
	return nil
}

// LoadFileInfos reads the cached types.FileInfo data from a previous scan.
// Returns nil — forcing a full rescan — when any of the following
// conditions hold:
//   - the cache file is missing, unreadable, or not valid JSON
//   - the payload has a SchemaVersion that doesn't match the
//     current cacheSchemaVersion (cache-wrapper layout changed)
//   - any per-language entry in ExtractorVersions disagrees with
//     the current extractorVersions map (a given extractor's
//     output semantics changed since this cache was written)
//   - the Checksum doesn't match SHA-256 over the Files payload
//     (on-disk corruption or truncation)
func LoadFileInfos(dir string) []*types.FileInfo {
	if files := loadChunkedFileInfos(dir); files != nil {
		return files
	}
	return loadLegacyFileInfos(dir)
}

func NewFileInfoCacheWriter(dir, repoRoot string) (*FileInfoCacheWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	generation := "fileinfos." + strconv.FormatInt(time.Now().UnixNano(), 36) + ".d"
	chunkDir := filepath.Join(dir, generation)
	if err := os.MkdirAll(chunkDir, 0o755); err != nil {
		return nil, err
	}
	// Drop the resume sentinel immediately, so chunks written before an
	// interruption are still recognisable as compatible. Written
	// atomically so a concurrent process never reads a partial
	// sentinel. Best-effort: a missing sentinel only means a later
	// resume distrusts this dir.
	if raw, err := json.Marshal(&chunkDirMeta{
		SchemaVersion:     cacheSchemaVersion,
		ExtractorVersions: cloneExtractorVersions(),
		RepoRoot:          repoRoot,
		WrittenAt:         time.Now().UTC().Format(time.RFC3339),
	}); err == nil {
		_ = writeFileAtomic(filepath.Join(chunkDir, cacheChunkDirMetaFile), raw)
	}
	return &FileInfoCacheWriter{
		dir:        dir,
		repoRoot:   repoRoot,
		generation: generation,
		chunkDir:   chunkDir,
		buf:        make([]*types.FileInfo, 0, cacheFileInfosChunkSize),
		overall:    sha256.New(),
	}, nil
}

func (w *FileInfoCacheWriter) Append(fi *types.FileInfo) error {
	if w == nil || w.closed || fi == nil {
		return nil
	}
	w.buf = append(w.buf, fi)
	w.total++
	if len(w.buf) < cacheFileInfosChunkSize {
		return nil
	}
	return w.flush()
}

func (w *FileInfoCacheWriter) Close() error {
	if w == nil || w.closed {
		return nil
	}
	if err := w.flush(); err != nil {
		return err
	}
	manifest := cacheFileInfosManifest{
		SchemaVersion:     cacheSchemaVersion,
		ExtractorVersions: cloneExtractorVersions(),
		RepoHead:          gitHeadSHA(w.repoRoot),
		WrittenAt:         time.Now().UTC().Format(time.RFC3339),
		TotalFiles:        w.total,
		ChunkSize:         cacheFileInfosChunkSize,
		ChunkDir:          w.generation,
		Checksum:          hex.EncodeToString(w.overall.Sum(nil)[:8]),
		Chunks:            w.chunks,
	}
	out, err := json.Marshal(&manifest)
	if err != nil {
		return fmt.Errorf("marshal fileinfos manifest: %w", err)
	}
	tmpManifest := filepath.Join(w.dir, cacheFileInfosManifestFile+".tmp-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	if err := os.WriteFile(tmpManifest, out, 0o644); err != nil {
		return fmt.Errorf("write fileinfos manifest: %w", err)
	}
	if err := os.Rename(tmpManifest, filepath.Join(w.dir, cacheFileInfosManifestFile)); err != nil {
		_ = os.Remove(tmpManifest)
		return fmt.Errorf("install fileinfos manifest: %w", err)
	}
	w.closed = true
	_ = os.Remove(filepath.Join(w.dir, cacheFileInfosFile))
	pruneOldFileInfoChunkDirs(w.dir, w.generation)
	return nil
}

func (w *FileInfoCacheWriter) Abort() {
	if w == nil || w.closed {
		return
	}
	_ = os.RemoveAll(w.chunkDir)
	w.closed = true
}

func (w *FileInfoCacheWriter) flush() error {
	if w == nil || len(w.buf) == 0 {
		return nil
	}
	data, err := json.Marshal(w.buf)
	if err != nil {
		return fmt.Errorf("marshal fileinfos chunk %d: %w", len(w.chunks), err)
	}
	sum := sha256.Sum256(data)
	name := fmt.Sprintf("chunk-%05d.json", len(w.chunks))
	// Atomic write: a chunk file is either absent or complete, never
	// truncated. A scan killed mid-flush leaves only a .tmp- file,
	// which resume skips — so neither an interrupted scan nor a
	// concurrent process ever sees a corrupt chunk.
	if err := writeFileAtomic(filepath.Join(w.chunkDir, name), data); err != nil {
		return fmt.Errorf("write fileinfos chunk %s: %w", name, err)
	}
	chunk := cacheFileInfosChunk{
		File:     name,
		Count:    len(w.buf),
		Checksum: hex.EncodeToString(sum[:8]),
	}
	w.chunks = append(w.chunks, chunk)
	w.overall.Write([]byte(chunk.File))
	w.overall.Write([]byte{0})
	w.overall.Write([]byte(strconv.Itoa(chunk.Count)))
	w.overall.Write([]byte{0})
	w.overall.Write([]byte(chunk.Checksum))
	w.overall.Write([]byte{'\n'})
	w.buf = w.buf[:0]
	return nil
}

func loadChunkedFileInfos(dir string) []*types.FileInfo {
	raw, err := os.ReadFile(filepath.Join(dir, cacheFileInfosManifestFile))
	if err != nil {
		return nil
	}
	var manifest cacheFileInfosManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil
	}
	if !cacheManifestVersionValid(manifest.SchemaVersion, manifest.ExtractorVersions) {
		return nil
	}
	if manifest.TotalFiles < 0 || manifest.ChunkDir == "" || len(manifest.Chunks) == 0 {
		return nil
	}
	if filepath.Clean(manifest.ChunkDir) != manifest.ChunkDir ||
		filepath.IsAbs(manifest.ChunkDir) ||
		strings.HasPrefix(manifest.ChunkDir, "..") ||
		strings.Contains(manifest.ChunkDir, string(filepath.Separator)+".."+string(filepath.Separator)) {
		return nil
	}

	chunkDir := filepath.Join(dir, manifest.ChunkDir)
	files := make([]*types.FileInfo, 0, manifest.TotalFiles)
	overall := sha256.New()
	for _, chunk := range manifest.Chunks {
		if chunk.File == "" || filepath.Clean(chunk.File) != chunk.File ||
			filepath.IsAbs(chunk.File) || strings.Contains(chunk.File, string(filepath.Separator)) {
			return nil
		}
		data, err := os.ReadFile(filepath.Join(chunkDir, chunk.File))
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		if chunk.Checksum != hex.EncodeToString(sum[:8]) {
			return nil
		}
		var part []*types.FileInfo
		if err := json.Unmarshal(data, &part); err != nil {
			return nil
		}
		if len(part) != chunk.Count {
			return nil
		}
		files = append(files, part...)
		overall.Write([]byte(chunk.File))
		overall.Write([]byte{0})
		overall.Write([]byte(strconv.Itoa(chunk.Count)))
		overall.Write([]byte{0})
		overall.Write([]byte(chunk.Checksum))
		overall.Write([]byte{'\n'})
	}
	if len(files) != manifest.TotalFiles {
		return nil
	}
	if manifest.Checksum != "" && manifest.Checksum != hex.EncodeToString(overall.Sum(nil)[:8]) {
		return nil
	}
	return files
}

func loadLegacyFileInfos(dir string) []*types.FileInfo {
	data, err := os.ReadFile(filepath.Join(dir, cacheFileInfosFile))
	if err != nil {
		return nil
	}
	var payload cachePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	if !cacheManifestVersionValid(payload.SchemaVersion, payload.ExtractorVersions) {
		return nil
	}
	if payload.Checksum != "" {
		filesData, err := json.Marshal(payload.Files)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(filesData)
		if payload.Checksum != hex.EncodeToString(sum[:8]) {
			return nil
		}
	}
	return payload.Files
}

func cacheManifestVersionValid(schemaVersion int, versions map[string]int) bool {
	if schemaVersion != cacheSchemaVersion {
		return false
	}
	for lang, ver := range extractorVersions {
		if versions[lang] != ver {
			return false
		}
	}
	return true
}

func cloneExtractorVersions() map[string]int {
	extractors := make(map[string]int, len(extractorVersions))
	for k, v := range extractorVersions {
		extractors[k] = v
	}
	return extractors
}

// chunkDirPruneGraceWindow shields a chunk directory from pruning for a
// while after its last write. A concurrently-running scan streaming
// into its own directory must not have that directory deleted out from
// under it; a genuine orphan stops being modified and ages past the
// window, at which point a later completed scan reclaims it. Resume
// consumes orphans meanwhile, so the delayed cleanup costs nothing.
const chunkDirPruneGraceWindow = 10 * time.Minute

func pruneOldFileInfoChunkDirs(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-chunkDirPruneGraceWindow)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == keep || !strings.HasPrefix(name, "fileinfos.") || !strings.HasSuffix(name, ".d") {
			continue
		}
		if info, ierr := entry.Info(); ierr == nil && info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(dir, name))
	}
}

// writeFileAtomic writes data to path via a temp file + rename, so no
// reader — including a concurrent codrax process sharing the cache
// directory — ever observes a partial file. The temp file is created
// alongside the destination to keep the rename on one filesystem.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// gitHeadSHA returns the short SHA of repoRoot's current HEAD or
// an empty string when the directory isn't a git repo. Purely
// diagnostic — the rest of the cache validation does not depend
// on it, so a missing git or a detached worktree never forces a
// rescan on its own.
func gitHeadSHA(repoRoot string) string {
	if repoRoot == "" {
		return ""
	}
	cmd, cancel := tool.NewGitCommand(nil, "-C", repoRoot, "rev-parse", "--short", "HEAD")
	defer cancel()
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func contentHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:8])
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return contentHash(data), nil
}

func saveHashes(dir string, g *types.Graph) error {
	var b strings.Builder
	b.WriteString("# File Hashes\n\n")
	for _, fi := range g.Files {
		b.WriteString(fmt.Sprintf("%s\t%s\t%s\t%d\n", fi.RelPath, fi.Hash, fi.Language, fi.Size))
	}
	return os.WriteFile(filepath.Join(dir, cacheHashesFile), []byte(b.String()), 0o644)
}

func saveSymbols(dir string, g *types.Graph) error {
	var b strings.Builder
	b.WriteString("# Symbols Index\n\n")

	for _, fi := range g.Files {
		if len(fi.Symbols) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s", fi.RelPath))
		if fi.Package != "" {
			b.WriteString(fmt.Sprintf(" [pkg:%s]", fi.Package))
		}
		b.WriteString(fmt.Sprintf(" [hash:%s]\n\n", fi.Hash))

		for _, sym := range fi.Symbols {
			exported := ""
			if sym.Exported {
				exported = " [exported]"
			}
			recv := ""
			if sym.Receiver != "" {
				recv = fmt.Sprintf("(%s) ", sym.Receiver)
			} else if sym.Parent != "" {
				recv = fmt.Sprintf("%s.", sym.Parent)
			}
			sig := ""
			if sym.Signature != "" {
				sig = " `" + sym.Signature + "`"
			}
			doc := ""
			if sym.Doc != "" {
				doc = " — " + sym.Doc
			}
			b.WriteString(fmt.Sprintf("- `%s%s` %s :%d-%d%s%s%s\n",
				recv, sym.Name, sym.Kind, sym.Line, sym.EndLine, exported, sig, doc))
		}
		b.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(dir, cacheSymbolsFile), []byte(b.String()), 0o644)
}

func saveRelations(dir string, g *types.Graph) error {
	var b strings.Builder
	b.WriteString("# Relations Index\n\n")

	// Group by kind
	groups := make(map[string][]types.Relation)
	for _, fi := range g.Files {
		for _, rel := range fi.Relations {
			groups[rel.Kind] = append(groups[rel.Kind], rel)
		}
		// import relations
		for _, imp := range fi.Imports {
			groups["import"] = append(groups["import"], types.Relation{
				Kind: "import",
				From: fi.RelPath,
				To:   imp.Path,
				File: fi.RelPath,
				Line: imp.Line,
			})
		}
	}

	kindOrder := []string{"import", "call", "type_usage", "reference", "inheritance", "embedding"}
	for _, kind := range kindOrder {
		rels, ok := groups[kind]
		if !ok || len(rels) == 0 {
			continue
		}
		heading := strings.ReplaceAll(kind, "_", " ")
		if len(heading) > 0 {
			heading = strings.ToUpper(heading[:1]) + heading[1:]
		}
		b.WriteString(fmt.Sprintf("## %s\n\n", heading))

		// deduplicate for display
		seen := make(map[string]bool)
		for _, rel := range rels {
			key := rel.From + "→" + rel.To
			if seen[key] {
				continue
			}
			seen[key] = true
			b.WriteString(fmt.Sprintf("- %s → %s :%d\n", rel.From, rel.To, rel.Line))
		}
		b.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(dir, cacheRelationsFile), []byte(b.String()), 0o644)
}

func saveMeta(dir string, g *types.Graph) error {
	var b strings.Builder
	b.WriteString("# Repo Map types.Metadata\n\n")
	b.WriteString(fmt.Sprintf("- **Scan time**: %s\n", g.Metadata.ScanTime.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- **Files**: %d\n", g.Metadata.FileCount))
	b.WriteString(fmt.Sprintf("- **Symbols**: %d\n", g.Metadata.SymbolCount))
	b.WriteString(fmt.Sprintf("- **Relations**: %d\n", g.Metadata.RelationCount))
	b.WriteString("\n## Languages\n\n")
	for lang, count := range g.Metadata.Languages {
		b.WriteString(fmt.Sprintf("- %s: %d\n", lang, count))
	}
	if len(g.Metadata.SpecialFiles) > 0 {
		b.WriteString("\n## Special Files\n\n")
		for _, f := range g.Metadata.SpecialFiles {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}
	return os.WriteFile(filepath.Join(dir, cacheMetaFile), []byte(b.String()), 0o644)
}

// LoadCachedHashes reads the file hashes from a previous scan.
// Returns a map of relPath → hash.
func LoadCachedHashes(dir string) map[string]string {
	data, err := os.ReadFile(filepath.Join(dir, cacheHashesFile))
	if err != nil {
		return nil
	}
	hashes := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) >= 2 {
			hashes[parts[0]] = parts[1]
		}
	}
	return hashes
}

// ChangedFiles returns the list of files whose current bytes differ from
// the cached hash snapshot. This deliberately compares content hashes even
// in git repositories: a clean checkout after pull/rebase/commit can have
// no working-tree diff while the on-disk bytes still differ from an older
// repomap cache. Correct symbol spans matter more than the small hashing
// cost.
func ChangedFiles(repoRoot, cacheDir string, entries []FileEntry) []string {
	return ChangedFilesWithProgress(repoRoot, cacheDir, entries, nil)
}

// ChangedFilesWithProgress returns the list of files whose current
// bytes differ from the cached hash snapshot and reports bounded
// progress while hashing large repositories.
//
// The public ChangedFiles wrapper keeps the legacy API. This variant
// lets repo_map surface "checking cache differences" immediately
// after file inventory, which avoids a long silent gap on large repos
// whose cache is hot but whose hash verification still touches tens
// of thousands of files.
func ChangedFilesWithProgress(repoRoot, cacheDir string, entries []FileEntry, progress func(done, total int)) []string {
	cachedHashes := LoadCachedHashes(cacheDir)
	if cachedHashes == nil {
		// no cache → everything is new
		all := make([]string, len(entries))
		for i, e := range entries {
			all[i] = e.RelPath
		}
		reportProgress(progress, len(entries), len(entries))
		return all
	}

	currentFiles := make(map[string]bool)
	changedFlags := make([]bool, len(entries))

	type hashJob struct {
		index int
		entry FileEntry
		old   string
	}
	jobs := make(chan hashJob)
	workers := changedFilesWorkers(len(entries))
	var (
		wg         sync.WaitGroup
		progressMu sync.Mutex
		done       int
	)
	reportOne := func() {
		if progress == nil {
			return
		}
		progressMu.Lock()
		done++
		progress(done, len(entries))
		progressMu.Unlock()
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				newHash, err := hashFile(job.entry.AbsPath)
				if err != nil || newHash != job.old {
					changedFlags[job.index] = true
				}
				reportOne()
			}
		}()
	}

	for i, entry := range entries {
		currentFiles[entry.RelPath] = true
		oldHash, exists := cachedHashes[entry.RelPath]
		if !exists {
			changedFlags[i] = true // new file
			reportOne()
			continue
		}
		jobs <- hashJob{index: i, entry: entry, old: oldHash}
	}
	close(jobs)
	wg.Wait()

	changed := make([]string, 0)
	for i, entry := range entries {
		if changedFlags[i] {
			changed = append(changed, entry.RelPath)
		}
	}

	// deleted files count as changed for graph invalidation
	deleted := make([]string, 0)
	for path := range cachedHashes {
		if !currentFiles[path] {
			deleted = append(deleted, path)
		}
	}
	sort.Strings(deleted)
	changed = append(changed, deleted...)
	return changed
}

func changedFilesWorkers(total int) int {
	if total <= 1 {
		return 1
	}
	n := scanCPUBudget() * 2
	if n < 2 {
		n = 2
	}
	if n > 16 {
		n = 16
	}
	if n > total {
		n = total
	}
	return n
}

func reportProgress(progress func(done, total int), done, total int) {
	if progress != nil {
		progress(done, total)
	}
}

// NeedsFullRescan returns true if the cache is missing or too stale.
func NeedsFullRescan(cacheDir string) bool {
	_, err := os.Stat(filepath.Join(cacheDir, cacheHashesFile))
	return err != nil
}

// ResumableFileInfos returns parse results that an earlier interrupted
// scan already produced and that are still valid for the given entries.
//
// A killed scan installs no manifest, so its chunk directory is
// orphaned — nothing references it, yet its chunk-*.json files hold
// complete parse results. This walks those orphans and returns the
// records still safe to reuse, keyed by RelPath.
//
// "Safe to reuse" means all of: the chunk directory carries a
// compatible resume sentinel (schema + extractor versions), the file
// still exists, and its current content hash and detected language
// match the cached record. Because reuse is content-hash gated, a
// reused FileInfo is byte-identical to what a fresh parse would
// produce — the resulting graph is unchanged.
//
// Returns nil when resume is disabled, when no orphan chunks exist, or
// when nothing survives verification. Hashing runs in parallel.
func ResumableFileInfos(cacheDir string, entries []FileEntry) map[string]*types.FileInfo {
	if !resumeInterruptedScan || cacheDir == "" || len(entries) == 0 {
		return nil
	}
	candidates := loadOrphanChunkFileInfos(cacheDir)
	if len(candidates) == 0 {
		return nil
	}
	return verifyResumableFileInfos(entries, candidates)
}

// liveManifestChunkDir returns the chunk-directory name referenced by a
// currently valid installed manifest, or "" when there is none. Resume
// must exclude this directory: it is the live cache, not an orphan.
func liveManifestChunkDir(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, cacheFileInfosManifestFile))
	if err != nil {
		return ""
	}
	var manifest cacheFileInfosManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return ""
	}
	if !cacheManifestVersionValid(manifest.SchemaVersion, manifest.ExtractorVersions) {
		return ""
	}
	return manifest.ChunkDir
}

// loadOrphanChunkFileInfos collects FileInfo records from every chunk
// directory that is NOT referenced by a valid installed manifest.
func loadOrphanChunkFileInfos(dir string) map[string]*types.FileInfo {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	live := liveManifestChunkDir(dir)
	out := make(map[string]*types.FileInfo)
	for _, e := range dirEntries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "fileinfos.") || !strings.HasSuffix(name, ".d") {
			continue
		}
		if live != "" && name == live {
			continue
		}
		loadChunkDirInto(filepath.Join(dir, name), out)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// loadChunkDirInto merges one chunk directory's FileInfo records into
// out. The directory is skipped wholesale when its resume sentinel is
// missing or version-incompatible. An individual chunk that fails to
// parse (e.g. a partial write from a kill mid-flush) is skipped — the
// files it held simply get re-parsed.
func loadChunkDirInto(chunkDir string, out map[string]*types.FileInfo) {
	metaRaw, err := os.ReadFile(filepath.Join(chunkDir, cacheChunkDirMetaFile))
	if err != nil {
		return
	}
	var meta chunkDirMeta
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return
	}
	if !cacheManifestVersionValid(meta.SchemaVersion, meta.ExtractorVersions) {
		return
	}
	files, err := os.ReadDir(chunkDir)
	if err != nil {
		return
	}
	for _, f := range files {
		name := f.Name()
		if f.IsDir() || !strings.HasPrefix(name, "chunk-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(chunkDir, name))
		if err != nil {
			continue
		}
		var part []*types.FileInfo
		if err := json.Unmarshal(data, &part); err != nil {
			continue
		}
		for _, fi := range part {
			if fi != nil && fi.RelPath != "" {
				out[fi.RelPath] = fi
			}
		}
	}
}

// verifyResumableFileInfos keeps only the candidates whose current file
// bytes (and detected language) still match the cached record. Hashing
// the candidate set is parallelised the same way ChangedFiles is.
func verifyResumableFileInfos(entries []FileEntry, candidates map[string]*types.FileInfo) map[string]*types.FileInfo {
	type job struct {
		entry     FileEntry
		candidate *types.FileInfo
	}
	var jobList []job
	for _, e := range entries {
		if c := candidates[e.RelPath]; c != nil {
			jobList = append(jobList, job{entry: e, candidate: c})
		}
	}
	if len(jobList) == 0 {
		return nil
	}

	verified := make(map[string]*types.FileInfo, len(jobList))
	var mu sync.Mutex
	jobs := make(chan job)
	var wg sync.WaitGroup
	for i := 0; i < changedFilesWorkers(len(jobList)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				// A re-classified extension would have parsed under a
				// different extractor — never reuse across a language
				// change.
				if j.entry.Language != j.candidate.Language {
					continue
				}
				h, err := hashFile(j.entry.AbsPath)
				if err != nil || h != j.candidate.Hash {
					continue
				}
				mu.Lock()
				verified[j.entry.RelPath] = j.candidate
				mu.Unlock()
			}
		}()
	}
	for _, j := range jobList {
		jobs <- j
	}
	close(jobs)
	wg.Wait()

	if len(verified) == 0 {
		return nil
	}
	return verified
}
