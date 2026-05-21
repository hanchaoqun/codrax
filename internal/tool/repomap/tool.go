package repomap

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/render"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/retrieve"
	ctypes "github.com/hanchaoqun/codrax/internal/types"
)

// forceReclaimMinParseableFiles is the parse-count threshold above
// which fullScan returns parse-phase memory to the OS before building
// the graph. Below it the reclaim is skipped: small repos and REPL
// turns gain nothing and should not pay the FreeOSMemory latency.
const forceReclaimMinParseableFiles = 2000

// RepoMapV2 is the tree-sitter-powered repo map tool.
type RepoMapV2 struct {
	tool.ReadOnly
	tool.NavigationTool
}

type repoMapParams struct {
	Path       string `json:"path"`
	View       string `json:"view,omitempty"`        // overview, file_map, task_map, call_path, edit_impact, semantic_subgraph
	Query      string `json:"query,omitempty"`       // for task_map
	TargetFile string `json:"target_file,omitempty"` // for edit_impact
	EntryPoint string `json:"entry_point,omitempty"` // for call_path
	TopN       int    `json:"top_n,omitempty"`       // max items
}

func (t *RepoMapV2) Name() string { return "repo_map" }
func (t *RepoMapV2) Description() string {
	return "Navigation index for the repository — shows which files, packages, and symbols exist and where they are. " +
		"Use this ONLY to decide where to look next; it is NOT a source of evidence. " +
		"After consulting the map, you MUST read or grep the actual files to obtain facts. " +
		"Supports views: overview (module summary), file_map (symbols per file), " +
		"task_map (relevant subgraph for a query), call_path (dependency chain from entry point), " +
		"edit_impact (what changes to a file would affect), " +
		"semantic_subgraph (topological summary: linear chains, hub files, articulation-point bridges)."
}

