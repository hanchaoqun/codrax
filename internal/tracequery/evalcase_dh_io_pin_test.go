package tracequery

// evalcase_dh_io_pin_test.go — EVALCASE-DH batch, IO / D-state family engine
// pins on the committed donghu.ftrace (mining ledger evalcase_donghu_mining.md
// §C; expectations re-collected at HEAD 1ada2c49f and hand-cross-checked).
//
// Cases:
//
//	DH-IO1 fscache 阻塞班车 + D 车道空转特异形 (窗 13762.795000..805000,
//	       target 17267): this trace's most distinctive physical fact — the
//	       main thread's fscache waits are reported as S sleeps (pid 17267
//	       has ZERO prev_state=D in the whole trace: R 3 / R+ 21 / S 65),
//	       so sched_blocked_reason is the ONLY witness lane. The census
//	       carries caller=fscache_page_get_an ×5 with Σdelay 1.469ms
//	       (hand check: raw delays 773+173+201+169+153 µs), the target has
//	       NO d_state seat, and the block pairing is complete (8/8).
//	DH-IO2 page-cache churn 回收风暴 (窗 13763.005000..024898): churn=2167
//	       on top inode 0x9903f, the host reclaim thread
//	       (sysmgr-reclaim0-9, tgid 2) owns the top delete stream (1515),
//	       the pressure source sh-19629 runs 19.402ms@cpu7, and the runnable
//	       victims include hilogcat-9503 (11.841ms@cpu9 — the largest
//	       single (thread,cpu) wait) — the DH-R1 tail-storm twin face.
//
// Fixture red line: real capture — every number is a measured pin.

import "testing"

// DH-IO1 — zero-D lane + blocked_reason census as the only witness.
func TestEvalcaseDHIO1FscacheZeroDLane(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	// Whole-trace structure fact: the target NEVER leaves the CPU in D.
	prevState := map[string]int{}
	for _, ev := range idx.Events {
		if ev.Type == EventSchedSwitch && ev.PrevPID == 17267 {
			prevState[ev.PrevState]++
		}
	}
	if prevState["D"] != 0 || prevState["S"] != 65 || prevState["R"] != 3 || prevState["R+"] != 21 {
		t.Fatalf("DH-IO1: zero-D structure fact drifted: %v (want D=0 S=65 R=3 R+=21)", prevState)
	}
	q := normalizeQuery(idx, Query{PID: 17267, TimeStart: 13762.795, TimeEnd: 13762.805})
	stats := ComputeWindowStats(idx, q)
	// The census lane: fscache_page_get_an ×5 Σdelay=1.469ms for the target.
	var target *BlockedReasonPIDCensus
	for i := range stats.BlockedReasonCensus {
		if stats.BlockedReasonCensus[i].Thread.PID == 17267 {
			target = &stats.BlockedReasonCensus[i]
		}
	}
	if target == nil {
		t.Fatalf("DH-IO1: target blocked_reason census row missing: %+v", stats.BlockedReasonCensus)
	}
	if target.Count != 5 || len(target.Callers) != 1 {
		t.Fatalf("DH-IO1: census shape drifted: %+v", target)
	}
	caller := target.Callers[0]
	// spec 踩点④ (evalcase_dh_spec.md): the fscache delay field's UNIT is
	// unproven on this trace physiology — the pin anchors the census (caller
	// identity + count) and deliberately NOT the delay magnitude. Cold-read
	// F1 (§29.137): the earlier DelayTotalMs≈1.469 assertion was an
	// undeclared spec deviation and is retired; the hand-sum record
	// (773+173+201+169+153 µs) stays here as an observation only.
	if caller.Caller != "fscache_page_get_an" || caller.Count != 5 {
		t.Fatalf("DH-IO1: fscache caller census drifted: %+v", caller)
	}
	// No fabricated D wall clock for the target — the D/IO ledgers must not
	// carry a 17267 seat in this window.
	for _, d := range stats.DStateTop {
		if d.Thread.PID == 17267 {
			t.Fatalf("DH-IO1: target must have no d_state seat (zero-D lane): %+v", d)
		}
	}
	for _, d := range stats.IOWaitTop {
		if d.Thread.PID == 17267 {
			t.Fatalf("DH-IO1: target must have no io_wait seat (fscache waits report as S): %+v", d)
		}
	}
	// Block pairing completeness: 8 issues, 8 completes.
	if stats.BlockIssueCount != 8 || stats.BlockCompleteCount != 8 {
		t.Fatalf("DH-IO1: block pairing drifted: issue=%d complete=%d (want 8/8)", stats.BlockIssueCount, stats.BlockCompleteCount)
	}
}

// DH-IO2 — reclaim churn storm with entity attribution.
func TestEvalcaseDHIO2PageCacheChurnStorm(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	q := normalizeQuery(idx, Query{PID: 24711, TimeStart: 13763.005, TimeEnd: 13763.024898})
	stats := ComputeWindowStats(idx, q)
	io := stats.IOPressureSummary
	if io == nil || io.Signal != "page_cache_churn" {
		t.Fatalf("DH-IO2: io signal drifted: %+v", io)
	}
	if io.PageCacheChurn != 2167 || io.TopInode != "0x9903f" || io.TopDev != "260:132" {
		t.Fatalf("DH-IO2: churn identity drifted: churn=%d inode=%s dev=%s", io.PageCacheChurn, io.TopInode, io.TopDev)
	}
	if !near(io.Score, 438.195, 0.001) {
		t.Fatalf("DH-IO2: churn score drifted: %.3f", io.Score)
	}
	if stats.MemoryEventCount != 2193 {
		t.Fatalf("DH-IO2: memory event census drifted: %d", stats.MemoryEventCount)
	}
	// Entity attribution: the host reclaim thread owns the top delete stream.
	if len(stats.PageCacheByInode) == 0 {
		t.Fatalf("DH-IO2: page cache inode ledger empty")
	}
	top := stats.PageCacheByInode[0]
	if top.Inode != "0x9903f" || top.Thread.Comm != "sysmgr-reclaim0" || top.Thread.PID != 9 || top.Thread.TGID != 2 {
		t.Fatalf("DH-IO2: reclaim entity drifted: %+v", top)
	}
	if top.Deletes != 1515 || top.Churn != 1515 {
		t.Fatalf("DH-IO2: top-inode delete stream drifted: deletes=%d churn=%d", top.Deletes, top.Churn)
	}
	// Pressure source and the largest single runnable victim.
	tr := stats.TopRunning[0]
	if tr.Thread.Comm != "sh" || tr.Thread.PID != 19629 || tr.CPU != 7 || !near(tr.DurationMs, 19.402, 0.001) {
		t.Fatalf("DH-IO2: pressure source seat drifted: %+v", tr)
	}
	rn := stats.RunnableTop[0]
	if rn.Thread.Comm != "hilogcat" || rn.Thread.PID != 9503 || rn.CPU != 9 || !near(rn.DurationMs, 11.841, 0.001) {
		t.Fatalf("DH-IO2: top runnable victim drifted: %+v", rn)
	}
}
