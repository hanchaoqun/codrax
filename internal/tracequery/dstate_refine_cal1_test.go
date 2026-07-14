package tracequery

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"
)

// dstate_refine_cal1_test.go — DSTATE-REFINE arm a engine pins (CAL-1 件③,
// ledger §29.39②/§29.47.2, 2026-07-12): the merged D/IO rank row mints the
// typed refined-D proof ONLY under blocked_reason 全覆盖∧全 iowait=0, and the
// unanimous semantic caller (dma_fence family — the donghu CompThread 12/12
// witness shape) rides beside it; an unmarked D segment keeps the honest
// merged identity (coverage absence never guesses).

func dstateRefineTrace(t *testing.T, marker string) *Index {
	t.Helper()
	rows := "" +
		"        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n" +
		"     worker-200 (200) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20\n" +
		"     worker-200 (200) [002] .... 3.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120\n" +
		// Engine-actual marker geometry (donghu witness lines 16223-16224):
		// the kernel emits sched_blocked_reason at the SAME timestamp as the
		// wakeup that ends the D segment — the interval lookup matches on the
		// shared endpoint (a strictly-later marker never matches).
		"       peer-300 (300) [003] .... 3.049500: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=002\n" +
		marker +
		"     worker-200 (200) [002] .... 3.050000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20\n" +
		"     worker-200 (200) [002] .... 3.119000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001\n" +
		"     worker-200 (200) [002] .... 3.119500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120\n" +
		"        app-100 (100) [001] .... 3.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n"
	return buildTraceIndex(t, "dstate_refine.systrace", rows)
}

func dstateRefineFindMergedRow(t *testing.T, rank RootCauseRankResult) RootCauseRankItem {
	t.Helper()
	for _, item := range rank.Items {
		// The refined proof mints at the window_stats D/IO fold site — the
		// wakeup_chain causal-impact TWIN of the same segments is a separate
		// lane (its wordface sync is the P2-3 display propagation).
		if item.Thread.PID == 200 && item.Type == "d_state_or_io_wait" &&
			strings.HasPrefix(item.Source, "window_stats") {
			return item
		}
	}
	t.Fatalf("fixture drifted: no merged window_stats D/IO row for worker-200: %+v", rank.Items)
	return RootCauseRankItem{}
}

// TestDStateRefineProofMintedUnderFullNonIOCoverage — the covered shape: the
// ONE D segment carries an iowait=0 marker with a semantic caller → the row
// mints the refined proof + the unanimous caller.
func TestDStateRefineProofMintedUnderFullNonIOCoverage(t *testing.T) {
	idx := dstateRefineTrace(t,
		"       peer-300 (300) [003] .... 3.049500: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_wait+0x74/0x160[sysmgr.elf] delay=842\n")
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.120, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	row := dstateRefineFindMergedRow(t, rank)
	if !row.DStateAllNonIOProven {
		t.Fatalf("full iowait=0 coverage must mint the refined-D proof: %+v", row)
	}
	if row.BlockedReasonCaller != "dma_fence_default_wait" {
		t.Fatalf("the unanimous semantic caller must ride the row, got %q", row.BlockedReasonCaller)
	}
	if row.IOWaitMs != 0 || row.DStateMs <= 0 {
		t.Fatalf("proof shape invariant drifted: %+v", row)
	}
}

// TestDStateRefineTrailingMarkerStillCovers — the donghu CompThread marker
// geometry (13762.793064 wakeup → .793065 marker, trailing ~1µs): the lookup
// widens by the ONE established wakeup-match tolerance (5µs), so the
// trailing marker still proves its own segment's coverage.
func TestDStateRefineTrailingMarkerStillCovers(t *testing.T) {
	idx := dstateRefineTrace(t,
		"       peer-300 (300) [003] .... 3.049501: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_w+0x260/0x4dc[devhost.elf] delay=3213\n")
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.120, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	row := dstateRefineFindMergedRow(t, rank)
	if !row.DStateAllNonIOProven || row.BlockedReasonCaller != "dma_fence_default_w" {
		t.Fatalf("a ≤5µs trailing marker must still prove coverage + caller, got proven=%v caller=%q", row.DStateAllNonIOProven, row.BlockedReasonCaller)
	}
}

