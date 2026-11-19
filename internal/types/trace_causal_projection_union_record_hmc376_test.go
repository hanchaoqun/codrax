package types

import (
	"fmt"
	"math"
	"reflect"
	"testing"
)

func hmc376WaitRow(id string, start, end, impact, windowStart, windowEnd float64, lineStart, lineEnd int) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Subject: "ease.cloudmusic-11077", Object: "gc_pause", EvidenceID: id,
		StartTs: start, EndTs: end, ImpactMS: impact, CumulativeImpactMS: impact,
		QueryWindowStartTs: windowStart, QueryWindowEndTs: windowEnd,
		LineStart: lineStart, LineEnd: lineEnd,
	}
}

// A wide query publishes the rank seat of a wait and the span that records it; a
// narrower drill-down reads the same wait again. The merged row is that one wait.
func TestHMC376OneRecordedWaitAcrossTwoQueriesCountsOnce(t *testing.T) {
	got := traceCausalProjectionAggregateSameKind([]TraceCausalProjectionNode{
		hmc376WaitRow("seat", 153.764012, 153.887015, 123.003, 153.763595, 153.887340, 572361, 573565),
		hmc376WaitRow("span", 153.763948, 153.887037, 123.089, 153.763595, 153.887340, 572361, 573565),
		hmc376WaitRow("narrow", 153.800000, 153.880000, 80.000, 153.800000, 153.880000, 572361, 573565),
	})
	if len(got) != 1 || got[0].MergedCount != 3 {
		t.Fatalf("want one merged row of three, got %+v", got)
	}
	if math.Abs(got[0].ImpactMS-123.089) > 0.001 {
		t.Fatalf("merged wait=%.3fms, want the recorded 123.089ms", got[0].ImpactMS)
	}
}

// Two rows of one window that cite different lines stay two facts.
func TestHMC376RowsCitingDifferentLinesStayDistinctFacts(t *testing.T) {
	got := traceCausalProjectionAggregateSameKind([]TraceCausalProjectionNode{
		hmc376WaitRow("first", 153.764012, 153.887015, 123.003, 153.763595, 153.887340, 572361, 573565),
		hmc376WaitRow("second", 153.763948, 153.887037, 123.089, 153.763595, 153.887340, 572400, 573600),
		hmc376WaitRow("narrow", 153.800000, 153.880000, 80.000, 153.800000, 153.880000, 572361, 573565),
	})
	if len(got) != 1 || math.Abs(got[0].ImpactMS-246.092) > 0.001 {
		t.Fatalf("distinct facts of one window still add up, got %+v", got)
	}
}

// A phase read without a query window (a line-anchored query) is still inside the
// collection the time-window queries read.
func TestHMC376PhaseWithoutAWindowAddsNothingToItsCollection(t *testing.T) {
	row := func(start, end, windowStart, windowEnd float64) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			Subject: "HeapTaskDaemon-11098", Object: "trace_span", StartTs: start, EndTs: end,
			ImpactMS: (end - start) * 1000, QueryWindowStartTs: windowStart, QueryWindowEndTs: windowEnd,
		}
	}
	got := traceCausalProjectionAggregateSameKind([]TraceCausalProjectionNode{
		row(10.00, 10.20, 10.00, 11.00),
		row(10.05, 10.20, 10.05, 11.00),
		row(10.01, 10.19, 0, 0),
	})
	if len(got) != 1 || math.Abs(got[0].ImpactMS-200) > 0.001 {
		t.Fatalf("the collection lasted 200ms, got %+v", got)
	}
}

// Rows of different roles compare as ties, so a yielding row sorted among the
// others could reorder them. The other rows keep the order they have without it.
func TestHMC376YieldingRowsLeaveTheOtherRowsInOrder(t *testing.T) {
	traceSemanticSeatYieldForTest(t)
	row := func(subject, span, role string, impact float64) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			Subject: subject, SpanName: span, Role: role, ChainRelevance: "adjacent",
			ImpactMS: impact, CumulativeImpactMS: impact,
		}
	}
	// Smallest first, so the sort has to move rows past the yielding rows between them.
	others := []TraceCausalProjectionNode{
		row("ver2", "VerifyClass c", "semantic_span", 30),
		row("ver1", "VerifyClass b", "semantic_span", 36.5),
		row("ver0", "VerifyClass a", "semantic_span", 40),
	}
	sorted := func(nodes []TraceCausalProjectionNode) []string {
		return traceSemanticSeatSubjects(traceCausalProjectionSortContextBucket(append([]TraceCausalProjectionNode(nil), nodes...), nil))
	}
	want := sorted(others)
	if !reflect.DeepEqual(want, []string{"ver0", "ver1", "ver2"}) {
		t.Fatalf("control: the rows sort by value without a yielding row, got %v", want)
	}
	for count := 1; count <= 2; count++ {
		var nodes []TraceCausalProjectionNode
		var waits []string
		for i, other := range others {
			nodes = append(nodes, other)
			if i < count {
				subject := fmt.Sprintf("wait%d", i)
				waits = append(waits, subject)
				nodes = append(nodes, row(subject, "vocabulary wait", "root_cause_context", 189.99-float64(i)))
			}
		}
		got := sorted(nodes)
		if !reflect.DeepEqual(got[:len(want)], want) {
			t.Fatalf("%d yielding rows reordered the others: %v, want %v first", count, got, want)
		}
		if !reflect.DeepEqual(got[len(want):], waits) {
			t.Fatalf("%d yielding rows must follow in value order: %v", count, got)
		}
	}
}
