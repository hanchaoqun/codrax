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
				ToEP:       repotypes.RelationEndpoint{Name: "rewrite", File: "src/pipeline.go", Line: 3},
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
	file := "src/pipeline.go"
	if len(graph.Files) > 0 && graph.Files[0] != nil && strings.TrimSpace(graph.Files[0].RelPath) != "" {
		file = graph.Files[0].RelPath
	}
	if features != nil {
		graph.FileIndex[file].LineFeatures = features
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
		"    return rewrite(input)",
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
	seedReadFileHistory(ctx, file, 1, lines[:readEnd]...)
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{file: true})
	closure.SetReadRanges(map[string][]types.LineRange{
		file: {{Start: 1, End: readEnd}},
	})
	return ctx
}

func mechanismSemanticDescentRetargetFixture(graph *repotypes.Graph, file, language, provenance string) {
	fi := graph.Files[0]
	old := fi.RelPath
	delete(graph.FileIndex, old)
	fi.RelPath = file
	fi.Language = language
	graph.FileIndex[file] = fi
	for idx := range fi.Symbols {
		fi.Symbols[idx].File = file
	}
	for idx := range fi.Relations {
		fi.Relations[idx].File = file
		fi.Relations[idx].FromEP.File = file
		fi.Relations[idx].ToEP.File = file
		fi.Relations[idx].Provenance = provenance
	}
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

func TestMechanismSemanticDescent_QueuesDirectReturnedCalleeBodyThenDirectHelper(t *testing.T) {
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

func TestMechanismSemanticDescent_DoesNotResolveCallArgumentByRepositoryName(t *testing.T) {
	graph, fi := mechanismSemanticDescentFixture()
	// The direct parser call is Transform. The ordinary argument `rm` happens
	// to share a name with an unrelated repository function. Punctuation proves
	// only that rm is handed to Transform; it does not bind that expression to
	// the repository callable or prove that the callable executes.
	fi.Relations[0].ToEP = repotypes.RelationEndpoint{Name: "Transform", File: "src/pipeline.go", Line: 3}
	rm := repotypes.Symbol{Name: "rm", Kind: "function", File: "src/pipeline.go", Line: 14, EndLine: 16}
	rm.ID = repotypes.DeriveSymbolID(fi, &rm)
	fi.Symbols = append(fi.Symbols, rm)
	graph.SymbolDefs["rm"] = []*repotypes.Symbol{&fi.Symbols[len(fi.Symbols)-1]}
	ctx := mechanismSemanticDescentContext(t, graph, 4, nil)
	seedReadFileHistory(ctx, "src/pipeline.go", 3, "    return Transform(input, rm)")

	if got := raiseMechanismSemanticDescentPendingReads(
		ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{mechanismSemanticDescentFact()}, nil,
	); got != 0 {
		t.Fatalf("ordinary argument was guessed as repository callable: demands=%d pending=%+v", got, ctx.Mutable.EvidenceClosure().PendingReads())
	}
	if pending := ctx.Mutable.EvidenceClosure().PendingReads(); len(pending) != 0 {
		t.Fatalf("argument-name collision created a semantic child read: %+v", pending)
	}
}

func TestMechanismSemanticDescent_CallChainUsesItsDedicatedClosureGates(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	ctx := mechanismSemanticDescentContext(t, graph, 4, nil)
	ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Kind = string(types.ReqCallChain)
	fact := mechanismSemanticDescentFact()

	if !genericForcedReadBoundaryCanUseModelPrincipalSet(ctx.AnalysisIR.RequestModel) {
		t.Fatal("call-chain request must retain the shared forced-read eligibility used by its dedicated closure gates")
	}
	if got := raiseMechanismSemanticDescentPendingReads(
		ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, nil,
	); got != 0 {
		t.Fatalf("typed call-chain request entered mechanism semantic descent: demands=%d pending=%+v", got, ctx.Mutable.EvidenceClosure().PendingReads())
	}
	if pending := ctx.Mutable.EvidenceClosure().PendingReads(); len(pending) != 0 {
		t.Fatalf("call-chain mechanism exclusion must not mint pending reads: %+v", pending)
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

func TestMechanismSemanticDescent_MechanismDefinitionSeedsWithoutMemberNotes(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	ctx := mechanismSemanticDescentContext(t, graph, 4, nil)
	fact := mechanismSemanticDescentFact()
	fact.MemberNotes = nil
	evidence := []types.EvidenceItem{{
		ID: "E-mechanism", Kind: types.EvidenceMechanism,
		Subject: "Render", AnchorSymbol: "Render",
		Source: "src/pipeline.go", LineStart: 2, LineEnd: 4, Scope: types.ScopeLineRange,
		AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerExplorerEmitEvidence,
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}}

	if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, evidence); got != 1 {
		t.Fatalf("mechanism-definition descent demands=%d, want rewrite body", got)
	}
	pending := ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 1 || pending[0].LineRanges[0] != (types.LineRange{Start: 6, End: 8}) {
		t.Fatalf("mechanism-definition pending=%+v, want rewrite body 6..8", pending)
	}
}

func TestMechanismSemanticDescent_ExecutableGuardReadsOnlySelectedOwnerBody(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	fact := mechanismSemanticDescentFact()
	fact.Members = []string{"OutcomeRendered"}
	fact.MemberNotes = nil
	fact.SupportRefs = []string{"OutcomeRendered @ src/pipeline.go:1"}
	evidence := []types.EvidenceItem{{
		ID: "E-guard", Kind: types.EvidenceConditional,
		Condition: "input != empty", AnchorSymbol: "input",
		Source: "src/pipeline.go", LineStart: 3, Scope: types.ScopeLine,
		AnchorKind: types.AnchorCondition, Producer: types.EvidenceProducerExplorerEmitEvidence,
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}}

	ctx := mechanismSemanticDescentContext(t, graph, 4, map[int][]repotypes.LineFeature{
		3: {repotypes.LineFeatureCallExpression},
		7: {repotypes.LineFeatureCallExpression},
	})
	if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, evidence); got != 0 {
		t.Fatalf("already-read executable owner must not authorize sibling call descent, got=%d", got)
	}
	pending := ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 0 {
		t.Fatalf("executable guard leaked sibling call reads: %+v", pending)
	}

	ctx = mechanismSemanticDescentContext(t, graph, 8, map[int][]repotypes.LineFeature{
		3: {repotypes.LineFeatureCallExpression},
		7: {repotypes.LineFeatureCallExpression},
	})
	if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, evidence); got != 0 {
		t.Fatalf("reading unrelated sibling body must not extend executable-owner authority, got=%d", got)
	}
	pending = ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 0 {
		t.Fatalf("executable guard leaked second-depth sibling reads: %+v", pending)
	}
}

func TestMechanismSemanticDescent_ExecutableSeedIsLanguageNeutralIncludingArkTSAndCangjie(t *testing.T) {
	tests := []struct {
		name, file, language string
		provenance           string
	}{
		{"go", "src/pipeline.go", repotypes.LangGo, repotypes.ProvenanceTreeSitter},
		{"java", "src/Pipeline.java", repotypes.LangJava, repotypes.ProvenanceTreeSitter},
		{"c", "src/pipeline.c", repotypes.LangC, repotypes.ProvenanceTreeSitter},
		{"cpp", "src/pipeline.cpp", repotypes.LangCpp, repotypes.ProvenanceTreeSitter},
		{"rust", "src/pipeline.rs", repotypes.LangRust, repotypes.ProvenanceTreeSitter},
		{"python", "src/pipeline.py", repotypes.LangPython, repotypes.ProvenanceTreeSitter},
		{"arkts", "src/pipeline.ets", repotypes.LangArkTS, repotypes.ProvenanceTreeSitter},
		{"cangjie", "src/pipeline.cj", repotypes.LangCangjie, repotypes.ProvenanceCangjieParser},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			graph, _ := mechanismSemanticDescentFixture()
			mechanismSemanticDescentRetargetFixture(graph, tc.file, tc.language, tc.provenance)
			ctx := mechanismSemanticDescentContext(t, graph, 1, nil)
			fact := mechanismSemanticDescentFact()
			fact.Members = []string{"OutcomeRendered"}
			fact.MemberNotes = nil
			fact.SupportRefs = []string{"OutcomeRendered @ " + tc.file + ":1"}
			evidence := []types.EvidenceItem{{
				Kind: types.EvidenceConditional, Condition: "input != empty", AnchorSymbol: "input",
				Source: tc.file, LineStart: 3, Scope: types.ScopeLine,
				AnchorKind: types.AnchorCondition, Producer: types.EvidenceProducerExplorerEmitEvidence,
				GroundingStatus: types.GroundingGrounded,
			}}
			if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, evidence); got != 1 {
				t.Fatalf("%s executable seed demands=%d, want owner body", tc.language, got)
			}
			pending := ctx.Mutable.EvidenceClosure().PendingReads()
			if len(pending) != 1 || pending[0].File != tc.file ||
				pending[0].LineRanges[0] != (types.LineRange{Start: 2, End: 4}) {
				t.Fatalf("%s pending=%+v, want %s:2..4", tc.language, pending, tc.file)
			}
		})
	}
}

