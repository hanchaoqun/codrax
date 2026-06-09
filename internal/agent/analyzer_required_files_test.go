package agent

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Helper: build a minimal Graph with FileIndex / SymbolDefs /
// QueryScores entries the ranker reads. Keeps the tests focused on
// ranking behaviour rather than on graph construction details.
func buildRankerGraph(
	fileSymbols map[string][]repomap.Symbol,
	queryScores map[string]float64,
) *repomap.Graph {
	g := &repomap.Graph{
		FileIndex:   make(map[string]*repomap.FileInfo),
		SymbolDefs:  make(map[string][]*repomap.Symbol),
		QueryScores: queryScores,
	}
	for path, syms := range fileSymbols {
		fi := &repomap.FileInfo{RelPath: path, Symbols: syms}
		g.FileIndex[path] = fi
		for i := range fi.Symbols {
			s := &fi.Symbols[i]
			s.File = path
			g.SymbolDefs[s.Name] = append(g.SymbolDefs[s.Name], s)
		}
	}
	return g
}

// TestRankAnalyzerRequiredFiles_UserNamedFilePromoted reproduces the
// u3a audit finding: user wrote "explorer.go" and "extractor.go" as
// entities, but doc-comment-heavy unrelated files outranked the named
// files under pure QueryScore sort. The exactPathFiles anchor tier
// must flip both back above the diffuse score hits.
func TestRankAnalyzerRequiredFiles_UserNamedFilePromoted(t *testing.T) {
	graph := buildRankerGraph(
		map[string][]repomap.Symbol{
			"internal/agent/explorer.go": {
				{Name: "ShouldStop", Kind: "method", Line: 959, Exported: true},
			},
			"internal/agent/extractor.go": {
				{Name: "ShouldStop", Kind: "method", Line: 280, Exported: true},
			},
			// Noise file that rode doc-comment matches to slot 1 in
			// the audit — has neither the requested path nor a
			// unique symbol definition.
			"internal/types/evidence_closure.go": {
				{Name: "ReadSet", Kind: "method", Line: 120, Exported: true},
			},
			"internal/agent/sub_explorer.go": {
				{Name: "ShouldStop", Kind: "method", Line: 151, Exported: true},
			},
		},
		map[string]float64{
			// Pre-anchor ordering: noise files score highest because
			// they mention the entity tokens in doc comments.
			"internal/types/evidence_closure.go": 5.0,
			"internal/agent/sub_explorer.go":     4.5,
			"internal/agent/explorer.go":         3.0,
			"internal/agent/extractor.go":        2.5,
		},
	)

	got := rankAnalyzerRequiredFiles(graph, []string{
		"explorer.go", "extractor.go", "ShouldStop", "ReAct",
	})

	// The two user-named files must appear before the noise files.
	explorerIdx, extractorIdx := -1, -1
	noiseIdx := -1
	for i, p := range got {
		switch p {
		case "internal/agent/explorer.go":
			explorerIdx = i
		case "internal/agent/extractor.go":
			extractorIdx = i
		case "internal/types/evidence_closure.go":
			noiseIdx = i
		}
	}
	if explorerIdx < 0 || extractorIdx < 0 {
		t.Fatalf("expected explorer.go and extractor.go in ranking, got %v", got)
	}
	if explorerIdx >= noiseIdx || extractorIdx >= noiseIdx {
		t.Errorf("user-named files must outrank noise: explorer=%d extractor=%d noise=%d (got=%v)",
			explorerIdx, extractorIdx, noiseIdx, got)
	}
}

// TestRankAnalyzerRequiredFiles_NonUniqueSymbolDoesNotAnchor: when an
// entity's symbol name is defined in many files (e.g. "ShouldStop"
// appears on explorer, extractor, sub_explorer, analyzer, ...), the
// anchor tier must NOT promote any of them — there is no way to tell
// which definition the user meant. Falls through to QueryScore.
func TestRankAnalyzerRequiredFiles_NonUniqueSymbolDoesNotAnchor(t *testing.T) {
	graph := buildRankerGraph(
		map[string][]repomap.Symbol{
			"a.go": {{Name: "ShouldStop", Kind: "method", Line: 10, Exported: true}},
			"b.go": {{Name: "ShouldStop", Kind: "method", Line: 20, Exported: true}},
			"c.go": {{Name: "ShouldStop", Kind: "method", Line: 30, Exported: true}},
		},
		map[string]float64{
			"a.go": 3.0,
			"b.go": 2.0,
			"c.go": 1.0,
		},
	)

	got := rankAnalyzerRequiredFiles(graph, []string{"ShouldStop"})

	// QueryScore-only ordering expected since no anchor fires.
	want := []string{"a.go", "b.go", "c.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("non-unique symbol must fall through to QueryScore order: got %v, want %v", got, want)
	}
}

