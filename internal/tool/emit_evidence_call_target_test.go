package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func callTargetTestGraph(files ...*repomap.FileInfo) *repomap.Graph {
	g := &repomap.Graph{
		Files:       files,
		FileIndex:   make(map[string]*repomap.FileInfo, len(files)),
		SymbolDefs:  make(map[string][]*repomap.Symbol),
		SymbolByID:  make(map[repomap.SymbolID]*repomap.Symbol),
		MethodIndex: make(map[repomap.MethodKey]*repomap.Symbol),
	}
	for _, fi := range files {
		g.FileIndex[fi.RelPath] = fi
		for i := range fi.Symbols {
			sym := &fi.Symbols[i]
			sym.ID = repomap.DeriveSymbolID(fi, sym)
			g.SymbolDefs[sym.Name] = append(g.SymbolDefs[sym.Name], sym)
			g.SymbolByID[sym.ID] = sym
			receiver := sym.Receiver
			if receiver == "" {
				receiver = sym.Parent
			}
			g.MethodIndex[repomap.MethodKey{Pkg: fi.Package, Receiver: receiver, Name: sym.Name}] = sym
		}
	}
	return g
}

func TestNormalizeCallbackHandoffEvidenceDoesNotMintDirectCall(t *testing.T) {
	gc := &ground.Context{LineIndex: map[string]map[int]string{
		"pipeline/runner.py": {17: "await loop.run_in_executor(None, plugin.handle, payload)"},
	}}
	item := types.EvidenceItem{
		Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		Source: "pipeline/runner.py", LineStart: 17,
		AnchorKind: types.AnchorCall, AnchorSymbol: "plugin.handle",
		Subject: "run_pipeline", Predicate: "calls", Object: "plugin.handle",
	}
	if !normalizeCallbackHandoffEvidence(&item, gc) {
		t.Fatal("expected exact callback argument to be reclassified")
	}
	if item.AnchorKind != types.AnchorCallback || item.Subject != "loop.run_in_executor" ||
		item.Object != "plugin.handle" || item.Predicate != "passes callback" {
		t.Fatalf("unexpected normalized callback item: %+v", item)
	}
	if got := types.ClaimFormOf(item); got != types.ClaimCallbackHandoff {
		t.Fatalf("claim form=%q want %q", got, types.ClaimCallbackHandoff)
	}
}

func TestNormalizeCallbackHandoffEvidenceConsumesPromptPreReadAuthority(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.RecordPreReadSource("pipeline/runner.py", []string{
		"async def run_pipeline(kind, payload):",
		"    return await loop.run_in_executor(None, plugin.handle, payload)",
	})
	gc := ground.BuildContext(&types.BusContext{Mutable: mut})
	item := types.EvidenceItem{
		Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		Source: "pipeline/runner.py", LineStart: 2,
		AnchorKind: types.AnchorCall, AnchorSymbol: "plugin.handle",
		Subject: "run_pipeline", Predicate: "calls", Object: "plugin.handle",
	}
	if !normalizeCallbackHandoffEvidence(&item, gc) {
		t.Fatal("prompt pre-read should provide the exact-line authority for callback normalization")
	}
	if item.AnchorKind != types.AnchorCallback || item.Subject != "loop.run_in_executor" || item.Object != "plugin.handle" {
		t.Fatalf("unexpected pre-read callback normalization: %+v", item)
	}
	if report := ground.GroundItemScoped(&item, gc); report.Status != types.GroundingGrounded {
		t.Fatalf("normalized pre-read callback did not ground: report=%+v item=%+v", report, item)
	}
}

