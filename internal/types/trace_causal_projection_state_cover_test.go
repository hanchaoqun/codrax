package types

import (
	"strconv"
	"testing"
)

func stateCoverTestNode(id string, start, end, value float64) TraceCausalProjectionNode {
	r := b1638b3RestrictionRecord(id, value, 10, 20)
	r.SourceRef.TimeDomain, r.SourceRef.CanonicalTimeDomain, r.SourceRef.ClockAlignment = "trace_seconds", "trace_seconds", "identity"
	r.MeasurementSources.Domains[0].WindowStartTs, r.MeasurementSources.Domains[0].WindowEndTs = start, end
	return TraceCausalProjectionNode{EvidenceID: id, Subject: "worker-42", Object: "s_sleep", Predicate: "wakeup_causal_impact",
		Value: strconv.FormatFloat(value, 'g', -1, 64), Unit: "ms", ImpactMS: value, StartTs: start, EndTs: end,
		StateAccountKey: "physical:" + id, StateAccountComplete: true, Summary: id,
		QueryWindowStartTs: 1, QueryWindowEndTs: 2,
		MeasurementOrigins: []TraceSchedulerMeasurementOrigin{{SourceRef: r.SourceRef, ObservedAt: r.ObservedAt, MeasurementSources: r.MeasurementSources}}}
}

func TestCompleteStateCoverNestedCensusAndPair(t *testing.T) {
	for _, state := range []string{"s_sleep", "runnable", "d_sleep", "io_wait", "running"} {
		for _, order := range [][]int{{0, 1}, {1, 0}, {0, 1, 2}, {2, 1, 0}} {
			all := []TraceCausalProjectionNode{stateCoverTestNode("total", 1, 2, 100), stateCoverTestNode("a", 1.1, 1.2, 10), stateCoverTestNode("b", 1.4, 1.5, 15)}
			var nodes []TraceCausalProjectionNode
			for _, i := range order {
				all[i].Object = state
				nodes = append(nodes, all[i])
			}
			got := traceCausalProjectionAggregateSameKind(nodes)
			if len(got) != 1 || got[0].ImpactMS != 100 || !got[0].MergedIntervalUnion || got[0].Summary != "total" || got[0].StateAccountComplete || len(got[0].MergedEvidenceIDs) != len(order)-1 {
				t.Fatalf("state=%s order=%v: lost census or provenance: %+v", state, order, got)
			}
		}
	}
}

func TestCompleteStateCoverRequiresAllProofs(t *testing.T) {
	for name, mutate := range map[string]func(*TraceCausalProjectionNode){
		"no completeness":       func(n *TraceCausalProjectionNode) { n.StateAccountComplete = false },
		"no physical inventory": func(n *TraceCausalProjectionNode) { n.StateAccountKey = "" },
		"discounted value":      func(n *TraceCausalProjectionNode) { n.ImpactMS = 5 },
		"inversion composite":   func(n *TraceCausalProjectionNode) { n.PriorityInversionCandidate = true },
		"supply composite":      func(n *TraceCausalProjectionNode) { n.SupplyFoldComputed = true },
		"unknown provenance":    func(n *TraceCausalProjectionNode) { n.MeasurementOrigins = nil },
		"other capture":         func(n *TraceCausalProjectionNode) { n.MeasurementOrigins[0].SourceRef.Path = "/other.trace" },
		"other state":           func(n *TraceCausalProjectionNode) { n.Object = "running" },
		"other thread": func(n *TraceCausalProjectionNode) {
			n.MeasurementOrigins[0].MeasurementSources.Domains[0].TargetTID = 7
		},
		"other line selection": func(n *TraceCausalProjectionNode) {
			n.MeasurementOrigins[0].MeasurementSources.Domains[0].QueryLineStart = 100
		},
		"converted clock": func(n *TraceCausalProjectionNode) { n.MeasurementOrigins[0].SourceRef.ClockAlignment = "offset" },
		"partial overlap": func(n *TraceCausalProjectionNode) { n.EndTs = 2.1 },
		"disjoint":        func(n *TraceCausalProjectionNode) { n.StartTs = 2.1; n.EndTs = 2.2 },
	} {
		t.Run(name, func(t *testing.T) {
			nodes := []TraceCausalProjectionNode{stateCoverTestNode("total", 1, 2, 100), stateCoverTestNode("child", 1.1, 1.2, 10)}
			mutate(&nodes[1])
			if _, ok := traceCausalProjectionCompleteStateCover(nodes, []int{0, 1}); ok {
				t.Fatal("unproved containment admitted")
			}
		})
	}
}
