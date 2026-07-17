package tracequery

import (
	"reflect"
	"strings"
	"testing"
)

// Wave-1 ENG audit pins (ledger real_trace_campaign_20260705.md §29.25 裁定②
// 处置委托, 2026-07-10). Each test below was written RED against the audited
// defect before the fix landed; keep them as behavior pins.

// Audit #42 (P1): threadIncarnationConflictForQuery consumed tracker.observe,
// which returns only conflicts[0] of observeAll while observeAll permanently
// deletes the lifecycle evidence (t.dead) for EVERY conflict it generated.
// One physical row yielding two conflicts with diverging window relevance
// (PriorDeadTs pre-window vs in-window) masked the relevant conflict forever:
// every later global identity guard returned nil and PID-keyed aggregates
// merged two task incarnations with zero caveat. The fallback now mirrors the
// threadIncarnationConflictForPIDSet shape: iterate observeAll and test window
// relevance per conflict.
func TestIncarnationGuardOneRowTwoConflictsWindowRelevanceNotMasked(t *testing.T) {
	idx := &Index{Events: []Event{
		// TID 101 dies BEFORE the query window.
		{Line: 1, Ts: 0.5, Type: EventSchedSwitch, CPU: 0, PrevPID: 101, PrevComm: "old-a", PrevState: "X", NextPID: 0},
		// TID 102 dies INSIDE the query window.
		{Line: 2, Ts: 1.5, Type: EventSchedSwitch, CPU: 0, PrevPID: 102, PrevComm: "old-b", PrevState: "X", NextPID: 0},
		// One physical row where BOTH dead TIDs reappear. Role order puts the
		// pre-window (irrelevant) conflict for 101 first; the in-window
		// (relevant) conflict for 102 must not be dropped with it.
		{Line: 3, Ts: 2.0, Type: EventSchedSwitch, CPU: 0, PID: 101, PrevPID: 101, PrevComm: "new-a", PrevState: "R", NextPID: 102, NextComm: "new-b"},
	}, FirstTs: 0.5, LastTs: 2.0, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{TimeStart: 1.0, TimeEnd: 3.0}

	conflict := threadIncarnationConflictForQuery(idx, q, 0)
	if conflict == nil || conflict.PID != 102 || conflict.Signal != "reappeared_after_dead" {
		t.Fatalf("window-relevant conflict on one row was masked by the irrelevant sibling conflict: %+v", conflict)
	}
	stats := ComputeWindowStats(idx, q)
	if !containsSubstring(stats.Caveats, "thread_identity_fail_closed=true") {
		t.Fatalf("global scheduler aggregates must fail closed on the masked-conflict shape: %+v", stats.Caveats)
	}
}

// Audit #42 control: the masking fix must not widen the guard — a conflict
// whose lifecycle boundary is entirely pre-window stays irrelevant.
func TestIncarnationGuardPreWindowConflictStaysIrrelevant(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 0.4, Type: EventSchedSwitch, CPU: 0, PrevPID: 101, PrevComm: "old-a", PrevState: "X", NextPID: 0},
		{Line: 2, Ts: 0.6, Type: EventSchedSwitch, CPU: 0, PrevPID: 0, NextPID: 101, NextComm: "new-a"},
		{Line: 3, Ts: 2.0, Type: EventSchedSwitch, CPU: 0, PrevPID: 0, NextPID: 101, NextComm: "new-a"},
	}, FirstTs: 0.4, LastTs: 2.0, TimestampOrder: TraceTimestampOrderMonotonic}
	if conflict := threadIncarnationConflictForQuery(idx, Query{TimeStart: 1.0, TimeEnd: 3.0}, 0); conflict != nil {
		t.Fatalf("fully pre-window generation cut must stay outside the selected duration domain: %+v", conflict)
	}
}

