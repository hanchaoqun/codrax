package types

import (
	"strings"
	"testing"
)

// ── B1-T1 AnswerBlockKind 闭枚举 ───────────────────────────────────

func TestAllAnswerBlockKindsCovered(t *testing.T) {
	kinds := AllAnswerBlockKinds()
	if len(kinds) != 9 {
		t.Fatalf("expected 9 declared block kinds; got %d (%v)", len(kinds), kinds)
	}
	seen := make(map[AnswerBlockKind]bool, len(kinds))
	for _, k := range kinds {
		if k == "" {
			t.Errorf("empty AnswerBlockKind in AllAnswerBlockKinds()")
		}
		if seen[k] {
			t.Errorf("duplicate AnswerBlockKind %q in AllAnswerBlockKinds()", k)
		}
		seen[k] = true
	}
	// Spot-check a few canonical members.
	for _, want := range []AnswerBlockKind{BlockSummary, BlockDiagram, BlockCaveat, BlockOrderedList} {
		if !seen[want] {
			t.Errorf("AllAnswerBlockKinds() missing %q", want)
		}
	}
}

func TestIsValidAnswerBlockKind(t *testing.T) {
	for _, k := range AllAnswerBlockKinds() {
		if !IsValidAnswerBlockKind(k) {
			t.Errorf("declared kind %q not accepted by IsValidAnswerBlockKind", k)
		}
	}
	for _, bad := range []AnswerBlockKind{"", "shape", "list_of_symbols", "bogus"} {
		if IsValidAnswerBlockKind(bad) {
			t.Errorf("invalid kind %q accepted by IsValidAnswerBlockKind", bad)
		}
	}
}

// ── B1-T3 BuildAnswerSemanticView 骨架 ────────────────────────────

func TestBuildAnswerSemanticView_NilInputReturnsNil(t *testing.T) {
	if got := BuildAnswerSemanticView(nil, nil); got != nil {
		t.Errorf("nil ir should yield nil view; got %+v", got)
	}
	if got := BuildAnswerSemanticViewForAgentContext(nil); got != nil {
		t.Errorf("nil ac should yield nil view; got %+v", got)
	}
	if got := BuildAnswerSemanticViewForBusContext(nil); got != nil {
		t.Errorf("nil bus should yield nil view; got %+v", got)
	}
}

func TestBuildAnswerSemanticView_PreservesTypedRelationAxis(t *testing.T) {
	ir := &AnalysisIR{RequestModel: RequestModel{PredicateAxis: AxisFlow}}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil || view.RelationAxis != AxisFlow {
		t.Fatalf("relation axis = %q, want %q", view.RelationAxis, AxisFlow)
	}
	clone := cloneAnswerSemanticView(view)
	if clone == nil || clone.RelationAxis != AxisFlow {
		t.Fatalf("cloned relation axis = %q, want %q", clone.RelationAxis, AxisFlow)
	}
}

