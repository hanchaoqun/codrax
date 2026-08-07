package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeTargetRelationEvaluator(graph *repomap.Graph) *explorerEvaluator {
	return &explorerEvaluator{
		searchResult: &keywordSearchResult{Graph: graph},
		analysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PredicateAxis: types.AxisCall,
			CallChainEndpointProfile: &types.CallChainEndpointProfile{
				Source:   "run_pipeline",
				SinkMode: types.CallChainSinkResolutionDiscover,
			},
		}},
	}
}

func TestBuildConcreteValuesSection_StandaloneStructuralRelationsReachHandoff(t *testing.T) {
	file := &repotypes.FileInfo{
		RelPath:  "pipeline/plugins.py",
		Language: repotypes.LangPython,
		Relations: []repotypes.Relation{
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "JsonPlugin"}, ToEP: repotypes.RelationEndpoint{Name: "TimestampMixin"}, File: "pipeline/plugins.py", Line: 8, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "python_base_class"},
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "JsonPlugin"}, ToEP: repotypes.RelationEndpoint{Name: "ValidationMixin"}, File: "pipeline/plugins.py", Line: 8, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "python_base_class"},
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "JsonPlugin"}, ToEP: repotypes.RelationEndpoint{Name: "BasePlugin"}, File: "pipeline/plugins.py", Line: 8, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "python_base_class"},
			// Same apparent shape, but regex salvage is not eligible for a
			// typed/citable structural relation.
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "Noise"}, ToEP: repotypes.RelationEndpoint{Name: "Guess"}, File: "pipeline/plugins.py", Line: 9, Confidence: repotypes.ConfidenceRegexSalvage, Provenance: repotypes.ProvenanceRegexFallback, ResolvedBy: "regex_guess"},
		},
	}
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{file})
	eval := runtimeTargetRelationEvaluator(graph)

	got := eval.buildConcreteValuesSection(context.Background(), t.TempDir(), map[string]bool{"pipeline/plugins.py": true}, nil)
	if len(got.evidence) != 3 {
		t.Fatalf("standalone AST relations must survive the no-concrete-value path: got %d items: %+v", len(got.evidence), got.evidence)
	}
	wantOrder := []string{"TimestampMixin", "ValidationMixin", "BasePlugin"}
	for i, item := range got.evidence {
		if item.Producer != types.EvidenceProducerRepoMapStructuralRelation || item.Kind != types.EvidenceRelationship ||
			item.Predicate != "inheritance" || item.Subject != "JsonPlugin" || !item.IsCitable() {
			t.Fatalf("unexpected structural relation evidence: %+v", item)
		}
		if item.Object == "Guess" {
			t.Fatalf("regex salvage must not become typed structural evidence: %+v", item)
		}
		if item.Object != wantOrder[i] || item.RelationOrdinal != i+1 {
			t.Fatalf("declared relation order was not preserved: index=%d got=%s ordinal=%d want=%s ordinal=%d; all=%+v",
				i, item.Object, item.RelationOrdinal, wantOrder[i], i+1, got.evidence)
		}
	}
	for _, want := range []string{
		"## Typed Type Relations",
		"structural type relationships, not invocation edges",
		"`JsonPlugin`",
		"`TimestampMixin`",
		"`ValidationMixin`",
		"`BasePlugin`",
	} {
		if !strings.Contains(got.markdown, want) {
			t.Fatalf("structural relation markdown missing %q:\n%s", want, got.markdown)
		}
	}
	if strings.Contains(got.markdown, "Noise") || strings.Contains(got.markdown, "Guess") {
		t.Fatalf("regex relation leaked into structural markdown:\n%s", got.markdown)
	}
}

