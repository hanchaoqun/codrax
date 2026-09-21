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

func TestTraceStateDrilldownWindowDisplayClassification(t *testing.T) {
	for _, source := range []string{"top_sleep", "top_runnable", "top_running", "top_io_wait", "top_d_state", "state_churn"} {
		for _, producer := range []string{"trace_query", "trace_query:run2"} {
			record := ObservationRecord{Producer: producer, Predicate: "state_drilldown", RichNotes: []string{TraceNoteKeySource + "=" + source}}
			if got := TraceObservationMeasurementWindowDisplayRole(record, false); got != "cumulative state measurement scope" {
				t.Errorf("%s/%s: cumulative scope not disclosed: %q", producer, source, got)
			}
		}
	}
	for _, source := range []string{"", "unknown", "arbitrary top_sleep prose"} {
		got := TraceMeasurementWindowDisplayRole("state_drilldown", source, false)
		if got != "state drilldown measurement scope (single occurrence unproven)" {
			t.Errorf("unknown source must not acquire single-occurrence proof: %q", got)
		}
	}
	for _, predicate := range []string{"thread_timeline", "trace_semantic_span", "trace_span", "unknown"} {
		if got := TraceMeasurementWindowDisplayRole(predicate, "top_sleep", false); got != "" {
			t.Errorf("other producer family changed role: %s: %s", predicate, got)
		}
	}
	if got := TraceObservationMeasurementWindowDisplayRole(ObservationRecord{Producer: "perf_trace", Predicate: "state_drilldown", RichNotes: []string{"source=top_sleep"}}, false); got != "" {
		t.Errorf("non-query producer acquired typed scope: %q", got)
	}
	if !TraceObservationUsesDependencyAnalysisWindow(ObservationRecord{Producer: "trace_query:run2", Predicate: "wakeup_causal_impact"}) {
		t.Error("dependency window also must consume the normalized producer family")
	}
}