func TestApplyDiagramParticipantObligationsUsesRequiredTypedDiagramSlateAcrossRelationAxes(t *testing.T) {
	participants := []DiagramParticipantHint{
		{Identity: "Analyzer", Role: DiagramParticipantIncidentRequired, SourceQuote: "Analyzer"},
		{Identity: "BusContext", Role: DiagramParticipantContextOnly, SourceQuote: "BusContext"},
		{Identity: "", Role: DiagramParticipantIncidentRequired},
		{Identity: "Invalid", Role: DiagramParticipantRole("invalid")},
	}
	newView := func() *AnswerSemanticView {
		return &AnswerSemanticView{DiagramPlan: &DiagramFacetGraph{Kind: DiagramFlow, Required: true}}
	}
	newIR := func() *AnalysisIR {
		return &AnalysisIR{RequestModel: RequestModel{
			Intent: IntentExplain, PredicateAxis: AxisFlow,
			DiagramHint: &DiagramHint{Kind: DiagramFlow, Required: true, Participants: participants},
		}}
	}

	view := BuildAnswerSemanticView(newIR(), &AnswerSurfacePlan{Diagram: &DiagramContract{
		Required: true, PreferredKinds: []DiagramKind{DiagramFlow},
	}})
	if view == nil || view.DiagramPlan == nil || !view.DiagramPlan.Required {
		t.Fatalf("test fixture failed to compile a required flow diagram: %+v", view)
	}
	if len(view.DiagramParticipantObligations) != 2 ||
		view.DiagramParticipantObligations[0].Identity != "Analyzer" ||
		view.DiagramParticipantObligations[1].Role != DiagramParticipantContextOnly {
		t.Fatalf("typed valid participant slate was not copied exactly: %+v", view.DiagramParticipantObligations)
	}
	clone := cloneAnswerSemanticView(view)
	clone.DiagramParticipantObligations[0].Identity = "mutated"
	if view.DiagramParticipantObligations[0].Identity != "Analyzer" {
		t.Fatalf("semantic-view cache clone aliases participant obligations: %+v", view.DiagramParticipantObligations)
	}

	for name, mutate := range map[string]func(*AnalysisIR, *AnswerSemanticView){
		"root cause trace": func(ir *AnalysisIR, _ *AnswerSemanticView) {
			ir.RequestModel.Intent = IntentRootCause
			ir.RequestModel.Scenario = ScenarioRootCause
		},
		"hint optional": func(ir *AnalysisIR, _ *AnswerSemanticView) { ir.RequestModel.DiagramHint.Required = false },
		"plan optional": func(_ *AnalysisIR, view *AnswerSemanticView) { view.DiagramPlan.Required = false },
	} {
		t.Run(name, func(t *testing.T) {
			ir, inactive := newIR(), newView()
			mutate(ir, inactive)
			applyDiagramParticipantObligations(inactive, ir)
			if len(inactive.DiagramParticipantObligations) != 0 {
				t.Fatalf("inactive lane leaked participant obligations: %+v", inactive.DiagramParticipantObligations)
			}
		})
	}

	callChain := newIR()
	callChain.RequestModel.Intent = IntentTrace
	callChain.RequestModel.PredicateAxis = AxisCall
	callChain.RequestModel.DiagramHint.Kind = DiagramSequence
	callView := &AnswerSemanticView{DiagramPlan: &DiagramFacetGraph{Kind: DiagramSequence, Required: true}}
	applyDiagramParticipantObligations(callView, callChain)
	if len(callView.DiagramParticipantObligations) != 2 {
		t.Fatalf("required source call-chain participants must retain the typed slate: %+v", callView.DiagramParticipantObligations)
	}
}

func TestApplyDiagramParticipantObligationsIgnoresUnanchoredAnalyzerGuess(t *testing.T) {
	view := &AnswerSemanticView{DiagramPlan: &DiagramFacetGraph{Kind: DiagramFlow, Required: true}}
	ir := &AnalysisIR{RequestModel: RequestModel{
		Intent:        IntentExplain,
		PredicateAxis: AxisFlow,
		DiagramHint: &DiagramHint{Kind: DiagramFlow, Required: true, Participants: []DiagramParticipantHint{
			{Identity: "AnalyzerInventedComponent", Role: DiagramParticipantIncidentRequired},
		}},
	}}
	applyDiagramParticipantObligations(view, ir)
	if len(view.DiagramParticipantObligations) != 0 {
		t.Fatalf("participant without validated current-request provenance became a hard obligation: %+v", view.DiagramParticipantObligations)
	}
}

func TestBuildAnswerSemanticView_SourceInventoryRowIdentityUsesTypedPlan(t *testing.T) {
	ir := &AnalysisIR{RequestModel: RequestModel{
		Intent: IntentEnumerate,
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
		},
	}}
	view := BuildAnswerSemanticView(ir, &AnswerSurfacePlan{
		SourceInventoryObservation: SourceInventoryObservation{
			Active: true,
			Sets:   []SourceInventoryObservationSet{{Role: AnswerCandidateRoleType}},
		},
	})
	if view == nil || !view.SourceInventoryRowIdentityAvailable {
		t.Fatalf("typed source-inventory plan should expose row identity fields: %+v", view)
	}
	view = BuildAnswerSemanticView(ir, &AnswerSurfacePlan{})
	if view == nil || view.SourceInventoryRowIdentityAvailable {
		t.Fatalf("profile without typed inventory observation must not expose row identity fields: %+v", view)
	}
}

