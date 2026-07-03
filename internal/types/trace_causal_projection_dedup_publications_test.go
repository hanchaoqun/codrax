package types

import "testing"

// V4 duplicate-publication dedup (customer revisit 2026-07-03, custom_1g):
// three irq_activity rows carrying the IDENTICAL 35.350ms measurement over
// overlapping line spans reached R2's ≥3 threshold and were SUMMED into a
// 106.05ms phantom (3× over-count of one wall-clock fact). The dedup pass runs
// BEFORE R2, folds them to one row with the value unchanged, and records the
// publication count in the typed DuplicatePublications field — MergedCount
// stays reserved for SUM aggregates.

func dedupPublicationRecord(id string, impact float64, lineStart, lineEnd int) ObservationRecord {
	return aggregateTestRecord(id, "root_cause_context", "root_cause_context:"+id,
		"irq_handler-100", "irq_activity", "35.350", impact, lineStart, lineEnd,
		"chain_relevance=adjacent", "causality=adjacent_to_chain")
}

func TestTraceCausalProjectionDuplicatePublicationsFoldBeforeR2Sum(t *testing.T) {
	// Customer shape: three publications of the same 35.350ms measurement over
	// pairwise-overlapping line spans.
	records := []ObservationRecord{
		dedupPublicationRecord("E13a", 35.350, 793201, 830007),
		dedupPublicationRecord("E13b", 35.350, 793204, 830012),
		dedupPublicationRecord("E13c", 35.350, 793199, 830001),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.AdjacentCauses) != 1 {
		t.Fatalf("duplicate publications must fold to ONE row before R2, got %d: %+v",
			len(got.AdjacentCauses), got.AdjacentCauses)
	}
	fold := got.AdjacentCauses[0]
	if fold.ImpactMS != 35.350 || fold.CumulativeImpactMS != 35.350 {
		t.Fatalf("the folded value is ONE measurement (±0), never a sum: %+v", fold)
	}
	if fold.DuplicatePublications != 3 {
		t.Fatalf("publication count must land in the typed field, got %d: %+v",
			fold.DuplicatePublications, fold)
	}
	if fold.MergedCount != 0 {
		t.Fatalf("dedup fold must not claim SUM-aggregate MergedCount semantics: %+v", fold)
	}
	wantIDs := map[string]bool{"E13b": true, "E13c": true}
	for _, id := range fold.MergedEvidenceIDs {
		delete(wantIDs, id)
	}
	if fold.EvidenceID != "E13a" || len(wantIDs) != 0 {
		t.Fatalf("first occurrence survives and evidence ids union losslessly: %+v", fold)
	}
}

// Negative pin: three EQUAL-value rows with disjoint spans are three REAL
// periodic bursts — they must keep the R2 SUM ×3 path (value equality alone
// never folds; the overlap requirement is load-bearing).
func TestTraceCausalProjectionSameValueDisjointSpansKeepR2Sum(t *testing.T) {
	records := []ObservationRecord{
		dedupPublicationRecord("B1", 35.350, 100, 110),
		dedupPublicationRecord("B2", 35.350, 200, 210),
		dedupPublicationRecord("B3", 35.350, 300, 320),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.AdjacentCauses) != 1 {
		t.Fatalf("three same-kind bursts should R2-aggregate to one ×3 row: %+v", got.AdjacentCauses)
	}
	agg := got.AdjacentCauses[0]
	if agg.MergedCount != 3 {
		t.Fatalf("distinct bursts must keep the ×3 SUM aggregate: %+v", agg)
	}
	if diff := agg.ImpactMS - 106.050; diff < -0.0005 || diff > 0.0005 {
		t.Fatalf("distinct bursts legitimately SUM (35.350×3): %+v", agg)
	}
	if agg.DuplicatePublications > 1 {
		t.Fatalf("no duplicate publication may be claimed without overlap: %+v", agg)
	}
}

// Mixed pin: after the dedup fold shrinks a group below R2's ≥3 threshold, the
// survivors must NOT be summed (dedup 后不足 3 条则不再 SUM).
func TestTraceCausalProjectionDedupShrinksGroupBelowR2Threshold(t *testing.T) {
	records := []ObservationRecord{
		dedupPublicationRecord("M1", 35.350, 793201, 830007),
		dedupPublicationRecord("M2", 35.350, 793204, 830012), // duplicate of M1
		dedupPublicationRecord("M3", 12.000, 900000, 900100), // distinct burst
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.AdjacentCauses) != 2 {
		t.Fatalf("dedup must leave 2 rows (fold + distinct), got %d: %+v",
			len(got.AdjacentCauses), got.AdjacentCauses)
	}
	for _, node := range got.AdjacentCauses {
		if node.MergedCount > 1 {
			t.Fatalf("post-dedup group of 2 is below the R2 threshold — no SUM row allowed: %+v", node)
		}
	}
}
