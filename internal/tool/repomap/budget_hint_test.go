package repomap

import (
	"strings"
	"testing"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func budgetTestGraph(files int) *Graph {
	g := &Graph{FileIndex: map[string]*rmtypes.FileInfo{}}
	for i := 0; i < files; i++ {
		g.FileIndex[strings.Repeat("a", 3)+string(rune('a'+i%26))+"/"+strings.Repeat("f", 1+i%5)+".go"+strings.Repeat("x", i/26)] = &rmtypes.FileInfo{}
	}
	return g
}

func TestBudgetHint_SmallScopeStaysSilent(t *testing.T) {
	g := budgetTestGraph(120)
	out := appendRepoMapBudgetHint(g, "task_map", 0, "BODY")
	if out != "BODY" {
		t.Errorf("tiny scope must add nothing (anti-noise), got:\n%s", out)
	}
}

func TestBudgetHint_LargeScopeAddsNarrowingNote(t *testing.T) {
	g := budgetTestGraph(6000)
	out := appendRepoMapBudgetHint(g, "task_map", 0, "BODY")
	if !strings.Contains(out, "Repo Map Budget Hint") || !strings.Contains(out, "6000 files indexed") {
		t.Errorf("large scope missing narrowing hint:\n%s", out)
	}
}

func TestBudgetHint_SuppressedWhenBroadFallbackAdvisoryPresent(t *testing.T) {
	g := budgetTestGraph(6000)
	out := appendRepoMapBudgetHint(g, "relation_map", 0, "BODY mode=broad_fallback BODY")
	if strings.Contains(out, "Repo Map Budget Hint") {
		t.Errorf("hint must not double up with the broad-fallback advisory:\n%s", out)
	}
}

func TestBudgetHint_ExplicitLargeTopNGetsSoftNoteNeverClamped(t *testing.T) {
	g := budgetTestGraph(6000)
	out := appendRepoMapBudgetHint(g, "task_map", 100, "BODY")
	if !strings.Contains(out, "top_n is large for this repository scope") {
		t.Errorf("missing top_n soft note:\n%s", out)
	}
}