func TestBuildRuntimeTargetStructuralRelations_ImplicitImplementersReachTypedHandoff(t *testing.T) {
	ifaceFile := &repotypes.FileInfo{
		RelPath: "contract.go", Language: repotypes.LangGo, Package: "sample",
		Symbols: []repotypes.Symbol{{Name: "LoopController", Kind: "interface", File: "contract.go", Line: 3, EndLine: 5, RequiredMethods: []string{"Observe(1)"}}},
	}
	implFile := &repotypes.FileInfo{
		RelPath: "controller.go", Language: repotypes.LangGo, Package: "sample",
		Symbols: []repotypes.Symbol{
			{Name: "workerController", Kind: "struct", File: "controller.go", Line: 7, EndLine: 9},
			{Name: "Observe", Kind: "method", File: "controller.go", Receiver: "workerController", Line: 12, EndLine: 15, Arity: 1},
		},
	}
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{ifaceFile, implFile})
	if got := graph.ImplementersOf("LoopController"); len(got) != 1 {
		t.Fatalf("fixture must establish one typed implementer, got %+v; interface=%+v concrete=%+v", got, ifaceFile.Symbols, implFile.Symbols)
	}
	eval := &explorerEvaluator{
		predicateAxis: types.AxisImplement,
		analysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PredicateAxis: types.AxisImplement,
			AnalyzerHints: types.AnalyzerHints{PrimaryEntities: []string{"LoopController"}},
		}},
	}
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(map[string]bool{"controller.go": true})
	closure.AddReadRanges(map[string][]types.LineRange{"controller.go": {{Start: 7, End: 7}}})

	got := eval.buildRuntimeTargetStructuralRelations(graph, map[string]bool{"controller.go": true}, map[string]bool{"controller.go": true}, closure)
	if len(got.evidence) != 1 {
		t.Fatalf("typed ImplementersOf relation must reach the evidence handoff: %+v", got.evidence)
	}
	item := got.evidence[0]
	if item.Subject != "workerController" || item.Predicate != "implements" || item.Object != "LoopController" ||
		item.Producer != types.EvidenceProducerRepoMapImplementerRelation || !item.IsCitable() {
		t.Fatalf("unexpected implicit implementer evidence: %+v", item)
	}
	if !strings.Contains(got.markdown, "typed_implementers_of") {
		t.Fatalf("typed relation authority must be visible to synthesis:\n%s", got.markdown)
	}
}

func TestBuildRuntimeTargetStructuralRelations_ImplicitImplementerRequiresExactDeclarationRead(t *testing.T) {
	ifaceFile := &repotypes.FileInfo{
		RelPath: "contract.go", Language: repotypes.LangGo, Package: "sample",
		Symbols: []repotypes.Symbol{{Name: "LoopController", Kind: "interface", File: "contract.go", Line: 3, RequiredMethods: []string{"Observe(1)"}}},
	}
	implFile := &repotypes.FileInfo{
		RelPath: "controller.go", Language: repotypes.LangGo, Package: "sample",
		Symbols: []repotypes.Symbol{
			{Name: "workerController", Kind: "struct", File: "controller.go", Line: 70},
			{Name: "Observe", Kind: "method", File: "controller.go", Receiver: "workerController", Line: 80, Arity: 1},
		},
	}
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{ifaceFile, implFile})
	eval := &explorerEvaluator{
		predicateAxis: types.AxisImplement,
		analysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PredicateAxis: types.AxisImplement,
			AnalyzerHints: types.AnalyzerHints{PrimaryEntities: []string{"LoopController"}},
		}},
	}
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(map[string]bool{"controller.go": true})
	closure.AddReadRanges(map[string][]types.LineRange{"controller.go": {{Start: 1, End: 20}}})

	got := eval.buildRuntimeTargetStructuralRelations(graph, map[string]bool{"controller.go": true}, map[string]bool{"controller.go": true}, closure)
	if len(got.evidence) != 0 {
		t.Fatalf("an unread concrete declaration must not mint a citable implementation edge: %+v", got.evidence)
	}
}

