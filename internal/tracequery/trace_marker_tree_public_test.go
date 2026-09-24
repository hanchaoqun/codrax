package tracequery

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

const markerTreeNestedTrace = `# tracer: nop
task-7 (7) [000] .... 0.999000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120
task-7 (7) [000] .... 1.000000: tracing_mark_write: B|7|Checkout
task-7 (7) [000] .... 1.002000: tracing_mark_write: B|7|Load cart
task-7 (7) [000] .... 1.003000: tracing_mark_write: B|7|GC pause
task-7 (7) [000] .... 1.004000: tracing_mark_write: E|7
task-7 (7) [000] .... 1.004000: sched_switch: prev_comm=task prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
task-7 (7) [000] .... 1.004010: sched_blocked_reason: pid=7 iowait=1 caller=io_wait
irq-9 (2) [000] .... 1.006000: sched_wakeup: comm=task pid=7 prio=120 target_cpu=000
task-7 (7) [000] .... 1.007000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120
task-7 (7) [000] .... 1.007000: tracing_mark_write: E|7
task-7 (7) [000] .... 1.010000: tracing_mark_write: E|7
task-7 (7) [000] .... 1.011000: sched_switch: prev_comm=task prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`

func markerTreePublic(t *testing.T, idx *Index, q Query) *TraceMarkerTreeStats {
	t.Helper()
	q.View = "window_stats"
	res := Run(idx, q)
	if res.WindowStats == nil || res.WindowStats.BusinessTree == nil {
		t.Fatalf("public tree missing: %+v", res)
	}
	return res.WindowStats.BusinessTree
}

func markerTreeNamed(t *testing.T, tree *TraceMarkerTreeStats, name string) TraceMarkerTreeNode {
	t.Helper()
	for _, node := range tree.Nodes {
		if node.Name == name {
			return node
		}
	}
	t.Fatalf("missing marker %q: %+v", name, tree)
	return TraceMarkerTreeNode{}
}

func TestTraceMarkerTreePublicNestedCostsAndStateSets(t *testing.T) {
	idx := buildTraceIndex(t, "marker-tree.systrace", markerTreeNestedTrace)
	q := Query{TimeStart: 1, TimeEnd: 1.01, PID: 7}
	before, _ := json.Marshal(idx.Events)
	tree := markerTreePublic(t, idx, q)
	if tree.NodeCount != 3 || tree.OmittedNodes != 0 || tree.Coverage != "observed_stream" {
		t.Fatalf("tree inventory: %+v", tree)
	}
	root, child, grand := markerTreeNamed(t, tree, "Checkout"), markerTreeNamed(t, tree, "Load cart"), markerTreeNamed(t, tree, "GC pause")
	if root.ParentID != "" || root.ParentStatus != "observed_root" || child.ParentID != root.ID || grand.ParentID != child.ID || child.ParentStatus != "observed_parent" {
		t.Fatalf("physical topology lost: %+v", tree.Nodes)
	}
	for _, tc := range []struct {
		n                                         TraceMarkerTreeNode
		inclusive, self, running, sleep, runnable float64
	}{
		{root, 10, 5, 5, 0, 0}, {child, 5, 4, 1, 2, 1}, {grand, 1, 1, 1, 0, 0},
	} {
		if tc.n.Inclusive == nil || tc.n.Self == nil || !near(tc.n.Inclusive.DurationMs, tc.inclusive, 1e-8) || !near(tc.n.Self.DurationMs, tc.self, 1e-8) {
			t.Fatalf("cost mismatch: %+v", tc.n)
		}
		s := tc.n.Self.States
		if s == nil || s.Coverage != "complete" || s.Values == nil || !near(s.Values.RunningMs, tc.running, 1e-8) || !near(s.Values.SleepMs, tc.sleep, 1e-8) || !near(s.Values.SleepIOWaitMs, tc.sleep, 1e-8) || !near(s.Values.RunnableMs, tc.runnable, 1e-8) || !near(s.Values.AccountedMs, tc.self, 1e-8) {
			t.Fatalf("%s self scheduler set mismatch: %+v", tc.n.Name, s)
		}
	}
	if len(root.Self.Segments) != 2 || root.Self.Segments[0].EndTs != 1.002 || root.Self.Segments[1].StartTs != 1.007 {
		t.Fatalf("self is not disconnected parent-minus-child: %+v", root.Self)
	}
	after, _ := json.Marshal(idx.Events)
	if string(before) != string(after) || q.TimeStart != 1 || q.TimeEnd != 1.01 || q.PID != 7 {
		t.Fatal("tree computation mutated query or shared events")
	}
	stats := ComputeWindowStats(idx, normalizeQuery(idx, q))
	chain := BuildWakeupChain(idx, normalizeQuery(idx, q))
	with := buildRootCauseRankFrom(idx, normalizeQuery(idx, q), chain, stats)
	stats.BusinessTree = nil
	if !reflect.DeepEqual(with, buildRootCauseRankFrom(idx, normalizeQuery(idx, q), chain, stats)) {
		t.Fatal("business tree changed root-cause ranking")
	}
	legacy, inventory, _, _ := computeTraceMarksWithInventory(idx, normalizeQuery(idx, q), 8)
	for i := range stats.TraceSpans {
		stats.TraceSpans[i].SchedulerStates = nil
	}
	if !reflect.DeepEqual(stats.TraceSpans, legacy) || !reflect.DeepEqual(stats.traceSpanFullInventory, inventory) {
		t.Fatal("tree altered old bounded or full marker faces")
	}
}

