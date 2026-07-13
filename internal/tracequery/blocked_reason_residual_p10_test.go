package tracequery

// blocked_reason_residual_p10_test.go — CR-3 件② P10 engine pins (§29.42
// P10, docs/design/real_trace_campaign_20260705.md, 2026-07-12; 冷读案7
// GPU-fence witness: the root-cause row said 未解析 while the window held
// sched_blocked_reason records for the pid). The merged D/IO rank row
// mints the UNCONSUMED residual (window marker count + distinct semantic
// symbols) exactly when the unanimous-caller lane minted nothing; a row
// that consumed its marker never carries the residual (mutual exclusion).

import (
	"fmt"
	"strings"
	"testing"
)

// TestBlockedReasonResidualMintedOnPartialCoverage — two D segments, only
// one marked.
//
// EVOLUTION RECORD (§29.50.5 证明分区, v5 P1 批 件②, 2026-07-13): the
// original pin asserted the SINGLE merged seat surfaces the in-hand marker
// through the residual pair (the unanimous lane withheld the caller under
// partial coverage). The proof partition now CONSUMES that marker into its
// own cause seat — the 等待对象 word rides the seat — so the residual's
// purpose (a marker in hand never silently vanishes) is met by the cause
// seat itself, and the honest remainder must NOT re-disclose the consumed
// marker (双说防线: blockedReasonFullByPID counts every window marker
// including consumed ones; a remainder count would double-speak the
// sibling's word). The unmatched/opaque residual shapes stay pinned below.
func TestBlockedReasonResidualMintedOnPartialCoverage(t *testing.T) {
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
	if cause.BlockedReasonCaller != "dma_fence_default_wait" {
		t.Fatalf("the in-hand marker is consumed by the cause seat's word, got %q", cause.BlockedReasonCaller)
	}
	if cause.BlockedReasonWindowCount != 0 || cause.BlockedReasonWindowCaller != "" {
		t.Fatalf("a consumed marker must not double-surface as residual on the cause seat: %+v", *cause)
	}
	if remainder.BlockedReasonWindowCount != 0 || remainder.BlockedReasonWindowCaller != "" {
		t.Fatalf("the remainder must not re-disclose the sibling-consumed marker: %+v", *remainder)
	}
}

// TestBlockedReasonResidualAbsentWhenCallerConsumed — full coverage mints
// the unanimous caller; the residual pair must stay zero (the marker was
// consumed, nothing is outstanding).
func TestBlockedReasonResidualAbsentWhenCallerConsumed(t *testing.T) {
	idx := dstateRefineTrace(t,
		"       peer-300 (300) [003] .... 3.049500: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_wait+0x74/0x160[sysmgr.elf] delay=842\n")
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.120, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	row := dstateRefineFindMergedRow(t, rank)
	if row.BlockedReasonCaller != "dma_fence_default_wait" {
		t.Fatalf("fixture drifted: unanimous caller expected, got %q", row.BlockedReasonCaller)
	}
	if row.BlockedReasonWindowCount != 0 || row.BlockedReasonWindowCaller != "" {
		t.Fatalf("a consumed marker must not double-surface as residual: %+v", row)
	}
}

// TestBlockedReasonResidualAbsentWhenWindowHoldsNone — no markers in the
// window: nothing to disclose (absence never guesses).
func TestBlockedReasonResidualAbsentWhenWindowHoldsNone(t *testing.T) {
	idx := dstateRefineTrace(t, "")
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.120, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	row := dstateRefineFindMergedRow(t, rank)
	if row.BlockedReasonWindowCount != 0 || row.BlockedReasonWindowCaller != "" {
		t.Fatalf("an empty window must mint no residual: %+v", row)
	}
}

