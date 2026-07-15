package types

// trace_causal_projection_rnb5b_test.go — RNB-5B 件⑥ (§29.96.2 终判⑥,
// 2026-07-15) wire-parse pin: the typed wire-fold source bit MergedWireFold
// mints EXACTLY at the folded_* note re-materialization (the engine's
// self-published take-MAX fold lane) and nowhere else — display merges build
// Merged* channels without it, so the 单次最大 equation face can key on the
// bit instead of the retired eff==MergedMaxMS numeric coincidence.

import (
	"testing"
)

func rnb5bWireFoldRecord() ObservationRecord {
	// The production shape of traceQueryWakeupCausalImpactFoldRecord
	// (internal/tool/trace_query.go) — subjectless, on-chain, folded_* family.
	return ObservationRecord{
		ID:              "trace_query:t#wakeup_causal_impact_fold",
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		ClaimKey:        "wakeup_causal_impact:folded_overflow",
		Predicate:       "wakeup_causal_impact",
		Value:           "5.000",
		Unit:            "ms",
		Confidence:      0.78,
		RichNotes: []string{
			"causality=on_wakeup_chain",
			"chain_relevance=on_chain",
			"impact_ms=5.000",
			"folded_rows=3",
			"folded_min_ms=1.000",
			"folded_max_ms=5.000",
			"folded_subjects=a-1,b-2",
		},
	}
}

// RNB-5B 件②: the strict relevance parser admits the non-channel token
// verbatim — falling to the causality fallback would re-mint the "adjacent"
// channel claim and feed the row back into the ◇ bucket-cap fold (the 17267
// production death this batch fixes).
func TestRNB5BSelfCaliberSideTokenSurvivesStrictParser(t *testing.T) {
	record := ObservationRecord{
		ID: "trace_query:t#root_cause_rank:18", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", Role: AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard, ClaimKey: "root_cause:x",
		Subject: "app-42", Predicate: "root_cause_caliber_side", Object: "page_cache_churn",
		Value: "81.616", Unit: "ms", Confidence: 0.66,
		RichNotes: []string{"tier=caliber_side", "type=page_cache_churn",
			"impact_ms=81.616", "effective_impact_ms=81.616",
			"causality=adjacent_to_wakeup_chain", "chain_relevance=self_caliber_side"},
	}
	projection := TraceCausalProjectionFromObservationRecords([]ObservationRecord{record})
	found := false
	for _, node := range projection.AdjacentCauses {
		if node.TypeToken != "page_cache_churn" {
			continue
		}
		found = true
		if node.ChainRelevance != "self_caliber_side" {
			t.Fatalf("the non-channel token must survive the strict parser (causality fallback = the retired adjacent claim): %+v", node)
		}
	}
	if !found {
		t.Fatalf("the side-rail row must ride the adjacent bucket carriage: %+v", projection.AdjacentCauses)
	}
}

func TestRNB5BWireFoldBitMintsAtFoldedNotesOnly(t *testing.T) {
	projection := TraceCausalProjectionFromObservationRecords([]ObservationRecord{rnb5bWireFoldRecord()})
	found := false
	for _, bucket := range [][]TraceCausalProjectionNode{
		projection.OnChainCauses, projection.AdjacentCauses, projection.BackgroundCauses,
	} {
		for _, node := range bucket {
			if node.MergedCount != 3 {
				continue
			}
			found = true
			if !node.MergedWireFold {
				t.Fatalf("the folded_* re-materialization must mint MergedWireFold: %+v", node)
			}
			if !node.OnChainOverflowFold {
				t.Fatalf("the wire fold row keeps its overflow-fold identity: %+v", node)
			}
		}
	}
	if !found {
		t.Fatalf("fixture drifted: the wire fold record must re-materialize as a MergedCount=3 node")
	}
	// Negative arm: a display-side aggregate (no folded_* notes) never mints
	// the bit — the same-kind ×3 merge path builds Merged* without it.
	nodes := []TraceCausalProjectionNode{}
	for i := 0; i < 3; i++ {
		nodes = append(nodes, TraceCausalProjectionNode{
			Role: TraceCausalRoleRootCauseContext, EvidenceID: "e" + string(rune('1'+i)),
			Subject: "worker-9", Object: "io_latency", TypeToken: "io_latency",
			ChainRelevance: "on_chain",
			ImpactMS:       float64(i + 1), CumulativeImpactMS: float64(i + 1),
			StartTs: float64(100 + 10*i), EndTs: float64(105 + 10*i),
			Confidence: 0.8, LineStart: 10 * (i + 1), LineEnd: 10*(i+1) + 5,
		})
	}
	merged := traceCausalProjectionAggregateSameKind(nodes)
	for _, node := range merged {
		if node.MergedCount > 1 && node.MergedWireFold {
			t.Fatalf("a display-side same-kind merge must never mint MergedWireFold: %+v", node)
		}
	}
}
