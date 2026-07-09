package types

// GAP-B (Wave-3.1 投影/显示批) types-side pins, ledger
// docs/design/real_trace_campaign_20260705.md §27/§28.3, 2026-07-09.
//
//   - G4 (§27.2): the elected trunk publishes its OWN typed selected_window
//     identity (WakeupPathQueryWindowStartTs/EndTs) — branch ordinals are
//     numbered per query window, so the display attach domain needs the
//     window dimension the compile side must carry.
//   - G5 (§27.3): TraceCausalProjectionMergeOccurrenceRows shares the ONE R2
//     merge authority (SUM + a–b range + lossless MergedEvidenceIDs) for the
//     display trunk's ×2 same-(thread,state) occurrence fold.
//
// MUTATION self-checks:
//   - dropping the wakeup_chain-case selected_window capture (or the elected
//     candidate's window publication) reds
//     TestGAPBWakeupPathCarriesElectedQueryWindow;
//   - re-seeding TraceCausalProjectionMergeOccurrenceRows off a private field
//     copy instead of the shared merge body reds
//     TestGAPBMergeOccurrenceRowsSharesR2Authority (sum/range/evidence drift).

import (
	"fmt"
	"testing"
)

func gapbWindowPathRecord(id, target, path string, branch int, ws, we float64) ObservationRecord {
	record := anchorB1PathRecord(id, target, path)
	record.RichNotes = []string{
		fmt.Sprintf("branch=%d", branch),
		fmt.Sprintf("branches=%d", branch),
		fmt.Sprintf("selected_window=%.6f..%.6f", ws, we),
	}
	return record
}

// G4 compile plumbing: the projection's trunk window is the ELECTED record's
// typed selected_window — not the first record's, not a guess.
func TestGAPBWakeupPathCarriesElectedQueryWindow(t *testing.T) {
	w1 := gapbWindowPathRecord("path-w1", "other-9",
		"disk-1 -> irq-2 -> other-9", 1, 100.0, 100.2)
	w2 := gapbWindowPathRecord("path-w2", "user.app-100",
		"hm-up-5 -> mmi-4 -> user.app-100", 1, 200.0, 200.3)
	got := TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{w1, w2}, []string{"100"})
	if !got.WakeupPathUserElected {
		t.Fatalf("the W2 record must win the entity election, got %+v", got.WakeupPath)
	}
	if got.WakeupPathQueryWindowStartTs != 200.0 || got.WakeupPathQueryWindowEndTs != 200.3 {
		t.Fatalf("trunk window must be the ELECTED record's selected_window (200.0..200.3), got %.3f..%.3f",
			got.WakeupPathQueryWindowStartTs, got.WakeupPathQueryWindowEndTs)
	}

	// Legacy no-entity lane: candidates[0]'s window travels with its path.
	legacy := TraceCausalProjectionFromObservationRecords([]ObservationRecord{w1, w2})
	if legacy.WakeupPathQueryWindowStartTs != 100.0 || legacy.WakeupPathQueryWindowEndTs != 100.2 {
		t.Fatalf("legacy candidates[0] lane must carry ITS window (100.0..100.2), got %.3f..%.3f",
			legacy.WakeupPathQueryWindowStartTs, legacy.WakeupPathQueryWindowEndTs)
	}
}

// G4 zero-value arm (§22.2 教训, audited separately): a wakeup_chain record
// WITHOUT a selected_window note leaves the trunk window ZERO — absence never
// guesses a window (and the display gate stays inert on a window-less trunk).
func TestGAPBWakeupPathWindowAbsenceStaysZero(t *testing.T) {
	record := anchorB1PathRecord("path-legacy", "user.app-100",
		"waker-3 -> mid-2 -> user.app-100")
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{record})
	if got.WakeupPathQueryWindowStartTs != 0 || got.WakeupPathQueryWindowEndTs != 0 {
		t.Fatalf("a note-less path record must leave the trunk window zero, got %.3f..%.3f",
			got.WakeupPathQueryWindowStartTs, got.WakeupPathQueryWindowEndTs)
	}
}

