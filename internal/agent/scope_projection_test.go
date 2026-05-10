package agent

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// scopeFixtureBuild returns a BuildFunc for tests; it produces a
// minimal graph so EnsureLoaded succeeds. We don't exercise SymbolDefs
// here — that's B2's territory; B1 only needs the topology to be
// active so ActiveSlugSnapshot returns the right slugs.
func scopeFixtureBuild(t *testing.T) multigraph.BuildFunc {
	return func(repoRoot, _ string) (*rmtypes.Graph, error) {
		return &rmtypes.Graph{
			Root:           repoRoot,
			FileIndex:      map[string]*rmtypes.FileInfo{},
			SymbolDefs:     map[string][]*rmtypes.Symbol{},
			ImportGraph:    map[string][]string{},
			ReverseImports: map[string][]string{},
			Scores:         map[string]float64{},
			QueryScores:    map[string]float64{},
			Metadata:       rmtypes.Metadata{ScanTime: time.Now()},
		}, nil
	}
}

func scopeFixtureMG(t *testing.T, repos []topology.SubRepo, ensure []string) *multigraph.MultiGraph {
	t.Helper()
	topo := &topology.RepoTopology{
		ParentRoot:   "/parent",
		ParentSlug:   "parent-deadbeef",
		Repos:        repos,
		DiscoveredAt: time.Now(),
	}
	mg, err := multigraph.New(multigraph.Config{
		Topology: topo,
		Build:    scopeFixtureBuild(t),
		Cap:      4,
	})
	if err != nil {
		t.Fatalf("multigraph.New: %v", err)
	}
	if len(ensure) > 0 {
		if err := mg.EnsureMany(ensure); err != nil {
			t.Fatalf("EnsureMany: %v", err)
		}
	}
	return mg
}

func ctxWithMG(mg *multigraph.MultiGraph) *types.AgentContext {
	return &types.AgentContext{MultiGraph: mg}
}

// TestProjectPrimaryScopes_NilContext returns nil safely (no panic).
func TestProjectPrimaryScopes_NilContext(t *testing.T) {
	if got := projectPrimaryScopes(nil, []string{"codrax"}); got != nil {
		t.Errorf("nil ctx: want nil, got %v", got)
	}
}

// TestProjectPrimaryScopes_EmptyEntities returns nil immediately.
func TestProjectPrimaryScopes_EmptyEntities(t *testing.T) {
	mg := scopeFixtureMG(t, []topology.SubRepo{
		{Slug: "a-1234", RootAbs: "/parent/a", RootRel: "a", FileCount: 1},
		{Slug: "b-5678", RootAbs: "/parent/b", RootRel: "b", FileCount: 1},
	}, []string{"a-1234", "b-5678"})
	ctx := ctxWithMG(mg)
	if got := projectPrimaryScopes(ctx, nil); got != nil {
		t.Errorf("empty entities: want nil, got %v", got)
	}
	if got := projectPrimaryScopes(ctx, []string{}); got != nil {
		t.Errorf("empty entities slice: want nil, got %v", got)
	}
}

// TestProjectPrimaryScopes_SingleRepo returns nil — IsSingle short-circuit.
func TestProjectPrimaryScopes_SingleRepo(t *testing.T) {
	mg := scopeFixtureMG(t, []topology.SubRepo{
		{Slug: "only-1234", RootAbs: "/parent", RootRel: ".", GitMode: "dir", FileCount: 1},
	}, nil)
	if !mg.IsSingle() {
		t.Fatal("expected IsSingle() = true")
	}
	got := projectPrimaryScopes(ctxWithMG(mg), []string{"foo", "bar"})
	if got != nil {
		t.Errorf("single-repo: want nil scopes, got %v", got)
	}
}

// TestProjectPrimaryScopes_NilMultiGraph returns nil — caller fast path.
func TestProjectPrimaryScopes_NilMultiGraph(t *testing.T) {
	got := projectPrimaryScopes(&types.AgentContext{}, []string{"codrax"})
	if got != nil {
		t.Errorf("nil mg: want nil, got %v", got)
	}
}

