package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestExpandKeywordsAbbreviations(t *testing.T) {
	tests := []struct {
		input []string
		want  string // at least this keyword should be in output
	}{
		{[]string{"auth"}, "authentication"},
		{[]string{"authentication"}, "auth"},
		{[]string{"config"}, "configuration"},
		{[]string{"configuration"}, "config"},
		{[]string{"ctx"}, "context"},
		{[]string{"exec"}, "execute"},
		{[]string{"eval"}, "evaluate"},
	}
	for _, tt := range tests {
		expanded := expandKeywords(tt.input)
		found := false
		for _, kw := range expanded {
			if strings.ToLower(kw) == tt.want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expandKeywords(%v): expected %q in result %v", tt.input, tt.want, expanded)
		}
	}
}

func TestExpandKeywordsNamingConventions(t *testing.T) {
	expanded := expandKeywords([]string{"sub_agent"})
	// Note: "subagent" (concatenated) is deduplicated with "SubAgent"
	// (CamelCase) because they share the same lowercase form.
	wants := []string{"sub_agent", "SubAgent", "sub-agent"}
	for _, w := range wants {
		found := false
		for _, kw := range expanded {
			if kw == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expandKeywords([sub_agent]): expected %q in result %v", w, expanded)
		}
	}
}

func TestExpandKeywordsNoAbbrevForUnknown(t *testing.T) {
	expanded := expandKeywords([]string{"foobar"})
	// "foobar" has no abbreviation pair, should only get itself
	if len(expanded) != 1 {
		t.Errorf("expandKeywords([foobar]): expected 1 keyword, got %d: %v", len(expanded), expanded)
	}
}

func TestNormalizeSearchPathStripsWindowsRepoRootWithSlashForms(t *testing.T) {
	repoRoot := `C:\Users\ssccv\codrax`
	got := normalizeSearchPath("C:/Users/ssccv/codrax/internal/skill/defaults.go", repoRoot)
	if got != "internal/skill/defaults.go" {
		t.Fatalf("normalizeSearchPath forward-slash Windows path = %q", got)
	}

	got = normalizeSearchPath(`C:\Users\ssccv\codrax\internal\agent\explorer.go`, "C:/Users/ssccv/codrax")
	if got != "internal/agent/explorer.go" {
		t.Fatalf("normalizeSearchPath backslash Windows path = %q", got)
	}
}

func TestDomainBoostFactor_MatchingPackageReturnsBoost(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/types/context.go":     {Package: "types"},
			"internal/tool/blob.go":         {Package: "tool"},
			"internal/orchestrator/main.go": {Package: "orchestrator"},
		},
	}
	// Term's Domain is "types"; only the types file gets the boost.
	got := domainBoostFactor("internal/types/context.go", graph, []string{"types"})
	if got <= 1.0 {
		t.Fatalf("expected boost > 1.0 for matching package, got %v", got)
	}
	got = domainBoostFactor("internal/tool/blob.go", graph, []string{"types"})
	if got != 1.0 {
		t.Fatalf("expected no boost for non-matching package, got %v", got)
	}
}

func TestDomainBoostFactor_EmptyInputsReturnsOne(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/types/context.go": {Package: "types"},
		},
	}
	// No domains, no boost.
	if got := domainBoostFactor("internal/types/context.go", graph, nil); got != 1.0 {
		t.Fatalf("nil domains should return 1.0, got %v", got)
	}
	// Nil graph, no boost.
	if got := domainBoostFactor("internal/types/context.go", nil, []string{"types"}); got != 1.0 {
		t.Fatalf("nil graph should return 1.0, got %v", got)
	}
	// File not in index, no boost.
	if got := domainBoostFactor("foo/unknown.go", graph, []string{"types"}); got != 1.0 {
		t.Fatalf("missing file should return 1.0, got %v", got)
	}
}

func TestDomainBoostFactor_EmptyPackageNotBoosted(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"scripts/build.sh": {Package: ""},
		},
	}
	// Empty hint must never match empty Package (otherwise every
	// package-less file would get boosted).
	if got := domainBoostFactor("scripts/build.sh", graph, []string{""}); got != 1.0 {
		t.Fatalf("empty-package file should not boost on empty hint, got %v", got)
	}
	if got := domainBoostFactor("scripts/build.sh", graph, []string{"scripts"}); got != 1.0 {
		t.Fatalf("empty Package cannot match any hint, got %v", got)
	}
}

