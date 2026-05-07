package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestPromoteSubTopicAnchor_FilePathLifts pins the qf_imports
// repair: LLM emits the values lane (imports) as PrimaryEntities
// and a file-anchor sub-topic. The helper must lift the file path
// into PrimaryEntities so coherence.go::R1.3 (entity_orphan)
// passes.
func TestPromoteSubTopicAnchor_FilePathLifts(t *testing.T) {
	target := &repotypes.FileInfo{RelPath: "internal/agent/explorer.go", Language: "go"}
	g := &repotypes.Graph{
		Files: []*repotypes.FileInfo{target},
		FileIndex: map[string]*repotypes.FileInfo{
			"internal/agent/explorer.go": target,
		},
	}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(g)
	ctx := &types.AgentContext{RepoRoot: ".", Mutable: mut}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		SubTopics: []types.SubTopic{
			{Summary: "explorer.go", Entities: []string{"internal/agent/explorer.go"}},
		},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"internal/types", "internal/skill", "internal/llm"},
		},
	}
	out := promoteSubTopicFileAnchorToPrimary(ctx, rm)
	if got, want := len(out), 4; got != want {
		t.Fatalf("expected %d entries, got %d (%v)", want, got, out)
	}
	if out[3] != "internal/agent/explorer.go" {
		t.Fatalf("expected file anchor appended last; got %q", out[3])
	}
}

// TestPromoteSubTopicAnchor_BasenameSuffixLifts covers the case
// where the sub-topic anchor is a file basename. The helper
// reuses lookupFileInfoWithSuffix so the resolution mirrors
// path (b)'s suffix-match fallback.
func TestPromoteSubTopicAnchor_BasenameSuffixLifts(t *testing.T) {
	target := &repotypes.FileInfo{RelPath: "internal/agent/explorer.go", Language: "go"}
	g := &repotypes.Graph{
		Files:     []*repotypes.FileInfo{target},
		FileIndex: map[string]*repotypes.FileInfo{"internal/agent/explorer.go": target},
	}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(g)
	ctx := &types.AgentContext{RepoRoot: ".", Mutable: mut}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		SubTopics: []types.SubTopic{
			{Summary: "explorer", Entities: []string{"explorer.go"}},
		},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"internal/types", "internal/skill"},
		},
	}
	out := promoteSubTopicFileAnchorToPrimary(ctx, rm)
	if got, want := len(out), 3; got != want {
		t.Fatalf("expected basename to lift; got %d entries (%v)", got, out)
	}
	if out[2] != "explorer.go" {
		t.Fatalf("expected basename verbatim appended; got %q", out[2])
	}
}

// TestPromoteSubTopicAnchor_InterfaceLifts mirrors the qf_imports
// fix for the implementer-discovery sibling shape: LLM emits
// concrete implementer names as PrimaryEntities and the
// interface as the lone sub-topic anchor.
func TestPromoteSubTopicAnchor_InterfaceLifts(t *testing.T) {
	ifaceFile := &repotypes.FileInfo{RelPath: "iface.go", Language: "go"}
	ifaceSym := &repotypes.Symbol{Name: "Looper", Kind: "interface", File: "iface.go"}
	ifaceSym.ID = repotypes.DeriveSymbolID(ifaceFile, ifaceSym)
	ifaceFile.Symbols = []repotypes.Symbol{*ifaceSym}
	ifaceSymPtr := &ifaceFile.Symbols[0]
	g := &repotypes.Graph{
		Files:      []*repotypes.FileInfo{ifaceFile},
		FileIndex:  map[string]*repotypes.FileInfo{"iface.go": ifaceFile},
		SymbolDefs: map[string][]*repotypes.Symbol{"Looper": {ifaceSymPtr}},
	}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(g)
	ctx := &types.AgentContext{RepoRoot: ".", Mutable: mut}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		SubTopics: []types.SubTopic{
			{Summary: "Looper interface", Entities: []string{"Looper"}},
		},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"evalA", "evalB", "evalC"},
		},
	}
	out := promoteSubTopicFileAnchorToPrimary(ctx, rm)
	if got, want := len(out), 4; got != want {
		t.Fatalf("expected interface to lift; got %d entries (%v)", got, out)
	}
	if out[3] != "Looper" {
		t.Fatalf("expected interface anchor appended; got %q", out[3])
	}
}

// TestPromoteSubTopicAnchor_NoOpWhenAnchorAlreadyPrimary asserts
// the helper does not duplicate when the LLM happened to include
// the anchor in both lanes (overlap already satisfies R1.3).
func TestPromoteSubTopicAnchor_NoOpWhenAnchorAlreadyPrimary(t *testing.T) {
	target := &repotypes.FileInfo{RelPath: "explorer.go"}
	g := &repotypes.Graph{
		FileIndex: map[string]*repotypes.FileInfo{"explorer.go": target},
	}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(g)
	ctx := &types.AgentContext{RepoRoot: ".", Mutable: mut}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		SubTopics: []types.SubTopic{
			{Entities: []string{"explorer.go"}},
		},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"explorer.go", "internal/types"},
		},
	}
	out := promoteSubTopicFileAnchorToPrimary(ctx, rm)
	if got, want := len(out), 2; got != want {
		t.Fatalf("expected no growth (overlap satisfied); got %d (%v)", got, out)
	}
}

