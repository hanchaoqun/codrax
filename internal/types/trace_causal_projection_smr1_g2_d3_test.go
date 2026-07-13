package types

import (
	"fmt"
	"testing"
)

// SMR-1 批 (smr_audit_report §②, 2026-07-12) — types-layer halves:
//
// WO-G2 (SMR-S12b, 42728 witness): a 0.000ms wake-instant marker seated as a
// real row beside its own valued wait row (E3(+1) 0.091ms iowait vs E4 0.000ms,
// both「对端线程未解析」) implies a second wait account. The marker folds into
// the enclosing valued row as a VALUELESS member — the §29.13 无时长值成员披露
// lane (same page's E8(+2)「1项无时长值」form). 禁裸删席: fold-with-disclosure
// is the only removal path.
//
// WO-D3 根修臂 (S3-TPF, 42729 E9/E15 · 62930 E9/E19): the typed-token
// completeness fork republishes ONE measurement with and without its TypeToken;
// the V4 exact lane now folds the single-side-absence shape (exact value +
// same subject + sentinel object + span overlap) — 去重先于合并.

func smr1MarkerRecord(id, subject, object string, ts float64, line int) ObservationRecord {
	return ObservationRecord{
		ID: id, Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: ClaimGroundingHard, Predicate: "critical_blocking",
		ClaimKey: "critical_blocking:" + id, Subject: subject, Object: object,
		Value: "0.000", Unit: "ms", Confidence: 0.6,
		SupportRefs: []string{"obs:" + id},
		Span:        ObservationSpan{LineStart: line, LineEnd: line, StartTs: ts, EndTs: ts},
		RichNotes:   []string{"chain_relevance=adjacent", "causality=adjacent_to_chain"},
	}
}

func smr1ValuedRecord(id, subject, object string, impact, startTs, endTs float64, lineStart, lineEnd int) ObservationRecord {
	return ObservationRecord{
		ID: id, Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: ClaimGroundingHard, Predicate: "critical_blocking",
		ClaimKey: "critical_blocking:" + id, Subject: subject, Object: object,
		Value: fmt.Sprintf("%.3f", impact), Unit: "ms", Confidence: 0.8,
		SupportRefs: []string{"obs:" + id},
		Span:        ObservationSpan{LineStart: lineStart, LineEnd: lineEnd, StartTs: startTs, EndTs: endTs},
		RichNotes: []string{
			fmt.Sprintf("impact_ms=%.3f", impact), fmt.Sprintf("cumulative_impact_ms=%.3f", impact),
			"chain_relevance=adjacent", "causality=adjacent_to_chain",
		},
	}
}

// 42728 E3/E4 pin: the marker folds; the valued row discloses the valueless
// member (×2 with MergedValuelessCount=1) and unions the E#.
func TestSMR1G2ZeroValueMarkerFoldsIntoEnclosingValuedRow(t *testing.T) {
	records := []ObservationRecord{
		smr1ValuedRecord("E3", "coldpool-6", "unknown-thread", 0.091, 100.000000, 100.000091, 500, 520),
		smr1MarkerRecord("E4", "coldpool-6", "unknown-thread", 100.000091, 510),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.AdjacentCauses) != 1 {
		t.Fatalf("the 0.000ms instant marker must fold into its enclosing valued row (折后单席), got %d rows: %+v",
			len(got.AdjacentCauses), got.AdjacentCauses)
	}
	fold := got.AdjacentCauses[0]
	if fold.ImpactMS != 0.091 || fold.CumulativeImpactMS != 0.091 {
		t.Fatalf("吸收=佩记号,数值不重计 — the valued account must stay untouched: %+v", fold)
	}
	if fold.MergedCount != 2 || fold.MergedValuelessCount != 1 {
		t.Fatalf("the marker must be disclosed as a valueless member (§29.13 lane): count=%d valueless=%d",
			fold.MergedCount, fold.MergedValuelessCount)
	}
	if len(fold.MergedEvidenceIDs) != 1 || fold.MergedEvidenceIDs[0] != "E4" {
		t.Fatalf("零静默消失 — the marker's E# must join the bracket: %+v", fold.MergedEvidenceIDs)
	}
}

// Fail-open control: a marker whose instant sits OUTSIDE the valued row's own
// occurrence segment proves nothing — both seats stay.
func TestSMR1G2MarkerOutsideSegmentKeepsBothSeats(t *testing.T) {
	records := []ObservationRecord{
		smr1ValuedRecord("E3", "coldpool-6", "unknown-thread", 0.091, 100.000000, 100.000091, 500, 520),
		smr1MarkerRecord("E4", "coldpool-6", "unknown-thread", 100.050000, 510),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.AdjacentCauses) != 2 {
		t.Fatalf("an out-of-segment marker is not this row's wake instant — fail-open keeps two seats, got %d",
			len(got.AdjacentCauses))
	}
}

