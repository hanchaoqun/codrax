package tracequery

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestTraceSpanSchedulerNativeDoesNotChangeQueryOrRank(t *testing.T) {
	idx := hmosperfEvalIndex(t, "hmosperf_business_io_chain")
	q := normalizeQuery(idx, Query{View: "root_cause_rank", PID: 100, TimeStart: .999, TimeEnd: 1.051})
	stats := ComputeWindowStats(idx, q)
	chain := BuildWakeupChain(idx, q)
	with := buildRootCauseRankFrom(idx, q, chain, stats)
	plain := stats
	plain.TraceSpans = append([]TraceSpanSummary(nil), stats.TraceSpans...)
	for i := range plain.TraceSpans {
		plain.TraceSpans[i].SchedulerStates = nil
	}
	without := buildRootCauseRankFrom(idx, q, chain, plain)
	if !reflect.DeepEqual(with, without) {
		t.Fatal("marker-local state account changed root-cause ranking or attribution")
	}
	if stats.Window.StartTs != .999 || stats.Window.EndTs != 1.051 || q.PID != 100 {
		t.Fatal("per-marker evidence selected a different query focus")
	}
	for _, span := range stats.traceSpanFullInventory {
		if span.SchedulerStates != nil {
			t.Fatal("bounded marker display mutated the unbounded engine inventory")
		}
	}
}

func TestTraceSpanSchedulerNativeSourceAndOwnershipBoundaries(t *testing.T) {
	idx := hmosperfEvalIndex(t, "hmosperf_business_io_chain")
	q := normalizeQuery(idx, Query{View: "window_stats", TimeStart: .999, TimeEnd: 1.051})
	stats := ComputeWindowStats(idx, q)
	if len(stats.TraceSpans) < 1 || stats.TraceSpans[0].SchedulerStates == nil {
		t.Fatal("positive native account missing")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Index, *TraceSpanSummary)
	}{
		{"async", func(_ *Index, s *TraceSpanSummary) { s.Kind = "async" }},
		{"semantic", func(_ *Index, s *TraceSpanSummary) { s.SemanticClass = "shader_compile" }},
		{"missing_owner", func(_ *Index, s *TraceSpanSummary) { s.Thread.PID = 0 }},
		{"different_source", func(_ *Index, s *TraceSpanSummary) { s.SourcePath += ".other" }},
		{"composite", func(i *Index, _ *TraceSpanSummary) { i.TraceArtifacts = append(i.TraceArtifacts, i.TraceArtifacts[0]) }},
		{"relation_subset", func(i *Index, _ *TraceSpanSummary) { i.RelationScoped = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			localIndex := *idx
			localIndex.TraceArtifacts = append([]TraceArtifactSource(nil), idx.TraceArtifacts...)
			span := stats.TraceSpans[0]
			span.SchedulerStates = nil
			tc.mutate(&localIndex, &span)
			rows := []TraceSpanSummary{span}
			stampTraceSpanSchedulerStates(&localIndex, q, rows)
			if rows[0].SchedulerStates != nil {
				t.Fatal("unsupported source/ownership acquired a marker-local account")
			}
		})
	}
}

func TestTraceSpanSchedulerNativeSleepIOOverlayRemainsIncluded(t *testing.T) {
	idx := buildTraceIndex(t, "state-overlay.systrace", `# tracer: nop
task-7 (7) [000] .... 3.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120
task-7 (7) [000] .... 3.001000: tracing_mark_write: B|7|UnlabelledWork
task-7 (7) [000] .... 3.002000: sched_switch: prev_comm=task prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
task-7 (7) [000] .... 3.002010: sched_blocked_reason: pid=7 iowait=1 caller=observed_site
irq-9 (2) [000] .... 3.006000: sched_wakeup: comm=task pid=7 prio=120 target_cpu=000
task-7 (7) [000] .... 3.007000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120
task-7 (7) [000] .... 3.008000: tracing_mark_write: E|7
`)
	res := Run(idx, Query{View: "window_stats", TimeStart: 3, TimeEnd: 3.008})
	if res.WindowStats == nil || len(res.WindowStats.TraceSpans) != 1 {
		t.Fatal("missing synchronous business marker")
	}
	states := res.WindowStats.TraceSpans[0].SchedulerStates
	if states == nil || math.Abs(states.AccountedMs-7) > 1e-6 || math.Abs(states.SleepMs-4) > 1e-6 ||
		math.Abs(states.SleepIOWaitMs-4) > 1e-6 || states.IOWaitMs != 0 {
		t.Fatalf("S IO refinement was lost or double-counted: %+v", states)
	}
	before, _ := json.Marshal(states)
	if !states.Matches(idx.Path, "task-7", 3.001, 3.008) {
		t.Fatal("original typed account failed integrity check")
	}
	after, _ := json.Marshal(states)
	if string(before) != string(after) {
		t.Fatal("account validation mutated native source data")
	}
}

