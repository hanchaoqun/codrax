package tracequery

import (
	"reflect"
	"testing"
)

func TestPerfSamplingTreeUnsupportedSelectorContractHMC235(t *testing.T) {
	q := Query{LineStart: 1, LineEnd: 2, Pattern: "work", Patterns: []string{"work"}, SpanName: "work", EventTypes: []EventType{"perf_sample"}, EventNames: []string{"sample"}, EventFieldFilters: []EventFieldFilter{{}}, TraceMarkActions: []TraceMarkAction{"B"}, TargetScope: TargetScopeProcess}
	want := []string{"line_start", "line_end", "pattern", "patterns", "span_name", "event_types", "event_names", "event_field_filters", "trace_mark_actions", "target_scope"}
	if got := PerfSamplingTreeUnsupportedSelectors(q); !reflect.DeepEqual(got, want) {
		t.Fatalf("selector contract drift: %v", got)
	}
	if got := PerfSamplingTreeUnsupportedSelectors(Query{PID: 11, Thread: "worker", TimeStart: 1, TimeEnd: 2, TargetScope: TargetScopeThread, Limit: 1}); len(got) != 0 {
		t.Fatalf("valid population/display controls rejected: %v", got)
	}
}