// Fail-open control: TWO enclosing valued rows = ambiguous host — the marker
// pass itself never folds (the pass-level unit form isolates it from the
// later R2 ×N sweep, which has its own G12 valueless accounting).
func TestSMR1G2AmbiguousHostFailsOpenAtPassLevel(t *testing.T) {
	nodes := []TraceCausalProjectionNode{
		{EvidenceID: "E3", Subject: "coldpool-6", Object: "unknown-thread",
			ImpactMS: 0.091, CumulativeImpactMS: 0.091, StartTs: 100.000000, EndTs: 100.000091},
		{EvidenceID: "E5", Subject: "coldpool-6", Object: "unknown-thread",
			ImpactMS: 0.150, CumulativeImpactMS: 0.150, StartTs: 100.000000, EndTs: 100.000200},
		{EvidenceID: "E4", Subject: "coldpool-6", Object: "unknown-thread",
			StartTs: 100.000050, EndTs: 100.000050, LineStart: 510, LineEnd: 510},
	}
	got := traceCausalProjectionFoldZeroValueMarkerRows(nodes)
	if len(got) != 3 {
		t.Fatalf("ambiguous enclosing hosts must fail open (no fold), got %d rows", len(got))
	}
	for _, node := range got {
		if node.MergedValuelessCount != 0 {
			t.Fatalf("no host may claim the marker under ambiguity: %+v", node)
		}
	}
}

func smr1TokenForkRecord(id, token string, impact float64, lineStart, lineEnd int) ObservationRecord {
	notes := []string{
		"impact_ms=4.884", "cumulative_impact_ms=4.884",
		"chain_relevance=on_chain", "causality=on_wakeup_chain",
	}
	if token != "" {
		notes = append(notes, "type="+token, "dominant_state=d_state")
	}
	return ObservationRecord{
		ID: id, Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: ClaimGroundingHard, Predicate: "critical_blocking",
		ClaimKey: "critical_blocking:" + id, Subject: "threadpoolforeg-60555",
		Object: "unknown-thread", Value: "4.884", Unit: "ms", Confidence: 0.8,
		SupportRefs: []string{"obs:" + id},
		Span:        ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		RichNotes:   notes,
	}
}

// S3-TPF pin (root arm): one measurement published token-less AND token-carrying
// folds on the exact lane; the survivor keeps the RICHER identity (typed token).
func TestSMR1D3TokenCompletenessForkFoldsOnExactLane(t *testing.T) {
	records := []ObservationRecord{
		smr1TokenForkRecord("E9", "", 4.884, 8712, 15131),
		smr1TokenForkRecord("E15", "d_state_or_io_wait", 4.884, 8714, 15120),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.OnChainCauses) != 1 {
		t.Fatalf("the typed-token completeness fork republishes ONE measurement — must fold (去重先于合并), got %d: %+v",
			len(got.OnChainCauses), got.OnChainCauses)
	}
	fold := got.OnChainCauses[0]
	if fold.ImpactMS != 4.884 {
		t.Fatalf("exact lane keeps the value untouched: %+v", fold)
	}
	if fold.DuplicatePublications != 2 {
		t.Fatalf("publication count must land in the typed field: %+v", fold)
	}
	if fold.TypeToken != "d_state_or_io_wait" {
		t.Fatalf("词位取最高车道 — the survivor must keep the typed token, got %q", fold.TypeToken)
	}
}

// 9µs strict-ruling guard: near values (not exact) on sentinel objects stay two
// rows even under the token-absence shape — the relaxation never enters the
// near lane.
func TestSMR1D3NearValueSentinelStaysTwoRows(t *testing.T) {
	a := smr1TokenForkRecord("E9", "", 4.884, 8712, 15131)
	b := smr1TokenForkRecord("E15", "d_state_or_io_wait", 4.884, 8714, 15120)
	b.RichNotes = []string{
		"impact_ms=4.879", "cumulative_impact_ms=4.879",
		"chain_relevance=on_chain", "causality=on_wakeup_chain",
		"type=d_state_or_io_wait", "dominant_state=d_state",
	}
	b.Value = "4.879"
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{a, b})
	if len(got.OnChainCauses) != 2 {
		t.Fatalf("near-value sentinel-object pairs are DISTINCT facts (9µs 裁定不动), got %d rows",
			len(got.OnChainCauses))
	}
}

// Two DIFFERENT non-empty tokens are two accounts — the relaxation covers only
// single-side absence.
func TestSMR1D3DistinctTokensStayTwoRows(t *testing.T) {
	a := smr1TokenForkRecord("E9", "io_wait", 4.884, 8712, 15131)
	b := smr1TokenForkRecord("E15", "d_state_or_io_wait", 4.884, 8714, 15120)
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{a, b})
	if len(got.OnChainCauses) != 2 {
		t.Fatalf("two different typed tokens are two accounts (W-A), got %d rows", len(got.OnChainCauses))
	}
}
