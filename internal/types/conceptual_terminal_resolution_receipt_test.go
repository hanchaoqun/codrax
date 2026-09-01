package types

import "testing"

func TestConceptualTerminalResolutionContractUsesCanonicalParserGroundedOperationRows(t *testing.T) {
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
	selectedCallable := good
	selectedCallable.ID = "ev-selected-callable"
	selectedCallable.Subject = "Repository.flush"
	selectedCallable.Object = "storage.commit"
	selectedCallable.Source = "src/Repository.java"
	selectedCallable.LineStart = 12
	selectedCallable.LineEnd = 12
	selectedCallable.Producer = EvidenceProducerRepoMapSelectedCallableBodyCall
	contract := BuildConceptualTerminalResolutionContract(profile, []EvidenceItem{noise, good, good, duplicateCoordinate, selectedCallable})
	if contract == nil || len(contract.Rows) != 2 {
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
	if selected := contract.Rows[1]; selected.EvidenceID != "ev-selected-callable" ||
		selected.TerminalCallable != "Repository.flush" || selected.ExactOperation != "storage.commit" {
		t.Fatalf("selected-callable exact body operation was not admitted: %+v", selected)
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
	selectedReceipt := &AnswerConceptualTerminalResolutionReceipt{
		EvidenceID: "ev-selected-callable",
		Conclusion: ConceptualTerminalResolutionDestinationUnproven,
	}
	if !BindConceptualTerminalResolutionReceipt(selectedReceipt, contract) ||
		selectedReceipt.BoundRow.ExactOperation != "storage.commit" {
		t.Fatalf("selected-callable model selection did not bind: %+v", selectedReceipt)
	}
}

func TestConceptualTerminalOperationPredicateKeepsBehaviorCandidatesOutOfTopologyAuthority(t *testing.T) {
	for _, producer := range []string{
		EvidenceProducerRepoMapTerminalBodyCall,
		EvidenceProducerRepoMapSelectedCallableBodyCall,
	} {
		item := EvidenceItem{
			ID: "ev", Kind: EvidenceRelationship, Subject: "Leaf.run", Predicate: "calls", Object: "sink.write",
			Source: "src/Leaf.java", LineStart: 8, Scope: ScopeLine, AnchorKind: AnchorCall, Producer: producer,
		}
		if !IsConceptualTerminalOperationEvidence(item) || !IsCallChainBodyEnrichmentEvidence(item) {
			t.Fatalf("parser body candidate lost shared classification: %+v", item)
		}
	}
	principal := EvidenceItem{
		ID: "principal", Kind: EvidenceRelationship, Subject: "Root.run", Predicate: "calls", Object: "Leaf.run",
		Source: "src/Root.java", LineStart: 4, Scope: ScopeLine, AnchorKind: AnchorCall,
	}
	if IsConceptualTerminalOperationEvidence(principal) || IsCallChainBodyEnrichmentEvidence(principal) {
		t.Fatal("principal call edge must not be relabeled as body enrichment")
	}
}

func TestConceptualTerminalResolutionAdmitsLatePrincipalLeafIncomingEdge(t *testing.T) {
	profile := &CallChainEndpointProfile{
		Source:   "VisitController.create",
		SinkMode: CallChainSinkResolutionDiscoverTerminal,
	}
	evidence := []EvidenceItem{
		{ID: "e1", Kind: EvidenceRelationship, Subject: "VisitController.create", Predicate: "calls", Object: "VisitService.schedule", Source: "src/Controller.java", LineStart: 18, Scope: ScopeLine, AnchorKind: AnchorCall},
		{ID: "e2", Kind: EvidenceRelationship, Subject: "VisitService.schedule", Predicate: "calls", Object: "VisitRepository.insert", Source: "src/Service.java", LineStart: 21, Scope: ScopeLine, AnchorKind: AnchorCall},
		{ID: "e3", Kind: EvidenceRelationship, Subject: "VisitRepository.insert", Predicate: "calls", Object: "AuditLog.record", Source: "src/Repository.java", LineStart: 23, Scope: ScopeLine, AnchorKind: AnchorCall},
		// This exact grounded edge arrived after the parser enrichment pass.
		{ID: "late-terminal", Kind: EvidenceRelationship, Subject: "AuditLog.record", Predicate: "calls", Object: "System.out.println", Source: "src/AuditLog.java", LineStart: 6, Scope: ScopeLine, AnchorKind: AnchorCall, GroundingStatus: GroundingGrounded},
	}
	contract := BuildConceptualTerminalResolutionContract(profile, evidence)
	if contract == nil {
		t.Fatal("conceptual terminal contract missing")
	}
	found := false
	for _, row := range contract.Rows {
		if row.EvidenceID == "late-terminal" {
			found = row.TerminalCallable == "AuditLog.record" && row.ExactOperation == "System.out.println"
		}
		if row.EvidenceID == "e3" {
			t.Fatal("non-terminal incoming edge must not enter the exact operation candidates")
		}
	}
	if !found {
		t.Fatalf("late exact terminal incoming operation was not published: %+v", contract.Rows)
	}
	receipt := &AnswerConceptualTerminalResolutionReceipt{
		EvidenceID: "late-terminal",
		Conclusion: ConceptualTerminalResolutionCurrentTerminalDiffers,
	}
	if !BindConceptualTerminalResolutionReceipt(receipt, contract) || !receipt.IsBound() {
		t.Fatalf("model-selected late terminal pair did not bind: %+v", receipt)
	}
}

func TestConceptualTerminalResolutionLeafCalculationIgnoresBodyEnrichment(t *testing.T) {
	evidence := []EvidenceItem{
		{ID: "principal", Kind: EvidenceRelationship, Subject: "Root.run", Predicate: "calls", Object: "Leaf.run", Source: "src/Root.java", LineStart: 4, Scope: ScopeLine, AnchorKind: AnchorCall},
		{ID: "body", Kind: EvidenceRelationship, Subject: "Leaf.run", Predicate: "calls", Object: "sink.write", Source: "src/Leaf.java", LineStart: 8, Scope: ScopeLine, AnchorKind: AnchorCall, Producer: EvidenceProducerRepoMapTerminalBodyCall},
	}
	rows := BuildConceptualTerminalResolutionRows(evidence)
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.EvidenceID] = true
	}
	if !seen["principal"] || !seen["body"] {
		t.Fatalf("principal leaf operation and its body behavior must remain separate candidates: %+v", rows)
	}
}

