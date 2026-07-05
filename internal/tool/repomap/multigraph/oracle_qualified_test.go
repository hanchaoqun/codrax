package multigraph

// Fan-out pins for the qualified-name oracle extension (QNO batch,
// 2026-07-05): the multi-repo oracle forwards QualifiedSymbolExists to
// every active sub-repo oracle that implements the extension, merges
// found/minTier like the flat lane, and SKIPS degraded sub-oracles
// (fallbackOracle) instead of approximating — 禁 fuzzy.

import (
	"testing"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// qualifiedStubOracle implements both the base SymbolOracle and the
// optional QualifiedSymbolOracle extension with fixed answers.
type qualifiedStubOracle struct {
	qualifiedFound bool
	qualifiedTier  int
}

func (s *qualifiedStubOracle) SymbolExists(string) (bool, int)     { return false, 0 }
func (s *qualifiedStubOracle) SymbolExistsFlat(string) (bool, int) { return false, 0 }
func (s *qualifiedStubOracle) QualifiedSymbolExists(string) (bool, int) {
	return s.qualifiedFound, s.qualifiedTier
}

func mgWithGraphs(t *testing.T, factory func(*rmtypes.Graph) types.SymbolOracle, slugs ...string) *MultiGraph {
	t.Helper()
	mg := &MultiGraph{active: NewLRU(len(slugs)), oracleFactory: factory}
	for _, slug := range slugs {
		mg.active.Put(slug, &rmtypes.Graph{Root: slug})
	}
	return mg
}

func TestMultiGraphOracle_QualifiedFanOutMergesMinTier(t *testing.T) {
	answers := map[string]*qualifiedStubOracle{
		"repo-a": {qualifiedFound: true, qualifiedTier: 3},
		"repo-b": {qualifiedFound: true, qualifiedTier: 1},
		"repo-c": {qualifiedFound: false},
	}
	mg := mgWithGraphs(t, func(g *rmtypes.Graph) types.SymbolOracle {
		return answers[g.Root]
	}, "repo-a", "repo-b", "repo-c")
	o := mg.Oracle()
	qo, ok := o.(types.QualifiedSymbolOracle)
	if !ok {
		t.Fatal("multigraph oracle must implement types.QualifiedSymbolOracle")
	}
	found, tier := qo.QualifiedSymbolExists("gate.Run")
	if !found || tier != 1 {
		t.Fatalf("fan-out should merge to (true,1); got (%v,%d)", found, tier)
	}
}

func TestMultiGraphOracle_QualifiedSkipsFallbackOracles(t *testing.T) {
	// Nil OracleFactory → fallbackOracle per sub-repo, which does NOT
	// implement the extension: the qualified lane stays an honest miss
	// instead of degrading to bare-name approximation.
	mg := mgWithGraphs(t, nil, "repo-a")
	qo, ok := mg.Oracle().(types.QualifiedSymbolOracle)
	if !ok {
		t.Fatal("multigraph oracle must implement types.QualifiedSymbolOracle even over fallback sub-oracles")
	}
	if found, tier := qo.QualifiedSymbolExists("gate.Run"); found || tier != 0 {
		t.Fatalf("fallback sub-oracles must be skipped → (false,0); got (%v,%d)", found, tier)
	}
	if _, isQ := interface{}(&fallbackOracle{}).(types.QualifiedSymbolOracle); isQ {
		t.Fatal("fallbackOracle must NOT implement QualifiedSymbolOracle — skipping is the honest-miss contract")
	}
}