func TestTraceSpanSchedulerWindowedCheckpointRetainsMarkerLocalStates(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "marker-head.systrace",
		" task-7 (7) [000] .... 0.100000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120",
		" task-7 (7) [000] .... 1.010000: tracing_mark_write: B|7|OuterWork",
		" task-7 (7) [000] .... 1.020000: tracing_mark_write: B|7|InnerWork",
		" task-7 (7) [000] .... 1.030000: sched_switch: prev_comm=task prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120",
		" irq-80 (2) [000] .... 1.060000: sched_wakeup: comm=task pid=7 prio=120 target_cpu=000",
		" task-7 (7) [000] .... 1.070000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120",
		" task-7 (7) [000] .... 1.080000: tracing_mark_write: E|7",
		" task-7 (7) [000] .... 1.090000: tracing_mark_write: E|7",
		" task-7 (7) [000] .... 1.110000: sched_switch: prev_comm=task prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120",
	)
	idx := buildSchedulerCarryWindow(t, path, 1, 1.1)
	if !idx.Windowed || idx.schedulerHeadAt(1) == nil || idx.schedulerHeadAt(1.01) != nil {
		t.Fatal("fixture must retain only the original query-head checkpoint")
	}
	for _, ev := range idx.Events {
		if ev.Ts == .1 {
			t.Fatal("fixture retained the pre-padding scheduler event")
		}
	}
	q := normalizeQuery(idx, Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.1})
	stats := ComputeWindowStats(idx, q)
	if len(stats.TraceSpans) != 2 {
		t.Fatalf("missing nested markers: %+v", stats.TraceSpans)
	}
	for _, span := range stats.TraceSpans {
		wantRunning, wantTotal := 40.0, 80.0
		if span.Name == "InnerWork" {
			wantRunning, wantTotal = 20, 60
		}
		s := span.SchedulerStates
		if s == nil || s.Coverage != "complete" || !near(s.RunningMs, wantRunning, 1e-6) ||
			!near(s.SleepMs, 30, 1e-6) || !near(s.RunnableMs, 10, 1e-6) || !near(s.AccountedMs, wantTotal, 1e-6) {
			t.Errorf("%s lost proven pre-marker state or borrowed broad totals: %+v", span.Name, s)
			continue
		}
		if s.Window.StartTs != span.StartTs || s.Window.EndTs != span.EndTs ||
			s.MeasurementDomain.WindowStartTs != span.StartTs || s.MeasurementDomain.WindowEndTs != span.EndTs ||
			s.HeadState == nil || s.HeadState.BoundaryTs != span.StartTs {
			t.Errorf("%s reused the broad window receipt instead of rebuilding the clipped account: %+v", span.Name, s)
		}
	}
	if stats.Window.StartTs != 1 || stats.Window.EndTs != 1.1 || idx.schedulerHeadAt(1.01) != nil {
		t.Fatal("marker-local measurement mutated query focus or shared index checkpoints")
	}
	cache := newChainQueryCache(idx, nil)
	for _, span := range stats.TraceSpans {
		local := q
		local.TimeStart, local.TimeEnd = span.StartTs, span.EndTs
		local.TimeStartSet, local.TimeEndSet = true, true
		traceSpanSchedulerTimeline(cache, q, local, span.Thread)
	}
	if len(cache.timelineByKey) != 1 {
		t.Fatalf("nested markers rebuilt the same owner timeline: %d entries", len(cache.timelineByKey))
	}
}

