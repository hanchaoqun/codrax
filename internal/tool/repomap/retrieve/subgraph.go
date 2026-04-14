package retrieve

import (
	"sort"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// Phase 5 plan item 8: semantic subgraph output.
//
// This file computes three graph-analytic summaries of a repomap's
// ImportGraph:
//
//   - Chains  — maximal linear import paths whose every intermediate
//               node has in-degree 1 and out-degree 1. They represent
//               pipeline-shaped subsystems (parser → lexer → token,
//               scanner → resolver → graph, …) and are useful as a
//               "read these files in order" hint for newcomers.
//
//   - Hubs    — files with high fan-in (many importers) or fan-out
//               (many dependencies). High-fan-in hubs are typically
//               shared utilities or core types; high-fan-out hubs are
//               typically orchestrators or top-level entry points.
//
//   - Bridges — articulation points in the undirected projection of
//               the ImportGraph. A bridge is a file whose removal
//               increases the number of connected components of the
//               import graph — i.e. a file that structurally joins
//               two otherwise-disjoint subsystems. Computed with
//               Tarjan's algorithm (O(V + E)).
//
// These functions are deterministic and do not mutate the graph.

// Chain is a maximal linear import path. Files are listed in import
// order: Files[0] imports Files[1] imports Files[2] … imports
// Files[len-1]. Length is always ≥ 3 (shorter paths are not
// considered "chains" because every pair of imports is a trivial
// 2-file chain).
type Chain struct {
	Files []string
}

// Hub is a high-degree node in the import graph.
type Hub struct {
	File   string
	FanIn  int // number of files that import this file
	FanOut int // number of files this file imports
}

// Bridge is an articulation point: a file whose removal disconnects
// the undirected import graph.
type Bridge struct {
	File string
	// Degree is the number of distinct undirected neighbours. Used
	// as a secondary ranking signal and to tie-break bridges whose
	// removal would split the graph into equally many components.
	Degree int
}

// ComputeChains returns up to topN maximal linear import chains,
// sorted by length descending with lexical tie-break on the first
// file path for determinism. topN ≤ 0 returns every chain.
//
// Interior nodes have both in-degree 1 and out-degree 1 in the
// global ImportGraph. A chain consists of a contiguous run of
// interior nodes, extended one step on each end to include the
// (possibly non-interior) head predecessor and tail successor.
// Chains of fewer than 3 files are dropped.
func ComputeChains(g *types.Graph, topN int) []Chain {
	if g == nil {
		return nil
	}
	inDeg := make(map[string]int, len(g.FileIndex))
	outDeg := make(map[string]int, len(g.FileIndex))
	for f, deps := range g.ImportGraph {
		outDeg[f] = len(deps)
	}
	for f, imps := range g.ReverseImports {
		inDeg[f] = len(imps)
	}

	interior := func(f string) bool {
		return inDeg[f] == 1 && outDeg[f] == 1
	}

	// Unique predecessor / successor lookups for interior walks.
	// When a node is interior its in- and out-degree are 1, so
	// ReverseImports[f][0] and ImportGraph[f][0] are well-defined.
	pred := func(f string) string {
		if imps := g.ReverseImports[f]; len(imps) == 1 {
			return imps[0]
		}
		return ""
	}
	succ := func(f string) string {
		if deps := g.ImportGraph[f]; len(deps) == 1 {
			return deps[0]
		}
		return ""
	}

	visited := make(map[string]bool)
	var chains []Chain
	// Walk every file in a deterministic order so repeated runs
	// emit the same chain order before sorting.
	files := make([]string, 0, len(g.FileIndex))
	for f := range g.FileIndex {
		files = append(files, f)
	}
	sort.Strings(files)

	for _, f := range files {
		if visited[f] || !interior(f) {
			continue
		}
		// Walk backwards through interior predecessors to find the
		// leftmost interior node in this run. Track a local seen
		// set so a cycle of interiors (a → b → c → a) terminates.
		left := f
		seenBack := map[string]bool{left: true}
		for {
			p := pred(left)
			if p == "" || !interior(p) || visited[p] || seenBack[p] {
				break
			}
			seenBack[p] = true
			left = p
		}
		// Walk forward from left collecting every interior node.
		var interiorRun []string
		cur := left
		for interior(cur) && !visited[cur] {
			visited[cur] = true
			interiorRun = append(interiorRun, cur)
			nxt := succ(cur)
			if nxt == "" {
				break
			}
			cur = nxt
		}
		if len(interiorRun) == 0 {
			continue
		}
		// Extend one step in each direction to include the
		// non-interior anchors (the chain's visible endpoints).
		// Skip the extension when the anchor already sits inside
		// the run, which happens for pure interior cycles.
		inRun := make(map[string]bool, len(interiorRun))
		for _, n := range interiorRun {
			inRun[n] = true
		}
		chain := make([]string, 0, len(interiorRun)+2)
		if head := pred(interiorRun[0]); head != "" && !inRun[head] {
			chain = append(chain, head)
		}
		chain = append(chain, interiorRun...)
		if tail := succ(interiorRun[len(interiorRun)-1]); tail != "" && !inRun[tail] {
			chain = append(chain, tail)
		}
		if len(chain) < 3 {
			continue
		}
		chains = append(chains, Chain{Files: chain})
	}

	sort.SliceStable(chains, func(i, j int) bool {
		if len(chains[i].Files) != len(chains[j].Files) {
			return len(chains[i].Files) > len(chains[j].Files)
		}
		return chains[i].Files[0] < chains[j].Files[0]
	})
	if topN > 0 && len(chains) > topN {
		chains = chains[:topN]
	}
	return chains
}

// ComputeHubs returns up to topN files ranked by total degree
// (FanIn + FanOut), descending. Files with zero total degree are
// skipped. Ties break on FanIn descending, then FanOut descending,
// then lex ascending on file path.
func ComputeHubs(g *types.Graph, topN int) []Hub {
	if g == nil {
		return nil
	}
	hubs := make([]Hub, 0, len(g.FileIndex))
	for f := range g.FileIndex {
		fanIn := len(g.ReverseImports[f])
		fanOut := len(g.ImportGraph[f])
		if fanIn == 0 && fanOut == 0 {
			continue
		}
		hubs = append(hubs, Hub{File: f, FanIn: fanIn, FanOut: fanOut})
	}
	sort.SliceStable(hubs, func(i, j int) bool {
		ti := hubs[i].FanIn + hubs[i].FanOut
		tj := hubs[j].FanIn + hubs[j].FanOut
		if ti != tj {
			return ti > tj
		}
		if hubs[i].FanIn != hubs[j].FanIn {
			return hubs[i].FanIn > hubs[j].FanIn
		}
		if hubs[i].FanOut != hubs[j].FanOut {
			return hubs[i].FanOut > hubs[j].FanOut
		}
		return hubs[i].File < hubs[j].File
	})
	if topN > 0 && len(hubs) > topN {
		hubs = hubs[:topN]
	}
	return hubs
}

// ComputeBridges returns up to topN articulation points in the
// undirected projection of the ImportGraph. Results are sorted by
// Degree descending, then lex ascending on file path.
//
// Uses Tarjan's articulation-point algorithm, iterative form to
// avoid stack blow-up on pathological graphs.
func ComputeBridges(g *types.Graph, topN int) []Bridge {
	if g == nil {
		return nil
	}
	adj := buildUndirectedAdj(g)

	// Deterministic iteration order over start nodes.
	nodes := make([]string, 0, len(adj))
	for n := range adj {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	disc := make(map[string]int, len(adj))
	low := make(map[string]int, len(adj))
	parent := make(map[string]string, len(adj))
	isArt := make(map[string]bool, len(adj))
	timer := 0

	type frame struct {
		u      string
		idx    int // index into adj[u] for next neighbour to visit
		child  int // number of spanning-tree children (for root check)
	}
	stack := make([]*frame, 0, 32)

	for _, start := range nodes {
		if _, seen := disc[start]; seen {
			continue
		}
		timer++
		disc[start] = timer
		low[start] = timer
		parent[start] = ""
		stack = append(stack, &frame{u: start})

		for len(stack) > 0 {
			top := stack[len(stack)-1]
			neigh := adj[top.u]
			if top.idx >= len(neigh) {
				// Pop: fold child low into parent's low and apply
				// the articulation predicate.
				stack = stack[:len(stack)-1]
				if len(stack) > 0 {
					p := stack[len(stack)-1]
					if low[top.u] < low[p.u] {
						low[p.u] = low[top.u]
					}
					if parent[p.u] != "" && low[top.u] >= disc[p.u] {
						isArt[p.u] = true
					}
				}
				continue
			}
			v := neigh[top.idx]
			top.idx++
			if _, seen := disc[v]; !seen {
				timer++
				disc[v] = timer
				low[v] = timer
				parent[v] = top.u
				top.child++
				stack = append(stack, &frame{u: v})
				continue
			}
			if v != parent[top.u] {
				if disc[v] < low[top.u] {
					low[top.u] = disc[v]
				}
			}
		}
		// Root articulation: a DFS-tree root is an articulation
		// point iff it has ≥ 2 spanning-tree children. The child
		// count is recorded on the frame, but the root was popped
		// above — recompute from the parent map instead.
		rootChildren := 0
		for _, p := range parent {
			if p == start {
				rootChildren++
				if rootChildren >= 2 {
					break
				}
			}
		}
		if rootChildren >= 2 {
			isArt[start] = true
		}
	}

	bridges := make([]Bridge, 0, len(isArt))
	for f := range isArt {
		bridges = append(bridges, Bridge{File: f, Degree: len(adj[f])})
	}
	sort.SliceStable(bridges, func(i, j int) bool {
		if bridges[i].Degree != bridges[j].Degree {
			return bridges[i].Degree > bridges[j].Degree
		}
		return bridges[i].File < bridges[j].File
	})
	if topN > 0 && len(bridges) > topN {
		bridges = bridges[:topN]
	}
	return bridges
}

// buildUndirectedAdj produces a deduped undirected adjacency list
// for the ImportGraph. Self-loops are dropped; duplicate edges
// (a file appearing more than once in ImportGraph[f] or symmetric
// ReverseImports entries) are coalesced. Adjacency lists are
// sorted so Tarjan visits neighbours in a deterministic order.
func buildUndirectedAdj(g *types.Graph) map[string][]string {
	adj := make(map[string]map[string]bool, len(g.FileIndex))
	add := func(a, b string) {
		if a == "" || b == "" || a == b {
			return
		}
		if adj[a] == nil {
			adj[a] = make(map[string]bool)
		}
		if adj[b] == nil {
			adj[b] = make(map[string]bool)
		}
		adj[a][b] = true
		adj[b][a] = true
	}
	for f, deps := range g.ImportGraph {
		for _, d := range deps {
			add(f, d)
		}
	}
	// Include isolated nodes so they still get visited as Tarjan
	// root candidates (harmless — they produce no articulation).
	for f := range g.FileIndex {
		if adj[f] == nil {
			adj[f] = make(map[string]bool)
		}
	}
	out := make(map[string][]string, len(adj))
	for u, set := range adj {
		list := make([]string, 0, len(set))
		for v := range set {
			list = append(list, v)
		}
		sort.Strings(list)
		out[u] = list
	}
	return out
}
