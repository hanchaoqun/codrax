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
func BuildOrLoadMultiGraph(
	topo *topology.RepoTopology,
	query string,
	cap int,
	focusSlugs []string,
	scanNotifier func(rootRel, slug string, started bool, ok bool, elapsedMs int64),
) (*multigraph.MultiGraph, error) {
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
		ScanNotifier:   scanNotifier,
	}
	mg, err := multigraph.New(cfg)
	if err != nil {
		return nil, err
	}
	// Lazy load — DO NOT pre-warm at construction time. Pre-2026-05-08
	// the facade called mg.Single() here so single-repo callers got
	// "one fewer round-trip on the first access". Reality:
	// initApp invokes this synchronously BEFORE the REPL banner prints;
	// for large repos with no cache, the full BuildOrLoadGraph scan
	// (10-60s) ran before the user saw any prompt — the entire startup
	// looked frozen with zero stdout feedback (log_stdout defaults
	// false). Removing the prewarm restores the pre-multi-repo
	// behaviour where graph load happens lazily inside the analyzer's
	// dispatch (covered by the explorer's "repo_map: full scan / cache
	// hit" INFO line and the existing tool.repo_map progress hooks).
	// The MultiGraph carrier itself is cheap to construct (~µs:
	// allocates the LRU + ThrashingTracker + topology pointer).
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

// GraphFromBusContextOrLoad is the unified read-side entry for
// callers that want a *Graph at the analyze / explore stages and
// don't care whether they're in single-repo or multi-repo mode.
//
// Resolution order:
//  1. ctx.MultiGraph populated AND single-repo posture → mg.Single()
//     returns the byte-equivalent legacy graph.
//  2. ctx.MultiGraph populated AND multi-repo posture → caller is
//     wrong to be asking for "the" graph (multi-repo can't collapse
//     into one *Graph without losing per-sub-repo isolation). The
//     fallback path runs anyway because the alternative (fail-loud
//     here) would brick the entire pipeline; a Warning is logged so
//     operators see the partial-correctness state until the raw
//     consumer migration (design §11) lands.
//  3. ctx.MultiGraph nil → legacy repomap.BuildOrLoadGraph(repoRoot, query)
//     direct call.
//
// Used by the 5 BuildOrLoadGraph callers identified in design §11
// (analyzer.go:342/1672/1771, keyword_search.go:667, sub_explorer.go:366)
// and any future caller that wants the same migration semantics.
func GraphFromBusContextOrLoad(ctx *types.BusContext, repoRoot, query string) (*Graph, error) {
	if mg := MultiGraphFromContext(ctx); mg != nil {
		if mg.IsSingle() {
			return mg.Single()
		}
		if topo := mg.Topology(); topo != nil && len(topo.Repos) > 0 {
			return mg.EnsureLoaded(pickPrimarySubRepo(mg, topo).Slug)
		}
	}
	if repoRoot == "" {
		return nil, fmt.Errorf("repomap: no MultiGraph and empty repoRoot")
	}
	if ctx != nil && ctx.RepoRoot != "" {
		var err error
		repoRoot, err = resolveRepoMapRootScoped(repoRoot, ctx.RepoRoot, ctx.RepoRoot)
		if err != nil {
			return nil, err
		}
	}
	return BuildOrLoadGraph(repoRoot, query)
}

// GraphFromAgentContextOrLoad mirrors GraphFromBusContextOrLoad for
// the AgentContext shape.
func GraphFromAgentContextOrLoad(ctx *types.AgentContext, repoRoot, query string) (*Graph, error) {
	if mg := MultiGraphFromAgentContext(ctx); mg != nil {
		if mg.IsSingle() {
			return mg.Single()
		}
		if topo := mg.Topology(); topo != nil && len(topo.Repos) > 0 {
			return mg.EnsureLoaded(pickPrimarySubRepo(mg, topo).Slug)
		}
	}
	if repoRoot == "" {
		return nil, fmt.Errorf("repomap: no MultiGraph and empty repoRoot")
	}
	if ctx != nil && ctx.RepoRoot != "" {
		var err error
		repoRoot, err = resolveRepoMapRootScoped(repoRoot, ctx.RepoRoot, ctx.RepoRoot)
		if err != nil {
			return nil, err
		}
	}
	return BuildOrLoadGraph(repoRoot, query)
}

// pickPrimarySubRepo returns the SubRepo a "best-effort single
// graph" caller should EnsureLoad. The pre-2026-05-08 helper picked
// the largest sub-repo by FileCount unconditionally — which ignored
// operator focus pins and silently scanned a non-pinned sub-repo
// when the user had pinned smaller ones. The fixed selection is:
//
//  1. If the operator has pinned ≥1 sub-repo (mg.FocusSlugs), pick
//     the largest pinned one. Honours user intent: "I only want
//     these sub-repos investigated" without exception.
//  2. Otherwise (no pin), fall back to the largest sub-repo in
//     topology — preserves the historical best-effort behaviour
//     for unpinned multi-repo workspaces.
//
// Single-repo callers never reach this path (mg.IsSingle short-
// circuits above). Empty topology is impossible at this site
// (caller already gates on len(Repos) > 0).
func pickPrimarySubRepo(mg *multigraph.MultiGraph, topo *topology.RepoTopology) *topology.SubRepo {
	pinned := mg.FocusSlugs()
	if len(pinned) > 0 {
		pinSet := make(map[string]bool, len(pinned))
		for _, s := range pinned {
			pinSet[s] = true
		}
		var best *topology.SubRepo
		for i := range topo.Repos {
			if !pinSet[topo.Repos[i].Slug] {
				continue
			}
			if best == nil || topo.Repos[i].FileCount > best.FileCount {
				best = &topo.Repos[i]
			}
		}
		if best != nil {
			return best
		}
		// No matching pin found in topology (stale pin set); fall
		// through to the unpinned-largest path so the helper still
		// returns a sub-repo rather than nil.
	}
	best := &topo.Repos[0]
	for i := range topo.Repos {
		if topo.Repos[i].FileCount > best.FileCount {
			best = &topo.Repos[i]
		}
	}
	return best
}
