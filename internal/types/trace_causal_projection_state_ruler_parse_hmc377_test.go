package types

import "testing"

// HMC377 round 12 (R11P-2, R-377-12 ⑤): a rank-board seat's typed state ruler
// is read from its source note (the engine's rank item Source): the
// window_stats census seat, a wakeup_chain seat (the view and its sub-lanes),
// or neither. Records outside the rank-board family carry no ruler. See
// trace_causal_projection_state_ruler_hmc377_test.go for the merge.
func TestHMC377R12StateRulerReadsTheSourceNote(t *testing.T) {
	nodes := r12Nodes(
		r12RankRecord(2, "window_stats", 60, 100, 100, "f1"),
		r12RankRecord(3, "wakeup_chain", 40, 100, 100, "f1"),
		r12RankRecord(4, "wakeup_chain.causal_impacts", 5, 100, 100, "f1"),
		r12RankRecord(5, "scheduler_latency_stats", 5, 100, 100, "f1"),
	)
	want := []traceRankStateRuler{traceRankStateRulerCensus, traceRankStateRulerWakeupChain, traceRankStateRulerWakeupChain, traceRankStateRulerNone}
	for i := range nodes {
		if nodes[i].stateRuler != want[i] {
			t.Errorf("node %s ruler %d, want %d", nodes[i].EvidenceID, nodes[i].stateRuler, want[i])
		}
	}
	chain := ObservationRecord{Predicate: "wakeup_causal_impact", RichNotes: []string{"source=window_stats"}}
	if got := traceCausalProjectionNodeFromRecord("causal_hop", chain).stateRuler; got != traceRankStateRulerNone {
		t.Errorf("a record outside the rank-board family carries no ruler, got %d", got)
	}
}
