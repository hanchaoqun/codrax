package tracequery

import "testing"

// A priority-inversion candidate is a wording caliber, not a demotion lane.
// Once typed on-chain it competes on the ordinary root-cause board by its
// published effective attribution. Only the off-chain sibling is background.
func TestPriorityInversionCandidateOnChainRanksByEffectiveImpactOffChainStaysBackground(t *testing.T) {
	items := []RootCauseRankItem{
		{
			Type: "priority_inversion_candidate", Thread: ThreadRef{Comm: "candidate", PID: 200},
			ImpactMs: 40, CumulativeImpactMs: 40, EffectiveImpactMs: 7,
			RunnableMs: 7, ChainRelevance: "on_chain", Causality: "on_wakeup_chain", LineStart: 20,
		},
		{
			Type: "runnable_wait", Thread: ThreadRef{Comm: "runnable", PID: 201},
			ImpactMs: 6, CumulativeImpactMs: 6, EffectiveImpactMs: 6,
			RunnableMs: 6, ChainRelevance: "on_chain", Causality: "on_wakeup_chain", LineStart: 10,
		},
		{
			Type: "priority_inversion_candidate", Thread: ThreadRef{Comm: "background", PID: 900},
			ImpactMs: 100, CumulativeImpactMs: 100, EffectiveImpactMs: 100,
			RunnableMs: 100, ChainRelevance: "background", Causality: "background", LineStart: 1,
		},
	}

	sortRootCauseRankItems(items, true)
	assignRootCauseRanksAndTiers(items)

	if items[0].Thread.PID != 200 || items[0].Rank != 1 || items[0].Tier != "primary" || items[0].BackgroundRank != 0 {
		t.Fatalf("on-chain inversion candidate must lead the ordinary board: %+v", items)
	}
	if got := rootCauseEffectiveImpactMs(items[0]); got != 7 {
		t.Fatalf("candidate must rank by published effective impact, got %.3f want 7.000: %+v", got, items[0])
	}
	if items[1].Thread.PID != 201 || items[1].Rank != 2 {
		t.Fatalf("lower-effective on-chain cause should follow candidate: %+v", items)
	}
	if items[2].Thread.PID != 900 || rootCauseItemIsOnChain(items[2]) || items[2].Tier == "primary" {
		t.Fatalf("off-chain inversion candidate must stay behind the main board as background: %+v", items)
	}
	// Ranking/tiering is classification only; it must not rewrite any value.
	if items[0].ImpactMs != 40 || items[0].CumulativeImpactMs != 40 || items[0].EffectiveImpactMs != 7 ||
		items[2].ImpactMs != 100 || items[2].EffectiveImpactMs != 100 {
		t.Fatalf("rank/tier assignment mutated impact channels: %+v", items)
	}
}
