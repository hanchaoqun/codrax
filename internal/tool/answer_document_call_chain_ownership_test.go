package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A call-chain semantic view may carry typed source/sink roles, but those roles
// only authorize direction checks and endpoint-boundary context. Pre-emit
// normalization still must not replace the model's conclusion or path.
// Individual structured edges remain subject to the ordinary typed evidence
// validators.
func TestNormalizeAnswerDocumentForPreEmit_CallChainConclusionRemainsModelOwned(t *testing.T) {
	mu := types.NewMutableState("narrative call-chain model ownership")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "explored call-chain nodes",
		Value: "5",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"VisitController.create",
			"VisitService.schedule",
			"VisitRepository.countOpenVisits",
			"VisitRepository.insert",
			"AuditLog.record",
		},
	}})
	mu.SetInvestigationComplete("narrative call-chain exploration complete")
	ctx := &types.BusContext{
		Language: "zh",
		Mutable:  mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:                   types.IntentTrace,
			PredicateAxis:            types.AxisCall,
			Language:                 "zh",
			CallChainEndpointProfile: &types.CallChainEndpointProfile{Source: "VisitController.create", Sink: "AuditLog.record"},
			Predicates: types.SemanticPredicates{
				IsRelationalLookup: true,
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFCallChain,
		RequiredMechanismAnchors: []types.AnswerRequiredAnchor{
			{Text: "VisitController", Kind: types.ContractTermSymbol},
			{Text: "VisitController.create", Kind: types.ContractTermSymbol},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "模型根据逐边证据归纳出的调用链结论。"},
		{
			ID:          "path",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetCurrentCodePath), string(types.FacetPrincipalPathEdge)},
			Items: []types.AnswerBlockItem{
				{ID: "controller", Label: "VisitController.create", Text: "调用服务层。", CitationRef: -1},
				{ID: "service", Label: "VisitService.schedule", Text: "完成容量检查并调用仓储层。", CitationRef: -1},
				{ID: "repository", Label: "VisitRepository.insert", Text: "写入后记录审计。", CitationRef: -1},
			},
		},
	}}
	want, err := json.Marshal(doc.Blocks)
	if err != nil {
		t.Fatalf("marshal precondition: %v", err)
	}

	normalizeAnswerDocumentForPreEmit("emit_answer_document", doc, view, ctx, newPreEmitCheckContext(ctx))

	got, err := json.Marshal(doc.Blocks)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("typed endpoint roles must not authorize replacing the model-authored call-chain conclusion or path:\n got=%s\nwant=%s", got, want)
	}
	if hints := preCheckAggregateMemberSetCoverage(doc, ctx); len(hints) != 0 {
		t.Fatalf("relation-only call-chain exploration member_set must not become a hard missing-row obligation: %+v", hints)
	}
	if hints := preCheckRelationMemberSetAnswerShape(doc, ctx); len(hints) != 0 {
		t.Fatalf("relation-only call-chain exploration member_set must not become a hard relation-table obligation: %+v", hints)
	}
}

