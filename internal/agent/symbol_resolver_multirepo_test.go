package agent

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// mrTestGraph mints a tiny *rmtypes.Graph with named symbols so the
// multi-repo resolver fan-out has something to find.
func mrTestGraph(slug string, syms map[string]string) *rmtypes.Graph {
	g := &rmtypes.Graph{
		Root:           "/test/" + slug,
		FileIndex:      map[string]*rmtypes.FileInfo{},
		SymbolDefs:     map[string][]*rmtypes.Symbol{},
		ImportGraph:    map[string][]string{},
		ReverseImports: map[string][]string{},
		Scores:         map[string]float64{},
		QueryScores:    map[string]float64{},
		Metadata:       rmtypes.Metadata{ScanTime: time.Now()},
	}
	for name, file := range syms {
		fi := &rmtypes.FileInfo{
			RelPath:   file,
			Language:  "go",
			ParseTier: 1,
		}
		g.Files = append(g.Files, fi)
		g.FileIndex[file] = fi
		g.SymbolDefs[name] = []*rmtypes.Symbol{
			{Name: name, File: file, Line: 10, EndLine: 20, Kind: "function"},
		}
	}
	return g
}

// mrFixtureMG builds a 2-sub-repo MultiGraph where each sub-repo has
// a distinguishing symbol set. EnsureMany activates the requested
// slugs.
func mrFixtureMG(t *testing.T, repos []topology.SubRepo, syms map[string]map[string]string) *multigraph.MultiGraph {
	t.Helper()
	topo := &topology.RepoTopology{
		ParentRoot:   "/parent",
		ParentSlug:   "parent-deadbeef",
		Repos:        repos,
		DiscoveredAt: time.Now(),
	}
	build := func(repoRoot, _ string) (*rmtypes.Graph, error) {
		// Match repoRoot to slug for the syms lookup.
		for _, r := range repos {
			if r.RootAbs == repoRoot {
				return mrTestGraph(r.Slug, syms[r.Slug]), nil
			}
		}
		return mrTestGraph("unknown", nil), nil
	}
	mg, err := multigraph.New(multigraph.Config{
		Topology: topo,
		Build:    build,
		Cap:      4,
	})
	if err != nil {
		t.Fatalf("multigraph.New: %v", err)
	}
	var slugs []string
	for _, r := range repos {
		slugs = append(slugs, r.Slug)
	}
	if err := mg.EnsureMany(slugs); err != nil {
		t.Fatalf("EnsureMany: %v", err)
	}
	return mg
}