func TestMechanismSemanticDescent_SelectedCallableDefinitionSeedsEnumRosterWithoutExplicitRole(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	ctx := mechanismSemanticDescentContext(t, graph, 4, nil)
	fact := types.AnswerAggregateFact{
		Kind: types.AnswerAggregateMemberSet, Value: "4",
		Members: []string{"OutcomeRendered", "OutcomeFallbackRune", "OutcomeUnsupportedKind", "OutcomeLibraryRejected"},
	}
	evidence := []types.EvidenceItem{
		{
			Kind: types.EvidenceMechanism, AnchorSymbol: "Render",
			Source: "src/pipeline.go", LineStart: 2, Scope: types.ScopeLine,
			AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerExplorerEmitEvidence,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			// Same-file secondary role description must not consume a second
			// unbound definition seed.
			Kind: types.EvidenceMechanism, AnchorSymbol: "rewrite",
			Source: "src/pipeline.go", LineStart: 6, Scope: types.ScopeLine,
			AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerExplorerEmitEvidence,
			GroundingStatus: types.GroundingGrounded,
		},
	}

	if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, evidence); got != 0 {
		t.Fatalf("already-read selected definition must not authorize child traversal, got=%d", got)
	}
	pending := ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 0 {
		t.Fatalf("selected definition leaked child reads: %+v", pending)
	}
}

