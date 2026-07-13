package tracequery

// blocked_reason_census_test.go — 件1 census 根修 pins (修复轮, 2026-07-13;
// 复核实锤: the old model-face census read the top-8 DISPLAY truncation with
// per-(iowait,offset) bucket splits, so a symbol's count under-reported and
// callers beyond the top rows vanished; 冷读 E1-F1: only the top-1 caller
// ever reached the feed). The census now folds the FULL accumulator
// (INODE §28.6) into a per-pid per-caller 符号×count×Σms wire face.

import (
	"math"
	"testing"
)

// censusTrace builds a window with three semantic callers on ONE pid
// (tieba ThreadPoolForeg shape: fscache dominant + two hmfs callers), an
// opaque-caller row (unknown bucket), and a second pid.
func censusTrace(t *testing.T) *Index {
	t.Helper()
	rows := "" +
		"        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n" +
		"     worker-200 (200) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20\n" +
		// pid=200: 3× fscache (delay complete), offsets DIFFER so the raw
		// accumulator splits them into two buckets — the census must still
		// merge them under one symbol with count=3 (the E1-F1 root).
		"       peer-300 (300) [003] .... 3.010000: sched_blocked_reason: pid=200 iowait=1 caller=fscache_page_wait_o+0x110/0x250[sysmgr.elf] delay=100\n" +
		"       peer-300 (300) [003] .... 3.020000: sched_blocked_reason: pid=200 iowait=1 caller=fscache_page_wait_o+0x110/0x250[sysmgr.elf] delay=200\n" +
		"       peer-300 (300) [003] .... 3.030000: sched_blocked_reason: pid=200 iowait=1 caller=fscache_page_wait_o+0x9c/0x250[sysmgr.elf] delay=300\n" +
		// 1× hmfs_read (delay present) + 1× hmfs_get_dnode (NO delay field:
		// Σms must stay unpublished for it — 宁缺勿假).
		"       peer-300 (300) [003] .... 3.040000: sched_blocked_reason: pid=200 iowait=1 caller=hmfs_read+0x1c8/0x4a0[sysmgr.elf] delay=145\n" +
		"       peer-300 (300) [003] .... 3.050000: sched_blocked_reason: pid=200 iowait=0 caller=hmfs_get_dnode+0x390/0xd38[sysmgr.elf]\n" +
		// 1× opaque caller → the explicit unknown bucket (counts stay
		// inside the pid total).\n
		"       peer-300 (300) [003] .... 3.060000: sched_blocked_reason: pid=200 iowait=0 caller=0xdeadbeef delay=50\n" +
		// 件B (2026-07-13): a PARTIAL-delay caller — two rows, only one
		// carries delay= → count publishes, Σms must NOT (宁缺勿假: the
		// delayCount==count gate's discriminating case).
		"       peer-300 (300) [003] .... 3.062000: sched_blocked_reason: pid=200 iowait=0 caller=rwsem_down_read+0x30/0x60[sysmgr.elf] delay=77\n" +
		"       peer-300 (300) [003] .... 3.064000: sched_blocked_reason: pid=200 iowait=0 caller=rwsem_down_read+0x30/0x60[sysmgr.elf]\n" +
		// second pid with one marker.
		"       peer-300 (300) [003] .... 3.070000: sched_blocked_reason: pid=100 iowait=0 caller=dma_fence_default_wait+0x74/0x160[sysmgr.elf] delay=842\n" +
		"     worker-200 (200) [002] .... 3.119500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120\n" +
		"        app-100 (100) [001] .... 3.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n"
	return buildTraceIndex(t, "blocked_reason_census.systrace", rows)
}

