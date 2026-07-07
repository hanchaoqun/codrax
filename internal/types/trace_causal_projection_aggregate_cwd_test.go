package types

import (
	"fmt"
	"testing"
)

// §21 CWD engine pins (real_trace_campaign_20260705.md §21, cmp_01 revisit
// audit 2026-07-07 — D-新P0 排队深度方向反转 engine half): the R2 ×N merge
// must never SUM members whose QUERY WINDOWS overlap when the §11-N2 union
// deduction cannot engage (墙钟跨窗不可加和). The specimen shape: 4
// supply_pressure rank-lane observations (no occurrence Span ts) from 4
// overlapping query windows SUMMED to 34008.569ms, which the display then
// divided by the 101ms anchor window — the flagship comparison's direction
// inverted (displayed 6.0 > 7.0, tool truth 7.0 > 6.0). Such groups now
// publish the member MAX with the lossless raw Σ and the max member's own
// query window kept as the display density base.

// cwdRankRecord builds one rank-lane observation with a typed selected_window
// identity and NO occurrence Span ts (root_cause rows carry line spans only —
// the exact shape whose cross-window occurrence disjointness is unprovable).
func cwdRankRecord(id, subject, object string, impact float64, lineStart, lineEnd int, window string) ObservationRecord {
	notes := []string{
		fmt.Sprintf("impact_ms=%.3f", impact),
		fmt.Sprintf("cumulative_impact_ms=%.3f", impact),
		"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
	}
	if window != "" {
		notes = append(notes, "selected_window="+window)
	}
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		Predicate:       "wakeup_causal_impact",
		ClaimKey:        "wakeup_causal_impact:" + id,
		Subject:         subject,
		Object:          object,
		Value:           fmt.Sprintf("%.3f", impact),
		Unit:            "ms",
		Confidence:      0.8,
		SupportRefs:     []string{"obs:" + id},
		Span:            ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		RichNotes:       notes,
	}
}

