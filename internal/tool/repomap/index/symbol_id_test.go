package index

import (
	"testing"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestMakeSymbolID locks the canonical string format so no consumer
// accidentally breaks it by string concatenation tricks. The format
// is public API for the drift-proof index: `<lang>::<pkg>::<receiver>
// ::<name>::<arity>`, with empty segments rendered as empty strings.
func TestMakeSymbolID(t *testing.T) {
	cases := []struct {
		lang, pkg, recv, name string
		arity                 int
		want                  types.SymbolID
	}{
		{"go", "agent", "BaseAgent", "Execute", 2, "go::agent::BaseAgent::Execute::2"},
		{"go", "agent", "", "buildAnalysisIR", 1, "go::agent::::buildAnalysisIR::1"},
		{"go", "types", "", "TaskItem", 0, "go::types::::TaskItem::0"},
		{"go", "repomap", "types.Graph", "CallersOf", 1, "go::repomap::types.Graph::CallersOf::1"},
		{"python", "foo.bar", "MyClass", "__init__", 3, "python::foo.bar::MyClass::__init__::3"},
	}
	for _, c := range cases {
		got := types.MakeSymbolID(c.lang, c.pkg, c.recv, c.name, c.arity)
		if got != c.want {
			t.Errorf("types.MakeSymbolID(%q,%q,%q,%q,%d) = %q, want %q",
				c.lang, c.pkg, c.recv, c.name, c.arity, got, c.want)
		}
	}
}

// TestBuildGraphPopulatesSymbolByID verifies that BuildGraph
// derives a types.SymbolID for every symbol with a known language and
// inserts it into the canonical SymbolByID index. This is the
// drift-proof replacement for SymbolDefs string keying that Phase
// 1 consumers will migrate to.
func TestBuildGraphPopulatesSymbolByID(t *testing.T) {
	// Two symbols sharing the name "Execute" on different receivers —
	// the classic receiver-drift corpus. Pre-Phase-1 they collapsed
	// into SymbolDefs["Execute"] with 2 entries and no way to tell
	// apart; post-Phase-1 they get distinct SymbolIDs.
	files := []*types.FileInfo{
		{
			RelPath:  "internal/agent/tool_a.go",
			Language: types.LangGo,
			Package:  "agent",
			Symbols: []types.Symbol{
				{Name: "Execute", Kind: "method", Receiver: "ToolA", Arity: 2, File: "internal/agent/tool_a.go", Line: 10},
				{Name: "Name", Kind: "method", Receiver: "ToolA", Arity: 0, File: "internal/agent/tool_a.go", Line: 20},
			},
		},
		{
			RelPath:  "internal/agent/tool_b.go",
			Language: types.LangGo,
			Package:  "agent",
			Symbols: []types.Symbol{
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
	idA := types.MakeSymbolID("go", "agent", "ToolA", "Execute", 2)
	idB := types.MakeSymbolID("go", "agent", "ToolB", "Execute", 2)
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
				t.Errorf("types.Symbol %s has empty ID", s.Name)
			}
		}
	}

	// The index size should equal the total symbol count when no two
	// symbols collide. 2 files × 2 symbols = 4.
	if len(g.SymbolByID) != 4 {
		t.Errorf("SymbolByID size = %d, want 4", len(g.SymbolByID))
	}
}

// TestCallersOfIDReceiverAware verifies that CallersOfID filters
// callers by the target's receiver type instead of collapsing every
// method that shares a name. Set up: two types ToolA and ToolB both
// define `Execute`; two caller files each call one of them, with
// the scope-resolved receiver populated on ToEP.
func TestCallersOfIDReceiverAware(t *testing.T) {
	files := []*types.FileInfo{
		{
			RelPath:  "agent/tool_a.go",
			Language: types.LangGo,
			Package:  "agent",
			Symbols: []types.Symbol{
				{Name: "Execute", Kind: "method", Receiver: "ToolA", Arity: 0, File: "agent/tool_a.go", Line: 10},
				{Name: "ToolA", Kind: "type", File: "agent/tool_a.go", Line: 5},
			},
		},
		{
			RelPath:  "agent/tool_b.go",
			Language: types.LangGo,
			Package:  "agent",
			Symbols: []types.Symbol{
				{Name: "Execute", Kind: "method", Receiver: "ToolB", Arity: 0, File: "agent/tool_b.go", Line: 10},
				{Name: "ToolB", Kind: "type", File: "agent/tool_b.go", Line: 5},
			},
		},
		{
			RelPath:  "agent/caller_a.go",
			Language: types.LangGo,
			Package:  "agent",
			Relations: []types.Relation{
				{Kind: "call", From: "agent/caller_a.go", To: "Execute", File: "agent/caller_a.go", Line: 20,
					ToEP: types.RelationEndpoint{Name: "Execute", Receiver: "ToolA", File: "agent/caller_a.go", Line: 20}},
			},
		},
		{
			RelPath:  "agent/caller_b.go",
			Language: types.LangGo,
			Package:  "agent",
			Relations: []types.Relation{
				{Kind: "call", From: "agent/caller_b.go", To: "Execute", File: "agent/caller_b.go", Line: 20,
					ToEP: types.RelationEndpoint{Name: "Execute", Receiver: "ToolB", File: "agent/caller_b.go", Line: 20}},
			},
		},
	}
	g := BuildGraph("/tmp/repo", files)

	// Legacy CallersOf returns BOTH caller files — the pre-Phase-1
	// receiver-drift bug. Kept for backward-compat verification.
	legacy := g.CallersOf("Execute")
	if len(legacy) != 2 {
		t.Errorf("legacy CallersOf(Execute) = %v, want 2 files", legacy)
	}

	// CallersOfID for ToolA.Execute returns ONLY caller_a.
	idA := types.MakeSymbolID("go", "agent", "ToolA", "Execute", 0)
	gotA := g.CallersOfID(idA)
	if len(gotA) != 1 || gotA[0] != "agent/caller_a.go" {
		t.Errorf("CallersOfID(ToolA.Execute) = %v, want [agent/caller_a.go]", gotA)
	}
	// CallersOfID for ToolB.Execute returns ONLY caller_b.
	idB := types.MakeSymbolID("go", "agent", "ToolB", "Execute", 0)
	gotB := g.CallersOfID(idB)
	if len(gotB) != 1 || gotB[0] != "agent/caller_b.go" {
		t.Errorf("CallersOfID(ToolB.Execute) = %v, want [agent/caller_b.go]", gotB)
	}

	// MethodIndex should have entries for both methods + both types.
	if _, ok := g.MethodIndex[types.MethodKey{Pkg: "agent", Receiver: "ToolA", Name: "Execute"}]; !ok {
		t.Error("MethodIndex missing ToolA.Execute")
	}
	if _, ok := g.MethodIndex[types.MethodKey{Pkg: "agent", Receiver: "ToolB", Name: "Execute"}]; !ok {
		t.Error("MethodIndex missing ToolB.Execute")
	}
}

// TestResolveCallTargetFallbacks verifies the multi-step resolution
// order in resolveCallTarget: same-package receiver match, bare
// function fallback, cross-package receiver scan.
func TestResolveCallTargetFallbacks(t *testing.T) {
	files := []*types.FileInfo{
		{
			RelPath:  "agent/a.go",
			Language: types.LangGo,
			Package:  "agent",
			Symbols: []types.Symbol{
				{Name: "helper", Kind: "function", Arity: 1, File: "agent/a.go", Line: 5},
			},
		},
		{
			RelPath:  "repomap/g.go",
			Language: types.LangGo,
			Package:  "repomap",
			Symbols: []types.Symbol{
				{Name: "CallersOf", Kind: "method", Receiver: "types.Graph", Arity: 1, File: "repomap/g.go", Line: 10},
				{Name: "types.Graph", Kind: "type", File: "repomap/g.go", Line: 5},
			},
		},
	}
	g := BuildGraph("/tmp/repo", files)

	callerFile := &types.FileInfo{RelPath: "agent/caller.go", Package: "agent", Language: types.LangGo}

	// Same-package bare function: resolves via Pkg="agent", Receiver="".
	rel := types.Relation{Kind: "call", To: "helper", ToEP: types.RelationEndpoint{Name: "helper"}}
	if s := g.ResolveCallTarget(callerFile, rel); s == nil || s.Name != "helper" {
		t.Errorf("bare-function resolution failed: got %+v", s)
	}

	// Cross-package method call: scope-resolved receiver "types.Graph" in
	// "agent" package should resolve to the "repomap" package method
	// via the linear cross-package scan.
	rel = types.Relation{Kind: "call", To: "CallersOf", ToEP: types.RelationEndpoint{Name: "CallersOf", Receiver: "types.Graph"}}
	if s := g.ResolveCallTarget(callerFile, rel); s == nil || s.Name != "CallersOf" {
		t.Errorf("cross-pkg resolution failed: got %+v", s)
	}

	// Unresolved: unknown method/receiver.
	rel = types.Relation{Kind: "call", To: "NoSuchThing", ToEP: types.RelationEndpoint{Name: "NoSuchThing", Receiver: "NoSuchType"}}
	if s := g.ResolveCallTarget(callerFile, rel); s != nil {
		t.Errorf("unresolved call should return nil, got %+v", s)
	}
}

// TestDeriveSymbolIDFallsBackToDirForNoPackage verifies that
// languages without a package concept (JS file-scoped, C) still get
// distinct IDs for same-named symbols in different directories,
// using filepath.Dir as the synthetic package.
func TestDeriveSymbolIDFallsBackToDirForNoPackage(t *testing.T) {
	files := []*types.FileInfo{
		{
			RelPath:  "src/a/util.js",
			Language: types.LangJavaScript,
			Package:  "", // JS file-scoped
			Symbols: []types.Symbol{
				{Name: "helper", Kind: "function", Arity: 1, File: "src/a/util.js", Line: 5},
			},
		},
		{
			RelPath:  "src/b/util.js",
			Language: types.LangJavaScript,
			Package:  "",
			Symbols: []types.Symbol{
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