// dstateRefineTwoSegmentTrace — 修复轮 P2-1 fixture: worker-200 blocks in D
// TWICE (3.010→3.0495 and 3.060→3.080); each wakeup optionally carries its
// own marker (engine-actual same-ts geometry).
func dstateRefineTwoSegmentTrace(t *testing.T, marker1, marker2 string) *Index {
	t.Helper()
	rows := "" +
		"        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n" +
		"     worker-200 (200) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20\n" +
		"     worker-200 (200) [002] .... 3.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120\n" +
		"       peer-300 (300) [003] .... 3.049500: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=002\n" +
		marker1 +
		"     worker-200 (200) [002] .... 3.050000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20\n" +
		"     worker-200 (200) [002] .... 3.060000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120\n" +
		"       peer-300 (300) [003] .... 3.080000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=002\n" +
		marker2 +
		"     worker-200 (200) [002] .... 3.080500: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20\n" +
		"     worker-200 (200) [002] .... 3.119000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001\n" +
		"     worker-200 (200) [002] .... 3.119500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120\n" +
		"        app-100 (100) [001] .... 3.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n"
	return buildTraceIndex(t, "dstate_refine_two.systrace", rows)
}

// TestDStateRefinePartialCoverageWithholdsProof — 修复轮 P2-1(a):
// TWO D segments, only ONE carries a marker.
//
// EVOLUTION RECORD (§29.50.5 证明分区, v5 P1 批 件②, 2026-07-13): the
// original pin asserted the SINGLE merged seat withholds proof + caller
// under partial coverage. The proof partition now carves the proven
// fragment into its own cause seat, so the assertion migrates WITHOUT
// weakening: the original intent — an unmarked fragment must never
// underwrite a proof claim — is enforced per fragment: the cause seat holds
// EXACTLY the marked fragment's account (never the thread total), and the
// unmarked fragment sits on the honest remainder with no proof, no caller,
// wearing the typed 原因未证 marker.
func TestDStateRefinePartialCoverageWithholdsProof(t *testing.T) {
	idx := dstateRefineTwoSegmentTrace(t,
		"       peer-300 (300) [003] .... 3.049500: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_wait+0x74/0x160[sysmgr.elf] delay=842\n",
		"")
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.120, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	var cause, remainder *RootCauseRankItem
	for i := range rank.Items {
		item := &rank.Items[i]
		if item.Thread.PID != 200 || item.Type != "d_state_or_io_wait" ||
			!strings.HasPrefix(item.Source, "window_stats") {
			continue
		}
		if item.BlockedReasonCaller != "" {
			cause = item
		} else {
			remainder = item
		}
	}
	if cause == nil || remainder == nil {
		t.Fatalf("partial coverage must partition into cause seat + remainder: %+v", rank.Items)
	}
	if cause.BlockedReasonCaller != "dma_fence_default_wait" || !cause.DStateAllNonIOProven {
		t.Fatalf("the cause seat carries exactly its proven fragment's proof: %+v", *cause)
	}
	if math.Abs(cause.CumulativeImpactMs-39.5) > 1e-6 {
		t.Fatalf("the cause seat must hold ONLY the marked fragment (39.5ms), got %.6f", cause.CumulativeImpactMs)
	}
	if remainder.DStateAllNonIOProven || remainder.BlockedReasonCaller != "" {
		t.Fatalf("the unmarked fragment must never underwrite a proof: %+v", *remainder)
	}
	if !remainder.DStateCauseUnprovenRemainder {
		t.Fatalf("the remainder must wear the typed 原因未证 marker: %+v", *remainder)
	}
	if math.Abs(remainder.CumulativeImpactMs-20.0) > 1e-6 {
		t.Fatalf("the remainder holds exactly the unmarked fragment (20ms), got %.6f", remainder.CumulativeImpactMs)
	}
}

