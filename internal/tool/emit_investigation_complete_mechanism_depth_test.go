package tool

import (
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func mechanismSemanticDescentFixture() (*repotypes.Graph, *repotypes.FileInfo) {
	fi := &repotypes.FileInfo{
		RelPath:  "src/pipeline.go",
		Language: repotypes.LangGo,
		Package:  "fixture",
		Symbols: []repotypes.Symbol{
			{Name: "Render", Kind: "function", File: "src/pipeline.go", Line: 2, EndLine: 4},
			{Name: "rewrite", Kind: "function", File: "src/pipeline.go", Line: 6, EndLine: 8},
			{Name: "fallback", Kind: "function", File: "src/pipeline.go", Line: 10, EndLine: 12},
		},
		Relations: []repotypes.Relation{
			{
				Kind: "call", File: "src/pipeline.go", Line: 3,
				FromEP:     repotypes.RelationEndpoint{Name: "Render", File: "src/pipeline.go", Line: 3},
				ToEP:       repotypes.RelationEndpoint{Name: "Transform", File: "src/pipeline.go", Line: 3},
				Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_ast_call",
			},
			{
				Kind: "call", File: "src/pipeline.go", Line: 7,
				FromEP:     repotypes.RelationEndpoint{Name: "rewrite", File: "src/pipeline.go", Line: 7},
				ToEP:       repotypes.RelationEndpoint{Name: "fallback", File: "src/pipeline.go", Line: 7},
				Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_ast_call",
			},
		},
		LineFeatures: map[int][]repotypes.LineFeature{
			3: {repotypes.LineFeatureReturnStmt, repotypes.LineFeatureCallExpression},
			7: {repotypes.LineFeatureReturnStmt, repotypes.LineFeatureCallExpression},
		},
	}
	graph := &repotypes.Graph{
		Files:       []*repotypes.FileInfo{fi},
		FileIndex:   map[string]*repotypes.FileInfo{fi.RelPath: fi},
		SymbolDefs:  map[string][]*repotypes.Symbol{},
		MethodIndex: map[repotypes.MethodKey]*repotypes.Symbol{},
	}
	for idx := range fi.Symbols {
		sym := &fi.Symbols[idx]
		sym.ID = repotypes.DeriveSymbolID(fi, sym)
		graph.SymbolDefs[sym.Name] = append(graph.SymbolDefs[sym.Name], sym)
		graph.MethodIndex[repotypes.MethodKey{Pkg: fi.Package, Name: sym.Name}] = sym
	}
	return graph, fi
}

func mechanismSemanticDescentContext(
	t *testing.T,
	graph *repotypes.Graph,
	readEnd int,
	features map[int][]repotypes.LineFeature,
) *types.BusContext {
	t.Helper()
	if features != nil {
		graph.FileIndex["src/pipeline.go"].LineFeatures = features
	}
	mut := types.NewMutableState("explain render behavior")
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		RepoRoot: t.TempDir(),
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			AnalyzerHints: types.AnalyzerHints{
				Kind: string(types.ReqMechanism),
			},
		}},
	}
	lines := []string{
		"package fixture",
		"func Render(input string) string {",
		"    return Transform(input, rewrite)",
		"}",
		"",
		"func rewrite(input string) string {",
		"    return fallback(input)",
		"}",
		"",
		"func fallback(input string) string {",
		"    return input",
		"}",
	}
	if readEnd > len(lines) {
		readEnd = len(lines)
	}
	seedReadFileHistory(ctx, "src/pipeline.go", 1, lines[:readEnd]...)
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"src/pipeline.go": true})
	closure.SetReadRanges(map[string][]types.LineRange{
		"src/pipeline.go": {{Start: 1, End: readEnd}},
	})
	return ctx
}

func mechanismSemanticDescentFact() types.AnswerAggregateFact {
	return types.AnswerAggregateFact{
		Kind:        types.AnswerAggregateMemberSet,
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Provenance:  string(types.AnswerEvidenceOriginCurrentSource),
		Label:       "render mechanism",
		Value:       "1",
		Members:     []string{"Render"},
		MemberNotes: []string{"entry responsible for the render path"},
		SupportRefs: []string{"Render @ src/pipeline.go:2"},
	}
}

