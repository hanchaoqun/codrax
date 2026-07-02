package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestViewCapacityTablePinsCurrentBehavior pins every numeric cap in the E4
// view capacity table. These values are literal consolidations of shipped
// behavior; 3bde10fa documents that a cap change pushes the model back to
// grep, so any future numeric change MUST be a deliberate edit of this test.
func TestViewCapacityTablePinsCurrentBehavior(t *testing.T) {
	cases := []struct {
		view         string
		defaultLimit int
		maxLimit     int
	}{
		{"event_search", 40, 0},
		{"span_window", 40, 0},
		{"scheduler_latency_stats", 20, 20},
		{"root_cause_rank", 12, 12},
		{"interaction_stats", 20, 20},
		{"frame_window", 20, 20},
		{"render_pipeline", 20, 20},
		{"frame_timeline", 20, 20},
		{"frame_flow", 20, 20},
		{"critical_blocking_calls", 20, 20},
		{"ipc_graph", 40, 0},
	}
	for _, tc := range cases {
		capacity := ViewCapacityFor(tc.view)
		if capacity.DefaultLimit != tc.defaultLimit || capacity.MaxLimit != tc.maxLimit {
			t.Fatalf("%s capacity = default %d / max %d, want %d / %d",
				tc.view, capacity.DefaultLimit, capacity.MaxLimit, tc.defaultLimit, tc.maxLimit)
		}
	}
	if got := ViewCapacityFor("span_window").FloorLimit; got != 8 {
		t.Fatalf("span_window floor limit = %d, want 8", got)
	}
	wakeup := ViewCapacityFor("wakeup_chain")
	// O10 ruling: MaxBranches=8 truncation is caveat-only — recorded, never raised.
	if wakeup.MaxDepth != 10 || wakeup.MaxBranches != 8 {
		t.Fatalf("wakeup_chain recursion caps = depth %d / branches %d, want 10 / 8", wakeup.MaxDepth, wakeup.MaxBranches)
	}
	stats := ViewCapacityFor("window_stats")
	if stats.AdvisoryBackgroundThreads != 4 || stats.AdvisoryBackgroundProcesses != 8 {
		t.Fatalf("window_stats advisory background caps = %d/%d, want 4/8",
			stats.AdvisoryBackgroundThreads, stats.AdvisoryBackgroundProcesses)
	}
	// v7 O1: the trace-mark span cap is a TWO-bucket cap (generic 8, semantic
	// 16 aligned with traceMarkSemanticSpanCap); collapsing them into one
	// number would revert the semantic-span reserved slots.
	if stats.AdvisoryGenericTraceMarkSpans != 8 || stats.AdvisorySemanticTraceMarkSpans != 16 {
		t.Fatalf("window_stats trace-mark span caps = generic %d / semantic %d, want 8 / 16",
			stats.AdvisoryGenericTraceMarkSpans, stats.AdvisorySemanticTraceMarkSpans)
	}
	if stats.AdvisorySemanticTraceMarkSpans != traceMarkSemanticSpanCap {
		t.Fatalf("semantic span advisory cap %d must mirror traceMarkSemanticSpanCap %d",
			stats.AdvisorySemanticTraceMarkSpans, traceMarkSemanticSpanCap)
	}
	// event_search IS the C3 escape hatch: it must never advertise a fallback
	// view of its own (suggesting it switch views would fight 133520c1).
	search := ViewCapacityFor("event_search")
	if search.FallbackView != "" || len(search.FallbackEventTypes) != 0 || search.HeavyView {
		t.Fatalf("event_search must stay the non-heavy escape hatch without a fallback: %+v", search)
	}
	for _, view := range []string{"span_window", "root_cause_rank", "window_stats", "recipe"} {
		capacity := ViewCapacityFor(view)
		if capacity.FallbackView != FallbackViewEventSearch {
			t.Fatalf("%s fallback view = %q, want %q", view, capacity.FallbackView, FallbackViewEventSearch)
		}
		if len(capacity.FallbackEventTypes) != 1 || capacity.FallbackEventTypes[0] != string(EventTraceMark) {
			t.Fatalf("%s fallback event types = %+v, want [trace_mark]", view, capacity.FallbackEventTypes)
		}
	}
}

// TestViewCapacityHeavyAndRelationScopedFlags pins the former tool-side
// switch lists that the table absorbed: every view except event_search is
// heavy, and only the two causal-chain views are relation-scopeable
// (verified design w9ffnwv29).
func TestViewCapacityHeavyAndRelationScopedFlags(t *testing.T) {
	heavy := []string{
		"scheduler_latency_stats", "root_cause_rank", "window_stats", "critical_blocking_calls", "evidence_pack", "recipe",
		"span_window", "frame_window", "render_pipeline", "frame_timeline", "frame_flow", "frame_root_cause_bundle",
		"thread_timeline", "ipc_graph", "wakeup_chain", "interaction_stats", "perf_stats", "perf_timeline", "trace_perf_bundle",
	}
	for _, view := range heavy {
		if !IsHeavyView(view) {
			t.Fatalf("view %q must stay heavy", view)
		}
	}
	for _, view := range []string{"event_search", "", "unknown_view"} {
		if IsHeavyView(view) {
			t.Fatalf("view %q must not be heavy", view)
		}
	}
	if !IsHeavyView("frame_bundle") {
		t.Fatalf("frame_bundle alias must resolve to the heavy frame_root_cause_bundle entry")
	}
	for _, view := range []string{"thread_timeline", "wakeup_chain"} {
		if !RelationScopedView(view) {
			t.Fatalf("view %q must stay relation-scopeable", view)
		}
	}
	for _, view := range []string{"root_cause_rank", "frame_root_cause_bundle", "window_stats", "event_search", ""} {
		if RelationScopedView(view) {
			t.Fatalf("view %q must not be relation-scopeable (pruning would drop whole-window aggregates)", view)
		}
	}
}

