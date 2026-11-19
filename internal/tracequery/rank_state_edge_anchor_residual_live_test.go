package tracequery

import (
	"math"
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
// outside those windows. Under the former [window start, latest direct edge]
// reading they were priced whole by the direct census edge at 34579.576675.
// RE-BASED (HMC377 E2, owner ruling 2026-11-18 "同意继续", given after being
// told it re-prices this witness — tieba thread 59566 — to zero): old → new =
// 0.635ms on-chain via=direct → 0ms; the account stays whole on its home lane.
// None of those IO segments lies inside a wait of 59843 that 59566's own
// wakeups ended (raw oracle: 31 wakeup rows, 30 ended a sleep, 23.103ms of
// waits, IO inside them 0.000). Kept meaning: the residual lane admits the
// chain member and judges it by the same windows as every other host.
func TestStateresTiebaChainMemberDIOResidualTakesDirectEdge(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseTiebaFixture)
	rank := BuildRootCauseRank(idx, Query{PID: 59843, TimeStart: 34579.450627, TimeEnd: 34579.595184,
		TraceFlavorHint: TraceFlavorHarmonyHitrace})
	var home *RootCauseRankItem
	for _, items := range [][]RootCauseRankItem{rank.Items, rank.preTruncationItems} {
		for i := range items {
			it := &items[i]
			if it.Thread.PID != 59566 || (it.Type != "io_wait" && it.Type != "d_state_or_io_wait") {
				continue
			}
			if it.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState || it.HostWakeupEdgeAnchorTs != 0 {
				t.Fatalf("59566's IO time outside the waits its wakeups ended must not take the wakeup credential: %+v", it)
			}
			if it.Type == "io_wait" && home == nil {
				home = it
			}
		}
	}
	if home == nil || math.Abs(home.IOWaitMs-0.635) > 0.002 {
		t.Fatalf("59566's io_wait account keeps its 0.635ms on its home lane: %+v", home)
	}
}