func TestMechanismSemanticDescent_QueuesUniqueReturnedCallbackBodyThenDirectHelper(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	fact := mechanismSemanticDescentFact()

	ctx := mechanismSemanticDescentContext(t, graph, 4, nil)
	if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, nil); got != 1 {
		t.Fatalf("first descent demands=%d, want unread callback body", got)
	}
	pending := ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 1 || pending[0].File != "src/pipeline.go" ||
		len(pending[0].LineRanges) != 1 || pending[0].LineRanges[0] != (types.LineRange{Start: 6, End: 8}) {
		t.Fatalf("first descent pending=%+v, want rewrite body 6..8", pending)
	}

	ctx = mechanismSemanticDescentContext(t, graph, 8, nil)
	if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, nil); got != 1 {
		t.Fatalf("second descent demands=%d, want unread direct helper body", got)
	}
	pending = ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 1 || pending[0].LineRanges[0] != (types.LineRange{Start: 10, End: 12}) {
		t.Fatalf("second descent pending=%+v, want fallback body 10..12", pending)
	}

	ctx = mechanismSemanticDescentContext(t, graph, 12, nil)
	if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, nil); got != 0 {
		t.Fatalf("fully read bounded frontier demands=%d, want 0", got)
	}
	if pending = ctx.Mutable.EvidenceClosure().PendingReads(); len(pending) != 0 {
		t.Fatalf("fully read bounded frontier left pending=%+v", pending)
	}
}

func TestMechanismSemanticDescent_EnumRosterContinuesFromExplorerAuthoredOperationLeaf(t *testing.T) {
	graph, fi := mechanismSemanticDescentFixture()
	// Add a parser-owned direct call next to the callback-bearing Transform
	// relation. The accepted operation row selects only this exact target.
	fi.Relations = append(fi.Relations, repotypes.Relation{
		Kind: "call", File: "src/pipeline.go", Line: 3,
		FromEP:     repotypes.RelationEndpoint{Name: "Render", File: "src/pipeline.go", Line: 3},
		ToEP:       repotypes.RelationEndpoint{Name: "rewrite", File: "src/pipeline.go", Line: 3},
		Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_ast_call",
	})
	ctx := mechanismSemanticDescentContext(t, graph, 4, nil)
	fact := mechanismSemanticDescentFact()
	fact.Members = []string{"OutcomeRendered"}
	fact.MemberNotes = []string{"one typed outcome in the explained mechanism"}
	fact.SupportRefs = []string{"OutcomeRendered @ src/pipeline.go:1"}
	evidence := []types.EvidenceItem{{
		ID: "E-operation", Kind: types.EvidenceRelationship,
		Subject: "Render", Object: "rewrite", AnchorSymbol: "rewrite",
		Source: "src/pipeline.go", LineStart: 3, Scope: types.ScopeLine,
		AnchorKind: types.AnchorCall, Producer: types.EvidenceProducerExplorerEmitEvidence,
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}}

	if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, evidence); got != 1 {
		t.Fatalf("operation-leaf descent demands=%d, want unread rewrite body", got)
	}
	pending := ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 1 || pending[0].LineRanges[0] != (types.LineRange{Start: 6, End: 8}) {
		t.Fatalf("operation-leaf pending=%+v, want rewrite body 6..8", pending)
	}

	ctx = mechanismSemanticDescentContext(t, graph, 8, nil)
	if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, evidence); got != 1 {
		t.Fatalf("operation-leaf second depth demands=%d, want fallback body", got)
	}
	pending = ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 1 || pending[0].LineRanges[0] != (types.LineRange{Start: 10, End: 12}) {
		t.Fatalf("operation-leaf second-depth pending=%+v, want fallback body 10..12", pending)
	}
}

