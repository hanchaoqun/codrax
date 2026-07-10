package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTimestampAuthorityRejectsBodyLookalikes(t *testing.T) {
	line := `evil 0.100: fake-42 (  42) [000] .... 2.000000: sched_switch: prev_comm=x prev_pid=1 prev_prio=20 prev_state=S ==> next_comm=y next_pid=2 next_prio=20 note=" 0.050: sched_wakeup:"`
	if ts, ok := parseLineTimestamp(line); !ok || ts != 2.0 {
		t.Fatalf("timestamp must come from the anchored ftrace header, got ts=%v ok=%v", ts, ok)
	}
	if _, ok := parseLineTimestamp(`payload only 9.000000: sched_switch: prev_pid=1`); ok {
		t.Fatal("timestamp-looking payload without an ftrace header must not enter window/monotonic gates")
	}

	path := filepath.Join(t.TempDir(), "body-lookalike.systrace")
	body := strings.Join([]string{
		`a-1 ( 1) [000] .... 1.000000: tracing_mark_write: B|1|body 99.000000: fake_event:`,
		`a-1 ( 1) [000] .... 2.000000: tracing_mark_write: E|1`,
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if idx.TimestampOrder != TraceTimestampOrderMonotonic || idx.ClockRegressions != 0 {
		t.Fatalf("body lookalike polluted complete timestamp proof: order=%v regressions=%d", idx.TimestampOrder, idx.ClockRegressions)
	}
}

func TestKnownTIDNeverFallsBackToTGIDCommOrFreeText(t *testing.T) {
	ev := Event{
		Type:      EventPerfSample,
		PID:       22,
		TGID:      100,
		Comm:      "worker",
		FieldText: "mentions worker-21",
		PerfFields: &PerfFields{
			PID: 100, TID: 22, Comm: "worker", Symbol: "worker-21",
		},
	}
	if perfSampleMatchesThread(ev, ThreadRef{PID: 21, Comm: "worker"}) {
		t.Fatal("known tid=21 must not match sibling tid=22 through comm")
	}
	if perfSampleMatchesThread(ev, ThreadRef{PID: 100, Comm: "worker"}) {
		t.Fatal("known tid must not match perf process pid/tgid")
	}
	if eventMentionsThread(ev, "worker-21") {
		t.Fatal("pid-bearing selector must not fall back to symbol/free text")
	}
	if eventMentionsPID(ev, 100) {
		t.Fatal("thread pid filter must not expand to process/tgid")
	}
	if !perfSampleMatchesThread(ev, ThreadRef{Comm: "worker"}) {
		t.Fatal("comm-only fallback should remain available when no tid is known")
	}
	if stateDrilldownPinnedTarget(ThreadRef{PID: 22, Comm: "worker"}, 21, "worker") {
		t.Fatal("a pinned tid miss must not be rescued by same-name comm")
	}
}

func TestStreamStateClusterNameAmbiguityFailsClosedAndPIDIsExact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "same-name.systrace")
	body := strings.Join([]string{
		`app-10 ( 100) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=10 next_prio=20`,
		`app-20 ( 100) [001] .... 1.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
		`app-10 ( 100) [000] .... 1.050000: sched_switch: prev_comm=app prev_pid=10 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
		`app-20 ( 100) [001] .... 1.060000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ambiguous, err := StreamStateCluster(context.Background(), path, Query{Thread: "app", ThreadInput: "app", TimeStart: 1, TimeEnd: 1.1}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous.WindowStats == nil || len(ambiguous.WindowStats.TopRunning) != 0 || !containsSubstring(ambiguous.Caveats, "thread_selector_ambiguous") {
		t.Fatalf("name ambiguity must fail closed with a caveat: %+v", ambiguous.WindowStats)
	}
	exact, err := StreamStateCluster(context.Background(), path, Query{PID: 10, TimeStart: 1, TimeEnd: 1.1}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if exact.WindowStats == nil || len(exact.WindowStats.TopRunning) != 1 || exact.WindowStats.TopRunning[0].Thread.PID != 10 {
		t.Fatalf("pid filter must retain only exact tid=10: %+v", exact.WindowStats)
	}
	processID, err := StreamStateCluster(context.Background(), path, Query{PID: 100, TimeStart: 1, TimeEnd: 1.1}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if processID.WindowStats == nil || len(processID.WindowStats.TopRunning) != 0 {
		t.Fatalf("pid=100 must not expand to sibling tids by TGID: %+v", processID.WindowStats)
	}
}

func TestSchedWakeupNewResetsGenerationAndCarriesRunnable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wakeup-new.systrace")
	body := strings.Join([]string{
		`old-42 ( 700) [000] .... 0.100000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=42 next_prio=20`,
		`old-42 ( 700) [000] .... 0.200000: sched_switch: prev_comm=old prev_pid=42 prev_prio=20 prev_state=X ==> next_comm=idle/0 next_pid=0 next_prio=120`,
		`creator-7 ( 7) [000] .... 0.300000: sched_wakeup_new: comm=new pid=42 prio=30 target_cpu=000`,
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 1, TimeEnd: 1.1, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	head := idx.schedulerHeadAt(1)
	state, ok := head.Threads[42]
	if head == nil || !head.Complete || !ok || state.State != StateRunnable || state.Thread.Comm != "new" || state.Thread.TGID != 0 || state.StartTs != 0.3 {
		t.Fatalf("sched_wakeup_new must replace old generation state: head=%+v state=%+v", head, state)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.1})
	if td := threadDurationForPID(stats.RunnableTop, 42); td == nil || td.DurationMs < 99.999 || td.DurationMs > 100.001 {
		t.Fatalf("indexed runnable carry missing after wakeup_new: %+v", stats.RunnableTop)
	}
	streamed, err := StreamStateCluster(context.Background(), path, Query{PID: 42, TimeStart: 1, TimeEnd: 1.1}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if td := threadDurationForPID(streamed.WindowStats.RunnableTop, 42); td == nil || td.DurationMs < 99.999 || td.DurationMs > 100.001 {
		t.Fatalf("stream runnable carry missing after wakeup_new: %+v", streamed.WindowStats.RunnableTop)
	}
}

func TestSchedWakeupNewNeverClosesOldGenerationSleepCausality(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1.000, Type: EventSchedSwitch, CPU: 0, PrevComm: "old", PrevPID: 42, PrevState: "S", NextComm: "idle/0"},
		{Line: 2, Ts: 1.100, Type: EventSchedWakeup, Name: "sched_wakeup_new", Comm: "creator", PID: 7, WakeeComm: "new", WakeePID: 42},
	}, FirstTs: 1, LastTs: 1.1, TimestampOrder: TraceTimestampOrderMonotonic}
	if wakeup, _ := findWakeupFor(idx, ThreadRef{PID: 42}, 1.0, 1.1); wakeup != nil {
		t.Fatalf("creation event was misused as a wakeup for the old TID occupant: %+v", wakeup)
	}
	if fallback := resolveCounterpartViaWakeupEdge(idx, ThreadRef{PID: 42}, 1.0, 1.1); fallback.OK {
		t.Fatalf("creation event fabricated a cross-generation counterpart: %+v", fallback)
	}
	chain := BuildWakeupChain(idx, Query{PID: 42, TimeStart: 1, TimeEnd: 1.2, MinDurationMs: 1, MaxDepth: 4})
	for _, edge := range chain.Edges {
		if edge.Waker.PID == 7 && edge.Wakee.PID == 42 {
			t.Fatalf("creator->old occupant is not a wakeup dependency: %+v", edge)
		}
	}
}

func TestCrossIncarnationTargetAndSchedulerAggregatesFailClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cross-incarnation.systrace")
	body := strings.Join([]string{
		`old-42 ( 700) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=42 next_prio=20`,
		`old-42 ( 700) [000] .... 1.100000: sched_switch: prev_comm=old prev_pid=42 prev_prio=20 prev_state=X ==> next_comm=idle/0 next_pid=0 next_prio=120`,
		`creator-7 ( 7) [000] .... 1.200000: sched_wakeup_new: comm=new pid=42 prio=30 target_cpu=000`,
		`idle-0 ( 0) [000] .... 1.300000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=new next_pid=42 next_prio=30`,
		`new-42 ( 800) [000] .... 1.400000: sched_switch: prev_comm=new prev_pid=42 prev_prio=30 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{PID: 42, TimeStart: 0.9, TimeEnd: 1.5}
	if tl := ThreadTimeline(idx, q); len(tl.Intervals) != 0 || !containsSubstring(tl.Caveats, "thread_identity_fail_closed=true") {
		t.Fatalf("target timeline merged two occupants of tid=42: %+v", tl)
	}
	chain := BuildWakeupChain(idx, q)
	if !containsSubstring(chain.Caveats, "wakeup_chain_fail_closed=true") || !containsSubstring(chain.Caveats, "thread_identity_fail_closed=true") {
		t.Fatalf("wakeup chain did not disclose target identity failure: %+v", chain)
	}
	for _, evidence := range chain.RootEvidence {
		if evidence.Type == "trace_gap" {
			t.Fatalf("identity failure was converted into fabricated trace-gap evidence: %+v", chain.RootEvidence)
		}
	}
	stats := ComputeWindowStats(idx, q)
	if len(stats.CPU) != 0 || len(stats.TopRunning) != 0 || len(stats.RunnableTop) != 0 || !containsSubstring(stats.Caveats, "thread_identity_fail_closed=true") {
		t.Fatalf("global scheduler durations must not publish PID-keyed cross-generation aggregates: %+v", stats)
	}
	streamed, err := StreamStateCluster(context.Background(), path, q, 8)
	if err != nil {
		t.Fatal(err)
	}
	if streamed.WindowStats == nil || len(streamed.WindowStats.TopRunning) != 0 || len(streamed.WindowStats.RunnableTop) != 0 || !containsSubstring(streamed.Caveats, "thread_identity_fail_closed=true") {
		t.Fatalf("streaming state cluster merged TID incarnations: %+v", streamed.WindowStats)
	}

	// A boundary exactly at the left edge selects only the new generation;
	// metadata must be enriched from that generation, never inherited from old.
	newOnly := ThreadTimeline(idx, Query{PID: 42, TimeStart: 1.2, TimeEnd: 1.5})
	if newOnly.Thread.Comm != "new" || newOnly.Thread.TGID != 800 || containsSubstring(newOnly.Caveats, "thread_identity_fail_closed=true") {
		t.Fatalf("new-only window inherited stale identity metadata: %+v", newOnly)
	}
}

func TestBoundedOldNameCannotBindReusedTID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bounded-old-name.systrace")
	body := strings.Join([]string{
		`old-name-42 (700) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old-name next_pid=42 next_prio=20`,
		`old-name-42 (700) [000] .... 1.100000: sched_switch: prev_comm=old-name prev_pid=42 prev_prio=20 prev_state=X ==> next_comm=idle/0 next_pid=0 next_prio=120`,
		`creator-7 (7) [000] .... 1.200000: sched_wakeup_new: comm=new-name pid=42 prio=30 target_cpu=000`,
		`idle-0 (0) [000] .... 1.300000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=new-name next_pid=42 next_prio=30`,
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	resolution := resolveThreadSelection(idx, Query{Thread: "old-name", ThreadInput: "old-name", TimeStart: 1.2, TimeEnd: 1.4})
	if resolution.Thread.PID != 0 || len(resolution.CandidatePIDs) != 0 {
		t.Fatalf("old-generation comm bound the new occupant of tid=42: %+v", resolution)
	}
}

func TestIncarnationBoundaryAtInclusiveRightEdgeFailsClosed(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1.0, Type: EventSchedSwitch, CPU: 0, PrevPID: 0, NextPID: 42, NextComm: "old"},
		{Line: 2, Ts: 1.2, Type: EventSchedWakeup, Name: "sched_wakeup_new", PID: 7, WakeePID: 42, WakeeComm: "new"},
	}, FirstTs: 1, LastTs: 1.2, TimestampOrder: TraceTimestampOrderMonotonic}
	timeline := ThreadTimeline(idx, Query{PID: 42, TimeStart: 0.9, TimeEnd: 1.2})
	if timeline.IntegrityFailure != "thread_incarnation_conflict" || len(timeline.Intervals) != 0 {
		t.Fatalf("inclusive right-edge creation was excluded from identity audit: %+v", timeline)
	}
}

