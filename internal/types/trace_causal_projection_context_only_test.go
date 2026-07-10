package types

import "testing"

func contextOnlyProjectionRecord(tier string) ObservationRecord {
	return ObservationRecord{
		ID:              "context-running",
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		Predicate:       "root_cause_primary",
		ClaimKey:        "root_cause_primary:running",
		Subject:         "worker-20",
		Object:          "running",
		Value:           "3.000",
		Unit:            "ms",
		Span:            ObservationSpan{LineStart: 120, LineEnd: 140},
		RichNotes: []string{
			"tier=" + tier,
			"rank=0",
			"effective_impact_ms=0",
			"impact_ms=3.000",
			"cumulative_impact_ms=3.000",
			"chain_relevance=on_chain",
			"causality=on_wakeup_chain",
			"chain_depth=1",
		},
		Confidence: 0.9,
	}
}

func TestTraceCausalProjectionContextOnlyKeepsEvidenceWithoutPrimarySeat(t *testing.T) {
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		contextOnlyProjectionRecord(TraceCausalTierContextOnly),
	})
	if got.PrimaryRootCause != nil || len(got.PrimaryRootCauses) != 0 {
		t.Fatalf("context-only root_cause_primary must not enter the primary bucket: %+v", got.PrimaryRootCauses)
	}
	if len(got.OnChainCauses) != 1 {
		t.Fatalf("context-only hand-off must retain one on-chain evidence seat: %+v", got.OnChainCauses)
	}
	node := got.OnChainCauses[0]
	if !node.IsContextOnlyRow() || node.Role != TraceCausalRoleRootCauseContext {
		t.Fatalf("typed context identity/role was not preserved: %+v", node)
	}
	if node.EvidenceID != "context-running" || node.LineStart != 120 || node.LineEnd != 140 {
		t.Fatalf("context-only projection lost evidence provenance: %+v", node)
	}
	if node.Rank != 0 || node.EffectiveImpactMS != 0 || node.ImpactMS != 3 {
		t.Fatalf("context-only row changed its producer calibers: %+v", node)
	}
}

func TestTraceCausalProjectionContextOnlyTokenIsExact(t *testing.T) {
	for _, tier := range []string{"context", "context_only_extra", "Context_Only"} {
		node := TraceCausalProjectionNode{Tier: tier}
		if node.IsContextOnlyRow() {
			t.Fatalf("non-exact tier %q must not become context-only", tier)
		}
	}
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		contextOnlyProjectionRecord("primary"),
	})
	if got.PrimaryRootCause == nil || len(got.PrimaryRootCauses) != 1 {
		t.Fatalf("ordinary root_cause_primary classification must remain unchanged: %+v", got)
	}
}

func TestTraceCausalProjectionContextOnlyPreservesOffChainBucket(t *testing.T) {
	for _, relevance := range []string{"adjacent", "background"} {
		record := contextOnlyProjectionRecord(TraceCausalTierContextOnly)
		for i, note := range record.RichNotes {
			if note == "chain_relevance=on_chain" {
				record.RichNotes[i] = "chain_relevance=" + relevance
			}
		}
		got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{record})
		var rows []TraceCausalProjectionNode
		if relevance == "adjacent" {
			rows = got.AdjacentCauses
		} else {
			rows = got.BackgroundCauses
		}
		if len(rows) != 1 || !rows[0].IsContextOnlyRow() {
			t.Fatalf("context-only relevance=%q must keep its off-chain bucket: %+v", relevance, got)
		}
		if len(got.OnChainCauses) != 0 || len(got.PrimaryRootCauses) != 0 {
			t.Fatalf("off-chain context must not be promoted to chain/primary: %+v", got)
		}
	}
}

func TestTraceCausalProjectionContextOnlyCausalHopKeepsHopRole(t *testing.T) {
	record := contextOnlyProjectionRecord(TraceCausalTierContextOnly)
	record.ID = "context-hop"
	record.Predicate = "wakeup_causal_impact"
	record.ClaimKey = "wakeup_causal_impact:worker-20"
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{record})
	if len(got.SupportingHops) != 1 {
		t.Fatalf("context-only causal hop must keep its SupportingHops edge seat: %+v", got.SupportingHops)
	}
	hop := got.SupportingHops[0]
	if hop.Role != TraceCausalRoleCausalHop || !hop.IsContextOnlyRow() || hop.EvidenceID != "context-hop" {
		t.Fatalf("context-only tier must not erase causal-hop identity/provenance: %+v", hop)
	}
}
