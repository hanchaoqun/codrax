package tracequery

// rank_direction_axiom_live_pin_test.go — INTERSECT-FIX live regression pin
// (§29.143 INTERSECT-REG 归因, 2026-07-19) on the committed donghu.ftrace,
// re-based by INTERFLOOR-1 (user ruling §29.150③, 2026-07-19) to carry BOTH
// arms of the relative de-minimis floor:
//
// INTERSECT-REG history: the §29.136 CHAIN-BUDGET regression folded the
// flagship 17267 board's io_latency single-segment row into its same-thread
// family and the 偏离④ familyMemberIntervals exclusion refused the family's
// exact member segments wholesale — basis="" → the seat silently left the
// AXIOM-V2 direction population and the board lost its ∩ disclosure for six
// commits (h11 caught it). INTERSECT-FIX re-admitted the exact-inventory
// family shape (family_member_segment_intervals, µs-identity gated); the
// recovered live pair was running × io_latency family = 0.114ms.
//
// INTERFLOOR-1 re-base: 0.114ms is 3.1% of the smaller seat's published eff
// (3.670ms) — one of the very live forms the user judged 「极小…算作噪音」 —
// so the pair now DEMOTES to the cross_direction_overlap_undisclosed typed
// token lane (negative arm: no roster entry, sentence face dead). The
// mechanism guard does NOT weaken: the population/basis machinery must still
// resolve the family inventory (INTERSECT-REG would still be caught — a
// basis-arm regression erases the typed undisclosed record too), and the
// SAME board's second live pair (running × the 2-member io family 0.941ms,
// overlap 0.116ms = 12.3% of the smaller seat) stays ABOVE the floor and
// must keep disclosing (significant-keep arm — an over-eager floor turns
// this red). 突变职责: stripping the family_member_segment_intervals basis
// arm kills the undisclosed record AND the kept pair (regression
// reproduces); flipping the de-minimis comparison kills the kept pair;
// dropping the demote branch resurrects the 0.114 roster entry.

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

	var running, ioFamily, ioSmall *RootCauseRankItem
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
		// The significant-keep partner: the second folded io family (live
		// shape: 0.941ms published, overlap with running 0.116ms = 12.3% of
		// this smaller seat — above the de-minimis floor).
		if item.Type == "io_latency" && item.MemberCount > 1 && approx(item.ImpactMs, 0.941) {
			ioSmall = item
		}
	}
	if running == nil || ioFamily == nil || ioSmall == nil {
		t.Fatalf("fixture drift: flagship board must hold the running r1 seat and both folded io_latency families (running=%v ioFamily=%v ioSmall=%v)",
			running != nil, ioFamily != nil, ioSmall != nil)
	}

	// 当届段数: the folded family carries its exact member inventory and the
	// µs identity holds (union == published value — the 载体在 witness; the
	// INTERSECT-REG day-one guard, unweakened by the floor re-base).
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

	// ── INTERFLOOR-1 negative arm (tiny demote): the physically-real 0.114ms
	// pair (3.1% of the 3.670 seat) mints NO roster entry on either side —
	// the sentence/chip face is silent — while the typed undisclosed token
	// stays on BOTH sides (宁降不删; a basis regression erases this too, so
	// INTERSECT-REG stays caught).
	for _, entry := range running.CrossDirectionOverlaps {
		if entry.Basis == RootCauseDirectionBasisFamilySegments && approx(entry.OverlapMs, 0.114) {
			t.Fatalf("de-minimis 降道: the 0.114ms pair must not mint a roster entry: %+v", running.CrossDirectionOverlaps)
		}
	}
	if len(ioFamily.CrossDirectionOverlaps) != 0 {
		t.Fatalf("de-minimis 降道: the 3.670 family's roster must be empty, got %+v", ioFamily.CrossDirectionOverlaps)
	}
	hasToken := func(tokens []string, want string) bool {
		for _, tok := range tokens {
			if tok == want {
				return true
			}
		}
		return false
	}
	if !hasToken(running.CrossDirectionOverlapUndisclosed, "io_latency") {
		t.Fatalf("undisclosed 记号在案 (running side): %v", running.CrossDirectionOverlapUndisclosed)
	}
	if !hasToken(ioFamily.CrossDirectionOverlapUndisclosed, "running") {
		t.Fatalf("undisclosed 记号在案 (family side): %v", ioFamily.CrossDirectionOverlapUndisclosed)
	}

	// ── INTERFLOOR-1 significant-keep arm: the SAME board's 0.116ms pair
	// (12.3% of the 0.941 seat) stays above the relative floor and keeps the
	// full mutual disclosure (both-or-neither) — an over-eager floor or a
	// flipped comparison turns this red.
	foundOnRunning := false
	for _, entry := range running.CrossDirectionOverlaps {
		if entry.Basis == RootCauseDirectionBasisFamilySegments && entry.Direction == "io_dependency" &&
			approx(entry.OverlapMs, 0.116) {
			foundOnRunning = true
		}
	}
	if !foundOnRunning {
		t.Fatalf("significant-keep 正臂: the running seat must keep disclosing the 0.116ms family pair: %+v", running.CrossDirectionOverlaps)
	}
	foundOnSmall := false
	for _, entry := range ioSmall.CrossDirectionOverlaps {
		if entry.Basis == RootCauseDirectionBasisSelfRunning && entry.Direction == "frequency_thermal" &&
			approx(entry.OverlapMs, 0.116) {
			foundOnSmall = true
		}
	}
	if !foundOnSmall {
		t.Fatalf("互指成对: the small family seat must mirror the kept pair: %+v", ioSmall.CrossDirectionOverlaps)
	}

	// 值/序数零动 live face: the disclosure lane rides published values that
	// stay the pre-fix board's (running eff 58.320 / family 3.670 / small
	// family 0.941).
	if !approx(running.EffectiveImpactMs, 58.320) || !approx(ioFamily.EffectiveImpactMs, 3.670) ||
		!approx(ioSmall.EffectiveImpactMs, 0.941) {
		t.Fatalf("value channels drifted: running eff %.6f family eff %.6f small eff %.6f",
			running.EffectiveImpactMs, ioFamily.EffectiveImpactMs, ioSmall.EffectiveImpactMs)
	}
}

