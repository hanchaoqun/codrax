package tracequery

// semantic_pre_edge_pricing_tripwire_test.go — CROWNSEM-1 (user ruling
// 2026-09-02, ledger colleague_merge_audit_20260802.md §40.28 ①, restoring
// §29.88.1 R3 / §29.88.2 R4).
//
// The one credential rule for EVERY state family: 边=凭证, 边前=有效, 边后=解除.
// B829/B830 had carved semantic spans out of that rule (effective=0 while the
// same host's runnable seat stayed priced by the same edge) — the tieba
// sentinel showed both seats in one report under two rules. These pins make
// the rule structural:
//
//   1. sentinel double-seat consistency — the host runnable seat and the
//      host VerifyClass span seat, same edge, same window, must BOTH price
//      their pre-edge share;
//   2. census tripwire — every published rank item whose on-chain basis is a
//      host-edge or interval-relation credential and whose pre-edge/
//      intersection projection is positive MUST publish a positive effective
//      (a future basis→zero mapping needs a registry-cited ruling, §7.2.1).

import (
	"context"
	"math"
	"strings"
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

func crownsemHostStateSeat(item RootCauseRankItem) bool {
	return item.Thread.PID == 61839 && !rootCauseItemIsSemanticSpanWork(item) &&
		(item.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState || strings.Contains(item.Summary, "fully pre-edge"))
}

func TestCrownsemSentinelHostStateAndSemanticSeatsShareOneCredentialRule(t *testing.T) {
	// 复核收编 (batch-one adversarial review, 2026-09-02): the two seats are
	// judged PER WINDOW — mixing the wide window's state seat with the narrow
	// window's span seat would let the pin pass on two different rules.
	ranks := crownsemSentinelRanks(t)
	wide := ranks[0]
	var stateSeat, spanSeat *RootCauseRankItem
	for i := range wide.Items {
		item := &wide.Items[i]
		if item.Thread.PID != 61839 {
			continue
		}
		if crownsemHostStateSeat(*item) && stateSeat == nil {
			stateSeat = item
		}
		if item.OnChainBasis == RootCauseOnChainBasisHostWakeupEdge && strings.Contains(item.SpanName, "VerifyClass") && spanSeat == nil {
			spanSeat = item
		}
	}
	if stateSeat == nil || spanSeat == nil {
		t.Fatalf("the wide sentinel window must expose both host seats (state=%v span=%v)", stateSeat != nil, spanSeat != nil)
	}
	if stateSeat.EffectiveImpactMs <= 0 {
		t.Fatalf("host state seat must price its pre-edge share: %+v", stateSeat)
	}
	assertCrownsemSpanSeatPriced(t, spanSeat)
}

// assertCrownsemSpanSeatPriced — the VerifyClass span (34579.495841..496126,
// 0.285ms) lies entirely before the 34579.496810 edge: its whole extent is the
// pre-edge share, priced on-chain, competing on the board, wearing the R4
// credential sentence plus the mechanism disclosure (披露≠清零).
func assertCrownsemSpanSeatPriced(t *testing.T, spanSeat *RootCauseRankItem) {
	t.Helper()
	if spanSeat.EffectiveImpactMs <= 0 || math.Abs(spanSeat.EffectiveImpactMs-0.285) > 0.002 {
		t.Fatalf("host semantic span seat must price its pre-edge share (R3/R4), got effective=%.3f: %s", spanSeat.EffectiveImpactMs, spanSeat.Summary)
	}
	if spanSeat.Rank <= 0 || spanSeat.Tier == RootCauseTierContextOnly {
		t.Fatalf("a priced pre-edge semantic seat competes on the board like the state seat: rank=%d tier=%s", spanSeat.Rank, spanSeat.Tier)
	}
	if !strings.Contains(spanSeat.Summary, "pre-edge=effective") || strings.Contains(spanSeat.Summary, "effective_impact=0.000") {
		t.Fatalf("summary must speak the R4 credential rule, not the retired relation-only caliber: %s", spanSeat.Summary)
	}
	if !strings.Contains(spanSeat.Summary, "mechanism unproven") {
		t.Fatalf("the mechanism-unproven disclosure must remain on the row: %s", spanSeat.Summary)
	}
}

// TestStateresNarrowWindowChainMemberHostResidualRunnableSeatPriced — the
// V-STATE-1 knowledge pin flipped into its positive form (STATERES-1, user
// ruling §40.30 plan A "RSPA 优先、R3 补残", 2026-09-02). In the narrow window
// the host 61839 is a CHAIN MEMBER (depth-1 node on branch 2, chain window
// 34579.496753..34579.496810 — the ledger's earlier "depth≥2" wording was a
// misread of the topology); its runnable segment 34579.496347..496442
// (0.095ms) lies outside that window and before the host's direct census
// edge toward the target at 34579.496810, so RSPA had nothing to price and
// the segment used to sit on ◇ 「邻近·无直接唤醒边」 while the VerifyClass
// span two rows up wore the credential of the very same edge. Now both seats
// of one host / one edge / one window follow ONE credential rule.
func TestStateresNarrowWindowChainMemberHostResidualRunnableSeatPriced(t *testing.T) {
	narrow := crownsemSentinelRanks(t)[1]
	var spanSeat, runnableSeat *RootCauseRankItem
	for i := range narrow.Items {
		item := &narrow.Items[i]
		if item.Thread.PID != 61839 {
			continue
		}
		if item.OnChainBasis == RootCauseOnChainBasisHostWakeupEdge && strings.Contains(item.SpanName, "VerifyClass") && spanSeat == nil {
			spanSeat = item
		}
		// The host's own pre-edge runnable segment (34579.496347..496442).
		if !rootCauseItemIsSemanticSpanWork(*item) && item.Type == "runnable_wait" &&
			item.StartTs > 34579.4963 && item.StartTs < 34579.4964 && runnableSeat == nil {
			runnableSeat = item
		}
	}
	if spanSeat == nil || runnableSeat == nil {
		t.Fatalf("narrow sentinel window must expose the host span seat and its runnable seat (span=%v runnable=%v)", spanSeat != nil, runnableSeat != nil)
	}
	assertCrownsemSpanSeatPriced(t, spanSeat)
	if !rootCauseItemIsOnChain(*runnableSeat) || runnableSeat.OnChainBasis != RootCauseOnChainBasisHostWakeupEdgeState {
		t.Fatalf("STATERES-1: the chain-member host's residual pre-edge runnable seat must take the host-edge state credential: %+v", runnableSeat)
	}
	if math.Abs(runnableSeat.EffectiveImpactMs-0.095) > 0.002 || runnableSeat.HostWakeupEdgeAnchorVia != HostWakeupEdgeAnchorViaDirect ||
		math.Abs(runnableSeat.HostWakeupEdgeAnchorTs-34579.496810) > 1e-5 {
		t.Fatalf("residual seat must price its whole pre-edge share by the DIRECT census edge (via=direct, edge 34579.496810): eff=%.3f via=%q ts=%.6f",
			runnableSeat.EffectiveImpactMs, runnableSeat.HostWakeupEdgeAnchorVia, runnableSeat.HostWakeupEdgeAnchorTs)
	}
	if !strings.Contains(runnableSeat.Summary, "fully pre-edge") || !strings.Contains(runnableSeat.Summary, "via=direct") {
		t.Fatalf("residual seat must wear the R4 family disclosure with via=direct: %s", runnableSeat.Summary)
	}
	// The two seats of one host / one edge / one window now agree on the edge.
	if math.Abs(runnableSeat.HostWakeupEdgeAnchorTs-spanSeat.HostWakeupEdgeAnchorTs) > 1e-6 {
		t.Fatalf("state seat and span seat must cite the same credential edge: %.6f vs %.6f", runnableSeat.HostWakeupEdgeAnchorTs, spanSeat.HostWakeupEdgeAnchorTs)
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