func TestMechanismSemanticDescent_SelectedDefinitionQueuesOwnBodyWithoutLineFeatureIndex(t *testing.T) {
	graph, fi := mechanismSemanticDescentFixture()
	fi.LineFeatures = nil
	ctx := mechanismSemanticDescentContext(t, graph, 1, nil)
	ctx.AnalysisIR.RequestModel.Scenario = types.ScenarioConfigTrace
	ctx.AnalysisIR.RequestModel.SubTopics = []types.SubTopic{
		{Summary: "runtime configuration", Entities: []string{"RuntimeSettings"}},
		{Summary: "render fallback", Entities: []string{"Render"}},
	}
	fact := types.AnswerAggregateFact{
		Kind: types.AnswerAggregateMemberSet, Value: "1", Role: types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"OutcomeRendered"},
	}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceMechanism, AnchorSymbol: "Render",
		Source: "src/pipeline.go", LineStart: 2, Scope: types.ScopeLine,
		AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerExplorerEmitEvidence,
		GroundingStatus: types.GroundingGrounded,
	}}

	if !genericForcedReadBoundaryCanUseModelPrincipalSet(ctx.AnalysisIR.RequestModel) {
		t.Fatal("typed multi-topic mechanism request unexpectedly failed the existing generic boundary")
	}
	if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, evidence); got != 1 {
		t.Fatalf("selected definition without line-feature index demands=%d, want its own unread body", got)
	}
	pending := ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 1 || pending[0].LineRanges[0] != (types.LineRange{Start: 2, End: 4}) {
		t.Fatalf("selected definition pending=%+v, want exact Render body 2..4", pending)
	}

	// Without the optional line-shape index, reading the selected body is the
	// honest stopping point: no child relation is inferred or authored.
	ctx = mechanismSemanticDescentContext(t, graph, 4, nil)
	ctx.AnalysisIR.RequestModel.Scenario = types.ScenarioConfigTrace
	ctx.AnalysisIR.RequestModel.SubTopics = []types.SubTopic{{Summary: "a"}, {Summary: "b"}}
	if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, evidence); got != 0 {
		t.Fatalf("missing line-feature index must not invent child reads, got=%d pending=%+v", got, ctx.Mutable.EvidenceClosure().PendingReads())
	}
}

func requestedSubTopicSymbolProvenance(surface string) types.EntityProvenance {
	return types.EntityProvenance{
		Surface: surface, Origin: types.EntityOriginSubTopicEntity,
		Resolution: types.EntityResolutionSymbol, Resolved: true,
		UseForSearch: true, UseForShape: true,
	}
}

func TestRequestedSubTopicCallableBody_CallSiteOnlyRequiresAlreadyReadBodyEvidence(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	ctx := mechanismSemanticDescentContext(t, graph, 12, nil)
	ctx.AnalysisIR.RequestModel.SubTopics = []types.SubTopic{
		{
			Summary: "entry routing", Entities: []string{"Render"},
			EntityProvenance: []types.EntityProvenance{requestedSubTopicSymbolProvenance("Render")},
		},
		{
			Summary: "fallback behavior", Entities: []string{"rewrite"},
			EntityProvenance: []types.EntityProvenance{requestedSubTopicSymbolProvenance("rewrite")},
		},
	}
	evidence := []types.EvidenceItem{
		{
			Kind: types.EvidenceMechanism, AnchorKind: types.AnchorReturn,
			AnchorSymbol: "Render", Source: "src/pipeline.go", LineStart: 3,
			Scope: types.ScopeLine, Producer: types.EvidenceProducerExplorerEmitEvidence,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
			AnchorSymbol: "rewrite", Subject: "Render", Object: "rewrite",
			Source: "src/pipeline.go", LineStart: 3, Scope: types.ScopeLine,
			Producer:        types.EvidenceProducerExplorerEmitEvidence,
			GroundingStatus: types.GroundingGrounded,
		},
	}

	got := requestedSubTopicCallableBodyDowngrade(ctx, ctx.Mutable.EvidenceClosure(), evidence)
	if !strings.Contains(got, "call-site evidence but no implementation-body evidence") ||
		!strings.Contains(got, "src/pipeline.go:6-8") {
		t.Fatalf("downgrade=%q, want bounded already-read rewrite body repair", got)
	}
	repairs := ctx.Mutable.EvidenceClosure().ActiveRepairs()
	if len(repairs) != 1 || repairs[0].Kind != types.RepairEmitEvidence ||
		len(repairs[0].Files) != 1 || repairs[0].Files[0] != "src/pipeline.go" ||
		len(repairs[0].LineRanges) != 1 || repairs[0].LineRanges[0] != (types.LineRange{Start: 6, End: 8}) {
		t.Fatalf("repairs=%+v, want surgical emit-evidence repair for rewrite body", repairs)
	}

	evidence = append(evidence, types.EvidenceItem{
		Kind: types.EvidenceMechanism, AnchorKind: types.AnchorDefinition,
		AnchorSymbol: "rewrite", Subject: "rewrite", Source: "src/pipeline.go", LineStart: 6,
		Scope: types.ScopeLine, Producer: types.EvidenceProducerExplorerEmitEvidence,
		GroundingStatus: types.GroundingGrounded,
	})
	if got := requestedSubTopicCallableBodyDowngrade(ctx, ctx.Mutable.EvidenceClosure(), evidence); got == "" {
		t.Fatal("multi-line callable declaration alone must not prove implementation behavior")
	}
	evidence = append(evidence, types.EvidenceItem{
		Kind: types.EvidenceMechanism, AnchorKind: types.AnchorReturn,
		AnchorSymbol: "rewrite", Subject: "rewrite", Source: "src/pipeline.go", LineStart: 7,
		Scope: types.ScopeLine, Producer: types.EvidenceProducerExplorerEmitEvidence,
		GroundingStatus: types.GroundingGrounded,
	})
	if got := requestedSubTopicCallableBodyDowngrade(ctx, ctx.Mutable.EvidenceClosure(), evidence); got != "" {
		t.Fatalf("body evidence must close requested sub-topic debt, got %q", got)
	}
}

