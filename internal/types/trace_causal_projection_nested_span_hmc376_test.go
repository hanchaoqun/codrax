package types

import "testing"

// A collection and the phases recorded inside it are one stretch of time.
func TestHMC376NestedSpansOfOneThreadAreNotRepeatedOccurrences(t *testing.T) {
	span := func(start, end, impact float64) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			Subject: "HeapTaskDaemon-11098", Object: "trace_span", StartTs: start, EndTs: end, ImpactMS: impact,
			QueryWindowStartTs: 153.763595, QueryWindowEndTs: 153.887340,
		}
	}
	impacts := func(nodes []TraceCausalProjectionNode) (sum float64) {
		for _, node := range nodes {
			sum += node.ImpactMS
		}
		return sum
	}
	nested := traceCausalProjectionAggregateSameKind([]TraceCausalProjectionNode{
		span(153.300484, 153.886909, 123.314), span(153.540347, 153.820840, 57.245), span(153.820842, 153.886224, 65.382),
	})
	if len(nested) != 3 {
		t.Fatalf("a collection and its phases must stay separate rows, got %d: %+v", len(nested), nested)
	}
	for _, node := range nested {
		if node.MergedCount != 0 || node.ImpactMS > 123.745 {
			t.Fatalf("no row may add a phase to its collection: %+v", node)
		}
	}
	disjoint := traceCausalProjectionAggregateSameKind([]TraceCausalProjectionNode{
		span(153.80, 153.81, 10), span(153.82, 153.83, 10), span(153.84, 153.85, 10),
	})
	if len(disjoint) != 1 || disjoint[0].MergedCount != 3 || impacts(disjoint) != 30 {
		t.Fatalf("repeated occurrences still merge and add up: %+v", disjoint)
	}
}

func TestHMC376SpansThatDoNotNestStillMerge(t *testing.T) {
	span := func(start, end, windowStart float64) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			Subject: "HeapTaskDaemon-11098", Object: "trace_span", StartTs: start, EndTs: end,
			ImpactMS: (end - start) * 1000, QueryWindowStartTs: windowStart, QueryWindowEndTs: 154,
		}
	}
	for name, rows := range map[string][]TraceCausalProjectionNode{
		"overlap-across-query-windows": {span(153.30, 153.50, 153.1), span(153.40, 153.60, 153.2), span(153.55, 153.75, 153.3)},
		"copies-of-one-interval":       {span(153.54, 153.82, 153.1), span(153.54, 153.82, 153.1), span(153.54, 153.82, 153.1)},
		"touching-intervals":           {span(153.10, 153.20, 153.1), span(153.20, 153.30, 153.1), span(153.30, 153.40, 153.1)},
	} {
		t.Run(name, func(t *testing.T) {
			got := traceCausalProjectionAggregateSameKind(rows)
			if len(got) != 1 || got[0].MergedCount != 3 {
				t.Fatalf("want one merged row of three, got %d rows: %+v", len(got), got)
			}
		})
	}
}

func hmc376NestedRow(start, end, impact, windowStart, windowEnd float64) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Subject: "HeapTaskDaemon-11098", Object: "trace_span", StartTs: start, EndTs: end, ImpactMS: impact,
		QueryWindowStartTs: windowStart, QueryWindowEndTs: windowEnd,
	}
}

// A second query over the same collection joins the union, and the phase inside
// the collection adds no time to it.
func TestHMC376NestedSpansAcrossQueryWindowsKeepOneUnion(t *testing.T) {
	got := traceCausalProjectionAggregateSameKind([]TraceCausalProjectionNode{
		hmc376NestedRow(11.00, 11.05, 50, 10, 20),
		hmc376NestedRow(11.01, 11.02, 10, 10, 20),
		hmc376NestedRow(11.00, 11.05, 50, 10.5, 20),
	})
	if len(got) != 1 || got[0].MergedCount != 3 {
		t.Fatalf("want one merged row of three, got %d rows: %+v", len(got), got)
	}
	if value := traceCausalProjectionDisplayValue(got[0]); value < 49.999 || value > 50.001 {
		t.Fatalf("merged value=%.3fms, want the 50.000ms collection", value)
	}
}

