package types

import "testing"

func TestTraceDependencyWindowDisplayClassification(t *testing.T) {
	for _, tc := range []struct {
		predicate, source string
		want              bool
	}{
		{"wakeup_causal_impact", "", true},
		{"wakeup_causal_aggregate", "", true},
		{"root_cause_primary", "wakeup_chain.causal_impacts", true},
		{"root_cause_secondary", "wakeup_chain.aggregated_impacts", true},
		{"root_cause_primary", "window_stats.io_wait_top", false},
		{"state_drilldown", "window_stats.sleep_top", false},
		{"thread_timeline", "", false},
		{"trace_semantic_span", "", false},
		{"root_cause_primary", "arbitrary prose wakeup_chain.causal_impacts", false},
	} {
		if got := TraceUsesDependencyAnalysisWindow(tc.predicate, tc.source); got != tc.want {
			t.Errorf("%s/%s: %t", tc.predicate, tc.source, got)
		}
	}
	if TraceObservationUsesDependencyAnalysisWindow(ObservationRecord{Producer: "perf_trace", Predicate: "wakeup_causal_impact"}) {
		t.Fatal("non-query text cannot acquire a typed query display meaning")
	}
}
