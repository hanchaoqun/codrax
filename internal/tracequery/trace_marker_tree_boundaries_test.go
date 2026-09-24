package tracequery

import (
	"fmt"
	"strings"
	"testing"
)

func TestTraceMarkerTreePublicDIOAndStoppedSelfStates(t *testing.T) {
	for _, io := range []int{0, 1} {
		t.Run(fmt.Sprintf("iowait_%d", io), func(t *testing.T) {
			raw := fmt.Sprintf(`task-7 (7) [000] .... 0.999000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120
task-7 (7) [000] .... 1.000000: tracing_mark_write: B|7|Outer
task-7 (7) [000] .... 1.001000: tracing_mark_write: B|7|Child
task-7 (7) [000] .... 1.002000: sched_switch: prev_comm=task prev_pid=7 prev_prio=120 prev_state=D ==> next_comm=idle next_pid=0 next_prio=120
task-7 (7) [000] .... 1.002010: sched_blocked_reason: pid=7 iowait=%d caller=wait_site
irq-9 (2) [000] .... 1.004000: sched_wakeup: comm=task pid=7 prio=120 target_cpu=000
task-7 (7) [000] .... 1.005000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120
task-7 (7) [000] .... 1.006000: tracing_mark_write: E|7
task-7 (7) [000] .... 1.007000: sched_switch: prev_comm=task prev_pid=7 prev_prio=120 prev_state=T ==> next_comm=idle next_pid=0 next_prio=120
irq-9 (2) [000] .... 1.008000: sched_wakeup: comm=task pid=7 prio=120 target_cpu=000
task-7 (7) [000] .... 1.009000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120
task-7 (7) [000] .... 1.010000: tracing_mark_write: E|7
task-7 (7) [000] .... 1.011000: sched_switch: prev_comm=task prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`, io)
			idx := buildTraceIndex(t, "dio-stopped.systrace", raw)
			tree := markerTreePublic(t, idx, Query{TimeStart: 1, TimeEnd: 1.01})
			outer, child := markerTreeNamed(t, tree, "Outer"), markerTreeNamed(t, tree, "Child")
			state := child.Self.States
			if state.Coverage != "complete" || state.Values == nil || !near(state.Values.DStateMs, float64(2*(1-io)), 1e-8) || !near(state.Values.IOWaitMs, float64(2*io), 1e-8) || !near(state.Values.AccountedMs, 5, 1e-8) {
				t.Fatalf("D versus IO evidence conflated: %+v", state)
			}
			self := outer.Self.States
			if self.Coverage != "complete" || self.Values == nil || !near(self.Values.StoppedMs, 1, 1e-8) || !near(self.Values.RunningMs, 3, 1e-8) || !near(self.Values.RunnableMs, 1, 1e-8) || self.Values.SleepIOWaitMs != 0 {
				t.Fatalf("self terminal/scheduling states lost: %+v", self)
			}
		})
	}
}

func TestTraceMarkerTreePublicOpenParentAndResetBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, reset, closure string }{
		{"unclosed", "", "open"},
		{"malformed", "task-7 (7) [000] .... 1.004000: tracing_mark_write: B|bad|broken\n", "invalidated"},
		{"new_incarnation", "parent-9 (9) [000] .... 1.004000: sched_wakeup_new: comm=task pid=7 prio=120 target_cpu=000\n", "invalidated"},
		{"dead_switch", "task-7 (7) [000] .... 1.004000: sched_switch: prev_comm=task prev_pid=7 prev_prio=120 prev_state=X ==> next_comm=idle next_pid=0 next_prio=120\n", "invalidated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := "task-7 (7) [000] .... 0.900000: tracing_mark_write: B|7|Open parent\n" +
				"task-7 (7) [000] .... 1.001000: tracing_mark_write: B|7|Closed child\n" +
				"task-7 (7) [000] .... 1.003000: tracing_mark_write: E|7\n" + tc.reset
			if tc.reset != "" {
				raw += "task-7 (7) [000] .... 1.005000: tracing_mark_write: B|7|Fresh\ntask-7 (7) [000] .... 1.006000: tracing_mark_write: E|7\n"
			}
			idx := buildTraceIndex(t, "open-parent.systrace", raw)
			tree := markerTreePublic(t, idx, Query{TimeStart: 1, TimeEnd: 1.01})
			parent, child := markerTreeNamed(t, tree, "Open parent"), markerTreeNamed(t, tree, "Closed child")
			if parent.Closure != tc.closure || parent.Inclusive != nil || parent.Self != nil || parent.ActualEndTs != nil || child.ParentID != parent.ID || child.Inclusive == nil || !near(child.Self.DurationMs, 2, 1e-8) {
				t.Fatalf("unclosed parent borrowed child duration or lost identity: %+v", tree)
			}
			if tc.reset != "" {
				fresh := markerTreeNamed(t, tree, "Fresh")
				if fresh.ParentID != "" || fresh.ParentStatus != "unknown_prefix" {
					t.Fatalf("pairing crossed reset: %+v", fresh)
				}
			}
		})
	}
}

