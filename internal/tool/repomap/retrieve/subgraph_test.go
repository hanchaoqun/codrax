package retrieve

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// makeGraph builds a minimal types.Graph from an adjacency list
// mapping file → directly-imported files. ReverseImports and
// FileIndex are derived so ComputeChains/Hubs/Bridges can work
// without any extractor or scanner dependency.
func makeGraph(t *testing.T, imports map[string][]string) *types.Graph {
	t.Helper()
	g := &types.Graph{
		FileIndex:      make(map[string]*types.FileInfo),
		ImportGraph:    make(map[string][]string),
		ReverseImports: make(map[string][]string),
	}
	touch := func(f string) {
		if g.FileIndex[f] == nil {
			g.FileIndex[f] = &types.FileInfo{RelPath: f}
		}
	}
	for f, deps := range imports {
		touch(f)
		g.ImportGraph[f] = append(g.ImportGraph[f], deps...)
		for _, d := range deps {
			touch(d)
			g.ReverseImports[d] = append(g.ReverseImports[d], f)
		}
	}
	return g
}

func TestComputeChains_LinearPipeline(t *testing.T) {
	// a → b → c → d → e; only b/c/d are interior (in=out=1).
	// head extends to a, tail extends to e.
	g := makeGraph(t, map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"d"},
		"d": {"e"},
	})
	got := ComputeChains(g, 0)
	if len(got) != 1 {
		t.Fatalf("want 1 chain, got %d: %+v", len(got), got)
	}
	want := []string{"a", "b", "c", "d", "e"}
	if !reflect.DeepEqual(got[0].Files, want) {
		t.Errorf("chain mismatch\n got %v\nwant %v", got[0].Files, want)
	}
}

func TestComputeChains_BranchingKillsChain(t *testing.T) {
	// b has out-degree 2 (imports c AND d), so b is not interior
	// and there is no 3-file chain to emit.
	g := makeGraph(t, map[string][]string{
		"a": {"b"},
		"b": {"c", "d"},
	})
	got := ComputeChains(g, 0)
	if len(got) != 0 {
		t.Errorf("want 0 chains, got %d: %+v", len(got), got)
	}
}

func TestComputeChains_CycleOfInteriorsTerminates(t *testing.T) {
	// Pure 3-cycle where every node has in=out=1. The walker
	// must terminate and not duplicate the cycle in head/tail.
	g := makeGraph(t, map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"a"},
	})
	got := ComputeChains(g, 0)
	// Exactly one chain, containing all three nodes, no dupes.
	if len(got) != 1 {
		t.Fatalf("want 1 chain, got %d: %+v", len(got), got)
	}
	if len(got[0].Files) != 3 {
		t.Errorf("cycle chain should have 3 files, got %v", got[0].Files)
	}
	seen := map[string]bool{}
	for _, f := range got[0].Files {
		if seen[f] {
			t.Errorf("duplicate file %q in chain %v", f, got[0].Files)
		}
		seen[f] = true
	}
}

func TestComputeChains_TooShortDropped(t *testing.T) {
	// a → b, length 2 — not a chain.
	g := makeGraph(t, map[string][]string{
		"a": {"b"},
	})
	if got := ComputeChains(g, 0); len(got) != 0 {
		t.Errorf("2-file path should not be a chain, got %+v", got)
	}
}

func TestComputeChains_SortedByLengthDesc(t *testing.T) {
	g := makeGraph(t, map[string][]string{
		// short chain: p → q → r → s (length 4)
		"p": {"q"},
		"q": {"r"},
		"r": {"s"},
		// long chain: a → b → c → d → e → f (length 6)
		"a": {"b"},
		"b": {"c"},
		"c": {"d"},
		"d": {"e"},
		"e": {"f"},
	})
	got := ComputeChains(g, 0)
	if len(got) != 2 {
		t.Fatalf("want 2 chains, got %d", len(got))
	}
	if len(got[0].Files) < len(got[1].Files) {
		t.Errorf("chains not sorted by length desc: %+v", got)
	}
}

func TestComputeHubs_RankByTotalDegree(t *testing.T) {
	// hub: imported by 3 files, imports 1 → fan-in 3, fan-out 1
	// leaf: imported by nothing, imports hub → fan-in 0, fan-out 1
	g := makeGraph(t, map[string][]string{
		"a":   {"hub"},
		"b":   {"hub"},
		"c":   {"hub"},
		"hub": {"x"},
	})
	got := ComputeHubs(g, 3)
	if len(got) == 0 {
		t.Fatalf("want at least one hub")
	}
	if got[0].File != "hub" {
		t.Errorf("want hub first, got %+v", got[0])
	}
	if got[0].FanIn != 3 || got[0].FanOut != 1 {
		t.Errorf("hub fan-in/out wrong: %+v", got[0])
	}
}

func TestComputeHubs_SkipsIsolates(t *testing.T) {
	g := &types.Graph{
		FileIndex:      map[string]*types.FileInfo{"lonely.go": {RelPath: "lonely.go"}},
		ImportGraph:    map[string][]string{},
		ReverseImports: map[string][]string{},
	}
	if got := ComputeHubs(g, 0); len(got) != 0 {
		t.Errorf("isolated node should not appear: %+v", got)
	}
}

func TestComputeBridges_ArticulationPoint(t *testing.T) {
	// Graph: a — b — c, and d — b — e.
	// b is the articulation point (removing it splits the graph
	// into {a}, {c}, {d}, {e}).
	g := makeGraph(t, map[string][]string{
		"a": {"b"},
		"b": {"c", "e"},
		"d": {"b"},
	})
	got := ComputeBridges(g, 0)
	if len(got) == 0 {
		t.Fatalf("want at least one bridge")
	}
	found := false
	for _, br := range got {
		if br.File == "b" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'b' to be an articulation point, got %+v", got)
	}
}

func TestComputeBridges_Cycle_NoArticulation(t *testing.T) {
	// Triangle a — b — c — a. No node's removal disconnects.
	g := makeGraph(t, map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"a"},
	})
	if got := ComputeBridges(g, 0); len(got) != 0 {
		t.Errorf("cycle should have no articulation points, got %+v", got)
	}
}

func TestComputeBridges_DisjointComponents(t *testing.T) {
	// Two disjoint edges: a — b, c — d. Neither is an
	// articulation (removing either leaves a singleton
	// component, but in the undirected articulation definition
	// a leaf removal doesn't *split* the subgraph — it's a
	// degree-1 node, not a cut vertex).
	g := makeGraph(t, map[string][]string{
		"a": {"b"},
		"c": {"d"},
	})
	if got := ComputeBridges(g, 0); len(got) != 0 {
		t.Errorf("disjoint edges should have no articulations, got %+v", got)
	}
}

func TestComputeBridges_Deterministic(t *testing.T) {
	// Same graph as the articulation test. Repeated calls should
	// return the same ordering.
	g := makeGraph(t, map[string][]string{
		"a": {"b"},
		"b": {"c", "e"},
		"d": {"b"},
	})
	first := ComputeBridges(g, 0)
	for i := 0; i < 3; i++ {
		if got := ComputeBridges(g, 0); !reflect.DeepEqual(got, first) {
			t.Errorf("non-deterministic bridges: run %d = %+v, first = %+v", i, got, first)
		}
	}
}