// TestProjectPrimaryScopes_MultiRepo_TwoMatches lights up both sub-repos.
func TestProjectPrimaryScopes_MultiRepo_TwoMatches(t *testing.T) {
	mg := scopeFixtureMG(t, []topology.SubRepo{
		{Slug: "codrax-1234", RootAbs: "/parent/codrax", RootRel: "codrax", FileCount: 1},
		{Slug: "opencode-5678", RootAbs: "/parent/opencode", RootRel: "opencode", FileCount: 1},
		{Slug: "test-9999", RootAbs: "/parent/test", RootRel: "test", FileCount: 1},
	}, []string{"codrax-1234", "opencode-5678"})
	got := projectPrimaryScopes(ctxWithMG(mg), []string{"codrax", "opencode", "SymbolOracle"})
	if len(got) != 2 {
		t.Fatalf("want 2 scopes, got %v", got)
	}
	have := map[string]bool{got[0]: true, got[1]: true}
	if !have["codrax"] || !have["opencode"] {
		t.Errorf("want {codrax, opencode}, got %v", got)
	}
}

// TestProjectPrimaryScopes_OnlyActiveScopesMatch — inactive sub-repo
// not lit even when its name appears in entities.
func TestProjectPrimaryScopes_OnlyActiveScopesMatch(t *testing.T) {
	mg := scopeFixtureMG(t, []topology.SubRepo{
		{Slug: "codrax-1234", RootAbs: "/parent/codrax", RootRel: "codrax", FileCount: 1},
		{Slug: "inactive-5678", RootAbs: "/parent/inactive", RootRel: "inactive", FileCount: 1},
	}, []string{"codrax-1234"}) // only codrax is active
	got := projectPrimaryScopes(ctxWithMG(mg), []string{"codrax", "inactive"})
	if len(got) != 1 || got[0] != "codrax" {
		t.Errorf("want [codrax], got %v", got)
	}
}

// TestProjectPrimaryScopes_NormalizeCodeKeyEquivalence — case + dash + underscore folded.
func TestProjectPrimaryScopes_NormalizeCodeKeyEquivalence(t *testing.T) {
	mg := scopeFixtureMG(t, []topology.SubRepo{
		{Slug: "my-project-1", RootAbs: "/parent/my-project", RootRel: "my-project", FileCount: 1},
	}, []string{"my-project-1"})
	cases := []string{"my-project", "my_project", "MyProject", "MY-PROJECT"}
	for _, c := range cases {
		got := projectPrimaryScopes(ctxWithMG(mg), []string{c})
		if len(got) != 1 || got[0] != "my-project" {
			t.Errorf("input %q: want [my-project], got %v", c, got)
		}
	}
}

// TestProjectPrimaryScopes_SkipsRootRelDot — single-repo "." sub-repo
// (degenerate case, normally caught by IsSingle but defence-in-depth).
func TestProjectPrimaryScopes_SkipsRootRelDot(t *testing.T) {
	// Force a non-IsSingle topology with one "." entry + one real entry.
	// (Edge case — wouldn't happen in production but the helper has to
	// be defensive.)
	mg := scopeFixtureMG(t, []topology.SubRepo{
		{Slug: "dot-1", RootAbs: "/parent", RootRel: ".", FileCount: 1},
		{Slug: "real-2", RootAbs: "/parent/real", RootRel: "real", FileCount: 1},
	}, []string{"real-2"})
	got := projectPrimaryScopes(ctxWithMG(mg), []string{".", "real"})
	if len(got) != 1 || got[0] != "real" {
		t.Errorf("want [real] (dot skipped), got %v", got)
	}
}