func TestDomainBoostFactor_BoostSmallerThanEntityBoost(t *testing.T) {
	// Contract: domain boost is strictly smaller than the weakest
	// entity-boost branch (1.3). Domain is a coarser sibling signal
	// and must never overpower an exact-name entity match.
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"foo.go": {Package: "foo"},
		},
	}
	got := domainBoostFactor("foo.go", graph, []string{"foo"})
	if got >= 1.3 {
		t.Fatalf("domain boost %v must be strictly less than entity boost floor 1.3", got)
	}
}

// T1.2 — keywordSearchFingerprint stability + order-independence.
func TestKeywordSearchFingerprint_StableAndOrderIndependent(t *testing.T) {
	a := keywordSearchFingerprint([]string{"foo", "bar"}, []string{"Baz"}, nil, nil, []string{"pkg"}, []string{"target"}, "same_family_grounded", 30, false, "")
	b := keywordSearchFingerprint([]string{"bar", "foo"}, []string{"Baz"}, nil, nil, []string{"pkg"}, []string{"target"}, "same_family_grounded", 30, false, "")
	if a != b {
		t.Fatalf("fingerprint is sensitive to keyword order: a=%q b=%q", a, b)
	}
}

func TestKeywordSearchFingerprint_DistinguishesInputs(t *testing.T) {
	base := keywordSearchFingerprint([]string{"foo"}, []string{"Baz"}, nil, nil, []string{"pkg"}, []string{"target"}, "same_family_grounded", 30, false, "")
	cases := map[string]string{
		"different keyword":           keywordSearchFingerprint([]string{"qux"}, []string{"Baz"}, nil, nil, []string{"pkg"}, []string{"target"}, "same_family_grounded", 30, false, ""),
		"different entity":            keywordSearchFingerprint([]string{"foo"}, []string{"Other"}, nil, nil, []string{"pkg"}, []string{"target"}, "same_family_grounded", 30, false, ""),
		"different mentioned entity":  keywordSearchFingerprint([]string{"foo"}, []string{"Baz"}, []string{"Mentioned"}, nil, []string{"pkg"}, []string{"target"}, "same_family_grounded", 30, false, ""),
		"different primary entities":  keywordSearchFingerprint([]string{"foo"}, []string{"Baz"}, nil, []string{"Primary"}, []string{"pkg"}, []string{"target"}, "same_family_grounded", 30, false, ""),
		"different domain":            keywordSearchFingerprint([]string{"foo"}, []string{"Baz"}, nil, nil, []string{"other"}, []string{"target"}, "same_family_grounded", 30, false, ""),
		"different exact target":      keywordSearchFingerprint([]string{"foo"}, []string{"Baz"}, nil, nil, []string{"pkg"}, []string{"other"}, "same_family_grounded", 30, false, ""),
		"different exact policy":      keywordSearchFingerprint([]string{"foo"}, []string{"Baz"}, nil, nil, []string{"pkg"}, []string{"target"}, "grounded_only", 30, false, ""),
		"different maxFiles":          keywordSearchFingerprint([]string{"foo"}, []string{"Baz"}, nil, nil, []string{"pkg"}, []string{"target"}, "same_family_grounded", 20, false, ""),
		"different exact-anchor mode": keywordSearchFingerprint([]string{"foo"}, []string{"Baz"}, nil, nil, []string{"pkg"}, []string{"target"}, "same_family_grounded", 30, true, ""),
		"different source scope":      keywordSearchFingerprint([]string{"foo"}, []string{"Baz"}, nil, nil, []string{"pkg"}, []string{"target"}, "same_family_grounded", 30, false, "test:aux-principal"),
		"empty all":                   keywordSearchFingerprint(nil, nil, nil, nil, nil, nil, "", 0, false, ""),
	}
	for name, fp := range cases {
		if fp == base {
			t.Errorf("%s should change fingerprint but matched base %q", name, fp)
		}
	}
}