// Audit #43 (P2): the per-query scheduler-order fallback ran ONE combined
// tracker whose observe() early-returns on the first lane violation and skips
// the remaining lane updates for that row — the exact CPU-masks-TID shape the
// split-tracker audit comment documents. An irrelevant violation (rows after
// q.TimeEnd) silently corrupted lane state and masked a later genuine
// in-window same-lane rollback, so scheduler durations published without the
// fail-close this gate exists to guarantee. The fallback now runs the same
// split-tracker complete audit as schedulerOrderFailuresFromEvents.
func TestSchedulerOrderFallbackSplitTrackersDoNotMaskInWindowRollback(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 80, Type: EventSchedSwitch, CPU: 0, PrevPID: 0, NextPID: 7, NextComm: "p"},
		{Line: 2, Ts: 105, Type: EventSchedSwitch, CPU: 1, PrevPID: 0, NextPID: 9, NextComm: "q"},
		// CPU-lane violation (105→100) is irrelevant to TimeEnd=98, but in the
		// combined tracker its early return skipped P's lane update to 100.
		{Line: 3, Ts: 100, Type: EventSchedSwitch, CPU: 1, PrevPID: 9, NextPID: 7, NextComm: "p"},
		// P's lane truly rolls back 100→95 inside the window.
		{Line: 4, Ts: 95, Type: EventSchedSwitch, CPU: 0, PrevPID: 7, PrevState: "S", NextPID: 0},
	}, FirstTs: 80, LastTs: 105}
	q := Query{TimeEnd: 98}

	violation := schedulerStateOrderViolationForQuery(idx, q, 0)
	if violation == nil || violation.Lane != "tid" || violation.ID != 7 {
		t.Fatalf("in-window TID rollback was masked by an irrelevant CPU violation on an earlier row: %+v", violation)
	}
	stats := ComputeWindowStats(idx, q)
	if !containsSubstring(stats.Caveats, "scheduler_duration_fail_closed=true") {
		t.Fatalf("scheduler durations must fail closed over a proven lane rollback: %+v", stats.Caveats)
	}
}

// Audit #43 control: a clean monotonic stream still returns no violation from
// the fallback (the split-tracker audit must not manufacture rollbacks).
func TestSchedulerOrderFallbackCleanStreamStaysNil(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1.0, Type: EventSchedSwitch, CPU: 0, PrevPID: 0, NextPID: 7, NextComm: "p"},
		{Line: 2, Ts: 1.1, Type: EventSchedSwitch, CPU: 1, PrevPID: 0, NextPID: 9, NextComm: "q"},
		{Line: 3, Ts: 1.2, Type: EventSchedSwitch, CPU: 0, PrevPID: 7, PrevState: "S", NextPID: 0},
	}, FirstTs: 1.0, LastTs: 1.2, TimestampOrder: TraceTimestampOrderMonotonic}
	if violation := schedulerStateOrderViolationForQuery(idx, Query{TimeEnd: 2}, 0); violation != nil {
		t.Fatalf("clean stream must not fail closed: %+v", violation)
	}
}

// Audit #44 (P2): ProcessDomainCensus was still published while the same
// WindowStats result declared a thread-incarnation conflict — the census
// thread count and catalog TGID/comm attribution can seat a reused TID's new
// task in the old task's process domain, exactly the incarnation mixing the
// conflict proved. The census now fails closed with every other PID-keyed
// face, and the identity caveat names it.
func TestProcessDomainCensusFailsClosedUnderIncarnationConflict(t *testing.T) {
	idx := buildTraceIndex(t, "census_identity_conflict.systrace", `
 app-20 (20) [001] .... 1.100000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=100
 app-20 (20) [001] .... 1.150000: sched_switch: prev_comm=app prev_pid=20 prev_prio=100 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
 victim-30 (30) [002] .... 1.300000: sched_switch: prev_comm=victim prev_pid=30 prev_prio=100 prev_state=X ==> next_comm=idle/2 next_pid=0 next_prio=120
 idle-0 (0) [002] .... 1.400000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=reborn next_pid=30 next_prio=100
`)
	q := Query{PID: 20, TimeStart: 1.0, TimeEnd: 1.5}
	stats := ComputeWindowStats(idx, q)
	if !containsSubstring(stats.Caveats, "thread_identity_fail_closed=true") {
		t.Fatalf("fixture must produce an in-window incarnation conflict: %+v", stats.Caveats)
	}
	if stats.ProcessDomainCensus != nil {
		t.Fatalf("process-domain census published under a declared identity conflict: %+v", stats.ProcessDomainCensus)
	}
	if !containsSubstring(stats.Caveats, "process-domain census") {
		t.Fatalf("identity caveat must disclose the census face it withholds: %+v", stats.Caveats)
	}
}