func TestTraceMarkerTreePublicCompositeCannotReorderPhysicalStack(t *testing.T) {
	idx := buildCPUScalarComposite(t,
		"rollback.ftrace", "task-7 (7) [000] .... 10.000000: tracing_mark_write: B|7|First\n"+
			"task-7 (7) [000] .... 11.000000: tracing_mark_write: E|7\n"+
			"task-7 (7) [000] .... 9.000000: tracing_mark_write: B|7|Later physical sibling\n"+
			"task-7 (7) [000] .... 12.000000: tracing_mark_write: E|7\n",
		"other.ftrace", "other-8 (8) [001] .... 10.500000: tracing_mark_write: B|8|Other\n"+
			"other-8 (8) [001] .... 10.600000: tracing_mark_write: E|8\n")
	res := Run(idx, Query{View: "window_stats", TimeStart: 8, TimeEnd: 13})
	if res.WindowStats == nil || res.WindowStats.BusinessTree != nil {
		t.Fatal("timestamp-sorted bundle forged physical sibling nesting")
	}
	if !containsSubstring(res.WindowStats.Caveats, "physical_marker_order_conflict") {
		t.Fatal("withheld tree lacked the physical-order limitation")
	}
}

func TestTraceMarkerTreePublicNoCrossThreadSourceOrAsyncEdges(t *testing.T) {
	idx := buildCPUScalarComposite(t,
		"first.ftrace", "task-7 (7) [000] .... 1.000000: tracing_mark_write: B|70|same\n"+
			"other-8 (8) [001] .... 1.001000: tracing_mark_write: B|70|same\n"+
			"other-8 (8) [001] .... 1.002000: tracing_mark_write: E|70\n"+
			"task-7 (7) [000] .... 1.003000: tracing_mark_write: S|70|async|5\n"+
			"other-8 (8) [001] .... 1.004000: tracing_mark_write: F|70|async|5\n"+
			"task-7 (7) [000] .... 1.005000: tracing_mark_write: E|70\n",
		"second.ftrace", "task-7 (7) [000] .... 1.001000: tracing_mark_write: B|70|same\n"+
			"task-7 (7) [000] .... 1.004000: tracing_mark_write: E|70\n")
	tree := markerTreePublic(t, idx, Query{TimeStart: 1, TimeEnd: 1.01})
	if tree.NodeCount != 3 {
		t.Fatalf("async marker entered synchronous tree: %+v", tree)
	}
	ids, sources := map[string]bool{}, map[string]bool{}
	for _, node := range tree.Nodes {
		if node.ParentID != "" || node.DirectChildCount != 0 || node.ParentStatus != "observed_root" || ids[node.ID] || node.Name != "same" || node.Inclusive.DurationMs != node.Self.DurationMs {
			t.Fatalf("name/containment/payload process synthesized nesting: %+v", node)
		}
		ids[node.ID], sources[node.SourcePath] = true, true
		if node.Inclusive.States.Values != nil {
			t.Fatal("composite PID borrowed another source scheduler timeline")
		}
		refs := idx.ResolveArtifactSpans(node.StartLine, node.StartLine)
		if len(refs) != 1 || refs[0].SourcePath != node.SourcePath {
			t.Fatalf("virtual begin line lost physical source: %+v", refs)
		}
	}
	if len(sources) != 2 {
		t.Fatal("physical source instances collapsed")
	}
}

