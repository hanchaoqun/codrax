package tracequery

// blocked_reason_census_test.go — 件1 census 根修 pins (修复轮, 2026-07-13;
// 复核实锤: the old model-face census read the top-8 DISPLAY truncation with
// per-(iowait,offset) bucket splits, so a symbol's count under-reported and
// callers beyond the top rows vanished; 冷读 E1-F1: only the top-1 caller
// ever reached the feed). The census now folds the FULL accumulator
// (INODE §28.6) into a per-pid per-caller 符号×count×Σms wire face.

import (
	"math"
	"reflect"
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
	census, pidOverflow := buildBlockedReasonCensus(in, 0, 2, 2)
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

// --- G4-ENGINE (2026-07-20): target-aware census admission ------------------

// g4CensusAccumulator builds a busy-background accumulator: pids 10..10+n-1
// with descending high counts, plus one low-count target pid=7 (count=3,
// the c2 shape: the focused thread's own row dies to the pid cap).
func g4CensusAccumulator(n int) map[string]BlockedReasonSummary {
	in := map[string]BlockedReasonSummary{}
	for i := 0; i < n; i++ {
		pid := 10 + i
		in[string(rune('A'+i))] = BlockedReasonSummary{
			Thread: ThreadRef{PID: pid, Comm: "bg"},
			Reason: "mmc_wait_for_req_do+0x10[m.elf]",
			Count:  100 - i,
			Line:   pid,
		}
	}
	in["target"] = BlockedReasonSummary{
		Thread: ThreadRef{PID: 7, Comm: "tieba"},
		Reason: "sync_buffer_read_wi+0x60/0x11c[sysmgr.elf]",
		Count:  3,
		Line:   2,
	}
	return in
}

// TestBlockedReasonCensus_TargetAdmissionBeyondCap — the analysis target's
// own row (count=3, out-competed by 4 background pids under pidCap=4) is
// admitted by evicting the lowest-count tail row; ordering stays count desc,
// the row is the verbatim full-accumulator fold, and the overflow COUNT is
// conserved (target leaves the overflow set, the evicted tail joins it).
func TestBlockedReasonCensus_TargetAdmissionBeyondCap(t *testing.T) {
	in := g4CensusAccumulator(5) // pids 10..14, counts 100..96; target pid=7 count=3
	census, overflow := buildBlockedReasonCensus(in, 7, 4, 8)
	if len(census) != 4 || overflow != 2 {
		t.Fatalf("cap size and conserved overflow expected (4 rows, overflow=2), got %d rows overflow=%d", len(census), overflow)
	}
	last := census[len(census)-1]
	if last.Thread.PID != 7 || last.Count != 3 {
		t.Fatalf("target row must be admitted at its count-sorted position with its verbatim count, got %+v", census)
	}
	if len(last.Callers) != 1 || last.Callers[0].Caller != "sync_buffer_read_wi" || last.Callers[0].Count != 3 {
		t.Fatalf("target row must carry its own caller account verbatim, got %+v", last.Callers)
	}
	for i := 0; i+1 < len(census); i++ {
		if census[i].Count < census[i+1].Count {
			t.Fatalf("census must stay count-desc sorted after admission: %+v", census)
		}
	}
	// 返工 P2-A (2026-07-20, 双官同抓): the admission's TRUE victim is the
	// lowest-count SURVIVOR of the plain cap — pid 13 (count 97), the tail of
	// kept=[10,11,12,13]. pid 14 (count 96) was already a plain-cap casualty
	// BEFORE the admission arm ran, so asserting only its absence let a
	// head-evicting mutant (pids[0]=targetPID) pass the whole battery. The
	// three busier survivors must stay seated and exactly pid 13 must yield.
	present := map[int]bool{}
	for _, row := range census {
		present[row.Thread.PID] = true
	}
	if present[13] {
		t.Fatalf("admission must evict the lowest-count SURVIVOR (pid 13), got %+v", census)
	}
	for _, pid := range []int{10, 11, 12} {
		if !present[pid] {
			t.Fatalf("admission must never evict a busier survivor (pid %d missing): %+v", pid, census)
		}
	}
	if present[14] {
		t.Fatalf("the plain-cap casualty (pid 14) must stay out: %+v", census)
	}
}

// TestBlockedReasonCensus_TargetAbsentAdmitsNothing — a targetPID with NO
// row in the full accumulator must never mint one (宁漏勿假指: admission is
// row selection, not row fabrication) and the published set stays the plain
// top-N byte-for-byte.
func TestBlockedReasonCensus_TargetAbsentAdmitsNothing(t *testing.T) {
	in := g4CensusAccumulator(5)
	delete(in, "target")
	withTarget, overflowWith := buildBlockedReasonCensus(in, 7, 4, 8)
	plain, overflowPlain := buildBlockedReasonCensus(in, 0, 4, 8)
	// 返工 P3-① (2026-07-20): the byte-identical claim is asserted as full
	// structural equality (名实相符), not a field sample.
	if overflowWith != overflowPlain || !reflect.DeepEqual(withTarget, plain) {
		t.Fatalf("absent target must keep the census identical: %+v/%d vs %+v/%d", withTarget, overflowWith, plain, overflowPlain)
	}
	for _, row := range withTarget {
		if row.Thread.PID == 7 {
			t.Fatalf("no accumulator row for the target — nothing may be fabricated: %+v", withTarget)
		}
	}
}

// TestBlockedReasonCensus_TargetInsideCapByteIdentical — a target that
// already survives the cap (and the no-cap / zero-target shapes) keeps the
// selection byte-identical to the pre-G4 top-N.
func TestBlockedReasonCensus_TargetInsideCapByteIdentical(t *testing.T) {
	in := g4CensusAccumulator(3) // 3 bg pids + target = 4 ≤ cap when pidCap=8
	uncapped, overflowUncapped := buildBlockedReasonCensus(in, 7, 8, 8)
	plainUncapped, overflowPlain := buildBlockedReasonCensus(in, 0, 8, 8)
	// 返工 P3-① (2026-07-20): byte-identical asserted as reflect.DeepEqual.
	if overflowUncapped != overflowPlain || !reflect.DeepEqual(uncapped, plainUncapped) {
		t.Fatalf("uncapped shapes must be identical: %+v/%d vs %+v/%d", uncapped, overflowUncapped, plainUncapped, overflowPlain)
	}
	// Target inside the capped top-N: admission arm must not re-order or
	// evict anything.
	inTop := g4CensusAccumulator(5)
	inTop["target"] = BlockedReasonSummary{
		Thread: ThreadRef{PID: 7, Comm: "tieba"},
		Reason: "sync_buffer_read_wi+0x60/0x11c[sysmgr.elf]",
		Count:  99, // slots between the 100..96 background rows
		Line:   2,
	}
	capped, overflowCapped := buildBlockedReasonCensus(inTop, 7, 4, 8)
	plainCapped, overflowPlainCapped := buildBlockedReasonCensus(inTop, 0, 4, 8)
	if overflowCapped != overflowPlainCapped || !reflect.DeepEqual(capped, plainCapped) {
		t.Fatalf("target already inside the cap must keep the census identical: %+v/%d vs %+v/%d", capped, overflowCapped, plainCapped, overflowPlainCapped)
	}
}
