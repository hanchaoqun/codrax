package compiler

import (
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// EnsureReadStageNodes projects implicit read pipeline stages into the
// analyzer-authored TaskGraph. The analyzer remains the sole writer: this runs
// inside deterministic IR construction, before downstream scheduler consumers
// see the graph.
func EnsureReadStageNodes(g *types.TaskGraph) {
	if g == nil || len(g.Nodes) == 0 {
		return
	}
	ensureSourceInventoryReprobeNode(g)
	if hasNodeType(*g, types.NodeExtract) {
		return
	}
	finalIdx := firstNodeIndexByType(*g, types.NodeFinalize)
	if finalIdx < 0 {
		return
	}
	finalID := g.Nodes[finalIdx].ID
	extractID := uniqueExtractNodeID(*g, finalID)
	extract := types.TaskNode{
		ID:        extractID,
		Type:      types.NodeExtract,
		Objective: "Distill accepted evidence into answer-ready symbols, hypothesis verdicts, and structured support.",
		Inputs: []string{
			"evidence_items",
			"answer_chains",
			"aggregate_facts",
		},
		Outputs: []string{
			"answer_symbols",
			"hypothesis_verdicts",
			"structured_support",
		},
		EntryConditions: []types.Criterion{
			{Kind: types.CritExtractInputReady},
		},
	}
	g.Nodes = append(g.Nodes, types.TaskNode{})
	copy(g.Nodes[finalIdx+1:], g.Nodes[finalIdx:])
	g.Nodes[finalIdx] = extract
	rewiredIncoming := false
	for i := range g.Edges {
		if g.Edges[i].To != finalID || g.Edges[i].EdgeType != types.EdgeHardDependency {
			continue
		}
		g.Edges[i].To = extractID
		rewiredIncoming = true
	}
	if rewiredIncoming || !taskGraphHasEdge(*g, extractID, finalID, types.EdgeHardDependency) {
		g.Edges = append(g.Edges, types.TaskEdge{
			From:     extractID,
			To:       finalID,
			EdgeType: types.EdgeHardDependency,
		})
	}
	g.ExecutionPolicy.CriticalPath = insertBeforeInPath(g.ExecutionPolicy.CriticalPath, extractID, finalID)
}

func ensureSourceInventoryReprobeNode(g *types.TaskGraph) {
	if g == nil || hasSourceInventoryReprobeNode(*g) {
		return
	}
	finalIdx := firstNodeIndexByType(*g, types.NodeFinalize)
	if finalIdx < 0 {
		return
	}
	finalID := g.Nodes[finalIdx].ID
	reprobe := types.TaskNode{
		ID:        uniqueSourceInventoryReprobeNodeID(*g, finalID),
		Type:      types.NodeProbe,
		Objective: "Refresh source-inventory source-class coverage when the current source-class universe is incomplete.",
		Inputs:    []string{"source_class_universe"},
		Outputs:   []string{"source_inventory_observation", "source_classes"},
		EntryConditions: []types.Criterion{
			{Kind: types.CritSourceClassUniverseIncomplete},
		},
		Optional:   true,
		OneShot:    true,
		MaxRetries: 1,
	}
	g.Nodes = append(g.Nodes, types.TaskNode{})
	copy(g.Nodes[finalIdx+1:], g.Nodes[finalIdx:])
	g.Nodes[finalIdx] = reprobe
	if !taskGraphHasEdge(*g, reprobe.ID, finalID, types.EdgeSoftDependency) {
		g.Edges = append(g.Edges, types.TaskEdge{
			From:     reprobe.ID,
			To:       finalID,
			EdgeType: types.EdgeSoftDependency,
		})
	}
}

func hasSourceInventoryReprobeNode(g types.TaskGraph) bool {
	for _, n := range g.Nodes {
		if !n.Optional || n.Type != types.NodeProbe {
			continue
		}
		for _, c := range n.EntryConditions {
			if c.Kind == types.CritSourceClassUniverseIncomplete {
				return true
			}
		}
	}
	return false
}

func hasNodeType(g types.TaskGraph, typ types.TaskNodeType) bool {
	return firstNodeIndexByType(g, typ) >= 0
}

func firstNodeIndexByType(g types.TaskGraph, typ types.TaskNodeType) int {
	for i := range g.Nodes {
		if g.Nodes[i].Type == typ {
			return i
		}
	}
	return -1
}

func uniqueExtractNodeID(g types.TaskGraph, finalID string) string {
	base := strings.TrimSpace(finalID)
	switch {
	case strings.HasSuffix(base, "finalize"):
		base = strings.TrimSuffix(base, "finalize") + "extract"
	case base != "":
		base = base + "_extract"
	default:
		base = "n_extract"
	}
	if !nodeIDExists(g, base) {
		return base
	}
	for i := 0; ; i++ {
		candidate := base + "_" + strconv.Itoa(i)
		if !nodeIDExists(g, candidate) {
			return candidate
		}
	}
}

func uniqueSourceInventoryReprobeNodeID(g types.TaskGraph, finalID string) string {
	base := strings.TrimSpace(finalID)
	switch {
	case strings.HasSuffix(base, "finalize"):
		base = strings.TrimSuffix(base, "finalize") + "source_inventory_reprobe"
	case base != "":
		base = base + "_source_inventory_reprobe"
	default:
		base = "n_source_inventory_reprobe"
	}
	if !nodeIDExists(g, base) {
		return base
	}
	for i := 0; ; i++ {
		candidate := base + "_" + strconv.Itoa(i)
		if !nodeIDExists(g, candidate) {
			return candidate
		}
	}
}

func nodeIDExists(g types.TaskGraph, id string) bool {
	for _, n := range g.Nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

func taskGraphHasEdge(g types.TaskGraph, from, to string, typ types.EdgeType) bool {
	for _, e := range g.Edges {
		if e.From == from && e.To == to && e.EdgeType == typ {
			return true
		}
	}
	return false
}

func insertBeforeInPath(path []string, insertID, beforeID string) []string {
	if insertID == "" || beforeID == "" || containsPathID(path, insertID) {
		return path
	}
	for i, id := range path {
		if id != beforeID {
			continue
		}
		out := make([]string, 0, len(path)+1)
		out = append(out, path[:i]...)
		out = append(out, insertID)
		out = append(out, path[i:]...)
		return out
	}
	return path
}

func containsPathID(path []string, id string) bool {
	for _, existing := range path {
		if existing == id {
			return true
		}
	}
	return false
}
