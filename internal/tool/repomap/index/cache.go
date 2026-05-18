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
	if err := os.WriteFile(filepath.Join(w.chunkDir, name), data, 0o644); err != nil {
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

func pruneOldFileInfoChunkDirs(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == keep || !strings.HasPrefix(name, "fileinfos.") || !strings.HasSuffix(name, ".d") {
			continue
		}
		_ = os.RemoveAll(filepath.Join(dir, name))
	}
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
	n := runtime.GOMAXPROCS(0) * 2
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
