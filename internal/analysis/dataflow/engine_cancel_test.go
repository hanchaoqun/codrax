package dataflow

import (
	"context"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

func TestAnalyzeHonorsCanceledContext(t *testing.T) {
	root, graph := setupTestRepo(t, []testFile{{
		relPath: "a.go",
		lang:    "go",
		content: "package p\nfunc A() { B() }\nfunc B() {}\n",
		symbols: []repomap.Symbol{
			{Name: "A", Kind: "function", Line: 2, EndLine: 2},
			{Name: "B", Kind: "function", Line: 3, EndLine: 3},
		},
	}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got := Analyze(graph, Options{
		Context:        ctx,
		RepoRoot:       root,
		CandidateFiles: []string{"a.go"},
	})
	if len(got.Evidence) != 0 || len(got.Findings) != 0 {
		t.Fatalf("canceled analysis should not continue producing output: %+v", got)
	}
}