func TestIncarnationBoundaryAtInclusiveLeftEdgeWithOldRowAtSameTimestampFailsClosed(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 2.0, Type: EventSchedSwitch, CPU: 0, PrevPID: 0, NextPID: 42, NextComm: "old"},
		{Line: 2, Ts: 2.0, Type: EventSchedWakeup, Name: "sched_wakeup_new", PID: 7, WakeePID: 42, WakeeComm: "new"},
	}, FirstTs: 2, LastTs: 2, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{PID: 42, TimeStart: 2.0, TimeEnd: 2.1}
	conflict := threadIncarnationConflictForQuery(idx, q, 42)
	if conflict == nil || conflict.PreviousTs != 2.0 || conflict.PreviousLine != 1 || conflict.BoundaryLine != 2 {
		t.Fatalf("inclusive left edge erased physical same-timestamp generation order: %+v", conflict)
	}
	if timeline := ThreadTimeline(idx, q); timeline.IntegrityFailure != "thread_incarnation_conflict" || len(timeline.Intervals) != 0 {
		t.Fatalf("same-timestamp old/new generations were merged at the inclusive left edge: %+v", timeline)
	}

	newOnly := &Index{Events: []Event{
		{Line: 1, Ts: 1.9, Type: EventSchedSwitch, CPU: 0, PrevPID: 0, NextPID: 42, NextComm: "old"},
		{Line: 2, Ts: 2.0, Type: EventSchedWakeup, Name: "sched_wakeup_new", PID: 7, WakeePID: 42, WakeeComm: "new"},
	}, FirstTs: 1.9, LastTs: 2, TimestampOrder: TraceTimestampOrderMonotonic}
	if conflict := threadIncarnationConflictForQuery(newOnly, q, 42); conflict != nil {
		t.Fatalf("left-edge creation with only pre-window old evidence must remain new-only: %+v", conflict)
	}
}

