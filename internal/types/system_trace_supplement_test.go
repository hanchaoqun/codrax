package types

import "testing"

// SUPP-CORE (DISPATCH-IND 批1, 2026-07-14) types-side pins: the typed
// call-window registry validity gate, the explore-fork delta fold (a
// parallel dispatch's recorded windows must survive MergeExploreFork without
// duplicating inherited ones), and the per-task reset boundary.

func TestTraceQueryCallWindowRegistryValidityGate(t *testing.T) {
	m := NewMutableState("q")
	m.RecordTraceQueryCallWindow(TraceQueryCallWindow{View: "event_search", TimeStart: 3.2, TimeEnd: 3.0})
	m.RecordTraceQueryCallWindow(TraceQueryCallWindow{View: "event_search", TimeStart: -1, TimeEnd: 1})
	m.RecordTraceQueryCallWindow(TraceQueryCallWindow{View: "event_search"})
	if got := m.TraceQueryCallWindows(); len(got) != 0 {
		t.Fatalf("invalid windows must be rejected, got %d", len(got))
	}
	m.RecordTraceQueryCallWindow(TraceQueryCallWindow{View: "event_search", TimeStart: 3.0, TimeEnd: 3.2})
	if got := m.TraceQueryCallWindows(); len(got) != 1 {
		t.Fatalf("valid window must be recorded, got %d", len(got))
	}
}

func TestTraceQueryCallWindowsSurviveExploreForkMerge(t *testing.T) {
	parent := NewMutableState("q")
	parent.RecordTraceQueryCallWindow(TraceQueryCallWindow{View: "event_search", TimeStart: 1.0, TimeEnd: 1.1})
	fork := parent.ForkForExploreDispatch()
	if got := fork.TraceQueryCallWindows(); len(got) != 1 {
		t.Fatalf("fork must inherit the parent's windows, got %d", len(got))
	}
	fork.RecordTraceQueryCallWindow(TraceQueryCallWindow{View: "window_stats", TimeStart: 2.0, TimeEnd: 2.1})
	fork.RecordTraceQueryCallWindow(TraceQueryCallWindow{View: "root_cause_rank", TimeStart: 2.0, TimeEnd: 2.1})
	parent.MergeExploreFork(fork)
	got := parent.TraceQueryCallWindows()
	if len(got) != 3 {
		t.Fatalf("merge must fold back exactly the fork's NEW windows (1 inherited + 2 new = 3), got %d: %+v", len(got), got)
	}
	if got[1].View != "window_stats" || got[2].View != "root_cause_rank" {
		t.Fatalf("fork delta order must be preserved: %+v", got)
	}
}

func TestSystemTraceSupplementResetBoundary(t *testing.T) {
	m := NewMutableState("q")
	m.RecordTraceQueryCallWindow(TraceQueryCallWindow{View: "event_search", TimeStart: 1.0, TimeEnd: 1.1})
	if !m.MarkSystemTraceSupplementAttempted() {
		t.Fatal("first latch mark must succeed")
	}
	m.SetSystemTraceSupplement(SystemTraceSupplementMeta{Views: []string{"root_cause_rank"}}, []ToolResult{{ToolName: "trace_query", Success: true}})
	m.ResetTurnAArtifacts()
	if m.SystemTraceSupplementAttempted() {
		t.Fatal("reset must clear the latch")
	}
	if m.SystemTraceSupplementMeta() != nil || len(m.SystemTraceSupplementResults()) != 0 || len(m.TraceQueryCallWindows()) != 0 {
		t.Fatal("reset must clear the supplement lane and the call-window registry")
	}
}
