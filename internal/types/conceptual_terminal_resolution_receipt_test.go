package types

import "testing"

func TestConceptualTerminalResolutionContractUsesOnlyTypedTerminalBodyCalls(t *testing.T) {
	profile := &CallChainEndpointProfile{
		Source:   "VisitController.create",
		SinkMode: CallChainSinkResolutionDiscoverTerminal,
	}
	good := EvidenceItem{
		ID:         "ev-terminal",
		Kind:       EvidenceRelationship,
		Subject:    "AuditLog.record",
		Predicate:  "calls",
		Object:     "System.out.println",
		Source:     "src/AuditLog.java",
		LineStart:  6,
		LineEnd:    6,
		Producer:   EvidenceProducerRepoMapTerminalBodyCall,
		Scope:      ScopeLine,
		AnchorKind: AnchorCall,
	}
	noise := good
	noise.ID = "ev-noise"
	noise.Producer = "test_non_terminal_call"
	duplicateCoordinate := good
	duplicateCoordinate.ID = "ev-terminal-duplicate"
	contract := BuildConceptualTerminalResolutionContract(profile, []EvidenceItem{noise, good, good, duplicateCoordinate})
	if contract == nil || len(contract.Rows) != 1 {
		t.Fatalf("conceptual terminal rows=%+v", contract)
	}
	row := contract.Rows[0]
	if row.EvidenceID != "ev-terminal" || row.TerminalCallable != "AuditLog.record" ||
		row.ExactOperation != "System.out.println" || row.Source != "src/AuditLog.java:6" {
		t.Fatalf("unexpected exact terminal row: %+v", row)
	}
	if len(row.AllowedConclusions) != 3 {
		t.Fatalf("model-owned conclusion choices=%v", row.AllowedConclusions)
	}

	receipt := &AnswerConceptualTerminalResolutionReceipt{
		EvidenceID: "ev-terminal",
		Conclusion: ConceptualTerminalResolutionCurrentTerminalDiffers,
	}
	if !BindConceptualTerminalResolutionReceipt(receipt, contract) || !receipt.IsBound() || receipt.BoundRow.ExactOperation != "System.out.println" {
		t.Fatalf("exact model selection did not bind: %+v", receipt)
	}
	bad := &AnswerConceptualTerminalResolutionReceipt{
		EvidenceID: "ev-noise",
		Conclusion: ConceptualTerminalResolutionDestinationSupported,
	}
	if BindConceptualTerminalResolutionReceipt(bad, contract) {
		t.Fatal("non-published evidence must not bind")
	}
}

func TestConceptualTerminalResolutionNoEvidenceAllowsOnlyModelUnproven(t *testing.T) {
	profile := &CallChainEndpointProfile{Source: "Source.run", SinkMode: CallChainSinkResolutionDiscoverTerminal}
	contract := BuildConceptualTerminalResolutionContract(profile, nil)
	if contract == nil || len(contract.Rows) != 0 {
		t.Fatalf("empty conceptual contract=%+v", contract)
	}
	unproven := &AnswerConceptualTerminalResolutionReceipt{
		Conclusion: ConceptualTerminalResolutionDestinationUnproven,
	}
	if !BindConceptualTerminalResolutionReceipt(unproven, contract) || !unproven.IsBound() {
		t.Fatalf("empty-evidence unproven selection did not bind: %+v", unproven)
	}
	unsupported := &AnswerConceptualTerminalResolutionReceipt{
		Conclusion: ConceptualTerminalResolutionDestinationSupported,
	}
	if BindConceptualTerminalResolutionReceipt(unsupported, contract) {
		t.Fatal("no-evidence contract must not admit a positive destination conclusion")
	}
	exactProfile := &CallChainEndpointProfile{Source: "A.run", Sink: "B.run", SinkMode: CallChainSinkResolutionExact}
	if got := BuildConceptualTerminalResolutionContract(exactProfile, nil); got != nil {
		t.Fatalf("exact code endpoint request must not inherit conceptual terminal obligation: %+v", got)
	}
	discoverPath := &CallChainEndpointProfile{SinkMode: CallChainSinkResolutionDiscoverPath}
	if got := BuildConceptualTerminalResolutionContract(discoverPath, nil); got != nil {
		t.Fatalf("role-bound path discovery must not inherit the stricter terminal-body conclusion obligation: %+v", got)
	}
}

