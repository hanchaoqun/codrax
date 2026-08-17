package tracequery

// rank_direction_axiom_live_pin_test.go — INTERSECT-FIX live regression pin
// (§29.143 INTERSECT-REG 归因, 2026-07-19) on the committed donghu.ftrace,
// re-based by INTERFLOOR-1 (user ruling §29.150③, 2026-07-19) to carry BOTH
// arms of the relative de-minimis floor:
//
// INTERSECT-REG history: the §29.136 CHAIN-BUDGET regression folded the
// flagship 17267 board's io_latency single-segment row into its same-thread
// family and the 偏离④ familyMemberIntervals exclusion refused the family's
// exact member segments wholesale. IO-CAL-1 subsequently corrected those
// member intervals from request residence to issuer switch-out→completion
// wake. On one thread that response-blocked ruler is scheduler-disjoint from
// running by construction; a running×IO overlap would now prove that the old
// mechanism ruler leaked back into response-impact accounting.
//
// The pin therefore keeps both obligations: the family must retain its exact
// typed member inventory, and neither side may publish a cross-direction
// overlap against the target's own running seat. This is a caliber correction,
// not a weakened de-minimis floor; the separate synthetic floor tests retain
// the positive/demoted AXIOM-V2 arms.

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
		// self-ruler release-point issuer-blocked waits totalling 12.658ms.
		if item.Type == "io_latency" && item.ResourceCompletionClosure &&
			item.MemberCount == 47 && approx(item.ImpactMs, 12.658) {
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

	// A completion-closed issuer-blocked interval is S/D wall clock. It cannot
	// overlap the same thread's running wall clock; the former 1.571ms pair was
	// an artifact of using request residence as response impact.
	for _, entry := range running.CrossDirectionOverlaps {
		if entry.Basis == RootCauseDirectionBasisFamilySegments && entry.Direction == "io_dependency" {
			t.Fatalf("request-residence overlap leaked into the response-impact direction account: %+v", running.CrossDirectionOverlaps)
		}
	}
	for _, entry := range ioFamily.CrossDirectionOverlaps {
		if entry.Basis == RootCauseDirectionBasisSelfRunning && entry.Direction == "frequency_thermal" {
			t.Fatalf("response-blocked IO family must not mirror an impossible same-thread running overlap: %+v", ioFamily.CrossDirectionOverlaps)
		}
	}

	if !approx(running.EffectiveImpactMs, 58.320) || !approx(ioFamily.EffectiveImpactMs, 12.658) {
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