// TestTraceCausalProjectionCrossWindowOverlapNoSpanPublishesMax pins the
// cmp_01 engine shape: three rank-lane members (no Span ts) from three query
// windows of which at least two OVERLAP → the merged row publishes the member
// MAX, carries the typed MergedCrossWindowMax marker, keeps the lossless raw
// Σ in MergedSumMS, and records the MAX member's own query window as the
// display density base. Reverting the merge to the unconditional SUM must
// red here (direction-correctness mutation pin).
func TestTraceCausalProjectionCrossWindowOverlapNoSpanPublishesMax(t *testing.T) {
	records := []ObservationRecord{
		cwdRankRecord("E1", "unknown-thread", "supply_pressure", 5563.967, 100, 200, "3680.568..3680.668"),
		cwdRankRecord("E2", "unknown-thread", "supply_pressure", 13821.784, 210, 300, "3680.569..3680.719"),
		cwdRankRecord("E3", "unknown-thread", "supply_pressure", 5800.766, 310, 400, "3680.818..3680.919"),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	agg := n2FindMerged(t, got.OnChainCauses, 3)

	sum := 5563.967 + 13821.784 + 5800.766
	if n2Close(agg.ImpactMS, sum) {
		t.Fatalf("§21 CWD: overlapping-window members must NOT publish the SUM %.3f: %+v", sum, agg)
	}
	if !n2Close(agg.ImpactMS, 13821.784) || !n2Close(agg.CumulativeImpactMS, 13821.784) {
		t.Fatalf("§21 CWD: the merged row must publish the member MAX 13821.784: %+v", agg)
	}
	if !agg.MergedCrossWindowMax {
		t.Fatalf("§21 CWD: the typed cross-window MAX marker must ride the row: %+v", agg)
	}
	if agg.MergedIntervalUnion {
		t.Fatalf("§21 CWD: MAX and union calibers are mutually exclusive: %+v", agg)
	}
	if !n2Close(agg.MergedSumMS, sum) {
		t.Fatalf("§21 CWD: the lossless raw Σ %.3f must survive in MergedSumMS: %+v", sum, agg)
	}
	// The MAX member (13821.784) was measured over 3680.569..3680.719 — that
	// window is the only same-base density denominator for the display layer.
	if agg.MergedMaxWindowStartTs != 3680.569 || agg.MergedMaxWindowEndTs != 3680.719 {
		t.Fatalf("§21 CWD: the MAX member's own query window must ride the row: %+v", agg)
	}
	// Lossless range + roster + no false single-window claim.
	if agg.MergedMinMS != 5563.967 || agg.MergedMaxMS != 13821.784 {
		t.Fatalf("per-instance min–max range lost: %+v", agg)
	}
	if len(agg.MergedQueryWindows) != 3 {
		t.Fatalf("window roster must list all three member query windows: %+v", agg.MergedQueryWindows)
	}
	if agg.QueryWindowStartTs != 0 || agg.QueryWindowEndTs != 0 {
		t.Fatalf("a multi-window merged row must not claim a single row-level query window: %+v", agg)
	}
}

// TestTraceCausalProjectionCrossWindowDisjointNoSpanKeepsSum pins the precise
// boundary: the §21-CWD MAX caliber engages on WINDOW overlap only. Rank-lane
// members (no Span ts) from fully DISJOINT query windows measured disjoint
// wall clock and keep the legacy SUM byte-identically — no flag, no
// MergedSumMS, roster still disclosed.
func TestTraceCausalProjectionCrossWindowDisjointNoSpanKeepsSum(t *testing.T) {
	records := []ObservationRecord{
		cwdRankRecord("E1", "unknown-thread", "supply_pressure", 20.000, 100, 200, "100.000..101.000"),
		cwdRankRecord("E2", "unknown-thread", "supply_pressure", 10.000, 210, 300, "101.400..101.800"),
		cwdRankRecord("E3", "unknown-thread", "supply_pressure", 5.000, 310, 400, "102.000..102.500"),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	agg := n2FindMerged(t, got.OnChainCauses, 3)

	if !n2Close(agg.ImpactMS, 35.0) || !n2Close(agg.CumulativeImpactMS, 35.0) {
		t.Fatalf("disjoint-window rank members must keep the SUM 35.000: %+v", agg)
	}
	if agg.MergedCrossWindowMax || agg.MergedIntervalUnion || agg.MergedSumMS != 0 {
		t.Fatalf("disjoint windows must not engage any cross-window caliber: %+v", agg)
	}
	if len(agg.MergedQueryWindows) != 3 {
		t.Fatalf("window roster disclosure survives the SUM lane: %+v", agg.MergedQueryWindows)
	}
}

// TestTraceCausalProjectionUnionStillWinsOverCrossWindowMax pins the caliber
// precedence: when the §11-N2 union CAN engage (valid, contained occurrence
// intervals overlapping across windows), the per-segment deduction stays the
// published caliber and the §21-CWD MAX marker must NOT appear — the union is
// strictly more precise and the two flags are mutually exclusive.
func TestTraceCausalProjectionUnionStillWinsOverCrossWindowMax(t *testing.T) {
	records := []ObservationRecord{
		n2Record("E1", "binder:8815_1-6581", "runnable", 104.127, 3680.6909, 3680.8192, 117231, 140719, "3680.569..3682.819"),
		n2Record("E2", "binder:8815_1-6581", "runnable", 50.057, 3680.9000, 3680.9800, 141000, 152000, "3680.569..3682.819"),
		n2Record("E3", "binder:8815_1-6581", "runnable", 15.206, 3680.7995, 3680.8192, 136600, 140719, "3680.800..3681.001"),
		n2Record("E4", "binder:8815_1-6581", "runnable", 14.550, 3681.0000, 3681.0600, 153000, 158000, "3680.800..3681.001"),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	agg := n2FindMerged(t, got.OnChainCauses, 4)
	if !agg.MergedIntervalUnion {
		t.Fatalf("union-capable group must keep the union caliber: %+v", agg)
	}
	if agg.MergedCrossWindowMax || agg.MergedMaxWindowStartTs != 0 || agg.MergedMaxWindowEndTs != 0 {
		t.Fatalf("union rows must never carry the cross-window MAX marker: %+v", agg)
	}
}
