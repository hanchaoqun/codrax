package types

// trace_causal_projection_rankdis_a3_test.go — RANKDIS-EXT A3 structural
// isolation (§29.104.16.1 M15, 2026-07-16): Node.Rank is board-seat currency
// (badges, election, the ◎ §29.112 population arm), so the projection
// compile parses the causal `rank` note ONLY on root_cause_* predicate
// records. The retired shape let a state_drilldown record's borrowed rank
// note mint Node.Rank like a board seat (empty chain_relevance = chain
// channel); the only防线 was prompt-side dedup coincidence.
//
// MUTATION self-check: dropping the predicate gate reds the drilldown arm;
// tightening it past the root_cause family reds the seat arms.

import "testing"

func rankdisA3ProjectionRecord(predicate, claimKey string, notes []string) ObservationRecord {
	return ObservationRecord{
		ID:              "trace_query:t#x:1",
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		SourceRef:       ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactID: "a.systrace"},
		ClaimKey:        claimKey,
		Subject:         "app-20",
		Predicate:       predicate,
		Object:          "s_sleep",
		Value:           "21.000",
		Unit:            "ms",
		RichNotes:       notes,
		Confidence:      0.74,
	}
}

func TestRankdisA3ProjectionRankParseIsPredicateGated(t *testing.T) {
	// Board seats keep parsing across the whole root_cause_* family.
	for _, predicate := range []string{
		"root_cause_primary", "root_cause_secondary", "root_cause_tertiary",
		"root_cause_background", "root_cause_absorbed", "root_cause_data_gap",
	} {
		node := traceCausalProjectionNodeFromRecord("classified", rankdisA3ProjectionRecord(
			predicate, predicate, []string{"rank=2", "tier=secondary"}))
		if node.Rank != 2 {
			t.Fatalf("%s: board-seat rank parse must survive the gate, got Rank=%d", predicate, node.Rank)
		}
	}
	// A non-board record carrying a rank-spelled note (the retired
	// state_drilldown borrow / any future leak) never mints a seat.
	for _, predicate := range []string{"state_drilldown", "state_churn", "wakeup_causal_impact"} {
		node := traceCausalProjectionNodeFromRecord("supporting", rankdisA3ProjectionRecord(
			predicate, predicate+":app-20", []string{"rank=1", "impact=21.000ms"}))
		if node.Rank != 0 {
			t.Fatalf("%s: a non-board record must never mint Node.Rank from a rank-spelled note, got Rank=%d", predicate, node.Rank)
		}
	}
	// The dedicated state_rank lane is display-only: it never feeds Node.Rank
	// on any predicate.
	node := traceCausalProjectionNodeFromRecord("supporting", rankdisA3ProjectionRecord(
		"state_drilldown", "state_drilldown:app-20", []string{"state_rank=1", "impact=21.000ms"}))
	if node.Rank != 0 {
		t.Fatalf("state_rank is a display lane, never board-seat currency: got Rank=%d", node.Rank)
	}
}