func TestTraceMarkerTreePublicWindowAndZeroPrecision(t *testing.T) {
	t.Run("explicit_zero_end_is_not_unbounded", func(t *testing.T) {
		idx := buildTraceIndex(t, "zero-end.systrace", `task-7 (7) [000] .... 0.000000000: tracing_mark_write: B|7|at zero
task-7 (7) [000] .... 0.000000000: tracing_mark_write: E|7
task-7 (7) [000] .... 0.000000001: tracing_mark_write: B|7|later
task-7 (7) [000] .... 0.000000002: tracing_mark_write: E|7
`)
		tree := markerTreePublic(t, idx, Query{TimeStartSet: true, TimeEndSet: true})
		if tree.NodeCount != 1 || tree.Nodes[0].Name != "at zero" || tree.Nodes[0].Inclusive.DurationMs != 0 || tree.WindowUnavailableReason != "" {
			t.Fatalf("explicit zero end became unbounded: %+v", tree)
		}
		data, _ := json.Marshal(tree.Nodes[0].Inclusive.Segments)
		if string(data) != `[{"start_ts":0,"end_ts":0}]` {
			t.Fatalf("known zero endpoints omitted: %s", data)
		}
	})
	t.Run("cross_window", func(t *testing.T) {
		idx := buildTraceIndex(t, "clipped.systrace", markerTreeNestedTrace)
		tree := markerTreePublic(t, idx, Query{TimeStart: 1.0035, TimeEnd: 1.008})
		root, child := markerTreeNamed(t, tree, "Checkout"), markerTreeNamed(t, tree, "Load cart")
		if root.ActualStartTs != 1 || root.ActualEndTs == nil || *root.ActualEndTs != 1.01 || !near(root.Inclusive.DurationMs, 4.5, 1e-8) || !near(root.Self.DurationMs, 1, 1e-8) || !near(child.Self.DurationMs, 3, 1e-8) {
			t.Fatalf("raw endpoints or clipped set mismatch: %+v", tree)
		}
	})
	t.Run("zero_start_nanosecond_and_zero_duration", func(t *testing.T) {
		idx := buildTraceIndex(t, "zero.systrace", `task-7 (7) [000] .... 0.000000000: tracing_mark_write: B|7|tiny
task-7 (7) [000] .... 0.000000001: tracing_mark_write: E|7
task-7 (7) [000] .... 0.000000001: tracing_mark_write: B|7|point
task-7 (7) [000] .... 0.000000001: tracing_mark_write: E|7
`)
		tree := markerTreePublic(t, idx, Query{TimeStartSet: true, TimeEnd: .000000002, TimeEndSet: true})
		tiny, point := markerTreeNamed(t, tree, "tiny"), markerTreeNamed(t, tree, "point")
		if tiny.ActualStartTs != 0 || tiny.ActualEndTs == nil || !near(tiny.Inclusive.DurationMs, .000001, 1e-15) || point.Inclusive == nil || point.Self == nil || point.Inclusive.DurationMs != 0 || point.ActualEndTs == nil {
			t.Fatalf("zero/precision changed: %+v", tree)
		}
		if tiny.Inclusive.States.Values != nil || tiny.Inclusive.States.Coverage != "unavailable" || !near(tiny.Inclusive.States.UnknownMs, .000001, 1e-15) {
			t.Fatal("unknown nanosecond became known zero")
		}
		data, _ := json.Marshal(tiny)
		if !strings.Contains(string(data), `"actual_start_ts":0`) {
			t.Fatal("zero endpoint omitted")
		}
		if !strings.Contains(string(data), `"segments":[{"start_ts":0,"end_ts":1e-9}]`) {
			t.Fatal("zero segment boundary omitted")
		}
	})
}