func TestRequestedSubTopicCallableBody_SelectedDefinitionParserCallClosesDebt(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	ctx := mechanismSemanticDescentContext(t, graph, 12, nil)
	ctx.AnalysisIR.RequestModel.SubTopics = []types.SubTopic{
		{
			Summary: "entry routing", Entities: []string{"Render"},
			EntityProvenance: []types.EntityProvenance{requestedSubTopicSymbolProvenance("Render")},
		},
		{
			Summary: "fallback behavior", Entities: []string{"rewrite"},
			EntityProvenance: []types.EntityProvenance{requestedSubTopicSymbolProvenance("rewrite")},
		},
	}
	evidence := []types.EvidenceItem{
		{
			ID: "selected-call", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
			AnchorSymbol: "rewrite", Subject: "Render", Object: "rewrite",
			Source: "src/pipeline.go", LineStart: 3, Scope: types.ScopeLine,
			Producer: types.EvidenceProducerExplorerEmitEvidence, GroundingStatus: types.GroundingGrounded,
		},
		{
			ID: "parser-body-call", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
			AnchorSymbol: "trim", Subject: "rewrite", Object: "trim",
			Source: "src/pipeline.go", LineStart: 7, Scope: types.ScopeLine,
			Producer:    types.EvidenceProducerRepoMapSelectedCallableBodyCall,
			DerivedFrom: []string{"selected-definition"}, GroundingStatus: types.GroundingGrounded,
		},
	}

	if got := requestedSubTopicCallableBodyDowngrade(ctx, ctx.Mutable.EvidenceClosure(), evidence); got != "" {
		t.Fatalf("exact parser-owned call from a model-selected, already-read body must close debt, got %q", got)
	}

	evidence[1].Producer = types.EvidenceProducerRepoMapStructuralRelation
	if got := requestedSubTopicCallableBodyDowngrade(ctx, ctx.Mutable.EvidenceClosure(), evidence); got == "" {
		t.Fatal("broad repo-map navigation evidence must not close model-owned body inspection")
	}
}

func TestRequestedSubTopicCallableBody_PreCompleteWirePin(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	ctx := mechanismSemanticDescentContext(t, graph, 12, nil)
	ctx.AnalysisIR.RequestModel.SubTopics = []types.SubTopic{
		{
			Summary: "entry routing", Entities: []string{"Render"},
			EntityProvenance: []types.EntityProvenance{requestedSubTopicSymbolProvenance("Render")},
		},
		{
			Summary: "fallback behavior", Entities: []string{"rewrite"},
			EntityProvenance: []types.EntityProvenance{requestedSubTopicSymbolProvenance("rewrite")},
		},
	}
	evidence := []types.EvidenceItem{
		{
			Kind: types.EvidenceMechanism, AnchorKind: types.AnchorReturn,
			AnchorSymbol: "Render", Source: "src/pipeline.go", LineStart: 3,
			Scope: types.ScopeLine, Producer: types.EvidenceProducerExplorerEmitEvidence,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
			AnchorSymbol: "rewrite", Subject: "Render", Object: "rewrite",
			Source: "src/pipeline.go", LineStart: 3, Scope: types.ScopeLine,
			Producer:        types.EvidenceProducerExplorerEmitEvidence,
			GroundingStatus: types.GroundingGrounded,
		},
	}

	got := preCompleteContractCheckWithEvidence(ctx, "", evidence)
	if !strings.Contains(got, "call-site evidence but no implementation-body evidence") ||
		!strings.Contains(got, "src/pipeline.go:6-8") {
		t.Fatalf("pre-complete wire output=%q, want requested sub-topic body downgrade", got)
	}
}

