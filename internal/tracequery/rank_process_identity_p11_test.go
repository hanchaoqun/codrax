package tracequery

// rank_process_identity_p11_test.go — CR-3 件③ P11 engine pins (§29.42
// P11, 2026-07-12; 冷读案8 关键角色裸线程名无 tgid): every rank row is
// stamped with its process attribution — the TGID the trace's second
// column published (catalog backfill) plus the owning process comm (the
// catalog's tgid==tid main-thread entry). Verbatim catalog facts only.

import (
	"strings"
	"testing"
)

func rankProcessIdentityTrace(t *testing.T) *Index {
	t.Helper()
	rows := "" +
		"        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n" +
		// worker-200 belongs to process 100 (second column = tgid 100).
		"     worker-200 (100) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20\n" +
		"     worker-200 (100) [002] .... 3.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120\n" +
		"       peer-300 (300) [003] .... 3.049500: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=002\n" +
		"     worker-200 (100) [002] .... 3.050000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20\n" +
		"     worker-200 (100) [002] .... 3.119000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001\n" +
		"     worker-200 (100) [002] .... 3.119500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120\n" +
		"        app-100 (100) [001] .... 3.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n"
	return buildTraceIndex(t, "rank_process_identity.systrace", rows)
}

// TestRankProcessIdentityStamped — the D-state seat for worker-200 carries
// its trace-published tgid and the owning process comm (app, the tgid==tid
// main thread).
func TestRankProcessIdentityStamped(t *testing.T) {
	idx := rankProcessIdentityTrace(t)
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.120, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	var found bool
	for _, item := range rank.Items {
		if item.Thread.PID != 200 || !strings.HasPrefix(item.Source, "window_stats") {
			continue
		}
		found = true
		if item.Thread.TGID != 100 {
			t.Fatalf("the trace-published tgid must ride the seat, got %+v", item.Thread)
		}
		if item.ProcessComm != "app" {
			t.Fatalf("the owning process comm must resolve from the catalog main thread, got %q", item.ProcessComm)
		}
	}
	if !found {
		t.Fatalf("fixture drifted: no window_stats seat for worker-200: %+v", rank.Items)
	}
}

// TestRankProcessIdentityMainThreadSelfOwned — a tgid==tid main-thread row
// resolves its own comm as the process comm (the honest identity case).
func TestRankProcessIdentityMainThreadSelfOwned(t *testing.T) {
	idx := dstateRefineTrace(t, "")
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.120, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	row := dstateRefineFindMergedRow(t, rank)
	if row.Thread.TGID != 200 || row.ProcessComm != "worker" {
		t.Fatalf("a self-owned main thread resolves its own comm as process comm, got tgid=%d comm=%q", row.Thread.TGID, row.ProcessComm)
	}
}

// TestRankProcessIdentityFrameBundleLane — 首放实证回归: the frame-bundle
// rank lane (BuildFrameRootCauseBundle) bypasses BuildRootCauseRank, and a
// lane-local stamp left its rows tgid-resolved but comm-less. The stamp
// now rides the ONE shared finalize tail, so every lane ships the process
// identity.
func TestRankProcessIdentityFrameBundleLane(t *testing.T) {
	idx := rankProcessIdentityTrace(t)
	bundle := BuildFrameRootCauseBundle(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.120, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	if bundle.RootCauseRank == nil {
		t.Fatalf("fixture drifted: no rank on the frame bundle")
	}
	var found bool
	for _, item := range bundle.RootCauseRank.Items {
		if item.Thread.PID != 200 {
			continue
		}
		found = true
		if item.Thread.TGID != 100 || item.ProcessComm != "app" {
			t.Fatalf("frame-bundle lane must ship the process identity, got tgid=%d comm=%q", item.Thread.TGID, item.ProcessComm)
		}
	}
	if !found {
		t.Fatalf("fixture drifted: no worker-200 row on the frame-bundle rank: %+v", bundle.RootCauseRank.Items)
	}
}

// TestRankProcessIdentityCommUnresolvable — 修复轮 P4 (donghu tgid=9581
// 实证形): the trace publishes a tgid whose main thread (tid==tgid) never
// appears in the window — the seat keeps the verbatim tgid and an EMPTY
// process comm (the thread's own comm never substitutes).
func TestRankProcessIdentityCommUnresolvable(t *testing.T) {
	rows := "" +
		"        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n" +
		// worker-200 belongs to process 999 — tid 999 itself never appears.
		"     worker-200 (999) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20\n" +
		"     worker-200 (999) [002] .... 3.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120\n" +
		"       peer-300 (300) [003] .... 3.049500: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=002\n" +
		"     worker-200 (999) [002] .... 3.050000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20\n" +
		"     worker-200 (999) [002] .... 3.119000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001\n" +
		"     worker-200 (999) [002] .... 3.119500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120\n" +
		"        app-100 (100) [001] .... 3.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n"
	idx := buildTraceIndex(t, "rank_process_identity_orphan.systrace", rows)
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.120, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	var found bool
	for _, item := range rank.Items {
		if item.Thread.PID != 200 || !strings.HasPrefix(item.Source, "window_stats") {
			continue
		}
		found = true
		if item.Thread.TGID != 999 {
			t.Fatalf("the verbatim tgid must survive, got %+v", item.Thread)
		}
		if item.ProcessComm != "" {
			t.Fatalf("an unresolvable process comm must stay empty, got %q", item.ProcessComm)
		}
	}
	if !found {
		t.Fatalf("fixture drifted: no window_stats seat for worker-200: %+v", rank.Items)
	}
}
