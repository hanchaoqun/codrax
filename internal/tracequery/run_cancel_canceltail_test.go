package tracequery

// run_cancel_canceltail_test.go — LT-HYG CANCEL-TAIL pins (§29.82 立案,
// 2026-07-14): the rank/frame_root_cause_bundle POST-FIRE tail. §29.82 件①/②
// stubbed the frame family and the stats sub-segments, but the integrity
// probes re-run by several builders of one Run (thread-incarnation lifecycle
// audit, scheduler order audit, binder pairing endpoint replay, scheduler
// head refinement, thread catalog) plus the bundle assembly chain itself
// stayed un-sampled — a canceled donghu rank/bundle Run still paid 24-45ms
// after the fire (measured; bundle with a pre-expired context at entry paid
// ~36ms).
//
// Pin shape (same as TestRunCancelStatsSubBuildersShortCircuitAfterFire): a
// control call must produce its verdict/value, the SAME call on an
// already-fired carrier must short-circuit empty — each arm is red if its
// gate/tick is removed (the fired run would then return the control value).
//
// 禁半账 note: every probe below feeds only faces that Run's attach gates
// discard whole once fired, so the empty short-circuit value can never be
// published as a false absence verdict.

import (
	"context"
	"fmt"
	"testing"
)

func cancelTailFiredQuery(t *testing.T, q Query) Query {
	t.Helper()
	dead, stop := context.WithCancel(context.Background())
	stop()
	fq := q.WithRunContext(dead)
	if !fq.runCancel.sample() {
		t.Fatal("dead context must fire at the boundary sample")
	}
	return fq
}

// ① thread-incarnation lifecycle audit: the fallback full scan short-circuits
// nil once fired (heaviest shared probe of the canceled-Run tail).
func TestCancelTailIncarnationProbeShortCircuitsAfterFire(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1.2, Type: EventSchedSwitch, CPU: 0, PrevPID: 101, PrevComm: "old-a", PrevState: "X", NextPID: 0},
		{Line: 2, Ts: 1.5, Type: EventSchedSwitch, CPU: 0, PrevPID: 0, NextPID: 101, NextComm: "new-a"},
	}, FirstTs: 1.2, LastTs: 1.5, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{TimeStart: 1.0, TimeEnd: 3.0}
	if conflict := threadIncarnationConflictForQuery(idx, q, 0); conflict == nil {
		t.Fatal("control: in-window generation cut must mint a conflict")
	}
	fq := cancelTailFiredQuery(t, q)
	if conflict := threadIncarnationConflictForQuery(idx, fq, 0); conflict != nil {
		t.Fatalf("fired: lifecycle audit must short-circuit nil, got %+v", conflict)
	}
}

// ② scheduler order audit: the fallback split-tracker replay short-circuits
// nil once fired.
func TestCancelTailSchedulerOrderProbeShortCircuitsAfterFire(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 2.0, Type: EventSchedSwitch, CPU: 0, PrevPID: 10, PrevComm: "a", PrevState: "S", NextPID: 11, NextComm: "b"},
		// Same CPU lane, timestamp regressed — an in-window order violation.
		{Line: 2, Ts: 1.5, Type: EventSchedSwitch, CPU: 0, PrevPID: 11, PrevComm: "b", PrevState: "S", NextPID: 10, NextComm: "a"},
	}, FirstTs: 1.5, LastTs: 2.0}
	q := Query{TimeStart: 1.0, TimeEnd: 3.0}
	if violation := schedulerStateOrderViolationForQuery(idx, q, 0); violation == nil {
		t.Fatal("control: lane timestamp regression must mint a violation")
	}
	fq := cancelTailFiredQuery(t, q)
	if violation := schedulerStateOrderViolationForQuery(idx, fq, 0); violation != nil {
		t.Fatalf("fired: order audit must short-circuit nil, got %+v", violation)
	}
}

// ③ binder pairing endpoint replay (the single heaviest sub-segment of the
// canceled bundle tail): the fired audit admits zero endpoints.
func TestCancelTailBinderPairingReplayShortCircuitsAfterFire(t *testing.T) {
	idx := buildTraceIndex(t, "canceltail-binder.systrace",
		"sender-10 (10) [001] .... 1.000000: binder_transaction: debug_id=42 dest_node=0 dest_proc=20 dest_thread=21 reply=0 flags=0x0 code=0x1\n"+
			"receiver-20 (20) [001] .... 1.001000: binder_transaction_received: transaction_id=42\n")
	q := Query{TimeStart: 0.5, TimeEnd: 2.0}
	if audit := auditBinderPairing(idx, q); audit.endpointCount == 0 {
		t.Fatal("control: the replay must admit the binder endpoints")
	}
	fq := cancelTailFiredQuery(t, q)
	if audit := auditBinderPairing(idx, fq); audit.endpointCount != 0 {
		t.Fatalf("fired: the replay must short-circuit before admitting endpoints, got %d", audit.endpointCount)
	}
}