func TestReappearanceAfterDeadWithoutWakeupNewFailsClosed(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 2.0, Type: EventSchedSwitch, CPU: 0, PrevPID: 42, PrevComm: "old", PrevState: "Z", NextPID: 0},
		{Line: 2, Ts: 2.1, Type: EventSchedSwitch, CPU: 0, PrevPID: 0, NextPID: 42, NextComm: "new"},
	}, FirstTs: 2, LastTs: 2.1, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{PID: 42, TimeStart: 1.9, TimeEnd: 2.2}
	conflict := threadIncarnationConflictForQuery(idx, q, 42)
	if conflict == nil || conflict.Signal != "reappeared_after_dead" {
		t.Fatalf("exact X/Z then reappearance must cut a generation: %+v", conflict)
	}
	if tl := ThreadTimeline(idx, q); len(tl.Intervals) != 0 || !containsSubstring(tl.Caveats, "thread_identity_fail_closed=true") {
		t.Fatalf("dead/reused TID was not failed closed: %+v", tl)
	}
}

func TestLifecycleAuditPreservesEveryConflictOnOneSchedulerRow(t *testing.T) {
	tracker := newThreadIncarnationTracker()
	// ENG audit #42 (2026-07-10): seeding switched observe→observeAll when the
	// first-conflict-only observe helper was removed (it masked window-relevant
	// sibling conflicts while consuming their lifecycle evidence).
	tracker.observeAll(Event{Line: 1, Ts: 1.0, Type: EventSchedSwitch, PrevPID: 10, PrevState: "X", NextPID: 0}, 0)
	tracker.observeAll(Event{Line: 2, Ts: 1.1, Type: EventSchedSwitch, PrevPID: 20, PrevState: "X", NextPID: 0}, 0)
	conflicts := tracker.observeAll(Event{Line: 3, Ts: 1.2, Type: EventSchedSwitch, PrevPID: 10, PrevState: "R", NextPID: 20}, 0)
	if len(conflicts) != 2 || conflicts[0].PID != 10 || conflicts[1].PID != 20 {
		t.Fatalf("one scheduler row masked an independent lifecycle conflict: %+v", conflicts)
	}
}

