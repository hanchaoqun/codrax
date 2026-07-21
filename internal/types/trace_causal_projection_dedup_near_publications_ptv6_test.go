package types

import (
	"fmt"
	"math"
	"testing"
)

// PTV6 批② #4 (2026-07-06) — aggregation integrity, single-thread phantom sum.
// Real specimen: THREE io_latency publications of one wall-clock wait on one
// subject — 1.354 / 1.382 / 1.383ms over pairwise-overlapping line spans
// 2908-3094 / 2911-3114 / 2913-3120 (boundary re-carved as adjacent samples
// landed). The 0.03ms drift escaped V4's exact-equality lane, R2 summed the
// trio into a 4.119ms phantom — 138% of the 2.992ms window, physically
// impossible for a single thread — and the window-source share check flagged
// the source at 138%. The V4 near lane (≤3% band behind the full identity +
// overlap gate) folds the trio to the member MAX with the publication count
// disclosed: 1.383ms = 46% of the window.

func nearDedupRecord(id string, impact float64, lineStart, lineEnd int) ObservationRecord {
	return aggregateTestRecord(id, "root_cause_context", "root_cause_context:"+id,
		"RenderWorker-4132", "io_latency", fmt.Sprintf("%.3f", impact), impact, lineStart, lineEnd,
		"chain_relevance=adjacent", "causality=adjacent_to_chain")
}

// Specimen pin (mutation guard: removing the near lane sends the trio back to
// the R2 SUM — ImpactMS becomes 4.119, MergedCount 3, DuplicatePublications 0
// — every assertion below goes red).
func TestTraceCausalProjectionNearDuplicatePublicationsFoldBeforeR2Sum(t *testing.T) {
	records := []ObservationRecord{
		nearDedupRecord("N1", 1.354, 2908, 3094),
		nearDedupRecord("N2", 1.382, 2911, 3114),
		nearDedupRecord("N3", 1.383, 2913, 3120),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.AdjacentCauses) != 1 {
		t.Fatalf("near-duplicate publications must fold to ONE row before R2, got %d: %+v",
			len(got.AdjacentCauses), got.AdjacentCauses)
	}
	fold := got.AdjacentCauses[0]
	// Member MAX — the widest boundary estimate of the one fact; the legacy
	// phantom SUM was 4.119ms (138% of the 2.992ms window).
	if fold.ImpactMS != 1.383 || fold.CumulativeImpactMS != 1.383 {
		t.Fatalf("near-lane fold must publish the member MAX (1.383), never a sum: %+v", fold)
	}
	if fold.DuplicatePublications != 3 {
		t.Fatalf("publication count must land in the typed disclosure field, got %d: %+v",
			fold.DuplicatePublications, fold)
	}
	if fold.MergedCount != 0 {
		t.Fatalf("near-lane fold must not claim SUM-aggregate MergedCount semantics: %+v", fold)
	}
	wantIDs := map[string]bool{"N1": true, "N2": true, "N3": true}
	delete(wantIDs, fold.EvidenceID)
	for _, id := range fold.MergedEvidenceIDs {
		delete(wantIDs, id)
	}
	if len(wantIDs) != 0 {
		t.Fatalf("all three publications' evidence ids must union losslessly, missing %v: %+v",
			wantIDs, fold)
	}
}

// True multi-segment pin (byte-invariance of the non-overlap SUM path, and the
// mutation guard for dropping V4's overlap requirement): the SAME three
// near-band values over DISJOINT line spans are three real bursts — R2 keeps
// the ×3 SUM exactly as before the near lane existed. Value proximity alone
// must never fold.
func TestTraceCausalProjectionNearValuesDisjointSpansKeepR2Sum(t *testing.T) {
	records := []ObservationRecord{
		nearDedupRecord("D1", 1.354, 2908, 3094),
		nearDedupRecord("D2", 1.382, 3200, 3300),
		nearDedupRecord("D3", 1.383, 3400, 3500),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.AdjacentCauses) != 1 {
		t.Fatalf("three disjoint same-kind bursts should R2-aggregate to one ×3 row: %+v",
			got.AdjacentCauses)
	}
	agg := got.AdjacentCauses[0]
	if agg.MergedCount != 3 {
		t.Fatalf("distinct bursts must keep the ×3 SUM aggregate: %+v", agg)
	}
	if diff := agg.ImpactMS - 4.119; diff < -0.0005 || diff > 0.0005 {
		t.Fatalf("distinct bursts legitimately SUM (1.354+1.382+1.383=4.119): %+v", agg)
	}
	if agg.MergedMinMS != 1.354 || agg.MergedMaxMS != 1.383 {
		t.Fatalf("×3 row keeps the lossless per-instance range: %+v", agg)
	}
	if agg.DuplicatePublications > 1 {
		t.Fatalf("no duplicate publication may be claimed without overlap: %+v", agg)
	}
}

