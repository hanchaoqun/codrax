package repomap

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/retrieve"
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
		WaitNotifier: func(ev types.RepoMapScanEvent) {
			notifyRepoMapScan(ev)
		},
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
// callers that want a *Graph at the analyze / explore stages.
//
// Resolution order:
//  1. ctx.MultiGraph populated AND single-repo posture → mg.Single()
//     returns the byte-equivalent legacy graph.
//  2. ctx.MultiGraph populated AND repoRoot exactly names a topology
//     sub-repo root → load that sub-repo graph. This precise path
//     signal always beats the migration fallback below; repo_map
//     calls with path="repo-a" and path="repo-b" must never collapse
//     to the same primary graph.
//  3. ctx.MultiGraph populated AND repoRoot is empty or the parent
//     workspace root → return the historical best-effort primary
//     graph. This exists only for raw consumers that have not yet
//     migrated to per-sub-repo routing (design §11).
//  4. ctx.MultiGraph nil → legacy repomap.BuildOrLoadGraph(repoRoot, query)
//     direct call.
//
// Used by the 5 BuildOrLoadGraph callers identified in design §11
// (analyzer.go:342/1672/1771, keyword_search.go:667, sub_explorer.go:366)
// and any future caller that wants the same migration semantics. New
// precise tool paths should prefer an exact sub-repo root and must
// not rely on the parent-root primary fallback.
func GraphFromBusContextOrLoad(ctx *types.BusContext, repoRoot, query string) (*Graph, error) {
	if mg := MultiGraphFromContext(ctx); mg != nil {
		if mg.IsSingle() {
			return graphFromSingleMultiGraphRootForBus(ctx, mg, repoRoot, query)
		}
		if g, ok, err := graphFromMultiGraphExactRoot(mg, repoRoot, query); ok || err != nil {
			return g, err
		}
		if topo := mg.Topology(); topo != nil && len(topo.Repos) > 0 {
			if multiGraphPrimaryFallbackAllowed(mg, repoRoot) {
				logging.Warning("repo_map: multi-repo caller requested parent graph; using primary sub-repo compatibility fallback")
				return rankedLoadedGraph(mg, pickPrimarySubRepo(mg, topo).Slug, query)
			}
		}
	}
	if ctx != nil && ctx.Mutable != nil {
		if g, ok := reusableSearchGraph(repoRoot, ctx.SearchGraph, query); ok {
			return g, nil
		}
		if g, ok := reusableSearchGraph(repoRoot, ctx.Mutable.SearchGraph(), query); ok {
			return g, nil
		}
		if ctx.RepoRoot != "" {
			if g, ok, err := projectedGraphFromBusContext(ctx, ctx.RepoRoot, repoRoot, query); ok || err != nil {
				return g, err
			}
		}
	} else if ctx != nil {
		if g, ok := reusableSearchGraph(repoRoot, ctx.SearchGraph, query); ok {
			return g, nil
		}
		if ctx.RepoRoot != "" {
			if g, ok, err := projectedGraphFromBusContext(ctx, ctx.RepoRoot, repoRoot, query); ok || err != nil {
				return g, err
			}
		}
	}
	if repoRoot == "" {
		return nil, fmt.Errorf("repomap: no MultiGraph and empty repoRoot")
	}
	if ctx != nil && ctx.RepoRoot != "" {
		return BuildOrLoadGraphWithin(repoRoot, ctx.RepoRoot, query)
	}
	return BuildOrLoadGraph(repoRoot, query)
}

// GraphFromAgentContextOrLoad mirrors GraphFromBusContextOrLoad for
// the AgentContext shape.
func GraphFromAgentContextOrLoad(ctx *types.AgentContext, repoRoot, query string) (*Graph, error) {
	if mg := MultiGraphFromAgentContext(ctx); mg != nil {
		if mg.IsSingle() {
			return graphFromSingleMultiGraphRootForAgent(ctx, mg, repoRoot, query)
		}
		if g, ok, err := graphFromMultiGraphExactRoot(mg, repoRoot, query); ok || err != nil {
			return g, err
		}
		if topo := mg.Topology(); topo != nil && len(topo.Repos) > 0 {
			if multiGraphPrimaryFallbackAllowed(mg, repoRoot) {
				logging.Warning("repo_map: multi-repo agent requested parent graph; using primary sub-repo compatibility fallback")
				return rankedLoadedGraph(mg, pickPrimarySubRepo(mg, topo).Slug, query)
			}
		}
	}
	if ctx != nil {
		if g, ok := reusableSearchGraph(repoRoot, ctx.SearchGraph, query); ok {
			return g, nil
		}
		if ctx.Mutable != nil {
			if g, ok := reusableSearchGraph(repoRoot, ctx.Mutable.SearchGraph(), query); ok {
				return g, nil
			}
			if ctx.RepoRoot != "" {
				if g, ok, err := projectedGraphFromAgentContext(ctx, ctx.RepoRoot, repoRoot, query); ok || err != nil {
					return g, err
				}
			}
		}
	}
	if repoRoot == "" {
		return nil, fmt.Errorf("repomap: no MultiGraph and empty repoRoot")
	}
	if ctx != nil && ctx.RepoRoot != "" {
		return BuildOrLoadGraphWithin(repoRoot, ctx.RepoRoot, query)
	}
	return BuildOrLoadGraph(repoRoot, query)
}

