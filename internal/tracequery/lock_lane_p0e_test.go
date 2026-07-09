package tracequery

import (
	"strings"
	"testing"
)

// P0-E 锁车道三修 engine pins (ledger §24.9-C, opendir_78/opendir_02 持锁翻转
// witness, 2026-07-09):
//
//	修1 fold 键补 waiter 身份 — two PHYSICALLY DIFFERENT contention spans (two
//	    different blocked threads on the same owner) never fold into a chimera
//	    (opendir: the target's 112.223ms victim row was swallowed whole by
//	    LegoHandler's 115.944ms span);
//	修2 持有者归因守护 — the payload '-->' hand-off chain is a typed witness
//	    (final holder ≠ whole-span holder; conservative envelope + disclosure,
//	    tenure boundaries are not recorded so segmentation would invent data),
//	    and the same-lock self-contradiction guard withdraws an INFERRED
//	    holder that was itself queued on the same lock for the majority of the
//	    attributed span (§12.3 未解析不准入 then keeps the row out of the
//	    direct lane and the 1.35 weight — unchanged machinery);
//	waiters=0 不设门 (裁定 adjudicated-no) — waiters counts the OTHER waiters
//	    already queued at emission; the emitting thread itself is blocked, so
//	    a waiters=0 row stays a legal candidate.
//
// MUTATION self-checks:
//   - removing the a.Thread.PID==b.Thread.PID key from sameLockContention reds
//     TestLockContentionFoldKeyRequiresSameWaiterP0E (the chimera returns);
//   - removing the Waiters-max port from foldLockContentionRow reds
//     TestLockContentionSameWaiterDualPrintStillFoldsP0E;
//   - deleting guardLockHolderSelfContradiction (or its Peer clear) reds
//     TestLockHolderSelfContradictionGuardP0E;
//   - dropping the OwnerHandoff parse reds TestLockContentionHandoffChainParsedP0E.

// opendirChimeraStats builds the opendir_78 lesion shape: two physically
// different waiters (LegoHandler-16865 and the target main thread 16547)
// blocked on the SAME payload owner tid 42067 (absent from the trace) over
// overlapping intervals.
func opendirChimeraStats() WindowStats {
	return WindowStats{TraceSpans: []TraceSpanSummary{
		{
			Name:    "monitor contention with owner #NetworkKit_GRS_GrsClient-Init_0 -->#NetworkKit_AssetsUtil_Operate_0 (42067) at void j.l.Object.wait()(Object.java:100) waiters=0 blocking from void a.b.C.d()(C.java:7)",
			Thread:  ThreadRef{Comm: "LegoHandler", PID: 16865, TGID: 16547},
			StartTs: 100.000, EndTs: 100.115944, DurationMs: 115.944,
			StartLine: 44100, EndLine: 79196,
		},
		{
			Name:    "monitor contention with owner #NetworkKit_AssetsUtil_Operate_0 (42067) at void j.l.Object.wait()(Object.java:100) waiters=1",
			Thread:  ThreadRef{Comm: "ugc.aweme.lite", PID: 16547, TGID: 16547},
			StartTs: 100.003, EndTs: 100.115226, DurationMs: 112.223,
			StartLine: 45696, EndLine: 79136,
		},
	}}
}

// opendirChimeraIndex supplies the closing wakeup edges: the main thread
// (16547) wakes LegoHandler inside its span (the relay hand-over wake), and
// RxComputationT-777 wakes the main thread inside its own span.
func opendirChimeraIndex(t *testing.T) *Index {
	return buildTraceIndex(t, "opendir_chimera.systrace", `
        rx-777 (16547) [003] .... 100.108000: sched_wakeup: comm=ugc.aweme.lite pid=16547 prio=110 target_cpu=001
        ugc.aweme.lite-16547 (16547) [001] .... 100.110000: sched_wakeup: comm=LegoHandler pid=16865 prio=120 target_cpu=002
        LegoHandler-16865 (16547) [002] .... 100.115900: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=LegoHandler next_pid=16865 next_prio=120
	`)
}

