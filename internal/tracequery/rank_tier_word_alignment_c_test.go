package tracequery

import "testing"

// rank_tier_word_alignment_c_test.go — 新裁定 C「tier 词空间对齐」 pins
// (rank_order_v2_design_20260712.md §6.4/§8 R14, GREENLIT 2026-07-12; RANK-U
// Stage 2 rider, 2026-07-13).
//
// Ruling: the ladder words primary/secondary are SEAT words — only
// ordinal-holding rows (chain/◇ channels) may wear or consume the election
// ladder; ▒ rows stay off the ladder on the fixed supporting tier (⌗ rows
// already wear caliber_side). The pre-ruling friction (4165 形): with every
// chain row eaten by the skip arms, a ▒ background row fell through the
// shared ladder and wore "primary" while 通道3 publishes no ordinal — and
// every ▒ fall-through silently ATE an electionPos slot from the seated rows
// below it. The full 「▒ 永 context 词」 word form is deferred to its own
// typed-token batch (see the engine arm's 实施注: the context_only pilot ate
// valued ▒ display faces on the tieba control).
//
// MUTATION self-check: removing the background-channel arm in
// assignRootCauseRanksAndTiers reds both pins below (the ▒ row re-takes
// "primary" and the chain row demotes to "secondary").

func tierWordAlignmentItem(typ, relevance string, eff float64) RootCauseRankItem {
	item := RootCauseRankItem{
		Type:               typ,
		Thread:             ThreadRef{PID: 300, Comm: "bg"},
		ImpactMs:           eff,
		CumulativeImpactMs: eff,
		EffectiveImpactMs:  eff,
		ChainRelevance:     relevance,
		Causality:          causalityFromChainRelevance(relevance),
	}
	// The participation accessors read the per-caliber state scalars — carry
	// them so every fixture row holds a genuinely positive effective (the
	// zero-eff context arm must never be the reason a pin "passes").
	switch typ {
	case "runnable_wait":
		item.RunnableMs = eff
	case "d_state_or_io_wait":
		item.DStateMs = eff
	case "io_latency":
		item.IOWaitMs = eff
	case "binder_wait":
		item.SleepMs = eff
	}
	return item
}

// TestTierWordAlignmentBackgroundRowsWearContextWord — the 4165 friction
// form: a magnitude-dominant ▒ row ahead of the seated rows neither wears a
// ladder word nor eats a ladder slot; the chain row behind it still opens at
// "primary"/Rank=1 and an adjacent row keeps its own channel ordinal + ladder
// word (ordinal-holding rows keep tier words by ruling).
func TestTierWordAlignmentBackgroundRowsWearContextWord(t *testing.T) {
	items := []RootCauseRankItem{
		tierWordAlignmentItem("d_state_or_io_wait", "background", 54.608), // wc_srvinit 形
		tierWordAlignmentItem("runnable_wait", "on_chain", 26.392),
		tierWordAlignmentItem("io_latency", "adjacent", 0.710),
	}
	assignRootCauseRanksAndTiers(items)
	if items[0].Tier != "tertiary" || items[0].Rank != 0 {
		t.Fatalf("▒ fall-through must wear the fixed supporting tier and no ordinal (新裁定 C): %+v", items[0])
	}
	if items[1].Tier != "primary" || items[1].Rank != 1 {
		t.Fatalf("the chain row must not lose its ladder slot to a seatless ▒ row: %+v", items[1])
	}
	if items[2].Tier != "secondary" || items[2].Rank != 1 {
		t.Fatalf("an adjacent row holds its own channel ordinal and keeps its ladder word: %+v", items[2])
	}
}

// TestTierWordAlignmentAllChainEatenNeverCrownsBackground — the exact 4165
// contradiction: every chain row demoted by a skip arm (target self symptom),
// only ▒ rows remain on the ladder — none may wear "primary".
func TestTierWordAlignmentAllChainEatenNeverCrownsBackground(t *testing.T) {
	symptom := tierWordAlignmentItem("binder_wait", "on_chain", 15.758)
	symptom.SubjectIsAnalysisTarget = true
	items := []RootCauseRankItem{
		symptom,
		tierWordAlignmentItem("d_state_or_io_wait", "background", 54.701),
		tierWordAlignmentItem("runnable_wait", "background", 30.103),
	}
	assignRootCauseRanksAndTiers(items)
	for i, item := range items[1:] {
		switch item.Tier {
		case "primary", "secondary":
			t.Fatalf("▒ row %d must never wear a primary/secondary seat word (4165 假 primary 形): %+v", i+1, item)
		}
		if item.Tier != "tertiary" || item.Rank != 0 {
			t.Fatalf("▒ row %d must wear the fixed supporting tier with no ordinal: %+v", i+1, item)
		}
	}
}
