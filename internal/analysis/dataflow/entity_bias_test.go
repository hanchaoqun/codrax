package dataflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

// T2.3 tests guard sortByEntityBias against the over-fitting failure
// modes that bit T1.1 / ERM Part 2:
//
//   - Path-only matches must rank correctly
//   - Symbol-name matches must rank correctly
//   - Files with no match must NOT be filtered out — only re-ranked
//   - Empty bias is a no-op (alphabetical fallback)

func makeFileGraph(files map[string][]string) *repomap.Graph {
	g := &repomap.Graph{
		FileIndex:  make(map[string]*repomap.FileInfo),
		SymbolDefs: make(map[string][]*repomap.Symbol),
	}
	for path, syms := range files {
		fi := &repomap.FileInfo{RelPath: path}
		for _, name := range syms {
			fi.Symbols = append(fi.Symbols, repomap.Symbol{Name: name, File: path})
		}
		g.FileIndex[path] = fi
		g.Files = append(g.Files, fi)
	}
	return g
}

func TestSortByEntityBias_PathMatchRanksFirst(t *testing.T) {
	graph := makeFileGraph(map[string][]string{
		"internal/agent/explorer.go":     {"explorerEvaluator", "BuildInitialPrompt"},
		"internal/agent/sub_explorer.go": {"SubExplorer"},
		"internal/tool/grep.go":          {"GrepTool"},
		"internal/tool/builtin.go":       {"DefaultRegistry"},
	})
	files := []string{
		"internal/agent/explorer.go",
		"internal/agent/sub_explorer.go",
		"internal/tool/grep.go",
		"internal/tool/builtin.go",
	}
	got := sortByEntityBias(files, graph, []string{"explorer"})
	if got[0] != "internal/agent/explorer.go" && got[0] != "internal/agent/sub_explorer.go" {
		t.Errorf("expected explorer-related file first, got %v", got)
	}
	// All input files must still be present.
	if len(got) != len(files) {
		t.Errorf("file count changed: in=%d out=%d", len(files), len(got))
	}
}

func TestSortByEntityBias_SymbolMatchRanks(t *testing.T) {
	graph := makeFileGraph(map[string][]string{
		"a.go": {"FooBar"},
		"b.go": {"BazQux"},
		"c.go": {"FooBaz"},
	})
	got := sortByEntityBias([]string{"a.go", "b.go", "c.go"}, graph, []string{"foo"})
	// "foo" matches FooBar in a.go and FooBaz in c.go (1 each).
	// b.go has no match.
	// Expect a.go and c.go before b.go (deterministic alpha tiebreak).
	if got[0] != "a.go" || got[1] != "c.go" || got[2] != "b.go" {
		t.Errorf("expected [a.go,c.go,b.go], got %v", got)
	}
}

func TestSortByEntityBias_NoFilterOnNoMatch(t *testing.T) {
	graph := makeFileGraph(map[string][]string{
		"a.go": {"Unrelated"},
		"b.go": {"AlsoUnrelated"},
	})
	got := sortByEntityBias([]string{"a.go", "b.go"}, graph, []string{"explorer"})
	// No file matches "explorer" — both must still be present, and order
	// must be alphabetical (deterministic tiebreak).
	if len(got) != 2 {
		t.Fatalf("file count changed: in=2 out=%d", len(got))
	}
	if got[0] != "a.go" || got[1] != "b.go" {
		t.Errorf("expected alphabetical [a.go,b.go], got %v", got)
	}
}

func TestSortByEntityBias_EmptyBiasNoOp(t *testing.T) {
	files := []string{"b.go", "a.go", "c.go"}
	got := sortByEntityBias(files, nil, nil)
	// Returns the input as-is (no sort) because the early-return fires.
	if len(got) != 3 {
		t.Fatalf("file count changed: in=3 out=%d", len(got))
	}
	for i, p := range files {
		if got[i] != p {
			t.Errorf("empty bias should be no-op; got[%d]=%s, want=%s", i, got[i], p)
		}
	}
}

func TestSortByEntityBias_PathScoresHigherThanSymbol(t *testing.T) {
	// Path matches are weighted 2 and symbol matches 1, so a file
	// whose path mentions the entity must rank above a file with
	// only symbol matches even if the symbol-only file has more
	// individual hits.
	graph := makeFileGraph(map[string][]string{
		"explorer.go": {"Foo"},
		"other.go":    {"Bar", "Baz"},
	})
	got := sortByEntityBias([]string{"other.go", "explorer.go"}, graph, []string{"explorer"})
	if got[0] != "explorer.go" {
		t.Errorf("expected explorer.go first (path match 2 > 0), got %v", got)
	}
}
