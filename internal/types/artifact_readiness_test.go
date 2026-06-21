package types

import "testing"

func TestRuntimeArtifactIDsAreSharedAndStable(t *testing.T) {
	ev := EvidenceItem{ID: " ev-1 ", Source: "internal/a.go", LineStart: 3}
	if got := RuntimeArtifactIDForEvidenceItem(ev); got != "ev-1" {
		t.Fatalf("evidence artifact id = %q, want ev-1", got)
	}
	chain := AnswerChain{Item: ev}
	if got := RuntimeArtifactIDForAnswerChain(chain); got != "answer_chain:ev-1" {
		t.Fatalf("answer-chain artifact id = %q", got)
	}
	fact := AnswerAggregateFact{Kind: AnswerAggregateMemberSet, Members: []string{"b", "a"}}
	identity := RuntimeArtifactIdentityForAggregateFact(fact)
	if identity == "" {
		t.Fatal("aggregate identity empty")
	}
	if got := RuntimeArtifactIDForAggregateFact(fact); got != RuntimeArtifactIDForAggregateFactIdentity(identity) {
		t.Fatalf("aggregate artifact id mismatch: %q vs identity-derived", got)
	}
}

func TestBuildArtifactReadinessViewResolvesLedgerRefs(t *testing.T) {
	ev := EvidenceItem{ID: "ev-1", Source: "a.go", LineStart: 1}
	chain := AnswerChain{Item: EvidenceItem{ID: "ev-chain", Source: "b.go", LineStart: 2}}
	fact := AnswerAggregateFact{Kind: AnswerAggregateMemberSet, Members: []string{"agent", "planner"}}
	records := []NodeArtifactRecord{
		{
			ProducerNodeID: "explore",
			Consumer:       RuntimeArtifactConsumerExtract,
			Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: RuntimeArtifactIDForEvidenceItem(ev)},
		},
		{
			ProducerNodeID: "explore",
			Consumer:       RuntimeArtifactConsumerExtract,
			Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactAnswerChain, ID: RuntimeArtifactIDForAnswerChain(chain)},
		},
		{
			ProducerNodeID: "explore",
			Consumer:       RuntimeArtifactConsumerExtract,
			Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactAggregateFact, ID: RuntimeArtifactIDForAggregateFact(fact)},
		},
	}
	view := BuildArtifactReadinessView(ArtifactReadinessInput{
		Contracts:      extractArtifactContractsFixture(),
		Ledger:         NodeArtifactLedger{Records: records},
		EvidenceItems:  []EvidenceItem{ev},
		AnswerChains:   []AnswerChain{chain},
		AggregateFacts: []AnswerAggregateFact{fact},
		Consumer:       RuntimeArtifactConsumerExtract,
	})
	if !view.Ready || view.ReasonCode != ArtifactReadinessReady {
		t.Fatalf("view should be ready: %+v", view)
	}
	if view.ResolvedRefCount != 3 || view.UnresolvedRefCount != 0 || view.UnsupportedRefCount != 0 {
		t.Fatalf("resolved counters = %+v", view)
	}
}

func TestBuildArtifactReadinessViewBlocksDanglingLedgerRef(t *testing.T) {
	view := BuildArtifactReadinessView(ArtifactReadinessInput{
		Contracts: extractArtifactContractsFixture(),
		Ledger: NodeArtifactLedger{Records: []NodeArtifactRecord{{
			ProducerNodeID: "explore",
			Consumer:       RuntimeArtifactConsumerExtract,
			Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: "ev-missing"},
		}}},
		EvidenceItems: []EvidenceItem{{ID: "ev-present"}},
		Consumer:      RuntimeArtifactConsumerExtract,
	})
	if view.Ready || !view.HasBlockingLedgerIssue() || view.ReasonCode != ArtifactReadinessPayloadMissing {
		t.Fatalf("dangling ref should block readiness: %+v", view)
	}
}

