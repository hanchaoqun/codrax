package repomap

import "testing"

// TestMakeSymbolID locks the canonical string format so no consumer
// accidentally breaks it by string concatenation tricks. The format
// is public API for the drift-proof index: `<lang>::<pkg>::<receiver>
// ::<name>::<arity>`, with empty segments rendered as empty strings.
func TestMakeSymbolID(t *testing.T) {
	cases := []struct {
		lang, pkg, recv, name string
		arity                 int
		want                  SymbolID
	}{
		{"go", "agent", "BaseAgent", "Execute", 2, "go::agent::BaseAgent::Execute::2"},
		{"go", "agent", "", "buildAnalysisIR", 1, "go::agent::::buildAnalysisIR::1"},
		{"go", "types", "", "TaskItem", 0, "go::types::::TaskItem::0"},
		{"go", "repomap", "Graph", "CallersOf", 1, "go::repomap::Graph::CallersOf::1"},
		{"python", "foo.bar", "MyClass", "__init__", 3, "python::foo.bar::MyClass::__init__::3"},
	}
	for _, c := range cases {
		got := MakeSymbolID(c.lang, c.pkg, c.recv, c.name, c.arity)
		if got != c.want {
			t.Errorf("MakeSymbolID(%q,%q,%q,%q,%d) = %q, want %q",
				c.lang, c.pkg, c.recv, c.name, c.arity, got, c.want)
		}
	}
}

// TestBuildGraphPopulatesSymbolByID verifies that BuildGraph
// derives a SymbolID for every symbol with a known language and
// inserts it into the canonical SymbolByID index. This is the
// drift-proof replacement for SymbolDefs string keying that Phase
// 1 consumers will migrate to.
func TestBuildGraphPopulatesSymbolByID(t *testing.T) {
	// Two symbols sharing the name "Execute" on different receivers —
	// the classic receiver-drift corpus. Pre-Phase-1 they collapsed
	// into SymbolDefs["Execute"] with 2 entries and no way to tell
	// apart; post-Phase-1 they get distinct SymbolIDs.
	files := []*FileInfo{
		{
			RelPath:  "internal/agent/tool_a.go",
			Language: LangGo,
			Package:  "agent",
			Symbols: []Symbol{
				{Name: "Execute", Kind: "method", Receiver: "ToolA", Arity: 2, File: "internal/agent/tool_a.go", Line: 10},
				{Name: "Name", Kind: "method", Receiver: "ToolA", Arity: 0, File: "internal/agent/tool_a.go", Line: 20},
			},
		},
		{
			RelPath:  "internal/agent/tool_b.go",
			Language: LangGo,
			Package:  "agent",
			Symbols: []Symbol{
				{Name: "Execute", Kind: "method", Receiver: "ToolB", Arity: 2, File: "internal/agent/tool_b.go", Line: 10},
				{Name: "Name", Kind: "method", Receiver: "ToolB", Arity: 0, File: "internal/agent/tool_b.go", Line: 20},
			},
		},
	}
	g := BuildGraph("/tmp/repo", files)

	// Legacy index still has 2 Executes under one name.
	if len(g.SymbolDefs["Execute"]) != 2 {
		t.Errorf("legacy SymbolDefs[Execute] = %d, want 2", len(g.SymbolDefs["Execute"]))
	}

	// Canonical index distinguishes them by receiver.
	idA := MakeSymbolID("go", "agent", "ToolA", "Execute", 2)
	idB := MakeSymbolID("go", "agent", "ToolB", "Execute", 2)
	if _, ok := g.SymbolByID[idA]; !ok {
		t.Errorf("SymbolByID missing %q", idA)
	}
	if _, ok := g.SymbolByID[idB]; !ok {
		t.Errorf("SymbolByID missing %q", idB)
	}
	if g.SymbolByID[idA] == g.SymbolByID[idB] {
		t.Error("SymbolByID collapsed two distinct receivers to one entry")
	}

	// Every symbol should have its ID populated post-BuildGraph.
	for _, fi := range g.Files {
		for i := range fi.Symbols {
			s := &fi.Symbols[i]
			if s.ID == "" {
				t.Errorf("Symbol %s has empty ID", s.Name)
			}
		}
	}

	// The index size should equal the total symbol count when no two
	// symbols collide. 2 files × 2 symbols = 4.
	if len(g.SymbolByID) != 4 {
		t.Errorf("SymbolByID size = %d, want 4", len(g.SymbolByID))
	}
}

// TestDeriveSymbolIDFallsBackToDirForNoPackage verifies that
// languages without a package concept (JS file-scoped, C) still get
// distinct IDs for same-named symbols in different directories,
// using filepath.Dir as the synthetic package.
func TestDeriveSymbolIDFallsBackToDirForNoPackage(t *testing.T) {
	files := []*FileInfo{
		{
			RelPath:  "src/a/util.js",
			Language: LangJavaScript,
			Package:  "", // JS file-scoped
			Symbols: []Symbol{
				{Name: "helper", Kind: "function", Arity: 1, File: "src/a/util.js", Line: 5},
			},
		},
		{
			RelPath:  "src/b/util.js",
			Language: LangJavaScript,
			Package:  "",
			Symbols: []Symbol{
				{Name: "helper", Kind: "function", Arity: 1, File: "src/b/util.js", Line: 5},
			},
		},
	}
	g := BuildGraph("/tmp/repo", files)

	// Legacy index sees one name, two defs.
	if len(g.SymbolDefs["helper"]) != 2 {
		t.Errorf("legacy SymbolDefs[helper] = %d, want 2", len(g.SymbolDefs["helper"]))
	}
	// Canonical index distinguishes by directory.
	if len(g.SymbolByID) != 2 {
		t.Errorf("SymbolByID size = %d, want 2 (two distinct dirs)", len(g.SymbolByID))
	}
}
