package types

// trace_causal_projection_elimv2_test.go — ELIM-V2 (2026-07-18): strict
// whole-tuple parse pins for the direction_conservation_excess note (the ◎
// 守恒尾行 violation transcription input) plus the 修补轮 件2 compile-side
// FixDirection carriage pins (the R1 same-fact absorb backfill and the
// semantic-span donor adoption — both through the PRODUCTION aggregate entry,
// never the raw helpers). A partial or malformed tuple could fake a violation
// claim — absence never judges.

import "testing"

func TestTraceCausalProjectionParseDirectionConservationStrictTuple(t *testing.T) {
	finding := traceCausalProjectionParseDirectionConservation("scheduling_supply@250.000@200.000@2")
	if finding == nil || finding.Direction != "scheduling_supply" ||
		finding.SumMS != 250.0 || finding.WindowMS != 200.0 || finding.SeatCount != 2 {
		t.Fatalf("the well-formed engine tuple must parse verbatim: %+v", finding)
	}
	for name, raw := range map[string]string{
		"empty":               "",
		"missing field":       "scheduling_supply@250.000@200.000",
		"extra field":         "scheduling_supply@250.000@200.000@2@x",
		"blank direction":     "@250.000@200.000@2",
		"single seat":         "scheduling_supply@250.000@200.000@1",
		"non-integer seats":   "scheduling_supply@250.000@200.000@two",
		"zero window":         "scheduling_supply@250.000@0@2",
		"sum not over window": "scheduling_supply@200.000@200.000@2",
		// 修补轮 件6②: ParseFloat accepts these spellings, NaN escapes every
		// ordering comparison and +Inf satisfies sum>window — non-finite
		// fields must never mint a finding.
		"NaN window":   "scheduling_supply@250.000@NaN@2",
		"infinite sum": "scheduling_supply@+Inf@200.000@2",
	} {
		if got := traceCausalProjectionParseDirectionConservation(raw); got != nil {
			t.Fatalf("%s: a malformed/compliant tuple must not mint a violation: %q → %+v", name, raw, got)
		}
	}
}

// elimv2FixDirectionHopNode — the ELIM-GAP carriage-1 shape (chain-view row,
// direction-bare: only RANK records carry the engine's fix_direction note).
func elimv2FixDirectionHopNode(id string, impact float64, lineStart, lineEnd int) TraceCausalProjectionNode {
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

// TestElimV2FixDirectionRidesR1Absorb — 修补轮 件2a: the engine stamps the fix
// direction on the RANK record only; when R1 folds that record into the
// direction-bare chain-view survivor (the ELIM-GAP carriage-1 production
// chain), the direction must ride the fold — the empty-slot backfill in
// traceCausalProjectionAbsorbSameFact is the carrying point (strip it → the
// merged seat strands in the ◎ 方向未定 tail).
func TestElimV2FixDirectionRidesR1Absorb(t *testing.T) {
	chainView := elimv2FixDirectionHopNode("E-hop-main", 8.211, 100, 400)
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
		FixDirection: "lock_priority",
		ImpactMS:     8.211, CumulativeImpactMS: 8.211, EffectiveImpactMS: 8.211,
		EffectiveImpactPublished: true,
		ChainRelevance:           "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
		LineStart: 100, LineEnd: 400, Confidence: 0.62,
	}

	out := &TraceCausalProjection{
		OnChainCauses: []TraceCausalProjectionNode{chainView, rankRecord},
	}
	traceCausalProjectionAggregateForPresentation(out)
	var survivors []TraceCausalProjectionNode
	for _, node := range out.OnChainCauses {
		if traceCausalProjectionCanonicalNode(node.Subject) ==
			traceCausalProjectionCanonicalNode("[GT]ColdPool#9-48667") {
			survivors = append(survivors, node)
		}
	}
	if len(survivors) != 1 {
		t.Fatalf("fixture: R1 must fold the pair into one survivor, got %d: %+v", len(survivors), survivors)
	}
	if survivors[0].Rank != 2 || survivors[0].Predicate != "wakeup_causal_impact" {
		t.Fatalf("fixture: the chain view must survive carrying the seat: %+v", survivors[0])
	}
	if survivors[0].FixDirection != "lock_priority" {
		t.Fatalf("件2a: the engine direction must ride the R1 fold onto the survivor, got %q", survivors[0].FixDirection)
	}
	// Empty-slot doctrine control: a survivor with its OWN published
	// direction never lets the absorbed side overwrite it.
	own := elimv2FixDirectionHopNode("E-hop-own", 8.211, 100, 400)
	own.EffectiveImpactMS = 8.211
	own.EffectiveImpactPublished = true
	own.FixDirection = "io_dependency"
	out = &TraceCausalProjection{OnChainCauses: []TraceCausalProjectionNode{own, rankRecord}}
	traceCausalProjectionAggregateForPresentation(out)
	if len(out.OnChainCauses) != 1 || out.OnChainCauses[0].FixDirection != "io_dependency" {
		t.Fatalf("件2a control: the survivor's own direction always wins: %+v", out.OnChainCauses)
	}
}

// TestElimV2FixDirectionRidesSemanticSpanDonor — 修补轮 件2b: the RANK-U
// bucket-copy hand-off (traceCausalProjectionUnifySemanticSpanSeats) adopts
// the classified survivor's seat onto the SemanticSpans display copy — the
// engine direction is part of that seat identity and must travel on the same
// production chain (the ✦ 语义 lane renders from SemanticSpans; without the
// carriage the adopted seat would section as 方向未定).
func TestElimV2FixDirectionRidesSemanticSpanDonor(t *testing.T) {
	bucketCopy := TraceCausalProjectionNode{
		Role:       TraceCausalRoleSemanticSpan,
		EvidenceID: "E-sem-rank",
		Subject:    "com.example.app-1234",
		Predicate:  "trace_semantic_span",
		Object:     "class_verification",
		Rank:       2, Tier: "secondary",
		FixDirection: "frequency_thermal",
		ImpactMS:     9.586, CumulativeImpactMS: 9.586, EffectiveImpactMS: 9.586,
		EffectiveImpactPublished: true,
		ChainRelevance:           "on_chain", Causality: "self_deterministic",
		LineStart: 700, LineEnd: 760, Confidence: 0.8,
	}
	displayCopy := TraceCausalProjectionNode{
		Role:       TraceCausalRoleSemanticSpan,
		EvidenceID: "E-sem-rank",
		Subject:    "com.example.app-1234",
		Predicate:  "trace_semantic_span",
		Object:     "class_verification",
		ImpactMS:   9.586, CumulativeImpactMS: 9.586,
		LineStart: 700, LineEnd: 760, Confidence: 0.8,
	}
	out := &TraceCausalProjection{
		OnChainCauses: []TraceCausalProjectionNode{bucketCopy},
		SemanticSpans: []TraceCausalProjectionNode{displayCopy},
	}
	traceCausalProjectionAggregateForPresentation(out)
	if len(out.SemanticSpans) != 1 {
		t.Fatalf("fixture: the display copy must survive, got %+v", out.SemanticSpans)
	}
	sem := out.SemanticSpans[0]
	if sem.Rank != 2 {
		t.Fatalf("fixture: the donor seat must adopt onto the display copy (RANK-U), got %+v", sem)
	}
	if sem.FixDirection != "frequency_thermal" {
		t.Fatalf("件2b: the engine direction must travel with the adopted seat, got %q", sem.FixDirection)
	}
}