func TestLockContentionFoldKeyRequiresSameWaiterP0E(t *testing.T) {
	rows := collectBlockingSpanRows(opendirChimeraIndex(t), opendirChimeraStats())
	if len(rows) != 2 {
		t.Fatalf("修1: two physically different waiters must mint TWO rows (chimera fold forbidden), got %d: %+v", len(rows), rows)
	}
	waiters := map[int]bool{}
	for _, row := range rows {
		waiters[row.cand.Thread.PID] = true
	}
	if !waiters[16865] || !waiters[16547] {
		t.Fatalf("修1: the victim row (waiter 16547) must survive next to the LegoHandler row, got %+v", waiters)
	}
	// waiters=0 不设门: the waiters=0 row is still a legal candidate row.
	for _, row := range rows {
		if row.cand.Thread.PID == 16865 && row.cand.BlockingKind == "" {
			t.Fatalf("waiters=0 must not gate the contention row, got %+v", row.cand)
		}
	}
}

func TestLockContentionSameWaiterDualPrintStillFoldsP0E(t *testing.T) {
	stats := WindowStats{TraceSpans: []TraceSpanSummary{
		{
			Name:    "monitor contention with owner #WorkerA -->#WorkerB (42067) at void j.l.Object.wait()(Object.java:100) waiters=0",
			Thread:  ThreadRef{Comm: "LegoHandler", PID: 16865, TGID: 16547},
			StartTs: 100.000, EndTs: 100.115944, DurationMs: 115.944,
			StartLine: 44100, EndLine: 79196,
		},
		{
			Name:    "monitor contention with owner #WorkerB (42067) at void j.l.Object.wait()(Object.java:100) waiters=1",
			Thread:  ThreadRef{Comm: "LegoHandler", PID: 16865, TGID: 16547},
			StartTs: 100.001, EndTs: 100.110000, DurationMs: 109.0,
			StartLine: 44102, EndLine: 79190,
		},
	}}
	rows := collectBlockingSpanRows(opendirChimeraIndex(t), stats)
	if len(rows) != 1 {
		t.Fatalf("修1 positive control: the SAME waiter's dual print forms still fold, got %d rows", len(rows))
	}
	cand := rows[0].cand
	if cand.Waiters != 1 {
		t.Fatalf("修1 口径洞: the fold keeps the MAX waiters count, got %d", cand.Waiters)
	}
	if len(cand.HolderHandoff) != 2 || cand.HolderHandoff[0] != "WorkerA" || cand.HolderHandoff[1] != "WorkerB" {
		t.Fatalf("修2: the hand-off witness must survive the fold, got %v", cand.HolderHandoff)
	}
}

func TestLockContentionHandoffChainParsedP0E(t *testing.T) {
	info, ok := parseLockContentionPayload("monitor contention with owner #NetworkKit_GRS_GrsClient-Init_0 -->#NetworkKit_AssetsUtil_Operate_0 (42067) at void j.l.Object.wait()(Object.java:100) waiters=2")
	if !ok {
		t.Fatal("payload must parse")
	}
	if len(info.OwnerHandoff) != 2 ||
		info.OwnerHandoff[0] != "NetworkKit_GRS_GrsClient-Init_0" ||
		info.OwnerHandoff[1] != "NetworkKit_AssetsUtil_Operate_0" {
		t.Fatalf("修2: '-->' hand-off chain must parse as a typed witness, got %v", info.OwnerHandoff)
	}
	if info.Owner.Comm != "NetworkKit_AssetsUtil_Operate_0" || info.Owner.PID != 42067 {
		t.Fatalf("the FINAL holder stays the resolved owner (unchanged parse), got %+v", info.Owner)
	}
	single, ok := parseLockContentionPayload("monitor contention with owner #OnlyHolder (42067) at void j.l.Object.wait()(Object.java:100) waiters=1")
	if !ok || single.OwnerHandoff != nil {
		t.Fatalf("a single-owner payload records NO hand-off witness, got %v", single.OwnerHandoff)
	}
}