func graphFromSingleMultiGraphRoot(mg *multigraph.MultiGraph, repoRoot, query string) (*Graph, error) {
	if mg == nil {
		return nil, fmt.Errorf("repomap: nil MultiGraph")
	}
	if repoRoot == "" || sameRepoMapRoot(mg.Root(), repoRoot) {
		return mg.Single()
	}
	return BuildOrLoadGraphWithin(repoRoot, mg.Root(), query)
}

func graphFromSingleMultiGraphRootForBus(ctx *types.BusContext, mg *multigraph.MultiGraph, repoRoot, query string) (*Graph, error) {
	if mg == nil {
		return nil, fmt.Errorf("repomap: nil MultiGraph")
	}
	if repoRoot == "" || sameRepoMapRoot(mg.Root(), repoRoot) {
		return mg.Single()
	}
	if g, ok, err := projectedGraphFromBusContext(ctx, mg.Root(), repoRoot, query); ok || err != nil {
		return g, err
	}
	if g, ok, err := projectedGraphFromActiveSingle(mg, repoRoot, query); ok || err != nil {
		return g, err
	}
	return BuildOrLoadGraphWithin(repoRoot, mg.Root(), query)
}

func graphFromSingleMultiGraphRootForAgent(ctx *types.AgentContext, mg *multigraph.MultiGraph, repoRoot, query string) (*Graph, error) {
	if mg == nil {
		return nil, fmt.Errorf("repomap: nil MultiGraph")
	}
	if repoRoot == "" || sameRepoMapRoot(mg.Root(), repoRoot) {
		return mg.Single()
	}
	if g, ok, err := projectedGraphFromAgentContext(ctx, mg.Root(), repoRoot, query); ok || err != nil {
		return g, err
	}
	if g, ok, err := projectedGraphFromActiveSingle(mg, repoRoot, query); ok || err != nil {
		return g, err
	}
	return BuildOrLoadGraphWithin(repoRoot, mg.Root(), query)
}

func projectedGraphFromActiveSingle(mg *multigraph.MultiGraph, repoRoot, query string) (*Graph, bool, error) {
	if mg == nil || !mg.IsSingle() || repoRoot == "" || sameRepoMapRoot(mg.Root(), repoRoot) {
		return nil, false, nil
	}
	for _, base := range mg.AllGraphs() {
		projected, err := projectGraphToRoot(base, mg.Root(), repoRoot, query)
		if err != nil {
			return nil, false, err
		}
		if projected != nil {
			logging.Info("repo_map: projected scoped graph from active in-memory graph (%d files)", projected.Metadata.FileCount)
			return projected, true, nil
		}
	}
	return nil, false, nil
}

func graphFromMultiGraphExactRoot(mg *multigraph.MultiGraph, repoRoot, query string) (*Graph, bool, error) {
	if mg == nil || repoRoot == "" {
		return nil, false, nil
	}
	topo := mg.Topology()
	if topo == nil {
		return nil, false, nil
	}
	for i := range topo.Repos {
		sr := topo.Repos[i]
		if !sameRepoMapRoot(sr.RootAbs, repoRoot) {
			continue
		}
		g, err := rankedLoadedGraph(mg, sr.Slug, query)
		return g, true, err
	}
	return nil, false, nil
}

func multiGraphPrimaryFallbackAllowed(mg *multigraph.MultiGraph, repoRoot string) bool {
	if mg == nil {
		return false
	}
	if repoRoot == "" {
		return true
	}
	return sameRepoMapRoot(mg.Root(), repoRoot)
}

func rankedLoadedGraph(mg *multigraph.MultiGraph, slug, query string) (*Graph, error) {
	g, err := mg.EnsureLoaded(slug)
	if err != nil {
		return nil, err
	}
	clone := cloneGraphForRanking(g)
	retrieve.RankGraph(clone, query)
	return clone, nil
}

func reusableSearchGraph(repoRoot string, handle any, query string) (*Graph, bool) {
	g, ok := handle.(*Graph)
	if !ok || g == nil || !sameRepoMapRoot(g.Root, repoRoot) {
		return nil, false
	}
	clone := cloneGraphForRanking(g)
	retrieve.RankGraph(clone, query)
	logging.Info("repo_map: reused in-memory graph (%d files)", clone.Metadata.FileCount)
	return clone, true
}

func cloneGraphForRanking(g *Graph) *Graph {
	if g == nil {
		return nil
	}
	// Field-by-field shallow clone: Graph carries an unexported
	// sync.Once-guarded memo that must not be copied by value (the
	// clone shares the same immutable symbol data, so it rebuilds its
	// own memo lazily if ever asked).
	clone := &Graph{
		Root:           g.Root,
		Files:          g.Files,
		FileIndex:      g.FileIndex,
		SymbolDefs:     g.SymbolDefs,
		SymbolByID:     g.SymbolByID,
		MethodIndex:    g.MethodIndex,
		ImportGraph:    g.ImportGraph,
		ReverseImports: g.ReverseImports,
		RankIndex:      g.RankIndex,
		Metadata:       g.Metadata,
	}
	clone.Scores = make(map[string]float64, len(g.Scores))
	clone.QueryScores = make(map[string]float64, len(g.QueryScores))
	return clone
}

func sameRepoMapRoot(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ca, errA := canonicalRepoMapPath(a)
	cb, errB := canonicalRepoMapPath(b)
	if errA != nil || errB != nil {
		return false
	}
	return ca == cb
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