// TestProjectPrimaryScopes_DeduplicatesIdenticalEntities
func TestProjectPrimaryScopes_DeduplicatesIdenticalEntities(t *testing.T) {
	mg := scopeFixtureMG(t, []topology.SubRepo{
		{Slug: "codrax-1", RootAbs: "/parent/codrax", RootRel: "codrax", FileCount: 1},
	}, []string{"codrax-1"})
	got := projectPrimaryScopes(ctxWithMG(mg), []string{"codrax", "Codrax", "codrax", "CODRAX"})
	if len(got) != 1 || got[0] != "codrax" {
		t.Errorf("dedup: want [codrax], got %v", got)
	}
}

// TestProjectSubTopicScopes_PerSubTopic — each sub-topic gets its own
// projection; sub-topics with no scope match keep Scopes==nil.
func TestProjectSubTopicScopes_PerSubTopic(t *testing.T) {
	mg := scopeFixtureMG(t, []topology.SubRepo{
		{Slug: "a-1", RootAbs: "/parent/a", RootRel: "a", FileCount: 1},
		{Slug: "b-2", RootAbs: "/parent/b", RootRel: "b", FileCount: 1},
	}, []string{"a-1", "b-2"})
	subs := []types.SubTopic{
		{Summary: "topic 1", Entities: []string{"a", "Symbol1"}},
		{Summary: "topic 2", Entities: []string{"Symbol2", "Symbol3"}}, // no scope match
		{Summary: "topic 3", Entities: []string{"b"}},
	}
	projectSubTopicScopes(ctxWithMG(mg), subs)
	if len(subs[0].Scopes) != 1 || subs[0].Scopes[0] != "a" {
		t.Errorf("topic 1: want Scopes=[a], got %v", subs[0].Scopes)
	}
	if subs[1].Scopes != nil {
		t.Errorf("topic 2: want Scopes=nil, got %v", subs[1].Scopes)
	}
	if len(subs[2].Scopes) != 1 || subs[2].Scopes[0] != "b" {
		t.Errorf("topic 3: want Scopes=[b], got %v", subs[2].Scopes)
	}
	// Entities lane MUST be untouched (COPY, not MOVE).
	if len(subs[0].Entities) != 2 || subs[0].Entities[0] != "a" || subs[0].Entities[1] != "Symbol1" {
		t.Errorf("topic 1 entities mutated: %v", subs[0].Entities)
	}
}

// TestProjectSubTopicScopes_EmptyInput is a no-op.
func TestProjectSubTopicScopes_EmptyInput(t *testing.T) {
	projectSubTopicScopes(&types.AgentContext{}, nil)
	projectSubTopicScopes(&types.AgentContext{}, []types.SubTopic{})
	// no panic == pass
}

// TestProjectPrimaryScopes_CopySemantics_SymbolNameSameAsScope — the
// degenerate but legal case where a single-repo Go module contains a
// top-level identifier named exactly like a sub-repo slug. COPY
// semantics ensures PrimaryEntities still flows downstream unchanged
// while PrimaryScopes captures the scope mention.
func TestProjectPrimaryScopes_CopySemantics_SymbolNameSameAsScope(t *testing.T) {
	mg := scopeFixtureMG(t, []topology.SubRepo{
		{Slug: "codrax-1", RootAbs: "/parent/codrax", RootRel: "codrax", FileCount: 1},
		{Slug: "test-2", RootAbs: "/parent/test", RootRel: "test", FileCount: 1},
	}, []string{"codrax-1", "test-2"})
	originalEntities := []string{"codrax", "RealSymbol"}
	got := projectPrimaryScopes(ctxWithMG(mg), originalEntities)
	if len(got) != 1 || got[0] != "codrax" {
		t.Errorf("want [codrax], got %v", got)
	}
	// Caller's slice reference must be untouched (COPY contract).
	if len(originalEntities) != 2 || originalEntities[0] != "codrax" || originalEntities[1] != "RealSymbol" {
		t.Errorf("original entities mutated: %v", originalEntities)
	}
}