// Band-boundary pins (mutation guard: widening the 3% tolerance folds the
// out-of-band pair and the first subtest goes red; narrowing it below the
// specimen drift breaks the specimen pin above).
func TestTraceCausalProjectionNearTierBandBoundary(t *testing.T) {
	t.Run("out_of_band_stays_unfolded", func(t *testing.T) {
		// 1.000 vs 1.040 over overlapping spans: 0.040/1.040 = 3.85% > 3% — a
		// genuine second segment, not boundary drift. Below the R2 ≥3 threshold
		// the two rows must simply both survive, unfolded and unsummed.
		records := []ObservationRecord{
			nearDedupRecord("O1", 1.000, 2908, 3094),
			nearDedupRecord("O2", 1.040, 2911, 3114),
		}
		got := TraceCausalProjectionFromObservationRecords(records)
		if len(got.AdjacentCauses) != 2 {
			t.Fatalf("out-of-band values must not fold, got %d rows: %+v",
				len(got.AdjacentCauses), got.AdjacentCauses)
		}
		for _, node := range got.AdjacentCauses {
			if node.DuplicatePublications > 1 || node.MergedCount > 1 {
				t.Fatalf("out-of-band pair must stay two plain rows: %+v", node)
			}
		}
	})
	t.Run("in_band_pair_folds_below_r2_threshold", func(t *testing.T) {
		// 1.000 vs 1.030: 0.030/1.030 = 2.91% ≤ 3% — one republished
		// measurement. R2 (≥3) never sees pairs, so only the V4-layer near
		// lane stops this fact rendering as two rows.
		records := []ObservationRecord{
			nearDedupRecord("P1", 1.000, 2908, 3094),
			nearDedupRecord("P2", 1.030, 2911, 3114),
		}
		got := TraceCausalProjectionFromObservationRecords(records)
		if len(got.AdjacentCauses) != 1 {
			t.Fatalf("in-band overlapping pair must fold to one row, got %d: %+v",
				len(got.AdjacentCauses), got.AdjacentCauses)
		}
		fold := got.AdjacentCauses[0]
		if fold.ImpactMS != 1.030 || fold.DuplicatePublications != 2 {
			t.Fatalf("pair fold publishes the MAX with ×2 disclosure: %+v", fold)
		}
	})
}

// TestTraceCausalProjectionNearBandExactEdgeInclusive (AUDITFIX-A G4) pins the
// exact 3.0% band edge, 含/不含双臂: the near band is INCLUSIVE at exactly 3%
// ((hi−lo)/hi == tolerance folds) and one float64 ULP of extra drift stays
// unfolded. 恰界形表示敏感 (µs 尘隙教训): the edge operands are exact integers
// — 100/97, whose ratio 3/100 rounds to the SAME float64 as the 0.03 literal —
// never hand-typed non-representable decimals (1.000/0.970 would land one ULP
// OUTSIDE the band), and the exclusive arm is one math.Nextafter ULP below the
// lower operand. Behavior probed live before pinning (ratio
// 0.029999999999999999 → in band; one ULP below 97 → ratio
// 0.030000000000000141 → out).
func TestTraceCausalProjectionNearBandExactEdgeInclusive(t *testing.T) {
	if !traceCausalProjectionNearDuplicateValues(100, 97) {
		t.Fatalf("(hi-lo)/hi exactly == 3%% must sit inside the inclusive band")
	}
	beyond := math.Nextafter(97, 0) // one ULP more drift than the exact edge
	if traceCausalProjectionNearDuplicateValues(100, beyond) {
		t.Fatalf("one ULP beyond the exact 3%% edge must stay outside the band")
	}

	// Full projection face at the exact edge: an overlapping-span pair sitting
	// exactly on the edge folds to ONE row publishing the member MAX with ×2
	// disclosure (same shape as the 2.91% in-band pair pin above).
	records := []ObservationRecord{
		nearDedupRecord("E1", 100, 2908, 3094),
		nearDedupRecord("E2", 97, 2911, 3114),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.AdjacentCauses) != 1 {
		t.Fatalf("exact-edge overlapping pair must fold to one row, got %d: %+v",
			len(got.AdjacentCauses), got.AdjacentCauses)
	}
	fold := got.AdjacentCauses[0]
	if fold.ImpactMS != 100 || fold.DuplicatePublications != 2 {
		t.Fatalf("exact-edge fold publishes the MAX with ×2 disclosure: %+v", fold)
	}
}