func TestLifecycleBoundaryClearsTraceMarkAndBinderStacks(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1.00, Type: EventTraceMark, PID: 42, Comm: "old", SpanAction: "B", SpanName: "transact[old.Interface:7]"},
		{Line: 2, Ts: 1.01, Type: EventTraceMark, PID: 42, Comm: "old", SpanAction: "S", SpanPID: 42, SpanName: "old-async", SpanValue: "9"},
		{Line: 3, Ts: 1.10, Type: EventSchedWakeup, Name: "sched_wakeup_new", PID: 7, WakeePID: 42, WakeeComm: "new"},
		{Line: 4, Ts: 1.20, Type: EventTraceMark, PID: 42, Comm: "new", SpanAction: "E"},
		{Line: 5, Ts: 1.21, Type: EventTraceMark, PID: 42, Comm: "new", SpanAction: "F", SpanPID: 42, SpanName: "old-async", SpanValue: "9"},
		{Line: 6, Ts: 1.22, Type: EventBinderTransaction, PID: 42, Comm: "new", BinderFields: &BinderFields{TransactionID: 1, DestThread: 88}},
	}, FirstTs: 1, LastTs: 1.22, TimestampOrder: TraceTimestampOrderMonotonic}

	spans, _, _ := computeTraceMarks(idx, Query{TimeStart: 0.9, TimeEnd: 1.3}, 16)
	if len(spans) != 0 {
		t.Fatalf("new TID occupant closed old generation trace-mark stacks: %+v", spans)
	}
	graph := BuildIPCGraph(idx, Query{TimeStart: 0.9, TimeEnd: 1.3})
	if len(graph.Edges) != 1 || graph.Edges[0].Interface != "" {
		t.Fatalf("old generation transact interface leaked onto new binder send: %+v", graph.Edges)
	}
}