// TestINTERFLOOR1KeepLivePinTieba61839 — the classic significant-keep live
// shape on the committed tieba fixture: the 61839 nested full-containment
// pair (running r7 × class_verification r8, overlap 0.285ms = 100% of the
// smaller seat's published eff) sits far above the relative floor and must
// keep its symmetric mutual disclosure. Day-one guard against an over-eager
// floor: if the gate ever eats full-containment forms, this turns red.
func TestINTERFLOOR1KeepLivePinTieba61839(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Skipf("tieba fixture not present: %v", err)
	}
	q := Query{PID: 61839, TimeStart: 34579.470, TimeEnd: 34579.520,
		TraceFlavorHint: TraceFlavorHarmonyHitrace}
	rank := BuildRootCauseRank(idx, q)
	approx := func(got, want float64) bool { return got-want <= 0.001 && got-want >= -0.001 }
	var running, verify *RootCauseRankItem
	for i := range rank.Items {
		item := &rank.Items[i]
		if item.Rank <= 0 {
			continue
		}
		if item.Type == "running" && approx(item.EffectiveImpactMs, 0.288) {
			running = item
		}
		if item.Type == "class_verification" && approx(item.EffectiveImpactMs, 0.285) {
			verify = item
		}
	}
	if running == nil || verify == nil {
		t.Fatalf("fixture drift: the 61839 board must hold the running/class_verification pair seats (running=%v verify=%v)",
			running != nil, verify != nil)
	}
	found := false
	for _, entry := range running.CrossDirectionOverlaps {
		if entry.Direction == "self_workload" && approx(entry.OverlapMs, 0.285) {
			found = true
		}
	}
	if !found {
		t.Fatalf("keep 正臂: the running seat must disclose the nested 0.285ms pair: %+v", running.CrossDirectionOverlaps)
	}
	found = false
	for _, entry := range verify.CrossDirectionOverlaps {
		if entry.Direction == "frequency_thermal" && approx(entry.OverlapMs, 0.285) {
			found = true
		}
	}
	if !found {
		t.Fatalf("互指成对: the verify seat must mirror the nested pair: %+v", verify.CrossDirectionOverlaps)
	}
}
