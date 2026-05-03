package agent

import (
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestImplementerFilesFromGraph_TypedSubset verifies P2 #4's typed
// override returns only the files containing concrete implementers
// of the named interface, not every file the keyword ranker would
// surface for the interface's name.
func TestImplementerFilesFromGraph_TypedSubset(t *testing.T) {
	// Build a tiny synthetic graph: one Looper interface, two
	// concrete implementers in distinct files, one unrelated file
	// that mentions Looper in a string literal but does NOT
	// implement the interface (the keyword-ranker case the typed
	// path should bypass).
	ifaceFile := &repotypes.FileInfo{RelPath: "iface.go", Language: "go"}
	implAFile := &repotypes.FileInfo{RelPath: "impl_a.go", Language: "go"}
	implBFile := &repotypes.FileInfo{RelPath: "impl_b.go", Language: "go"}
	noiseFile := &repotypes.FileInfo{RelPath: "noise.go", Language: "go"}

	ifaceSym := &repotypes.Symbol{Name: "Looper", Kind: "interface", File: "iface.go"}
	ifaceSym.ID = repotypes.DeriveSymbolID(ifaceFile, ifaceSym)
	ifaceFile.Symbols = []repotypes.Symbol{*ifaceSym}

	implA := repotypes.Symbol{Name: "evalA", Kind: "struct", File: "impl_a.go", Implements: []repotypes.SymbolID{ifaceSym.ID}}
	implA.ID = repotypes.DeriveSymbolID(implAFile, &implA)
	implAFile.Symbols = []repotypes.Symbol{implA}

	implB := repotypes.Symbol{Name: "evalB", Kind: "struct", File: "impl_b.go", Implements: []repotypes.SymbolID{ifaceSym.ID}}
	implB.ID = repotypes.DeriveSymbolID(implBFile, &implB)
	implBFile.Symbols = []repotypes.Symbol{implB}

	// noise has no Implements link — keyword ranker would have
	// matched on a comment / string / unrelated method, but the
	// typed override must NOT include it.
	noiseSym := repotypes.Symbol{Name: "unrelated", Kind: "struct", File: "noise.go"}
	noiseSym.ID = repotypes.DeriveSymbolID(noiseFile, &noiseSym)
	noiseFile.Symbols = []repotypes.Symbol{noiseSym}

	// Re-pin canonical ifaceSym pointer in defs so ImplementersOf's
	// SymbolDefs lookup hits.
	ifaceSymPtr := &ifaceFile.Symbols[0]
	g := &repotypes.Graph{
		Files:      []*repotypes.FileInfo{ifaceFile, implAFile, implBFile, noiseFile},
		FileIndex:  map[string]*repotypes.FileInfo{"iface.go": ifaceFile, "impl_a.go": implAFile, "impl_b.go": implBFile, "noise.go": noiseFile},
		SymbolDefs: map[string][]*repotypes.Symbol{"Looper": {ifaceSymPtr}},
		SymbolByID: map[repotypes.SymbolID]*repotypes.Symbol{
			ifaceSymPtr.ID: ifaceSymPtr,
			implA.ID:       &implAFile.Symbols[0],
			implB.ID:       &implBFile.Symbols[0],
			noiseSym.ID:    &noiseFile.Symbols[0],
		},
	}
	files := implementerFilesFromGraph(g, []string{"Looper"})
	if len(files) != 2 {
		t.Fatalf("expected 2 implementer files; got %d (%v)", len(files), files)
	}
	wantSeen := map[string]bool{"impl_a.go": false, "impl_b.go": false}
	for _, f := range files {
		if _, ok := wantSeen[f]; ok {
			wantSeen[f] = true
		} else {
			t.Errorf("unexpected file in result: %q", f)
		}
	}
	for f, seen := range wantSeen {
		if !seen {
			t.Errorf("expected %q in result; got %v", f, files)
		}
	}
}

func TestImplementerFilesFromGraph_NilGuards(t *testing.T) {
	if got := implementerFilesFromGraph(nil, []string{"X"}); got != nil {
		t.Errorf("expected nil on nil graph; got %v", got)
	}
	g := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{}}
	if got := implementerFilesFromGraph(g, nil); got != nil {
		t.Errorf("expected nil on nil entities; got %v", got)
	}
	if got := implementerFilesFromGraph(g, []string{""}); got != nil {
		t.Errorf("expected nil on empty entity; got %v", got)
	}
}

func TestImplementerFilesFromGraph_NoMatchingInterfaceReturnsNil(t *testing.T) {
	// A symbol exists but its Kind=struct, so it doesn't qualify as
	// the "interface side" of the implements relation.
	fi := &repotypes.FileInfo{RelPath: "x.go", Language: "go"}
	sym := repotypes.Symbol{Name: "Foo", Kind: "struct", File: "x.go"}
	sym.ID = repotypes.DeriveSymbolID(fi, &sym)
	fi.Symbols = []repotypes.Symbol{sym}
	g := &repotypes.Graph{
		Files:      []*repotypes.FileInfo{fi},
		SymbolDefs: map[string][]*repotypes.Symbol{"Foo": {&fi.Symbols[0]}},
		SymbolByID: map[repotypes.SymbolID]*repotypes.Symbol{sym.ID: &fi.Symbols[0]},
	}
	if got := implementerFilesFromGraph(g, []string{"Foo"}); got != nil {
		t.Errorf("expected nil when no interface matches; got %v", got)
	}
}