func TestBuildAnswerSemanticView_ItemEvidenceIdentityUsesCurrentSourcePlan(t *testing.T) {
	ir := &AnalysisIR{RequestModel: RequestModel{Intent: IntentExplain}}
	view := BuildAnswerSemanticView(ir, &AnswerSurfacePlan{CurrentSourceEvidenceOrigin: true})
	if view == nil || !view.ItemEvidenceIdentityAvailable {
		t.Fatalf("typed current-source plan should expose optional item evidence identity: %+v", view)
	}
	clone := cloneAnswerSemanticView(view)
	if clone == nil || !clone.ItemEvidenceIdentityAvailable {
		t.Fatalf("semantic-view clone lost item evidence identity availability: %+v", clone)
	}
	view = BuildAnswerSemanticView(ir, &AnswerSurfacePlan{})
	if view == nil || view.ItemEvidenceIdentityAvailable {
		t.Fatalf("non-current-source plan must not expose item evidence identity: %+v", view)
	}
}

func TestCompileCallChainEndpointBoundary_UsesTypedNoDirectedPathOnly(t *testing.T) {
	rm := RequestModel{
		Intent:                   IntentTrace,
		PredicateAxis:            AxisCall,
		CallChainEndpointProfile: &CallChainEndpointProfile{Source: "buildAnalysisIR", Sink: "gate.Run"},
		AnalyzerHints: AnalyzerHints{
			Kind:         string(ReqCallChain),
			ExactTargets: []string{"buildAnalysisIR", "gate.Run"},
		},
	}
	waiver := &PrincipalSpanWaiver{
		Reason:    PrincipalSpanWaiverNoDirectedPath,
		Rationale: "typed call edges stop at gate.RunWith and gate.Run points the other way",
	}
	got := CompileCallChainEndpointBoundary(rm, waiver)
	if got == nil || !got.Active() {
		t.Fatalf("typed no-directed-path waiver should compile a boundary: %+v", got)
	}
	if got.Disposition != CallChainEndpointNoDirectedPath ||
		got.SourceEndpoint != "buildAnalysisIR" || got.RequestedSink != "gate.Run" {
		t.Fatalf("unexpected boundary: %+v", got)
	}

	adjacent := *waiver
	adjacent.Reason = PrincipalSpanWaiverEndpointsDirectlyAdjacent
	if got := CompileCallChainEndpointBoundary(rm, &adjacent); got != nil {
		t.Fatalf("an adjacency/span waiver must not become a missing-path boundary: %+v", got)
	}
	withoutEndpoints := rm
	withoutEndpoints.CallChainEndpointProfile = nil
	if got := CompileCallChainEndpointBoundary(withoutEndpoints, waiver); got != nil {
		t.Fatalf("a single endpoint cannot compile a source/sink boundary: %+v", got)
	}
	rootCauseTrace := rm
	rootCauseTrace.Intent = IntentRootCause
	rootCauseTrace.Scenario = ScenarioRootCause
	if ResolveQuestionFamily(rootCauseTrace) != QFRootCauseTrace {
		t.Fatalf("test fixture must resolve to RootCauseTrace, got %s", ResolveQuestionFamily(rootCauseTrace))
	}
	if got := CompileCallChainEndpointBoundary(rootCauseTrace, waiver); got != nil {
		t.Fatalf("source-call endpoint capsule must not enter RootCauseTrace authority: %+v", got)
	}
}

func TestBuildAnswerSemanticViewForAgentContext_RefreshesTypedCallChainBoundary(t *testing.T) {
	mut := NewMutableState("call chain endpoint boundary")
	ir := &AnalysisIR{RequestModel: RequestModel{
		Intent:                   IntentTrace,
		PredicateAxis:            AxisCall,
		CallChainEndpointProfile: &CallChainEndpointProfile{Source: "Source.run", Sink: "Sink.run"},
		AnalyzerHints: AnalyzerHints{
			Kind:         string(ReqCallChain),
			ExactTargets: []string{"Source.run", "Sink.run"},
		},
	}}
	ctx := &AgentContext{AnalysisIR: ir, Mutable: mut}
	if got := BuildAnswerSemanticViewForAgentContext(ctx); got == nil || got.CallChainEndpointBoundary != nil {
		t.Fatalf("boundary must be absent before the typed waiver: %+v", got)
	}
	mut.SetPrincipalSpanWaiver(&PrincipalSpanWaiver{
		Reason:    PrincipalSpanWaiverNoDirectedPath,
		Rationale: "source inspection found no same-direction path",
	})
	got := BuildAnswerSemanticViewForAgentContext(ctx)
	if got == nil || got.CallChainEndpointBoundary == nil ||
		got.CallChainEndpointBoundary.RequestedSink != "Sink.run" {
		t.Fatalf("cached semantic view did not refresh the mutable typed boundary: %+v", got)
	}
	mut.ClearPrincipalSpanWaiver()
	if got := BuildAnswerSemanticViewForAgentContext(ctx); got == nil || got.CallChainEndpointBoundary != nil {
		t.Fatalf("cleared waiver must retract the semantic boundary: %+v", got)
	}
}

