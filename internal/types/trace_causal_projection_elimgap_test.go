package types

// trace_causal_projection_elimgap_test.go — ELIM-GAP carrier pins (§29.104.15,
// customer witness customlogs/cust_total_del.txt, 2026-07-16). Compile-side
// truths the display fixes ride on:
//
//	(1) the rank seat RIDES the R1 same-fact absorb backfill through the R2
//	    ×N merge (§29.67): the surviving chain-view row carries Rank with its
//	    hop predicate and empty RankFoldPeers — the exact carriage the ◎
//	    种群臂 fourth arm (tool 件A) admits; if this carriage ever changes,
//	    the display arm must be re-audited;
//	(2) the overflow fold row inherits overflow[0].Predicate VERBATIM while
//	    the typed member truth lives on MergedAllDataGap / the valued-split
//	    accounting — the display word faces (tool 件C) read the typed truth,
//	    never the seed token.
import "testing"

func elimGapHopNode(id string, impact float64, lineStart, lineEnd int) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Role:       TraceCausalRoleRootCauseContext,
		EvidenceID: id,
		Subject:    "[GT]ColdPool#9-48667",
		Predicate:  "wakeup_causal_impact",
		Object:     "runnable",
		StateKind:  "runnable",
		ImpactMS:   impact, CumulativeImpactMS: impact,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
		LineStart: lineStart, LineEnd: lineEnd, Confidence: 0.9,
	}
}

// TestElimGapRankRidesR1AbsorbThenR2Merge pins carriage (1): the witness
// E15(+2) compile chain — the chain-view survivor absorbs its
// root_cause_secondary rank record (Rank=2 backfill, hop predicate kept, the
// rank record's Object surviving as an 影响点), then merges two same-kind
// occurrences (MergedCount=3). ONE node leaves compile carrying the seat.
func TestElimGapRankRidesR1AbsorbThenR2Merge(t *testing.T) {
	chainView := elimGapHopNode("E-hop-main", 8.211, 100, 400)
	chainView.CumulativeImpactMS = 10.061
	chainView.EffectiveImpactMS = 8.211
	chainView.EffectiveImpactPublished = true

	rankRecord := TraceCausalProjectionNode{
		Role:       TraceCausalRoleRootCauseContext,
		EvidenceID: "E-rank2",
		Subject:    "[GT]ColdPool#9-48667",
		Predicate:  "root_cause_secondary",
		Object:     "priority_inversion_candidate",
		StateKind:  "runnable",
		Rank:       2, Tier: "secondary",
		ImpactMS: 8.211, CumulativeImpactMS: 10.061, EffectiveImpactMS: 8.211,
		EffectiveImpactPublished: true,
		ChainRelevance:           "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
		LineStart: 100, LineEnd: 400, Confidence: 0.62,
	}

	occ2 := elimGapHopNode("E-hop-2", 1.200, 500, 520)
	occ3 := elimGapHopNode("E-hop-3", 0.412, 600, 620)

	out := &TraceCausalProjection{
		OnChainCauses: []TraceCausalProjectionNode{chainView, rankRecord, occ2, occ3},
	}
	traceCausalProjectionAggregateForPresentation(out)

	var cold9 []TraceCausalProjectionNode
	for _, node := range out.OnChainCauses {
		if traceCausalProjectionCanonicalNode(node.Subject) ==
			traceCausalProjectionCanonicalNode("[GT]ColdPool#9-48667") {
			cold9 = append(cold9, node)
		}
	}
	if len(cold9) != 1 {
		t.Fatalf("expected ONE surviving ColdPool#9 node after R1+R2, got %d: %+v", len(cold9), cold9)
	}
	survivor := cold9[0]
	if survivor.Rank != 2 {
		t.Fatalf("the rank seat must ride the merged survivor (R1 §29.67 backfill + R2 carry), got Rank=%d", survivor.Rank)
	}
	if survivor.Predicate != "wakeup_causal_impact" {
		t.Fatalf("the survivor keeps the hop predicate (never root_cause_*), got %q", survivor.Predicate)
	}
	if survivor.MergedCount != 3 {
		t.Fatalf("the ×N merge must report MergedCount=3, got %d", survivor.MergedCount)
	}
	hasInversionImpactPoint := false
	for _, object := range survivor.SecondaryObjects {
		if traceCausalProjectionCanonicalNode(object) == "priority_inversion_candidate" {
			hasInversionImpactPoint = true
		}
	}
	if !hasInversionImpactPoint {
		t.Fatalf("the absorbed rank record's Object must survive as an 影响点, got %v", survivor.SecondaryObjects)
	}
}

// TestElimGapOverflowFoldSeedPredicateAndTypedTruth pins carriage (2): the
// fold constructor's seed-predicate inheritance beside the typed member
// truth. A mixed overflow (one trace_gap seed + two valued members) yields
// Predicate=="trace_gap" (verbatim seed) with MergedAllDataGap==false and the
// honest valued/valueless accounting — the display layer's word-face gate
// (tool 件C) keys on exactly these fields.
func TestElimGapOverflowFoldSeedPredicateAndTypedTruth(t *testing.T) {
	gap := TraceCausalProjectionNode{
		Role:       TraceCausalRoleRootCauseContext,
		EvidenceID: "E-gap",
		Subject:    "OS_IPC_5_2838-2838",
		Predicate:  "trace_gap",
		Object:     "trace_gap",
		TypeToken:  "trace_gap",
		LineStart:  900, LineEnd: 910, Confidence: 0.6,
		ChainRelevance: "on_chain",
	}
	valued1 := TraceCausalProjectionNode{
		Role:       TraceCausalRoleRootCauseContext,
		EvidenceID: "E-v1",
		Subject:    "RenderThread-48660",
		Predicate:  "wakeup_causal_impact",
		Object:     "runnable",
		StateKind:  "runnable",
		CumulativeImpactMS: 24.000,
		LineStart:          920, LineEnd: 930, Confidence: 0.6,
		ChainRelevance: "on_chain",
	}
	valued2 := valued1
	valued2.EvidenceID = "E-v2"
	valued2.Subject = "[GT]ColdPool#1-48598"
	valued2.CumulativeImpactMS = 10.822
	valued2.LineStart, valued2.LineEnd = 940, 950

	fold := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{gap, valued1, valued2})
	if fold.Predicate != "trace_gap" {
		t.Fatalf("the fold inherits overflow[0].Predicate verbatim (carrier truth), got %q", fold.Predicate)
	}
	if fold.MergedAllDataGap {
		t.Fatalf("MergedAllDataGap must be false with 2 valued members — the typed truth the display gate reads")
	}
	if fold.MergedMaxMS != 24.000 || fold.MergedValuelessCount != 1 || fold.MergedCount != 3 {
		t.Fatalf("member accounting drifted: count=%d max=%.3f valueless=%d",
			fold.MergedCount, fold.MergedMaxMS, fold.MergedValuelessCount)
	}
	// All-gap negative arm: the typed truth flips exactly when every member
	// is a data gap.
	gap2 := gap
	gap2.EvidenceID = "E-gap2"
	gap2.Subject = "worker-9"
	gap2.LineStart, gap2.LineEnd = 960, 970
	pure := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{gap, gap2})
	if !pure.MergedAllDataGap {
		t.Fatalf("an all-gap fold must publish MergedAllDataGap=true")
	}
}
