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
