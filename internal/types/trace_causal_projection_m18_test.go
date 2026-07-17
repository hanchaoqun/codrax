package types

// trace_causal_projection_m18_test.go — RANKDIS-M18 (§29.104.17 裁定②
// 2026-07-16) reader-side pins: composite-score rank rows publish the *_score
// note twins instead of the ms-semantic keys, and the projection compile's
// value/presence lanes read the union, so a caliber-side row's magnitude and
// its context sentinel keep flowing exactly as before the wire rename.
//
// MUTATION self-check: dropping TraceNoteKeyEffectiveImpactScore /
// TraceNoteKeyCumulativeImpactScore / TraceNoteKeyImpactScore from the
// projection unions reds these arms.

import "testing"

func TestM18ProjectionReadsCompositeScoreNoteTwins(t *testing.T) {
	record := ObservationRecord{
		ID:              "trace_query:w#root_cause_rank:1",
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		ClaimKey:        "root_cause_caliber_side",
		Subject:         "io-42",
		Predicate:       "root_cause_caliber_side",
		Object:          "block_io_by_inode",
		Value:           "2.694",
		Unit:            TraceObservationUnitCompositeScore,
		RichNotes: []string{
			"tier=caliber_side",
			"type=block_io_by_inode",
			"impact_score=2.116",
			"cumulative_impact_score=2.694",
			"effective_impact_score=2.694",
			"projected_total_score=2.694",
		},
	}
	node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record)
	if node.EffectiveImpactMS != 2.694 {
		t.Fatalf("effective union must read the score twin, got %.3f", node.EffectiveImpactMS)
	}
	if !node.EffectiveImpactPublished {
		t.Fatalf("presence union must see the score twin (EPUB marker)")
	}
	if node.CumulativeImpactMS != 2.694 {
		t.Fatalf("cumulative union must read the score twin, got %.3f", node.CumulativeImpactMS)
	}
	if got := traceCausalProjectionImpact(record); got != 2.116 {
		t.Fatalf("impact union must read the score twin, got %.3f", got)
	}
}

// TestM18ProjectionCompositeSentinelOnScoreKey: the context-only
// ranking-exclusion sentinel (authoritative zero) travels on the score twin
// for composite rows — present, zero, published.
func TestM18ProjectionCompositeSentinelOnScoreKey(t *testing.T) {
	record := ObservationRecord{
		ID:              "trace_query:w#root_cause_rank:1",
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		ClaimKey:        "root_cause_context_only",
		Subject:         "io_pressure",
		Predicate:       "root_cause_context_only",
		Object:          "io_pressure",
		Value:           "61.540",
		Unit:            TraceObservationUnitCompositeScore,
		RichNotes: []string{
			"tier=context_only",
			"type=io_pressure",
			"impact_score=61.540",
			"cumulative_impact_score=61.540",
			"effective_impact_score=0.000",
		},
	}
	node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record)
	if node.EffectiveImpactMS != 0 {
		t.Fatalf("the sentinel zero stays authoritative, got %.3f", node.EffectiveImpactMS)
	}
	if !node.EffectiveImpactPublished {
		t.Fatalf("the sentinel zero on the score twin must still mint the published marker")
	}
}