func TestTraceMarkerTreePublicWindowedMissingParentKeepsHeadState(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "missing-parent.systrace",
		"task-7 (7) [000] .... 0.100000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120",
		"task-7 (7) [000] .... 0.200000: tracing_mark_write: B|7|Unretained parent",
		"task-7 (7) [000] .... 1.010000: tracing_mark_write: B|7|Visible child",
		"task-7 (7) [000] .... 1.020000: tracing_mark_write: E|7",
		"task-7 (7) [000] .... 1.030000: tracing_mark_write: E|7",
		"task-7 (7) [000] .... 1.110000: sched_switch: prev_comm=task prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120")
	idx := buildSchedulerCarryWindow(t, path, 1, 1.1)
	if !idx.Windowed || idx.schedulerHeadAt(1) == nil {
		t.Fatal("fixture lacks bounded parse with actual scheduler checkpoint")
	}
	tree := markerTreePublic(t, idx, Query{TimeStart: 1, TimeEnd: 1.1})
	child := markerTreeNamed(t, tree, "Visible child")
	if tree.NodeCount != 1 || tree.Coverage != "partial_topology" || child.ParentID != "" || child.ParentStatus != "unknown_prefix" {
		t.Fatalf("missing stack checkpoint became real root: %+v", tree)
	}
	if child.Self.States.Coverage != "complete" || child.Self.States.Values == nil || !near(child.Self.States.Values.RunningMs, 10, 1e-7) {
		t.Fatalf("scheduler head lost despite valid checkpoint: %+v", child.Self.States)
	}
}

func TestTraceMarkerTreePublicUnknownHeadAndBadScheduler(t *testing.T) {
	t.Run("unknown_head_known_tail", func(t *testing.T) {
		idx := buildTraceIndex(t, "partial.systrace", `task-7 (7) [000] .... 1.000000: tracing_mark_write: B|7|Partial
irq-9 (2) [000] .... 1.003000: sched_wakeup: comm=task pid=7 prio=120 target_cpu=000
task-7 (7) [000] .... 1.004000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120
task-7 (7) [000] .... 1.010000: tracing_mark_write: E|7
task-7 (7) [000] .... 1.011000: sched_switch: prev_comm=task prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`)
		n := markerTreeNamed(t, markerTreePublic(t, idx, Query{TimeStart: 1, TimeEnd: 1.01}), "Partial")
		s := n.Self.States
		if s.Coverage != "partial" || s.Values == nil || !near(s.UnknownMs, 3, 1e-7) || !near(s.Values.RunningMs, 6, 1e-7) || !near(s.Values.RunnableMs, 1, 1e-7) {
			t.Fatalf("unknown head replaced by zero or known tail lost: %+v", s)
		}
	})
	t.Run("bad_scheduler_keeps_marker_costs_only", func(t *testing.T) {
		idx := buildTraceIndex(t, "bad-scheduler.systrace", strings.Replace(markerTreeNestedTrace, "prev_comm=task prev_pid=7 prev_prio=120 prev_state=S", "prev_comm=task prev_pid=7 prev_prio=120", 1))
		res := Run(idx, Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.01})
		if res.WindowStats == nil || res.WindowStats.BusinessTree == nil || len(res.WindowStats.TraceSpans) != 0 {
			t.Fatal("bad scheduler either erased independent marker topology or restored old span lane")
		}
		n := markerTreeNamed(t, res.WindowStats.BusinessTree, "Checkout")
		if !near(n.Self.DurationMs, 5, 1e-8) || n.Self.States.Coverage != "unavailable" || n.Self.States.Values != nil || !near(n.Self.States.UnknownMs, 5, 1e-8) {
			t.Fatalf("bad scheduler minted state totals or erased costs: %+v", n.Self)
		}
	})
	t.Run("bad_marker_order_omits_tree", func(t *testing.T) {
		idx := buildTraceIndex(t, "bad-marker.systrace", "task-7 (7) [000] .... 2.000000: tracing_mark_write: B|7|Bad\ntask-7 (7) [000] .... 1.000000: tracing_mark_write: E|7\n")
		res := Run(idx, Query{View: "window_stats", TimeStart: .5, TimeEnd: 2.5})
		if res.WindowStats == nil || res.WindowStats.BusinessTree != nil {
			t.Fatal("bad physical marker order acquired tree costs")
		}
	})
}

func TestTraceMarkerTreePublicLineSelectionRetainsPhysicalIntervalScope(t *testing.T) {
	idx := buildTraceIndex(t, "lines.systrace", markerTreeNestedTrace)
	tree := markerTreePublic(t, idx, Query{LineStart: 4, LineEnd: 6})
	root := markerTreeNamed(t, tree, "Checkout")
	if tree.WindowUnavailableReason != "line_selected_time_window_undetermined" || root.Inclusive == nil || !near(root.Inclusive.DurationMs, 10, 1e-8) || root.Inclusive.Segments[0].StartTs != 1 || root.Inclusive.Segments[0].EndTs != 1.01 {
		t.Fatalf("line selection invented a continuous clock window or lost physical cost: %+v", tree)
	}
	if root.Inclusive.States.Coverage != "unavailable" {
		t.Fatal("line-bounded partial scheduler roster became complete state evidence")
	}
}
