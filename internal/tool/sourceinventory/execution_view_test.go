package sourceinventory

import (
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestExecutionViewScopesSortsAndCachesFiles(t *testing.T) {
	graph := &repotypes.Graph{Files: []*repotypes.FileInfo{
		{RelPath: "pkg/b.py", Language: "python"},
		{RelPath: "pkg/a.go", Language: "go"},
		{RelPath: "docs/readme.md", Language: "markdown"},
		{RelPath: "pkg/config.yaml", IsSpecial: true, SpecialType: "config"},
	}}
	view := NewExecutionView(ExecutionViewOptions{
		Graph:  graph,
		Scopes: []string{"pkg"},
	})
	if !view.Complete() || !view.HasInventoryFiles() || view.BudgetTruncated() {
		t.Fatalf("unexpected view state: complete=%v inventory=%v truncated=%v", view.Complete(), view.HasInventoryFiles(), view.BudgetTruncated())
	}
	files := view.Files("")
	if got, want := len(files), 3; got != want {
		t.Fatalf("file count = %d, want %d", got, want)
	}
	if files[0].RelPath != "pkg/a.go" || files[1].RelPath != "pkg/b.py" || files[2].RelPath != "pkg/config.yaml" {
		t.Fatalf("files not sorted/scoped: %+v", files)
	}
	goFiles := view.Files("go")
	if len(goFiles) != 1 || goFiles[0].RelPath != "pkg/a.go" {
		t.Fatalf("go files = %+v", goFiles)
	}
	goFiles[0] = &repotypes.FileInfo{RelPath: "mutated"}
	again := view.Files("go")
	if len(again) != 1 || again[0].RelPath != "pkg/a.go" {
		t.Fatalf("language cache leaked caller mutation: %+v", again)
	}
	if !view.ContainsFile("pkg/b.py") || view.ContainsFile("docs/readme.md") {
		t.Fatalf("contains file mismatch")
	}
}

func TestExecutionViewUsesScopeMatcher(t *testing.T) {
	graph := &repotypes.Graph{Files: []*repotypes.FileInfo{
		{RelPath: "src/internal/tool.go", Language: "go"},
	}}
	view := NewExecutionView(ExecutionViewOptions{
		Graph:  graph,
		Scopes: []string{"tool.go"},
		ScopeMatcher: func(file, scope string) bool {
			return file == "src/internal/tool.go" && scope == "tool.go"
		},
	})
	if len(view.Files("")) != 1 || !view.ContainsFile("src/internal/tool.go") {
		t.Fatalf("scope matcher did not include qualified file: %+v", view.Files(""))
	}
}

func TestExecutionViewBudgetTruncatesScan(t *testing.T) {
	files := make([]*repotypes.FileInfo, 0, 10)
	for i := 0; i < 10; i++ {
		files = append(files, &repotypes.FileInfo{RelPath: string(rune('a'+i)) + ".go", Language: "go"})
	}
	budget := NewBudget(BudgetOptions{
		TopN:              1,
		RepoFileCount:     FileThreshold + 1,
		GraphFileCount:    FileThreshold + 1,
		ForceAdvisoryOnly: true,
		MaxScanPerRole:    3,
	})
	view := NewExecutionView(ExecutionViewOptions{
		Graph:  &repotypes.Graph{Files: files},
		Scopes: []string{"."},
		Budget: &budget,
	})
	if !view.BudgetTruncated() || view.Complete() {
		t.Fatalf("expected budget truncated incomplete view: complete=%v truncated=%v", view.Complete(), view.BudgetTruncated())
	}
	if got, want := len(view.Files("")), 3; got != want {
		t.Fatalf("scanned files = %d, want %d", got, want)
	}
}
