package retrieve

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func rankOptionsTestGraph() *types.Graph {
	files := []*types.FileInfo{
		{
			RelPath: "pkg/server.go", Language: types.LangGo, Package: "pkg",
			Symbols: []types.Symbol{{Name: "Serve", Kind: "function", File: "pkg/server.go", Line: 1, EndLine: 9, Exported: true}},
		},
		{
			RelPath: "pkg/server_test.go", Language: types.LangGo, Package: "pkg",
			Symbols: []types.Symbol{{Name: "TestServe", Kind: "function", File: "pkg/server_test.go", Line: 1, EndLine: 9}},
		},
	}
	g := &types.Graph{Root: "/repo", Files: files}
	g.FileIndex = map[string]*types.FileInfo{}
	for _, fi := range files {
		g.FileIndex[fi.RelPath] = fi
	}
	return g
}

// Zero-value options must be byte-identical to the historical path:
// keyword_search applies its own 0.35x auxiliary multiplier on
// combined scores that include these, so an unconditional core change
// would double-penalize (~0.12x).
func TestRankGraphScoresWithOptions_ZeroValueIdentical(t *testing.T) {
	g := rankOptionsTestGraph()
	base := RankGraphScores(g, "serve")
	opt := RankGraphScoresWithOptions(g, "serve", RankOptions{})
	for path, want := range base.Scores {
		if got := opt.Scores[path]; got != want {
			t.Errorf("zero-value options changed score for %s: %v != %v", path, got, want)
		}
	}
}

func TestRankGraphScoresWithOptions_AuxiliaryDownrankIsSoft(t *testing.T) {
	g := rankOptionsTestGraph()
	base := RankGraphScores(g, "serve")
	down := RankGraphScoresWithOptions(g, "serve", RankOptions{DeprioritizeAuxiliary: true})

	prod := "pkg/server.go"
	test := "pkg/server_test.go"
	if down.Scores[prod] != base.Scores[prod] {
		t.Errorf("production file score changed under aux downrank: %v != %v", down.Scores[prod], base.Scores[prod])
	}
	if base.Scores[test] <= 0 {
		t.Fatalf("fixture broken: test file scored %v without downrank", base.Scores[test])
	}
	want := base.Scores[test] * rankAuxiliaryMultiplier
	if got := down.Scores[test]; got != want {
		t.Errorf("aux multiplier not applied exactly once: got %v want %v", got, want)
	}
	if down.Scores[test] <= 0 {
		t.Errorf("downrank zeroed the score — must stay soft (listable)")
	}
}

// A test-only query match must still surface the test file even under
// downranking: multiplied, never excluded.
func TestRankGraphScoresWithOptions_TestOnlyMatchStillSelectable(t *testing.T) {
	g := rankOptionsTestGraph()
	down := RankGraphScoresWithOptions(g, "TestServe", RankOptions{DeprioritizeAuxiliary: true})
	if down.Scores["pkg/server_test.go"] <= 0 {
		t.Errorf("test-only match was rank-zeroed under downranking: %v", down.Scores)
	}
}