func TestPreCheckCallChainEndpointBoundaryFacetOwnership_RejectsSiblingCallsOnlyFromPrincipalCarrier(t *testing.T) {
	mu := types.NewMutableState("typed endpoint-boundary facet ownership")
	evidence := []types.EvidenceItem{
		{ID: "source-edge", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "buildAnalysisIR", Predicate: "calls", Object: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 2722, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
		{ID: "sink-edge", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "gate.Run", Predicate: "calls", Object: "gate.RunWith", Source: "internal/analysis/gate/gate.go", LineStart: 135, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
		{ID: "sibling-edge", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "buildAnalysisIR", Predicate: "calls", Object: "normalizer.Normalize", Source: "internal/agent/analyzer.go", LineStart: 2321, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
	}
	mu.AppendEvidence(evidence)
	ctx := &types.BusContext{Mutable: mu, EvidenceItems: evidence}
	view := &types.AnswerSemanticView{
		Family: types.QFCallChain,
		CallChainEndpointBoundary: &types.CallChainEndpointBoundary{
			Disposition:    types.CallChainEndpointNoDirectedPath,
			SourceEndpoint: "buildAnalysisIR",
			RequestedSink:  "gate.Run",
			EvidenceCapsule: &types.CallChainEndpointEvidenceCapsule{
				Status:     types.CallChainEndpointEvidenceSharedCalleeBoundary,
				SourcePath: []types.CallChainEvidenceEdge{{From: "buildAnalysisIR", To: "gate.RunWith", EvidenceID: "source-edge", Source: "internal/agent/analyzer.go", LineStart: 2722}},
				SinkPath:   []types.CallChainEvidenceEdge{{From: "gate.Run", To: "gate.RunWith", EvidenceID: "sink-edge", Source: "internal/analysis/gate/gate.go", LineStart: 135}},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "internal/agent/analyzer.go", Line: 2722},
			{File: "internal/analysis/gate/gate.go", Line: 135},
			{File: "internal/agent/analyzer.go", Line: 2321},
		},
		Blocks: []types.AnswerBlock{
			{
				ID: "boundary", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
				FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
				ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, FacetID: string(types.FacetPrincipalPathEdge)}},
				Items: []types.AnswerBlockItem{
					{ID: "source", CitationRef: 0},
					{ID: "sink", CitationRef: 1},
					{ID: "sibling", CitationRef: 2},
				},
			},
			{
				ID: "support", Kind: types.BlockBulletList,
				Items: []types.AnswerBlockItem{{ID: "independent-local-call", CitationRef: 2}},
			},
		},
	}

	hints := preCheckCallChainEndpointBoundaryFacetOwnership(doc, view, newPreEmitCheckContext(ctx))
	if len(hints) != 1 || !strings.Contains(hints[0].ExpectedShape, `item="sibling"`) {
		t.Fatalf("principal boundary carrier must reject only the sibling item, got %+v", hints)
	}
	if strings.Contains(hints[0].ExpectedShape, `independent-local-call`) {
		t.Fatalf("the same grounded sibling must remain legal in a separate support block: %+v", hints[0])
	}
	tagged := tagPreEmitHints(types.ViolFacetUncovered, hints)
	hard, advisory := splitPreEmitHintsByGate(tagged)
	if len(hard) != 1 || len(advisory) != 0 || hard[0].HardSignal != preEmitHardSignalTypedFacetCandidateOwnership {
		t.Fatalf("exact facet-candidate mismatch must use the narrow typed hard lane: hard=%+v advisory=%+v", hard, advisory)
	}
	integrated := runPreEmitChecks(doc, view, nil, ctx)
	integratedFound := false
	for _, hint := range integrated {
		if hint.HardSignal == preEmitHardSignalTypedFacetCandidateOwnership {
			integratedFound = true
			break
		}
	}
	if !integratedFound {
		t.Fatalf("runPreEmitChecks lost the endpoint-boundary facet ownership wiring: %+v", integrated)
	}

	doc.Blocks[0].Items = doc.Blocks[0].Items[:2]
	if hints := preCheckCallChainEndpointBoundaryFacetOwnership(doc, view, newPreEmitCheckContext(ctx)); len(hints) != 0 {
		t.Fatalf("exact endpoint-boundary pair must pass while support facts remain available: %+v", hints)
	}
}

func TestNormalizeAnswerDocumentRowsBeforePersist_NoDirectedPathDoesNotAuthorReachableRoster(t *testing.T) {
	mu := types.NewMutableState("typed no-directed-path system supplement boundary")
	mu.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
		Reason:    types.PrincipalSpanWaiverNoDirectedPath,
		Rationale: "buildAnalysisIR reaches gate.RunWith while gate.Run calls RunWith",
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{resolvedNoPathAggregateFactForTest()})
	mu.SetInvestigationComplete("no directed path accepted")
	ctx := &types.BusContext{
		Language: "zh",
		Mutable:  mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:                   types.IntentTrace,
			PredicateAxis:            types.AxisCall,
			Language:                 "zh",
			CallChainEndpointProfile: &types.CallChainEndpointProfile{Source: "buildAnalysisIR", Sink: "gate.Run"},
			AnalyzerHints: types.AnalyzerHints{
				Kind:         string(types.ReqCallChain),
				ExactTargets: []string{"buildAnalysisIR", "gate.Run"},
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "模型将自行归纳端点边界。"},
	}}
	want, err := json.Marshal(doc.Blocks)
	if err != nil {
		t.Fatal(err)
	}

	normalizeAnswerDocumentRowsBeforePersist("emit_answer_document", ctx, doc)
	got, err := json.Marshal(doc.Blocks)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("system must not cast requested-but-unreachable sink as a reachable roster member:\n got=%s\nwant=%s", got, want)
	}
	if fixed := appendPrincipalEnumerationTypedSupplements(doc, ctx); fixed != 0 {
		t.Fatalf("principal enumeration supplement must be suppressed for typed no-directed-path, fixed=%d", fixed)
	}
	if fixed := normalizeAggregateMemberSetCarriers(doc, ctx); fixed != 0 {
		t.Fatalf("aggregate member-set supplement must be suppressed for typed no-directed-path, fixed=%d", fixed)
	}
}

func resolvedNoPathAggregateFactForTest() types.AnswerAggregateFact {
	return types.AnswerAggregateFact{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "buildAnalysisIR to gate.Run call chain",
		Value:   "3",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"buildAnalysisIR", "gate.RunWith", "gate.Run"},
		SupportRefs: []string{
			"internal/agent/analyzer.go:1822",
			"internal/agent/analyzer.go:2666",
			"internal/analysis/gate/gate.go:134",
		},
	}
}