// TestDStateRefineCallerConflictKeepsProofDropsCaller — 修复轮 P2-1(b): both
// segments marked iowait=0 but with DIFFERENT semantic callers.
//
// EVOLUTION RECORD (§29.50.5 证明分区, v5 P1 批 件②, 2026-07-13): the
// original pin asserted the single merged seat keeps the proof but withholds
// the 等待对象 word (no unanimous symbol). The partition now mints one seat
// per proven wait object; the original intent — never fabricate ONE symbol
// over conflicting fragments — migrates without weakening: no seat claims a
// symbol its own fragments did not prove, and each seat's account is exactly
// its own fragment's.
func TestDStateRefineCallerConflictKeepsProofDropsCaller(t *testing.T) {
	idx := dstateRefineTwoSegmentTrace(t,
		"       peer-300 (300) [003] .... 3.049500: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_wait+0x74/0x160[sysmgr.elf] delay=842\n",
		"       peer-300 (300) [003] .... 3.080000: sched_blocked_reason: pid=200 iowait=0 caller=kthread_worker_fn+0x14c/0x1ec[devhost.elf] delay=100\n")
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.120, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	byCaller := map[string]*RootCauseRankItem{}
	for i := range rank.Items {
		item := &rank.Items[i]
		if item.Thread.PID == 200 && item.Type == "d_state_or_io_wait" &&
			strings.HasPrefix(item.Source, "window_stats") {
			if _, dup := byCaller[item.BlockedReasonCaller]; dup {
				t.Fatalf("duplicate seat for caller %q: %+v", item.BlockedReasonCaller, rank.Items)
			}
			byCaller[item.BlockedReasonCaller] = item
		}
	}
	dma, kthread := byCaller["dma_fence_default_wait"], byCaller["kthread_worker_fn"]
	if dma == nil || kthread == nil || len(byCaller) != 2 {
		t.Fatalf("conflicting callers must mint one seat per proven wait object: %+v", rank.Items)
	}
	if math.Abs(dma.CumulativeImpactMs-39.5) > 1e-6 || math.Abs(kthread.CumulativeImpactMs-20.0) > 1e-6 {
		t.Fatalf("each seat holds exactly its own fragment, got dma=%.6f kthread=%.6f",
			dma.CumulativeImpactMs, kthread.CumulativeImpactMs)
	}
	for _, seat := range byCaller {
		if !seat.DStateAllNonIOProven {
			t.Fatalf("each fully-marked pure-D seat keeps the refined proof: %+v", *seat)
		}
		if seat.DStateCauseUnprovenRemainder {
			t.Fatalf("cause seats never wear the remainder marker: %+v", *seat)
		}
	}
}

// TestDStateRefineProofWithheldWithoutMarker — the uncovered shape: the same
// D segment WITHOUT any blocked_reason marker keeps the honest merged form
// (no proof, no caller — a markerless segment could still be IO wait).
func TestDStateRefineProofWithheldWithoutMarker(t *testing.T) {
	idx := dstateRefineTrace(t, "")
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.120, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	row := dstateRefineFindMergedRow(t, rank)
	if row.DStateAllNonIOProven {
		t.Fatalf("a markerless D segment must never mint the refined proof: %+v", row)
	}
	if row.BlockedReasonCaller != "" {
		t.Fatalf("no marker → no caller disclosure, got %q", row.BlockedReasonCaller)
	}
}

// TestDStateRefineHexCallerCollapsesToNoDisclosure — a pure-hex caller was
// collapsed to "unknown" at parse (blockedReasonSemanticCaller); the proof
// still mints (coverage is about the marker's presence + iowait flag) but no
// caller word is disclosed.
func TestDStateRefineHexCallerCollapsesToNoDisclosure(t *testing.T) {
	idx := dstateRefineTrace(t,
		"       peer-300 (300) [003] .... 3.049500: sched_blocked_reason: pid=200 iowait=0 caller=0x69680100fffe0000\n")
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.120, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	row := dstateRefineFindMergedRow(t, rank)
	if !row.DStateAllNonIOProven {
		t.Fatalf("an iowait=0 marker with an opaque caller still proves coverage: %+v", row)
	}
	if row.BlockedReasonCaller != "" {
		t.Fatalf("an opaque caller must not fabricate a 等待对象 word, got %q", row.BlockedReasonCaller)
	}
	if !strings.Contains(row.Summary, "d_state=") {
		t.Fatalf("summary account form drifted: %q", row.Summary)
	}
}