// Audit #44 control: without a conflict the census face stays published.
func TestProcessDomainCensusPublishesWithoutConflict(t *testing.T) {
	idx := buildTraceIndex(t, "census_clean.systrace", `
 app-20 (20) [001] .... 1.100000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=100
 app-20 (20) [001] .... 1.150000: sched_switch: prev_comm=app prev_pid=20 prev_prio=100 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`)
	stats := ComputeWindowStats(idx, Query{PID: 20, TimeStart: 1.0, TimeEnd: 1.5})
	if stats.ProcessDomainCensus == nil || stats.ProcessDomainCensus.ThreadCount == 0 {
		t.Fatalf("clean window must keep the census face: %+v", stats.ProcessDomainCensus)
	}
}

// Audit #45 (P3): the scheduler head snapshot stored PriorityClass classified
// with a hardcoded TraceFlavorGenericFtrace — dead-but-wrong data on Harmony
// traces (prio 100 stored as raw_scheduler_prio instead of ohos_rt) and a
// latent trap for any future consumer reading the stored field instead of
// reclassifying from Priority with the query flavor (which every consumer
// does). The flavor-sensitive field is removed from the head structs; this
// reflection pin keeps it out.
func TestSchedulerHeadSnapshotStoresNoFlavorSensitivePriorityClass(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(schedulerHeadThread{}), reflect.TypeOf(schedulerHeadCPU{})} {
		if _, ok := typ.FieldByName("PriorityClass"); ok {
			t.Fatalf("%s must not store a flavor-sensitive priority class; classify from Priority with the query flavor at read time", typ.Name())
		}
		if _, ok := typ.FieldByName("Priority"); !ok {
			t.Fatalf("%s must keep the raw Priority the consumers reclassify from", typ.Name())
		}
	}
}

// Audit #46 (P3), a2 refinement: Query.PID is an exact TID selector. Neither
// the Event envelope PID nor the typed TGID process identity may widen it to
// every worker in that process.
func TestPerfTimelineQueryPIDIsTIDOnlyNotTGIDOrEnvelope(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1.10, Type: EventPerfSample, CPU: 0, PID: 100, Comm: "worker-a",
			PerfFields: &PerfFields{TID: 101, PID: 100, Comm: "worker-a", Period: 1, EventName: "cpu-cycles", Symbol: "a"}},
		{Line: 2, Ts: 1.20, Type: EventPerfSample, CPU: 1, PID: 100, Comm: "worker-b",
			PerfFields: &PerfFields{TID: 102, PID: 100, Comm: "worker-b", Period: 1, EventName: "cpu-cycles", Symbol: "b"}},
	}, FirstTs: 1.1, LastTs: 1.3, TimestampOrder: TraceTimestampOrderMonotonic}
	if res := BuildPerfTimeline(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.5}); len(res.Buckets) != 0 {
		t.Fatalf("TGID/envelope PID widened an exact TID selector: %+v", res)
	}
	res := BuildPerfTimeline(idx, Query{PID: 101, TimeStart: 1.0, TimeEnd: 1.5})
	total := 0
	for _, bucket := range res.Buckets {
		total += bucket.SampleCount
	}
	if total != 1 || len(res.Buckets[0].ThreadIdentities) != 1 || res.Buckets[0].ThreadIdentities[0].TID != 101 {
		t.Fatalf("exact TID selector did not retain precisely one worker: %+v", res)
	}
}

