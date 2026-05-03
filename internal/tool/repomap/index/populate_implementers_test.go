package index

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestPopulateImplementers_GoStructural verifies the typed
// interface-implementation post-pass catches Go's structural
// typing case: a concrete type T satisfies interface I via
// method-set agreement without textually mentioning I.
func TestPopulateImplementers_GoStructural(t *testing.T) {
	src := []byte(`package agent

type Looper interface {
	Observe(s string) bool
}

type evalA struct{}

func (e *evalA) Observe(s string) bool { return true }

type evalB struct{}

// uses _ blank identifier — byte-grep "Observe(s" misses this
func (e *evalB) Observe(_ string) bool { return false }

type unrelated struct{}

func (u *unrelated) DoSomething() {}
`)
	parser := sitter.NewParser()
	parser.SetLanguage(types.GetSitterLanguage(types.LangGo))
	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil || tree == nil {
		t.Skip("tree-sitter-go not available")
	}
	_, syms, _, _ := extractGo(tree.RootNode(), src, "agent.go")
	fi := &types.FileInfo{
		RelPath:  "agent.go",
		Language: types.LangGo,
		Symbols:  syms,
	}
	g := &types.Graph{
		Root:       ".",
		Files:      []*types.FileInfo{fi},
		FileIndex:  map[string]*types.FileInfo{"agent.go": fi},
		SymbolDefs: make(map[string][]*types.Symbol),
		SymbolByID: make(map[types.SymbolID]*types.Symbol),
	}
	for i := range fi.Symbols {
		s := &fi.Symbols[i]
		s.ID = types.DeriveSymbolID(fi, s)
		g.SymbolDefs[s.Name] = append(g.SymbolDefs[s.Name], s)
		g.SymbolByID[s.ID] = s
	}

	populateImplementers(g)

	// Sanity-check: interface symbol has RequiredMethods populated
	var iface *types.Symbol
	for i := range fi.Symbols {
		if fi.Symbols[i].Name == "Looper" && fi.Symbols[i].Kind == "interface" {
			iface = &fi.Symbols[i]
		}
	}
	if iface == nil {
		t.Fatal("Looper interface symbol not found")
	}
	if len(iface.RequiredMethods) == 0 {
		t.Fatalf("Looper.RequiredMethods empty; want at least Observe(1); got %v", iface.RequiredMethods)
	}
	wantSig := "Observe(1)"
	found := false
	for _, m := range iface.RequiredMethods {
		if m == wantSig {
			found = true
		}
	}
	if !found {
		t.Errorf("Looper.RequiredMethods missing %q; got %v", wantSig, iface.RequiredMethods)
	}

	// Both evalA and evalB should implement Looper despite evalB
	// using `_` as parameter name (byte-grep would miss it).
	implementers := g.ImplementersOf("Looper")
	t.Logf("Looper implementers: %d", len(implementers))
	for _, id := range implementers {
		if sym, ok := g.SymbolByID[id]; ok {
			t.Logf("  - %s (Kind=%s)", sym.Name, sym.Kind)
		}
	}
	if len(implementers) < 2 {
		t.Errorf("expected ≥2 implementers; got %d", len(implementers))
	}
	wantNames := map[string]bool{"evalA": true, "evalB": true}
	gotNames := map[string]bool{}
	for _, id := range implementers {
		if sym, ok := g.SymbolByID[id]; ok {
			gotNames[sym.Name] = true
		}
	}
	for w := range wantNames {
		if !gotNames[w] {
			t.Errorf("expected implementer %q in result; got %v", w, gotNames)
		}
	}
	// `unrelated` should NOT be flagged as a Looper implementer
	if gotNames["unrelated"] {
		t.Errorf("unrelated has wrong method set; should not be Looper implementer")
	}
}

// TestPopulateImplementers_NoFalsePositive verifies that types
// missing one or more required methods are NOT marked as
// implementers.
func TestPopulateImplementers_NoFalsePositive(t *testing.T) {
	src := []byte(`package agent

type TwoMethod interface {
	Foo() int
	Bar() string
}

type onlyFoo struct{}

func (o *onlyFoo) Foo() int { return 1 }
`)
	parser := sitter.NewParser()
	parser.SetLanguage(types.GetSitterLanguage(types.LangGo))
	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil || tree == nil {
		t.Skip("tree-sitter-go not available")
	}
	_, syms, _, _ := extractGo(tree.RootNode(), src, "agent.go")
	fi := &types.FileInfo{
		RelPath:  "agent.go",
		Language: types.LangGo,
		Symbols:  syms,
	}
	g := &types.Graph{
		Root:       ".",
		Files:      []*types.FileInfo{fi},
		FileIndex:  map[string]*types.FileInfo{"agent.go": fi},
		SymbolDefs: make(map[string][]*types.Symbol),
		SymbolByID: make(map[types.SymbolID]*types.Symbol),
	}
	for i := range fi.Symbols {
		s := &fi.Symbols[i]
		s.ID = types.DeriveSymbolID(fi, s)
		g.SymbolDefs[s.Name] = append(g.SymbolDefs[s.Name], s)
		g.SymbolByID[s.ID] = s
	}
	populateImplementers(g)
	if got := len(g.ImplementersOf("TwoMethod")); got != 0 {
		t.Errorf("type with subset of required methods should NOT be implementer; got %d implementers", got)
	}
}