func TestRequestedSubTopicCallableBody_UnreadUniqueBodyQueuesBoundedRead(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	ctx := mechanismSemanticDescentContext(t, graph, 4, nil)
	ctx.AnalysisIR.RequestModel.SubTopics = []types.SubTopic{
		{
			Summary: "entry routing", Entities: []string{"Render"},
			EntityProvenance: []types.EntityProvenance{requestedSubTopicSymbolProvenance("Render")},
		},
		{
			Summary: "fallback behavior", Entities: []string{"rewrite"},
			EntityProvenance: []types.EntityProvenance{requestedSubTopicSymbolProvenance("rewrite")},
		},
	}
	evidence := []types.EvidenceItem{
		{
			Kind: types.EvidenceMechanism, AnchorKind: types.AnchorReturn,
			AnchorSymbol: "Render", Source: "src/pipeline.go", LineStart: 3,
			Scope: types.ScopeLine, Producer: types.EvidenceProducerExplorerEmitEvidence,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
			AnchorSymbol: "rewrite", Subject: "Render", Object: "rewrite",
			Source: "src/pipeline.go", LineStart: 3, Scope: types.ScopeLine,
			Producer:        types.EvidenceProducerExplorerEmitEvidence,
			GroundingStatus: types.GroundingGrounded,
		},
	}

	if got := requestedSubTopicCallableBodyDowngrade(ctx, ctx.Mutable.EvidenceClosure(), evidence); got != "" {
		t.Fatalf("unread body should route through pending-read surface, got %q", got)
	}
	pending := ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 1 || pending[0].File != "src/pipeline.go" ||
		len(pending[0].LineRanges) != 1 || pending[0].LineRanges[0] != (types.LineRange{Start: 6, End: 8}) {
		t.Fatalf("pending=%+v, want bounded rewrite body read", pending)
	}
}

func TestRequestedSubTopicCallableBody_AmbiguousOrConceptEntityFailsOpen(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	ctx := mechanismSemanticDescentContext(t, graph, 12, nil)
	ctx.AnalysisIR.RequestModel.SubTopics = []types.SubTopic{
		{
			Summary: "entry routing", Entities: []string{"Render"},
			EntityProvenance: []types.EntityProvenance{requestedSubTopicSymbolProvenance("Render")},
		},
		{
			Summary: "external fallback", Entities: []string{"rewrite"},
			EntityProvenance: []types.EntityProvenance{{
				Surface: "rewrite", Origin: types.EntityOriginSubTopicEntity,
				Resolution: types.EntityResolutionAmbiguousSymbol, UseForSearch: true,
			}},
		},
	}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
		AnchorSymbol: "rewrite", Subject: "Render", Object: "rewrite",
		Source: "src/pipeline.go", LineStart: 3, Scope: types.ScopeLine,
		Producer:        types.EvidenceProducerExplorerEmitEvidence,
		GroundingStatus: types.GroundingGrounded,
	}}

	if got := requestedSubTopicCallableBodyDowngrade(ctx, ctx.Mutable.EvidenceClosure(), evidence); got != "" {
		t.Fatalf("ambiguous typed entity must fail open, got %q", got)
	}
	if pending := ctx.Mutable.EvidenceClosure().PendingReads(); len(pending) != 0 {
		t.Fatalf("ambiguous entity queued reads: %+v", pending)
	}
}

func TestRequestedSubTopicCallableBody_LanguageNeutralTypedResolution(t *testing.T) {
	tests := []struct {
		name, file, language, provenance string
	}{
		{"go", "src/pipeline.go", repotypes.LangGo, repotypes.ProvenanceTreeSitter},
		{"java", "src/Pipeline.java", repotypes.LangJava, repotypes.ProvenanceTreeSitter},
		{"c", "src/pipeline.c", repotypes.LangC, repotypes.ProvenanceTreeSitter},
		{"cpp", "src/pipeline.cpp", repotypes.LangCpp, repotypes.ProvenanceTreeSitter},
		{"rust", "src/pipeline.rs", repotypes.LangRust, repotypes.ProvenanceTreeSitter},
		{"python", "src/pipeline.py", repotypes.LangPython, repotypes.ProvenanceTreeSitter},
		{"arkts", "src/pipeline.ets", repotypes.LangArkTS, repotypes.ProvenanceTreeSitter},
		{"cangjie", "src/pipeline.cj", repotypes.LangCangjie, repotypes.ProvenanceCangjieParser},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			graph, _ := mechanismSemanticDescentFixture()
			mechanismSemanticDescentRetargetFixture(graph, tc.file, tc.language, tc.provenance)
			topics := []types.SubTopic{{
				Summary: "fallback behavior", Entities: []string{"rewrite"},
				EntityProvenance: []types.EntityProvenance{requestedSubTopicSymbolProvenance("rewrite")},
			}}
			evidence := []types.EvidenceItem{{
				Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
				AnchorSymbol: "rewrite", Subject: "Render", Object: "rewrite",
				Source: tc.file, LineStart: 3, Scope: types.ScopeLine,
				Producer:        types.EvidenceProducerExplorerEmitEvidence,
				GroundingStatus: types.GroundingGrounded,
			}}
			debts := requestedSubTopicCallableBodyDebts(topics, graph, evidence)
			if len(debts) != 1 || debts[0].file != tc.file || debts[0].sym == nil || debts[0].sym.Name != "rewrite" {
				t.Fatalf("%s debts=%+v, want exact local rewrite callable", tc.language, debts)
			}
		})
	}
}

