package tracequery

import (
	"math"
	"strings"
	"testing"
)

// rank_state_edge_anchor_residual_live_test.go — STATERES-1 live witnesses on
// the real-trace fixtures (skip when absent): the residual lane fires on
// exactly the seats the pre-construction census predicted, prices them by
// the host's DIRECT census edge, and leaves the neighbouring forms alone.

func stateresFindSeat(items []RootCauseRankItem, typ string, pid int, startLo, startHi float64) *RootCauseRankItem {
	for i := range items {
		it := &items[i]
		if it.Type == typ && it.Thread.PID == pid && it.StartTs >= startLo && it.StartTs <= startHi && !it.ChainAnchorRemainderSeat {
			return it
		}
	}
	return nil
}

// Tieba CookieMonsterCl-59843 full window (DHM-A1a board): the chain member
// com.baidu.tieba-59566 (depth-1 node windows 34579.535253..541763 /
// 562598..564763 / 566647..568075) carries an io_wait account whose segments
// (451701..451839 / 452934..453081 / 471372..471722, 0.635ms) all lie
// outside those windows and before its DIRECT census edge toward 59843 at
// 34579.576675 — the chain-member residual population itself (RSPA priced
// nothing; the pre-batch lane refused the pid). Adversarial review of the
// batch (2026-09-02) replaced the earlier 61842 witness, which turned out to
// be a bare-census host (already priced by ONCHAIN-3c on HEAD).
func TestStateresTiebaChainMemberDIOResidualTakesDirectEdge(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseTiebaFixture)
	rank := BuildRootCauseRank(idx, Query{PID: 59843, TimeStart: 34579.450627, TimeEnd: 34579.595184,
		TraceFlavorHint: TraceFlavorHarmonyHitrace})
	var seat *RootCauseRankItem
	for i := range rank.Items {
		it := &rank.Items[i]
		if it.Thread.PID == 59566 && it.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState && !it.ChainAnchorRemainderSeat &&
			(it.Type == "io_wait" || it.Type == "d_state_or_io_wait") {
			seat = it
			break
		}
	}
	if seat == nil {
		t.Fatalf("59566's io_wait residual must take the host-edge state credential via the chain-member residual lane")
	}
	if math.Abs(seat.EffectiveImpactMs-0.635) > 0.002 || seat.HostWakeupEdgeAnchorVia != HostWakeupEdgeAnchorViaDirect ||
		math.Abs(seat.HostWakeupEdgeAnchorTs-34579.576675) > 1e-5 || !seat.ledgerAnchorStamped || seat.ledgerAnchoredDMs+seat.ledgerAnchoredIOMs != 0 {
		t.Fatalf("residual D/IO seat must price 0.635ms by the direct edge 34579.576675 with a zero-anchored ledger stamp: eff=%.3f via=%q ts=%.6f stamped=%v ledger=%.3f",
			seat.EffectiveImpactMs, seat.HostWakeupEdgeAnchorVia, seat.HostWakeupEdgeAnchorTs, seat.ledgerAnchorStamped, seat.ledgerAnchoredDMs+seat.ledgerAnchoredIOMs)
	}
	if !strings.Contains(seat.Summary, "via=direct") {
		t.Fatalf("residual seat wears the R4 family sentence with via=direct: %s", seat.Summary)
	}
}
