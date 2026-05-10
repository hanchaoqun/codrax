package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/gate"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// === B7 (2026-05-10) — end-to-end multi-repo comparison ===
//
// Reconstructs the forensic-anchor scenario from
// /home/chatpp/.codrax/logs/chatpp-7d46dee4/codrax-20260510-011154-000-3930920.log:
//   - workspace has multiple sub-repos
//   - user asks a cross-sub-repo comparison question
//   - analyzer's emitted PrimaryEntities = sub-repo names
//   - sub-topic.Entities are internal symbols owned by each sub-repo
//
// Pre-B-series: this combination tripped subtopic_coherence's R1.3
// (entities don't share with primary) AND R1.5 (asymmetric resolver
// resolution) — both unsatisfiable simultaneously. Six emit_analysis
// retries exhausted the budget across two turns.
//
// Post-B-series: scope projection (B1) + multi-graph resolver (B2) +
// scope-aware coherence (B3) let the cleanest emit shape pass on the
// first attempt.

// e2eFixtureMG mints a 2-sub-repo MultiGraph mirroring the forensic
// scenario: codrax-fixture has rich SymbolDefs for hallucination-
// detection symbols; opencode-fixture has its own client class.
func e2eFixtureMG(t *testing.T) (*multigraph.MultiGraph, *types.AgentContext) {
	t.Helper()
	repos := []topology.SubRepo{
		{Slug: "codrax-fixture-1", RootAbs: "/parent/codrax-fixture", RootRel: "codrax-fixture", FileCount: 100, PrimaryLangs: []string{"go"}},
		{Slug: "opencode-fixture-2", RootAbs: "/parent/opencode-fixture", RootRel: "opencode-fixture", FileCount: 50, PrimaryLangs: []string{"typescript"}},
	}
	syms := map[string]map[string]string{
		"codrax-fixture-1": {
			"SymbolOracle":   "internal/tool/ground/oracle.go",
			"contractCheck":  "internal/contract/check.go",
			"validateInline": "internal/contract/validate.go",
		},
		"opencode-fixture-2": {
			"OpencodeClient": "packages/sdk/sdk.gen.ts",
			"agentClient":    "packages/core/agent.ts",
		},
	}
	build := func(repoRoot, _ string) (*rmtypes.Graph, error) {
		for _, r := range repos {
			if r.RootAbs == repoRoot {
				g := &rmtypes.Graph{
					Root:           repoRoot,
					FileIndex:      map[string]*rmtypes.FileInfo{},
					SymbolDefs:     map[string][]*rmtypes.Symbol{},
					ImportGraph:    map[string][]string{},
					ReverseImports: map[string][]string{},
					Scores:         map[string]float64{},
					QueryScores:    map[string]float64{},
					Metadata:       rmtypes.Metadata{ScanTime: time.Now()},
				}
				for name, file := range syms[r.Slug] {
					fi := &rmtypes.FileInfo{RelPath: file, Language: r.PrimaryLangs[0], ParseTier: 1}
					g.Files = append(g.Files, fi)
					g.FileIndex[file] = fi
					g.SymbolDefs[name] = []*rmtypes.Symbol{{Name: name, File: file, Line: 1, EndLine: 5, Kind: "function"}}
				}
				return g, nil
			}
		}
		t.Fatalf("e2e build: unknown repoRoot %q", repoRoot)
		return nil, nil
	}
	topo := &topology.RepoTopology{
		ParentRoot:   "/parent",
		ParentSlug:   "parent-cafebabe",
		Repos:        repos,
		DiscoveredAt: time.Now(),
	}
	mg, err := multigraph.New(multigraph.Config{
		Topology: topo,
		Build:    build,
		Cap:      4,
	})
	if err != nil {
		t.Fatalf("multigraph.New: %v", err)
	}
	if err := mg.EnsureMany([]string{"codrax-fixture-1", "opencode-fixture-2"}); err != nil {
		t.Fatalf("EnsureMany: %v", err)
	}
	return mg, &types.AgentContext{MultiGraph: mg, RepoRoot: "/parent"}
}

// TestE2E_MultiRepoComparison_GateAccepts — the cleanest legal emit
// shape (sub-topics anchored to sub-repo NAMES via PrimaryScopes)
// MUST pass the coherence gate on the first attempt.
func TestE2E_MultiRepoComparison_GateAccepts(t *testing.T) {
	mg, ctx := e2eFixtureMG(t)
	_ = mg

	// Reconstruct the analyzer-post-emit IR shape: PrimaryEntities
	// = sub-repo names; PrimaryScopes auto-projected from those.
	primaryEntities := []string{"codrax-fixture", "opencode-fixture"}
	primaryScopes := projectPrimaryScopes(ctx, primaryEntities)
	if len(primaryScopes) != 2 {
		t.Fatalf("setup: projection should return 2 scopes, got %v", primaryScopes)
	}

	subs := []types.SubTopic{
		{
			Summary:  "codrax-fixture's hallucination-detection mechanism",
			Entities: []string{"codrax-fixture", "SymbolOracle", "contractCheck"},
		},
		{
			Summary:  "opencode-fixture's hallucination handling",
			Entities: []string{"opencode-fixture", "OpencodeClient"},
		},
	}
	projectSubTopicScopes(ctx, subs)
	for _, st := range subs {
		if len(st.Scopes) == 0 {
			t.Errorf("each sub-topic should have at least one scope projection; got %v", st)
		}
	}

	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{
			Entities:        primaryEntities,
			PrimaryEntities: primaryEntities,
			PrimaryScopes:   primaryScopes,
		},
		SubTopics: subs,
	}
	ir := &types.AnalysisIR{
		Version:        types.AnalysisIRVersion,
		RequestModel:   rm,
		AnswerContract: types.AnswerContract{},
	}

	// Use the multi-repo resolver — same wiring buildAnalysisIR uses.
	resolver := analyzerSymbolResolver(ctx, rm)
	if resolver == nil {
		t.Fatal("multi-graph resolver should not be nil")
	}
	report := gate.RunWith(ir, gate.GlobalThresholds(), "read", gate.RunOptions{Resolver: resolver})

	// Find subtopic_coherence specifically — other gates may fail on
	// this minimal IR (coverage / hypothesis_coverage are
	// orthogonal), but subtopic_coherence MUST pass.
	for _, c := range report.Checks {
		if c.Name == "subtopic_coherence" {
			if !c.Passed {
				t.Errorf("subtopic_coherence MUST pass on the cleanest cross-sub-repo emit; got %+v", c)
			}
			// R1.3 / R1.5 / R1.8 must NOT fire as hard fails
			for _, banned := range []string{
				"R1.3 entity_orphan",
				"R1.5 entity_unresolvable",
			} {
				if strings.Contains(c.Detail, banned) {
					t.Errorf("subtopic_coherence detail must not include %q; got %q", banned, c.Detail)
				}
			}
		}
	}
}