func (t *RepoMapV2) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Root path of the repository to analyze"
    },
    "view": {
      "type": "string",
      "enum": ["overview", "file_map", "task_map", "call_path", "edit_impact", "semantic_subgraph"],
      "description": "Type of map to generate (default: overview)"
    },
    "query": {
      "type": "string",
      "description": "Search query for task_map view — matches against file names, symbol names, and docstrings"
    },
    "target_file": {
      "type": "string",
      "description": "File path (relative to repo root) for edit_impact view"
    },
    "entry_point": {
      "type": "string",
      "description": "File path for call_path view (defaults to main/index file)"
    },
    "top_n": {
      "type": "integer",
      "description": "Maximum number of files/items to include (default varies by view)"
    }
  },
  "required": ["path"]
}`)
}

// findFirstDenialFromCtx returns the first TypedDenial in s matching
// `tok`. Mirrors internal/tool's findFirstDenial; duplicated rather
// than imported to avoid the tool→tool/repomap↔tool cycle.
func findFirstDenialFromCtx(s *ctypes.TypedDenialSet, tok string, pathShaped bool) ctypes.TypedDenial {
	if s == nil || tok == "" {
		return ctypes.TypedDenial{}
	}
	for _, d := range s.Denials {
		if pathShaped {
			if d.Token == tok {
				return d
			}
		} else {
			if d.Token == tok {
				return d
			}
		}
	}
	return ctypes.TypedDenial{}
}

func (t *RepoMapV2) Execute(ctx *ctypes.BusContext, params json.RawMessage) (ctypes.ToolResult, error) {
	var p repoMapParams
	if err := json.Unmarshal(params, &p); err != nil {
		return ctypes.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("invalid params: %v", err),
			Timestamp: time.Now(),
		}, err
	}

	if p.View == "" {
		p.View = "overview"
	}

	// L1 negative-knowledge gate (R3 second-axis enforcement):
	// requested target_file / entry_point already shown absent in
	// the current repository. Refuse with the generic per-class
	// reason (no internal pipeline terminology, no fixture-fitted
	// examples).
	if ctx != nil {
		if p.TargetFile != "" && ctx.TypedDenials.IsPathDenied(p.TargetFile) {
			denial := findFirstDenialFromCtx(&ctx.TypedDenials, p.TargetFile, true)
			return ctypes.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   denial.HumanRefusalReason("repo_map"),
				Timestamp: time.Now(),
			}, nil
		}
		if p.EntryPoint != "" && ctx.TypedDenials.IsSymbolDenied(p.EntryPoint) {
			denial := findFirstDenialFromCtx(&ctx.TypedDenials, p.EntryPoint, false)
			return ctypes.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   denial.HumanRefusalReason("repo_map"),
				Timestamp: time.Now(),
			}, nil
		}
	}

	// L1 active-set hard gate (Phase 1.L1, 2026-05-08): in multi-repo
	// posture, refuse parent-wide scans, refuse paths that fall inside
	// inactive sub-repos, and refuse ambiguous bare paths that match
	// multiple active sub-repos. Reached via the
	// types.MultiRepoActiveSetGater interface so the gate's
	// implementation in multigraph stays single-source.
	allowedRoot := ""
	if ctx != nil && ctx.RepoRoot != "" {
		allowedRoot = ctx.RepoRoot
	}
	if ctx != nil {
		if gater, ok := ctx.MultiGraph.(ctypes.MultiRepoActiveSetGater); ok && gater != nil {
			// repo_map points at directories, not single files —
			// pass fileExists=nil so the gate forces the LLM to
			// specify a sub-repo prefix on bare paths.
			gate := gater.ResolveActiveSetPath(ctx, t.Name(), p.Path, nil)
			if !gate.Allowed {
				return ctypes.ToolResult{
					ToolName:  t.Name(),
					Success:   false,
					Summary:   gate.RefusalProse,
					Timestamp: time.Now(),
				}, nil
			}
			p.Path = gate.ResolvedPath
			if ctx.RepoRoot != "" && gate.SubRepoRootRel != "" && gate.SubRepoRootRel != "." {
				allowedRoot = filepath.Join(ctx.RepoRoot, gate.SubRepoRootRel)
			}
		}
	}

	// Resolve LLM-supplied path against ctx.RepoRoot. The LLM treats
	// the repo root as its own CWD ("." = "the repo I'm investigating"),
	// but the codrax process CWD is wherever the user invoked the
	// binary. Without resolution, `repo_map(path=".")` scans the codrax
	// process CWD instead of the user's --repo target — the LLM then
	// faithfully cites content from the wrong tree (Q2 glamour-vs-codrax
	// regression). Empty / "." / relative paths are rooted at
	// ctx.RepoRoot; absolute paths are accepted only when they remain
	// inside the current repository scope. This is a hard safety guard:
	// a model must not be able to make repo_map scan a parent directory
	// or an unrelated absolute path, because repo_map can recurse through
	// very large repositories.
	repoRoot, err := resolveRepoMapRootScoped(p.Path, "", allowedRoot)
	if ctx != nil && ctx.RepoRoot != "" {
		repoRoot, err = resolveRepoMapRootScoped(p.Path, ctx.RepoRoot, allowedRoot)
	}
	if err != nil {
		return ctypes.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   repoMapScopeRefusal(p.Path),
			Timestamp: time.Now(),
		}, nil
	}

	// Build, load, or reuse the graph. The scope check above runs
	// before cache selection and file discovery, so refused paths never
	// reach index.CacheDir / index.ScanFiles. Route through the context
	// facade so an analyzer prewarm stored on Mutable.SearchGraph or a
	// single-repo MultiGraph resident can satisfy this tool call without
	// triggering a second full scan.
	graph, err := GraphFromBusContextOrLoad(ctx, repoRoot, p.Query)
	if err != nil {
		return ctypes.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("scan failed: %v", err),
			Timestamp: time.Now(),
		}, nil
	}

	// Generate the requested view
	viewParams := ViewParams{
		Query:      p.Query,
		TargetFile: p.TargetFile,
		EntryPoint: p.EntryPoint,
		TopN:       p.TopN,
	}
	output := render.GenerateView(graph, p.View, viewParams)

	summary, ref := tool.StoreBlob(ctx, t.Name(), output)
	return ctypes.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}

func resolveRepoMapRootScoped(requestedPath, sessionRoot, allowedRoot string) (string, error) {
	repoRoot := requestedPath
	if sessionRoot != "" {
		switch {
		case repoRoot == "" || repoRoot == ".":
			repoRoot = sessionRoot
		case !filepath.IsAbs(repoRoot):
			repoRoot = filepath.Join(sessionRoot, repoRoot)
		}
	}
	if repoRoot == "" {
		repoRoot = "."
	}
	if allowedRoot != "" {
		if err := ensureRepoMapRootWithin(repoRoot, allowedRoot); err != nil {
			return "", err
		}
	}
	return repoRoot, nil
}

func ensureRepoMapRootWithin(targetRoot, allowedRoot string) error {
	target, err := canonicalRepoMapPath(targetRoot)
	if err != nil {
		return err
	}
	allowed, err := canonicalRepoMapPath(allowedRoot)
	if err != nil {
		return err
	}
	if !repoMapPathWithinRoot(allowed, target) {
		return fmt.Errorf("repo_map path %q resolves outside allowed root %q", targetRoot, allowedRoot)
	}
	return nil
}

func canonicalRepoMapPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if evaluated, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
		evalAbs, absErr := filepath.Abs(evaluated)
		if absErr == nil {
			abs = filepath.Clean(evalAbs)
		}
		return abs, nil
	}
	return canonicalRepoMapExistingPrefix(abs), nil
}

func canonicalRepoMapExistingPrefix(abs string) string {
	cur := abs
	var suffix []string
	for {
		if evaluated, err := filepath.EvalSymlinks(cur); err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				evaluated = filepath.Join(evaluated, suffix[i])
			}
			evalAbs, err := filepath.Abs(evaluated)
			if err != nil {
				return filepath.Clean(evaluated)
			}
			return filepath.Clean(evalAbs)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		suffix = append(suffix, filepath.Base(cur))
		cur = parent
	}
}

func repoMapPathWithinRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func repoMapScopeRefusal(requestedPath string) string {
	display := strings.TrimSpace(requestedPath)
	if display == "" {
		display = "."
	}
	return fmt.Sprintf(
		"repo_map refused: path %q resolves outside the current repository scope. "+
			"repo_map can only scan the configured workspace or active sub-repo; pass `.` "+
			"or a path under the repository root.",
		display,
	)
}

// BuildOrLoadGraph builds or loads a cached repo graph, ranks files
// by the given query, and returns the result. This is the trusted
// low-level API for already-authorized roots (CLI/eval harnesses,
// topology discovery, tests). Model-influenced paths must use
// BuildOrLoadGraphWithin or a context-aware facade so workspace scope
// is checked before cache selection and file discovery.
func BuildOrLoadGraph(repoRoot, query string) (*Graph, error) {
	return buildOrLoadGraph(repoRoot, query)
}

// BuildOrLoadGraphWithin is the scoped graph loader for any caller
// whose repoRoot may come from an agent/tool parameter. It rejects
// parent traversal, absolute paths outside allowedRoot, and symlink
// escapes before index.CacheDir or index.ScanFiles can touch the
// filesystem.
func BuildOrLoadGraphWithin(repoRoot, allowedRoot, query string) (*Graph, error) {
	resolved, err := resolveRepoMapRootScoped(repoRoot, "", allowedRoot)
	if err != nil {
		return nil, err
	}
	return buildOrLoadGraph(resolved, query)
}

func buildOrLoadGraph(repoRoot, query string) (*Graph, error) {
	// Cap the Go runtime to the scan CPU budget for the whole scan —
	// parse, change detection, graph build, ranking and GC — so it
	// leaves the reserved cores free for interactive processes.
	// Restored on return; no-op when repomap_scan_reserve_cpus is 0.
	defer index.ApplyScanGOMAXPROCS()()

	cacheDir := index.CacheDir(repoRoot)

	// Scan files
	inventoryProgress := newRepoMapScanProgress(repoRoot, "", 0, 0)
	inventoryProgress.startPhase(ctypes.RepoMapScanPhaseFileScan, 0)
	entries, err := index.ScanFiles(repoRoot)
	if err != nil {
		inventoryProgress.finish(false, err)
		return nil, fmt.Errorf("file scan: %w", err)
	}
	notifyRepoMapScan(ctypes.RepoMapScanEvent{
		RepoRoot:       repoRoot,
		Phase:          ctypes.RepoMapScanPhaseFileScan,
		Progress:       true,
		OK:             true,
		TotalFiles:     len(entries),
		ParseableFiles: countParseableEntries(entries),
		ParsedFiles:    len(entries),
		ElapsedMs:      time.Since(inventoryProgress.start).Milliseconds(),
	})

	if len(entries) == 0 {
		inventoryProgress.finish(false, fmt.Errorf("no source files found"))
		return nil, fmt.Errorf("no source files found in %s", repoRoot)
	}

	// No cache at all → full scan
	if index.NeedsFullRescan(cacheDir) {
		logging.Info("repo_map: full scan (%d files, no cache)", len(entries))
		progress := newRepoMapScanProgress(repoRoot, ctypes.RepoMapScanFull, len(entries), len(entries))
		return fullScan(repoRoot, cacheDir, entries, query, progress)
	}

	// Detect which files changed
	changeProgress := newRepoMapScanProgress(repoRoot, "", len(entries), 0)
	changeProgress.startPhase(ctypes.RepoMapScanPhaseChangeScan, 0)
	changed := index.ChangedFilesWithProgress(repoRoot, cacheDir, entries, func(done, total int) {
		changeProgress.parsed(done, total)
	})

	// Nothing changed → load from cache directly
	if len(changed) == 0 {
		logging.Info("repo_map: cache hit (%d files, 0 changed)", len(entries))
		progress := newRepoMapScanProgress(repoRoot, ctypes.RepoMapScanCacheHit, len(entries), 0)
		progress.startPhase(ctypes.RepoMapScanPhaseCacheLoad, countParseableEntries(entries))
		return loadFromCache(repoRoot, cacheDir, entries, query, progress)
	}

	// >30% changed → full rescan is faster than incremental
	if float64(len(changed))/float64(len(entries)) > 0.3 {
		logging.Info("repo_map: full rescan (%d files, %d changed >30%%)", len(entries), len(changed))
		progress := newRepoMapScanProgress(repoRoot, ctypes.RepoMapScanFullRescan, len(entries), len(changed))
		return fullScan(repoRoot, cacheDir, entries, query, progress)
	}

	// Incremental: reparse only changed files, merge with cached data
	logging.Info("repo_map: incremental (%d files, %d changed)", len(entries), len(changed))
	progress := newRepoMapScanProgress(repoRoot, ctypes.RepoMapScanIncremental, len(entries), len(changed))
	return incrementalScan(repoRoot, cacheDir, entries, changed, query, progress)
}

func countParseableEntries(entries []FileEntry) int {
	n := 0
	for _, e := range entries {
		if e.Language != "" {
			n++
		}
	}
	return n
}

func loadFromCache(repoRoot, cacheDir string, entries []FileEntry, query string, progress *repoMapScanProgress) (*Graph, error) {
	cached := index.LoadFileInfos(cacheDir)
	if cached == nil {
		// Cache corrupt or missing JSON → fall back to full scan
		if len(entries) == 0 {
			var err error
			entries, err = index.ScanFiles(repoRoot)
			if err != nil {
				return nil, fmt.Errorf("file scan: %w", err)
			}
		}
		if progress != nil {
			progress.mode = ctypes.RepoMapScanFull
			progress.changedFiles = len(entries)
		}
		return fullScan(repoRoot, cacheDir, entries, query, progress)
	}

	if progress != nil {
		progress.parseableFiles = countParseableFileInfos(cached)
		progress.setPhase(ctypes.RepoMapScanPhaseBuildGraph)
	}
	graph := index.BuildGraph(repoRoot, cached)
	if progress != nil {
		progress.setPhase(ctypes.RepoMapScanPhaseRank)
	}
	retrieve.RankGraph(graph, query)
	if progress != nil {
		progress.finish(true, nil)
	}
	return graph, nil
}

func countParseableFileInfos(files []*FileInfo) int {
	n := 0
	for _, fi := range files {
		if fi != nil && fi.Language != "" {
			n++
		}
	}
	return n
}

func incrementalScan(repoRoot, cacheDir string, entries []FileEntry, changed []string, query string, progress *repoMapScanProgress) (*Graph, error) {
	cached := index.LoadFileInfos(cacheDir)
	if cached == nil {
		if progress != nil {
			progress.mode = ctypes.RepoMapScanFull
			progress.changedFiles = len(entries)
		}
		return fullScan(repoRoot, cacheDir, entries, query, progress)
	}

	// Build lookup of cached files by path
	cachedByPath := make(map[string]*FileInfo, len(cached))
	for _, fi := range cached {
		cachedByPath[fi.RelPath] = fi
	}

	// Build set of changed files for fast lookup
	changedSet := make(map[string]bool, len(changed))
	for _, c := range changed {
		changedSet[c] = true
	}

	// Split entries into changed (need reparse) and unchanged (keep cached)
	var toReparse []FileEntry
	currentPaths := make(map[string]bool, len(entries))
	for _, e := range entries {
		currentPaths[e.RelPath] = true
		if changedSet[e.RelPath] {
			toReparse = append(toReparse, e)
		}
	}

	// Parse only changed files in parallel
	var parseable []FileEntry
	var unparseable []FileEntry
	for _, e := range toReparse {
		if e.Language != "" {
			parseable = append(parseable, e)
		} else {
			unparseable = append(unparseable, e)
		}
	}

	progress.startScan(len(parseable))
	var scanErr error
	defer func() {
		progress.finish(scanErr == nil, scanErr)
	}()

	var onProgress func(done, total int)
	if progress != nil {
		onProgress = progress.parsed
	}
	freshInfos := index.ParseFilesWithProgress(parseable, repoRoot, onProgress)
	for _, e := range unparseable {
		freshInfos = append(freshInfos, index.BasicFileInfo(e))
	}

	// Build fresh lookup
	freshByPath := make(map[string]*FileInfo, len(freshInfos))
	for _, fi := range freshInfos {
		freshByPath[fi.RelPath] = fi
	}

	// Merge: for each current file, use fresh if changed, cached otherwise
	merged := make([]*FileInfo, 0, len(entries))
	for _, e := range entries {
		if fi, ok := freshByPath[e.RelPath]; ok {
			merged = append(merged, fi)
		} else if fi, ok := cachedByPath[e.RelPath]; ok {
			merged = append(merged, fi)
		}
		// else: file disappeared between scan and merge, skip
	}

	// Build graph, rank, save
	progress.setPhase(ctypes.RepoMapScanPhaseBuildGraph)
	graph := index.BuildGraph(repoRoot, merged)
	progress.setPhase(ctypes.RepoMapScanPhaseRank)
	retrieve.RankGraph(graph, query)
	progress.setPhase(ctypes.RepoMapScanPhaseCacheWrite)
	if err := index.SaveCache(cacheDir, graph); err != nil {
		logging.Warning("repo_map: cache save failed: %v", err)
	}
	return graph, nil
}

func fullScan(repoRoot, cacheDir string, entries []FileEntry, query string, progress *repoMapScanProgress) (*Graph, error) {
	// Filter to only parseable files (with known language)
	var parseable []FileEntry
	var unparseable []FileEntry
	for _, e := range entries {
		if e.Language != "" {
			parseable = append(parseable, e)
		} else {
			unparseable = append(unparseable, e)
		}
	}

	// Resume: reuse parse results from an earlier scan that was
	// interrupted (e.g. OOM-killed) before it could install its cache
	// manifest. index.ResumableFileInfos content-hash verifies every
	// reused record, so the resulting graph is byte-identical to a
	// full re-parse. Nil when resume is disabled or no orphan chunks
	// exist — degrading cleanly to a from-scratch full scan.
	resumed := index.ResumableFileInfos(cacheDir, parseable)
	var toParse []FileEntry
	var reused []*FileInfo
	if len(resumed) > 0 {
		reused = make([]*FileInfo, 0, len(resumed))
		toParse = make([]FileEntry, 0, len(parseable)-len(resumed))
		for _, e := range parseable {
			if fi := resumed[e.RelPath]; fi != nil {
				reused = append(reused, fi)
			} else {
				toParse = append(toParse, e)
			}
		}
		logging.Info("repo_map: resuming interrupted scan — reused %d of %d source files, %d to re-parse",
			len(reused), len(parseable), len(toParse))
	} else {
		toParse = parseable
	}

	// Parse the remaining files in parallel.
	progress.startScan(len(toParse))
	var scanErr error
	defer func() {
		progress.finish(scanErr == nil, scanErr)
	}()

	var onProgress func(done, total int)
	if progress != nil {
		onProgress = progress.parsed
	}
	var cacheWriter *index.FileInfoCacheWriter
	var cacheSink func(*FileInfo) error
	if cacheDir != "" {
		if w, err := index.NewFileInfoCacheWriter(cacheDir, repoRoot); err != nil {
			logging.Warning("repo_map: cache stream setup failed: %v", err)
		} else {
			cacheWriter = w
			cacheSink = w.Append
		}
	}

	// Stream reused records into the new scan's cache first, so an
	// interruption of THIS scan still leaves them for the next resume:
	// progress converges across repeated interruptions instead of
	// restarting from zero.
	var cacheStreamErr error
	if cacheWriter != nil {
		for _, fi := range reused {
			if cacheStreamErr = cacheWriter.Append(fi); cacheStreamErr != nil {
				break
			}
		}
	}

	parsedInfos, parseStreamErr := index.ParseFilesWithProgressAndSink(toParse, repoRoot, onProgress, cacheSink)
	if cacheStreamErr == nil {
		cacheStreamErr = parseStreamErr
	}

	fileInfos := make([]*FileInfo, 0, len(parseable)+len(unparseable))
	fileInfos = append(fileInfos, reused...)
	fileInfos = append(fileInfos, parsedInfos...)

	// Add unparseable files with basic metadata
	for _, e := range unparseable {
		fi := index.BasicFileInfo(e)
		fileInfos = append(fileInfos, fi)
		if cacheWriter != nil && cacheStreamErr == nil {
			cacheStreamErr = cacheWriter.Append(fi)
		}
	}
	if cacheWriter != nil {
		if cacheStreamErr != nil {
			cacheWriter.Abort()
			logging.Warning("repo_map: streaming fileinfo cache failed: %v", cacheStreamErr)
		} else if err := cacheWriter.Close(); err != nil {
			cacheStreamErr = err
			cacheWriter.Abort()
			logging.Warning("repo_map: streaming fileinfo cache finalize failed: %v", err)
		}
	}

	// Return parse-phase memory to the OS before the graph-build and
	// ranking phases allocate on top of it: the file bytes and
	// tree-sitter ASTs from parsing are all dead here. Gated on a
	// meaningful parse count so small repos / REPL turns pay nothing.
	if len(toParse) >= forceReclaimMinParseableFiles {
		debug.FreeOSMemory()
		logging.Info("repo_map: reclaimed parse-phase memory before graph build (%d source files parsed)", len(toParse))
	}

	// Build graph
	progress.setPhase(ctypes.RepoMapScanPhaseBuildGraph)
	graph := index.BuildGraph(repoRoot, fileInfos)

	// Rank
	progress.setPhase(ctypes.RepoMapScanPhaseRank)
	retrieve.RankGraph(graph, query)

	// Save cache (non-blocking — errors are tolerable)
	progress.setPhase(ctypes.RepoMapScanPhaseCacheWrite)
	var saveErr error
	if cacheStreamErr == nil && cacheWriter != nil {
		saveErr = index.SaveCacheWithoutFileInfos(cacheDir, graph)
	} else {
		saveErr = index.SaveCache(cacheDir, graph)
	}
	if saveErr != nil {
		logging.Warning("repo_map: cache save failed: %v", saveErr)
	}

	return graph, nil
}

// ToolDescription returns a short summary for status messages.
func ToolDescription(view, query string) string {
	switch view {
	case "task_map":
		return fmt.Sprintf("Generating task map for %q", query)
	case "edit_impact":
		return "Analyzing edit impact"
	case "call_path":
		return "Tracing call paths"
	case "file_map":
		return "Generating file map"
	case "semantic_subgraph":
		return "Summarizing semantic subgraphs"
	default:
		return "Generating repository overview"
	}
}