func TestConceptualTerminalResolutionBoundIsFairAcrossCallableGroups(t *testing.T) {
	var evidence []EvidenceItem
	for i := 1; i <= 20; i++ {
		evidence = append(evidence, EvidenceItem{
			ID: "utility-" + string(rune('a'+i-1)), Kind: EvidenceRelationship,
			Subject: "UtilityHeavy.run", Predicate: "calls", Object: "helper." + string(rune('a'+i-1)),
			Source: "src/A.java", LineStart: i, Scope: ScopeLine, AnchorKind: AnchorCall,
			Producer: EvidenceProducerRepoMapTerminalBodyCall,
		})
	}
	evidence = append(evidence, EvidenceItem{
		ID: "other-leaf", Kind: EvidenceRelationship, Subject: "OtherLeaf.run", Predicate: "calls", Object: "sink.write",
		Source: "src/Z.java", LineStart: 2, Scope: ScopeLine, AnchorKind: AnchorCall,
		Producer: EvidenceProducerRepoMapSelectedCallableBodyCall,
	})
	rows := BuildConceptualTerminalResolutionRows(evidence)
	if len(rows) != 16 {
		t.Fatalf("bounded row count=%d, want 16", len(rows))
	}
	found := false
	for _, row := range rows {
		found = found || row.EvidenceID == "other-leaf"
	}
	if !found {
		t.Fatalf("utility-heavy callable monopolized bounded candidate schema: %+v", rows)
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