func TestBuildConcreteValuesSection_ExactReadRelationsSurviveActiveFrontierChange(t *testing.T) {
	file := &repotypes.FileInfo{
		RelPath:  "pipeline/plugins.py",
		Language: repotypes.LangPython,
		Relations: []repotypes.Relation{
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "JsonPlugin"}, ToEP: repotypes.RelationEndpoint{Name: "TimestampMixin"}, File: "pipeline/plugins.py", Line: 18, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "python_base_class"},
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "JsonPlugin"}, ToEP: repotypes.RelationEndpoint{Name: "ValidationMixin"}, File: "pipeline/plugins.py", Line: 18, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "python_base_class"},
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "JsonPlugin"}, ToEP: repotypes.RelationEndpoint{Name: "BasePlugin"}, File: "pipeline/plugins.py", Line: 18, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "python_base_class"},
		},
	}
	repoRoot := t.TempDir()
	graph := repomap.BuildGraph(repoRoot, []*repotypes.FileInfo{file})
	eval := runtimeTargetRelationEvaluator(graph)
	// Simulate a later exploration focus in a different directory. Broad
	// concrete-value scanning may legitimately leave plugins.py behind, but
	// its already-read declaration line remains authoritative.
	eval.exactAnchorFiles = []string{"other/anchor.py"}
	readSet := map[string]bool{"pipeline/plugins.py": true}
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(readSet)
	closure.AddReadRanges(map[string][]types.LineRange{"pipeline/plugins.py": {{Start: 18, End: 18}}})
	frontier := eval.activeFrontierFileSet(readSet, "")
	if frontier["pipeline/plugins.py"] {
		t.Fatalf("fixture must reproduce the volatile-frontier exclusion: %+v", frontier)
	}

	got := eval.buildConcreteValuesSection(context.Background(), repoRoot, readSet, closure)
	if len(got.evidence) != 3 {
		t.Fatalf("all exactly-read structural relations must survive frontier changes: %+v", got.evidence)
	}
	for i, want := range []string{"TimestampMixin", "ValidationMixin", "BasePlugin"} {
		if got.evidence[i].Object != want || got.evidence[i].RelationOrdinal != i+1 {
			t.Fatalf("source declaration order lost after frontier change: %+v", got.evidence)
		}
	}
	merged := mergeEvidenceItems(got.evidence)
	if len(merged) != 3 {
		t.Fatalf("compound relations from one declaration line must survive the unified evidence merge: %+v", merged)
	}
}

func TestBuildRuntimeTargetStructuralRelations_RequiresExactReadLine(t *testing.T) {
	file := &repotypes.FileInfo{
		RelPath:  "src/types.cj",
		Language: repotypes.LangCangjie,
		Relations: []repotypes.Relation{
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "FastHandler"}, ToEP: repotypes.RelationEndpoint{Name: "Handler"}, File: "src/types.cj", Line: 5, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceCangjieParser, ResolvedBy: "cangjie_inheritance_clause"},
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "HiddenHandler"}, ToEP: repotypes.RelationEndpoint{Name: "Handler"}, File: "src/types.cj", Line: 45, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceCangjieParser, ResolvedBy: "cangjie_inheritance_clause"},
		},
	}
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{file})
	eval := runtimeTargetRelationEvaluator(graph)
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(map[string]bool{"src/types.cj": true})
	closure.AddReadRanges(map[string][]types.LineRange{"src/types.cj": {{Start: 1, End: 10}}})

	got := eval.buildRuntimeTargetStructuralRelations(graph, map[string]bool{"src/types.cj": true}, map[string]bool{"src/types.cj": true}, closure)
	if len(got.evidence) != 1 || got.evidence[0].Subject != "FastHandler" {
		t.Fatalf("only the precisely read relation line may be promoted: %+v", got.evidence)
	}
	if strings.Contains(got.markdown, "HiddenHandler") {
		t.Fatalf("unread structural relation line leaked into prompt:\n%s", got.markdown)
	}
}

