package types

import (
	"strings"
	"testing"
)

// PTS-2 F1 pin (复核裁定 2026-07-06, 计数吸收 — 不做桶位豁免): when the
// compile-side on-chain bucket fold absorbs a member that is ITSELF an
// overflow fold row (engine aggregate fold / wire-cap fold, typed
// OnChainOverflowFold marker), the bucket row's N counts that member's
// MergedCount rows (G), not 1, and merges its roster under the global cap;
// per-record evidence absorption stays unchanged (one evidence id per
// absorbed record).
func TestPTS2BucketFoldAbsorbsEngineFoldCountAndRoster(t *testing.T) {
	limit := 3
	var nodes []TraceCausalProjectionNode
	for i := 0; i < limit; i++ {
		nodes = append(nodes, TraceCausalProjectionNode{
			Role: TraceCausalRoleCausalHop, EvidenceID: "e-keep-" + string(rune('a'+i)),
			Subject: "keeper-" + string(rune('a'+i)), Predicate: "wakeup_causal_impact",
			ChainRelevance: "on_chain", ImpactMS: 100 - float64(i),
		})
	}
	// Overflow: two ordinary rows + the engine aggregate fold row (G=4).
	nodes = append(nodes,
		TraceCausalProjectionNode{
			Role: TraceCausalRoleCausalHop, EvidenceID: "e-ovf-1",
			Subject: "ovf-1", Predicate: "wakeup_causal_impact",
			ChainRelevance: "on_chain", ImpactMS: 9,
		},
		TraceCausalProjectionNode{
			Role: TraceCausalRoleCausalHop, EvidenceID: "e-ovf-2",
			Subject: "ovf-2", Predicate: "wakeup_causal_impact",
			ChainRelevance: "on_chain", ImpactMS: 8,
		},
		TraceCausalProjectionNode{
			Role: TraceCausalRoleCausalHop, EvidenceID: "e-engine-fold",
			Predicate: "wakeup_causal_aggregate", ChainRelevance: "on_chain",
			OnChainOverflowFold: true, MergedCount: 4,
			MergedMinMS: 0.5, MergedMaxMS: 2.5,
			MergedSubjects: []string{"eng-a", "eng-b"},
			ImpactMS:       2.5, CumulativeImpactMS: 2.5,
		},
	)
	out := traceCausalProjectionLimitNodesOnChainFold(nodes, limit, nil)
	if len(out) != limit+1 {
		t.Fatalf("bucket fold must keep limit rows plus ONE fold row: %d", len(out))
	}
	fold := out[len(out)-1]
	if !fold.OnChainOverflowFold {
		t.Fatalf("last row must be the bucket fold: %+v", fold)
	}
	// N = 2 ordinary members + G(4) absorbed from the engine fold member.
	if fold.MergedCount != 2+4 {
		t.Fatalf("bucket fold must absorb the engine fold's row count (2 ordinary + G=4): got %d", fold.MergedCount)
	}
	// Roster merges the engine fold's subjects under the global cap (4).
	roster := strings.Join(fold.MergedSubjects, ",")
	if len(fold.MergedSubjects) != traceCausalProjectionMergedSubjectCap ||
		!strings.Contains(roster, "ovf-1") || !strings.Contains(roster, "ovf-2") ||
		!strings.Contains(roster, "eng-a") || !strings.Contains(roster, "eng-b") {
		t.Fatalf("bucket fold roster must merge the engine fold subjects under the global cap: %+v", fold.MergedSubjects)
	}
	// Evidence absorption unchanged: one id per absorbed RECORD (+N face) —
	// the engine fold contributes its own single record id, not G.
	ids := append([]string{fold.EvidenceID}, fold.MergedEvidenceIDs...)
	if len(ids) != 3 || !strings.Contains(strings.Join(ids, ","), "e-engine-fold") {
		t.Fatalf("evidence absorption must stay per-record: %+v", ids)
	}
}

// 突变面: an ordinary ×N presentation-aggregate member (MergedCount>1 WITHOUT
// the OnChainOverflowFold marker) still counts 1 — its ×N stays inside its
// absorbed evidence IDs, byte-identical to the pre-F1 behavior.
func TestPTS2BucketFoldOrdinaryAggregateMemberStillCountsOne(t *testing.T) {
	limit := 2
	nodes := []TraceCausalProjectionNode{
		{Role: TraceCausalRoleCausalHop, EvidenceID: "e-1", Subject: "keep-1",
			Predicate: "wakeup_causal_impact", ChainRelevance: "on_chain", ImpactMS: 50},
		{Role: TraceCausalRoleCausalHop, EvidenceID: "e-2", Subject: "keep-2",
			Predicate: "wakeup_causal_impact", ChainRelevance: "on_chain", ImpactMS: 40},
		{Role: TraceCausalRoleCausalHop, EvidenceID: "e-agg", Subject: "agg",
			Predicate: "wakeup_causal_impact", ChainRelevance: "on_chain",
			MergedCount: 3, MergedEvidenceIDs: []string{"e-agg-2", "e-agg-3"}, ImpactMS: 5},
	}
	out := traceCausalProjectionLimitNodesOnChainFold(nodes, limit, nil)
	fold := out[len(out)-1]
	if fold.MergedCount != 1 {
		t.Fatalf("an ordinary ×N aggregate member must still count 1 row: %+v", fold)
	}
}