func TestKeywordSearchScopedFingerprint_DistinguishesRepoAndActiveSet(t *testing.T) {
	base := keywordSearchFingerprint([]string{"foo"}, []string{"Baz"}, nil, nil, nil, nil, "", 20, false, "")
	ctxA := &types.AgentContext{
		RepoRoot:        "/tmp/repo-a",
		PendingSubRepos: []string{"inactive-a"},
		SubRepos: []types.SubRepoSnapshot{
			{Slug: "core", RootRel: "."},
			{Slug: "tools", RootRel: "tools"},
		},
	}
	ctxB := &types.AgentContext{
		RepoRoot:        "/tmp/repo-b",
		PendingSubRepos: []string{"inactive-a"},
		SubRepos: []types.SubRepoSnapshot{
			{Slug: "core", RootRel: "."},
			{Slug: "tools", RootRel: "tools"},
		},
	}
	ctxC := &types.AgentContext{
		RepoRoot:        "/tmp/repo-a",
		PendingSubRepos: []string{"inactive-b"},
		SubRepos: []types.SubRepoSnapshot{
			{Slug: "core", RootRel: "."},
			{Slug: "tools", RootRel: "tools"},
		},
	}
	ctxD := &types.AgentContext{
		RepoRoot:        "/tmp/repo-a",
		PendingSubRepos: []string{"inactive-a"},
		SubRepos: []types.SubRepoSnapshot{
			{Slug: "core", RootRel: "."},
			{Slug: "alt", RootRel: "alt"},
		},
	}
	a := keywordSearchScopedFingerprint(ctxA, base)
	for name, fp := range map[string]string{
		"repo":     keywordSearchScopedFingerprint(ctxB, base),
		"pending":  keywordSearchScopedFingerprint(ctxC, base),
		"topology": keywordSearchScopedFingerprint(ctxD, base),
	} {
		if fp == a {
			t.Fatalf("scoped fingerprint did not distinguish %s: %q", name, fp)
		}
	}
}

func TestShouldDeprioritizeAuxiliaryExactHit(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetKind:   types.SubjectConfigKey,
		TargetLabel:  "config key",
		Targets:      []string{"explore_mid_loop_hint_budget"},
		AllowAbsence: true,
	}
	if !shouldDeprioritizeAuxiliaryExactHit("internal/skill/analysis_contract.go", contract) {
		t.Fatal("analysis_contract auxiliary exact hit should be deprioritized")
	}
	if shouldDeprioritizeAuxiliaryExactHit("internal/config/runtime.go", contract) {
		t.Fatal("production config file should not be deprioritized")
	}
}

func TestShouldDeprioritizeAuxiliaryBySourceScope(t *testing.T) {
	testRole := types.ClassifySourcePathRole("tests/cangjie/feature_test.cj")
	if !shouldDeprioritizeAuxiliaryBySourceScope(testRole, &types.SourceScopeProfile{RequestedScope: types.SourceScopeProduction}) {
		t.Fatal("production scope should down-rank auxiliary test paths")
	}
	if shouldDeprioritizeAuxiliaryBySourceScope(testRole, &types.SourceScopeProfile{RequestedScope: types.SourceScopeTest, IncludeAuxiliaryAsPrincipal: true}) {
		t.Fatal("explicit test scope should not down-rank test paths")
	}
	if shouldDeprioritizeAuxiliaryBySourceScope(types.SourcePathRoleProduction, nil) {
		t.Fatal("production path must not be down-ranked as auxiliary")
	}
}

func TestGrepFiles_RootOnlyNoiseDoesNotHideNestedProjectDirs(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "logs"), 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "internal", "logs"), 0o755); err != nil {
		t.Fatalf("mkdir internal/logs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "logs", "root_noise.go"), []byte("SEARCH_SCOPE_TOKEN\n"), 0o644); err != nil {
		t.Fatalf("write root logs file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "internal", "logs", "nested_logger.go"), []byte("SEARCH_SCOPE_TOKEN\n"), 0o644); err != nil {
		t.Fatalf("write nested logs file: %v", err)
	}

	got := grepFiles("SEARCH_SCOPE_TOKEN", repo, false)
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, "root_noise.go") {
		t.Fatalf("root logs/ should be filtered from keyword search, got %q", joined)
	}
	if !strings.Contains(joined, "nested_logger.go") {
		t.Fatalf("nested internal/logs/ should remain searchable, got %q", joined)
	}
}

// rmTestGraphWithScore mints a tiny Graph with one keyword-scored
// file so multi-repo aggregation has something to surface.
func rmTestGraphWithScore(slug, file string, score float64) *rmtypes.Graph {
	g := &rmtypes.Graph{
		Root:           "/test/" + slug,
		FileIndex:      map[string]*rmtypes.FileInfo{},
		SymbolDefs:     map[string][]*rmtypes.Symbol{},
		ImportGraph:    map[string][]string{},
		ReverseImports: map[string][]string{},
		Scores:         map[string]float64{file: score},
		QueryScores:    map[string]float64{file: score},
		Metadata:       rmtypes.Metadata{ScanTime: time.Now()},
	}
	fi := &rmtypes.FileInfo{RelPath: file, Language: "go", ParseTier: 1}
	g.Files = []*rmtypes.FileInfo{fi}
	g.FileIndex[file] = fi
	return g
}