// TestViewCapacityClampLimitMatchesHistoricalClamps pins ClampLimit against
// the exact per-site literal expressions it replaced.
func TestViewCapacityClampLimitMatchesHistoricalClamps(t *testing.T) {
	scheduler := ViewCapacityFor("scheduler_latency_stats")
	for requested, want := range map[int]int{-1: 20, 0: 20, 5: 5, 20: 20, 21: 20, 100: 20} {
		if got := scheduler.ClampLimit(requested); got != want {
			t.Fatalf("scheduler ClampLimit(%d) = %d, want %d", requested, got, want)
		}
	}
	rank := ViewCapacityFor("root_cause_rank")
	for requested, want := range map[int]int{0: 12, 6: 6, 12: 12, 13: 12} {
		if got := rank.ClampLimit(requested); got != want {
			t.Fatalf("root_cause_rank ClampLimit(%d) = %d, want %d", requested, got, want)
		}
	}
	search := ViewCapacityFor("event_search")
	for requested, want := range map[int]int{0: 40, 5: 5, 400: 400} {
		if got := search.ClampLimit(requested); got != want {
			t.Fatalf("event_search ClampLimit(%d) = %d, want %d", requested, got, want)
		}
	}
}

// TestRunPublishesTypedCompactionForIndexedEventSearchLimit pins the T2 typed
// record at the indexed event_search limit-reached point: the scan stops at
// the cap, so Total stays 0 (unknown) while LastEmittedTs/Line come from the
// final emitted event — the anchors for a concrete window split.
func TestRunPublishesTypedCompactionForIndexedEventSearchLimit(t *testing.T) {
	idx := buildTraceIndex(t, "typed_compaction_event_search.systrace", `
      app-20 (20) [000] .... 9.000000: print: B|20|Choreographer#doFrame 170048
      app-20 (20) [000] .... 9.001000: print: E|20
      app-20 (20) [000] .... 9.010000: print: B|20|Choreographer#doFrame 170323
      app-20 (20) [000] .... 9.011000: print: E|20
      app-20 (20) [000] .... 9.020000: print: B|20|Choreographer#doFrame 173073
      app-20 (20) [000] .... 9.021000: print: E|20
`)
	res := Run(idx, Query{View: "event_search", Pattern: "Choreographer#doFrame", Limit: 2})
	if len(res.Events) != 2 {
		t.Fatalf("expected two limited rows, got %+v", res.Events)
	}
	if len(res.Compactions) != 1 {
		t.Fatalf("expected one typed compaction, got %+v", res.Compactions)
	}
	comp := res.Compactions[0]
	if comp.View != "event_search" || comp.Dimension != CompactionDimensionEvents {
		t.Fatalf("bad compaction identity: %+v", comp)
	}
	if comp.Total != 0 || comp.Emitted != 2 {
		t.Fatalf("indexed event_search compaction must report unknown total: %+v", comp)
	}
	if comp.LastEmittedTs != 9.010000 || comp.LastEmittedLine == 0 {
		t.Fatalf("compaction must anchor the last emitted event: %+v", comp)
	}
	// The prose caveat stays verbatim next to the typed record.
	if !containsSubstring(res.Caveats, "event_search_limit_reached=true") {
		t.Fatalf("limit caveat must stay verbatim: %+v", res.Caveats)
	}
}

