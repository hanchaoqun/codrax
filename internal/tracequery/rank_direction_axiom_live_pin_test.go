package tracequery

// rank_direction_axiom_live_pin_test.go — INTERSECT-FIX live regression pin
// (§29.143 INTERSECT-REG 归因, 2026-07-19) on the committed donghu.ftrace.
//
// The §29.136 CHAIN-BUDGET regression: the flagship 17267 board's io_latency
// single-segment row folded into its same-thread family (MemberCount>1 →
// single_segment_identity arm dead), and the family's exact member segments
// were refused wholesale by the 偏离④ familyMemberIntervals exclusion —
// basis="" → the seat silently left the AXIOM-V2 direction population and the
// board lost its ∩ cross-direction disclosure for six commits (h11 caught
// it). This pin replays the PRODUCTION chain (BuildRootCauseRank, engine
// defaults — the h11 eval lane's exact query shape) and pins the recovered
// live pair: the carrier is real (typed family inventory, interval_union
// caliber, union == published value to the µs) and the physical overlap is
// real. 突变职责: stripping the family_member_segment_intervals basis arm
// reverts the regression and turns this red.

import (
	"context"
	"testing"
)

func TestAXIOMV2IntersectLivePinDonghuFlagship(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Skipf("donghu fixture not present: %v", err)
	}
	// The h11 eval lane's root_cause_rank call: engine defaults, no
	// min-duration/limit/depth overrides.
	q := Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		TraceFlavorHint: TraceFlavorHarmonyHitrace}
	rank := BuildRootCauseRank(idx, q)

	approx := func(got, want float64) bool { return got-want <= 0.001 && got-want >= -0.001 }

	var running, ioFamily *RootCauseRankItem
	for i := range rank.Items {
		item := &rank.Items[i]
		if item.Thread.PID != 17267 || item.Rank <= 0 {
			continue
		}
		if item.Type == "running" && item.Rank == 1 {
			running = item
		}
		// The regression seat: the folded io_latency family (live shape at
		// re-base time: 4 members, interval_union, 3.670ms published).
		if item.Type == "io_latency" && item.MemberFoldCaliber == RootCauseMemberFoldCaliberIntervalUnion &&
			item.MemberCount > 1 && approx(item.ImpactMs, 3.670) {
			ioFamily = item
		}
	}
	if running == nil || ioFamily == nil {
		t.Fatalf("fixture drift: flagship board must hold the running r1 seat and the folded io_latency family (running=%v ioFamily=%v)", running != nil, ioFamily != nil)
	}

	// 当届段数: the folded family carries its exact member inventory and the
	// µs identity holds (union == published value — the 载体在 witness).
	if len(ioFamily.familyMemberIntervals) != 4 {
		t.Fatalf("当届形: the io_latency family carries 4 member segments, got %d", len(ioFamily.familyMemberIntervals))
	}
	unionMs, _ := foldIntervalUnionMs(ioFamily.familyMemberIntervals)
	if !approx(unionMs, ioFamily.ImpactMs) {
		t.Fatalf("µs identity: union %.6f vs published %.6f", unionMs, ioFamily.ImpactMs)
	}
	if ioFamily.directionSupportBasis != RootCauseDirectionBasisFamilySegments {
		t.Fatalf("the family seat must enter through the family-segments basis, got %q", ioFamily.directionSupportBasis)
	}

	// The recovered ∩ pair (both-or-neither): the running seat discloses the
	// family partner (0.114ms live), the family seat mirrors it.
	foundOnRunning := false
	for _, entry := range running.CrossDirectionOverlaps {
		if entry.Basis == RootCauseDirectionBasisFamilySegments && entry.Direction == "io_dependency" &&
			approx(entry.OverlapMs, 0.114) {
			foundOnRunning = true
		}
	}
	if !foundOnRunning {
		t.Fatalf("回归 pin: the running seat must disclose the io_latency family pair (0.114ms, family basis): %+v", running.CrossDirectionOverlaps)
	}
	foundOnFamily := false
	for _, entry := range ioFamily.CrossDirectionOverlaps {
		if entry.Basis == RootCauseDirectionBasisSelfRunning && entry.Direction == "frequency_thermal" &&
			approx(entry.OverlapMs, 0.114) {
			foundOnFamily = true
		}
	}
	if !foundOnFamily {
		t.Fatalf("互指成对: the family seat must mirror the running pair: %+v", ioFamily.CrossDirectionOverlaps)
	}

	// 值/序数零动 live face: the disclosure lane rides published values that
	// stay the pre-fix board's (running eff 58.320 / family 3.670).
	if !approx(running.EffectiveImpactMs, 58.320) || !approx(ioFamily.EffectiveImpactMs, 3.670) {
		t.Fatalf("value channels drifted: running eff %.6f family eff %.6f", running.EffectiveImpactMs, ioFamily.EffectiveImpactMs)
	}
}
