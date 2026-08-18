package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// The cross-package call fallback resolves through a memoized
// (receiver, name) index. It must find a method defined in another
// package and resolve deterministically to the first definition in
// file order (the old whole-MethodIndex scan returned a random pick).
func TestResolveCallTarget_CrossPackageDeterministic(t *testing.T) {
	fa := &types.FileInfo{RelPath: "a/store.go", Language: "go", Package: "a",
		Symbols: []types.Symbol{{Name: "Get", Receiver: "Store", Kind: "method"}}}
	fb := &types.FileInfo{RelPath: "b/store.go", Language: "go", Package: "b",
		Symbols: []types.Symbol{{Name: "Get", Receiver: "Store", Kind: "method"}}}
	caller := &types.FileInfo{RelPath: "c/use.go", Language: "go", Package: "c",
		Relations: []types.Relation{{Kind: "call", ToEP: types.RelationEndpoint{Name: "Get", Receiver: "Store"},
			Confidence: types.ConfidenceAST, Provenance: types.ProvenanceTreeSitter, ResolvedBy: "test_fixture"}}}
	g := BuildGraph(".", []*types.FileInfo{fa, fb, caller})

	var first *types.Symbol
	for i := 0; i < 20; i++ {
		got := g.ResolveCallTarget(caller, caller.Relations[0])
		if got == nil {
			t.Fatal("cross-package call must resolve")
		}
		if first == nil {
			first = got
		} else if got != first {
			t.Fatal("resolution must be deterministic across calls")
		}
	}
	if first != &fa.Symbols[0] {
		t.Fatalf("must resolve to the first-in-order package's symbol, got %q", first.ID)
	}
}

func TestResolveCallTarget_SamePackagePreferred(t *testing.T) {
	local := &types.FileInfo{RelPath: "p/a.go", Language: "go", Package: "p",
		Symbols: []types.Symbol{{Name: "Run", Receiver: "Job", Kind: "method"}},
		Relations: []types.Relation{{Kind: "call", ToEP: types.RelationEndpoint{Name: "Run", Receiver: "Job"},
			Confidence: types.ConfidenceAST, Provenance: types.ProvenanceTreeSitter, ResolvedBy: "test_fixture"}}}
	other := &types.FileInfo{RelPath: "q/b.go", Language: "go", Package: "q",
		Symbols: []types.Symbol{{Name: "Run", Receiver: "Job", Kind: "method"}}}
	g := BuildGraph(".", []*types.FileInfo{local, other})
	got := g.ResolveCallTarget(local, local.Relations[0])
	if got != &local.Symbols[0] {
		t.Fatalf("same-package method must win, got %+v", got)
	}
}

func TestResolveCallTarget_ResolvedImportAliasFindsPackageFunction(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.test/project\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	module := &types.FileInfo{RelPath: "go.mod", IsSpecial: true}
	target := &types.FileInfo{RelPath: "internal/context/builder.go", Language: types.LangGo, Package: "context",
		Symbols: []types.Symbol{{Name: "BuildAgentContext", Kind: "function", File: "internal/context/builder.go", Line: 26}}}
	caller := &types.FileInfo{RelPath: "internal/orchestrator/extract_work.go", Language: types.LangGo, Package: "orchestrator",
		Imports: []types.Import{{Path: "example.test/project/internal/context", Alias: "ctxbuilder"}},
		Relations: []types.Relation{{Kind: "call", ToEP: types.RelationEndpoint{Name: "BuildAgentContext", Receiver: "ctxbuilder"},
			Confidence: types.ConfidenceAST, Provenance: types.ProvenanceTreeSitter, ResolvedBy: "go_call"}}}
	g := BuildGraph(repo, []*types.FileInfo{module, target, caller})
	if got := g.ResolvedImports[caller.RelPath]; len(got) != 1 || len(got[0].Targets) != 1 || got[0].Targets[0] != target.RelPath {
		t.Fatalf("graph must preserve the exact resolved import declaration: %+v", got)
	}
	if got := g.ResolveCallTarget(caller, caller.Relations[0]); got != &target.Symbols[0] {
		t.Fatalf("explicit import alias must resolve the package function, got %+v", got)
	}

	second := &types.FileInfo{RelPath: "other/context/builder.go", Language: types.LangGo, Package: "context",
		Symbols: []types.Symbol{{Name: "BuildAgentContext", Kind: "function", File: "other/context/builder.go", Line: 8}}}
	g = BuildGraph(repo, []*types.FileInfo{module, target, second, caller})
	g.ResolvedImports[caller.RelPath] = []types.ResolvedImportBinding{{
		Import: caller.Imports[0], Targets: []string{target.RelPath, second.RelPath},
	}}
	if got := g.ResolveCallTarget(caller, caller.Relations[0]); got != nil {
		t.Fatalf("ambiguous imported definitions must fail closed, got %+v", got)
	}
}
