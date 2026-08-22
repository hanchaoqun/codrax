package agent

import (
	"context"
	"os"
	"path/filepath"
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

func TestDeterministicEnrichmentFileSet_AcceptedCompletionUsesOnlyReadFiles(t *testing.T) {
	eval := &explorerEvaluator{
		investigationComplete:     true,
		exactAnchorFiles:          []string{"src/exact.go"},
		declarativeAnchorFiles:    []string{"src/declaration.go"},
		declarativeCandidateFiles: []string{"src/candidate.go"},
		requiredFiles:             []string{"src/required.go"},
		preScannedFiles:           []string{"src/prescanned.go"},
		allScoredFiles:            []string{"src/scored.go"},
	}
	got := eval.deterministicEnrichmentFileSet(map[string]bool{
		"src/read.go":  true,
		"src/false.go": false,
	}, "exact declaration required candidate prescanned scored")
	if len(got) != 1 || !got["src/read.go"] {
		t.Fatalf("accepted completion must retain exactly the typed read set, got %+v", got)
	}
	for _, forbidden := range []string{"src/exact.go", "src/declaration.go", "src/candidate.go", "src/required.go", "src/prescanned.go", "src/scored.go"} {
		if got[forbidden] {
			t.Fatalf("accepted completion reopened unread frontier file %q: %+v", forbidden, got)
		}
	}
}

func TestDeterministicEnrichmentFileSet_AcceptedExternalCompletionDoesNotFallbackToSource(t *testing.T) {
	eval := &explorerEvaluator{
		investigationComplete: true,
		exactAnchorFiles:      []string{"src/exact.go"},
		preScannedFiles:       []string{"src/prescanned.go"},
		allScoredFiles:        []string{"src/scored.go"},
	}
	if got := eval.deterministicEnrichmentFileSet(nil, ""); len(got) != 0 {
		t.Fatalf("trace/log-only accepted completion must not fall back to source discovery: %+v", got)
	}
}

func TestDeterministicEnrichmentFileSet_OpenInvestigationKeepsActiveFrontier(t *testing.T) {
	eval := &explorerEvaluator{
		exactAnchorFiles: []string{"src/exact.go"},
		preScannedFiles:  []string{"src/prescanned.go"},
	}
	got := eval.deterministicEnrichmentFileSet(map[string]bool{"src/read.go": true}, "")
	if !got["src/read.go"] || !got["src/exact.go"] {
		t.Fatalf("open investigation must preserve active-frontier discovery, got %+v", got)
	}
}

func TestGetConcreteValuesCached_AcceptedCompletionInvalidatesOpenFrontierCache(t *testing.T) {
	eval := &explorerEvaluator{}
	readSet := map[string]bool{"src/read.go": true}
	eval.cachedConcreteValues = &concreteValuesResult{}
	eval.cachedConcreteValuesCoverageKey = concreteValuesReadCoverageKey(readSet, nil) +
		concreteReturnOwnerAuthorityCoverageKey(nil) + "\x00scope=" + eval.deterministicEnrichmentScopeKey()
	openKey := eval.cachedConcreteValuesCoverageKey
	eval.investigationComplete = true
	closedKey := concreteValuesReadCoverageKey(readSet, nil) +
		concreteReturnOwnerAuthorityCoverageKey(nil) + "\x00scope=" + eval.deterministicEnrichmentScopeKey()
	if openKey == closedKey {
		t.Fatalf("accepted completion must invalidate any broad pre-completion concrete-value cache: %q", openKey)
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

func TestBuildRuntimeTargetTerminalBodyCalls_PromotesSelectedTerminalUtilityCall(t *testing.T) {
	file := &repotypes.FileInfo{
		RelPath: "src/main/java/com/clinic/repo/AuditLog.java", Language: repotypes.LangJava,
		Symbols: []repotypes.Symbol{
			{Name: "AuditLog", Kind: "class", File: "src/main/java/com/clinic/repo/AuditLog.java", Line: 3, EndLine: 8},
			{Name: "record", Kind: "method", Parent: "AuditLog", File: "src/main/java/com/clinic/repo/AuditLog.java", Line: 5, EndLine: 7},
		},
		Relations: []repotypes.Relation{
			{Kind: "call", ToEP: repotypes.RelationEndpoint{Name: "println", Receiver: "System.out"}, File: "src/main/java/com/clinic/repo/AuditLog.java", Line: 6, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "java_method_invocation"},
			{Kind: "call", ToEP: repotypes.RelationEndpoint{Name: "guess", Receiver: "Noise"}, File: "src/main/java/com/clinic/repo/AuditLog.java", Line: 7, Confidence: repotypes.ConfidenceRegexSalvage, Provenance: repotypes.ProvenanceRegexFallback, ResolvedBy: "regex_guess"},
		},
	}
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{file})
	eval := runtimeTargetRelationEvaluator(graph)
	eval.structuredEvidence = []types.EvidenceItem{
		{ID: "incoming", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "VisitRepository.insert", Object: "AuditLog.record", Source: "VisitRepository.java", LineStart: 23, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
		{ID: "selection", Kind: types.EvidenceDirect, AnchorKind: types.AnchorInitializer, Subject: "audit", Object: "AuditLog", Snippet: "private final AuditLog audit = new AuditLog();", Source: "VisitRepository.java", LineStart: 9, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
	}
	readSet := map[string]bool{"src/main/java/com/clinic/repo/AuditLog.java": true}
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(readSet)
	closure.AddReadRanges(map[string][]types.LineRange{"src/main/java/com/clinic/repo/AuditLog.java": {{Start: 5, End: 6}}})

	got := eval.buildRuntimeTargetTerminalBodyCalls(graph, readSet, readSet, closure)
	if len(got.evidence) != 1 {
		t.Fatalf("selected terminal's exact AST body call must survive utility-call filtering: %+v", got.evidence)
	}
	item := got.evidence[0]
	if item.Subject != "AuditLog.record" || item.Object != "System.out.println" || item.LineStart != 6 ||
		item.Producer != types.EvidenceProducerRepoMapTerminalBodyCall || types.ClaimFormOf(item) != types.ClaimCallEdge || !item.IsCitable() {
		t.Fatalf("unexpected selected-terminal body evidence: %+v", item)
	}
	for _, want := range []string{"Typed Selected-Terminal Body Calls", "`AuditLog.record`", "`System.out.println`", "model decides"} {
		if !strings.Contains(got.markdown, want) {
			t.Fatalf("terminal body handoff missing %q:\n%s", want, got.markdown)
		}
	}
	if strings.Contains(got.markdown, "Noise.guess") {
		t.Fatalf("regex relation must not become terminal body authority:\n%s", got.markdown)
	}
}

func TestBuildRuntimeTargetTerminalBodyCalls_RealJavaFixturePreservesTerminalEffect(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", "eval", "fixtures", "java-layered-service"))
	entries, err := repomap.ScanFiles(repoRoot)
	if err != nil {
		t.Fatalf("scan production-shaped Java fixture: %v", err)
	}
	graph := repomap.BuildGraph(repoRoot, repomap.ParseFiles(entries, repoRoot))
	eval := runtimeTargetRelationEvaluator(graph)
	eval.structuredEvidence = []types.EvidenceItem{
		{ID: "incoming", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "VisitRepository.insert", Object: "AuditLog.record", Source: "src/main/java/com/clinic/repo/VisitRepository.java", LineStart: 23, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
		{ID: "selection", Kind: types.EvidenceDirect, AnchorKind: types.AnchorInitializer, Subject: "audit", Object: "AuditLog", Snippet: "private final AuditLog audit = new AuditLog();", Source: "src/main/java/com/clinic/repo/VisitRepository.java", LineStart: 9, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
	}
	readSet := map[string]bool{
		"src/main/java/com/clinic/repo/VisitRepository.java": true,
		"src/main/java/com/clinic/repo/AuditLog.java":        true,
	}
	closure := types.NewEvidenceClosure(repoRoot)
	closure.SetReadSet(readSet)
	closure.AddReadRanges(map[string][]types.LineRange{
		"src/main/java/com/clinic/repo/VisitRepository.java": {{Start: 1, End: 27}},
		"src/main/java/com/clinic/repo/AuditLog.java":        {{Start: 1, End: 9}},
	})

	got := eval.buildRuntimeTargetTerminalBodyCalls(graph, readSet, readSet, closure)
	if len(got.evidence) != 1 || got.evidence[0].Subject != "AuditLog.record" || got.evidence[0].Object != "System.out.println" {
		t.Fatalf("real parsed terminal effect must reach typed handoff, graph file=%+v evidence=%+v markdown=%s",
			graph.FileIndex["src/main/java/com/clinic/repo/AuditLog.java"], got.evidence, got.markdown)
	}
}

func TestBuildRuntimeTargetTerminalBodyCalls_ConceptualTerminalUsesGroundedStaticLeafWithoutRuntimeSelection(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", "eval", "fixtures", "java-layered-service"))
	entries, err := repomap.ScanFiles(repoRoot)
	if err != nil {
		t.Fatalf("scan production-shaped Java fixture: %v", err)
	}
	graph := repomap.BuildGraph(repoRoot, repomap.ParseFiles(entries, repoRoot))
	eval := runtimeTargetRelationEvaluator(graph)
	eval.analysisIR.RequestModel.CallChainEndpointProfile = &types.CallChainEndpointProfile{
		Source:   "VisitController.create",
		SinkMode: types.CallChainSinkResolutionDiscoverTerminal,
	}
	eval.structuredEvidence = []types.EvidenceItem{
		{ID: "first", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "VisitController.create", Object: "VisitService.createVisit", Source: "src/main/java/com/clinic/controller/VisitController.java", LineStart: 18, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
		{ID: "second", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "VisitService.createVisit", Object: "VisitRepository.insert", Source: "src/main/java/com/clinic/service/VisitService.java", LineStart: 17, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
		{ID: "third", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "VisitRepository.insert", Object: "AuditLog.record", Source: "src/main/java/com/clinic/repo/VisitRepository.java", LineStart: 23, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
	}
	readSet := map[string]bool{"src/main/java/com/clinic/repo/AuditLog.java": true}
	closure := types.NewEvidenceClosure(repoRoot)
	closure.SetReadSet(readSet)
	closure.AddReadRanges(map[string][]types.LineRange{
		"src/main/java/com/clinic/repo/AuditLog.java": {{Start: 1, End: 9}},
	})

	got := eval.buildRuntimeTargetTerminalBodyCalls(graph, readSet, readSet, closure)
	if len(got.evidence) != 1 || got.evidence[0].Subject != "AuditLog.record" || got.evidence[0].Object != "System.out.println" {
		t.Fatalf("grounded conceptual-terminal leaf must expose its exact body effect without a fabricated runtime-selection fact: evidence=%+v markdown=%s", got.evidence, got.markdown)
	}
	if eval.callChainDiscoverySelectionPending() || eval.analysisIR.RequestModel.CallChainEndpointProfile.RequiresRuntimeSelectionEvidence() {
		t.Fatal("static conceptual-terminal discovery must not request registration/binding/initializer evidence")
	}
	for _, want := range []string{
		"Typed Terminal-Candidate Body Calls",
		"Each leaf is only a candidate for the requested conceptual destination",
		"parser-authored terminal-candidate body call",
	} {
		if !strings.Contains(got.markdown+got.evidence[0].Summary, want) {
			t.Fatalf("conceptual-terminal candidate handoff missing %q: evidence=%+v markdown=%s", want, got.evidence, got.markdown)
		}
	}
}

func TestBuildRuntimeTargetTerminalBodyCalls_RejectsUnselectedLeafAndUnreadLine(t *testing.T) {
	file := &repotypes.FileInfo{
		RelPath: "src/Sinks.ets", Language: repotypes.LangArkTS,
		Symbols: []repotypes.Symbol{
			{Name: "record", Kind: "method", Parent: "AuditLog", File: "src/Sinks.ets", Line: 4, EndLine: 7},
			{Name: "flush", Kind: "method", Parent: "OtherSink", File: "src/Sinks.ets", Line: 10, EndLine: 13},
		},
		Relations: []repotypes.Relation{
			{Kind: "call", ToEP: repotypes.RelationEndpoint{Name: "write", Receiver: "Console"}, File: "src/Sinks.ets", Line: 6, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "js_ast_member_call"},
			{Kind: "call", ToEP: repotypes.RelationEndpoint{Name: "send", Receiver: "Network"}, File: "src/Sinks.ets", Line: 12, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "js_ast_member_call"},
		},
	}
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{file})
	eval := runtimeTargetRelationEvaluator(graph)
	eval.structuredEvidence = []types.EvidenceItem{
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "Entry.run", Object: "AuditLog.record", Source: "src/Entry.ets", LineStart: 2, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "Entry.run", Object: "OtherSink.flush", Source: "src/Entry.ets", LineStart: 3, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceDirect, AnchorKind: types.AnchorInitializer, Subject: "audit", Object: "AuditLog", Snippet: "const audit: AuditLog = new AuditLog();", Source: "src/Entry.ets", LineStart: 1, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
	}
	readSet := map[string]bool{"src/Sinks.ets": true}
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(readSet)
	closure.AddReadRanges(map[string][]types.LineRange{"src/Sinks.ets": {{Start: 4, End: 6}}})

	got := eval.buildRuntimeTargetTerminalBodyCalls(graph, readSet, readSet, closure)
	if len(got.evidence) != 1 || got.evidence[0].Subject != "AuditLog.record" || got.evidence[0].Object != "Console.write" {
		t.Fatalf("selection/read gates must exclude the sibling terminal and unread relation: %+v", got.evidence)
	}
}

func TestBuildRuntimeTargetTerminalBodyCalls_DiscoverSinkDoesNotFallbackBeforeSelection(t *testing.T) {
	file := &repotypes.FileInfo{
		RelPath: "src/Sinks.java", Language: repotypes.LangJava,
		Symbols:   []repotypes.Symbol{{Name: "count", Kind: "method", Parent: "OtherSink", File: "src/Sinks.java", Line: 4, EndLine: 7}},
		Relations: []repotypes.Relation{{Kind: "call", ToEP: repotypes.RelationEndpoint{Name: "startsWith", Receiver: "String"}, File: "src/Sinks.java", Line: 6, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "java_method_invocation"}},
	}
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{file})
	eval := runtimeTargetRelationEvaluator(graph)
	eval.structuredEvidence = []types.EvidenceItem{{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
		Subject: "Entry.run", Object: "OtherSink.count", Source: "src/Entry.java", LineStart: 2,
		Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
	}}
	readSet := map[string]bool{"src/Sinks.java": true}
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(readSet)
	closure.AddReadRanges(map[string][]types.LineRange{"src/Sinks.java": {{Start: 4, End: 7}}})

	got := eval.buildRuntimeTargetTerminalBodyCalls(graph, readSet, readSet, closure)
	if len(got.evidence) != 0 {
		t.Fatalf("discover-sink without typed selection must not promote an arbitrary leaf body: %+v", got.evidence)
	}
}

func TestPostCompletionReadySignal_SelectedLocalTerminalRequestsBoundedBodyRead(t *testing.T) {
	file := &repotypes.FileInfo{
		RelPath: "src/AuditLog.java", Language: repotypes.LangJava,
		Symbols:   []repotypes.Symbol{{Name: "record", Kind: "method", Parent: "AuditLog", File: "src/AuditLog.java", Line: 5, EndLine: 7}},
		Relations: []repotypes.Relation{{Kind: "call", ToEP: repotypes.RelationEndpoint{Name: "println", Receiver: "System.out"}, File: "src/AuditLog.java", Line: 6, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "java_method_invocation"}},
	}
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{file})
	mut := types.NewMutableState("opaque")
	eval := runtimeTargetRelationEvaluator(graph)
	eval.phase = 1
	eval.heuristics = types.ExploreHeuristics{MidLoopMinIteration: 1}
	eval.mutable = mut
	eval.structuredEvidence = []types.EvidenceItem{
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "VisitRepository.insert", Object: "AuditLog.record", Source: "src/VisitRepository.java", LineStart: 23, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceDirect, AnchorKind: types.AnchorInitializer, Subject: "audit", Object: "AuditLog", Snippet: "private final AuditLog audit = new AuditLog();", Source: "src/VisitRepository.java", LineStart: 9, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
	}

	sig := eval.postCompletionReadySignal(LoopObservation{Iteration: 2, AllToolResults: []types.ToolResult{{ToolName: "emit_evidence", Success: true}}})
	if !sig.HintRequested || sig.HintKey != "explorer.mid-loop.call-chain-terminal-body-read" {
		t.Fatalf("selected local terminal must request one bounded body read before generic closure: %+v", sig)
	}
	for _, want := range []string{"`AuditLog.record`", `path="src/AuditLog.java"`, "line_start=5", "line_end=7", "let the final model decide"} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("terminal-body read hint missing %q: %s", want, sig.Hint)
		}
	}
	if eval.midLoopCompletionReadySent {
		t.Fatal("terminal-body read hint must not consume generic completion-ready latch")
	}

	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"src/AuditLog.java": true})
	closure.AddReadRanges(map[string][]types.LineRange{"src/AuditLog.java": {{Start: 5, End: 7}}})
	if targets := eval.callChainUnreadTerminalBodyTargets(); len(targets) != 0 {
		t.Fatalf("fully read selected terminal body must clear the navigation debt: %+v", targets)
	}
}

func TestCallChainUnreadTerminalBodyTargets_DisjointEndpointReadsDoNotHideBodyGap(t *testing.T) {
	file := &repotypes.FileInfo{
		RelPath: "src/AuditLog.java", Language: repotypes.LangJava,
		Symbols: []repotypes.Symbol{{Name: "record", Kind: "method", Parent: "AuditLog", File: "src/AuditLog.java", Line: 5, EndLine: 9}},
	}
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{file})
	mut := types.NewMutableState("opaque")
	eval := runtimeTargetRelationEvaluator(graph)
	eval.mutable = mut
	eval.structuredEvidence = []types.EvidenceItem{
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "VisitRepository.insert", Object: "AuditLog.record", Source: "src/VisitRepository.java", LineStart: 23, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceDirect, AnchorKind: types.AnchorInitializer, Subject: "audit", Object: "AuditLog", Snippet: "private final AuditLog audit = new AuditLog();", Source: "src/VisitRepository.java", LineStart: 9, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
	}
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"src/AuditLog.java": true})
	closure.AddReadRanges(map[string][]types.LineRange{"src/AuditLog.java": {{Start: 5, End: 5}, {Start: 9, End: 9}}})

	targets := eval.callChainUnreadTerminalBodyTargets()
	if len(targets) != 1 || targets[0].LineStart != 5 || targets[0].LineEnd != 9 {
		t.Fatalf("disjoint endpoint reads must not masquerade as full body coverage: %+v", targets)
	}
}

