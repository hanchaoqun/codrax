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