func TestRelationScopedIndexNeverPublishesGlobalCarryAggregates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relation-head.systrace")
	body := strings.Join([]string{
		`noise-99 ( 99) [003] .... 0.500000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=noise next_pid=99 next_prio=20`,
		`app-20 ( 20) [001] .... 0.600000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`waker-10 ( 10) [000] .... 1.050000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`,
		`app-20 ( 20) [001] .... 1.080000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
		`noise-99 ( 99) [003] .... 1.100000: sched_switch: prev_comm=noise prev_pid=99 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120`,
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 1, TimeEnd: 1.2, TimeStartSet: true, TimeEndSet: true,
		AllowWindowedParse: true, ScopePID: 20, RelationScoped: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !idx.RelationScoped {
		t.Fatal("expected precise relation-scoped marker")
	}
	head := idx.schedulerHeadAt(1)
	if head == nil || !head.Complete || len(head.CPUs) != 0 {
		t.Fatalf("relation-scoped head must not expose CPU-global checkpoints: %+v", head)
	}
	if _, leaked := head.Threads[99]; leaked {
		t.Fatalf("unrelated thread leaked into scoped head: %+v", head.Threads)
	}
	res := Run(idx, Query{View: "wakeup_chain", PID: 20, TimeStart: 1, TimeEnd: 1.2})
	if res.WindowStats != nil || !containsSubstring(res.Caveats, "relation_scoped_window_stats_unavailable") {
		t.Fatalf("scoped wakeup chain must omit global stats: stats=%+v caveats=%v", res.WindowStats, res.Caveats)
	}
	direct := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.2})
	if len(direct.CPU) != 0 || !containsSubstring(direct.Caveats, "relation_scoped_window_stats_unavailable") {
		t.Fatalf("direct global stats on a scoped index must fail closed: %+v", direct)
	}
}

func TestAggregateHeadCoverageMarksMissingSubjects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-subject.systrace")
	body := strings.Join([]string{
		`worker-20 ( 20) [002] .... 1.050000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=20 next_prio=20`,
		`worker-20 ( 20) [002] .... 1.080000: sched_switch: prev_comm=worker prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120`,
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 1, TimeEnd: 1.1, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.1})
	cov := stats.SchedulerHeadCoverage
	if cov == nil || cov.Status != "partial_unknown" || cov.MissingCPUCount != 1 || cov.MissingThreadCount == 0 || len(cov.MissingCPUs) != 1 || cov.MissingCPUs[0] != 2 {
		t.Fatalf("missing per-subject checkpoints must be typed: %+v", cov)
	}
	if !containsSubstring(stats.Caveats, "scheduler_head_subjects_unknown") {
		t.Fatalf("missing subject coverage caveat: %v", stats.Caveats)
	}
	if td := threadDurationForPID(stats.TopRunning, 20); td == nil || td.DurationMs < 29.999 || td.DurationMs > 30.001 {
		t.Fatalf("known post-first-switch interval should remain, without fabricating the 50ms head: %+v", stats.TopRunning)
	}
}