func TestLockHolderSelfContradictionGuardP0E(t *testing.T) {
	rows := collectBlockingSpanRows(opendirChimeraIndex(t), opendirChimeraStats())
	var lego, victim *blockingSpanRow
	for i := range rows {
		switch rows[i].cand.Thread.PID {
		case 16865:
			lego = &rows[i]
		case 16547:
			victim = &rows[i]
		}
	}
	if lego == nil || victim == nil {
		t.Fatalf("both rows expected, got %+v", rows)
	}
	// LegoHandler's closing waker is the main thread 16547 — but 16547 was
	// itself queued on owner 42067 for ~97%% of the span: the guard withdraws
	// the inference back to UNRESOLVED (§12.3 keeps it out of the direct lane
	// and the 1.35 weight automatically).
	if lego.cand.Peer.PID != 0 || lego.cand.HolderSource != "" {
		t.Fatalf("修2 guard: the self-contradicted holder must demote to unresolved, got peer=%+v source=%q",
			lego.cand.Peer, lego.cand.HolderSource)
	}
	if lego.cand.HolderSelfContradiction == "" ||
		!strings.Contains(lego.cand.HolderSelfContradiction, "42067") {
		t.Fatalf("修2 guard: the typed contradiction witness must name the shared owner, got %q",
			lego.cand.HolderSelfContradiction)
	}
	if !strings.Contains(lego.cand.Summary, "self-contradiction") {
		t.Fatalf("修2 guard: the summary must disclose the withdrawal, got %q", lego.cand.Summary)
	}
	if lego.cand.OwnerTidRaw != 42067 {
		t.Fatalf("the phantom payload tid stays on the audit field, got %d", lego.cand.OwnerTidRaw)
	}
	// The hand-off witness stays on the row (the '-->' chain came from the
	// payload, independent of the withdrawn inference).
	if len(lego.cand.HolderHandoff) != 2 {
		t.Fatalf("修2: the hand-off witness must survive the withdrawal, got %v", lego.cand.HolderHandoff)
	}
	if !strings.Contains(lego.cand.Summary, "hand-off chain") {
		t.Fatalf("修2: the hand-off disclosure must ride the summary, got %q", lego.cand.Summary)
	}
	// The victim row keeps ITS inference: its closing waker (rx-777) has no
	// same-lock contention span of its own — no contradiction, no withdrawal.
	if victim.cand.Peer.PID != 777 || victim.cand.HolderSource != CounterpartSourceWakeupEdge {
		t.Fatalf("victim row's own holder inference must survive, got peer=%+v source=%q",
			victim.cand.Peer, victim.cand.HolderSource)
	}
	if victim.cand.HolderSelfContradiction != "" {
		t.Fatalf("victim row must not carry a contradiction witness, got %q", victim.cand.HolderSelfContradiction)
	}

	// Rank face: the withdrawn row mints a waiter-subject row (never a
	// holder-subject seat), so the direct-lane admission pair reads false.
	item := rootCauseItemFromLockContentionCandidate(Query{}, map[int]bool{}, false, *lego)
	if item.SubjectIsLockHolder || item.Thread.PID != 16865 {
		t.Fatalf("withdrawn row must rank as the WAITER's unresolved contention, got subject=%+v holder=%v",
			item.Thread, item.SubjectIsLockHolder)
	}
	if item.HolderSelfContradiction == "" || len(item.HolderHandoff) != 2 {
		t.Fatalf("rank row must port both witnesses, got contradiction=%q handoff=%v",
			item.HolderSelfContradiction, item.HolderHandoff)
	}
}

// 复核收尾③b: the coverage threshold is INCLUSIVE — overlap of exactly 50%
// of the attributed span fires the guard (binary-exact ts values so the
// boundary comparison is not float-fuzzed).
func TestLockHolderSelfContradictionExactHalfBoundaryP0E(t *testing.T) {
	stats := WindowStats{TraceSpans: []TraceSpanSummary{
		{
			Name:   "monitor contention with owner #Ghost (42067) at void j.l.Object.wait()(Object.java:100) waiters=0",
			Thread: ThreadRef{Comm: "LegoHandler", PID: 16865, TGID: 16547},
			// span exactly 1000ms: 100.0 .. 101.0 (both binary-exact).
			StartTs: 100.0, EndTs: 101.0, DurationMs: 1000,
			StartLine: 44100, EndLine: 79196,
		},
		{
			Name:   "monitor contention with owner #Ghost (42067) at void j.l.Object.wait()(Object.java:100) waiters=1",
			Thread: ThreadRef{Comm: "ugc.aweme.lite", PID: 16547, TGID: 16547},
			// overlap [100.5, 101.0] = exactly 500ms = 50% of the span.
			StartTs: 100.5, EndTs: 101.5, DurationMs: 1000,
			StartLine: 45696, EndLine: 79300,
		},
	}}
	idx := buildTraceIndex(t, "p0e_half_boundary.systrace", `
        ugc.aweme.lite-16547 (16547) [001] .... 100.900000: sched_wakeup: comm=LegoHandler pid=16865 prio=120 target_cpu=002
        LegoHandler-16865 (16547) [002] .... 100.950000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=LegoHandler next_pid=16865 next_prio=120
	`)
	rows := collectBlockingSpanRows(idx, stats)
	for _, row := range rows {
		if row.cand.Thread.PID != 16865 {
			continue
		}
		if row.cand.HolderSelfContradiction == "" || row.cand.Peer.PID != 0 {
			t.Fatalf("恰 50%% 覆盖必须触发守护(阈值含等号), got peer=%+v witness=%q",
				row.cand.Peer, row.cand.HolderSelfContradiction)
		}
		return
	}
	t.Fatal("LegoHandler row missing")
}

