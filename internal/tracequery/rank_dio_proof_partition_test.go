package tracequery

// rank_dio_proof_partition_test.go — §29.50.5 证明分区式合计参赛 engine pins
// (user ruling 2026-07-12, real_trace_campaign_20260705.md §29.50.4/§29.50.5;
// v5 P1 批 件②, 2026-07-13).
//
// The D/IO participation aggregation key refines from (thread, state family)
// to (thread, state family, root-cause identity):
//   - 逐片段证明门: a fragment joins a CAUSE seat only when ITS OWN typed
//     sched_blocked_reason marker proves the concrete wait object (semantic
//     caller symbol — same symbol at different offsets is ONE wait object);
//   - 跨 token 合并: D fragments and iowait fragments proving the SAME wait
//     object merge into ONE cause seat (token 细分不得稀释真根因) — the
//     dma_fence D+iowait witness shape;
//   - 诚实余数: unproven fragments stay on the generic (thread, type) seat
//     wearing the typed cause-unproven-remainder marker (「D-state(原因未证)」
//     display form) — 绝不灌根因席 (P9 13× 教训: 无证归因即假叙事);
//   - 假设永不并: the runnable-lane hypothesis seats (调度压力 vs 反转) are
//     different type tokens and never touch this partition;
//   - §29.19 sum_disjoint 语义保全: every seat's account is the Σ of its own
//     disjoint fragments; Σ over the partition == the legacy single seat.
//
// MUTATION self-checks:
//   - dropping the partition (legacy single (thread) seat) reds
//     TestDIOProofPartitionSplitsProvenCauseFromRemainder;
//   - merging unproven fragments into the cause seat (灌根因席) reds the
//     cause-seat value assertion in the same test;
//   - dropping the cross-token merge reds
//     TestDIOProofPartitionMergesCrossTokenSameCause (via the same fixture);
//   - dropping the fold-key cause dimension (downstream re-merge) reds the
//     final-rank two-seat assertion;
//   - regressing the all-proven / no-marker single-seat shapes reds
//     TestDIOProofPartitionAllProvenSingleSeat /
//     TestDIOProofPartitionNoMarkersKeepsLegacySingleSeat.

import (
	"math"
	"strings"
	"testing"
)

// dioPartitionPartialTrace: worker-200 blocks in D three times on three cpus.
// seg1 (cpu2, 20ms) proves dma_fence_default_wait via iowait=0 marker; seg2
// (cpu3, 30ms) proves THE SAME symbol via an iowait=1 marker (cross-token);
// seg3 (cpu4, 10ms) carries no marker (unproven).
const dioPartitionPartialTrace = `
        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 3.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.030000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
       peer-300 (300) [003] .... 3.030000: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_wait+0x74/0x160[sysmgr.elf] delay=842
     worker-200 (200) [003] .... 3.031000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.040000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/3 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.070000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=004
       peer-300 (300) [003] .... 3.070000: sched_blocked_reason: pid=200 iowait=1 caller=dma_fence_default_wait+0x88/0x160[sysmgr.elf] delay=913
     worker-200 (200) [004] .... 3.071000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [004] .... 3.080000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/4 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.090000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=004
     worker-200 (200) [004] .... 3.091000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [004] .... 3.092000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/4 next_pid=0 next_prio=120
        app-100 (100) [001] .... 3.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

// dioPartitionTwoCausesTrace: seg1 (cpu2, 20ms) proves dma_fence_default_wait;
// seg2 (cpu3, 30ms) proves mmc_wait_data — two DISTINCT wait objects, both
// fully proven → two cause seats, no remainder.
const dioPartitionTwoCausesTrace = `
        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 3.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.030000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
       peer-300 (300) [003] .... 3.030000: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_wait+0x74/0x160[sysmgr.elf] delay=842
     worker-200 (200) [003] .... 3.031000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.040000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/3 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.070000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
       peer-300 (300) [003] .... 3.070000: sched_blocked_reason: pid=200 iowait=0 caller=mmc_wait_data+0x11c/0x2a0[kernel] delay=913
     worker-200 (200) [003] .... 3.071000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.072000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 3.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