func TestCrossLaneWidePaddingRecoversTargetHead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wide-padding-regressed.systrace")
	body := strings.Join([]string{
		`app-20 ( 20) [001] .... 0.100000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`noise-99 ( 99) [002] .... 2.000000: sched_switch: prev_comm=noise prev_pid=99 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120`,
		`waker-10 ( 10) [000] .... 0.500000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`,
		`app-20 ( 20) [001] .... 1.050000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 1, TimeEnd: 1.1, TimeStartSet: true, TimeEndSet: true,
		TimePaddingBefore: 1, TimePaddingAfter: 1, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	tl := ThreadTimeline(idx, Query{PID: 20, TimeStart: 1, TimeEnd: 1.1})
	if tl.HeadState == nil || tl.HeadState.Status != "recovered" || tl.HeadState.State != StateRunnable {
		t.Fatalf("cross-lane physical interleave must preserve a monotonic target carry: %+v", tl.HeadState)
	}
	if len(tl.Intervals) == 0 || tl.Intervals[0].State != StateRunnable || tl.Intervals[0].StartTs != 1 {
		t.Fatalf("safe per-lane head recovery did not seed the target at the window boundary: %+v", tl.Intervals)
	}
	if containsSubstring(tl.Caveats, "scheduler_head_state_unknown") {
		t.Fatalf("global interleave must not poison a lane-monotonic head: %v", tl.Caveats)
	}
}

func TestIndexedStateMachinesAcceptCrossLanePhysicalInterleave(t *testing.T) {
	path := writeTimestampRegressionTrace(t, "indexed-same-tid.systrace",
		sweepSwitchLine(2.000000, "idle/0", 0, "R", "app", 20),
		strings.Replace(sweepSwitchLine(4.000000, "noise", 99, "S", "idle/2", 0), "[001]", "[002]", 1),
		sweepSwitchLine(2.500000, "app", 20, "S", "idle/0", 0),
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if idx.TimestampOrder != TraceTimestampOrderRegressed {
		t.Fatalf("fixture must be typed regressed, got %v", idx.TimestampOrder)
	}
	tl := ThreadTimeline(idx, Query{PID: 20, TimeStart: 1.5, TimeEnd: 3})
	if len(tl.Intervals) != 2 || tl.Intervals[0].State != StateRunning || tl.Intervals[0].DurationMs < 499.999 || tl.Intervals[0].DurationMs > 500.001 {
		t.Fatalf("an unrelated lane interleave must not invalidate a monotonic target lane: %+v", tl.Intervals)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.5, TimeEnd: 3})
	if td := threadDurationForPID(stats.TopRunning, 20); td == nil || td.DurationMs < 499.999 || td.DurationMs > 500.001 {
		t.Fatalf("cross-lane physical interleave must preserve each monotonic scheduler lane: %+v", stats.TopRunning)
	}
}

func TestSchedulerDurationFacesFailClosedOnSameLaneClockRollback(t *testing.T) {
	path := writeTimestampRegressionTrace(t, "same-lane-rollback.systrace",
		sweepSwitchLine(2.000000, "idle/0", 0, "R", "app", 20),
		sweepSwitchLine(2.600000, "app", 20, "S", "idle/0", 0),
		sweepSwitchLine(2.400000, "idle/0", 0, "R", "app", 20),
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	tl := ThreadTimeline(idx, Query{PID: 20, TimeStart: 1.5, TimeEnd: 3})
	if len(tl.Intervals) != 0 || !containsSubstring(tl.Caveats, "scheduler_duration_fail_closed=true") {
		t.Fatalf("timeline fabricated elapsed time across a same-TID rollback: %+v", tl)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.5, TimeEnd: 3})
	if len(stats.CPU) != 0 || len(stats.TopRunning) != 0 || !containsSubstring(stats.Caveats, "scheduler_duration_fail_closed=true") {
		t.Fatalf("aggregate scheduler durations must fail closed on the same rollback: %+v", stats)
	}
	streamed, err := StreamStateCluster(context.Background(), path, Query{PID: 20, TimeStart: 1.5, TimeEnd: 3}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if streamed.WindowStats == nil || len(streamed.WindowStats.TopRunning) != 0 || !containsSubstring(streamed.Caveats, "stream_state_cluster_fail_closed=true") {
		t.Fatalf("streaming state durations must fail closed on the same rollback: %+v", streamed.WindowStats)
	}
}

func TestLineWindowUnknownOrderStillFailsClosedOnObservedLaneRollback(t *testing.T) {
	path := writeTimestampRegressionTrace(t, "line-window-lane-rollback.systrace",
		sweepSwitchLine(3.000000, "idle/0", 0, "R", "app", 20),
		sweepSwitchLine(2.000000, "app", 20, "S", "idle/0", 0),
		sweepSwitchLine(4.000000, "idle/0", 0, "R", "other", 30),
	)
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		AllowWindowedParse: true,
		LineStart:          2,
		LineEnd:            3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if idx.TimestampOrder != TraceTimestampOrderUnknown || idx.ClockRegressions == 0 {
		t.Fatalf("partial line scan must expose an unproven order plus its observed regression: order=%v regressions=%d", idx.TimestampOrder, idx.ClockRegressions)
	}
	stats := ComputeWindowStats(idx, Query{LineStart: 2, LineEnd: 3})
	if len(stats.CPU) != 0 || !containsSubstring(stats.Caveats, "scheduler_duration_fail_closed=true") {
		t.Fatalf("unknown complete-file order cannot hide a precise in-window lane rollback: %+v", stats)
	}
	streamed, err := StreamStateCluster(context.Background(), path, Query{PID: 20, LineStart: 2, LineEnd: 3}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if streamed.WindowStats == nil || len(streamed.WindowStats.TopRunning) != 0 || !containsSubstring(streamed.Caveats, "stream_state_cluster_fail_closed=true") {
		t.Fatalf("stream line-window durations did not fail closed: %+v", streamed.WindowStats)
	}
}

func TestIndexSchedulerHeadMemoIsBounded(t *testing.T) {
	idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic}
	for i := 1; i <= 8; i++ {
		idx.Events = append(idx.Events, Event{Type: EventSchedSwitch, Ts: float64(i), CPU: i % 2, NextPID: i, NextComm: "worker"})
	}
	for _, boundary := range []float64{2.5, 4.5, 6.5, 8.5} {
		if head := idx.schedulerHeadAt(boundary); head == nil || !head.Complete {
			t.Fatalf("head missing at %.1f", boundary)
		}
	}
	if len(idx.schedulerHeads) > indexSchedulerHeadMemoMaxEntries || idx.schedulerHeadBytes > indexSchedulerHeadMemoBudgetBytes {
		t.Fatalf("per-index scheduler head memo grew unbounded: entries=%d bytes=%d", len(idx.schedulerHeads), idx.schedulerHeadBytes)
	}
}

func TestWindowedSchedulerHeadBytesAreChargedToIndexLRU(t *testing.T) {
	idx := &Index{Windowed: true, Events: []Event{{Type: EventSchedSwitch}}}
	base := traceIndexCacheCost(idx)
	idx.setSchedulerHead(&schedulerHeadSnapshot{
		BoundaryTs: 1,
		Complete:   true,
		Threads: map[int]schedulerHeadThread{
			42: {Thread: ThreadRef{PID: 42, Comm: "worker"}, State: StateRunnable},
		},
		CPUs: map[int]schedulerHeadCPU{},
	})
	if idx.schedulerHeadBytes <= 0 {
		t.Fatal("fixture did not retain a scheduler head checkpoint")
	}
	if got, want := traceIndexCacheCost(idx)-base, idx.schedulerHeadBytes; got != want {
		t.Fatalf("global index LRU undercharged scheduler head bytes: got delta=%d want=%d", got, want)
	}
}
