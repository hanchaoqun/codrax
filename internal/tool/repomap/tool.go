package repomap

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/render"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/retrieve"
	ctypes "github.com/hanchaoqun/codrax/internal/types"
)

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
		}
	}

	// Resolve LLM-supplied path against ctx.RepoRoot. The LLM treats
	// the repo root as its own CWD ("." = "the repo I'm investigating"),
	// but the codrax process CWD is wherever the user invoked the
	// binary. Without resolution, `repo_map(path=".")` scans the codrax
	// process CWD instead of the user's --repo target — the LLM then
	// faithfully cites content from the wrong tree (Q2 glamour-vs-codrax
	// regression). Absolute paths from the LLM are honored as-is; an
	// empty / "." / relative path is rooted at ctx.RepoRoot.
	repoRoot := p.Path
	if ctx != nil && ctx.RepoRoot != "" {
		switch {
		case repoRoot == "" || repoRoot == ".":
			repoRoot = ctx.RepoRoot
		case !filepath.IsAbs(repoRoot):
			repoRoot = filepath.Join(ctx.RepoRoot, repoRoot)
		}
	}
	if repoRoot == "" {
		repoRoot = "."
	}

	// Build or load the graph
	graph, err := buildOrLoadGraph(repoRoot, p.Query)
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

// BuildOrLoadGraph builds or loads a cached repo graph, ranks files
// by the given query, and returns the result. Exported for use by
// the keyword search system in the agent package.
func BuildOrLoadGraph(repoRoot, query string) (*Graph, error) {
	return buildOrLoadGraph(repoRoot, query)
}

func buildOrLoadGraph(repoRoot, query string) (*Graph, error) {
	cacheDir := index.CacheDir(repoRoot)

	// Scan files
	entries, err := index.ScanFiles(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("file scan: %w", err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no source files found in %s", repoRoot)
	}

	// No cache at all → full scan
	if index.NeedsFullRescan(cacheDir) {
		logging.Info("repo_map: full scan (%d files, no cache)", len(entries))
		return fullScan(repoRoot, cacheDir, entries, query)
	}

	// Detect which files changed
	changed := index.ChangedFiles(repoRoot, cacheDir, entries)

	// Nothing changed → load from cache directly
	if len(changed) == 0 {
		logging.Info("repo_map: cache hit (%d files, 0 changed)", len(entries))
		return loadFromCache(repoRoot, cacheDir, query)
	}

	// >30% changed → full rescan is faster than incremental
	if float64(len(changed))/float64(len(entries)) > 0.3 {
		logging.Info("repo_map: full rescan (%d files, %d changed >30%%)", len(entries), len(changed))
		return fullScan(repoRoot, cacheDir, entries, query)
	}

	// Incremental: reparse only changed files, merge with cached data
	logging.Info("repo_map: incremental (%d files, %d changed)", len(entries), len(changed))
	return incrementalScan(repoRoot, cacheDir, entries, changed, query)
}

func loadFromCache(repoRoot, cacheDir, query string) (*Graph, error) {
	cached := index.LoadFileInfos(cacheDir)
	if cached == nil {
		// Cache corrupt or missing JSON → fall back to full scan
		entries, err := index.ScanFiles(repoRoot)
		if err != nil {
			return nil, fmt.Errorf("file scan: %w", err)
		}
		return fullScan(repoRoot, cacheDir, entries, query)
	}

	graph := index.BuildGraph(repoRoot, cached)
	retrieve.RankGraph(graph, query)
	return graph, nil
}

func incrementalScan(repoRoot, cacheDir string, entries []FileEntry, changed []string, query string) (*Graph, error) {
	cached := index.LoadFileInfos(cacheDir)
	if cached == nil {
		return fullScan(repoRoot, cacheDir, entries, query)
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

	freshInfos := index.ParseFiles(parseable, repoRoot)
	for _, e := range unparseable {
		fi := &FileInfo{
			RelPath:  e.RelPath,
			Language: "",
			Size:     e.Size,
		}
		if ok, stype := index.IsSpecialFile(e.RelPath); ok {
			fi.IsSpecial = true
			fi.SpecialType = stype
		}
		freshInfos = append(freshInfos, fi)
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
	graph := index.BuildGraph(repoRoot, merged)
	retrieve.RankGraph(graph, query)
	_ = index.SaveCache(cacheDir, graph)
	return graph, nil
}

func fullScan(repoRoot, cacheDir string, entries []FileEntry, query string) (*Graph, error) {
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

	// Parse all files in parallel
	fileInfos := index.ParseFiles(parseable, repoRoot)

	// Add unparseable files with basic metadata
	for _, e := range unparseable {
		fi := &FileInfo{
			RelPath:  e.RelPath,
			Language: "",
			Size:     e.Size,
		}
		if ok, stype := index.IsSpecialFile(e.RelPath); ok {
			fi.IsSpecial = true
			fi.SpecialType = stype
		}
		fileInfos = append(fileInfos, fi)
	}

	// Build graph
	graph := index.BuildGraph(repoRoot, fileInfos)

	// Rank
	retrieve.RankGraph(graph, query)

	// Save cache (non-blocking — errors are tolerable)
	_ = index.SaveCache(cacheDir, graph)

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
