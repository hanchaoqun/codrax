package tracequery

import "testing"

func TestHMC376VocabularySpanWithADurationSeatDoesNotYield(t *testing.T) {
	const wait = "GC: Wait For Completion Alloc"
	spans := []TraceSpanSummary{
		{Name: wait, StartLine: 1, DurationMs: 90}, // first by duration: published without the vocabulary too
		{Name: "worker a", StartLine: 2, DurationMs: 80},
		{Name: "worker b", StartLine: 3, DurationMs: 70},
		{Name: wait, StartLine: 4, DurationMs: 60}, // past the duration bound
	}
	bounded, _ := boundTraceMarkSpansForTarget(spans, 3, semanticSeatQuery{})
	seatedOnly := map[int]bool{}
	for _, span := range bounded {
		seatedOnly[span.StartLine] = span.seatedByVocabularyOnly
	}
	if seatedOnly[1] || !seatedOnly[4] {
		t.Fatalf("seated by vocabulary only: line 1=%v line 4=%v, want false and true", seatedOnly[1], seatedOnly[4])
	}
}

func TestHMC376BlockingListKeepsRowsPublishedWithoutTheVocabulary(t *testing.T) {
	items := []CriticalBlockingCandidate{
		{Type: "vocabulary-off-chain", seatedByVocabularyOnly: true, ChainRelevance: "background"},
		{Type: "vocabulary-on-chain", seatedByVocabularyOnly: true, ChainRelevance: "on_chain"},
		{Type: "plain-background", ChainRelevance: "background"},
		{Type: "plain-on-chain", ChainRelevance: "on_chain"},
	}
	got := criticalBlockingVocabularyRowsLast(items)
	want := []string{"vocabulary-on-chain", "plain-background", "plain-on-chain", "vocabulary-off-chain"}
	for i, typ := range want {
		if got[i].Type != typ {
			t.Fatalf("seat %d=%q, want %q (all: %+v)", i, got[i].Type, typ, got)
		}
	}
}

func TestHMC376FoldedBlockingRowYieldsOnlyWhenEveryMemberDoes(t *testing.T) {
	row := func(yields bool) blockingSpanRow {
		return blockingSpanRow{cand: CriticalBlockingCandidate{seatedByVocabularyOnly: yields}}
	}
	if foldLockContentionRow(row(true), row(false)).cand.seatedByVocabularyOnly {
		t.Fatal("a row that carries one published member must not yield")
	}
	if !foldLockContentionRow(row(true), row(true)).cand.seatedByVocabularyOnly {
		t.Fatal("a row made only of vocabulary seats yields")
	}
}
