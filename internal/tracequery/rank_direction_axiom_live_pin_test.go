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
	"fmt"
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
		// IO-WAKE re-base: the exact completion→issuer census replaces the
		// accidental public Top-8 families. This no-override query carries 47
		// self-ruler release-point request waits totalling 13.830ms.
		if item.Type == "io_latency" && item.ResourceCompletionClosure &&
			item.MemberCount == 47 && approx(item.ImpactMs, 13.830) {
			ioFamily = item
		}
	}
	if running == nil || ioFamily == nil {
		var shapes []string
		for _, item := range rank.Items {
			if item.Thread.PID == 17267 && item.Type == "io_latency" {
				shapes = append(shapes, fmt.Sprintf("rank=%d eff=%.3f members=%d closure=%v basis=%s fold=%s", item.Rank, item.EffectiveImpactMs, item.MemberCount, item.ResourceCompletionClosure, item.OnChainBasis, item.MemberFoldCaliber))
			}
		}
		t.Fatalf("fixture drift: flagship board must hold running r1 and the full strict io_latency family (running=%v ioFamily=%v shapes=%v)", running != nil, ioFamily != nil, shapes)
	}

	// The full causal family still carries exact typed segments and enters the
	// direction-support lane through that inventory rather than its envelope.
	if len(ioFamily.familyMemberIntervals) != 47 {
		t.Fatalf("当届形: the io_latency family carries 47 member segments, got %d", len(ioFamily.familyMemberIntervals))
	}
	unionMs, _ := foldIntervalUnionMs(ioFamily.familyMemberIntervals)
	if !approx(unionMs, ioFamily.ImpactMs) {
		t.Fatalf("µs identity: union %.6f vs published %.6f", unionMs, ioFamily.ImpactMs)
	}
	if ioFamily.directionSupportBasis != RootCauseDirectionBasisFamilySegments {
		t.Fatalf("the family seat must enter through the family-segments basis, got %q", ioFamily.directionSupportBasis)
	}

	// The 1.571ms running×IO overlap is significant relative to the causal
	// family and must be published symmetrically on both typed seats.
	foundOnRunning := false
	for _, entry := range running.CrossDirectionOverlaps {
		if entry.Basis == RootCauseDirectionBasisFamilySegments && entry.Direction == "io_dependency" &&
			approx(entry.OverlapMs, 1.571) {
			foundOnRunning = true
		}
	}
	if !foundOnRunning {
		t.Fatalf("significant-keep 正臂: the running seat must disclose the 1.571ms causal IO pair: %+v", running.CrossDirectionOverlaps)
	}
	foundOnFamily := false
	for _, entry := range ioFamily.CrossDirectionOverlaps {
		if entry.Basis == RootCauseDirectionBasisSelfRunning && entry.Direction == "frequency_thermal" &&
			approx(entry.OverlapMs, 1.571) {
			foundOnFamily = true
		}
	}
	if !foundOnFamily {
		t.Fatalf("互指成对: the causal family must mirror the kept pair: %+v", ioFamily.CrossDirectionOverlaps)
	}

	if !approx(running.EffectiveImpactMs, 58.320) || !approx(ioFamily.EffectiveImpactMs, 13.830) {
		t.Fatalf("value channels drifted: running eff %.6f causal IO eff %.6f",
			running.EffectiveImpactMs, ioFamily.EffectiveImpactMs)
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