func TestStampEvidenceTypedIdentityBindingsUsesExactParserDeclarations(t *testing.T) {
	const source = "src/pipeline.go"
	graph := callTargetTestGraph(&repomap.FileInfo{
		RelPath: source, Language: repomap.LangGo,
		Symbols: []repomap.Symbol{
			{Name: "busCtx", Kind: "field", Parent: "Orchestrator", DeclaredType: "*types.BusContext", Line: 5, EndLine: 5},
			{Name: "applyStageOutput", Kind: "method", Parent: "Orchestrator", Line: 10, EndLine: 30},
		},
	})
	gc := &ground.Context{Graph: graph}
	declaration := types.EvidenceItem{
		Kind: types.EvidenceDirect, Subject: "busCtx", Source: source, LineStart: 5,
		Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "busCtx",
		GroundingStatus: types.GroundingGrounded,
	}
	if !stampEvidenceTypedIdentityBindings(&declaration, gc) {
		t.Fatal("exact typed declaration should stamp identity metadata")
	}
	if declaration.DeclaredBinding != "Orchestrator.busCtx" || declaration.DeclaredType != "*types.BusContext" || declaration.DeclaredOwner != "Orchestrator" {
		t.Fatalf("unexpected declaration identity metadata: %+v", declaration)
	}

	operation := types.EvidenceItem{
		Kind: types.EvidenceRelationship, Subject: "o.busCtx.EvidenceItems", Object: "output.EvidenceItems",
		Source: source, LineStart: 20, Scope: types.ScopeLine, AnchorKind: types.AnchorAssignment,
		GroundingStatus: types.GroundingGrounded,
	}
	if !stampEvidenceTypedIdentityBindings(&operation, gc) || operation.OwnerIdentity != "Orchestrator.applyStageOutput" {
		t.Fatalf("operation should carry its exact parser-owned callable identity: %+v", operation)
	}
	if operation.DeclaredBinding != "" || operation.DeclaredType != "" {
		t.Fatalf("an operation must not inherit declaration identity directly: %+v", operation)
	}
}

func TestStampEvidenceTypedIdentityBindingsFailsClosedOnAmbiguousOrUntypedDeclaration(t *testing.T) {
	const source = "src/pipeline.go"
	for _, tc := range []struct {
		name    string
		symbols []repomap.Symbol
	}{
		{name: "untyped", symbols: []repomap.Symbol{{Name: "busCtx", Kind: "field", Parent: "Orchestrator", Line: 5, EndLine: 5}}},
		{name: "ambiguous", symbols: []repomap.Symbol{
			{Name: "busCtx", Kind: "field", Parent: "Left", DeclaredType: "BusContext", Line: 5, EndLine: 5},
			{Name: "busCtx", Kind: "field", Parent: "Right", DeclaredType: "BusContext", Line: 5, EndLine: 5},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gc := &ground.Context{Graph: callTargetTestGraph(&repomap.FileInfo{RelPath: source, Language: repomap.LangGo, Symbols: tc.symbols})}
			item := types.EvidenceItem{
				Kind: types.EvidenceDirect, Subject: "busCtx", Source: source, LineStart: 5,
				Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "busCtx",
				GroundingStatus: types.GroundingGrounded,
			}
			stampEvidenceTypedIdentityBindings(&item, gc)
			if item.DeclaredBinding != "" || item.DeclaredType != "" || item.DeclaredOwner != "" {
				t.Fatalf("%s declaration must not publish a typed alias: %+v", tc.name, item)
			}
		})
	}
}

func TestNormalizeCallEvidenceDirectionPrefersResolvedSemanticCallee(t *testing.T) {
	caller := &repomap.FileInfo{
		RelPath: "VisitController.java", Language: repomap.LangJava, Package: "com.clinic",
		Symbols: []repomap.Symbol{{Name: "create", Kind: "method", Parent: "VisitController", File: "VisitController.java", Line: 10, EndLine: 20}},
		Relations: []repomap.Relation{{
			Kind: "call", File: "VisitController.java", Line: 18,
			ToEP:       repomap.RelationEndpoint{Name: "schedule", Receiver: "VisitService", File: "VisitController.java", Line: 18},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "java_method_invocation",
		}},
	}
	target := &repomap.FileInfo{
		RelPath: "VisitService.java", Language: repomap.LangJava, Package: "com.clinic",
		Symbols: []repomap.Symbol{{Name: "schedule", Kind: "method", Parent: "VisitService", File: "VisitService.java", Line: 8, EndLine: 16}},
	}
	graph := callTargetTestGraph(caller, target)
	gc := &ground.Context{
		Graph: graph,
		LineIndex: map[string]map[int]string{
			"VisitController.java": {18: "return service.schedule(petId, reason);"},
		},
	}
	ev := types.EvidenceItem{
		AnchorKind: types.AnchorCall, AnchorSymbol: "schedule",
		Subject: "VisitController.create", Predicate: "calls", Object: "service.schedule",
		Source: "VisitController.java", LineStart: 18,
	}
	if !normalizeCallEvidenceDirection(&ev, gc) {
		t.Fatal("expected semantic target normalization to change the receiver expression")
	}
	if ev.Subject != "VisitController.create" || ev.Object != "VisitService.schedule" {
		t.Fatalf("normalized edge=%q -> %q, want VisitController.create -> VisitService.schedule", ev.Subject, ev.Object)
	}
}