// TestRepoMapRank_SkipsPendingSubRepos is the regression test for the
// 2026-05-16 finalize loop where keyword_search's multi-repo aggregator
// surfaced files from LRU-resident-but-pending sub-repos. The phantom
// files entered Phase1Ranking → CGEC forced-read pending list, the
// explorer could not satisfy them (file-system tools refused on the
// same active set), and the only escape was an
// evidence_floor_waiver(no_repo_intersection) that then incorrectly
// engaged finalizer's observation-only citation policy.
func TestRepoMapRank_SkipsPendingSubRepos(t *testing.T) {
	repos := []topology.SubRepo{
		{Slug: "active-repo", RootAbs: "/parent/active-repo", RootRel: "active-repo", FileCount: 1},
		{Slug: "pending-repo", RootAbs: "/parent/pending-repo", RootRel: "pending-repo", FileCount: 1},
	}
	topo := &topology.RepoTopology{
		ParentRoot:   "/parent",
		ParentSlug:   "parent-test",
		Repos:        repos,
		DiscoveredAt: time.Now(),
	}
	build := func(repoRoot, _ string) (*rmtypes.Graph, error) {
		switch repoRoot {
		case "/parent/active-repo":
			return rmTestGraphWithScore("active-repo", "internal/foo.go", 1.0), nil
		case "/parent/pending-repo":
			return rmTestGraphWithScore("pending-repo", "internal/bar.go", 1.0), nil
		}
		return rmTestGraphWithScore("unknown", "x.go", 0), nil
	}
	mg, err := multigraph.New(multigraph.Config{
		Topology: topo,
		Build:    build,
		Cap:      4,
	})
	if err != nil {
		t.Fatalf("multigraph.New: %v", err)
	}
	if err := mg.EnsureMany([]string{"active-repo", "pending-repo"}); err != nil {
		t.Fatalf("EnsureMany: %v", err)
	}

	scores, _ := repoMapRank(
		[]string{"foo", "bar"}, nil, "/parent", mg,
		[]string{"pending-repo"},
	)
	if _, ok := scores["active-repo/internal/foo.go"]; !ok {
		t.Errorf("active-repo file missing from scores: %+v", scores)
	}
	if _, ok := scores["pending-repo/internal/bar.go"]; ok {
		t.Errorf("pending-repo file MUST be filtered out, but appears in scores: %+v", scores)
	}
}