// G5: the exported occurrence merge is the SAME R2 authority — SUM value,
// per-instance a–b range, MergedCount, lossless MergedEvidenceIDs, line/ts
// envelope, best rank, min confidence. Threshold policy (2 vs the R2 pass's
// ≥3) belongs to the CALLER; the field semantics must be one implementation.
func TestGAPBMergeOccurrenceRowsSharesR2Authority(t *testing.T) {
	a := TraceCausalProjectionNode{
		Subject: "OS_mmi_EventHdr-43103", Object: "sleep_wait", StateKind: "s_sleep",
		EvidenceID: "E-a", ImpactMS: 4.431, CumulativeImpactMS: 4.431,
		LineStart: 100, LineEnd: 200, Rank: 5, Confidence: 0.9,
	}
	b := TraceCausalProjectionNode{
		Subject: "OS_mmi_EventHdr-43103", Object: "sleep_wait", StateKind: "s_sleep",
		EvidenceID: "E-b", ImpactMS: 0.904, CumulativeImpactMS: 0.904,
		LineStart: 300, LineEnd: 400, Rank: 3, Confidence: 0.7,
	}
	merged := TraceCausalProjectionMergeOccurrenceRows([]TraceCausalProjectionNode{a, b})
	if merged.MergedCount != 2 {
		t.Fatalf("MergedCount = %d, want 2", merged.MergedCount)
	}
	if merged.ImpactMS != 4.431+0.904 || merged.CumulativeImpactMS != 4.431+0.904 {
		t.Fatalf("×N value must be the member SUM (5.335), got impact=%.3f cum=%.3f",
			merged.ImpactMS, merged.CumulativeImpactMS)
	}
	if merged.MergedMinMS != 0.904 || merged.MergedMaxMS != 4.431 {
		t.Fatalf("per-instance range must be 0.904–4.431, got %.3f–%.3f",
			merged.MergedMinMS, merged.MergedMaxMS)
	}
	if len(merged.MergedEvidenceIDs) != 1 || merged.MergedEvidenceIDs[0] != "E-b" {
		t.Fatalf("member evidence must absorb losslessly, got %v", merged.MergedEvidenceIDs)
	}
	if merged.LineStart != 100 || merged.LineEnd != 400 {
		t.Fatalf("line envelope must span the members, got %d-%d", merged.LineStart, merged.LineEnd)
	}
	if merged.Rank != 3 || merged.Confidence != 0.7 {
		t.Fatalf("best rank / min confidence must follow the R2 rules, got rank=%d conf=%.2f",
			merged.Rank, merged.Confidence)
	}
	// Degenerate lanes: 0/1 members never invent a merge.
	if got := TraceCausalProjectionMergeOccurrenceRows(nil); got.MergedCount != 0 {
		t.Fatalf("empty input must stay a zero node, got %+v", got)
	}
	if got := TraceCausalProjectionMergeOccurrenceRows([]TraceCausalProjectionNode{a}); got.MergedCount != 0 || got.ImpactMS != 4.431 {
		t.Fatalf("single input must pass through unmerged, got %+v", got)
	}
}

// 复核 P1-1 捎带 (2026-07-09, pre-existing R2 blind spot): the R2 group key
// carried no predicate dimension, so a wakeup_causal_aggregate row (a DERIVED
// VIEW whose per-hop member rows are retained beside it) bucketed WITH its own
// members — a ≥3 group then SUMmed the identical wall clock twice. Aggregate
// views bucket apart; the member-only control keeps the legacy ×3 fold.
func TestGAPBR2NeverBucketsAggregateViewWithMembers(t *testing.T) {
	member := func(id string, ms float64) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			Subject: "OS_mmi_EventHdr-43103", Object: "sleep_wait", StateKind: "s_sleep",
			Predicate: "wakeup_causal_impact", EvidenceID: id,
			ImpactMS: ms, CumulativeImpactMS: ms, Confidence: 0.8,
		}
	}
	agg := member("agg-1", 5.335)
	agg.Predicate = "wakeup_causal_aggregate"
	mixed := traceCausalProjectionAggregateSameKind(
		[]TraceCausalProjectionNode{agg, member("occ-1", 4.431), member("occ-2", 0.904)})
	if len(mixed) != 3 {
		t.Fatalf("the view must never bucket with its members (no fold), got %d rows", len(mixed))
	}
	for _, node := range mixed {
		if node.MergedCount > 1 {
			t.Fatalf("view+member ×N fold fabricated a double count: %+v", node)
		}
	}
	// Control: three plain members keep the legacy ≥3 SUM fold byte-stably.
	folded := traceCausalProjectionAggregateSameKind(
		[]TraceCausalProjectionNode{member("occ-1", 4.431), member("occ-2", 0.904), member("occ-3", 1.0)})
	if len(folded) != 1 || folded[0].MergedCount != 3 {
		t.Fatalf("the member-only control must keep the legacy ×3 fold, got %+v", folded)
	}
}