// ④ scheduler head + thread catalog: both short-circuit empty once fired.
func TestCancelTailHeadAndCatalogShortCircuitAfterFire(t *testing.T) {
	idx := buildTraceIndex(t, "canceltail-head.systrace",
		"        app-100 (100) [000] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120\n"+
			"     worker-200 (200) [001] .... 2.000000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=000\n")
	q := Query{TimeStart: 1.5, TimeEnd: 3.0}
	if head := schedulerHeadForQuery(idx, q); head == nil {
		t.Fatal("control: full-index head checkpoint expected")
	}
	if catalog := buildThreadCatalog(idx, q); len(catalog) == 0 {
		t.Fatal("control: thread catalog entries expected")
	}
	fq := cancelTailFiredQuery(t, q)
	if head := schedulerHeadForQuery(idx, fq); head != nil {
		t.Fatalf("fired: head checkpoint must short-circuit nil, got %+v", head)
	}
	if catalog := buildThreadCatalog(idx, fq); len(catalog) != 0 {
		t.Fatalf("fired: thread catalog must short-circuit empty, got %+v", catalog)
	}
}

// ⑤ bundle assembly stage-gate family: a bundle entered after the fire
// returns the zero value instead of assembling every stage (whose faces the
// attach gate would discard anyway). Red when the fired-gate FAMILY inside
// BuildFrameRootCauseBundle is removed (verified: stripping all 7 gates reds
// this pin — the fired build then attaches a non-nil empty FrameTimeline
// pointer); any single surviving gate upstream of the timeline attach keeps
// the zero-value contract, which is exactly the guarded behavior.
func TestCancelTailBundleEntryGateShortCircuitsAfterFire(t *testing.T) {
	idx := buildTraceIndex(t, "canceltail-bundle.systrace",
		"        app-100 (100) [000] .... 1.000000: tracing_mark_write: B|100|doFrame\n"+
			"        app-100 (100) [000] .... 1.010000: tracing_mark_write: E|100\n"+
			"        app-100 (100) [000] .... 1.020000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120\n")
	q := Query{View: "frame_root_cause_bundle", PID: 100, TimeStart: 0.5, TimeEnd: 2.0}
	if bundle := BuildFrameRootCauseBundle(idx, q); bundle.FrameTimeline == nil {
		t.Fatal("control: the bundle must attach a frame timeline")
	}
	fq := cancelTailFiredQuery(t, q)
	if bundle := BuildFrameRootCauseBundle(idx, fq); bundle.FrameTimeline != nil {
		t.Fatal("fired: the bundle entry gate must return the zero value")
	}
}

// ⑥ exit-lane completeness caveats: derived completeness claims are minted
// from a full timeline; post-fire that timeline would be TRUNCATED (its scan
// loops tick-exit immediately), so the pass publishes nothing (半账禁) — the
// cancellation caveat already explains the run.
func TestCancelTailCompletenessCaveatsSuppressedAfterFire(t *testing.T) {
	idx := buildTraceIndex(t, "canceltail-caveat.systrace",
		"        app-100 (100) [000] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120\n"+
			"        app-100 (100) [000] .... 1.200000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n")
	q := Query{View: "wakeup_chain", PID: 100, TimeStart: 0.5, TimeEnd: 2.0, MinDurationMs: 1}
	res := Result{View: "wakeup_chain"}
	if got := traceCompletenessCaveats(idx, q, res); len(got) == 0 {
		t.Fatal("control: the wakeup-less sleep interval must mint a completeness caveat")
	}
	fq := cancelTailFiredQuery(t, q)
	if got := traceCompletenessCaveats(idx, fq, res); len(got) != 0 {
		t.Fatalf("fired: completeness claims from a truncated timeline must be suppressed, got %+v", got)
	}
}

// ⑦ contributor-scoped incarnation audit (LT-HYG §29.84 残留④, RSPA-HYG 残余批
// 2026-07-14): threadIncarnationConflictForPIDSet — the ForQuery twin's
// fallback full scan gained its gate/tick in ① while the perf_timeline /
// stats-facet-family twin stayed unsampled. Same contract: an armed-but-
// unfired carrier returns the byte-identical verdict of the nil carrier (the
// tick is a pure read on the untriggered path), a fired carrier short-circuits
// nil (its consumers' faces are attach-gate discarded whole, so the empty
// verdict is never published as a false absence).
func TestCancelTailIncarnationPIDSetProbeShortCircuitsAfterFire(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1.2, Type: EventSchedSwitch, CPU: 0, PrevPID: 101, PrevComm: "old-a", PrevState: "X", NextPID: 0},
		{Line: 2, Ts: 1.5, Type: EventSchedSwitch, CPU: 0, PrevPID: 0, NextPID: 101, NextComm: "new-a"},
	}, FirstTs: 1.2, LastTs: 1.5, TimestampOrder: TraceTimestampOrderMonotonic}
	q := Query{TimeStart: 1.0, TimeEnd: 3.0}
	pids := map[int]bool{101: true}
	control := threadIncarnationConflictForPIDSet(idx, q, pids)
	if control == nil {
		t.Fatal("control: in-window generation cut of a contributor must mint a conflict")
	}
	armed, stop := context.WithCancel(context.Background())
	defer stop()
	armedGot := threadIncarnationConflictForPIDSet(idx, q.WithRunContext(armed), pids)
	if armedGot == nil || fmt.Sprintf("%#v", *armedGot) != fmt.Sprintf("%#v", *control) {
		t.Fatalf("armed-but-unfired carrier must return the byte-identical verdict:\ncontrol: %#v\narmed:   %#v", control, armedGot)
	}
	fq := cancelTailFiredQuery(t, q)
	if conflict := threadIncarnationConflictForPIDSet(idx, fq, pids); conflict != nil {
		t.Fatalf("fired: contributor-scoped lifecycle audit must short-circuit nil, got %+v", conflict)
	}
}