// TestStreamEventSearchPublishesTypedCompaction pins the streamed
// event_search family that caveat-substring sniffing used to miss: the typed
// record carries the exact matched total because the stream keeps counting
// past the cap.
func TestStreamEventSearchPublishesTypedCompaction(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "typed_compaction_stream.systrace")
	body := strings.Join([]string{
		`      app-20 (20) [000] .... 9.000000: print: B|20|Choreographer#doFrame 170048`,
		`      app-20 (20) [000] .... 9.001000: print: E|20`,
		`      app-20 (20) [000] .... 9.010000: print: B|20|Choreographer#doFrame 173073`,
		`      app-20 (20) [000] .... 9.011000: print: E|20`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := StreamEventSearch(context.Background(), path, Query{Pattern: "Choreographer#doFrame", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Compactions) != 1 {
		t.Fatalf("expected one typed compaction, got %+v", res.Compactions)
	}
	comp := res.Compactions[0]
	if comp.View != "event_search" || comp.Total != 2 || comp.Emitted != 1 {
		t.Fatalf("bad streamed compaction: %+v", comp)
	}
	if comp.LastEmittedTs != 9.000000 || comp.LastEmittedLine != 1 {
		t.Fatalf("streamed compaction must anchor the last emitted event: %+v", comp)
	}
	if !containsSubstring(res.Caveats, "event_search_stream_compacted=true") {
		t.Fatalf("stream compaction caveat must stay verbatim: %+v", res.Caveats)
	}
}

// TestRunPublishesTypedCompactionForSpanWindow pins the FindSpanWindows cap:
// the typed record rides on the top-level Result next to the verbatim
// "span_window compacted from ..." caveat.
func TestRunPublishesTypedCompactionForSpanWindow(t *testing.T) {
	idx := buildTraceIndex(t, "typed_compaction_span_window.systrace", `
      app-20 (20) [000] .... 9.000000: print: B|20|PhaseOne
      app-20 (20) [000] .... 9.001000: print: E|20
      app-20 (20) [000] .... 9.010000: print: B|20|PhaseTwo
      app-20 (20) [000] .... 9.011000: print: E|20
      app-20 (20) [000] .... 9.020000: print: B|20|PhaseThree
      app-20 (20) [000] .... 9.021000: print: E|20
`)
	res := Run(idx, Query{View: "span_window", Limit: 2})
	if len(res.SpanWindows) != 2 {
		t.Fatalf("expected two limited spans, got %+v", res.SpanWindows)
	}
	if !containsSubstring(res.Caveats, "span_window compacted from 3 to 2 span(s)") {
		t.Fatalf("span compaction caveat must stay verbatim: %+v", res.Caveats)
	}
	if len(res.Compactions) != 1 {
		t.Fatalf("expected one typed compaction, got %+v", res.Compactions)
	}
	comp := res.Compactions[0]
	if comp.View != "span_window" || comp.Dimension != CompactionDimensionSpans || comp.Total != 3 || comp.Emitted != 2 {
		t.Fatalf("bad span_window compaction: %+v", comp)
	}
	if comp.LastEmittedTs != 9.010000 {
		t.Fatalf("span_window compaction must anchor the last emitted span start: %+v", comp)
	}
}

// TestIPCGraphPublishesTypedCompactions pins both ipc_graph truncation
// families (edges + binder aux rows) with their verbatim caveats intact.
func TestIPCGraphPublishesTypedCompactions(t *testing.T) {
	idx := buildTraceIndex(t, "typed_compaction_ipc.systrace", `
     client-20   (   20) [001] .... 3.010000: binder_transaction: transaction=1 dest_node=0 dest_proc=100 dest_thread=101 reply=0 flags=0x10 code=0x3
 binder:100_1-101 (  100) [002] .... 3.011000: binder_transaction_received: transaction=1
     client-20   (   20) [001] .... 3.020000: binder_transaction: transaction=2 dest_node=0 dest_proc=100 dest_thread=101 reply=0 flags=0x10 code=0x3
 binder:100_1-101 (  100) [002] .... 3.021000: binder_transaction_received: transaction=2
     client-20   (   20) [001] .... 3.030000: binder_transaction_alloc_buf: transaction=1 data_size=64 offsets_size=0 extra_buffers_size=0
     client-20   (   20) [001] .... 3.031000: binder_transaction_alloc_buf: transaction=2 data_size=64 offsets_size=0 extra_buffers_size=0
`)
	ipc := BuildIPCGraph(idx, Query{PID: 20, TimeStart: 3.0, TimeEnd: 3.04, Limit: 1})
	if len(ipc.Edges) != 1 || len(ipc.BinderEvents) != 1 {
		t.Fatalf("expected compacted edges and aux rows: %+v", ipc)
	}
	if !containsSubstring(ipc.Caveats, "ipc graph compacted from 2 to 1 edge(s)") ||
		!containsSubstring(ipc.Caveats, "binder auxiliary events compacted from 2 to 1 row(s)") {
		t.Fatalf("ipc compaction caveats must stay verbatim: %+v", ipc.Caveats)
	}
	if len(ipc.Compactions) != 2 {
		t.Fatalf("expected typed compactions for edges and aux rows, got %+v", ipc.Compactions)
	}
	byDimension := map[string]ViewCompaction{}
	for _, comp := range ipc.Compactions {
		if comp.View != "ipc_graph" {
			t.Fatalf("bad ipc compaction view: %+v", comp)
		}
		byDimension[comp.Dimension] = comp
	}
	edges, ok := byDimension[CompactionDimensionEdges]
	if !ok || edges.Total != 2 || edges.Emitted != 1 || edges.LastEmittedTs != 3.010000 {
		t.Fatalf("bad ipc edge compaction: %+v", byDimension)
	}
	aux, ok := byDimension[CompactionDimensionEvents]
	if !ok || aux.Total != 2 || aux.Emitted != 1 {
		t.Fatalf("bad ipc aux compaction: %+v", byDimension)
	}
}