func TestMechanismSemanticDescent_SelectedDefinitionSurvivesSupportingRosterDemotion(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	ctx := mechanismSemanticDescentContext(t, graph, 1, nil)
	ctx.AnalysisIR.RequestModel.Scenario = types.ScenarioConfigTrace
	ctx.AnalysisIR.RequestModel.SubTopics = []types.SubTopic{
		{Summary: "runtime configuration", Entities: []string{"RuntimeSettings"}},
		{Summary: "render fallback", Entities: []string{"Render"}},
	}
	// A narrative roster may be honestly demoted to supporting coverage when
	// it lacks an index-aligned behavioral note for every member. That display
	// role must not erase a separate, exact Explorer selection of a callable.
	fact := types.AnswerAggregateFact{
		Kind: types.AnswerAggregateMemberSet, Value: "4", Role: types.AnswerAggregateRoleSupportingCoverage,
		Members:     []string{"OutcomeRendered", "OutcomeFallbackRune", "OutcomeUnsupportedKind", "OutcomeLibraryRejected"},
		SupportRefs: []string{"src/pipeline.go:1", "src/pipeline.go:2", "src/pipeline.go:3", "src/pipeline.go:4", "src/pipeline.go:5"},
	}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceMechanism, AnchorSymbol: "Render",
		Source: "src/pipeline.go", LineStart: 2, Scope: types.ScopeLine,
		AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerExplorerEmitEvidence,
		GroundingStatus: types.GroundingGrounded,
	}}

	if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, evidence); got != 1 {
		t.Fatalf("exact selected definition behind supporting roster demands=%d, want own unread body", got)
	}
	pending := ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 1 || pending[0].LineRanges[0] != (types.LineRange{Start: 2, End: 4}) {
		t.Fatalf("supporting-roster selected definition pending=%+v, want exact Render body 2..4", pending)
	}
}

func TestMechanismSemanticDescent_SelectedCallableDefinitionPreciseNoTriggerBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*types.BusContext, *types.AnswerAggregateFact, *types.EvidenceItem)
	}{
		{
			name: "direct definition is not a mechanism selection",
			mutate: func(_ *types.BusContext, _ *types.AnswerAggregateFact, item *types.EvidenceItem) {
				item.Kind = types.EvidenceDirect
			},
		},
		{
			name: "system role description cannot expand scope",
			mutate: func(_ *types.BusContext, _ *types.AnswerAggregateFact, item *types.EvidenceItem) {
				item.Producer = types.EvidenceProducerAutoPairRoleDescription
			},
		},
		{
			name: "text reference has no callable definition authority",
			mutate: func(_ *types.BusContext, _ *types.AnswerAggregateFact, item *types.EvidenceItem) {
				item.AnchorKind = types.AnchorTextReference
			},
		},
		{
			name: "anchor identity must match parser callable",
			mutate: func(_ *types.BusContext, _ *types.AnswerAggregateFact, item *types.EvidenceItem) {
				item.AnchorSymbol = "different"
			},
		},
		{
			name: "supporting completion cannot expand principal scope",
			mutate: func(_ *types.BusContext, fact *types.AnswerAggregateFact, _ *types.EvidenceItem) {
				fact.Role = types.AnswerAggregateRoleSupportingCoverage
			},
		},
		{
			name: "trace request remains isolated",
			mutate: func(ctx *types.BusContext, _ *types.AnswerAggregateFact, _ *types.EvidenceItem) {
				ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			graph, _ := mechanismSemanticDescentFixture()
			ctx := mechanismSemanticDescentContext(t, graph, 4, nil)
			fact := types.AnswerAggregateFact{
				Kind: types.AnswerAggregateMemberSet, Value: "1", Members: []string{"OutcomeRendered"},
			}
			item := types.EvidenceItem{
				Kind: types.EvidenceMechanism, AnchorSymbol: "Render",
				Source: "src/pipeline.go", LineStart: 2, Scope: types.ScopeLine,
				AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerExplorerEmitEvidence,
				GroundingStatus: types.GroundingGrounded,
			}
			tc.mutate(ctx, &fact, &item)
			if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, []types.EvidenceItem{item}); got != 0 {
				t.Fatalf("no-trigger selected definition queued %d read(s): %+v", got, ctx.Mutable.EvidenceClosure().PendingReads())
			}
		})
	}
}