func TestBuildAnswerSemanticViewForAgentContext_EndpointBoundaryUsesFinalizerHandoffEvidence(t *testing.T) {
	mut := NewMutableState("call chain finalizer handoff")
	mut.SetPrincipalSpanWaiver(&PrincipalSpanWaiver{
		Reason:    PrincipalSpanWaiverNoDirectedPath,
		Rationale: "source inspection found no same-direction path",
	})
	ir := &AnalysisIR{RequestModel: RequestModel{
		Intent: IntentTrace, PredicateAxis: AxisCall,
		CallChainEndpointProfile: &CallChainEndpointProfile{Source: "buildAnalysisIR", Sink: "gate.Run"},
		AnalyzerHints: AnalyzerHints{
			Kind:         string(ReqCallChain),
			ExactTargets: []string{"buildAnalysisIR", "gate.Run"},
		},
	}}
	ctx := &AgentContext{
		AnalysisIR: ir,
		Mutable:    mut,
		EvidenceItems: []EvidenceItem{
			{ID: "E1", Kind: EvidenceRelationship, AnchorKind: AnchorCall, Subject: "buildAnalysisIR", Object: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 2666, GroundingStatus: GroundingGrounded},
			{ID: "D1", Kind: EvidenceDirect, AnchorKind: AnchorDefinition, Subject: "gate.Run", AnchorSymbol: "Run", Source: "internal/analysis/gate/gate.go", LineStart: 134, GroundingStatus: GroundingGrounded},
		},
	}

	got := BuildAnswerSemanticViewForAgentContext(ctx)
	if got == nil || got.CallChainEndpointBoundary == nil || got.CallChainEndpointBoundary.EvidenceCapsule == nil {
		t.Fatalf("finalizer handoff did not produce endpoint evidence capsule: %+v", got)
	}
	capsule := got.CallChainEndpointBoundary.EvidenceCapsule
	if capsule.SourceProof != CallChainEndpointExistenceCallEdge || capsule.RequestedSinkProof != CallChainEndpointExistenceDefinitionOnly {
		t.Fatalf("finalizer handoff proof drifted from completion authority: %+v", capsule)
	}

	bus := &BusContext{
		AnalysisIR:    ir,
		Mutable:       mut,
		EvidenceItems: ctx.EvidenceItems,
	}
	got = BuildAnswerSemanticViewForBusContext(bus)
	if got == nil || got.CallChainEndpointBoundary == nil || got.CallChainEndpointBoundary.EvidenceCapsule == nil {
		t.Fatalf("bus handoff did not produce endpoint evidence capsule: %+v", got)
	}
	capsule = got.CallChainEndpointBoundary.EvidenceCapsule
	if capsule.SourceProof != CallChainEndpointExistenceCallEdge || capsule.RequestedSinkProof != CallChainEndpointExistenceDefinitionOnly {
		t.Fatalf("bus handoff proof drifted from completion authority: %+v", capsule)
	}
}

// AnswerShape constants retired in PR5 of the AnswerShape
// terminal-retirement migration. The
// "EveryShapeProducesNonNilView" loop test that lived here is
// gone with them — V2 view rendering keys off QuestionFamily
// (covered by the QuestionFamily helper tests in
// answer_semantic_view_helpers_test.go).

func TestBuildAnswerSemanticView_FacetCoverageAliasedFromPlan(t *testing.T) {
	fc := &FacetCoverageContract{Family: QFRoleLookup}
	plan := &AnswerSurfacePlan{
		FacetCoverage:      fc,
		SummarySurfaceMode: AnswerSummarySurfaceMinimalScalarRoleLocate,
	}
	ir := &AnalysisIR{
		RequestModel: RequestModel{Intent: IntentExplain},
	}
	view := BuildAnswerSemanticView(ir, plan)
	if view == nil {
		t.Fatal("view nil")
	}
	if view.FacetCoverage != fc {
		t.Errorf("FacetCoverage not aliased from plan; got %p want %p", view.FacetCoverage, fc)
	}
	if view.SummaryMode != AnswerSummarySurfaceMinimalScalarRoleLocate {
		t.Errorf("SummaryMode not propagated; got %q", view.SummaryMode)
	}
}