// Only the rows that nest stay out of the repeated family.
func TestHMC376DisjointOccurrencesMergeBesideANestedPhase(t *testing.T) {
	rows := []TraceCausalProjectionNode{hmc376NestedRow(10.00, 10.10, 100, 9, 30), hmc376NestedRow(10.02, 10.08, 60, 9, 30)}
	for i := 1; i <= 11; i++ {
		start := 10 + float64(i)
		rows = append(rows, hmc376NestedRow(start, start+0.10, 100, 9, 30))
	}
	got := traceCausalProjectionAggregateSameKind(rows)
	if len(got) != 3 {
		t.Fatalf("want collection, phase and one repeated family, got %d rows: %+v", len(got), got)
	}
	merged := 0
	for _, row := range got {
		if row.MergedCount == 11 {
			merged++
		}
	}
	if merged != 1 {
		t.Fatalf("the eleven disjoint occurrences must form one family: %+v", got)
	}
}

// Two lookups a fraction of a millisecond apart read one window, and a row
// without a window cannot prove it read a different one.
func TestHMC376NestedSpansStaySeparateAcrossNearAndMissingWindows(t *testing.T) {
	for name, phaseWindowStart := range map[string]float64{"near-window": 10.0004, "missing-window": 0} {
		t.Run(name, func(t *testing.T) {
			phaseWindowEnd := 20.0
			if phaseWindowStart == 0 {
				phaseWindowEnd = 0
			}
			got := traceCausalProjectionAggregateSameKind([]TraceCausalProjectionNode{
				hmc376NestedRow(11.00, 11.10, 100, 10, 20),
				hmc376NestedRow(11.01, 11.05, 40, phaseWindowStart, phaseWindowEnd),
				hmc376NestedRow(11.06, 11.09, 30, phaseWindowStart, phaseWindowEnd),
			})
			if len(got) != 3 {
				t.Fatalf("want three separate rows, got %d: %+v", len(got), got)
			}
		})
	}
}

func hmc376ContextRow(subject string, start, end, impact float64) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Subject: subject, Object: "trace_span", StartTs: start, EndTs: end, ImpactMS: impact,
		QueryWindowStartTs: 10, QueryWindowEndTs: 20,
	}
}

func hmc376ContextSubjects(rows []TraceCausalProjectionNode) []string {
	subjects := make([]string, 0, len(rows))
	for _, row := range rows {
		subjects = append(subjects, row.Subject)
	}
	return subjects
}

// A phase shown beside its seated collection leaves the seat to another thread.
func TestHMC376ContainedPhaseTakesNoContextSeat(t *testing.T) {
	got := traceCausalProjectionLimitContextNodesFold([]TraceCausalProjectionNode{
		hmc376ContextRow("gc", 11.00, 11.10, 100),
		hmc376ContextRow("gc", 11.01, 11.07, 60),
		hmc376ContextRow("a", 11.00, 11.09, 90),
		hmc376ContextRow("b", 11.00, 11.08, 80),
		hmc376ContextRow("c", 11.00, 11.07, 70),
	}, 3, "adjacent")
	subjects := hmc376ContextSubjects(got)
	if len(got) != 5 || subjects[0] != "gc" || subjects[1] != "a" || subjects[2] != "b" || got[3].MergedCount < 1 || subjects[4] != "gc" {
		t.Fatalf("want gc, a, b seated, c folded and the phase published last, got %v: %+v", subjects, got)
	}
	if got[4].ImpactMS != 60 {
		t.Fatalf("the published phase changed: %+v", got[4])
	}
}

// With its collection folded, a phase folds too. A merged family spans its
// occurrences, so a row between two occurrences is not inside it.
func TestHMC376ContextPhaseNeedsASeatedCollection(t *testing.T) {
	got := traceCausalProjectionLimitContextNodesFold([]TraceCausalProjectionNode{
		hmc376ContextRow("a", 11.00, 11.09, 90),
		hmc376ContextRow("gc", 11.00, 11.08, 80),
		hmc376ContextRow("gc", 11.01, 11.07, 60),
	}, 1, "adjacent")
	if len(got) != 2 || got[0].Subject != "a" || got[1].MergedCount != 2 {
		t.Fatalf("collection and phase must fold together: %+v", got)
	}
	family := hmc376ContextRow("gc", 11.00, 12.00, 300)
	family.MergedCount = 3
	if traceCausalProjectionSpanRowContainedByAny(hmc376ContextRow("gc", 11.40, 11.45, 50), []TraceCausalProjectionNode{family}) {
		t.Fatal("a merged family contains no row")
	}
}
