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

func hmc376SeatSpan(comm string, pid, line int, name string, ms float64) TraceSpanSummary {
	return TraceSpanSummary{Name: name, Thread: ThreadRef{Comm: comm, PID: pid}, StartLine: line, DurationMs: ms}
}

func hmc376SeatedLines(spans []TraceSpanSummary) map[int]bool {
	lines := map[int]bool{}
	for _, span := range spans {
		lines[span.StartLine] = true
	}
	return lines
}

// One family filling the duration bound keeps every seat it had; the vocabulary
// only adds a seat for the span past the bound.
func TestHMC376DurationSeatedPlatformSpansAllKeepTheirSeats(t *testing.T) {
	const wait = "GC: Wait For Completion Alloc"
	var spans []TraceSpanSummary
	for i := 0; i < 8; i++ {
		spans = append(spans, hmc376SeatSpan("gcwaiter", 201, i+1, wait, float64(100-i)))
	}
	spans = append(spans, hmc376SeatSpan("otherwaiter", 202, 9, wait, 50))
	bounded, _ := boundTraceMarkSpansForTarget(spans, 8, semanticSeatQuery{})
	seated := hmc376SeatedLines(bounded)
	for line := 1; line <= 9; line++ {
		if !seated[line] {
			t.Fatalf("line %d lost its seat: %+v", line, bounded)
		}
	}
}

// Moving platform spans out of the generic pool frees no duration seat.
func TestHMC376PlatformSpansFreeNoDurationSeatForGenericSpans(t *testing.T) {
	const wait = "GC: Wait For Completion Alloc"
	var spans []TraceSpanSummary
	for i := 0; i < 3; i++ {
		spans = append(spans, hmc376SeatSpan("gcwaiter", 201+i, i+1, wait, float64(500-i)))
	}
	for i := 0; i < 8; i++ {
		spans = append(spans, hmc376SeatSpan("worker", 301+i, 11+i, "worker lock wait", float64(300-i)))
	}
	bounded, _ := boundTraceMarkSpansForTarget(spans, 8, semanticSeatQuery{})
	if len(bounded) != 8 {
		t.Fatalf("the duration bound seats 8 spans, got %d: %+v", len(bounded), bounded)
	}
	seated := hmc376SeatedLines(bounded)
	for _, line := range []int{16, 17, 18} {
		if seated[line] {
			t.Fatalf("generic span at line %d is past the duration bound: %+v", line, bounded)
		}
	}
}

// A name an operator pattern classifies keeps the classified pool it had before
// the vocabulary named it, and never yields as a vocabulary-only row.
func TestHMC376OperatorClassifiedVocabularySpanKeepsItsPool(t *testing.T) {
	const wait = "GC: Wait For Completion Alloc"
	SetSemanticSpanPatterns([]SemanticSpanPattern{{SemanticClass: "gc_pause", Contains: []string{"wait for completion"}}})
	t.Cleanup(func() { SetSemanticSpanPatterns(nil) })
	var spans []TraceSpanSummary
	for i := 0; i < 8; i++ {
		spans = append(spans, hmc376SeatSpan("worker", 301+i, i+1, "worker lock wait", float64(300-i)))
	}
	for i := 0; i < 12; i++ {
		spans = append(spans, hmc376SeatSpan("gcwaiter", 201+i, 11+i, wait, float64(100-i)))
	}
	bounded, _ := boundTraceMarkSpansForTarget(spans, 8, semanticSeatQuery{})
	if len(bounded) != 20 {
		t.Fatalf("all 20 spans had a seat, got %d", len(bounded))
	}
	for _, span := range bounded {
		if span.seatedByVocabularyOnly {
			t.Fatalf("an operator-classified span must not yield: %+v", span)
		}
	}
}

func TestHMC376RunningWorkTotalLeavesOutRecordedWaits(t *testing.T) {
	target := ThreadRef{Comm: "main", PID: 100}
	stats := &WindowStats{TraceSpans: []TraceSpanSummary{
		{Name: "GC: Wait For Completion Alloc", Thread: target, StartTs: 10.000, EndTs: 10.010, DurationMs: 10},
		{Name: "JIT compiling void a.b()", Thread: target, StartTs: 10.020, EndTs: 10.022, DurationMs: 2},
	}}
	running := []foldInterval{{start: 10.000, end: 10.001}, {start: 10.020, end: 10.021}}
	got := targetSemanticRunningMs(stats, target, TimeWindow{StartTs: 10, EndTs: 11}, running)
	if got < 0.999 || got > 1.001 {
		t.Fatalf("running work=%.3fms, want the 1.000ms of the compilation only", got)
	}
}

func TestHMC376RecordedWaitAndExecutedWorkFormSeparateFamilies(t *testing.T) {
	thread := ThreadRef{Comm: "main", PID: 100}
	families := FoldSemanticSpanFamilies(nil, []TraceSpanSummary{
		{Name: "SuspendTime::WaitGCFinished", Thread: thread, StartTs: 10.000, EndTs: 10.010, DurationMs: 10},
		{Name: "PartialGC::RunPhases", Thread: thread, StartTs: 10.020, EndTs: 10.022, DurationMs: 2},
	})
	if len(families) != 2 {
		t.Fatalf("a wait and executed work must not share a family, got %d: %+v", len(families), families)
	}
}