// 复核收尾③c: the guard's scope is INFERRED holders only — a payload-DIRECT
// holder (rung ①: owner tid present in the trace, comm cross-check passed)
// passes through untouched even when a same-owner overlapping span exists
// (the identity is a direct payload claim, not a closing-wake inference).
func TestLockHolderPayloadDirectExemptFromGuardP0E(t *testing.T) {
	stats := WindowStats{TraceSpans: []TraceSpanSummary{
		{
			Name:    "monitor contention with owner #Worker (42067) at void j.l.Object.wait()(Object.java:100) waiters=0",
			Thread:  ThreadRef{Comm: "LegoHandler", PID: 16865, TGID: 16547},
			StartTs: 100.000, EndTs: 100.115944, DurationMs: 115.944,
			StartLine: 44100, EndLine: 79196,
		},
		{
			// Worker itself carries a same-owner overlapping contention span
			// (the shape that WOULD withdraw an inferred attribution).
			Name:    "monitor contention with owner #Worker (42067) at void j.l.Object.wait()(Object.java:100) waiters=1",
			Thread:  ThreadRef{Comm: "Worker", PID: 42067, TGID: 16547},
			StartTs: 100.003, EndTs: 100.115226, DurationMs: 112.223,
			StartLine: 45696, EndLine: 79136,
		},
	}}
	// Worker-42067 is PRESENT in this trace under the payload's comm → rung ①
	// payload-direct resolution for the LegoHandler row.
	idx := buildTraceIndex(t, "p0e_payload_direct.systrace", `
        Worker-42067 (16547) [003] .... 100.050000: sched_switch: prev_comm=Worker prev_pid=42067 prev_prio=120 prev_state=R ==> next_comm=idle/3 next_pid=0 next_prio=120
        ugc.aweme.lite-16547 (16547) [001] .... 100.110000: sched_wakeup: comm=LegoHandler pid=16865 prio=120 target_cpu=002
	`)
	rows := collectBlockingSpanRows(idx, stats)
	for _, row := range rows {
		if row.cand.Thread.PID != 16865 {
			continue
		}
		if row.cand.HolderSource != CounterpartSourceContentionPayload || row.cand.Peer.PID != 42067 {
			t.Fatalf("fixture drift: expected a payload-direct rung① resolution, got source=%q peer=%+v",
				row.cand.HolderSource, row.cand.Peer)
		}
		if row.cand.HolderSelfContradiction != "" {
			t.Fatalf("payload-direct holders are exempt from the guard, got %q", row.cand.HolderSelfContradiction)
		}
		return
	}
	t.Fatal("LegoHandler row missing")
}

// Negative arm: sub-majority overlap (a legitimate release-then-requeue
// hand-over adjacency) never withdraws the inference.
func TestLockHolderSelfContradictionSubMajorityKeepsInferenceP0E(t *testing.T) {
	stats := opendirChimeraStats()
	// Shrink the would-be contradicting span to ~26% coverage of the
	// LegoHandler span.
	stats.TraceSpans[1].StartTs = 100.085
	stats.TraceSpans[1].EndTs = 100.115226
	stats.TraceSpans[1].DurationMs = 30.226
	rows := collectBlockingSpanRows(opendirChimeraIndex(t), stats)
	for _, row := range rows {
		if row.cand.Thread.PID != 16865 {
			continue
		}
		if row.cand.Peer.PID != 16547 || row.cand.HolderSource != CounterpartSourceWakeupEdge {
			t.Fatalf("sub-majority overlap must keep the inference, got peer=%+v source=%q",
				row.cand.Peer, row.cand.HolderSource)
		}
		if row.cand.HolderSelfContradiction != "" {
			t.Fatalf("sub-majority overlap must not mint a contradiction witness, got %q", row.cand.HolderSelfContradiction)
		}
		return
	}
	t.Fatal("LegoHandler row missing")
}
