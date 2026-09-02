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

// TestCrownsemNarrowWindowChainMemberHostStateSeatKnownGap — KNOWLEDGE PIN
// (batch-one adversarial review finding, colleague_merge_audit §40.29.1
// V-STATE-1, 2026-09-02, 待用户裁定). In the narrow window the host 61839
// becomes a CHAIN MEMBER (depth≥2 expansion at MinDurationMs 0.05): its
// VerifyClass span is priced by the R3 host-edge credential, while its own
// pre-edge runnable segment (34579.496347..496442) is NOT — the state lane's
// ONCHAIN-3c door admits only NON-chain-member hosts (RSPA owns chain
// members' state-credential vocabulary, and the segment lies outside the
// host's chain window). One host, one edge, one window, two admissions.
// This pin records the CURRENT shape so the ruling lands as a conscious
// re-pin (先红后绿), not as silent drift; it is not an endorsement.
func TestCrownsemNarrowWindowChainMemberHostStateSeatKnownGap(t *testing.T) {
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
	// CURRENT shape: the segment sits on the ◇ adjacent lane with NO credential
	// basis (「邻近支撑(无直接唤醒边)」) while the span two rows up wears the
	// host-edge credential of the very same edge.
	if rootCauseItemIsOnChain(*runnableSeat) || strings.TrimSpace(runnableSeat.OnChainBasis) != "" {
		t.Fatalf("V-STATE-1 ruling landed? the chain-member host's pre-edge runnable seat is now credentialed (%+v) — re-pin this knowledge pin against §40.29.1 and retire the gap entry", runnableSeat)
	}
	if runnableSeat.ChainRelevance != "adjacent" {
		t.Fatalf("knowledge pin drift: the pre-edge runnable seat changed lane without a ruling: %+v", runnableSeat)
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