// TestPromoteSubTopicAnchor_NoOpWhenAnchorNotInGraph asserts the
// helper refuses to invent: a sub-topic entity that does not
// resolve to any file or interface symbol must not be lifted.
// (The downstream R1.3 fail then surfaces normally so the LLM
// retries with a real anchor.)
func TestPromoteSubTopicAnchor_NoOpWhenAnchorNotInGraph(t *testing.T) {
	g := &repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{}}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(g)
	ctx := &types.AgentContext{RepoRoot: ".", Mutable: mut}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		SubTopics: []types.SubTopic{
			{Entities: []string{"FabricatedAnchor"}},
		},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"a", "b"},
		},
	}
	out := promoteSubTopicFileAnchorToPrimary(ctx, rm)
	if got, want := len(out), 2; got != want {
		t.Fatalf("expected no-op when anchor not in graph; got %d (%v)", got, out)
	}
}

// TestPromoteSubTopicAnchor_NoOpWhenNotEnumeration asserts gate
// #1 — the helper is scoped to enumeration questions and must
// not touch non-enumeration emits.
func TestPromoteSubTopicAnchor_NoOpWhenNotEnumeration(t *testing.T) {
	target := &repotypes.FileInfo{RelPath: "x.go"}
	g := &repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{"x.go": target}}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(g)
	ctx := &types.AgentContext{RepoRoot: ".", Mutable: mut}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: false},
		SubTopics:  []types.SubTopic{{Entities: []string{"x.go"}}},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"a", "b"},
		},
	}
	out := promoteSubTopicFileAnchorToPrimary(ctx, rm)
	if got, want := len(out), 2; got != want {
		t.Fatalf("expected no-op for non-enumeration; got %d (%v)", got, out)
	}
}

// TestPromoteSubTopicAnchor_NoOpWhenSubTopicCountWrong covers
// gate #2: nSub != 1 must short-circuit (≥2 is R1.4 territory,
// 0 is benign).
func TestPromoteSubTopicAnchor_NoOpWhenSubTopicCountWrong(t *testing.T) {
	target := &repotypes.FileInfo{RelPath: "x.go"}
	g := &repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{"x.go": target}}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(g)
	ctx := &types.AgentContext{RepoRoot: ".", Mutable: mut}

	// nSub=2 — out of scope for path (c).
	rm2 := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		SubTopics: []types.SubTopic{
			{Entities: []string{"x.go"}},
			{Entities: []string{"y.go"}},
		},
		AnalyzerHints: types.AnalyzerHints{PrimaryEntities: []string{"a", "b"}},
	}
	if got := promoteSubTopicFileAnchorToPrimary(ctx, rm2); len(got) != 2 {
		t.Fatalf("expected no-op for nSub=2; got %v", got)
	}

	// nSub=0 — no orphan to repair.
	rm0 := types.RequestModel{
		Predicates:    types.SemanticPredicates{IsCategoryEnumeration: true},
		AnalyzerHints: types.AnalyzerHints{PrimaryEntities: []string{"a", "b"}},
	}
	if got := promoteSubTopicFileAnchorToPrimary(ctx, rm0); len(got) != 2 {
		t.Fatalf("expected no-op for nSub=0; got %v", got)
	}

	// nSub=1 with 0 entities — no anchor to lift.
	rmEmpty := types.RequestModel{
		Predicates:    types.SemanticPredicates{IsCategoryEnumeration: true},
		SubTopics:     []types.SubTopic{{Summary: "loose"}},
		AnalyzerHints: types.AnalyzerHints{PrimaryEntities: []string{"a", "b"}},
	}
	if got := promoteSubTopicFileAnchorToPrimary(ctx, rmEmpty); len(got) != 2 {
		t.Fatalf("expected no-op for empty sub_topic.entities; got %v", got)
	}
}

// TestPromoteSubTopicAnchor_NoOpWhenPrimaryUnderTwo guards gate
// #3: with primary count <2 the R1.3 orphan gate doesn't fire,
// so there is nothing to repair (and path (a)/(b) handles
// count==1).
func TestPromoteSubTopicAnchor_NoOpWhenPrimaryUnderTwo(t *testing.T) {
	target := &repotypes.FileInfo{RelPath: "x.go"}
	g := &repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{"x.go": target}}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(g)
	ctx := &types.AgentContext{RepoRoot: ".", Mutable: mut}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		SubTopics:  []types.SubTopic{{Entities: []string{"x.go"}}},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"only-one"},
		},
	}
	out := promoteSubTopicFileAnchorToPrimary(ctx, rm)
	if got, want := len(out), 1; got != want {
		t.Fatalf("expected no-op for primary count<2; got %d (%v)", got, out)
	}
}

// TestSubTopicAnchorResolvesInGraph_KindFilter pins the
// interface-side kind filter: structs / functions never qualify
// as the "container" anchor, only interface / trait / protocol
// kinds.
func TestSubTopicAnchorResolvesInGraph_KindFilter(t *testing.T) {
	fi := &repotypes.FileInfo{RelPath: "x.go"}
	structSym := repotypes.Symbol{Name: "Foo", Kind: "struct", File: "x.go"}
	structSym.ID = repotypes.DeriveSymbolID(fi, &structSym)
	fi.Symbols = []repotypes.Symbol{structSym}
	g := &repotypes.Graph{
		FileIndex:  map[string]*repotypes.FileInfo{},
		SymbolDefs: map[string][]*repotypes.Symbol{"Foo": {&fi.Symbols[0]}},
	}
	if subTopicAnchorResolvesInGraph(g, "Foo") {
		t.Errorf("struct must NOT count as a container anchor")
	}
	// Flip kind to interface and assert true.
	fi.Symbols[0].Kind = "interface"
	if !subTopicAnchorResolvesInGraph(g, "Foo") {
		t.Errorf("interface kind must qualify as anchor")
	}
}