func TestBuildArtifactReadinessViewBlocksUnsupportedExtractLedgerKind(t *testing.T) {
	view := BuildArtifactReadinessView(ArtifactReadinessInput{
		Contracts: extractArtifactContractsFixture(),
		Ledger: NodeArtifactLedger{Records: []NodeArtifactRecord{{
			ProducerNodeID: "explore",
			Consumer:       RuntimeArtifactConsumerExtract,
			Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactSource, ID: "a.go"},
		}}},
		EvidenceItems: []EvidenceItem{{ID: "ev-present"}},
		Consumer:      RuntimeArtifactConsumerExtract,
	})
	if view.Ready || !view.HasBlockingLedgerIssue() || view.ReasonCode != ArtifactReadinessKindUnsupported {
		t.Fatalf("unsupported ref should block readiness: %+v", view)
	}
}

func TestBuildArtifactReadinessViewIgnoresConsumedRecords(t *testing.T) {
	view := BuildArtifactReadinessView(ArtifactReadinessInput{
		Contracts: extractArtifactContractsFixture(),
		Ledger: NodeArtifactLedger{Records: []NodeArtifactRecord{{
			Direction:      RuntimeArtifactConsumed,
			ProducerNodeID: "explore",
			ConsumerNodeID: "extract",
			Consumer:       RuntimeArtifactConsumerExtract,
			Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: "ev-consumed"},
		}}},
		EvidenceItems: []EvidenceItem{{ID: "ev-consumed"}},
		Consumer:      RuntimeArtifactConsumerExtract,
	})
	if !view.Ready || view.ResolvedRefCount != 0 || view.HasBlockingLedgerIssue() {
		t.Fatalf("view = %+v, want payload fallback ready without consumed refs", view)
	}
}

func TestConsumedNodeArtifactRecordsFromReadiness(t *testing.T) {
	view := BuildArtifactReadinessView(ArtifactReadinessInput{
		Contracts: extractArtifactContractsFixture(),
		Ledger: NodeArtifactLedger{Records: []NodeArtifactRecord{{
			ProducerNodeID: "explore",
			Consumer:       RuntimeArtifactConsumerExtract,
			Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: "ev-ready"},
		}}},
		EvidenceItems: []EvidenceItem{{ID: "ev-ready"}},
		Consumer:      RuntimeArtifactConsumerExtract,
	})
	records := ConsumedNodeArtifactRecordsFromReadiness("extract", view)
	if len(records) != 1 {
		t.Fatalf("consumed records = %+v, want 1", records)
	}
	got := records[0]
	if got.Direction != RuntimeArtifactConsumed ||
		got.ProducerNodeID != "explore" ||
		got.ConsumerNodeID != "extract" ||
		got.ReasonCode != RuntimeArtifactReasonReadinessConsumed ||
		got.Artifact.ID != "ev-ready" {
		t.Fatalf("consumed record = %+v", got)
	}
}

func TestBuildArtifactReadinessViewUsesTypedPayloadFallbackWithoutLedger(t *testing.T) {
	view := BuildArtifactReadinessView(ArtifactReadinessInput{
		Contracts:     extractArtifactContractsFixture(),
		EvidenceItems: []EvidenceItem{{ID: "ev-present"}},
		Consumer:      RuntimeArtifactConsumerExtract,
	})
	if !view.Ready || view.HasBlockingLedgerIssue() || view.ReasonCode != ArtifactReadinessReady {
		t.Fatalf("typed payload fallback should be ready: %+v", view)
	}
}

func TestBuildArtifactReadinessViewReportsMissingContractInputsWithoutBlockingFallbacks(t *testing.T) {
	view := BuildArtifactReadinessView(ArtifactReadinessInput{
		Contracts: extractArtifactContractsFixture(),
		Consumer:  RuntimeArtifactConsumerExtract,
	})
	if view.Ready || view.HasBlockingLedgerIssue() || view.ReasonCode != ArtifactReadinessRequiredInputMissing {
		t.Fatalf("empty typed payload should report contract miss without ledger blocker: %+v", view)
	}
	if len(view.MissingRequiredInputs) != 3 {
		t.Fatalf("missing required inputs = %+v", view.MissingRequiredInputs)
	}
}

func extractArtifactContractsFixture() []TaskArtifactContract {
	return []TaskArtifactContract{{
		NodeID: "extract",
		Inputs: []string{
			taskArtifactInputEvidenceItems,
			taskArtifactInputAnswerChains,
			taskArtifactInputAggregateFacts,
		},
	}}
}
