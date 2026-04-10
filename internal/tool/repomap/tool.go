package repomap

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// RepoMapV2 is the tree-sitter-powered repo map tool.
type RepoMapV2 struct {
	tool.ReadOnly
}

type repoMapParams struct {
	Path       string `json:"path"`
	View       string `json:"view,omitempty"`        // overview, file_map, task_map, call_path, edit_impact
	Query      string `json:"query,omitempty"`        // for task_map
	TargetFile string `json:"target_file,omitempty"`  // for edit_impact
	EntryPoint string `json:"entry_point,omitempty"`  // for call_path
	TopN       int    `json:"top_n,omitempty"`        // max items
}

func (t *RepoMapV2) Name() string { return "repo_map" }
func (t *RepoMapV2) Description() string {
	return "Generate a rich repository map with symbols, relations, and importance ranking. " +
		"Supports multiple views: overview (module summary), file_map (symbols per file), " +
		"task_map (relevant subgraph for a query), call_path (dependency chain from entry point), " +
		"edit_impact (what changes to a file would affect)."
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
      "enum": ["overview", "file_map", "task_map", "call_path", "edit_impact"],
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

func (t *RepoMapV2) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p repoMapParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("invalid params: %v", err),
			Timestamp: time.Now(),
		}, err
	}

	if p.Path == "" {
		p.Path = "."
	}
	if p.View == "" {
		p.View = "overview"
	}

	repoRoot := p.Path

	// Build or load the graph
	graph, err := buildOrLoadGraph(repoRoot, p.Query)
	if err != nil {
		return types.ToolResult{
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
	output := GenerateView(graph, p.View, viewParams)

	summary, ref := tool.StoreBlob(ctx, t.Name(), output)
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}

func buildOrLoadGraph(repoRoot, query string) (*Graph, error) {
	cacheDir := CacheDir(repoRoot)

	// Scan files
	entries, err := ScanFiles(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("file scan: %w", err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no source files found in %s", repoRoot)
	}

	// Check if we need a full rescan or incremental
	if NeedsFullRescan(cacheDir) {
		return fullScan(repoRoot, cacheDir, entries, query)
	}

	// Check which files changed
	changed := ChangedFiles(repoRoot, cacheDir, entries)
	if len(changed) == 0 {
		// nothing changed — but we still need to rebuild the in-memory graph
		// because our cache is markdown (not serialized Go structs)
		return fullScan(repoRoot, cacheDir, entries, query)
	}

	// For simplicity, if >30% of files changed, do a full rescan
	if float64(len(changed))/float64(len(entries)) > 0.3 {
		return fullScan(repoRoot, cacheDir, entries, query)
	}

	// Incremental: only reparse changed files, merge with cached data
	// For now, do a full rescan since we don't serialize the graph
	// TODO: implement true incremental by parsing only changed files
	return fullScan(repoRoot, cacheDir, entries, query)
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
	fileInfos := ParseFiles(parseable, repoRoot)

	// Add unparseable files with basic metadata
	for _, e := range unparseable {
		fi := &FileInfo{
			RelPath:  e.RelPath,
			Language: "",
			Size:     e.Size,
		}
		if ok, stype := IsSpecialFile(e.RelPath); ok {
			fi.IsSpecial = true
			fi.SpecialType = stype
		}
		fileInfos = append(fileInfos, fi)
	}

	// Build graph
	graph := BuildGraph(repoRoot, fileInfos)

	// Rank
	RankGraph(graph, query)

	// Save cache (non-blocking — errors are tolerable)
	_ = SaveCache(cacheDir, graph)

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
	default:
		return "Generating repository overview"
	}
}
