package tracequery

// semantic_pre_edge_pricing_tripwire_test.go — CROWNSEM-1 (user ruling
// 2026-09-02, ledger colleague_merge_audit_20260802.md §40.28 ①, restoring
// §29.88.1 R3 / §29.88.2 R4).
//
// The one credential rule for EVERY state family, as bounded by HMC377 E2
// (owner ruling 2026-11-18): a thread's wakeup of the target is the
// credential, and it prices the thread's own time inside the waits of the
// target that wakeup ended. B829/B830 had carved semantic spans out of the
// rule (effective=0 while the same host's runnable seat stayed priced by the
// same edge). These pins make the rule structural:
//
//   1. sentinel double-seat consistency — the host runnable seat and the
//      host VerifyClass span seat, same edge, same window, follow the same
//      verdict (both refuse on the tieba sentinel since E2);
//   2. census tripwire — every published rank item whose on-chain basis is a
//      host-edge or interval-relation credential and whose inside/
//      intersection projection is positive MUST publish a positive effective
//      (a future basis→zero mapping needs a registry-cited ruling, §7.2.1);
//   3. structural bound (target_wait_dependency_test.go) — every priced
//      host-edge segment lies inside a wait of the target that host ended.

import (
	"context"
	"testing"
)

func crownsemSentinelRanks(t *testing.T) []RootCauseRankResult {
	t.Helper()
	idx, err := BuildIndex(context.Background(), r3TiebaTrace)
	if err != nil {
		t.Fatal(err)
	}
	// Two sentinel shapes: the R3 acceptance window and a narrow window
	// around the VerifyClass span (34579.495841..496126) / the 61839→59566
	// credential edge (34579.496810). Both must expose the host seats under
	// ONE credential rule.
	return []RootCauseRankResult{
		BuildRootCauseRank(idx, r3RankQuery(59566, 34579.490, 34579.500)),
		BuildRootCauseRank(idx, Query{PID: 59566, TimeStart: 34579.495, TimeEnd: 34579.4975,
			MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 40}),
	}
}

// TestCrownsemSentinelHostStateAndSemanticSeatsShareOneCredentialRule — one
// credential rule for the state seat and the span seat of one host.
// RE-BASED (HMC377 E2, owner ruling 2026-11-18 "同意继续", given after being
// told it re-prices this witness to zero). Old: in the wide sentinel window
// both 61839 seats were present and priced (runnable > 0, VerifyClass 0.285)
// before the 34579.496810 edge. New: both are absent, by the same rule — that
// edge ended the target's 0.057ms sleep [34579.496753, 34579.496810] and
// neither the verification (34579.495841..496126) nor the ready time
// (34579.496347..496442) lies inside it. The meaning "state seat and span
// seat of one host follow ONE rule" is kept: they are judged by the same
// windows and both refuse.
func TestCrownsemSentinelHostStateAndSemanticSeatsShareOneCredentialRule(t *testing.T) {
	// 复核收编 (batch-one adversarial review, 2026-09-02): the two seats are
	// judged PER WINDOW.
	for wi, rank := range crownsemSentinelRanks(t) {
		for _, item := range append(append([]RootCauseRankItem(nil), rank.Items...), rank.preTruncationItems...) {
			if item.Thread.PID != 61839 {
				continue
			}
			if item.OnChainBasis == RootCauseOnChainBasisHostWakeupEdge || item.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState ||
				item.HostWakeupEdgeAnchorTs != 0 {
				t.Fatalf("window %d: no 61839 seat may take the wakeup credential — its wakeup closed a wait that holds none of its time: %+v", wi, item)
			}
		}
	}
}

// TestStateresNarrowWindowChainMemberHostResidualRunnableSeatPriced — the
// STATERES-1 residual lane (user ruling §40.30 plan A "RSPA 优先、R3 补残",
// 2026-09-02) in the narrow sentinel window, where 61839 is a chain member
// (depth-1 node on branch 2, chain window 34579.496753..34579.496810).
// RE-BASED (HMC377 E2, owner ruling 2026-11-18): old → new = span 0.285 +
// residual ready 0.095 priced at the direct edge 34579.496810 → neither
// priced. The ready segment [34579.496347 (line 5597), 34579.496442 (line
// 5614)] ends 0.311ms before the only wait its wakeup closed begins
// (34579.496753, line 5635); the residual lane can price only waits the
// expansion did not walk, and this wait IS the walked chain window. Kept
// meaning: the residual lane and the span lane read one rule; the residual
// lane still converts when the host's segment lies inside an un-walked wait
// (TestChainMemberResidualStateSeatTakesDirectEdgeCredential).
func TestStateresNarrowWindowChainMemberHostResidualRunnableSeatPriced(t *testing.T) {
	narrow := crownsemSentinelRanks(t)[1]
	var runnableSeat *RootCauseRankItem
	for i := range narrow.Items {
		item := &narrow.Items[i]
		if item.Thread.PID != 61839 {
			continue
		}
		if item.OnChainBasis == RootCauseOnChainBasisHostWakeupEdge || item.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState {
			t.Fatalf("no 61839 seat may be priced by its wakeup of the target: %+v", item)
		}
		if !rootCauseItemIsSemanticSpanWork(*item) && item.Type == "runnable_wait" &&
			item.StartTs > 34579.4963 && item.StartTs < 34579.4964 && runnableSeat == nil {
			runnableSeat = item
		}
	}
	if runnableSeat != nil && (rootCauseItemIsOnChain(*runnableSeat) || runnableSeat.HostWakeupEdgeAnchorTs != 0) {
		t.Fatalf("the ready time before the wait began keeps its home lane: %+v", runnableSeat)
	}
}

func TestCrownsemCensusCredentialedPreEdgeShareIsAlwaysPriced(t *testing.T) {
	var items []RootCauseRankItem
	for _, rank := range crownsemSentinelRanks(t) {
		items = append(items, rank.Items...)
	}
	for _, item := range items {
		switch item.OnChainBasis {
		case RootCauseOnChainBasisHostWakeupEdge, RootCauseOnChainBasisHostWakeupEdgeState,
			RootCauseOnChainBasisSemanticChainIntervalRelation:
		default:
			continue
		}
		if item.ProjectedImpactMs > 0 && item.EffectiveImpactMs <= 0 {
			t.Fatalf("credentialed seat with a positive pre-edge/intersection share published effective=0 — a basis→zero mapping needs a registry-cited ruling (§7.2.1): %+v", item)
		}
	}
}