func TestMechanismSemanticDescent_ExecutableEvidencePreciseNoTriggerBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*types.BusContext, *types.AnswerAggregateFact, *types.EvidenceItem)
	}{
		{
			name: "definition fact is not executable",
			mutate: func(_ *types.BusContext, _ *types.AnswerAggregateFact, item *types.EvidenceItem) {
				item.Kind = types.EvidenceMechanism
				item.AnchorKind = types.AnchorDefinition
			},
		},
		{
			name: "non explorer row cannot expand model scope",
			mutate: func(_ *types.BusContext, _ *types.AnswerAggregateFact, item *types.EvidenceItem) {
				item.Producer = types.EvidenceProducerRepoMapStructuralRelation
			},
		},
		{
			name: "support fact is not a principal completion boundary",
			mutate: func(_ *types.BusContext, fact *types.AnswerAggregateFact, _ *types.EvidenceItem) {
				fact.Role = types.AnswerAggregateRoleSupportingCoverage
				fact.Provenance = ""
			},
		},
		{
			name: "evidence outside a callable fails open",
			mutate: func(_ *types.BusContext, _ *types.AnswerAggregateFact, item *types.EvidenceItem) {
				item.LineStart = 1
			},
		},
		{
			name: "trace request remains isolated",
			mutate: func(ctx *types.BusContext, _ *types.AnswerAggregateFact, _ *types.EvidenceItem) {
				ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			graph, _ := mechanismSemanticDescentFixture()
			ctx := mechanismSemanticDescentContext(t, graph, 4, map[int][]repotypes.LineFeature{
				3: {repotypes.LineFeatureCallExpression},
			})
			fact := mechanismSemanticDescentFact()
			fact.Members = []string{"OutcomeRendered"}
			fact.MemberNotes = nil
			fact.SupportRefs = []string{"OutcomeRendered @ src/pipeline.go:1"}
			item := types.EvidenceItem{
				Kind: types.EvidenceConditional, Condition: "input != empty", AnchorSymbol: "input",
				Source: "src/pipeline.go", LineStart: 3, Scope: types.ScopeLine,
				AnchorKind: types.AnchorCondition, Producer: types.EvidenceProducerExplorerEmitEvidence,
				GroundingStatus: types.GroundingGrounded,
			}
			tc.mutate(ctx, &fact, &item)
			if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, []types.EvidenceItem{item}); got != 0 {
				t.Fatalf("no-trigger executable row queued %d read(s): %+v", got, ctx.Mutable.EvidenceClosure().PendingReads())
			}
		})
	}
}

func TestMechanismSemanticDescent_MechanismDefinitionPreciseNoTriggerBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*types.BusContext, *types.AnswerAggregateFact, *types.EvidenceItem)
	}{
		{
			name: "direct definition is not a typed mechanism selection",
			mutate: func(_ *types.BusContext, _ *types.AnswerAggregateFact, item *types.EvidenceItem) {
				item.Kind = types.EvidenceDirect
			},
		},
		{
			name: "non explorer mechanism cannot expand model scope",
			mutate: func(_ *types.BusContext, _ *types.AnswerAggregateFact, item *types.EvidenceItem) {
				item.Producer = types.EvidenceProducerRepoMapStructuralRelation
			},
		},
		{
			name: "trace request remains isolated",
			mutate: func(ctx *types.BusContext, _ *types.AnswerAggregateFact, _ *types.EvidenceItem) {
				ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			graph, _ := mechanismSemanticDescentFixture()
			ctx := mechanismSemanticDescentContext(t, graph, 4, nil)
			fact := mechanismSemanticDescentFact()
			fact.MemberNotes = nil
			item := types.EvidenceItem{
				Kind: types.EvidenceMechanism, Subject: "Render", AnchorSymbol: "Render",
				Source: "src/pipeline.go", LineStart: 2, LineEnd: 4, Scope: types.ScopeLineRange,
				AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerExplorerEmitEvidence,
				GroundingStatus: types.GroundingGrounded,
			}
			tc.mutate(ctx, &fact, &item)
			if got := raiseMechanismSemanticDescentPendingReads(ctx, ctx.Mutable.EvidenceClosure(), []types.AnswerAggregateFact{fact}, []types.EvidenceItem{item}); got != 0 {
				t.Fatalf("no-trigger mechanism definition queued %d read(s): %+v", got, ctx.Mutable.EvidenceClosure().PendingReads())
			}
		})
	}
}