func TestNormalizeCallEvidenceDirectionKeepsSourceExpressionWhenGraphTargetUnresolved(t *testing.T) {
	caller := &repomap.FileInfo{
		RelPath: "Dynamic.java", Language: repomap.LangJava, Package: "com.clinic",
		Symbols: []repomap.Symbol{{Name: "run", Kind: "method", Parent: "Dynamic", File: "Dynamic.java", Line: 1, EndLine: 5}},
		Relations: []repomap.Relation{{
			Kind: "call", File: "Dynamic.java", Line: 3,
			ToEP:       repomap.RelationEndpoint{Name: "dispatch", File: "Dynamic.java", Line: 3},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "java_method_invocation",
		}},
	}
	graph := callTargetTestGraph(caller)
	gc := &ground.Context{
		Graph: graph,
		LineIndex: map[string]map[int]string{
			"Dynamic.java": {3: "runtime.dispatch(job);"},
		},
	}
	ev := types.EvidenceItem{
		AnchorKind: types.AnchorCall, AnchorSymbol: "dispatch",
		Subject: "Dynamic.run", Predicate: "calls", Object: "runtime.dispatch",
		Source: "Dynamic.java", LineStart: 3,
	}
	normalizeCallEvidenceDirection(&ev, gc)
	if ev.Object != "runtime.dispatch" {
		t.Fatalf("unresolved dynamic target=%q, want byte-exact source expression", ev.Object)
	}
}

func TestNormalizeCallEvidenceDirectionKeepsRustInlineWrapperAndCoreDistinct(t *testing.T) {
	fi := &repomap.FileInfo{
		RelPath: "src/lib.rs", Language: repomap.LangRust, Package: "core",
		Symbols: []repomap.Symbol{
			{Name: "tokenize_bytes", Kind: "function", File: "src/lib.rs", Line: 10, EndLine: 18, Arity: 2},
			{Name: "tokenize_bytes", Kind: "function", Parent: "py", File: "src/lib.rs", Line: 40, EndLine: 43, Arity: 2},
		},
		Relations: []repomap.Relation{{
			Kind: "call", File: "src/lib.rs", Line: 42,
			FromEP:     repomap.RelationEndpoint{Name: "tokenize_bytes", Receiver: "py", File: "src/lib.rs", Line: 42},
			ToEP:       repomap.RelationEndpoint{Name: "tokenize_bytes", Receiver: "super", File: "src/lib.rs", Line: 42},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "rust_ast_scoped_call",
		}},
	}
	graph := callTargetTestGraph(fi)
	gc := &ground.Context{
		Graph: graph,
		LineIndex: map[string]map[int]string{
			"src/lib.rs": {42: "super::tokenize_bytes(&data, &table)"},
		},
	}
	ev := types.EvidenceItem{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
		AnchorSymbol: "tokenize_bytes", Predicate: "calls",
		Subject: "tokenize_bytes", Object: "tokenize_bytes",
		Source: "src/lib.rs", LineStart: 42,
	}
	if !normalizeCallEvidenceDirection(&ev, gc) {
		t.Fatal("expected nested Rust call direction to gain lexical owner identity")
	}
	if ev.Subject != "py.tokenize_bytes" || ev.Object != "tokenize_bytes" {
		t.Fatalf("normalized edge=%q -> %q, want py.tokenize_bytes -> tokenize_bytes", ev.Subject, ev.Object)
	}
}

