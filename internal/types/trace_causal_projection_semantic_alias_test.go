package types

import (
	"strconv"
	"strings"
	"testing"
)

func semanticAliasRecord(id, ref string, lineStart int, start, end, queryStart, queryEnd float64) ObservationRecord {
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		Predicate:       "trace_semantic_span",
		ClaimKey:        "trace_semantic_span:class_verification:VerifyClass com.example.Foo",
		Subject:         "worker-7",
		Object:          "class_verification",
		Value:           "0.285",
		Unit:            "ms",
		SupportRefs:     []string{ref},
		Span: ObservationSpan{
			LineStart: lineStart,
			LineEnd:   lineStart + 20,
			StartTs:   start,
			EndTs:     end,
		},
		RichNotes: []string{
			"span_name=VerifyClass com.example.Foo",
			"span_kind=sync",
			"span_category=class_verification",
			"semantic_class=class_verification",
			"chain_relevance=on_chain",
			"causality=on_wakeup_chain",
			"chain_depth=1",
			"selected_window=" + traceCausalProjectionTestWindow(queryStart, queryEnd),
		},
		Confidence: 0.82,
	}
}

func traceCausalProjectionTestWindow(start, end float64) string {
	return formatFloat6(start) + ".." + formatFloat6(end)
}

func formatFloat6(value float64) string {
	return strconv.FormatFloat(value, 'f', 6, 64)
}

func TestTraceCausalProjectionDedupesExactSemanticSpanAcrossAttachmentAliases(t *testing.T) {
	a := semanticAliasRecord("E24", "attached_trace.txt:5553-5573", 5553,
		34579.495841, 34579.496126, 34579.472865, 34579.587805)
	b := semanticAliasRecord("E25", "donghu_tieba_frame.systrace:5552-5572", 5552,
		34579.495841, 34579.496126, 34579.472865, 34579.587805)
	b.SystemSupplement = true

	projection := CompileTraceCausalProjection(ObservationLedger{Records: []ObservationRecord{a, b}})
	if len(projection.SemanticSpans) != 1 || len(projection.OnChainCauses) != 1 {
		t.Fatalf("one physical semantic span published through two aliases must occupy one display/chain seat: semantic=%+v on_chain=%+v",
			projection.SemanticSpans, projection.OnChainCauses)
	}
	for _, node := range []TraceCausalProjectionNode{projection.SemanticSpans[0], projection.OnChainCauses[0]} {
		if node.EvidenceID != "E24" || len(node.MergedEvidenceIDs) != 1 || node.MergedEvidenceIDs[0] != "E25" {
			t.Fatalf("alias fold must retain both evidence identities without minting an occurrence: %+v", node)
		}
		if len(node.SupportRefs) != 2 {
			t.Fatalf("alias fold must retain both physical locators: %+v", node.SupportRefs)
		}
	}
}

func TestTraceCausalProjectionSemanticAliasDedupePreservesDistinctIntervalsAndQueryDomains(t *testing.T) {
	base := semanticAliasRecord("base", "trace.systrace:10-20", 10,
		10.100000, 10.100285, 10.000000, 10.200000)
	tests := []struct {
		name string
		peer ObservationRecord
	}{
		{
			name: "one_microsecond_distinct_occurrence",
			peer: semanticAliasRecord("interval", "trace.systrace:30-40", 30,
				10.100001, 10.100286, 10.000000, 10.200000),
		},
		{
			name: "distinct_query_domain",
			peer: semanticAliasRecord("query", "trace.systrace:10-20", 10,
				10.100000, 10.100285, 10.050000, 10.250000),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projection := CompileTraceCausalProjection(ObservationLedger{Records: []ObservationRecord{base, tc.peer}})
			if len(projection.SemanticSpans) != 2 || len(projection.OnChainCauses) != 2 {
				t.Fatalf("distinct occurrence intervals/query domains must never be alias-folded: semantic=%+v on_chain=%+v",
					projection.SemanticSpans, projection.OnChainCauses)
			}
		})
	}
}

func TestTraceCausalProjectionExactSemanticOccurrenceUsesSingleTypedChainSeatAcrossQueryViews(t *testing.T) {
	chain := semanticAliasRecord("chain", "trace.systrace:10-20", 10,
		10.100000, 10.100285, 10.000000, 10.200000)
	background := semanticAliasRecord("background", "trace.systrace:10-20", 10,
		10.100000, 10.100285, 10.050000, 10.250000)
	for i, note := range background.RichNotes {
		switch {
		case strings.HasPrefix(note, "chain_relevance="):
			background.RichNotes[i] = "chain_relevance=background"
		case strings.HasPrefix(note, "causality="):
			background.RichNotes[i] = "causality=background"
		}
	}

	projection := CompileTraceCausalProjection(ObservationLedger{Records: []ObservationRecord{background, chain}})
	if len(projection.OnChainCauses) != 1 || len(projection.BackgroundCauses) != 0 {
		t.Fatalf("one exact physical occurrence with one typed chain authority must occupy only the chain seat: on=%+v background=%+v",
			projection.OnChainCauses, projection.BackgroundCauses)
	}
	if len(projection.SemanticSpans) != 1 || projection.SemanticSpans[0].ChainRelevance != "on_chain" {
		t.Fatalf("semantic display copy must inherit the same chain-authoritative seat: %+v", projection.SemanticSpans)
	}
	for _, node := range []TraceCausalProjectionNode{projection.OnChainCauses[0], projection.SemanticSpans[0]} {
		if node.EvidenceID != "chain" || len(node.MergedEvidenceIDs) != 1 || node.MergedEvidenceIDs[0] != "background" {
			t.Fatalf("chain seat must retain the weaker query-view evidence without letting it mint another lane: %+v", node)
		}
	}
}

func TestTraceCausalProjectionSemanticPhysicalSeatDoesNotAdjudicateTwoChainDomains(t *testing.T) {
	a := semanticAliasRecord("chain-a", "trace.systrace:10-20", 10,
		10.100000, 10.100285, 10.000000, 10.200000)
	b := semanticAliasRecord("chain-b", "trace.systrace:10-20", 10,
		10.100000, 10.100285, 10.050000, 10.250000)

	projection := CompileTraceCausalProjection(ObservationLedger{Records: []ObservationRecord{a, b}})
	if len(projection.OnChainCauses) != 2 || len(projection.SemanticSpans) != 2 {
		t.Fatalf("two independently typed chain domains require upstream adjudication and must remain lossless: on=%+v semantic=%+v",
			projection.OnChainCauses, projection.SemanticSpans)
	}
}

func TestTraceCausalProjectionDuplicatePublicationNeverNearFoldsSemanticSpans(t *testing.T) {
	a := TraceCausalProjectionNode{
		Role:      TraceCausalRoleSemanticSpan,
		Subject:   "worker-7",
		Predicate: "trace_semantic_span",
		Object:    "class_verification",
		ImpactMS:  0.285,
		StartTs:   10.100000,
		EndTs:     10.100285,
	}
	b := a
	b.EvidenceID = "E2"
	b.StartTs += 0.000001
	b.EndTs += 0.000001
	if traceCausalProjectionSameDuplicatePublication(a, b) {
		t.Fatal("overlapping semantic spans with distinct occurrence intervals must not enter V4 near-publication fold")
	}
}