func TestMechanismSemanticDescent_RosterBoundDefinitionRejectsSupportSourceMismatch(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	ctx := mechanismSemanticDescentContext(t, graph, 4, nil)
	fact := mechanismSemanticDescentFact()
	fact.MemberNotes = nil
	fact.SupportRefs = []string{"Render @ src/other.go:2"}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceMechanism, Subject: "Render", AnchorSymbol: "Render",
		Source: "src/pipeline.go", LineStart: 2, LineEnd: 4, Scope: types.ScopeLineRange,
		AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerExplorerEmitEvidence,
		GroundingStatus: types.GroundingGrounded,
	}}
	if got := mechanismSemanticDescentDefinitionSeeds(ctx, graph, []types.AnswerAggregateFact{fact}, evidence); len(got) != 0 {
		t.Fatalf("roster-bound definition accepted mismatched support source: %+v", got)
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

func TestMechanismSemanticDescent_PreCompleteWiringConsumesTypedMechanismDefinition(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	ctx := mechanismSemanticDescentContext(t, graph, 4, nil)
	fact := mechanismSemanticDescentFact()
	fact.MemberNotes = nil
	evidence := []types.EvidenceItem{{
		ID: "E-mechanism", Kind: types.EvidenceMechanism,
		Subject: "Render", AnchorSymbol: "Render",
		Source: "src/pipeline.go", LineStart: 2, LineEnd: 4, Scope: types.ScopeLineRange,
		AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerExplorerEmitEvidence,
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}}

	downgrade := preCompleteContractCheckWithPreflight(ctx, "", completionPreflightView{
		Evidence: evidence, EffectiveAggregateFacts: []types.AnswerAggregateFact{fact},
	})
	if !strings.Contains(downgrade, "pending forced reads block the closure") ||
		!strings.Contains(downgrade, "src/pipeline.go") || !strings.Contains(downgrade, "rewrite") {
		t.Fatalf("pre-complete typed mechanism definition was not wired as a blocking read: %s", downgrade)
	}
	pending := ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 1 || pending[0].LineRanges[0] != (types.LineRange{Start: 6, End: 8}) ||
		!types.PendingReadBlocksAcceptedClosure(pending[0]) || types.IsGenericForcedReadOrigin(pending[0].Origin) {
		t.Fatalf("typed mechanism definition pending read lost its exact citation-class contract: %+v", pending)
	}
}

func TestMechanismSemanticDescent_PreCompleteWiringConsumesTypedExecutableOwner(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	ctx := mechanismSemanticDescentContext(t, graph, 1, map[int][]repotypes.LineFeature{
		3: {repotypes.LineFeatureCallExpression},
	})
	fact := mechanismSemanticDescentFact()
	fact.Members = []string{"OutcomeRendered"}
	fact.MemberNotes = nil
	fact.SupportRefs = []string{"OutcomeRendered @ src/pipeline.go:1"}
	evidence := []types.EvidenceItem{{
		ID: "E-guard", Kind: types.EvidenceConditional,
		Condition: "input != empty", AnchorSymbol: "input",
		Source: "src/pipeline.go", LineStart: 3, Scope: types.ScopeLine,
		AnchorKind: types.AnchorCondition, Producer: types.EvidenceProducerExplorerEmitEvidence,
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}}

	downgrade := preCompleteContractCheckWithPreflight(ctx, "", completionPreflightView{
		Evidence: evidence, EffectiveAggregateFacts: []types.AnswerAggregateFact{fact},
	})
	if !strings.Contains(downgrade, "pending forced reads block the closure") ||
		!strings.Contains(downgrade, "src/pipeline.go") || !strings.Contains(downgrade, "Render") {
		t.Fatalf("pre-complete executable owner was not wired as a blocking read: %s", downgrade)
	}
	pending := ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 1 || pending[0].LineRanges[0] != (types.LineRange{Start: 2, End: 4}) ||
		!types.PendingReadBlocksAcceptedClosure(pending[0]) || types.IsGenericForcedReadOrigin(pending[0].Origin) {
		t.Fatalf("typed executable owner pending read lost its exact citation-class contract: %+v", pending)
	}
	if strings.Contains(pending[0].Rationale, "returns or delegates") || !strings.Contains(pending[0].Rationale, "sibling calls require their own typed operation evidence") {
		t.Fatalf("selected owner read must not claim an unproved delegation: %+v", pending[0])
	}
}

func TestMechanismSemanticDescent_PreCompleteWiringConsumesSelectedCallableDefinition(t *testing.T) {
	graph, _ := mechanismSemanticDescentFixture()
	ctx := mechanismSemanticDescentContext(t, graph, 1, nil)
	fact := types.AnswerAggregateFact{
		Kind: types.AnswerAggregateMemberSet, Value: "1", Members: []string{"OutcomeRendered"},
	}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceMechanism, AnchorSymbol: "Render",
		Source: "src/pipeline.go", LineStart: 2, Scope: types.ScopeLine,
		AnchorKind: types.AnchorDefinition, Producer: types.EvidenceProducerExplorerEmitEvidence,
		GroundingStatus: types.GroundingGrounded,
	}}

	downgrade := preCompleteContractCheckWithPreflight(ctx, "", completionPreflightView{
		Evidence: evidence, EffectiveAggregateFacts: []types.AnswerAggregateFact{fact},
	})
	if !strings.Contains(downgrade, "pending forced reads block the closure") ||
		!strings.Contains(downgrade, "src/pipeline.go") || !strings.Contains(downgrade, "Render") {
		t.Fatalf("pre-complete selected callable definition was not wired as a blocking read: %s", downgrade)
	}
	pending := ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 1 || pending[0].LineRanges[0] != (types.LineRange{Start: 2, End: 4}) ||
		!types.PendingReadBlocksAcceptedClosure(pending[0]) || types.IsGenericForcedReadOrigin(pending[0].Origin) {
		t.Fatalf("selected callable definition pending read lost its exact citation-class contract: %+v", pending)
	}
}
