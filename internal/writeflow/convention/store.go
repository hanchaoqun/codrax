package convention

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type Store struct {
	root string
}

func NewStore(root string) *Store {
	return &Store{root: strings.TrimSpace(root)}
}

func Path(root, runID string) string {
	runID = sanitizeRunID(runID)
	if runID == "" {
		runID = "unknown"
	}
	return filepath.Join(root, "workflows", "knowledge", runID, "conventions.json")
}

func (s *Store) Load(runID string) (types.ConventionGraph, error) {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return types.ConventionGraph{}, errors.New("convention store root is empty")
	}
	path := Path(s.root, runID)
	data, err := os.ReadFile(path)
	if err != nil {
		return types.ConventionGraph{}, err
	}
	var graph types.ConventionGraph
	if err := json.Unmarshal(data, &graph); err != nil {
		return types.ConventionGraph{}, err
	}
	return types.NormalizeConventionGraph(graph), nil
}

func (s *Store) MergeAndSave(runID string, graphs ...types.ConventionGraph) (types.ConventionGraph, string, error) {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return types.ConventionGraph{}, "", errors.New("convention store root is empty")
	}
	if existing, err := s.Load(runID); err == nil {
		graphs = append([]types.ConventionGraph{existing}, graphs...)
	} else if !errors.Is(err, os.ErrNotExist) {
		return types.ConventionGraph{}, "", err
	}
	merged := Merge(runID, graphs...)
	path := Path(s.root, runID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return types.ConventionGraph{}, "", err
	}
	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return types.ConventionGraph{}, "", err
	}
	if err := types.AtomicWriteFileSync(path, append(data, '\n'), 0o644); err != nil {
		return types.ConventionGraph{}, "", err
	}
	return merged, path, nil
}

func Merge(runID string, graphs ...types.ConventionGraph) types.ConventionGraph {
	out := types.ConventionGraph{
		GraphID: "convention-graph:" + sanitizeRunID(runID),
		Source:  "convention_store",
	}
	for _, graph := range graphs {
		graph = types.NormalizeConventionGraph(graph)
		if out.BatchID == "" {
			out.BatchID = graph.BatchID
		}
		if out.Goal == "" {
			out.Goal = graph.Goal
		}
		out.Nodes = append(out.Nodes, graph.Nodes...)
	}
	return types.NormalizeConventionGraph(out)
}

func SelectTopN(graph types.ConventionGraph, activePaths []string, activeSymbols []string, limit int) types.ConventionGraph {
	graph = types.NormalizeConventionGraph(graph)
	if limit <= 0 || len(graph.Nodes) <= limit {
		return graph
	}
	pathSet := stringSet(activePaths)
	symbolSet := stringSet(activeSymbols)
	nodes := append([]types.ConventionNode(nil), graph.Nodes...)
	sort.SliceStable(nodes, func(i, j int) bool {
		ri := conventionNodeRank(nodes[i], pathSet, symbolSet)
		rj := conventionNodeRank(nodes[j], pathSet, symbolSet)
		if ri != rj {
			return ri < rj
		}
		return nodes[i].ID < nodes[j].ID
	})
	graph.Nodes = nodes[:limit]
	return types.NormalizeConventionGraph(graph)
}

func conventionNodeRank(node types.ConventionNode, paths, symbols map[string]bool) int {
	if node.Source != "" && paths[normalizeConventionPath(node.Source)] {
		return 0
	}
	if node.AnchorSymbol != "" && symbols[strings.TrimSpace(node.AnchorSymbol)] {
		return 1
	}
	if node.Subject != "" && symbols[strings.TrimSpace(node.Subject)] {
		return 1
	}
	if node.Source != "" {
		for path := range paths {
			if path != "" && strings.HasPrefix(normalizeConventionPath(node.Source), path) {
				return 2
			}
		}
	}
	return 3
}

func stringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = normalizeConventionPath(value)
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func sanitizeRunID(runID string) string {
	runID = strings.TrimSpace(runID)
	var b strings.Builder
	for _, r := range runID {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func normalizeConventionPath(raw string) string {
	path := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}
