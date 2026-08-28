package tool

import (
	"encoding/json"
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
	if len(operation.DeclaredIdentityBindings) != 1 ||
		operation.DeclaredIdentityBindings[0] != (types.EvidenceDeclaredIdentityBinding{
			Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
		}) {
		t.Fatalf("operation should carry its exact parser-owned endpoint binding: %+v", operation.DeclaredIdentityBindings)
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

func TestStampEvidenceTypedIdentityBindingsOperationFailsClosedWithoutExactOwnerOrEndpoint(t *testing.T) {
	const source = "src/pipeline.go"
	graph := callTargetTestGraph(&repomap.FileInfo{
		RelPath: source, Language: repomap.LangGo,
		Symbols: []repomap.Symbol{
			{Name: "busCtx", Kind: "field", Parent: "Orchestrator", DeclaredType: "*types.BusContext", Line: 5, EndLine: 5},
			{Name: "apply", Kind: "method", Parent: "Worker", Line: 10, EndLine: 19},
			{Name: "apply", Kind: "method", Parent: "Orchestrator", Line: 20, EndLine: 30},
		},
	})
	for _, tc := range []struct {
		name    string
		symbols []repomap.Symbol
		subject string
		line    int
	}{
		{name: "wrong owner", subject: "o.busCtx.Items", line: 15},
		{name: "missing endpoint segment", subject: "o.other.Items", line: 25},
		{name: "ambiguous static type", subject: "o.busCtx.Items", line: 25, symbols: []repomap.Symbol{
			{Name: "busCtx", Kind: "field", Parent: "Orchestrator", DeclaredType: "*types.OtherContext", Line: 6, EndLine: 6},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caseGraph := graph
			if len(tc.symbols) > 0 {
				file := *graph.FileIndex[source]
				file.Symbols = append(append([]repomap.Symbol(nil), file.Symbols...), tc.symbols...)
				caseGraph = callTargetTestGraph(&file)
			}
			caseGC := &ground.Context{Graph: caseGraph}
			item := types.EvidenceItem{
				Kind: types.EvidenceRelationship, Subject: tc.subject, Object: "value.Items",
				Source: source, LineStart: tc.line, Scope: types.ScopeLine, AnchorKind: types.AnchorAssignment,
				GroundingStatus: types.GroundingGrounded,
			}
			stampEvidenceTypedIdentityBindings(&item, caseGC)
			if len(item.DeclaredIdentityBindings) != 0 {
				t.Fatalf("non-exact operation must not publish endpoint bindings: %+v", item.DeclaredIdentityBindings)
			}
		})
	}
}

func TestEmitEvidenceStampsExactOperationBindingWithoutDeclarationEvidence(t *testing.T) {
	const source = "src/pipeline.go"
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, source, 20, "o.busCtx.AnalysisIR = output.AnalysisIR")
	ctx.Mutable.SetSearchGraph(callTargetTestGraph(&repomap.FileInfo{
		RelPath: source, Language: repomap.LangGo,
		Symbols: []repomap.Symbol{
			{Name: "busCtx", Kind: "field", Parent: "Orchestrator", DeclaredType: "*types.BusContext", Line: 5, EndLine: 5},
			{Name: "applyStageOutput", Kind: "method", Parent: "Orchestrator", Line: 10, EndLine: 30},
		},
	}))
	params := json.RawMessage(`{"items":[{
		"scope":"line","evidence_kind":"relationship","subject":"o.busCtx.AnalysisIR",
		"predicate":"assigns","object":"output.AnalysisIR","source":"src/pipeline.go","line_start":20,
		"summary":"the stage result is assigned to the shared context","anchor_kind":"assignment",
		"anchor_symbol":"o.busCtx.AnalysisIR","snippet":"o.busCtx.AnalysisIR = output.AnalysisIR"
	}]}`)
	res, err := (&EmitEvidence{}).Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("Execute: err=%v result=%+v", err, res)
	}
	items := ctx.Mutable.EmittedEvidence()
	if len(items) != 1 {
		t.Fatalf("emitted items=%d, want 1", len(items))
	}
	item := items[0]
	if item.OwnerIdentity != "Orchestrator.applyStageOutput" || len(item.DeclaredIdentityBindings) != 1 ||
		item.DeclaredIdentityBindings[0].Type != "*types.BusContext" ||
		item.DeclaredIdentityBindings[0].Binding != "Orchestrator.busCtx" {
		t.Fatalf("exact operation lost parser-owned identity metadata: %+v", item)
	}
	if got := types.FlowOperationEvidence(items); len(got) != 1 {
		t.Fatalf("identity metadata must leave the model-authored operation authoritative: %+v", got)
	}
}