func TestMechanismSemanticDescent_OperationLeafPreciseNoTriggerBoundaries(t *testing.T) {
	baseFact := mechanismSemanticDescentFact()
	baseFact.Members = []string{"OutcomeRendered"}
	baseFact.MemberNotes = []string{"one typed outcome in the explained mechanism"}
	baseFact.SupportRefs = []string{"OutcomeRendered @ src/pipeline.go:1"}
	tests := []struct {
		name   string
		mutate func(*types.BusContext, *types.EvidenceItem)
	}{
		{
			name: "non explorer operation cannot expand model scope",
			mutate: func(_ *types.BusContext, item *types.EvidenceItem) {
				item.Producer = "repomap"
			},
		},
		{
			name: "definition row is not an operation leaf",
			mutate: func(_ *types.BusContext, item *types.EvidenceItem) {
				item.AnchorKind = types.AnchorDefinition
			},
		},
		{
			name: "mismatched endpoint fails open",
			mutate: func(_ *types.BusContext, item *types.EvidenceItem) {
				item.AnchorSymbol = "different"
				item.Object = "different"
			},
		},
		{
			name: "trace request stays isolated",
			mutate: func(ctx *types.BusContext, _ *types.EvidenceItem) {
				ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			graph, fi := mechanismSemanticDescentFixture()
			fi.Relations = append(fi.Relations, repotypes.Relation{
				Kind: "call", File: "src/pipeline.go", Line: 3,
				FromEP:     repotypes.RelationEndpoint{Name: "Render", File: "src/pipeline.go", Line: 3},
				ToEP:       repotypes.RelationEndpoint{Name: "rewrite", File: "src/pipeline.go", Line: 3},
				Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
			})
			ctx := mechanismSemanticDescentContext(t, graph, 4, nil)
			item := types.EvidenceItem{
				Kind: types.EvidenceRelationship, Subject: "Render", Object: "rewrite", AnchorSymbol: "rewrite",
				Source: "src/pipeline.go", LineStart: 3, Scope: types.ScopeLine, AnchorKind: types.AnchorCall,
				Producer: types.EvidenceProducerExplorerEmitEvidence, GroundingStatus: types.GroundingGrounded,
			}
			tc.mutate(ctx, &item)
			if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{baseFact}, []types.EvidenceItem{item}); got != 0 {
				t.Fatalf("no-trigger operation leaf queued %d read(s): %+v", got, ctx.Mutable.EvidenceClosure().PendingReads())
			}
		})
	}
}

func TestMechanismSemanticDescent_PreciseNoTriggerBoundaries(t *testing.T) {
	baseFeatures := map[int][]repotypes.LineFeature{
		3: {repotypes.LineFeatureReturnStmt, repotypes.LineFeatureCallExpression},
		7: {repotypes.LineFeatureReturnStmt, repotypes.LineFeatureCallExpression},
	}
	tests := []struct {
		name   string
		mutate func(*types.BusContext, *types.AnswerAggregateFact, *repotypes.Graph)
	}{
		{
			name: "identity-only member set has no behavioral completion claim",
			mutate: func(_ *types.BusContext, fact *types.AnswerAggregateFact, _ *repotypes.Graph) {
				fact.MemberNotes = nil
			},
		},
		{
			name: "non-return call line is not a semantic descent edge",
			mutate: func(_ *types.BusContext, _ *types.AnswerAggregateFact, graph *repotypes.Graph) {
				graph.FileIndex["src/pipeline.go"].LineFeatures[3] = []repotypes.LineFeature{repotypes.LineFeatureCallExpression}
			},
		},
		{
			name: "trace request stays on trace authority lanes",
			mutate: func(ctx *types.BusContext, _ *types.AnswerAggregateFact, _ *repotypes.Graph) {
				ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
			},
		},
		{
			name: "ambiguous callable identity fails open",
			mutate: func(_ *types.BusContext, _ *types.AnswerAggregateFact, graph *repotypes.Graph) {
				other := &repotypes.Symbol{Name: "rewrite", Kind: "function", File: "src/other.go", Line: 1, EndLine: 2}
				graph.SymbolDefs["rewrite"] = append(graph.SymbolDefs["rewrite"], other)
				graph.FileIndex["src/other.go"] = &repotypes.FileInfo{
					RelPath: "src/other.go", Language: repotypes.LangGo, Package: "other",
					Symbols: []repotypes.Symbol{*other}, LineFeatures: map[int][]repotypes.LineFeature{2: {repotypes.LineFeatureReturnStmt}},
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			graph, _ := mechanismSemanticDescentFixture()
			features := map[int][]repotypes.LineFeature{}
			for line, values := range baseFeatures {
				features[line] = append([]repotypes.LineFeature(nil), values...)
			}
			ctx := mechanismSemanticDescentContext(t, graph, 4, features)
			fact := mechanismSemanticDescentFact()
			tc.mutate(ctx, &fact, graph)
			if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, nil); got != 0 {
				t.Fatalf("no-trigger boundary queued %d read(s): %+v", got, ctx.Mutable.EvidenceClosure().PendingReads())
			}
		})
	}
}

