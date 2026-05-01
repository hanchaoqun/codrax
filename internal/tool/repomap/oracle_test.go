package repomap

import (
	"testing"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestSymbolOracle_NilGraph(t *testing.T) {
	o := NewSymbolOracle(nil)
	if found, tier := o.SymbolExists("Foo"); found || tier != 0 {
		t.Errorf("nil graph should return (false, 0); got (%v, %d)", found, tier)
	}
}

func TestSymbolOracle_NotFound(t *testing.T) {
	g := &rmtypes.Graph{
		SymbolDefs: map[string][]*rmtypes.Symbol{},
		FileIndex:  map[string]*rmtypes.FileInfo{},
	}
	o := NewSymbolOracle(g)
	if found, tier := o.SymbolExists("DoesNotExist"); found || tier != 0 {
		t.Errorf("absent symbol should return (false, 0); got (%v, %d)", found, tier)
	}
}

func TestSymbolOracle_FoundAtTier1(t *testing.T) {
	fi := &rmtypes.FileInfo{RelPath: "foo.go", Language: "go", ParseTier: 1}
	sym := &rmtypes.Symbol{Name: "Foo", File: "foo.go"}
	g := &rmtypes.Graph{
		SymbolDefs: map[string][]*rmtypes.Symbol{"Foo": {sym}},
		FileIndex:  map[string]*rmtypes.FileInfo{"foo.go": fi},
	}
	o := NewSymbolOracle(g)
	if found, tier := o.SymbolExists("Foo"); !found || tier != 1 {
		t.Errorf("Foo at Tier 1 should return (true, 1); got (%v, %d)", found, tier)
	}
}

func TestSymbolOracle_PicksLowestTier(t *testing.T) {
	// Same name defined in two files at different parse tiers; oracle
	// should report the LOWEST (=most reliable) tier.
	fi1 := &rmtypes.FileInfo{RelPath: "a.go", Language: "go", ParseTier: 3}
	fi2 := &rmtypes.FileInfo{RelPath: "b.go", Language: "go", ParseTier: 1}
	g := &rmtypes.Graph{
		SymbolDefs: map[string][]*rmtypes.Symbol{
			"Bar": {
				{Name: "Bar", File: "a.go"},
				{Name: "Bar", File: "b.go"},
			},
		},
		FileIndex: map[string]*rmtypes.FileInfo{"a.go": fi1, "b.go": fi2},
	}
	o := NewSymbolOracle(g)
	if found, tier := o.SymbolExists("Bar"); !found || tier != 1 {
		t.Errorf("multi-tier symbol should report min-tier=1; got (%v, %d)", found, tier)
	}
}

func TestSymbolOracle_TrimsWhitespace(t *testing.T) {
	o := NewSymbolOracle(&rmtypes.Graph{
		SymbolDefs: map[string][]*rmtypes.Symbol{},
		FileIndex:  map[string]*rmtypes.FileInfo{},
	})
	if found, _ := o.SymbolExists("   "); found {
		t.Error("whitespace-only name should not match")
	}
	if found, _ := o.SymbolExists(""); found {
		t.Error("empty name should not match")
	}
}

func TestSymbolOracle_StaleIndexFallsBackToTier4(t *testing.T) {
	// Defs entry exists but FileInfo missing — the conservative
	// fallback is Tier 4 so callers that filter by tier reject.
	sym := &rmtypes.Symbol{Name: "Stale", File: "missing.go"}
	g := &rmtypes.Graph{
		SymbolDefs: map[string][]*rmtypes.Symbol{"Stale": {sym}},
		FileIndex:  map[string]*rmtypes.FileInfo{}, // no entry
	}
	o := NewSymbolOracle(g)
	found, tier := o.SymbolExists("Stale")
	if !found {
		t.Error("stale-index symbol should report found=true")
	}
	if tier != 4 {
		t.Errorf("stale-index symbol should fall back to tier=4; got %d", tier)
	}
}
