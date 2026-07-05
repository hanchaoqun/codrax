package multigraph

import (
	"strings"
	"sync"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// multiGraphOracle implements types.SymbolOracle by fanning out
// across every active sub-repo's per-graph oracle. Lazy-builds the
// per-graph oracle on first use of each sub-repo (sync.Once gate
// keeps the build single-threaded). The wrapped oracle is reused
// for the rest of the MultiGraph's lifetime — sub-repos that get
// LRU-evicted lose their cached oracle along with their *Graph
// reference (GC'd).
//
// Behaviour contract (matches types.SymbolOracle):
//   - SymbolExists / SymbolExistsFlat return (true, minTier) when
//     ANY active sub-repo's oracle reports found; minTier is the
//     minimum (most reliable) across all matching sub-repos.
//   - Returns (false, 0) when no active sub-repo matches.
//   - Inactive sub-repos are NOT consulted — the caller observes
//     the partial-typed-lane state via MultiGraph.PartialTypedLane()
//     and discloses it in the LLM-facing summary (R3 red line).
// cachedPerGraphOracle pairs the oracle with the graph it was built
// for: an LRU eviction + reload gives the slug a NEW *Graph, and the
// stale oracle would otherwise keep answering from (and retaining)
// the evicted snapshot.
type cachedPerGraphOracle struct {
	g      *rmtypes.Graph
	oracle types.SymbolOracle
}

type multiGraphOracle struct {
	mg *MultiGraph

	once   sync.Once
	cached map[string]cachedPerGraphOracle
	mu     sync.Mutex // guards cached map mutations beyond the Once
}

// fallbackOracle wraps a *Graph using only its built-in SymbolExists
// method. Used when Config.OracleFactory is nil — SymbolExistsFlat
// degrades to SymbolExists (at least it doesn't crash).
type fallbackOracle struct {
	graph *rmtypes.Graph
}

func (o *fallbackOracle) SymbolExists(name string) (bool, int) {
	if o == nil || o.graph == nil {
		return false, 0
	}
	return o.graph.SymbolExists(strings.TrimSpace(name))
}

func (o *fallbackOracle) SymbolExistsFlat(name string) (bool, int) {
	// No flat index in fallback mode — best-effort exact match.
	return o.SymbolExists(name)
}

// perGraphOracle returns the (cached) SymbolOracle for slug. Builds
// the oracle on first call. Returns nil when the slug is not in the
// active set (caller should skip).
func (o *multiGraphOracle) perGraphOracle(slug string, g *rmtypes.Graph) types.SymbolOracle {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cached == nil {
		o.cached = make(map[string]cachedPerGraphOracle)
	}
	if c, ok := o.cached[slug]; ok && c.g == g {
		return c.oracle
	}
	var oracle types.SymbolOracle
	if o.mg.oracleFactory != nil {
		oracle = o.mg.oracleFactory(g)
	} else {
		oracle = &fallbackOracle{graph: g}
	}
	o.cached[slug] = cachedPerGraphOracle{g: g, oracle: oracle}
	return oracle
}

// SymbolExists fans out across active sub-repos.
func (o *multiGraphOracle) SymbolExists(name string) (bool, int) {
	if o == nil || o.mg == nil {
		return false, 0
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false, 0
	}
	graphs := o.mg.AllGraphs()
	if len(graphs) == 0 {
		return false, 0
	}
	found := false
	minTier := 0
	for slug, g := range graphs {
		if g == nil {
			continue
		}
		sub := o.perGraphOracle(slug, g)
		if sub == nil {
			continue
		}
		ok, tier := sub.SymbolExists(name)
		if !ok {
			continue
		}
		found = true
		if minTier == 0 || tier < minTier {
			minTier = tier
		}
	}
	return found, minTier
}

// QualifiedSymbolExists fans out the qualified-name existence check
// (types.QualifiedSymbolOracle, QNO batch 2026-07-05). Sub-oracles
// that do not implement the optional extension — the degraded
// fallbackOracle used when Config.OracleFactory is nil — are skipped:
// a qualified spelling stays an honest miss there rather than being
// approximated (禁 fuzzy). Same found / minTier merge semantics as
// SymbolExists / SymbolExistsFlat.
func (o *multiGraphOracle) QualifiedSymbolExists(name string) (bool, int) {
	if o == nil || o.mg == nil {
		return false, 0
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false, 0
	}
	graphs := o.mg.AllGraphs()
	if len(graphs) == 0 {
		return false, 0
	}
	found := false
	minTier := 0
	for slug, g := range graphs {
		if g == nil {
			continue
		}
		sub := o.perGraphOracle(slug, g)
		qsub, ok := sub.(types.QualifiedSymbolOracle)
		if !ok {
			continue
		}
		ok, tier := qsub.QualifiedSymbolExists(name)
		if !ok {
			continue
		}
		found = true
		if minTier == 0 || tier < minTier {
			minTier = tier
		}
	}
	return found, minTier
}

// Compile-time: the fan-out oracle carries the optional qualified
// extension so top-level consumers can type-assert it regardless of
// single- vs multi-repo posture.
var _ types.QualifiedSymbolOracle = (*multiGraphOracle)(nil)

// SymbolExistsFlat fans out the case-form-aware existence check.
func (o *multiGraphOracle) SymbolExistsFlat(name string) (bool, int) {
	if o == nil || o.mg == nil {
		return false, 0
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false, 0
	}
	graphs := o.mg.AllGraphs()
	if len(graphs) == 0 {
		return false, 0
	}
	found := false
	minTier := 0
	for slug, g := range graphs {
		if g == nil {
			continue
		}
		sub := o.perGraphOracle(slug, g)
		if sub == nil {
			continue
		}
		ok, tier := sub.SymbolExistsFlat(name)
		if !ok {
			continue
		}
		found = true
		if minTier == 0 || tier < minTier {
			minTier = tier
		}
	}
	return found, minTier
}