func TestMechanismSemanticDescent_PreCompleteWiringKeepsReadBlockingAfterModelBoundary(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	ctx := mechanismSemanticDescentContext(t, graph, 4, nil)
	fact := mechanismSemanticDescentFact()
	evidence := []types.EvidenceItem{{
		ID:              "E-render",
		Kind:            types.EvidenceMechanism,
		Subject:         "Render",
		Summary:         "Render is the public render entry",
		Source:          "src/pipeline.go",
		LineStart:       2,
		LineEnd:         4,
		Scope:           types.ScopeLineRange,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Render",
		OwnerSymbol:     "fixture.Render",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}}
	downgrade := preCompleteContractCheckWithPreflight(ctx, "", completionPreflightView{
		Evidence:                evidence,
		EffectiveAggregateFacts: []types.AnswerAggregateFact{fact},
	})
	if !strings.Contains(downgrade, "pending forced reads block the closure") ||
		!strings.Contains(downgrade, "src/pipeline.go") ||
		!strings.Contains(downgrade, "rewrite") {
		t.Fatalf("pre-complete wiring did not preserve semantic-descent read as blocking: %s", downgrade)
	}
	pending := ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 1 || !types.PendingReadBlocksAcceptedClosure(pending[0]) ||
		types.IsGenericForcedReadOrigin(pending[0].Origin) {
		t.Fatalf("semantic-descent read must remain citation-class after model boundary: %+v", pending)
	}
}

func TestMechanismSemanticDescent_PreCompleteWiringConsumesExplorerOperationLeaf(t *testing.T) {
	graph, fi := mechanismSemanticDescentFixture()
	fi.Relations = append(fi.Relations, repotypes.Relation{
		Kind: "call", File: "src/pipeline.go", Line: 3,
		FromEP:     repotypes.RelationEndpoint{Name: "Render", File: "src/pipeline.go", Line: 3},
		ToEP:       repotypes.RelationEndpoint{Name: "rewrite", File: "src/pipeline.go", Line: 3},
		Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
	})
	ctx := mechanismSemanticDescentContext(t, graph, 4, nil)
	fact := mechanismSemanticDescentFact()
	fact.Members = []string{"OutcomeRendered"}
	fact.MemberNotes = []string{"one typed outcome in the explained mechanism"}
	fact.SupportRefs = []string{"OutcomeRendered @ src/pipeline.go:1"}
	evidence := []types.EvidenceItem{{
		ID: "E-operation", Kind: types.EvidenceRelationship,
		Subject: "Render", Object: "rewrite", AnchorSymbol: "rewrite",
		Source: "src/pipeline.go", LineStart: 3, Scope: types.ScopeLine,
		AnchorKind: types.AnchorCall, Producer: types.EvidenceProducerExplorerEmitEvidence,
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}}

	downgrade := preCompleteContractCheckWithPreflight(ctx, "", completionPreflightView{
		Evidence: evidence, EffectiveAggregateFacts: []types.AnswerAggregateFact{fact},
	})
	if !strings.Contains(downgrade, "pending forced reads block the closure") ||
		!strings.Contains(downgrade, "src/pipeline.go") || !strings.Contains(downgrade, "rewrite") {
		t.Fatalf("pre-complete operation leaf was not wired as a blocking read: %s", downgrade)
	}
	pending := ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 1 || pending[0].LineRanges[0] != (types.LineRange{Start: 6, End: 8}) ||
		!types.PendingReadBlocksAcceptedClosure(pending[0]) || types.IsGenericForcedReadOrigin(pending[0].Origin) {
		t.Fatalf("operation-leaf pending read lost its exact citation-class contract: %+v", pending)
	}
}