// TestBlockedReasonCensus_PerCallerFullEnumeration — the wire census carries
// EVERY caller symbol of the pid (符号×count×Σms), offset buckets merged,
// per-caller counts summing to the pid total.
func TestBlockedReasonCensus_PerCallerFullEnumeration(t *testing.T) {
	idx := censusTrace(t)
	stats := ComputeWindowStats(idx, Query{TimeStart: 3.0, TimeEnd: 3.120, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	var row *BlockedReasonPIDCensus
	for i := range stats.BlockedReasonCensus {
		if stats.BlockedReasonCensus[i].Thread.PID == 200 {
			row = &stats.BlockedReasonCensus[i]
		}
	}
	if row == nil {
		t.Fatalf("pid=200 must carry a census row: %+v", stats.BlockedReasonCensus)
	}
	if row.Count != 8 {
		t.Fatalf("pid total must count every window marker, got %d", row.Count)
	}
	if len(row.Callers) != 5 || row.CallerOverflow != 0 {
		t.Fatalf("per-caller FULL enumeration expected (fscache/rwsem/hmfs_read/hmfs_get_dnode/unknown), got %+v overflow=%d", row.Callers, row.CallerOverflow)
	}
	sum := 0
	byName := map[string]BlockedReasonCensusCaller{}
	for _, c := range row.Callers {
		sum += c.Count
		byName[c.Caller] = c
	}
	if sum != row.Count {
		t.Fatalf("per-caller counts must sum to the pid total: %d != %d", sum, row.Count)
	}
	fscache := byName["fscache_page_wait_o"]
	if fscache.Count != 3 {
		t.Fatalf("offset buckets of one symbol must merge (count=3), got %+v", fscache)
	}
	if math.Abs(fscache.DelayTotalMs-0.600) > 1e-9 {
		t.Fatalf("delay-complete caller must publish Σms (0.600), got %+v", fscache)
	}
	if hr := byName["hmfs_read"]; hr.Count != 1 || math.Abs(hr.DelayTotalMs-0.145) > 1e-9 {
		t.Fatalf("hmfs_read ×1 Σ0.145ms expected, got %+v", hr)
	}
	if hd := byName["hmfs_get_dnode"]; hd.Count != 1 || hd.DelayTotalMs != 0 {
		t.Fatalf("a caller with no delay field must keep count and omit Σms, got %+v", hd)
	}
	if unk := byName["unknown"]; unk.Count != 1 {
		t.Fatalf("opaque rows keep their count under the explicit unknown bucket, got %+v", unk)
	}
	// 件B: PARTIAL delay coverage — count publishes, Σms never does (the
	// delayCount==count gate; a delayCount>0 relaxation must trip this).
	if rw := byName["rwsem_down_read"]; rw.Count != 2 || rw.DelayTotalMs != 0 {
		t.Fatalf("partial-delay caller must keep count=2 and omit Σms, got %+v", rw)
	}
	// The dominant caller sorts first (count desc).
	if row.Callers[0].Caller != "fscache_page_wait_o" {
		t.Fatalf("census caller order must be count desc, got %+v", row.Callers)
	}
	// The second pid keeps its own row (pid-keyed, never line-keyed).
	found100 := false
	for _, c := range stats.BlockedReasonCensus {
		if c.Thread.PID == 100 && c.Count == 1 && len(c.Callers) == 1 && c.Callers[0].Caller == "dma_fence_default_wait" {
			found100 = true
		}
	}
	if !found100 {
		t.Fatalf("pid=100 census row missing: %+v", stats.BlockedReasonCensus)
	}
}

// TestBlockedReasonCensus_CapsDiscloseOverflow — the pid and caller caps
// trim the published lists with explicit overflow counts (never silent).
func TestBlockedReasonCensus_CapsDiscloseOverflow(t *testing.T) {
	in := map[string]BlockedReasonSummary{}
	for pid := 1; pid <= 3; pid++ {
		for c := 0; c < 3; c++ {
			key := string(rune('a'+c)) + string(rune('0'+pid))
			in[key] = BlockedReasonSummary{
				Thread: ThreadRef{PID: pid, Comm: "t"},
				Reason: "sym_" + string(rune('a'+c)) + "+0x10[m.elf]",
				Count:  pid, // pid 3 dominates
				Line:   pid*10 + c,
			}
		}
	}
	census, pidOverflow := buildBlockedReasonCensus(in, 2, 2)
	if len(census) != 2 || pidOverflow != 1 {
		t.Fatalf("pid cap must trim with explicit overflow, got %d rows overflow=%d", len(census), pidOverflow)
	}
	if census[0].Thread.PID != 3 {
		t.Fatalf("census pid order must be count desc, got %+v", census[0])
	}
	if len(census[0].Callers) != 2 || census[0].CallerOverflow != 1 {
		t.Fatalf("caller cap must trim with explicit overflow, got %+v", census[0])
	}
	if census[0].Count != 9 {
		t.Fatalf("pid total must stay the FULL count past the caller cap, got %d", census[0].Count)
	}
}