func TestBuildRuntimeTargetCooperativeCalls_PromotesExactReadSameOperationSuperCall(t *testing.T) {
	file := &repotypes.FileInfo{
		RelPath: "pipeline/base.py", Language: repotypes.LangPython,
		Symbols: []repotypes.Symbol{
			{Name: "TimestampMixin", Kind: "class", File: "pipeline/base.py", Line: 36, EndLine: 43},
			{Name: "handle", Kind: "method", Parent: "TimestampMixin", File: "pipeline/base.py", Line: 39, EndLine: 42},
			{Name: "helper", Kind: "method", Parent: "TimestampMixin", File: "pipeline/base.py", Line: 45, EndLine: 48},
		},
		Relations: []repotypes.Relation{
			{Kind: "call", ToEP: repotypes.RelationEndpoint{Name: "handle", Receiver: "super()"}, File: "pipeline/base.py", Line: 42, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "python_ast_attribute_call"},
			// An explicit super call to a different operation is real source
			// evidence but is not a cooperative same-operation handoff.
			{Kind: "call", ToEP: repotypes.RelationEndpoint{Name: "close", Receiver: "super()"}, File: "pipeline/base.py", Line: 47, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "python_ast_attribute_call"},
		},
	}
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{file})
	eval := runtimeTargetRelationEvaluator(graph)
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(map[string]bool{"pipeline/base.py": true})
	closure.AddReadRanges(map[string][]types.LineRange{"pipeline/base.py": {{Start: 39, End: 42}}})

	got := eval.buildRuntimeTargetCooperativeCalls(graph, map[string]bool{"pipeline/base.py": true}, map[string]bool{"pipeline/base.py": true}, closure)
	if len(got.evidence) != 1 {
		t.Fatalf("exact same-operation super call must reach typed handoff once: %+v", got.evidence)
	}
	item := got.evidence[0]
	if item.Subject != "TimestampMixin.handle" || item.Predicate != "cooperative_super_call" ||
		item.Object != "super.handle" || item.OwnerSymbol != "TimestampMixin.handle" ||
		item.Producer != types.EvidenceProducerRepoMapCooperativeCall || types.ClaimFormOf(item) != types.ClaimCallEdge || !item.IsCitable() {
		t.Fatalf("unexpected cooperative delegation evidence: %+v", item)
	}
	for _, want := range []string{"## Typed Cooperative Delegations", "`TimestampMixin.handle`", "`super.handle`", "concrete next implementation still depends"} {
		if !strings.Contains(got.markdown, want) {
			t.Fatalf("cooperative delegation markdown missing %q:\n%s", want, got.markdown)
		}
	}
}

func TestRuntimeTargetExplicitSuperCall_SupportedExtractorMatrixAndStandDown(t *testing.T) {
	for _, tc := range []struct {
		name       string
		resolvedBy string
		receiver   string
		provenance string
		want       bool
	}{
		{name: "python", resolvedBy: "python_ast_attribute_call", receiver: "super()", provenance: repotypes.ProvenanceTreeSitter, want: true},
		{name: "java", resolvedBy: "java_method_invocation", receiver: "super", provenance: repotypes.ProvenanceTreeSitter, want: true},
		{name: "javascript-typescript-arkts", resolvedBy: "js_ast_member_call", receiver: "super", provenance: repotypes.ProvenanceTreeSitter, want: true},
		{name: "kotlin", resolvedBy: "kotlin_ast_navigation_call", receiver: "super", provenance: repotypes.ProvenanceTreeSitter, want: true},
		{name: "swift", resolvedBy: "swift_ast_navigation_call", receiver: "super", provenance: repotypes.ProvenanceTreeSitter, want: true},
		{name: "cangjie", resolvedBy: "cangjie_token_call", receiver: "super", provenance: repotypes.ProvenanceCangjieParser, want: true},
		{name: "ordinary receiver", resolvedBy: "java_method_invocation", receiver: "service", provenance: repotypes.ProvenanceTreeSitter, want: false},
		{name: "regex cannot authorize", resolvedBy: "java_method_invocation", receiver: "super", provenance: repotypes.ProvenanceRegexFallback, want: false},
		{name: "rust module super is not base dispatch", resolvedBy: "rust_ast_scoped_call", receiver: "super", provenance: repotypes.ProvenanceTreeSitter, want: false},
		{name: "unknown extractor", resolvedBy: "future_guess", receiver: "super", provenance: repotypes.ProvenanceTreeSitter, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rel := repotypes.Relation{Kind: "call", ToEP: repotypes.RelationEndpoint{Name: "handle", Receiver: tc.receiver}, Confidence: repotypes.ConfidenceAST, Provenance: tc.provenance, ResolvedBy: tc.resolvedBy}
			if got := runtimeTargetExplicitSuperCall(rel); got != tc.want {
				t.Fatalf("runtimeTargetExplicitSuperCall(%+v)=%v, want %v", rel, got, tc.want)
			}
		})
	}
}