func TestBuildRuntimeTargetCooperativeMethodDefinitions_RequiresTypedRosterOperationAndExactRead(t *testing.T) {
	file := &repotypes.FileInfo{
		RelPath: "pipeline/base.py", Language: repotypes.LangPython,
		Symbols: []repotypes.Symbol{
			{Name: "handle", Kind: "method", Parent: "BasePlugin", File: "pipeline/base.py", Line: 15, EndLine: 18},
			{Name: "handle", Kind: "method", Parent: "ValidationMixin", File: "pipeline/base.py", Line: 30, EndLine: 33},
			{Name: "handle", Kind: "method", Parent: "TimestampMixin", File: "pipeline/base.py", Line: 39, EndLine: 42},
			{Name: "close", Kind: "method", Parent: "TimestampMixin", File: "pipeline/base.py", Line: 45, EndLine: 48},
			{Name: "handle", Kind: "method", Parent: "UnrelatedMixin", File: "pipeline/base.py", Line: 52, EndLine: 55},
		},
	}
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{file})
	eval := runtimeTargetRelationEvaluator(graph)
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(map[string]bool{"pipeline/base.py": true})
	closure.AddReadRanges(map[string][]types.LineRange{"pipeline/base.py": {{Start: 15, End: 48}}})
	structural := []types.EvidenceItem{
		{Subject: "JsonPlugin", Object: "TimestampMixin", Producer: types.EvidenceProducerRepoMapStructuralRelation, RelationOrdinal: 1},
		{Subject: "JsonPlugin", Object: "ValidationMixin", Producer: types.EvidenceProducerRepoMapStructuralRelation, RelationOrdinal: 2},
		{Subject: "JsonPlugin", Object: "BasePlugin", Producer: types.EvidenceProducerRepoMapStructuralRelation, RelationOrdinal: 3},
	}
	delegations := []types.EvidenceItem{
		{Subject: "TimestampMixin.handle", OwnerSymbol: "TimestampMixin.handle", Producer: types.EvidenceProducerRepoMapCooperativeCall},
		{Subject: "ValidationMixin.handle", OwnerSymbol: "ValidationMixin.handle", Producer: types.EvidenceProducerRepoMapCooperativeCall},
	}

	got := eval.buildRuntimeTargetCooperativeMethodDefinitions(
		graph, map[string]bool{"pipeline/base.py": true}, map[string]bool{"pipeline/base.py": true}, closure, structural, delegations,
	)
	if len(got.evidence) != 3 {
		t.Fatalf("three exact roster method declarations must be promoted: %+v", got.evidence)
	}
	want := []string{"BasePlugin.handle", "ValidationMixin.handle", "TimestampMixin.handle"}
	for i, item := range got.evidence {
		if item.Subject != want[i] || item.OwnerSymbol != want[i] || item.AnchorKind != types.AnchorDefinition ||
			item.Producer != types.EvidenceProducerRepoMapCooperativeMethod || !item.IsCitable() {
			t.Fatalf("unexpected cooperative method definition at %d: %+v", i, item)
		}
	}
	for _, forbidden := range []string{"TimestampMixin.close", "UnrelatedMixin.handle", "runtime_mro_status=proven"} {
		if strings.Contains(got.markdown, forbidden) {
			t.Fatalf("cooperative definition builder leaked unrelated or proven-MRO surface %q:\n%s", forbidden, got.markdown)
		}
	}
}

