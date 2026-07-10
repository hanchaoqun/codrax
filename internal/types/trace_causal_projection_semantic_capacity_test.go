package types

import (
	"fmt"
	"testing"
)

func traceProjectionSemanticCapacityRecord(id, subject, relevance string) ObservationRecord {
	causality := "background"
	depth := ""
	if relevance == "on_chain" {
		causality = "on_wakeup_chain"
		depth = "chain_depth=1"
	}
	notes := []string{
		"span_name=VerifyClass " + subject,
		"semantic_class=class_verification",
		"chain_relevance=" + relevance,
		"causality=" + causality,
	}
	if depth != "" {
		notes = append(notes, depth)
	}
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		Predicate:       "trace_semantic_span",
		ClaimKey:        "trace_semantic_span:class_verification:" + id,
		Subject:         subject,
		Object:          "class_verification",
		Value:           "1.000",
		Unit:            "ms",
		RichNotes:       notes,
		Confidence:      0.82,
	}
}

func TestTraceCausalProjectionSemanticCapacityKeepsAllOnChainBoundsOnlyOffChain(t *testing.T) {
	const onChainCount = 20
	const offChainCount = 20
	records := make([]ObservationRecord, 0, onChainCount+offChainCount)
	for i := 0; i < onChainCount; i++ {
		records = append(records, traceProjectionSemanticCapacityRecord(
			fmt.Sprintf("on-%02d", i), fmt.Sprintf("on-worker-%02d", i), "on_chain"))
	}
	for i := 0; i < offChainCount; i++ {
		records = append(records, traceProjectionSemanticCapacityRecord(
			fmt.Sprintf("off-%02d", i), fmt.Sprintf("off-worker-%02d", i), "background"))
	}

	got := CompileTraceCausalProjection(ObservationLedger{Records: records})
	if len(got.SemanticSpans) != onChainCount+traceCausalProjectionSemanticOffChainLimit {
		t.Fatalf("semantic projection must retain all %d on-chain + %d bounded off-chain nodes, got %d: %+v",
			onChainCount, traceCausalProjectionSemanticOffChainLimit, len(got.SemanticSpans), got.SemanticSpans)
	}
	for i := 0; i < onChainCount; i++ {
		want := fmt.Sprintf("on-%02d", i)
		if got.SemanticSpans[i].EvidenceID != want || !traceCausalProjectionNodeOnChain(got.SemanticSpans[i]) {
			t.Fatalf("on-chain semantic ordering/retention drift at %d: got %+v want evidence=%s", i, got.SemanticSpans[i], want)
		}
	}
	for i := 0; i < traceCausalProjectionSemanticOffChainLimit; i++ {
		at := onChainCount + i
		want := fmt.Sprintf("off-%02d", i)
		if got.SemanticSpans[at].EvidenceID != want || traceCausalProjectionNodeOnChain(got.SemanticSpans[at]) {
			t.Fatalf("off-chain semantic ordering/bound drift at %d: got %+v want evidence=%s", at, got.SemanticSpans[at], want)
		}
	}
	for _, node := range got.SemanticSpans {
		if node.EvidenceID == "off-16" || node.EvidenceID == "off-19" {
			t.Fatalf("off-chain semantic detail must stop at %d rows: %+v", traceCausalProjectionSemanticOffChainLimit, got.SemanticSpans)
		}
	}
	for _, node := range got.BackgroundCauses {
		if traceCausalProjectionNodeOnChain(node) {
			t.Fatalf("BackgroundCauses must contain only off-chain nodes: %+v", node)
		}
		if len(node.EvidenceID) >= 3 && node.EvidenceID[:3] == "on-" {
			t.Fatalf("on-chain semantic node leaked into background board: %+v", node)
		}
	}
	if len(got.BackgroundCauses) == 0 {
		t.Fatal("off-chain background control disappeared")
	}
}
