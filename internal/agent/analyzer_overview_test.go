package agent

import (
	"strings"
	"testing"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestExtractAnalyzerOverviewCautionTokens(t *testing.T) {
	got := extractAnalyzerOverviewCautionTokens("`buildAnalysisIR` 和 explore_mid_loop_hint_budget 以及 foo.bar，另外还有普通文字")
	for _, want := range []string{"buildAnalysisIR", "explore_mid_loop_hint_budget", "foo.bar"} {
		found := false
		for _, token := range got {
			if token == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected caution token %q, got %v", want, got)
		}
	}
}

func TestRenderAnalyzerOverviewPrescanCaution_FlagsAuxiliaryOnlyAndUnresolved(t *testing.T) {
	docDef := &repomap.Symbol{Name: "explore_mid_loop_hint_budget", File: "docs/reference.md", Line: 12}
	docFile := &repomap.FileInfo{
		RelPath:  "docs/reference.md",
		Language: "markdown",
		Symbols:  []repomap.Symbol{*docDef},
	}
	runtimeDef := &repomap.Symbol{Name: "ResolvedExploreHeuristics", File: "internal/types/config.go", Line: 700}
	runtimeFile := &repomap.FileInfo{
		RelPath:  "internal/types/config.go",
		Language: "go",
		Symbols:  []repomap.Symbol{*runtimeDef},
	}
	graph := &repomap.Graph{
		Files:       []*repomap.FileInfo{docFile, runtimeFile},
		FileIndex:   map[string]*repomap.FileInfo{docFile.RelPath: docFile, runtimeFile.RelPath: runtimeFile},
		SymbolDefs:  map[string][]*repomap.Symbol{docDef.Name: {docDef}, runtimeDef.Name: {runtimeDef}},
		Scores:      map[string]float64{},
		QueryScores: map[string]float64{},
	}
	got := renderAnalyzerOverviewPrescanCaution(graph, "`explore_mid_loop_hint_budget` and `missing_exact_token`")
	for _, want := range []string{
		"Identifier Pre-scan Cautions",
		"Do not let auxiliary-only hits in docs / tests / examples upgrade a token into production proof",
		"Do not promote nearby files or symbols from these cautions into new `exact_targets`",
		"explore_mid_loop_hint_budget",
		"docs/reference.md",
		"keep this token unverified",
		"missing_exact_token",
		"no current exact production hit",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("overview caution missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnalyzerAuthoritativeLogOverview_SkipsTaskMapNoise(t *testing.T) {
	bundle := &types.LogBundle{
		ResolvedFiles: []string{"internal/agent/analyzer.go"},
		Meta:          types.LogMeta{Signals: []types.LogSignal{types.SignalPanic}},
		Errors: []types.LogError{{
			Frames: []types.LogFrame{
				{File: "internal/agent/analyzer.go", Line: 250, Func: "github.com/hanchaoqun/codrax/internal/agent.buildAnalysisIR"},
				{File: "internal/agent/analyzer.go", Line: 320, Func: "github.com/hanchaoqun/codrax/internal/agent.(*analyzerEvaluator).ParseOutput"},
			},
		}},
	}
	got := renderAnalyzerAuthoritativeLogOverview(bundle)
	for _, want := range []string{
		"Authoritative runtime anchors",
		"file-set ceiling",
		"`internal/agent/analyzer.go`",
		"buildAnalysisIR",
		"ParseOutput",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("authoritative overview missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "task_map") {
		t.Fatalf("authoritative log overview should bypass task_map noise, got:\n%s", got)
	}
}
