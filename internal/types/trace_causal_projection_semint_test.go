package types

// trace_causal_projection_semint_test.go — 审计 #5/#62 compile-promotion pin
// (§29.25 处置委托 + §29.26 待主会话落账, 2026-07-10): the promoted
// projected_impact/overlap notes materialize into SemanticChainProjectedMS on
// ON-CHAIN trace_semantic_span records ONLY — rank rows and off-chain
// semantic rows keep the zero fail-open, and the record's display value
// (Value/ImpactMS) stays the complete member union untouched.

import "testing"

func semIntRecord(predicate, relevance string, notes ...string) ObservationRecord {
	base := []string{"semantic_class=texture_upload", "chain_relevance=" + relevance}
	return ObservationRecord{
		Predicate: predicate,
		Subject:   "worker-200",
		Object:    "texture_upload",
		Value:     "9.300",
		Unit:      "ms",
		RichNotes: append(base, notes...),
	}
}

func TestSemanticChainProjectedPromotionScope(t *testing.T) {
	// Family record: projected_impact wins.
	node := traceCausalProjectionNodeFromRecord("ctx",
		semIntRecord("trace_semantic_span", "on_chain", "projected_impact=5.500", "overlap=5.500"))
	if node.SemanticChainProjectedMS != 5.5 {
		t.Fatalf("on-chain family record must promote projected_impact: %+v", node.SemanticChainProjectedMS)
	}
	if node.ImpactMS != 9.3 {
		t.Fatalf("the display value must stay the lossless union: %v", node.ImpactMS)
	}
	// Single-span record: the overlap note is the fallback carrier.
	node = traceCausalProjectionNodeFromRecord("ctx",
		semIntRecord("trace_semantic_span", "on_chain", "overlap=5.500"))
	if node.SemanticChainProjectedMS != 5.5 {
		t.Fatalf("on-chain single-span record must promote overlap: %v", node.SemanticChainProjectedMS)
	}
	// Off-chain semantic records never promote (no chain intersection exists).
	node = traceCausalProjectionNodeFromRecord("ctx",
		semIntRecord("trace_semantic_span", "background", "overlap=5.500"))
	if node.SemanticChainProjectedMS != 0 {
		t.Fatalf("off-chain semantic record must keep the zero fail-open: %v", node.SemanticChainProjectedMS)
	}
	// Rank rows never promote — their overlap note is a different lane.
	node = traceCausalProjectionNodeFromRecord("ctx",
		semIntRecord("root_cause_primary", "on_chain", "overlap=5.500"))
	if node.SemanticChainProjectedMS != 0 {
		t.Fatalf("rank rows must never mint the semantic intersection field: %v", node.SemanticChainProjectedMS)
	}
}

func TestCrownsemCredentialedBasesCarryPublishedEffectiveVerbatim(t *testing.T) {
	// CROWNSEM-1 (user ruling 2026-09-02, colleague_merge_audit §40.28 ①):
	// B829/B830's compile-time "relation-only ⇒ effective=0" override is
	// retired. The host-edge / interval bases are CREDENTIALS (R3/R4): the
	// producer's published effective (pre-edge share / exact intersection)
	// is carried verbatim, and a record without an effective note stays
	// unpublished — never fabricated, never zeroed by basis.
	for _, tc := range []struct {
		name, basis string
		projected   string
	}{
		{"host edge pre-span", TraceCausalOnChainBasisHostWakeupEdgeSpan, "5.500"},
		{"chain interval relation", TraceCausalOnChainBasisSemanticChainIntervalRelation, "4.600"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := semIntRecord("trace_semantic_span", "on_chain",
				"on_chain_basis="+tc.basis, "projected_impact="+tc.projected, "effective_impact_ms="+tc.projected)
			node := traceCausalProjectionNodeFromRecord(TraceCausalRoleSemanticSpan, record)
			if !node.IsHostEdgeOrIntervalCredentialedSemantic() {
				t.Fatalf("typed credential basis was not preserved: %+v", node)
			}
			if !node.EffectiveImpactPublished || node.EffectiveImpactMS == 0 {
				t.Fatalf("published pre-edge/intersection effective must be carried verbatim: published=%v value=%v", node.EffectiveImpactPublished, node.EffectiveImpactMS)
			}
			if node.ImpactMS != 9.3 {
				t.Fatalf("raw occupancy carriers must remain lossless: impact=%v", node.ImpactMS)
			}
			// No effective note ⇒ unpublished (fail-open to the display lanes),
			// never a basis-minted zero.
			legacy := traceCausalProjectionNodeFromRecord(TraceCausalRoleSemanticSpan,
				semIntRecord("trace_semantic_span", "on_chain", "on_chain_basis="+tc.basis, "projected_impact="+tc.projected))
			if legacy.EffectiveImpactPublished {
				t.Fatalf("a record without an effective note must not mint a published value: %+v", legacy)
			}
		})
	}
}