// TestRankAnalyzerRequiredFiles_QualifiedSymbolOutranksPath: a
// qualified-symbol anchor (rank 3) beats a path-only anchor (rank 2)
// in the same entity batch. Defends the tier ordering produced by
// exactEntityAnchors.
func TestRankAnalyzerRequiredFiles_QualifiedSymbolOutranksPath(t *testing.T) {
	graph := buildRankerGraph(
		map[string][]repomap.Symbol{
			// Qualified match: Runner.Execute exists uniquely here.
			"internal/engine/runner.go": {
				{Name: "Execute", Kind: "method", Line: 42, Exported: true, Receiver: "Runner"},
			},
			// Path match: basename "engine.go" uniquely in repo.
			"internal/engine.go": {
				{Name: "Start", Kind: "function", Line: 10, Exported: true},
			},
		},
		map[string]float64{
			"internal/engine/runner.go": 1.0,
			"internal/engine.go":        2.0, // higher raw score
		},
	)

	got := rankAnalyzerRequiredFiles(graph, []string{"Runner.Execute", "engine.go"})

	// engine.go is path_exact (rank 2) and Runner.Execute is
	// qualified_symbol_exact (rank 3). Rank 3 beats rank 2 even
	// though engine.go has a higher raw QueryScore.
	if len(got) == 0 || got[0] != "internal/engine/runner.go" {
		t.Errorf("qualified-symbol anchor must outrank path anchor: got %v", got)
	}
}

// TestRankAnalyzerRequiredFiles_AnchorsNotInQueryScoresStillIncluded:
// if a file is not in QueryScores but IS an exact anchor, it must
// still appear in the output. Guards against the "files with 0
// QueryScore get dropped" edge case where e.g. a renamed or rarely-
// referenced file happens to be the exact path the user asked about.
func TestRankAnalyzerRequiredFiles_AnchorsNotInQueryScoresStillIncluded(t *testing.T) {
	graph := buildRankerGraph(
		map[string][]repomap.Symbol{
			"internal/niche/tool.go": {
				{Name: "RareFn", Kind: "function", Line: 5, Exported: true},
			},
			"other.go": {
				{Name: "Other", Kind: "function", Line: 1, Exported: true},
			},
		},
		map[string]float64{
			// Niche file intentionally absent from QueryScores. Only
			// the anchor tier can rescue it.
			"other.go": 1.0,
		},
	)

	got := rankAnalyzerRequiredFiles(graph, []string{"tool.go"})

	if len(got) == 0 || got[0] != "internal/niche/tool.go" {
		t.Errorf("anchor-only file missing from QueryScores must still land top-1: got %v", got)
	}
}

// TestRankAnalyzerRequiredFiles_EmptyInputs: defensive paths.
func TestRankAnalyzerRequiredFiles_EmptyInputs(t *testing.T) {
	if got := rankAnalyzerRequiredFiles(nil, []string{"X"}); got != nil {
		t.Errorf("nil graph must return nil, got %v", got)
	}
	empty := &repomap.Graph{
		FileIndex:   map[string]*repomap.FileInfo{},
		SymbolDefs:  map[string][]*repomap.Symbol{},
		QueryScores: map[string]float64{},
	}
	if got := rankAnalyzerRequiredFiles(empty, []string{"X"}); got != nil {
		t.Errorf("empty graph must return nil, got %v", got)
	}
	graph := buildRankerGraph(
		map[string][]repomap.Symbol{"a.go": {{Name: "A", Kind: "function"}}},
		map[string]float64{"a.go": 1.0},
	)
	if got := rankAnalyzerRequiredFiles(graph, nil); got != nil {
		// Pure QueryScore path with no entities still works — the
		// scoring is keyed by path, not entity text.
		_ = got
	}
}

// TestRankAnalyzerRequiredFiles_CapAtMaxFiles: the slice is capped at
// maxAnalyzerRequiredFiles (3 after session-22 F1.2, down from 10 —
// the cap now matches how many candidates a focused investigation
// realistically opens; ten was letting ranker-noise siblings into the
// explorer's RequiredFiles prompt and the Check-6 ranker-coverage
// gate) regardless of tier composition.
func TestRankAnalyzerRequiredFiles_CapAtMaxFiles(t *testing.T) {
	fileSymbols := make(map[string][]repomap.Symbol, 20)
	queryScores := make(map[string]float64, 20)
	for i := 0; i < 20; i++ {
		path := pathN(i)
		fileSymbols[path] = []repomap.Symbol{{Name: "Unique" + itoaN(i), Kind: "function", Exported: true}}
		queryScores[path] = float64(20 - i) // decreasing scores
	}
	graph := buildRankerGraph(fileSymbols, queryScores)
	got := rankAnalyzerRequiredFiles(graph, []string{"nothing-matches"})
	if len(got) > 3 {
		t.Errorf("result must be capped at 3 (session-22 F1.2), got %d", len(got))
	}
}

func TestAnalyzerRequiredFiles_ExternalSourceLogSkipsRepoSeeds(t *testing.T) {
	ctx := &types.AgentContext{
		RepoRoot: t.TempDir(),
		Mutable:  types.NewMutableState("test"),
	}
	ctx.Mutable.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "panic"}},
	})
	rm := types.RequestModel{
		RawRequest: "Which goroutines are failing together?",
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"writeSession", "analyzer.go"},
		},
	}
	if got := analyzerRequiredFiles(ctx, rm); got != nil {
		t.Fatalf("external-source log should suppress repo required files, got %v", got)
	}
}

