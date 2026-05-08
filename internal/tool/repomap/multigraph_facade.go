package repomap

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// BuildOrLoadMultiGraph constructs the multi-repo carrier wired to
// BuildOrLoadGraph. The caller supplies a *topology.RepoTopology
// (typically from cmd/root.go::initApp via the orchestrator) and the
// session's effective cap + focus pins. Single-repo topologies get
// the underlying *Graph pre-warmed automatically so callers can
// immediately use mg.Single() / mg.AllGraphs() without an extra
// EnsureLoaded round-trip.
//
// Multi-repo topologies return an empty MultiGraph (no graphs in the
// active LRU yet); the caller is responsible for routing fold +
// EnsureMany before consuming Z (Oracle/Locator) or Y (AllGraphs /
// Files) surfaces.
//
// Returns (nil, error) on nil topology or empty Repos. Wraps
// repomap.NewSymbolOracle / repomap.NewSymbolLocator as the per-graph
// factories so SymbolExistsFlat (Fix E flat index) keeps working
// across the multigraph fan-out.
//
// Refs: docs/design/multi_repo_discovery_and_lazy_load.md §4.5 Phase 4.1.
func BuildOrLoadMultiGraph(topo *topology.RepoTopology, query string, cap int, focusSlugs []string) (*multigraph.MultiGraph, error) {
	if topo == nil {
		return nil, fmt.Errorf("repomap.BuildOrLoadMultiGraph: nil topology")
	}
	cfg := multigraph.Config{
		Topology:       topo,
		Build:          BuildOrLoadGraph,
		Query:          query,
		Cap:            cap,
		FocusSlugs:     focusSlugs,
		OracleFactory:  func(g *rmtypes.Graph) types.SymbolOracle { return NewSymbolOracle(g) },
		LocatorFactory: func(g *rmtypes.Graph) types.SymbolLocator { return NewSymbolLocator(g) },
	}
	mg, err := multigraph.New(cfg)
	if err != nil {
		return nil, err
	}
	if mg.IsSingle() {
		// Pre-warm so single-repo callers see byte-identical
		// behaviour to the legacy BuildOrLoadGraph path with one
		// fewer round-trip on the first access.
		if _, err := mg.Single(); err != nil {
			return nil, fmt.Errorf("repomap.BuildOrLoadMultiGraph: single-repo prewarm: %w", err)
		}
	}
	return mg, nil
}

// SubRepoSnapshotsFromTopology copies topology.SubRepo entries into
// the BusContext-friendly types.SubRepoSnapshot shape. types/ stays
// free of import cycles on topology by virtue of this conversion
// living in repomap (which already imports both).
func SubRepoSnapshotsFromTopology(topo *topology.RepoTopology) []types.SubRepoSnapshot {
	if topo == nil || len(topo.Repos) == 0 {
		return nil
	}
	out := make([]types.SubRepoSnapshot, len(topo.Repos))
	for i, sr := range topo.Repos {
		out[i] = types.SubRepoSnapshot{
			Slug:             sr.Slug,
			RootAbs:          sr.RootAbs,
			RootRel:          sr.RootRel,
			GitMode:          sr.GitMode,
			PrimaryLangs:     append([]string(nil), sr.PrimaryLangs...),
			PrimaryLangsTier: sr.PrimaryLangsTier,
			FileCount:        sr.FileCount,
		}
	}
	return out
}

// MultiGraphFromContext casts ctx.MultiGraph back to a concrete
// *multigraph.MultiGraph reference. Returns nil when the field is
// unset or carries an unexpected type — callers fall back to legacy
// BuildOrLoadGraph(ctx.RepoRoot, query) in that case.
//
// Lives in repomap (not types) so types/ stays free of an import
// dependency on multigraph.
func MultiGraphFromContext(ctx *types.BusContext) *multigraph.MultiGraph {
	if ctx == nil {
		return nil
	}
	mg, _ := ctx.MultiGraph.(*multigraph.MultiGraph)
	return mg
}

// MultiGraphFromAgentContext mirrors MultiGraphFromContext for the
// AgentContext shape used by per-agent dispatch.
func MultiGraphFromAgentContext(ctx *types.AgentContext) *multigraph.MultiGraph {
	if ctx == nil {
		return nil
	}
	mg, _ := ctx.MultiGraph.(*multigraph.MultiGraph)
	return mg
}