// TestE2E_MultiRepoComparison_OneScopeUnanchoredAdvisory — when the
// LLM only anchors sub-topics to ONE of the two sub-repo scopes,
// R1.8 SOFT fires advisory but the gate still passes.
func TestE2E_MultiRepoComparison_OneScopeUnanchoredAdvisory(t *testing.T) {
	_, ctx := e2eFixtureMG(t)

	primaryEntities := []string{"codrax-fixture", "opencode-fixture"}
	primaryScopes := projectPrimaryScopes(ctx, primaryEntities)

	// Both sub-topics anchor to codrax-fixture; opencode-fixture left out
	subs := []types.SubTopic{
		{Summary: "codrax-fixture topic 1", Entities: []string{"codrax-fixture", "SymbolOracle"}},
		{Summary: "codrax-fixture topic 2", Entities: []string{"codrax-fixture", "contractCheck"}},
	}
	projectSubTopicScopes(ctx, subs)

	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{
			Entities:        primaryEntities,
			PrimaryEntities: primaryEntities,
			PrimaryScopes:   primaryScopes,
		},
		SubTopics: subs,
	}
	ir := &types.AnalysisIR{
		Version:        types.AnalysisIRVersion,
		RequestModel:   rm,
		AnswerContract: types.AnswerContract{},
	}
	resolver := analyzerSymbolResolver(ctx, rm)
	report := gate.RunWith(ir, gate.GlobalThresholds(), "read", gate.RunOptions{Resolver: resolver})
	for _, c := range report.Checks {
		if c.Name != "subtopic_coherence" {
			continue
		}
		if !c.Passed {
			t.Errorf("R1.8 is advisory; subtopic_coherence MUST still pass; got %+v", c)
		}
		if !strings.Contains(c.Detail, "R1.8 scope_anchor_distribution") {
			t.Errorf("expected R1.8 advisory in detail; got %q", c.Detail)
		}
	}
}

// TestE2E_MultiScope_Overview_RendersBothScopes — pre-emit pre-inject
// path: when the question names ≥2 scopes, buildAnalyzerRepoOverview
// renders a per-scope mini task_map. This is the analyzer-overview
// analog of the resolver path B2 fixed.
func TestE2E_MultiScope_Overview_RendersBothScopes(t *testing.T) {
	_, ctx := e2eFixtureMG(t)
	objective := "compare codrax-fixture and opencode-fixture on hallucination prevention"
	scopes := detectScopesFromQuestion(ctx, objective)
	if len(scopes) != 2 {
		t.Fatalf("want 2 scopes detected, got %v", scopes)
	}
	out, primary := buildMultiScopeRepoOverview(ctx, objective, scopes)
	if out == "" {
		t.Fatal("multi-scope overview should be non-empty")
	}
	if primary == nil {
		t.Error("primary graph should be set")
	}
	if !strings.Contains(out, "# Task Map: codrax-fixture") {
		t.Errorf("missing codrax-fixture task_map; got:\n%s", out)
	}
	if !strings.Contains(out, "# Task Map: opencode-fixture") {
		t.Errorf("missing opencode-fixture task_map; got:\n%s", out)
	}
}

// TestE2E_RetryHintSanitization_HidesInternalShapes — gate detail
// strings that carry dotted/Go-style internal token shapes get
// sanitized at the LLM-prompt boundary so the LLM doesn't index
// them as code identifiers (forensic anchor).
func TestE2E_RetryHintSanitization_HidesInternalShapes(t *testing.T) {
	report := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "subtopic_coherence", Passed: false, Detail: "R1.2 predicate_contradiction: predicates.is_cross_component=true but only 0 sub-topic emitted — re-emit emit_analysis with at least 2 sub_topics"},
		},
	}
	hint := composeCoherenceRetryHint(report)
	if strings.Contains(hint, "predicates.is_cross_component") {
		t.Errorf("dotted internal form must NOT leak to LLM; got %q", hint)
	}
	if !strings.Contains(hint, "is_cross_component flag") {
		t.Errorf("expected sanitized prose form; got %q", hint)
	}
	// Tool name emit_analysis stays — the LLM has to call it.
	if !strings.Contains(hint, "emit_analysis") {
		t.Errorf("tool name emit_analysis must survive sanitization; got %q", hint)
	}
}