func TestGetConcreteValuesCached_RebuildsWhenExactReadCoverageExpands(t *testing.T) {
	file := &repotypes.FileInfo{
		RelPath:  "pipeline/plugins.py",
		Language: repotypes.LangPython,
		Relations: []repotypes.Relation{
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "CsvPlugin"}, ToEP: repotypes.RelationEndpoint{Name: "ValidationMixin"}, File: "pipeline/plugins.py", Line: 9, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "python_base_class"},
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "JsonPlugin"}, ToEP: repotypes.RelationEndpoint{Name: "TimestampMixin"}, File: "pipeline/plugins.py", Line: 18, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "python_base_class"},
		},
	}
	repoRoot := t.TempDir()
	graph := repomap.BuildGraph(repoRoot, []*repotypes.FileInfo{file})
	eval := runtimeTargetRelationEvaluator(graph)
	closure := types.NewEvidenceClosure("")
	readSet := map[string]bool{"pipeline/plugins.py": true}
	closure.SetReadSet(readSet)
	closure.AddReadRanges(map[string][]types.LineRange{"pipeline/plugins.py": {{Start: 1, End: 10}}})

	first := eval.getConcreteValuesCached(context.Background(), repoRoot, readSet, closure)
	if len(first.evidence) != 1 || first.evidence[0].Subject != "CsvPlugin" {
		t.Fatalf("initial partial coverage should expose only CsvPlugin: %+v", first.evidence)
	}

	closure.AddReadRanges(map[string][]types.LineRange{"pipeline/plugins.py": {{Start: 11, End: 23}}})
	second := eval.getConcreteValuesCached(context.Background(), repoRoot, readSet, closure)
	if len(second.evidence) != 2 {
		t.Fatalf("expanded exact coverage must invalidate stale relation cache: %+v", second.evidence)
	}
	foundJSON := false
	for _, item := range second.evidence {
		if item.Subject == "JsonPlugin" && item.Object == "TimestampMixin" {
			foundJSON = true
		}
	}
	if !foundJSON {
		t.Fatalf("newly read JsonPlugin relation missing after cache rebuild: %+v", second.evidence)
	}
}

func TestBuildRuntimeTargetStructuralRelations_InactiveForUnrelatedQuestion(t *testing.T) {
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{{
		RelPath:   "x.go",
		Relations: []repotypes.Relation{{Kind: "embedding", FromEP: repotypes.RelationEndpoint{Name: "X"}, ToEP: repotypes.RelationEndpoint{Name: "Y"}, File: "x.go", Line: 1, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_struct_embedding"}},
	}})
	eval := &explorerEvaluator{analysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{PredicateAxis: types.AxisDefine}}}
	got := eval.buildRuntimeTargetStructuralRelations(graph, map[string]bool{"x.go": true}, map[string]bool{"x.go": true}, nil)
	if len(got.evidence) != 0 || got.markdown != "" {
		t.Fatalf("unrelated requests must not inherit runtime-target prompt load: %+v", got)
	}
}