// Audit #46 control: the shared admission predicate keeps the thread-selector
// arm — a Thread-addressed timeline must not admit other threads' samples.
func TestPerfTimelineThreadSelectorScopesSamples(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1.10, Type: EventPerfSample, CPU: 0, PID: 200, Comm: "render",
			PerfFields: &PerfFields{TID: 200, Comm: "render", Period: 1, EventName: "cpu-cycles", Symbol: "draw"}},
		{Line: 2, Ts: 1.20, Type: EventPerfSample, CPU: 1, PID: 300, Comm: "other",
			PerfFields: &PerfFields{TID: 300, Comm: "other", Period: 1, EventName: "cpu-cycles", Symbol: "noise"}},
	}, FirstTs: 1.1, LastTs: 1.2, TimestampOrder: TraceTimestampOrderMonotonic}
	res := BuildPerfTimeline(idx, Query{Thread: "render", TimeStart: 1.0, TimeEnd: 1.5})
	total := 0
	for _, bucket := range res.Buckets {
		total += bucket.SampleCount
	}
	if total != 1 {
		t.Fatalf("thread-addressed timeline admitted foreign samples: %+v", res.Buckets)
	}
}

// Audit #46 control: the subject force-add must not fail closed a clean
// pid-addressed timeline.
func TestPerfTimelinePIDSelectorStillPublishesWithoutConflict(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1.30, Type: EventPerfSample, CPU: 0, PID: 20, Comm: "app",
			PerfFields: &PerfFields{TID: 77, PID: 20, Period: 1, EventName: "cpu-cycles", Symbol: "hot"}},
	}, FirstTs: 1.3, LastTs: 1.3, TimestampOrder: TraceTimestampOrderMonotonic}
	res := BuildPerfTimeline(idx, Query{PID: 77, TimeStart: 1.0, TimeEnd: 1.5})
	if len(res.Buckets) == 0 {
		t.Fatalf("clean pid-addressed timeline must publish: %+v", res)
	}
}

// Audit #47 (P3): schedulerFieldsValidationFailure's addPID accepted any
// strconv-parseable value as a valid identity token, so a parseable NEGATIVE
// pid passed the hard integrity gate with nothing recorded — and every
// downstream consumer skips pid<=0 roles, silently unbooking that transition.
// A parseable pid < 0 is now missing_or_invalid; explicit pid 0 stays valid
// (idle).
func TestSchedulerRowNegativePIDFailsIntegrityGate(t *testing.T) {
	cases := []struct {
		name, line, field string
	}{
		{
			name:  "switch_negative_prev_pid",
			line:  ` app-20 (20) [000] .... 1.100000: sched_switch: prev_comm=app prev_pid=-1 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
			field: "prev_pid",
		},
		{
			name:  "switch_negative_next_pid",
			line:  ` app-20 (20) [000] .... 1.100000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=ghost next_pid=-7 next_prio=120`,
			field: "next_pid",
		},
		{
			name:  "wakeup_negative_pid",
			line:  ` waker-10 (10) [001] .... 1.100000: sched_wakeup: comm=app pid=-5 prio=20 target_cpu=000`,
			field: "pid",
		},
		{
			name:  "migrate_negative_pid",
			line:  ` app-20 (20) [000] .... 1.100000: sched_migrate_task: comm=app pid=-2 prio=20 orig_cpu=0 dest_cpu=1`,
			field: "pid",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failure := schedulerRowValidationFailure(2, tc.line)
			if failure == nil {
				t.Fatalf("parseable negative pid must be reported missing_or_invalid, not accepted as identity")
			}
			found := false
			for _, field := range failure.Fields {
				if field == tc.field {
					found = true
				}
			}
			if !found || !failure.AffectsAllPIDs {
				t.Fatalf("negative pid failure must name %q and affect all PIDs: %+v", tc.field, failure)
			}
		})
	}

	// Explicit zero remains a valid identity token (idle).
	valid := ` idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`
	if failure := schedulerRowValidationFailure(1, valid); failure != nil {
		t.Fatalf("explicit pid 0 must remain valid: %+v", failure)
	}
}

// Audit #47 end-to-end: the corrupt row makes scheduler durations fail closed.
func TestSchedulerNegativePIDRowFailsDurationsClosed(t *testing.T) {
	idx := buildTraceIndex(t, "negative_pid.systrace", strings.Join(schedulerIntegrityFixture(
		` app-20 (20) [000] .... 1.100000: sched_switch: prev_comm=app prev_pid=-1 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
	), "\n"))
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.4})
	if !containsSubstring(stats.Caveats, "scheduler_duration_fail_closed=true") {
		t.Fatalf("negative-pid scheduler row must fail durations closed: %+v", stats.Caveats)
	}
}