// TestDStateFamilySegmentTruthDonghuE8 — F-1 (冷读 P1, 2026-07-12), donghu
// E8 witness shape: CompThread_0-2955's window D account is 11 raw segments
// grouped into 4 per-CPU sums (16.064/10.424/6.495/3.774; true single-segment
// max 3.853ms). The published a–b range must be the TRUE segment range and
// multi-segment roster members must speak 「合计…(N段)」 — a group sum never
// masquerades as a 段. Skip-gated on the gold witness trace.
//
// EVOLUTION RECORD (RSPA §29.61.10a/b/c, matrix witness §2.1, 2026-07-14):
// the ❶ 36.757 seat re-anchored — only 3.598ms of the account intersects
// CompThread's typed wakeup-dependency jump windows (90.2% held no chain
// credential). The single 36.757 window seat is now the same-source
// bipartition: ⛓ anchored seat 3.598 (on-chain, roster/caller/segment-truth
// intact) + ◇ remainder seat 33.159 (adjacent, no chain credential), with
// 3.598 + 33.159 == 36.757 exactly (the ELIM-1 identity). The F-1 segment
// truths ride both halves unchanged.
func TestDStateFamilySegmentTruthDonghuE8(t *testing.T) {
	const donghuWitnessTracePath = "/Users/han/opt/donghu/donghu.ftrace"
	if _, err := os.Stat(donghuWitnessTracePath); err != nil {
		t.Skipf("witness trace unavailable: %v", err)
	}
	idx, err := BuildIndex(context.Background(), donghuWitnessTracePath)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	rank := BuildRootCauseRank(idx, Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	var row, remainder RootCauseRankItem
	found, foundRemainder := false, false
	for _, item := range rank.Items {
		if item.Thread.PID == 2955 && item.Type == "d_state_or_io_wait" && strings.HasPrefix(item.Source, "window_stats") {
			if item.ChainAnchorRemainderSeat {
				remainder, foundRemainder = item, true
				continue
			}
			row, found = item, true
		}
	}
	if !found {
		t.Fatalf("witness drifted: no CompThread window_stats D row: %+v", rank.Items)
	}
	if !foundRemainder {
		t.Fatalf("RSPA: the ◇ remainder half of the bipartition must stay visible: %+v", rank.Items)
	}
	// RSPA ⛓ seat: the anchored value is the published account; the full
	// window account rides the typed decomposition fields.
	if math.Abs(row.CumulativeImpactMs-3.598) > 0.002 || row.MemberCount != 4 {
		t.Fatalf("witness account drifted (anchored/groups): %+v", row)
	}
	if row.ChainRelevance != "on_chain" || math.Abs(row.ChainAnchoredMs-3.598) > 0.002 || math.Abs(row.ChainAnchorFullMs-36.757) > 0.002 {
		t.Fatalf("witness anchored decomposition drifted: %+v", row)
	}
	// ELIM-1 identity: 旧全窗席值 = ⛓锚定 + ◇余段 (同源二分, µs 容差).
	if remainder.ChainRelevance != "adjacent" ||
		math.Abs(row.CumulativeImpactMs+remainder.CumulativeImpactMs-36.757) > 0.002 ||
		math.Abs(remainder.ChainAnchorFullMs-36.757) > 0.002 {
		t.Fatalf("RSPA bipartition identity broken: anchored=%.3f remainder=%.3f (%+v)", row.CumulativeImpactMs, remainder.CumulativeImpactMs, remainder)
	}
	// TRUE single-segment range: max 3.853ms (11 raw segments), never the
	// 16.064 group sum the pre-fix N次(a~b) claimed as 单段.
	if math.Abs(row.MemberMaxMs-3.853) > 0.005 {
		t.Fatalf("MemberMaxMs must be the true single-segment max (~3.853), got %.3f", row.MemberMaxMs)
	}
	if row.MemberMinMs <= 0 || row.MemberMinMs > row.MemberMaxMs {
		t.Fatalf("MemberMinMs must be a true segment value, got %.3f", row.MemberMinMs)
	}
	// Multi-segment roster members speak the group-sum truth.
	joined := strings.Join(row.MemberRoster, "\n")
	if !strings.Contains(joined, "合计16.064ms(") || !strings.Contains(joined, "段)") {
		t.Fatalf("multi-segment roster member must speak 合计…(N段), got %q", joined)
	}
	// The refined-D proof + caller ride the same witness (12/12 iowait=0).
	if !row.DStateAllNonIOProven || row.BlockedReasonCaller != "dma_fence_default_w" {
		t.Fatalf("witness refined proof drifted: proven=%v caller=%q", row.DStateAllNonIOProven, row.BlockedReasonCaller)
	}
}