func TestTraceSpanSchedulerWindowedUnknownTimeIsNotRecoveredFromMarkers(t *testing.T) {
	for _, tc := range []struct {
		name, prefix, middle, coverage string
		running, runnable, accounted   float64
	}{
		{
			name: "unknown_head", coverage: "partial", running: 40, runnable: 10, accounted: 50,
			prefix: " other-9 (9) [001] .... 0.100000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=9 next_prio=120",
			middle: " irq-80 (2) [000] .... 1.040000: sched_wakeup: comm=task pid=7 prio=120 target_cpu=000\n" +
				" task-7 (7) [000] .... 1.050000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120",
		},
		{
			name: "all_unknown", coverage: "unavailable",
			prefix: " other-9 (9) [001] .... 0.100000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=9 next_prio=120",
			middle: " other-9 (9) [001] .... 1.040000: tracing_mark_write: C|9|UnrelatedValue|1",
		},
		{
			name: "unclassified_middle", coverage: "partial", running: 50, runnable: 10, accounted: 60,
			prefix: " task-7 (7) [000] .... 0.100000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120",
			middle: " task-7 (7) [000] .... 1.030000: sched_switch: prev_comm=task prev_pid=7 prev_prio=120 prev_state=T ==> next_comm=idle next_pid=0 next_prio=120\n" +
				" irq-80 (2) [000] .... 1.050000: sched_wakeup: comm=task pid=7 prio=120 target_cpu=000\n" +
				" task-7 (7) [000] .... 1.060000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeSchedulerCarryTrace(t, "marker-unknown.systrace", tc.prefix,
				" task-7 (7) [000] .... 1.010000: tracing_mark_write: B|7|OrdinaryWork", tc.middle,
				" task-7 (7) [000] .... 1.090000: tracing_mark_write: E|7",
				" other-9 (9) [001] .... 1.110000: tracing_mark_write: C|9|UnrelatedValue|2",
			)
			idx := buildSchedulerCarryWindow(t, path, 1, 1.1)
			stats := ComputeWindowStats(idx, normalizeQuery(idx, Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.1}))
			if len(stats.TraceSpans) != 1 || stats.TraceSpans[0].SchedulerStates == nil {
				t.Fatalf("marker-local coverage disclosure missing: %+v", stats.TraceSpans)
			}
			s := stats.TraceSpans[0].SchedulerStates
			if s.Coverage != tc.coverage || !near(s.RunningMs, tc.running, 1e-6) || !near(s.RunnableMs, tc.runnable, 1e-6) || !near(s.AccountedMs, tc.accounted, 1e-6) {
				t.Fatalf("unknown or unclassified state was silently filled: %+v", s)
			}
		})
	}
}

func TestTraceSpanSchedulerWindowedQueryClippedSleepCarry(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "marker-sleep-carry.systrace",
		" task-7 (7) [000] .... 0.010000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120",
		" task-7 (7) [000] .... 0.960000: tracing_mark_write: B|7|AcrossWindowWork",
		" task-7 (7) [000] .... 0.970000: sched_switch: prev_comm=task prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120",
		" irq-80 (2) [000] .... 1.060000: sched_wakeup: comm=task pid=7 prio=120 target_cpu=000",
		" task-7 (7) [000] .... 1.070000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=7 next_prio=120",
		" task-7 (7) [000] .... 1.090000: tracing_mark_write: E|7",
		" task-7 (7) [000] .... 1.110000: sched_switch: prev_comm=task prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120",
	)
	idx := buildSchedulerCarryWindow(t, path, 1, 1.1)
	q := normalizeQuery(idx, Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.1})
	stats := ComputeWindowStats(idx, q)
	if len(stats.TraceSpans) != 1 || stats.TraceSpans[0].SchedulerStates == nil {
		t.Fatalf("query-clipped marker or account missing: %+v", stats.TraceSpans)
	}
	span := stats.TraceSpans[0]
	s := span.SchedulerStates
	if s.Coverage != "complete" || !near(s.RunningMs, 20, 1e-6) || !near(s.RunnableMs, 10, 1e-6) ||
		!near(s.SleepMs, 60, 1e-6) || !near(s.AccountedMs, 90, 1e-6) || span.ActualStartTs != .96 || span.StartTs != 1 {
		t.Fatalf("explicit query clipping lost sleep carry or borrowed full marker duration: %+v / %+v", span, s)
	}
}