// Exact-lane byte-invariance pin: a fold of BIT-EQUAL ImpactMS publications
// (the original V4 lane) leaves the survivor's values untouched, even when the
// folded copy carries a larger CumulativeImpactMS — publication count and
// evidence union only (mutation guard: hoisting the near-lane value lift out
// of its values-differ fork goes red here). Direct absorb-level pin: the full
// pipeline orders bucket rows by magnitude, which would put the larger
// cumulative first and mask the lift.
func TestTraceCausalProjectionExactLaneFoldLeavesValuesUntouched(t *testing.T) {
	survivor := TraceCausalProjectionNode{
		EvidenceID: "X1", Subject: "RenderWorker-4132", Object: "io_latency",
		ImpactMS: 2.000, CumulativeImpactMS: 2.000, LineStart: 2908, LineEnd: 3094,
	}
	dup := TraceCausalProjectionNode{
		EvidenceID: "X2", Subject: "RenderWorker-4132", Object: "io_latency",
		ImpactMS: 2.000, CumulativeImpactMS: 9.000, LineStart: 2911, LineEnd: 3114,
	}
	if !traceCausalProjectionSameDuplicatePublication(survivor, dup) {
		t.Fatalf("bit-equal overlapping publications must match the exact lane")
	}
	traceCausalProjectionAbsorbDuplicatePublication(&survivor, dup)
	if survivor.ImpactMS != 2.000 || survivor.CumulativeImpactMS != 2.000 {
		t.Fatalf("exact-lane fold must leave the survivor's values byte-identical: %+v", survivor)
	}
	if survivor.DuplicatePublications != 2 || len(survivor.MergedEvidenceIDs) != 1 ||
		survivor.MergedEvidenceIDs[0] != "X2" {
		t.Fatalf("exact-lane fold is publication count + evidence union only: %+v", survivor)
	}
}

// Sentinel-object guard pin (the user-adjudicated strict shape): two
// same-subject waits on UNRESOLVED peers — object is the unknown-thread
// sentinel — 112.223 vs 112.214ms (9µs = 0.008% apart, deep inside any band)
// over overlapping enclosing ranges are DISTINCT facts. A sentinel object
// carries no identity to support an approximate merge, so the near lane must
// fail open to separate rows (mutation guard: dropping the
// traceCausalProjectionKnownSubject(Object) gate goes red here).
func TestTraceCausalProjectionNearLaneRefusesSentinelObject(t *testing.T) {
	a := TraceCausalProjectionNode{
		EvidenceID: "S1", Subject: ".ugc.aweme.lite-16547", Object: "unknown-thread",
		ImpactMS: 112.223, CumulativeImpactMS: 112.223, LineStart: 45696, LineEnd: 79000,
	}
	b := TraceCausalProjectionNode{
		EvidenceID: "S2", Subject: ".ugc.aweme.lite-16547", Object: "unknown-thread",
		ImpactMS: 112.214, CumulativeImpactMS: 112.214, LineStart: 45697, LineEnd: 79000,
	}
	if traceCausalProjectionSameDuplicatePublication(a, b) {
		t.Fatalf("near-value waits on an unresolved peer are distinct facts — sentinel objects must never near-fold")
	}
	// The same drift with a REAL object identity is the republication shape.
	a.Object, b.Object = "io_latency", "io_latency"
	if !traceCausalProjectionSameDuplicatePublication(a, b) {
		t.Fatalf("a real object identity with in-band drift and overlap must match the near lane")
	}
}