func TestBuildAnswerSemanticView_ExactResolutionPropagated(t *testing.T) {
	er := &ExactResolutionContract{}
	ir := &AnalysisIR{
		RequestModel: RequestModel{Intent: IntentExplain},
		AnswerContract: AnswerContract{
			ExactResolution: er,
		},
	}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil || view.ExactResolution != er {
		t.Errorf("ExactResolution not aliased; view=%+v", view)
	}
}

func TestBuildAnswerSemanticView_RequestedCandidateRolesPropagated(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Intent: IntentReturnValue,
			Predicates: SemanticPredicates{
				IsScalarAnswer: true,
			},
			AnswerRoleProfile: &AnswerRoleProfile{
				IsRoleBindingRequested: true,
				RequiredCandidateRoles: []AnswerCandidateRole{
					AnswerCandidateRoleBudgetCap,
					AnswerCandidateRoleBudgetCap,
					AnswerCandidateRoleAttemptCounter,
				},
			},
		},
	}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil {
		t.Fatal("view nil")
	}
	if got := view.RequiredCandidateRoles; len(got) != 2 ||
		got[0] != AnswerCandidateRoleBudgetCap ||
		got[1] != AnswerCandidateRoleAttemptCounter {
		t.Fatalf("required candidate roles not propagated/deduped: %+v", got)
	}
}

func TestBuildAnswerSemanticView_RequiredMechanismAnchorsFromTypedExactLane(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Intent:   IntentExplain,
			Scenario: ScenarioArchitectureExplain,
			AnalyzerHints: AnalyzerHints{
				MentionedEntities: []string{"helperCandidate", "kind", "json"},
				ExactTargets:      []string{"runTaskGraph", "emit_evidence"},
			},
		},
		AnswerContract: AnswerContract{
			MustIncludeTerms: []ContractTerm{
				{Text: "runTaskGraph", Kind: ContractTermSymbol, Source: ContractTermSourceAnalyzerEntity},
				{Text: "emit_evidence", Kind: ContractTermToolName, Source: ContractTermSourceAnalyzerEntity},
				{Text: "derivedOnly", Kind: ContractTermSymbol, Source: ContractTermSourceAnalyzerEntity},
				{Text: "upstream evidence node", Kind: ContractTermUserPhrase},
			},
		},
	}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil {
		t.Fatal("view nil")
	}
	got := view.RequiredMechanismAnchors
	if len(got) != 2 {
		t.Fatalf("required anchors len=%d want 2: %+v", len(got), got)
	}
	if got[0].Text != "runTaskGraph" || got[0].Kind != ContractTermSymbol {
		t.Fatalf("first anchor = %+v, want runTaskGraph symbol", got[0])
	}
	if got[1].Text != "emit_evidence" || got[1].Kind != ContractTermToolName {
		t.Fatalf("second anchor = %+v, want emit_evidence tool_name", got[1])
	}
}

func TestBuildAnswerSemanticView_RequiredMechanismAnchorsDisabledForScalar(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Intent: IntentReturnValue,
			Predicates: SemanticPredicates{
				IsScalarAnswer: true,
			},
			AnalyzerHints: AnalyzerHints{
				MentionedEntities: []string{"runTaskGraph"},
			},
		},
		AnswerContract: AnswerContract{
			MustIncludeTerms: []ContractTerm{{Text: "runTaskGraph", Kind: ContractTermSymbol}},
		},
	}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil {
		t.Fatal("view nil")
	}
	if len(view.RequiredMechanismAnchors) != 0 {
		t.Fatalf("scalar lookup should not gain mechanism anchors: %+v", view.RequiredMechanismAnchors)
	}
}