func TestConceptualTerminalResolutionRoleIsDerivedNotAnalyzerFacing(t *testing.T) {
	for _, role := range AllRequestedAnswerDimensionRoles() {
		if role == RequestedAnswerDimensionConceptualTerminalResolution {
			t.Fatal("derived conceptual-terminal role must not increase analyzer JSON enum mind or create a role/schema conflict")
		}
	}
}

func TestConceptualTerminalResolutionClonesContractAndBoundReceipt(t *testing.T) {
	view := &AnswerSemanticView{
		ConceptualTerminalResolutionContract: &ConceptualTerminalResolutionContract{
			Rows: []ConceptualTerminalResolutionRow{{
				EvidenceID: "ev-terminal",
				AllowedConclusions: []ConceptualTerminalResolutionConclusion{
					ConceptualTerminalResolutionCurrentTerminalDiffers,
				},
			}},
		},
	}
	clonedView := cloneAnswerSemanticView(view)
	clonedView.ConceptualTerminalResolutionContract.Rows[0].AllowedConclusions[0] = ConceptualTerminalResolutionDestinationUnproven
	if view.ConceptualTerminalResolutionContract.Rows[0].AllowedConclusions[0] != ConceptualTerminalResolutionCurrentTerminalDiffers {
		t.Fatal("semantic-view clone aliased conceptual-terminal conclusion choices")
	}

	mutable := NewMutableState("conceptual terminal")
	mutable.SetAnswerDocumentV2WithMutation(MutationReplaceAll, &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "summary", Kind: BlockSummary, Text: "model conclusion",
		ConceptualTerminalResolution: &AnswerConceptualTerminalResolutionReceipt{
			EvidenceID: "ev-terminal",
			Conclusion: ConceptualTerminalResolutionCurrentTerminalDiffers,
			Bound:      true,
			BoundRow: ConceptualTerminalResolutionRow{
				EvidenceID: "ev-terminal",
				AllowedConclusions: []ConceptualTerminalResolutionConclusion{
					ConceptualTerminalResolutionCurrentTerminalDiffers,
				},
			},
		},
	}}})
	first := mutable.AnswerDocumentV2()
	first.Blocks[0].ConceptualTerminalResolution.BoundRow.AllowedConclusions[0] = ConceptualTerminalResolutionDestinationUnproven
	second := mutable.AnswerDocumentV2()
	if got := second.Blocks[0].ConceptualTerminalResolution.BoundRow.AllowedConclusions[0]; got != ConceptualTerminalResolutionCurrentTerminalDiffers {
		t.Fatalf("mutable-state clone aliased conceptual-terminal receipt: %s", got)
	}
}

func TestAnswerSemanticViewAppliesConceptualTerminalContractForAgentAndBus(t *testing.T) {
	ir := &AnalysisIR{RequestModel: RequestModel{
		Intent:        IntentTrace,
		Scenario:      ScenarioArchitectureExplain,
		PredicateAxis: AxisCall,
		AnalyzerHints: AnalyzerHints{Kind: string(ReqCallChain)},
		CallChainEndpointProfile: &CallChainEndpointProfile{
			Source: "VisitController.create", SinkMode: CallChainSinkResolutionDiscoverTerminal,
		},
	}}
	evidence := []EvidenceItem{{
		ID: "ev-terminal", Kind: EvidenceRelationship,
		Subject: "AuditLog.record", Predicate: "calls", Object: "System.out.println",
		Source: "src/AuditLog.java", LineStart: 6, LineEnd: 6,
		Producer: EvidenceProducerRepoMapTerminalBodyCall, Scope: ScopeLine, AnchorKind: AnchorCall,
	}}
	agentView := BuildAnswerSemanticViewForAgentContext(&AgentContext{
		AnalysisIR: ir, Mutable: NewMutableState("agent conceptual terminal"), EvidenceItems: evidence,
	})
	if agentView == nil || agentView.ConceptualTerminalResolutionContract == nil || len(agentView.ConceptualTerminalResolutionContract.Rows) != 1 {
		t.Fatalf("agent semantic view lost conceptual-terminal contract: %+v", agentView)
	}
	busView := BuildAnswerSemanticViewForBusContext(&BusContext{
		AnalysisIR: ir, Mutable: NewMutableState("bus conceptual terminal"), EvidenceItems: evidence,
	})
	if busView == nil || busView.ConceptualTerminalResolutionContract == nil || len(busView.ConceptualTerminalResolutionContract.Rows) != 1 {
		t.Fatalf("bus semantic view lost conceptual-terminal contract: %+v", busView)
	}
}
