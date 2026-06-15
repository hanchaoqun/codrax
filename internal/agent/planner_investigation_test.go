package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	repomaptypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// planner_investigation_test.go pins Batch 1 (Modules A + B): the
// planner's pre-emit orientation hint composed of:
//   - "## Likely-relevant files" — top-N from keyword search ranking
//   - "## Test surface" — manifests / test dirs / runner hints from
//     the cached repomap Graph
// Both sections are FACT-ONLY (no prescriptive prose); the planner
// reads them and decides what to do, the system never says "you
// should add X to manifest Y".

// TestExtractTestProfile_RecognizesManifestsAndTestDirs verifies the
// pure derivation function extracts the right facts from a repomap
// Graph populated with realistic file paths. No disk I/O — the graph
// is the only source.
func TestExtractTestProfile_RecognizesManifestsAndTestDirs(t *testing.T) {
	g := &repomaptypes.Graph{
		Files: []*repomaptypes.FileInfo{
			{RelPath: "go.mod", Language: "go"},
			{RelPath: "package.json", Language: "json"},
			{RelPath: "tests/test_main.py", Language: "python"},
			{RelPath: "spec/handler_spec.rb", Language: "ruby"},
			{RelPath: "src/main.go", Language: "go"},
			{RelPath: "internal/handler.go", Language: "go"},
		},
	}
	p := extractTestProfile(g)

	// Manifests
	hasGo := false
	hasNode := false
	for _, m := range p.Manifests {
		if m == "go.mod" {
			hasGo = true
		}
		if m == "package.json" {
			hasNode = true
		}
	}
	if !hasGo {
		t.Errorf("Manifests should include go.mod; got %v", p.Manifests)
	}
	if !hasNode {
		t.Errorf("Manifests should include package.json; got %v", p.Manifests)
	}
	// Test dirs
	hasTests := false
	hasSpec := false
	for _, d := range p.TestDirs {
		if d == "tests" {
			hasTests = true
		}
		if d == "spec" {
			hasSpec = true
		}
	}
	if !hasTests {
		t.Errorf("TestDirs should include 'tests'; got %v", p.TestDirs)
	}
	if !hasSpec {
		t.Errorf("TestDirs should include 'spec'; got %v", p.TestDirs)
	}
	// Runner hints — go.mod → go runner, package.json → node runner
	hasGoRunner := false
	hasNodeRunner := false
	for _, r := range p.RunnerHints {
		if strings.HasPrefix(r, "go ") {
			hasGoRunner = true
		}
		if strings.HasPrefix(r, "node ") {
			hasNodeRunner = true
		}
	}
	if !hasGoRunner {
		t.Errorf("RunnerHints should include a 'go ' runner; got %v", p.RunnerHints)
	}
	if !hasNodeRunner {
		t.Errorf("RunnerHints should include a 'node ' runner; got %v", p.RunnerHints)
	}
}

// TestExtractTestProfile_EmptyGraphReturnsEmpty pins the
// degraded path: no graph or empty graph means no Test surface
// section — the planner falls through to its workflow without
// orientation rather than seeing a misleading empty section.
func TestExtractTestProfile_EmptyGraphReturnsEmpty(t *testing.T) {
	if !profileEmpty(extractTestProfile(nil)) {
		t.Error("nil graph should produce empty profile")
	}
	if !profileEmpty(extractTestProfile(&repomaptypes.Graph{})) {
		t.Error("empty graph should produce empty profile")
	}
	g := &repomaptypes.Graph{
		Files: []*repomaptypes.FileInfo{
			{RelPath: "src/main.go"}, // no manifest, no test dir
		},
	}
	if !profileEmpty(extractTestProfile(g)) {
		t.Error("graph with no manifests/test dirs should produce empty profile")
	}
}

// profileEmpty wraps the pointer-method empty() so the test can call
// it on the value returned by extractTestProfile.
func profileEmpty(p repoTestProfile) bool {
	return p.empty()
}