func TestBuildRuntimeTargetDecoratorApplications_PreservesSelectorRoleWithoutRegistrationClaim(t *testing.T) {
	file := &repotypes.FileInfo{
		RelPath: "pipeline/plugins.py", Language: repotypes.LangPython,
		Relations: []repotypes.Relation{
			{Kind: "decoration", FromEP: repotypes.RelationEndpoint{Name: "register"}, ToEP: repotypes.RelationEndpoint{Name: "JsonPlugin"}, File: "pipeline/plugins.py", Line: 17, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "python_literal_decorator_application", Metadata: map[string]string{"application_surface": `@register("json")`, "selector_literal": "json"}},
			{Kind: "decoration", FromEP: repotypes.RelationEndpoint{Name: "dynamic"}, ToEP: repotypes.RelationEndpoint{Name: "DynamicPlugin"}, File: "pipeline/plugins.py", Line: 25, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "python_literal_decorator_application", Metadata: map[string]string{"application_surface": "@dynamic(NAME)"}},
		},
	}
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{file})
	eval := runtimeTargetRelationEvaluator(graph)
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(map[string]bool{"pipeline/plugins.py": true})
	closure.AddReadRanges(map[string][]types.LineRange{"pipeline/plugins.py": {{Start: 1, End: 30}}})

	got := eval.buildRuntimeTargetDecoratorApplications(
		graph, map[string]bool{"pipeline/plugins.py": true}, map[string]bool{"pipeline/plugins.py": true}, closure,
	)
	if len(got.evidence) != 1 {
		t.Fatalf("only the static literal decorator application should be promoted: %+v", got.evidence)
	}
	item := got.evidence[0]
	if item.Subject != `@register("json")` || item.Predicate != "decorator_selector_application" || item.Object != "JsonPlugin" ||
		item.Producer != types.EvidenceProducerRepoMapDecoratorApplication || item.AnchorKind != types.AnchorDefinition || !item.IsCitable() {
		t.Fatalf("unexpected typed decorator application: %+v", item)
	}
	if item.SelectorApplication == nil || item.SelectorApplication.Owner != "register" || item.SelectorApplication.Literal != "json" {
		t.Fatalf("selector metadata must survive as a typed system carrier: %+v", item.SelectorApplication)
	}
	for _, forbidden := range []string{"registry binding", "registration_edge", "@dynamic(NAME)"} {
		if strings.Contains(got.markdown, forbidden) {
			t.Fatalf("decorator syntax invented semantics or admitted a dynamic selector %q:\n%s", forbidden, got.markdown)
		}
	}
}