func TestEmitEvidenceStampsCallableParameterBindingOnExactOperationEndpoint(t *testing.T) {
	const source = "internal/context/builder.go"
	graph := callTargetTestGraph(&repomap.FileInfo{
		RelPath: source, Language: repomap.LangGo, Package: "context",
		Symbols: []repomap.Symbol{{
			Name: "BuildAgentContext", Kind: "function", File: source, Line: 26, EndLine: 80,
			ParameterBindings: []repomap.CallableParameterBinding{{Binding: "bus", Type: "*types.BusContext"}},
		}},
	})
	item := types.EvidenceItem{
		Kind: types.EvidenceRelationship, Subject: "Mutable", Predicate: "assigns", Object: "bus.Mutable",
		Source: source, LineStart: 59, Scope: types.ScopeLine, AnchorKind: types.AnchorInitializer,
		AnchorSymbol: "Mutable", Snippet: "Mutable: bus.Mutable,", GroundingStatus: types.GroundingGrounded,
	}
	stampEvidenceTypedIdentityBindings(&item, &ground.Context{Graph: graph})
	if item.OwnerIdentity != "context.BuildAgentContext" || len(item.DeclaredIdentityBindings) != 1 ||
		item.DeclaredIdentityBindings[0].Binding != "context.BuildAgentContext.bus" ||
		item.DeclaredIdentityBindings[0].Type != "*types.BusContext" {
		t.Fatalf("exact callable parameter identity was not stamped on the operation: %+v", item)
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

func TestNormalizeCallEvidenceDirectionPreservesSourceQualifierForResolvedTail(t *testing.T) {
	caller := &repomap.FileInfo{
		RelPath: "analyzer.go", Language: repomap.LangGo, Package: "agent",
		Symbols: []repomap.Symbol{{Name: "buildAnalysisIR", Kind: "function", File: "analyzer.go", Line: 1, EndLine: 20}},
		Relations: []repomap.Relation{{
			Kind: "call", File: "analyzer.go", Line: 8,
			ToEP:       repomap.RelationEndpoint{Name: "Evaluate", File: "risk.go", Line: 3},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "go_call_expression",
		}},
	}
	target := &repomap.FileInfo{
		RelPath: "risk.go", Language: repomap.LangGo,
		Symbols: []repomap.Symbol{{Name: "Evaluate", Kind: "function", File: "risk.go", Line: 3, EndLine: 5}},
	}
	gc := &ground.Context{
		Graph:     callTargetTestGraph(caller, target),
		LineIndex: map[string]map[int]string{"analyzer.go": {8: "rm.Risk = risk.Evaluate(rm)"}},
	}
	ev := types.EvidenceItem{
		AnchorKind: types.AnchorCall, AnchorSymbol: "Evaluate",
		Subject: "buildAnalysisIR", Predicate: "calls", Object: "risk.Evaluate",
		Source: "analyzer.go", LineStart: 8,
	}
	if !normalizeCallEvidenceDirection(&ev, gc) {
		t.Fatal("expected call identity normalization")
	}
	if ev.Subject != "buildAnalysisIR" || ev.Object != "risk.Evaluate" || ev.AnchorSymbol != "risk.Evaluate" {
		t.Fatalf("normalized edge=%q -> %q anchor=%q, want exact source-qualified risk.Evaluate", ev.Subject, ev.Object, ev.AnchorSymbol)
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