// TestKeywordSearchWithOptions_GrepPhaseFiltersPendingSubRepos is the
// follow-up regression for the 2026-05-16 fix: the initial L1 patch
// only filtered the repo_map (structural) phase. The grep phase
// shells out to ripgrep from the workspace root (which spans every
// sub-repo in a multi-repo posture) and reintroduced inactive paths
// into the candidate pool. The CGEC phase1_unread gate then queued
// forced-reads on `claude/codrax/...` and `small/codrax-small/...`
// even though those paths were in the pending set. The FS-tool
// active-set gate refused the reads, the explorer stalled, and the
// run wound up calling emit_investigation_complete repeatedly until
// it ran out of budget. The keyword_search aggregator must apply the
// pending filter to BOTH phases.
func TestKeywordSearchWithOptions_GrepPhaseFiltersPendingSubRepos(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "active-repo", "internal"), 0o755); err != nil {
		t.Fatalf("mkdir active: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "pending-repo", "internal"), 0o755); err != nil {
		t.Fatalf("mkdir pending: %v", err)
	}
	const token = "GREPFILTERKEYWORD"
	if err := os.WriteFile(
		filepath.Join(workspace, "active-repo", "internal", "foo.go"),
		[]byte("package x\n// "+token+"\n"), 0o644,
	); err != nil {
		t.Fatalf("write active: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(workspace, "pending-repo", "internal", "bar.go"),
		[]byte("package y\n// "+token+"\n"), 0o644,
	); err != nil {
		t.Fatalf("write pending: %v", err)
	}

	// Skip the structural phase entirely (no MultiGraph) — this test
	// targets the grep-phase filter specifically. Without the fix,
	// the pending-repo file would land in the result. With the fix,
	// it is dropped before merge.
	got := keywordSearchWithOptions([]string{token}, workspace, keywordSearchOptions{
		PendingSubRepos: []string{"pending-repo"},
	})
	if got == nil {
		t.Fatal("keywordSearchWithOptions returned nil")
	}
	for _, f := range got.Files {
		if strings.HasPrefix(f.Path, "pending-repo/") {
			t.Errorf("pending sub-repo file MUST be filtered, got %q in results: %+v", f.Path, got.Files)
		}
	}
	foundActive := false
	for _, f := range got.Files {
		if strings.HasPrefix(f.Path, "active-repo/") {
			foundActive = true
			break
		}
	}
	if !foundActive {
		t.Errorf("active-repo file should survive: %+v", got.Files)
	}
}

func TestKeywordSearchWithOptions_GrepPhaseUsesActiveMultiRepoRoots(t *testing.T) {
	workspace := t.TempDir()
	activeRoot := filepath.Join(workspace, "active-repo")
	inactiveRoot := filepath.Join(workspace, "inactive-repo")
	if err := os.MkdirAll(filepath.Join(activeRoot, "internal"), 0o755); err != nil {
		t.Fatalf("mkdir active: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(inactiveRoot, "internal"), 0o755); err != nil {
		t.Fatalf("mkdir inactive: %v", err)
	}
	const token = "ACTIVEONLYKEYWORD"
	if err := os.WriteFile(
		filepath.Join(activeRoot, "internal", "foo.go"),
		[]byte("package x\n// "+token+"\n"), 0o644,
	); err != nil {
		t.Fatalf("write active: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(inactiveRoot, "internal", "bar.go"),
		[]byte("package y\n// "+token+"\n"), 0o644,
	); err != nil {
		t.Fatalf("write inactive: %v", err)
	}

	topo := &topology.RepoTopology{
		ParentRoot: workspace,
		ParentSlug: "workspace",
		Repos: []topology.SubRepo{
			{Slug: "active-repo", RootAbs: activeRoot, RootRel: "active-repo", FileCount: 1},
			{Slug: "inactive-repo", RootAbs: inactiveRoot, RootRel: "inactive-repo", FileCount: 1},
		},
		DiscoveredAt: time.Now(),
	}
	mg, err := multigraph.New(multigraph.Config{
		Topology: topo,
		Build: func(repoRoot, query string) (*rmtypes.Graph, error) {
			switch repoRoot {
			case activeRoot:
				return rmTestGraphWithScore("active-repo", "internal/foo.go", 1.0), nil
			case inactiveRoot:
				return rmTestGraphWithScore("inactive-repo", "internal/bar.go", 1.0), nil
			default:
				t.Fatalf("unexpected repo root %q", repoRoot)
				return nil, nil
			}
		},
		Cap: 2,
	})
	if err != nil {
		t.Fatalf("multigraph.New: %v", err)
	}
	if _, err := mg.EnsureLoaded("active-repo"); err != nil {
		t.Fatalf("EnsureLoaded(active): %v", err)
	}

	got := keywordSearchWithOptions([]string{token}, workspace, keywordSearchOptions{
		MultiGraph: mg,
	})
	if got == nil {
		t.Fatal("keywordSearchWithOptions returned nil")
	}
	foundActive := false
	for _, f := range got.Files {
		if strings.HasPrefix(f.Path, "inactive-repo/") {
			t.Fatalf("inactive sub-repo was scanned despite not being active: %+v", got.Files)
		}
		if f.Path == "active-repo/internal/foo.go" {
			foundActive = true
		}
	}
	if !foundActive {
		t.Fatalf("active sub-repo file should survive: %+v", got.Files)
	}
}

func TestCountSourceFilesCachesPerRoot(t *testing.T) {
	sourceCountMu.Lock()
	old := sourceCountByRoot
	sourceCountByRoot = make(map[string]int)
	sourceCountMu.Unlock()
	t.Cleanup(func() {
		sourceCountMu.Lock()
		sourceCountByRoot = old
		sourceCountMu.Unlock()
	})

	rootA := t.TempDir()
	rootB := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootA, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootB, "b.go"), []byte("package b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootB, "c.go"), []byte("package b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := countSourceFiles(rootA); got != 1 {
		t.Fatalf("rootA count = %d, want 1", got)
	}
	if got := countSourceFiles(rootB); got != 2 {
		t.Fatalf("rootB count = %d, want 2", got)
	}
}