func TestMissingRequiredMechanismAnchors_UsesStructuredFieldsOnly(t *testing.T) {
	required := []AnswerRequiredAnchor{
		{Text: "runTaskGraph", Kind: ContractTermSymbol},
		{Text: "EdgeValidationFeedback", Kind: ContractTermSymbol},
	}
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{
		{
			ID:   "summary",
			Kind: BlockSummary,
			Text: "The prose mentions EdgeValidationFeedback, but prose is not the anchor carrier.",
		},
		{
			ID:    "anchors",
			Kind:  BlockOrderedList,
			Items: []AnswerBlockItem{{Label: "`runTaskGraph`"}},
		},
	}}
	missing := MissingRequiredMechanismAnchors(doc, required)
	if len(missing) != 1 || missing[0].Text != "EdgeValidationFeedback" {
		t.Fatalf("missing anchors = %+v, want EdgeValidationFeedback only", missing)
	}
	doc.Blocks[1].Items = append(doc.Blocks[1].Items, AnswerBlockItem{Label: "EdgeValidationFeedback()"})
	if missing := MissingRequiredMechanismAnchors(doc, required); len(missing) != 0 {
		t.Fatalf("structured item labels should satisfy anchors, missing %+v", missing)
	}
}

func TestMissingRequiredMechanismAnchors_StructuredQualifiedLabelsSatisfyParts(t *testing.T) {
	required := []AnswerRequiredAnchor{
		{Text: "StageOutput", Kind: ContractTermSymbol},
		{Text: "AnalysisIR", Kind: ContractTermSymbol},
	}
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID:   "mechanism",
		Kind: BlockSection,
		Items: []AnswerBlockItem{{
			Label: "StageOutput.AnalysisIR",
			Text:  "AnalysisIR is the analyzer result field on StageOutput.",
		}},
	}}}
	if missing := MissingRequiredMechanismAnchors(doc, required); len(missing) != 0 {
		t.Fatalf("qualified structured labels should satisfy owner/member anchors, missing %+v", missing)
	}
}

func TestMissingRequiredMechanismAnchors_QualifiedRequiredEndpointIsExact(t *testing.T) {
	required := []AnswerRequiredAnchor{{Text: "gate.Run", Kind: ContractTermSymbol}}
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID:   "mechanism",
		Kind: BlockSection,
		Items: []AnswerBlockItem{{
			Label: "gate.RunWith",
			Text:  "The collected call edge reaches RunWith, not the requested Run endpoint.",
		}},
	}}}
	missing := MissingRequiredMechanismAnchors(doc, required)
	if len(missing) != 1 || missing[0].Text != "gate.Run" {
		t.Fatalf("qualified sibling must not satisfy exact endpoint, missing=%+v", missing)
	}
	doc.Blocks[0].Items = append(doc.Blocks[0].Items, AnswerBlockItem{
		Label: "gate.Run",
		Text:  "No citable call-edge path to this exact endpoint was established.",
	})
	if missing := MissingRequiredMechanismAnchors(doc, required); len(missing) != 0 {
		t.Fatalf("exact qualified endpoint should satisfy itself, missing=%+v", missing)
	}
	doc.Blocks[0].Items = nil
	doc.Blocks[0].Items = []AnswerBlockItem{{Cells: []string{
		"requested endpoint", "gate.RunWith is the observed callee",
	}}}
	if missing := MissingRequiredMechanismAnchors(doc, required); len(missing) != 1 {
		t.Fatalf("qualified sibling in a table cell must not satisfy exact endpoint, missing=%+v", missing)
	}
	doc.Blocks[0].Items[0].Cells[1] = "gate.Run (requested endpoint): no citable path established"
	if missing := MissingRequiredMechanismAnchors(doc, required); len(missing) != 0 {
		t.Fatalf("annotated exact endpoint in a table cell should satisfy it, missing=%+v", missing)
	}
}

func TestMissingRequiredMechanismAnchors_StructuredIdentifierVariantSatisfiesToolName(t *testing.T) {
	required := []AnswerRequiredAnchor{
		{Text: "emit_analysis", Kind: ContractTermToolName},
	}
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID:   "mechanism",
		Kind: BlockSection,
		Items: []AnswerBlockItem{{
			Label: "EmitAnalysis",
			Text:  "tool carrier type",
		}},
	}}}
	if missing := MissingRequiredMechanismAnchors(doc, required); len(missing) != 0 {
		t.Fatalf("structured code identifier variants should satisfy tool-name anchors, missing %+v", missing)
	}
}