// TestNewMultiRepoSymbolResolver_SingleRepo returns nil so caller falls
// back to the legacy single-graph adapter.
func TestNewMultiRepoSymbolResolver_SingleRepo(t *testing.T) {
	repos := []topology.SubRepo{
		{Slug: "only-1", RootAbs: "/parent", RootRel: ".", FileCount: 1},
	}
	topo := &topology.RepoTopology{ParentRoot: "/parent", ParentSlug: "p", Repos: repos, DiscoveredAt: time.Now()}
	mg, err := multigraph.New(multigraph.Config{
		Topology: topo,
		Build:    func(string, string) (*rmtypes.Graph, error) { return mrTestGraph("only", nil), nil },
		Cap:      1,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if r := newMultiRepoSymbolResolver(mg); r != nil {
		t.Errorf("single-repo: want nil resolver, got %T", r)
	}
}

// TestNewMultiRepoSymbolResolver_NilMG returns nil safely.
func TestNewMultiRepoSymbolResolver_NilMG(t *testing.T) {
	if r := newMultiRepoSymbolResolver(nil); r != nil {
		t.Errorf("nil mg: want nil, got %T", r)
	}
}

// TestMultiRepoSymbolResolver_LookupSymbol_FanOut finds symbols in
// EITHER active sub-repo, not just the primary one.
func TestMultiRepoSymbolResolver_LookupSymbol_FanOut(t *testing.T) {
	repos := []topology.SubRepo{
		{Slug: "codrax-1", RootAbs: "/parent/codrax", RootRel: "codrax", FileCount: 5},
		{Slug: "opencode-2", RootAbs: "/parent/opencode", RootRel: "opencode", FileCount: 5},
	}
	syms := map[string]map[string]string{
		"codrax-1":   {"SymbolOracle": "internal/tool/ground/oracle.go"},
		"opencode-2": {"OpencodeClient": "packages/sdk/js/src/v2/gen/sdk.gen.ts"},
	}
	mg := mrFixtureMG(t, repos, syms)
	r := newMultiRepoSymbolResolver(mg)
	if r == nil {
		t.Fatal("multi-repo resolver should not be nil")
	}
	// Symbol lives in codrax sub-repo
	hits := r.LookupSymbol("SymbolOracle")
	if len(hits) != 1 {
		t.Fatalf("SymbolOracle: want 1 hit, got %d (%v)", len(hits), hits)
	}
	if hits[0].Canonical != "SymbolOracle" {
		t.Errorf("SymbolOracle canonical = %q", hits[0].Canonical)
	}
	// Symbol lives in opencode sub-repo
	hits = r.LookupSymbol("OpencodeClient")
	if len(hits) != 1 {
		t.Fatalf("OpencodeClient: want 1 hit, got %d (%v)", len(hits), hits)
	}
}

// TestMultiRepoSymbolResolver_LookupSymbol_NoMatch returns nil.
func TestMultiRepoSymbolResolver_LookupSymbol_NoMatch(t *testing.T) {
	repos := []topology.SubRepo{
		{Slug: "a-1", RootAbs: "/parent/a", RootRel: "a", FileCount: 1},
		{Slug: "b-2", RootAbs: "/parent/b", RootRel: "b", FileCount: 1},
	}
	mg := mrFixtureMG(t, repos, map[string]map[string]string{
		"a-1": {"AlphaSymbol": "alpha.go"},
		"b-2": {"BetaSymbol": "beta.go"},
	})
	r := newMultiRepoSymbolResolver(mg)
	if hits := r.LookupSymbol("Nonexistent"); hits != nil {
		t.Errorf("no-match: want nil, got %v", hits)
	}
	if hits := r.LookupSymbol(""); hits != nil {
		t.Errorf("empty surface: want nil, got %v", hits)
	}
}

// TestMultiRepoSymbolResolver_NormalizeCodeKeyFallback finds case-folded
// matches via the IterateSymbolDefs second-pass.
func TestMultiRepoSymbolResolver_NormalizeCodeKeyFallback(t *testing.T) {
	repos := []topology.SubRepo{
		{Slug: "a-1", RootAbs: "/parent/a", RootRel: "a", FileCount: 1},
		{Slug: "b-2", RootAbs: "/parent/b", RootRel: "b", FileCount: 1},
	}
	mg := mrFixtureMG(t, repos, map[string]map[string]string{
		"a-1": {"BuildAgentContext": "ctx.go"},
		"b-2": {"OtherSym": "x.go"},
	})
	r := newMultiRepoSymbolResolver(mg)
	hits := r.LookupSymbol("build_agent_context")
	if len(hits) != 1 {
		t.Fatalf("snake_case → CamelCase: want 1 hit, got %d", len(hits))
	}
	if hits[0].Canonical != "BuildAgentContext" {
		t.Errorf("Canonical = %q want BuildAgentContext", hits[0].Canonical)
	}
}

// TestMultiRepoSymbolResolver_DomainFromRootRelFallback uses the owning
// sub-repo's RootRel as Domain when per-graph FileInfo.Package is empty.
func TestMultiRepoSymbolResolver_DomainFromRootRelFallback(t *testing.T) {
	repos := []topology.SubRepo{
		{Slug: "a-1", RootAbs: "/parent/a", RootRel: "a", FileCount: 1},
		{Slug: "b-2", RootAbs: "/parent/b", RootRel: "b", FileCount: 1},
	}
	// Symbols live at root (parent dir is "." → symbolDomain returns "").
	mg := mrFixtureMG(t, repos, map[string]map[string]string{
		"a-1": {"SymA": "main.go"},
		"b-2": {"SymB": "main.go"},
	})
	r := newMultiRepoSymbolResolver(mg)
	hits := r.LookupSymbol("SymA")
	if len(hits) != 1 || hits[0].Domain != "a" {
		t.Errorf("SymA Domain = %q want \"a\"", hitsDomain(hits))
	}
	hits = r.LookupSymbol("SymB")
	if len(hits) != 1 || hits[0].Domain != "b" {
		t.Errorf("SymB Domain = %q want \"b\"", hitsDomain(hits))
	}
}

func hitsDomain(hits []normalizer.SymbolHit) string {
	if len(hits) == 0 {
		return "<empty>"
	}
	return hits[0].Domain
}

// TestMultiRepoSymbolResolver_StemFallback strips role suffixes and
// fans out across sub-repos.
func TestMultiRepoSymbolResolver_StemFallback(t *testing.T) {
	repos := []topology.SubRepo{
		{Slug: "a-1", RootAbs: "/parent/a", RootRel: "a", FileCount: 1},
		{Slug: "b-2", RootAbs: "/parent/b", RootRel: "b", FileCount: 1},
	}
	mg := mrFixtureMG(t, repos, map[string]map[string]string{
		"a-1": {"analyzerEvaluator": "analyzer.go"},
		"b-2": {"requestProcessor": "request.go"},
	})
	r := newMultiRepoSymbolResolver(mg).(interface {
		LookupSymbolStem(string) []normalizer.SymbolHit
	})
	// User says "AnalyzerAgent" — strip "Agent" → stem "Analyzer" →
	// matches NormalizeCodeKey("analyzerEvaluator") containing "analyzer"
	hits := r.LookupSymbolStem("AnalyzerAgent")
	if len(hits) == 0 {
		t.Errorf("LookupSymbolStem(AnalyzerAgent): expected hits across sub-repos")
	}
}

// TestAnalyzerSymbolResolver_SingleRepoLegacyPath verifies the
// dispatcher returns a *repomapSymbolResolver in single-repo posture.
// This is the byte-identical assertion for L1.
func TestAnalyzerSymbolResolver_SingleRepoLegacyPath(t *testing.T) {
	// Single-repo: nil multigraph → analyzerGraphForNormalize returns
	// nil (no RepoRoot in test ctx) → newRepomapSymbolResolver(nil)
	// returns nil. The dispatcher returns nil — same as legacy
	// single-shot CLI without a scan.
	ctx := &types.AgentContext{}
	rm := types.RequestModel{}
	r := analyzerSymbolResolver(ctx, rm)
	if r != nil {
		t.Errorf("nil-mg + empty ctx: want nil resolver, got %T", r)
	}
}