// TestBlockedReasonResidualOpaqueCallerCountOnly — a raw-address marker
// folds to "unknown": the count discloses, the symbol slot stays empty
// (never a synthesized identity).
func TestBlockedReasonResidualOpaqueCallerCountOnly(t *testing.T) {
	idx := dstateRefineTwoSegmentTrace(t,
		"       peer-300 (300) [003] .... 3.049500: sched_blocked_reason: pid=200 iowait=0 caller=0x69680100fffe0000 delay=842\n",
		"")
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.120, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	row := dstateRefineFindMergedRow(t, rank)
	if row.BlockedReasonWindowCount != 1 {
		t.Fatalf("the opaque marker still counts, got %d", row.BlockedReasonWindowCount)
	}
	if row.BlockedReasonWindowCaller != "" {
		t.Fatalf("an opaque caller must never surface a symbol, got %q", row.BlockedReasonWindowCaller)
	}
}

// TestBlockedReasonResidualCountsBeyondTopEightInventory — 修复轮 P2 (冷读
// 直核 2026-07-12: the ❺ residual said 17 while the window held 19 — the
// count had second-aggregated the top-8 truncated inventory). Fixture: the
// D-thread's three marker buckets (count 1 each) all fall BELOW eight
// larger other-pid buckets, so the truncated inventory holds ZERO rows for
// the pid; the residual must still count all three from the full
// pre-truncation accumulator (INODE §28.6 precedent) and carry the first
// two distinct semantic symbols.
func TestBlockedReasonResidualCountsBeyondTopEightInventory(t *testing.T) {
	var noise string
	for i := 0; i < 8; i++ {
		// Eight distinct (pid, reason) buckets with two markers each — they
		// own the entire top-8 inventory. Timestamps stay monotonic inside
		// the marker2 slot (3.0801xx, between the 3.080000 wakeup and the
		// 3.080500 switch) so the scheduler lane never sees a regression.
		for j := 0; j < 2; j++ {
			noise += fmt.Sprintf("       peer-300 (300) [003] .... 3.0801%d%d: sched_blocked_reason: pid=%d iowait=0 caller=noise_wait_%d+0x10/0x20 delay=10\n", i, j, 900+i, i)
		}
	}
	// EVOLUTION RECORD (§29.50.5 证明分区, v5 P1 批 件②, 2026-07-13): the
	// original fixture put both pid-200 markers ON D-segment endpoints with
	// conflicting callers — the proof partition now consumes those into two
	// cause seats, so the residual shape migrated to the still-reachable
	// UNMATCHED-marker form: both markers sit ~200µs past the segment-end
	// match tolerance (in-window, matched to NO segment), so no cause
	// partitions, the unanimous lane stays empty, and the residual must
	// still count them from the FULL pre-truncation accumulator (the
	// original intent, unweakened) with the symbol pair in (count, line)
	// order.
	idx := dstateRefineTwoSegmentTrace(t,
		"       peer-300 (300) [003] .... 3.049700: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_wait+0x74/0x160[sysmgr.elf] delay=842\n"+
			"       peer-300 (300) [003] .... 3.049800: sched_blocked_reason: pid=200 iowait=0 caller=hmfs_read+0x20/0x40 delay=11\n",
		noise)
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.120, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	row := dstateRefineFindMergedRow(t, rank)
	if row.BlockedReasonCaller != "" {
		t.Fatalf("fixture drifted: unmatched markers must leave the unanimous lane empty, got %q", row.BlockedReasonCaller)
	}
	// Fixture sanity: the pid-200 buckets must actually be OUTSIDE the
	// published top-8 inventory (each count 1 vs eight count-2 buckets).
	stats := ComputeWindowStats(idx, Query{TimeStart: 3.0, TimeEnd: 3.120})
	for _, br := range stats.BlockedReasons {
		if br.Thread.PID == 200 {
			t.Fatalf("fixture drifted: pid-200 bucket survived the top-8 truncation: %+v", stats.BlockedReasons)
		}
	}
	if row.BlockedReasonWindowCount != 2 {
		t.Fatalf("the residual must count the FULL accumulator (2 markers, both outside the cap), got %d", row.BlockedReasonWindowCount)
	}
	if row.BlockedReasonWindowCaller != "dma_fence_default_wait/hmfs_read" {
		t.Fatalf("the first two distinct symbols must ride in (count, line) order, got %q", row.BlockedReasonWindowCaller)
	}
}