func TestAnalyzerRequiredFiles_CapabilityQueryUsesAuthorityFiles(t *testing.T) {
	ctx := &types.AgentContext{
		RepoRoot: t.TempDir(),
		Mutable:  types.NewMutableState("test"),
	}
	rm := types.RequestModel{
		RawRequest: "Explorer stage 之前的 analyzer stage 里是否允许调用 read_file？",
		AnalyzerHints: types.AnalyzerHints{
			CapabilitySurface: &types.CapabilitySurfaceHint{
				Binding: types.StageBinding{
					Stage: types.StageAnalyze,
					Agent: types.AgentAnalyzer,
					Skill: "analysis-skill",
				},
				Tool: "read_file",
			},
		},
	}
	got := analyzerRequiredFiles(ctx, rm)
	want := []string{
		"internal/orchestrator/topology.go",
		"internal/skill/analysis_contract.go",
		"internal/agent/agent.go",
		"internal/agent/analyzer.go",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("capability query required files = %v, want %v", got, want)
	}
}

func TestStageTopologyAuthorityRequiredFiles_StageLikeArchitectureEnumeration(t *testing.T) {
	graph := buildRankerGraph(
		map[string][]repomap.Symbol{
			"internal/dataworkflow/stage.go": {
				{Name: "StageCoverRequiredMaterials", Kind: "const", Line: 4},
				{Name: "StageComplete", Kind: "const", Line: 11},
			},
			types.ReadModePipelineEnumsFile: {
				{Name: "StageAnalyze", Kind: "const", Line: 33},
				{Name: "StageExplore", Kind: "const", Line: 34},
				{Name: "StageExtract", Kind: "const", Line: 35},
				{Name: "StageFinalize", Kind: "const", Line: 36},
			},
			types.ReadModePipelineStageBindingFile: {
				{Name: "StageBinding", Kind: "type", Line: 6},
			},
			types.ReadModePipelineTopologyFile: {
				{Name: "pipelineTopology", Kind: "var", Line: 17},
			},
		},
		map[string]float64{
			"internal/dataworkflow/stage.go": 9.0,
		},
	)
	rm := types.RequestModel{
		Scenario: types.ScenarioArchitectureExplain,
		AnalyzerHints: types.AnalyzerHints{
			Kind: string(types.ReqEnumeration),
		},
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
	}

	got := stageTopologyAuthorityRequiredFiles(graph, rm, []string{"StageCoverRequiredMaterials"})
	want := types.ReadModePipelineAuthorityFiles()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("authority files = %v, want %v", got, want)
	}
}

func TestStageTopologyAuthorityRequiredFiles_DoesNotFireForNonStageOrNonArchitecture(t *testing.T) {
	graph := buildRankerGraph(
		map[string][]repomap.Symbol{
			"internal/dataworkflow/stage.go": {
				{Name: "StageCoverRequiredMaterials", Kind: "const", Line: 4},
			},
			types.ReadModePipelineEnumsFile:        {{Name: "StageAnalyze", Kind: "const"}},
			types.ReadModePipelineStageBindingFile: {{Name: "StageBinding", Kind: "type"}},
			types.ReadModePipelineTopologyFile:     {{Name: "pipelineTopology", Kind: "var"}},
		},
		nil,
	)
	architectureEnum := types.RequestModel{
		Scenario: types.ScenarioArchitectureExplain,
		AnalyzerHints: types.AnalyzerHints{
			Kind: string(types.ReqEnumeration),
		},
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
	}
	if got := stageTopologyAuthorityRequiredFiles(graph, architectureEnum, []string{"WorkflowState"}); got != nil {
		t.Fatalf("non-stage entity should not seed topology authority, got %v", got)
	}

	nonArchitectureEnum := architectureEnum
	nonArchitectureEnum.Scenario = types.ScenarioGeneric
	if got := stageTopologyAuthorityRequiredFiles(graph, nonArchitectureEnum, []string{"StageCoverRequiredMaterials"}); got != nil {
		t.Fatalf("non-architecture request should not seed topology authority, got %v", got)
	}
}

func TestShouldMergeLogTriageEntities_SkipsExternalSource(t *testing.T) {
	if shouldMergeLogTriageEntities(&types.LogBundle{
		Errors: []types.LogError{{Type: "panic"}},
	}) {
		t.Fatal("external-source bundle should not merge frame-derived entities into analyzer hints")
	}
	if !shouldMergeLogTriageEntities(&types.LogBundle{
		ResolvedFiles: []string{"internal/agent/analyzer.go"},
		Errors:        []types.LogError{{Type: "panic"}},
	}) {
		t.Fatal("repo-resolved log bundle should still merge entities")
	}
}

func pathN(i int) string {
	return "pkg/f" + itoaN(i) + ".go"
}

func itoaN(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