func TestNormalizeCallEvidenceDirectionUsesOwningSubRepoGraphForCaller(t *testing.T) {
	fi := &repomap.FileInfo{
		RelPath: "fastlex/tokenizer.py", Language: repomap.LangPython, Package: "fastlex",
		Symbols: []repomap.Symbol{{
			Name: "tokenize", Kind: "method", Parent: "FastTokenizer",
			File: "fastlex/tokenizer.py", Line: 18, EndLine: 23,
		}},
		Relations: []repomap.Relation{{
			Kind: "call", File: "fastlex/tokenizer.py", Line: 21,
			FromEP:     repomap.RelationEndpoint{Name: "tokenize", Receiver: "FastTokenizer", File: "fastlex/tokenizer.py", Line: 21},
			ToEP:       repomap.RelationEndpoint{Name: "tokenize_bytes", Receiver: "_fastlex", File: "fastlex/tokenizer.py", Line: 21},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "python_call",
		}},
	}
	ownerGraph := callTargetTestGraph(fi)
	primaryGraph := callTargetTestGraph(&repomap.FileInfo{RelPath: "src/lib.rs", Language: repomap.LangRust})
	gc := &ground.Context{
		Graph: primaryGraph,
		LineIndex: map[string]map[int]string{
			"bindings-py/fastlex/tokenizer.py": {21: "return _fastlex.tokenize_bytes(list(data), self._merges)"},
		},
		SourceGraphFile: func(source string) (*repomap.Graph, *repomap.FileInfo, string, bool) {
			if source != "bindings-py/fastlex/tokenizer.py" {
				return nil, nil, "", false
			}
			return ownerGraph, fi, "fastlex/tokenizer.py", true
		},
	}
	ev := types.EvidenceItem{
		Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		AnchorKind: types.AnchorCall, AnchorSymbol: "tokenize_bytes",
		Subject: "_fastlex", Predicate: "calls", Object: "tokenize_bytes",
		Source: "bindings-py/fastlex/tokenizer.py", LineStart: 21, LineEnd: 21,
	}
	if !normalizeCallEvidenceDirection(&ev, gc) {
		t.Fatal("expected owner-routed graph to replace the model-authored caller")
	}
	if ev.Subject != "FastTokenizer.tokenize" || ev.Object != "_fastlex.tokenize_bytes" || ev.AnchorSymbol != "_fastlex.tokenize_bytes" {
		t.Fatalf("normalized edge=%q -> %q anchor=%q, want FastTokenizer.tokenize -> _fastlex.tokenize_bytes", ev.Subject, ev.Object, ev.AnchorSymbol)
	}
	if stabilizeUnprovenCallAnchorAuthority(&ev, gc) {
		t.Fatalf("owner-routed exact call was downgraded: %+v", ev)
	}
}

type emitEvidenceSourceGraphStub struct {
	graph  *repomap.Graph
	file   *repomap.FileInfo
	source string
}

func (s *emitEvidenceSourceGraphStub) SourceGraphFile(source string) (*repomap.Graph, *repomap.FileInfo, string, bool) {
	if s == nil || source != "repo-b/"+s.source {
		return nil, nil, "", false
	}
	return s.graph, s.file, s.source, true
}

func TestAttachEmitEvidenceSourceGraphResolverPinsMultiRepoWiring(t *testing.T) {
	fi := &repomap.FileInfo{RelPath: "src/main.c", Language: repomap.LangC}
	graph := callTargetTestGraph(fi)
	gc := &ground.Context{}
	ctx := &types.BusContext{MultiGraph: &emitEvidenceSourceGraphStub{graph: graph, file: fi, source: "src/main.c"}}
	attachEmitEvidenceSourceGraphResolver(ctx, gc)
	gotGraph, gotFile, gotSource, visibleSource, ok := ground.ResolveSourceGraphFile(gc, "repo-b/src/main.c")
	if !ok || gotGraph != graph || gotFile != fi || gotSource != "src/main.c" || visibleSource != "repo-b/src/main.c" {
		t.Fatalf("owner resolver wiring = (%p, %p, %q, %q, %t)", gotGraph, gotFile, gotSource, visibleSource, ok)
	}
}

func TestStabilizeUnprovenCallAnchorRejectsModelCallerWhenOwnerGraphCannotProveIt(t *testing.T) {
	fi := &repomap.FileInfo{
		RelPath: "fastlex/tokenizer.py", Language: repomap.LangPython,
		Relations: []repomap.Relation{{
			Kind: "call", File: "fastlex/tokenizer.py", Line: 21,
			ToEP:       repomap.RelationEndpoint{Name: "tokenize_bytes", Receiver: "_fastlex", File: "fastlex/tokenizer.py", Line: 21},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "python_call",
		}},
	}
	ownerGraph := callTargetTestGraph(fi)
	gc := &ground.Context{
		LineIndex: map[string]map[int]string{
			"bindings-py/fastlex/tokenizer.py": {21: "return _fastlex.tokenize_bytes(list(data), self._merges)"},
		},
		SourceGraphFile: func(source string) (*repomap.Graph, *repomap.FileInfo, string, bool) {
			return ownerGraph, fi, "fastlex/tokenizer.py", true
		},
	}
	ev := types.EvidenceItem{
		Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		AnchorKind: types.AnchorCall, AnchorSymbol: "tokenize_bytes",
		Subject: "invented_caller", Predicate: "calls", Object: "_fastlex.tokenize_bytes",
		Source: "bindings-py/fastlex/tokenizer.py", LineStart: 21, LineEnd: 21,
	}
	if !stabilizeUnprovenCallAnchorAuthority(&ev, gc) {
		t.Fatal("expected directed call authority to be removed when owning graph has no enclosing caller")
	}
	if ev.AnchorKind != types.AnchorTextReference || ev.Subject != "" || ev.Predicate != "" || ev.Object != "" {
		t.Fatalf("unexpected downgraded item: %+v", ev)
	}
}