// TestExtractTestProfile_DeterministicAndCapped pins ordering +
// cap so the rendered prompt is stable across runs and bounded for
// monorepos.
func TestExtractTestProfile_DeterministicAndCapped(t *testing.T) {
	files := []*repomaptypes.FileInfo{}
	// Inject 12 manifests and 12 test dirs — both should cap at
	// plannerTestSurfaceMaxItems (8).
	manifests := []string{
		"go.mod", "package.json", "pyproject.toml", "Cargo.toml",
		"pom.xml", "Gemfile", "Makefile", "CMakeLists.txt",
		"meson.build", "Package.swift", "build.gradle", "cjpm.toml",
	}
	for _, m := range manifests {
		files = append(files, &repomaptypes.FileInfo{RelPath: m})
	}
	for i := 0; i < 12; i++ {
		files = append(files, &repomaptypes.FileInfo{
			RelPath: "tests/sub" + string(rune('a'+i)) + "/test_x.py",
		})
	}
	g := &repomaptypes.Graph{Files: files}
	p := extractTestProfile(g)
	if len(p.Manifests) != plannerTestSurfaceMaxItems {
		t.Errorf("Manifests should cap at %d; got %d", plannerTestSurfaceMaxItems, len(p.Manifests))
	}
	if len(p.TestDirs) > plannerTestSurfaceMaxItems {
		t.Errorf("TestDirs should cap at %d; got %d", plannerTestSurfaceMaxItems, len(p.TestDirs))
	}
	// Must be sorted (deterministic).
	for i := 1; i < len(p.Manifests); i++ {
		if p.Manifests[i] < p.Manifests[i-1] {
			t.Errorf("Manifests not sorted: %v", p.Manifests)
			break
		}
	}
}

// TestPlannerBuildInitialInstruction_TestSurfaceFromCachedGraph
// verifies that when Mutable.SearchGraph() carries a graph (e.g. set
// by an earlier explorer dispatch in the same Run), the planner
// renders a non-empty `## Test surface` section without doing its
// own keyword search (the keyword path needs IR which a stubbed ctx
// won't provide).
func TestPlannerBuildInitialInstruction_TestSurfaceFromCachedGraph(t *testing.T) {
	mu := types.NewMutableState("test plan")
	mu.SetSearchGraph(&repomaptypes.Graph{
		Files: []*repomaptypes.FileInfo{
			{RelPath: "go.mod"},
			{RelPath: "tests/test_main.py"},
		},
	})
	e := &plannerEvaluator{}
	ctx := &types.AgentContext{Mutable: mu, RepoRoot: ""} // empty RepoRoot → skip module A keyword search
	got := e.BuildInitialInstruction(ctx, nil)

	if !strings.Contains(got, "## Test surface") {
		t.Errorf("expected `## Test surface` section in instruction; got:\n%s", got)
	}
	if !strings.Contains(got, "go.mod") {
		t.Errorf("Test surface should list go.mod manifest; got:\n%s", got)
	}
	if !strings.Contains(got, "tests") {
		t.Errorf("Test surface should list test dir; got:\n%s", got)
	}
	// Forbidden — the section must NOT contain prescriptive prose.
	for _, banned := range []string{"you should", "you must", "Most common", "make sure"} {
		if strings.Contains(got, banned) {
			t.Errorf("Test surface must not contain prescriptive prose %q; got:\n%s", banned, got)
		}
	}
}

func TestPlannerInvestigationSeedIsFactOnly(t *testing.T) {
	repo := writePlannerInvestigationFixture(t)
	mu := types.NewMutableState("fix NeedleWidget")
	ctx := &types.AgentContext{
		Mutable:  mu,
		RepoRoot: repo,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Keywords: []string{"NeedleWidget"},
				},
			},
		},
	}
	e := &plannerEvaluator{}

	got := e.buildInvestigationSeed(ctx)
	if !strings.Contains(got, "## Likely-relevant files") {
		t.Fatalf("expected likely-file seed from analyzer keywords; got:\n%s", got)
	}
	for _, banned := range []string{
		"Use read_file / grep",
		"before deciding what to change",
		"Use line_offset/limit",
	} {
		if strings.Contains(got, banned) {
			t.Fatalf("investigation seed should stay fact-only; found %q in:\n%s", banned, got)
		}
	}
}