func TestTraceMarkerTreePublicDisplayCapsDoNotChangeCosts(t *testing.T) {
	var raw strings.Builder
	fmt.Fprintln(&raw, "task-7 (7) [000] .... 1.000000: tracing_mark_write: B|7|Batch")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&raw, "task-7 (7) [000] .... %.6f: tracing_mark_write: B|7|same\n", 1.001+float64(i)*.002)
		fmt.Fprintf(&raw, "task-7 (7) [000] .... %.6f: tracing_mark_write: E|7\n", 1.002+float64(i)*.002)
	}
	fmt.Fprintln(&raw, "task-7 (7) [000] .... 1.100000: tracing_mark_write: E|7")
	idx := buildTraceIndex(t, "bounded.systrace", raw.String())
	tree := markerTreePublic(t, idx, Query{TimeStart: 1, TimeEnd: 1.1})
	root := markerTreeNamed(t, tree, "Batch")
	if tree.NodeCount != 41 || tree.OmittedNodes != 9 || len(tree.Nodes) != 32 || root.DirectChildCount != 40 || root.Self.OmittedSegments != 25 || len(root.Self.Segments) != 16 || !near(root.Self.DurationMs, 60, 1e-8) {
		t.Fatalf("display budget entered computation: %+v root=%+v", tree, root.Self)
	}
	ids := map[string]bool{}
	for _, node := range tree.Nodes {
		if ids[node.ID] {
			t.Fatal("same-name instances merged")
		}
		ids[node.ID] = true
	}
}

func TestTraceMarkerTreeSubtractsChildUnion(t *testing.T) {
	self := traceMarkerTreeSubtractChildren(TimeWindow{StartTs: 1, EndTs: 11}, []TimeWindow{{StartTs: 2, EndTs: 6}, {StartTs: 3, EndTs: 7}, {StartTs: 2, EndTs: 6}})
	want := []TimeWindow{{StartTs: 1, EndTs: 2}, {StartTs: 7, EndTs: 11}}
	if !reflect.DeepEqual(self, want) {
		t.Fatalf("overlapping direct children double counted: %+v", self)
	}
}

func TestTraceMarkerTreePublicCancellationNoPartialTree(t *testing.T) {
	idx := buildTraceIndex(t, "cancel.systrace", markerTreeNestedTrace)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := Run(idx, Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.01}.WithRunContext(ctx))
	if res.WindowStats != nil {
		t.Fatal("canceled window published a partial stats/tree")
	}
}

func TestTraceMarkerTreeStateIntegralScalesByOwner(t *testing.T) {
	idx := buildTraceIndex(t, "cached.systrace", markerTreeNestedTrace)
	q := normalizeQuery(idx, Query{TimeStart: 1, TimeEnd: 1.01})
	cache := newTraceMarkerTreeStateCache(idx, q, true)
	node := TraceMarkerTreeNode{SourcePath: idx.Path, Thread: ThreadRef{PID: 7, Comm: "task"}}
	for i := 0; i < 100; i++ {
		cache.account(node, []TimeWindow{{StartTs: 1 + float64(i)*.00001, EndTs: 1.01}})
	}
	if cache.cache == nil || len(cache.cache.timelineByKey) != 1 || len(cache.owners) != 1 {
		t.Fatal("nested intervals rebuilt owner timelines")
	}
	integral := traceMarkerTreeIntegral{}
	integral.append(0, 1)
	integral.append(2, 3)
	if math.Abs(integral.sum([]TimeWindow{{StartTs: .5, EndTs: 2.5}})-1000) > 1e-8 {
		t.Fatal("prefix integration crossed an unknown gap")
	}
}
