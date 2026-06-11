package multigraph

import (
	"testing"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

type countingOracle struct{ hits *int }

func (c *countingOracle) SymbolExists(string) (bool, int)     { *c.hits++; return true, 1 }
func (c *countingOracle) SymbolExistsFlat(string) (bool, int) { *c.hits++; return true, 1 }

// An eviction + reload hands the slug a NEW *Graph; the per-slug oracle
// cache must rebuild for the new graph instead of answering from (and
// retaining) the evicted snapshot.
func TestPerGraphOracle_RebuildsWhenGraphIdentityChanges(t *testing.T) {
	builds := 0
	o := &multiGraphOracle{mg: &MultiGraph{oracleFactory: func(g *rmtypes.Graph) types.SymbolOracle {
		builds++
		return &countingOracle{hits: new(int)}
	}}}
	g1 := &rmtypes.Graph{Root: "a"}
	g2 := &rmtypes.Graph{Root: "a-reloaded"}
	first := o.perGraphOracle("repo-a", g1)
	if again := o.perGraphOracle("repo-a", g1); again != first {
		t.Fatal("same graph identity must reuse the cached oracle")
	}
	if builds != 1 {
		t.Fatalf("expected single build for stable graph, got %d", builds)
	}
	rebuilt := o.perGraphOracle("repo-a", g2)
	if rebuilt == first {
		t.Fatal("new graph identity must rebuild the oracle, not serve the evicted snapshot")
	}
	if builds != 2 {
		t.Fatalf("expected rebuild for new graph, got %d builds", builds)
	}
}