func TestPlannerBuildInitialInstruction_ExplorationHandoffSuppressesInvestigationSeed(t *testing.T) {
	repo := writePlannerInvestigationFixture(t)
	mu := types.NewMutableState("fix NeedleWidget")
	mu.SetWriteContextPack(&types.WriteContextPack{
		SourceStage: "explore",
		Items: []types.WriteContextItem{{
			Priority:    types.WriteContextP1,
			Kind:        "target_file",
			Text:        "pkg/needle.go",
			SourceStage: "explore",
			Consumers:   []types.WriteContextConsumer{types.WriteConsumerPlanner},
		}},
	})
	ctx := &types.AgentContext{
		Mutable:  mu,
		RepoRoot: repo,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Keywords: []string{"NeedleWidget"},
				},
			},
		},
	}
	e := &plannerEvaluator{}
	if seed := e.buildInvestigationSeed(ctx); !strings.Contains(seed, "## Likely-relevant files") {
		t.Fatalf("fixture should be able to produce generic likely-file seed; got:\n%s", seed)
	}

	got := e.BuildInitialInstruction(ctx, nil)
	if !strings.Contains(got, "## Priority write context pack") {
		t.Fatalf("expected exploration context pack to render; got:\n%s", got)
	}
	if strings.Contains(got, "## Likely-relevant files") {
		t.Fatalf("exploration-grade handoff should suppress generic likely-file rediscovery seed; got:\n%s", got)
	}
}

func writePlannerInvestigationFixture(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	body := []byte("package pkg\n\nfunc NeedleWidget() string {\n\treturn \"needle\"\n}\n")
	if err := os.WriteFile(filepath.Join(repo, "pkg", "needle.go"), body, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return repo
}

// TestPlannerBuildInitialInstruction_PlanningContextStillFires
// confirms the existing PlanningHint path (verify→plan retry,
// scaffold directive, stall recovery) is preserved by the
// three-section refactor. Composition: when a hint is set, the
// returned instruction includes the `## Planning context` section.
func TestPlannerBuildInitialInstruction_PlanningContextStillFires(t *testing.T) {
	mu := types.NewMutableState("test plan")
	mu.SetPlanningHint("RETRY DIRECTIVE — previous attempt 1 verify failed.")
	e := &plannerEvaluator{}
	ctx := &types.AgentContext{Mutable: mu}
	got := e.BuildInitialInstruction(ctx, nil)

	if !strings.Contains(got, "## Planning context") {
		t.Errorf("expected `## Planning context` section; got:\n%s", got)
	}
	if !strings.Contains(got, "RETRY DIRECTIVE") {
		t.Errorf("planning context should carry the hint body; got:\n%s", got)
	}
	// Consume-once: the hint should be cleared after the first
	// BuildInitialInstruction.
	if mu.PlanningHint() != "" {
		t.Errorf("PlanningHint should be cleared after consume; got %q", mu.PlanningHint())
	}
}

// TestPlannerBuildInitialInstruction_NoOpWhenAllSectionsEmpty pins
// the degraded path: when there's no graph, no hint, no IR, the
// planner returns "" and falls through to the skill workflow alone.
func TestPlannerBuildInitialInstruction_NoOpWhenAllSectionsEmpty(t *testing.T) {
	mu := types.NewMutableState("test plan")
	e := &plannerEvaluator{}
	ctx := &types.AgentContext{Mutable: mu}
	got := e.BuildInitialInstruction(ctx, nil)
	if got != "" {
		t.Errorf("expected empty instruction when no signal; got:\n%q", got)
	}
}