func TestMergeConcreteValuesResults_PreservesEveryDeterministicLaneWithoutLiterals(t *testing.T) {
	merged := mergeConcreteValuesResults(
		concreteValuesResult{markdown: "types\n", evidence: []types.EvidenceItem{{ID: "E-type"}}},
		concreteValuesResult{markdown: "super\n", evidence: []types.EvidenceItem{{ID: "E-super"}}},
		concreteValuesResult{markdown: "methods\n", evidence: []types.EvidenceItem{{ID: "E-method"}}},
	)
	if merged.markdown != "types\nsuper\nmethods\n" || len(merged.evidence) != 3 || merged.evidence[2].ID != "E-method" {
		t.Fatalf("deterministic typed lanes were dropped during empty-literal merge: %+v", merged)
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

func TestGetConcreteValuesCached_RebuildsWhenTypedReturnOwnerAppears(t *testing.T) {
	repoRoot := t.TempDir()
	rel := "pipeline/registry.py"
	abs := filepath.Join(repoRoot, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("class Registry:\n    def resolve(self, key):\n        cls = REGISTRY[key]\n        return cls()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := &repotypes.FileInfo{
		RelPath:  rel,
		Language: repotypes.LangPython,
		Symbols: []repotypes.Symbol{{
			Name: "resolve", Kind: "method", Parent: "Registry", File: rel, Line: 2, EndLine: 4,
		}},
	}
	graph := repomap.BuildGraph(repoRoot, []*repotypes.FileInfo{file})
	eval := runtimeTargetRelationEvaluator(graph)
	readSet := map[string]bool{rel: true}

	first := eval.getConcreteValuesCached(context.Background(), repoRoot, readSet, nil)
	for _, item := range first.evidence {
		if item.Producer == "concrete_values" && item.Object == "cls()" {
			t.Fatalf("non-literal return must not be retained before typed owner authority: %+v", first.evidence)
		}
	}

	eval.structuredEvidence = []types.EvidenceItem{{
		ID:              "E-resolve-def",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          rel,
		LineStart:       2,
		LineEnd:         2,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "resolve",
		Subject:         "Registry.resolve",
		GroundingStatus: types.GroundingGrounded,
		Producer:        "explorer.emit_evidence",
	}}
	second := eval.getConcreteValuesCached(context.Background(), repoRoot, readSet, nil)
	found := false
	for _, item := range second.evidence {
		if item.Producer == "concrete_values" && item.Subject == "Registry.resolve" &&
			item.Predicate == "returns" && item.Object == "cls()" && item.AnchorKind == types.AnchorReturn {
			found = true
		}
	}
	if !found {
		t.Fatalf("typed same-file owner must invalidate the cache and retain exact call return: %+v", second.evidence)
	}
}

func TestGetConcreteValuesCached_RebuildsWhenCurrentDispatchCallPathSelectsTerminal(t *testing.T) {
	file := &repotypes.FileInfo{
		RelPath: "src/Sinks.ets", Language: repotypes.LangArkTS,
		Symbols: []repotypes.Symbol{
			{Name: "record", Kind: "method", Parent: "AuditLog", File: "src/Sinks.ets", Line: 4, EndLine: 7},
		},
		Relations: []repotypes.Relation{
			{Kind: "call", ToEP: repotypes.RelationEndpoint{Name: "write", Receiver: "Console"}, File: "src/Sinks.ets", Line: 6, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "js_ast_member_call"},
		},
	}
	repoRoot := t.TempDir()
	graph := repomap.BuildGraph(repoRoot, []*repotypes.FileInfo{file})
	mut := types.NewMutableState("opaque")
	eval := &explorerEvaluator{
		searchResult: &keywordSearchResult{Graph: graph},
		mutable:      mut,
		analysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PredicateAxis: types.AxisCall,
			CallChainEndpointProfile: &types.CallChainEndpointProfile{
				SinkMode: types.CallChainSinkResolutionDiscoverPath,
			},
		}},
	}
	readSet := map[string]bool{"src/Sinks.ets": true}
	closure := types.NewEvidenceClosure(repoRoot)
	closure.SetReadSet(readSet)
	closure.AddReadRanges(map[string][]types.LineRange{"src/Sinks.ets": {{Start: 4, End: 6}}})

	first := eval.getConcreteValuesCached(context.Background(), repoRoot, readSet, closure)
	for _, item := range first.evidence {
		if item.Producer == types.EvidenceProducerRepoMapTerminalBodyCall {
			t.Fatalf("no typed path has selected a terminal before emit: %+v", first.evidence)
		}
	}

	mut.AppendEvidence([]types.EvidenceItem{{
		ID: "incoming", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
		Subject: "Entry.run", Object: "AuditLog.record", Source: "src/Entry.ets", LineStart: 2,
		Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded, Producer: types.EvidenceProducerExplorerEmitEvidence,
	}})
	second := eval.getConcreteValuesCached(context.Background(), repoRoot, readSet, closure)
	found := false
	for _, item := range second.evidence {
		if item.Producer == types.EvidenceProducerRepoMapTerminalBodyCall &&
			item.Subject == "AuditLog.record" && item.Object == "Console.write" {
			found = true
		}
	}
	if !found {
		t.Fatalf("same-dispatch typed call path must invalidate the stale preview and expose the selected terminal body: %+v", second.evidence)
	}
}

func TestConcreteReturnOwnerHasTypedAuthority_FailsClosedOnAmbiguousShortOwner(t *testing.T) {
	values := []concreteValue{
		{file: "src/factory.ext", method: "Alpha.create", kind: concreteValueKindReturns, value: "buildAlpha()"},
		{file: "src/factory.ext", method: "Beta.create", kind: concreteValueKindReturns, value: "buildBeta()"},
	}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceDirect, Scope: types.ScopeLine, Source: "src/factory.ext", LineStart: 1,
		AnchorKind: types.AnchorDefinition, Subject: "create", GroundingStatus: types.GroundingGrounded,
	}}
	sets := concreteReturnOwnerIdentitySets(values)
	for _, value := range values {
		if concreteReturnOwnerHasTypedAuthority(value, evidence, sets) {
			t.Fatalf("ambiguous short owner must not authorize qualified sibling return: %+v", value)
		}
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
