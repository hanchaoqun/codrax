package index

import (
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
