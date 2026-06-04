package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTryExactMultiRepoFocus_FindsOwningSubRepo(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "repo-tools-py/tools/processor.py", "def process_request(req):\n    return req\n")
	writeTestFile(t, root, "repo-stub-rust/src/lib.rs", "pub fn unrelated_constant() -> i32 {\n    42\n}\n")

	mg := testExactFocusMultiGraph(t, root, 3, []topology.SubRepo{
		testSubRepo(root, "repo-tools-py", "python"),
		testSubRepo(root, "repo-stub-rust", "rust"),
	})

	decision := tryExactMultiRepoFocus(mg, "函数 unrelated_constant 是在哪个文件里定义的？它返回什么值？")
	if decision == nil {
		t.Fatal("expected exact pre-scan decision")
	}
	if decision.Source != types.MultiRepoFocusSourceExactPrescan {
		t.Fatalf("source=%q, want exact_prescan", decision.Source)
	}
	if len(decision.Candidates) != 1 {
		t.Fatalf("candidates=%+v, want exactly one", decision.Candidates)
	}
	if decision.Candidates[0].RootRel != "repo-stub-rust" {
		t.Fatalf("candidate root=%q, want repo-stub-rust", decision.Candidates[0].RootRel)
	}
	if decision.Candidates[0].Reason == "" {
		t.Fatal("expected reason with exact hit sample")
	}
}

func TestTryExactMultiRepoFocus_NoCodeLikeTokenFallsBack(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "repo-a/main.go", "package main\n")
	writeTestFile(t, root, "repo-b/main.go", "package main\n")

	mg := testExactFocusMultiGraph(t, root, 2, []topology.SubRepo{
		testSubRepo(root, "repo-a", "go"),
		testSubRepo(root, "repo-b", "go"),
	})

	if got := tryExactMultiRepoFocus(mg, "请概括这个工作区的整体架构"); got != nil {
		t.Fatalf("expected nil for prose-only request, got %+v", got)
	}
}

func TestTryExactMultiRepoFocus_OverCapDoesNotTrim(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "repo-a/a.go", "package a\nfunc SharedThing() {}\n")
	writeTestFile(t, root, "repo-b/b.go", "package b\nfunc SharedThing() {}\n")
	writeTestFile(t, root, "repo-c/c.go", "package c\nfunc SharedThing() {}\n")

	mg := testExactFocusMultiGraph(t, root, 2, []topology.SubRepo{
		testSubRepo(root, "repo-a", "go"),
		testSubRepo(root, "repo-b", "go"),
		testSubRepo(root, "repo-c", "go"),
	})

	if got := tryExactMultiRepoFocus(mg, "SharedThing 在哪里定义？"); got != nil {
		t.Fatalf("expected nil when exact hit set exceeds cap instead of silent trim, got %+v", got)
	}
}

func TestExtractMultiRepoExactFocusTokens_CodeShapesOnly(t *testing.T) {
	got := extractMultiRepoExactFocusTokens("where is unrelated_constant and GreetServiceImpl defined, and what does echo return?")
	want := map[string]bool{"unrelated_constant": true, "GreetServiceImpl": true}
	if len(got) != len(want) {
		t.Fatalf("tokens=%v, want exactly %v", got, want)
	}
	for _, token := range got {
		if !want[token] {
			t.Fatalf("unexpected token %q from %v", token, got)
		}
	}
}

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testSubRepo(parentRoot, rootRel, lang string) topology.SubRepo {
	return topology.SubRepo{
		Slug:         rootRel,
		RootAbs:      filepath.Join(parentRoot, filepath.FromSlash(rootRel)),
		RootRel:      rootRel,
		PrimaryLangs: []string{lang},
		FileCount:    1,
	}
}

func testExactFocusMultiGraph(t *testing.T, root string, cap int, repos []topology.SubRepo) *multigraph.MultiGraph {
	t.Helper()
	mg, err := multigraph.New(multigraph.Config{
		Topology: &topology.RepoTopology{
			ParentRoot: root,
			ParentSlug: "parent",
			Repos:      repos,
		},
		Build: func(_, _ string) (*rmtypes.Graph, error) {
			return &rmtypes.Graph{}, nil
		},
		Cap: cap,
	})
	if err != nil {
		t.Fatal(err)
	}
	return mg
}