// dioPartitionAllProvenTrace: both segments prove ONE symbol via iowait=0
// markers (the h2 CompThread unanimous shape over two cpu groups) — one seat,
// refined proof, caller, no remainder flag.
const dioPartitionAllProvenTrace = `
        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 3.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.030000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
       peer-300 (300) [003] .... 3.030000: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_wait+0x74/0x160[sysmgr.elf] delay=842
     worker-200 (200) [003] .... 3.031000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.040000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/3 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.070000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
       peer-300 (300) [003] .... 3.070000: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_wait+0x88/0x160[sysmgr.elf] delay=913
     worker-200 (200) [003] .... 3.071000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.072000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 3.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

func dioPartitionRank(t *testing.T, content string) RootCauseRankResult {
	t.Helper()
	idx := buildTraceIndex(t, "dio_partition.systrace", content)
	return BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.2,
		MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 16})
}

func dioPartitionSeats(rank RootCauseRankResult) []RootCauseRankItem {
	var seats []RootCauseRankItem
	for _, item := range rank.Items {
		if item.Thread.PID == 200 &&
			(item.Type == "d_state_or_io_wait" || item.Type == "io_wait") &&
			strings.HasPrefix(item.Source, "window_stats") {
			seats = append(seats, item)
		}
	}
	return seats
}

// TestDIOProofPartitionSplitsProvenCauseFromRemainder — the partial-proof
// form: the proven dma_fence fragments (D 20ms + iowait 30ms, cross-token)
// merge into ONE cause seat; the unproven 10ms fragment stays on the honest
// remainder seat wearing the typed marker; both seats SURVIVE the downstream
// family fold (the participation key's root-cause-identity dimension).
func TestDIOProofPartitionSplitsProvenCauseFromRemainder(t *testing.T) {
	rank := dioPartitionRank(t, dioPartitionPartialTrace)
	seats := dioPartitionSeats(rank)
	if len(seats) != 2 {
		t.Fatalf("partial proof must mint exactly cause seat + remainder seat, got %d: %+v", len(seats), seats)
	}
	var cause, remainder *RootCauseRankItem
	for i := range seats {
		if seats[i].BlockedReasonCaller != "" {
			cause = &seats[i]
		} else {
			remainder = &seats[i]
		}
	}
	if cause == nil || remainder == nil {
		t.Fatalf("expected one cause seat and one remainder seat, got %+v", seats)
	}
	if cause.BlockedReasonCaller != "dma_fence_default_wait" {
		t.Fatalf("cause seat must carry the proven wait object, got %q", cause.BlockedReasonCaller)
	}
	// 跨 token 合并: the cause seat holds the D fragment AND the iowait
	// fragment of the same wait object.
	if math.Abs(cause.CumulativeImpactMs-50.0) > 1e-6 ||
		math.Abs(cause.DStateMs-20.0) > 1e-6 || math.Abs(cause.IOWaitMs-30.0) > 1e-6 {
		t.Fatalf("cause seat must sum exactly its proven fragments (d=20 io=30), got total=%.6f d=%.6f io=%.6f",
			cause.CumulativeImpactMs, cause.DStateMs, cause.IOWaitMs)
	}
	if cause.DStateCauseUnprovenRemainder {
		t.Fatalf("cause seat must not wear the remainder marker: %+v", *cause)
	}
	// The mixed cause seat is NOT the refined pure-D form (io share nonzero).
	if cause.DStateAllNonIOProven {
		t.Fatalf("a mixed D+io cause seat must not claim the refined non-IO proof: %+v", *cause)
	}
	// 诚实余数: the unproven fragment stays generic and wears the marker.
	if math.Abs(remainder.CumulativeImpactMs-10.0) > 1e-6 || math.Abs(remainder.DStateMs-10.0) > 1e-6 {
		t.Fatalf("remainder seat must hold exactly the unproven fragment, got %+v", *remainder)
	}
	if !remainder.DStateCauseUnprovenRemainder {
		t.Fatalf("remainder seat must wear the typed cause-unproven marker: %+v", *remainder)
	}
	if remainder.BlockedReasonCaller != "" {
		t.Fatalf("remainder seat must never claim a cause (绝不灌根因席), got %q", remainder.BlockedReasonCaller)
	}
	// The sibling cause seat consumed the window markers — the P10 residual
	// disclosure must not double-speak them on the remainder.
	if remainder.BlockedReasonWindowCount != 0 {
		t.Fatalf("remainder beside a cause seat must not re-disclose consumed markers, got %d",
			remainder.BlockedReasonWindowCount)
	}
	// §29.19 sum_disjoint 保全: the partition preserves the thread account.
	if math.Abs((cause.CumulativeImpactMs+remainder.CumulativeImpactMs)-60.0) > 1e-6 {
		t.Fatalf("partition must preserve the thread total, got %.6f",
			cause.CumulativeImpactMs+remainder.CumulativeImpactMs)
	}
}

// TestDIOProofPartitionTwoCausesTwoSeats — two distinct proven wait objects
// mint two cause seats (each fully proven pure D → refined proof + caller),
// no remainder.
func TestDIOProofPartitionTwoCausesTwoSeats(t *testing.T) {
	rank := dioPartitionRank(t, dioPartitionTwoCausesTrace)
	seats := dioPartitionSeats(rank)
	if len(seats) != 2 {
		t.Fatalf("two proven causes must mint two cause seats, got %d: %+v", len(seats), seats)
	}
	byCaller := map[string]*RootCauseRankItem{}
	for i := range seats {
		byCaller[seats[i].BlockedReasonCaller] = &seats[i]
	}
	dma, mmc := byCaller["dma_fence_default_wait"], byCaller["mmc_wait_data"]
	if dma == nil || mmc == nil {
		t.Fatalf("expected dma_fence_default_wait and mmc_wait_data cause seats, got %+v", seats)
	}
	if math.Abs(dma.CumulativeImpactMs-20.0) > 1e-6 || math.Abs(mmc.CumulativeImpactMs-30.0) > 1e-6 {
		t.Fatalf("each cause seat must sum exactly its own fragments, got dma=%.6f mmc=%.6f",
			dma.CumulativeImpactMs, mmc.CumulativeImpactMs)
	}
	for _, seat := range []*RootCauseRankItem{dma, mmc} {
		if !seat.DStateAllNonIOProven {
			t.Fatalf("a fully-proven pure-D cause seat keeps the refined proof: %+v", *seat)
		}
		if seat.DStateCauseUnprovenRemainder {
			t.Fatalf("cause seats never wear the remainder marker: %+v", *seat)
		}
	}
}

// TestDIOProofPartitionAllProvenSingleSeat — the h2 CompThread unanimous
// shape: one wait object, full coverage → ONE seat byte-compatible with the
// legacy DSTATE-REFINE outcome (refined proof + caller, no marker).
func TestDIOProofPartitionAllProvenSingleSeat(t *testing.T) {
	rank := dioPartitionRank(t, dioPartitionAllProvenTrace)
	seats := dioPartitionSeats(rank)
	if len(seats) != 1 {
		t.Fatalf("an all-proven single cause must keep ONE seat, got %d: %+v", len(seats), seats)
	}
	seat := seats[0]
	if !seat.DStateAllNonIOProven || seat.BlockedReasonCaller != "dma_fence_default_wait" {
		t.Fatalf("the unanimous shape must keep proof + caller, got %+v", seat)
	}
	if seat.DStateCauseUnprovenRemainder {
		t.Fatalf("a single-partition seat never wears the remainder marker: %+v", seat)
	}
	if math.Abs(seat.CumulativeImpactMs-50.0) > 1e-6 || seat.MemberCount != 2 {
		t.Fatalf("the unanimous seat account drifted: %+v", seat)
	}
}

// TestDIOProofPartitionNoMarkersKeepsLegacySingleSeat — absence never
// partitions: a thread without any marker keeps today's single generic seat
// with no remainder marker (the case1 pure-D shape).
func TestDIOProofPartitionNoMarkersKeepsLegacySingleSeat(t *testing.T) {
	rank := dioPartitionRank(t, case1PureDTrace)
	seats := dioPartitionSeats(rank)
	if len(seats) != 1 {
		t.Fatalf("no markers must keep ONE generic seat, got %d: %+v", len(seats), seats)
	}
	seat := seats[0]
	if seat.DStateCauseUnprovenRemainder {
		t.Fatalf("a lone generic seat must not wear the remainder marker (no sibling cause seat): %+v", seat)
	}
	if seat.BlockedReasonCaller != "" || seat.DStateAllNonIOProven {
		t.Fatalf("no markers must not mint any proof, got %+v", seat)
	}
	if math.Abs(seat.CumulativeImpactMs-50.0) > 1e-6 || seat.MemberCount != 2 {
		t.Fatalf("legacy seat account drifted: %+v", seat)
	}
}

// TestDIOProofPartitionCase1ReconStaysSymmetric — the CASE-1 cross-lane
// absorption stays 1:1 under the partition: the chain lane mints per-ledger
// -group candidates keyed by the SAME cause partition, so the ×2 cause seat
// absorbs exactly its own two chain rows (the single-member remainder stays
// outside the family scope — MemberCount≥2 unchanged).
func TestDIOProofPartitionCase1ReconStaysSymmetric(t *testing.T) {
	idx := buildTraceIndex(t, "dio_partition_recon.systrace", dioPartitionPartialTrace)
	bundle := BuildFrameRootCauseBundle(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.2,
		MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 16})
	if bundle.RootCauseRank == nil || bundle.CriticalBlocking == nil {
		t.Fatal("bundle must carry both lanes")
	}
	var cause *RootCauseRankItem
	for i := range bundle.RootCauseRank.Items {
		item := &bundle.RootCauseRank.Items[i]
		if item.Thread.PID == 200 && item.BlockedReasonCaller == "dma_fence_default_wait" &&
			strings.HasPrefix(item.Source, "window_stats") {
			cause = item
		}
	}
	if cause == nil {
		t.Fatalf("no cause seat minted: %+v", bundle.RootCauseRank.Items)
	}
	if cause.MemberCount != 2 || len(cause.familyMemberIntervals) != 2 {
		t.Fatalf("cause seat must carry its 2-member inventory, got count=%d intervals=%d",
			cause.MemberCount, len(cause.familyMemberIntervals))
	}
	if cause.AbsorbedChainRows != 2 {
		t.Fatalf("the cause seat must absorb exactly its own two chain rows, got %d", cause.AbsorbedChainRows)
	}
	absorbed := 0
	for _, row := range bundle.CriticalBlocking.Items {
		if row.Thread.PID != 200 {
			continue
		}
		if row.AbsorbedByRankFamily {
			absorbed++
			if row.AbsorbedIntoFamily != cause.RankFamilyKey {
				t.Fatalf("absorbed row must join the cause seat, got %q vs %q",
					row.AbsorbedIntoFamily, cause.RankFamilyKey)
			}
		}
	}
	if absorbed != 2 {
		t.Fatalf("exactly the two proven chain rows absorb, got %d", absorbed)
	}
}