func TestMissingRequiredMechanismAnchors_TypedRelationLabelsCarryExactEndpoints(t *testing.T) {
	required := []AnswerRequiredAnchor{
		{Text: "buildAnalysisIR", Kind: ContractTermSymbol},
		{Text: "gate.Run", Kind: ContractTermSymbol},
	}
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID:        "edges",
		Kind:      BlockOrderedList,
		ClaimUses: []RenderedClaimUse{{ClaimForm: ClaimCallEdge}},
		Items: []AnswerBlockItem{
			{Label: "buildAnalysisIR -> gate.RunWith"},
			{Label: "gate.Run → RunWith"},
		},
	}}}
	if missing := MissingRequiredMechanismAnchors(doc, required); len(missing) != 0 {
		t.Fatalf("typed relation labels should carry their exact endpoints, missing=%+v", missing)
	}

	doc.Blocks[0].Items[1].Label = "gate.RunWith -> RunWith"
	missing := MissingRequiredMechanismAnchors(doc, required)
	if len(missing) != 1 || missing[0].Text != "gate.Run" {
		t.Fatalf("qualified sibling endpoint must not satisfy gate.Run, missing=%+v", missing)
	}
}

func TestMissingRequiredMechanismAnchors_TypedDiagramAliasesCarryDeclaredEndpointsOnly(t *testing.T) {
	required := []AnswerRequiredAnchor{
		{Text: "buildAnalysisIR", Kind: ContractTermSymbol},
		{Text: "gate.RunWith", Kind: ContractTermSymbol},
	}
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "diagram", Kind: BlockDiagram,
		Diagram: &AnswerDiagramBlock{Kind: DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant IR as buildAnalysisIR",
			"  participant RW as gate.RunWith",
			"  IR->>RW: resolve(json)",
		}, "\n")},
		EdgeAnchors: []DiagramEdgeAnchor{{FromNode: "IR", ToNode: "RW", RelationKind: DiagramRelCall, ClaimForm: ClaimCallEdge}},
	}}}
	if missing := MissingRequiredMechanismAnchors(doc, required); len(missing) != 0 {
		t.Fatalf("typed diagram aliases should carry declared endpoint identities, missing=%+v", missing)
	}

	// Message payload is display text, not endpoint identity. The same typed
	// edge must not satisfy an unrelated exact anchor merely because the
	// operation appears after the sequence message colon.
	messageOnly := []AnswerRequiredAnchor{{Text: "resolve", Kind: ContractTermSymbol}}
	if missing := MissingRequiredMechanismAnchors(doc, messageOnly); len(missing) != 1 {
		t.Fatalf("sequence message payload minted endpoint identity: %+v", missing)
	}
}

func TestMissingRequiredMechanismAnchors_TypedFlowchartAliasesCarryLanguageNeutralEndpoints(t *testing.T) {
	for _, identity := range []string{
		"Go.Service.Run", "JavaService.run", "PythonService.run", "ArkTSService.run",
		"cangjie::Service::run", "rust::service::run", "CppService::run",
	} {
		t.Run(identity, func(t *testing.T) {
			doc := &AnswerDocumentV2{Blocks: []AnswerBlock{{
				ID: "flow", Kind: BlockDiagram,
				Diagram: &AnswerDiagramBlock{Kind: DiagramFlow, Language: "mermaid", Body: strings.Join([]string{
					"flowchart TD",
					"  A[\"" + identity + "\"] --> B[\"Sink.write\"]",
				}, "\n")},
				EdgeAnchors: []DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: DiagramRelCall, ClaimForm: ClaimCallEdge}},
			}}}
			if missing := MissingRequiredMechanismAnchors(doc, []AnswerRequiredAnchor{{Text: identity, Kind: ContractTermSymbol}}); len(missing) != 0 {
				t.Fatalf("flowchart alias lost %q: %+v", identity, missing)
			}
		})
	}
}

func TestMissingRequiredMechanismAnchors_AmbiguousDiagramAliasFailsClosed(t *testing.T) {
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{
		{
			ID: "d1", Kind: BlockDiagram,
			Diagram:     &AnswerDiagramBlock{Kind: DiagramSequence, Body: "sequenceDiagram\n participant A as First.Run\n participant B as Sink.Run\n A->>B: call"},
			EdgeAnchors: []DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: DiagramRelCall, ClaimForm: ClaimCallEdge}},
		},
		{
			ID: "d2", Kind: BlockDiagram,
			Diagram:     &AnswerDiagramBlock{Kind: DiagramSequence, Body: "sequenceDiagram\n participant A as Second.Run\n participant C as Sink.Run\n A->>C: call"},
			EdgeAnchors: []DiagramEdgeAnchor{{FromNode: "A", ToNode: "C", RelationKind: DiagramRelCall, ClaimForm: ClaimCallEdge}},
		},
	}}
	for _, identity := range []string{"First.Run", "Second.Run"} {
		if missing := MissingRequiredMechanismAnchors(doc, []AnswerRequiredAnchor{{Text: identity, Kind: ContractTermSymbol}}); len(missing) != 1 {
			t.Fatalf("ambiguous reused alias should not choose %q: %+v", identity, missing)
		}
	}
}

func TestMissingRequiredMechanismAnchors_RelationLikeFreeLabelsDoNotMintEndpoints(t *testing.T) {
	required := []AnswerRequiredAnchor{{Text: "gate.Run", Kind: ContractTermSymbol}}
	for name, block := range map[string]AnswerBlock{
		"untyped_relation_label": {
			ID: "untyped", Kind: BlockOrderedList,
			Items: []AnswerBlockItem{{Label: "gate.Run -> RunWith"}},
		},
		"definition_claim_block": {
			ID: "definition", Kind: BlockOrderedList,
			ClaimUses: []RenderedClaimUse{{ClaimForm: ClaimDefinitionFact}},
			Items:     []AnswerBlockItem{{Label: "gate.Run -> RunWith"}},
		},
		"cpp_member_access": {
			ID: "cpp", Kind: BlockOrderedList,
			ClaimUses: []RenderedClaimUse{{ClaimForm: ClaimCallEdge}},
			Items:     []AnswerBlockItem{{Label: "gate.Run->RunWith"}},
		},
		"multi_hop_relation": {
			ID: "chain", Kind: BlockOrderedList,
			ClaimUses: []RenderedClaimUse{{ClaimForm: ClaimCallEdge}},
			Items:     []AnswerBlockItem{{Label: "gate.Run -> helper -> RunWith"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			missing := MissingRequiredMechanismAnchors(&AnswerDocumentV2{Blocks: []AnswerBlock{block}}, required)
			if len(missing) != 1 || missing[0].Text != "gate.Run" {
				t.Fatalf("non-authoritative relation-like label minted an endpoint: %+v", missing)
			}
		})
	}
}

func TestBuildAnswerSemanticView_ErrorGranularityRequiresPrincipalDecision(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Intent: IntentReturnValue,
			Predicates: SemanticPredicates{
				IsScalarAnswer: true,
			},
			ErrorGranularityProfile: &ErrorGranularityProfile{
				IsGranularityQuestion: true,
				Confidence:            0.9,
			},
		},
	}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil {
		t.Fatal("view nil")
	}
	if view.ErrorGranularityProfile == nil || !view.ErrorGranularityProfile.Active() {
		t.Fatalf("error granularity profile not propagated: %+v", view.ErrorGranularityProfile)
	}
	for _, req := range view.RequiredBlocks {
		if req.Kind != BlockDecision {
			continue
		}
		if !req.Required || req.MinCount != 1 || req.MaxCount != 1 || req.SurfaceRoleHint != SurfacePrincipal {
			t.Fatalf("decision block requirement wrong: %+v", req)
		}
		return
	}
	t.Fatalf("missing required decision block: %+v", view.RequiredBlocks)
}

func TestBuildAnswerSemanticView_DiagnosticMechanismDoesNotRequireErrorGranularityDecision(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Intent:        IntentRootCause,
			Scenario:      ScenarioRootCause,
			AnalyzerHints: AnalyzerHints{Kind: string(ReqMechanism)},
			Predicates: SemanticPredicates{
				IsDiagnosticQuestion: true,
			},
			ErrorGranularityProfile: &ErrorGranularityProfile{
				IsGranularityQuestion: true,
				RequestedVerdictOptions: []ErrorGranularityVerdict{
					ErrorGranularityPerItemRejection,
					ErrorGranularityWholeBatch,
				},
				Confidence: 0.85,
			},
		},
	}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil {
		t.Fatal("view nil")
	}
	if view.ErrorGranularityProfile != nil {
		t.Fatalf("diagnostic mechanism explanation must not propagate error granularity profile: %+v", view.ErrorGranularityProfile)
	}
	for _, req := range view.RequiredBlocks {
		if req.Kind == BlockDecision && req.SurfaceRoleHint == SurfacePrincipal {
			t.Fatalf("diagnostic mechanism explanation should not add principal decision block: %+v", req)
		}
	}
}
