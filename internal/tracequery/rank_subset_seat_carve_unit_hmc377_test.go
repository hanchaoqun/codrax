package tracequery

import (
	"math"
	"strings"
	"testing"
)

// HMC377 round 11 (batch-owner ruling R-377-11 ②): hand-board unit pins of
// the one-carrier pass (carveCPUConstraintReadyInstants). The real-capture
// pins of the same behaviour live in rank_subset_seat_carve_hmc377_test.go;
// these two cover the branches no in-repo capture reaches (case A'
// divergence and an emptied remainder beside a chain seat in the pool).

// Case A' (the thread's chain seat publishes its own account, which diverges
// from the anchored ledger — review 10, 105 网易云 后台销毁 #4
// nputmethod.anco-38135: chain seat 7.121 against an anchored ledger of
// 7.413): the restriction seat's share inside the walked windows is left to
// the chain seat, and the seat speaks the same double-account form as the
// thread's remainder, never "held by the chain seat" at the ledger value.
// Hand board in the review-10 shape (one walked window, a divergent ◇
// remainder, a restriction seat whose instants straddle the window).
func TestHMC377R11RestrictionSeatSpeaksTheDivergentFormOfItsThread(t *testing.T) {
	const pid = 38135
	walked := []foldInterval{{start: 10.000, end: 10.010}}
	chain := RootCauseRankItem{Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: ThreadRef{PID: pid},
		DominantState: string(StateRunnable), RunnableMs: 7.121, EffectiveImpactMs: 7.121, ImpactMs: 7.121, Rank: 1,
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain"}
	rspaHold(&chain, RootCauseShareHolderChainSeat, false, walked, 7.121)
	rest := []foldInterval{{start: 10.020, end: 10.060}, {start: 10.070, end: 10.080}}
	rem := RootCauseRankItem{Type: "runnable_wait", Source: "window_stats", Thread: ThreadRef{PID: pid}, DominantState: string(StateRunnable),
		RunnableMs: 50, EffectiveImpactMs: 50, ImpactMs: 50, CumulativeImpactMs: 50, ProjectedImpactMs: 50, Rank: 2,
		ChainAnchorRemainderSeat: true, ChainAnchoredMs: 7.413, ChainAnchorFullMs: 57.413, ChainAnchorOwnershipDivergent: true,
		ChainAnchorChainLaneMs: 7.121, ChainAnchorCensusMs: 7.413, Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent"}
	rspaHold(&rem, RootCauseShareHolderRemainderSeat, false, rest, 50)
	// 7.413 inside the walked window would be the ledger; the restriction
	// covers 5ms of it and 20ms of the rest.
	restricted := []foldInterval{{start: 10.000, end: 10.005}, {start: 10.040, end: 10.060}}
	seat := RootCauseRankItem{Type: "cpu_affinity_or_cpuset", Source: "window_stats.cpu_constraints", Thread: ThreadRef{PID: pid},
		DominantState: string(StateRunnable), RunnableMs: 25, EffectiveImpactMs: 25, ImpactMs: 25, CumulativeImpactMs: 25,
		ProjectedImpactMs: 25, Rank: 3, Summary: "restriction", cpuConstraintRestrictedIntervals: restricted,
		ChainCredentialLaneDemoted: true, Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent"}
	items := []RootCauseRankItem{chain, rem, seat}
	carveCPUConstraintReadyInstants(items)
	resolveRankShareClaims(items, nil)
	got := items[2]
	t.Run("split", func(t *testing.T) {
		if !got.ChainAnchorRemainderSeat || math.Abs(got.ChainAnchoredMs-5) > 0.002 || math.Abs(got.ChainAnchorFullMs-25) > 0.002 ||
			math.Abs(got.RunnableMs-20) > 0.002 || math.Abs(got.EffectiveImpactMs-20) > 0.002 || got.ChainCredentialLaneDemoted {
			t.Fatalf("restriction seat: %+v", got)
		}
		if math.Abs(items[1].RunnableMs-30) > 0.002 {
			t.Fatalf("the remainder leaves the 20ms restricted share to the seat: %.3f", items[1].RunnableMs)
		}
	})
	t.Run("divergent form", func(t *testing.T) {
		if !got.ChainAnchorOwnershipDivergent || math.Abs(got.ChainAnchorChainLaneMs-7.121) > 0.002 || math.Abs(got.ChainAnchorCensusMs-7.413) > 0.002 {
			t.Fatalf("the seat must carry its thread's divergence verdict: %+v", got)
		}
		account := false
		for _, c := range got.ShareClaims {
			if c.Part == RootCauseShareClaimChainAccount && c.Resolved {
				account = true
			}
		}
		if !account || !strings.Contains(got.Summary, "anchored-ownership divergence: the chain seat publishes its own account 7.121ms while the anchored ledger sum is 7.413ms") {
			t.Fatalf("divergent sentence and its chain-account claim: %s / %+v", got.Summary, got.ShareClaims)
		}
	})
}

// A remainder emptied by leaving every instant to the thread's restriction
// seat is that seat's account: it rides as the restriction seat's lossless
// detail, never as the chain seat's (whose anchored-decomposition arm absorbs
// a remainder that is zero because the chain seat holds the whole account).
// Hand board in the shape of review-population window w52w1, thread
// 26043 (chain seat 3.638 + remainder 155.164 + restriction 158.802 on
// 158.802ms of ready time): every instant outside the walked window is
// restricted.
func TestHMC377R11EmptiedRemainderRidesAsTheRestrictionSeatsDetail(t *testing.T) {
	const pid = 26043
	window := func(it *RootCauseRankItem) { it.StatsWindowStartTs, it.StatsWindowEndTs = 9.9, 10.2 }
	walked := []foldInterval{{start: 10.000, end: 10.004}}
	chain := RootCauseRankItem{Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: ThreadRef{PID: pid},
		DominantState: string(StateRunnable), RunnableMs: 4, EffectiveImpactMs: 4, ImpactMs: 4, CumulativeImpactMs: 4, Rank: 1,
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain", LineStart: 10, LineEnd: 20}
	window(&chain)
	rspaHold(&chain, RootCauseShareHolderChainSeat, false, walked, 4)
	rest := []foldInterval{{start: 10.010, end: 10.110}}
	rem := RootCauseRankItem{Type: "runnable_wait", Source: "window_stats", Thread: ThreadRef{PID: pid}, DominantState: string(StateRunnable),
		RunnableMs: 100, EffectiveImpactMs: 100, ImpactMs: 100, CumulativeImpactMs: 100, ProjectedImpactMs: 100, Rank: 2,
		ChainAnchorRemainderSeat: true, ChainAnchoredMs: 4, ChainAnchorFullMs: 104, Causality: "adjacent_to_wakeup_chain",
		ChainRelevance: "adjacent", LineStart: 10, LineEnd: 20}
	window(&rem)
	rspaHold(&rem, RootCauseShareHolderRemainderSeat, false, rest, 100)
	seat := RootCauseRankItem{Type: "cpu_affinity_or_cpuset", Source: "window_stats.cpu_constraints", Thread: ThreadRef{PID: pid},
		DominantState: string(StateRunnable), RunnableMs: 104, EffectiveImpactMs: 104, ImpactMs: 104, CumulativeImpactMs: 104,
		ProjectedImpactMs: 104, Rank: 3, Summary: "restriction", cpuConstraintRestrictedIntervals: append(append([]foldInterval(nil), walked...), rest...),
		ChainCredentialLaneDemoted: true, Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent", LineStart: 15, LineEnd: 15}
	window(&seat)
	rank := RootCauseRankResult{Items: []RootCauseRankItem{chain, rem, seat}, Window: TimeWindow{StartTs: 9.9, EndTs: 10.2}}
	carveCPUConstraintReadyInstants(rank.Items)
	reconcileExactCrossTypeRankSeats(&rank)
	resolveRankShareClaims(rank.Items, rank.AbsorbedItems)
	t.Run("restriction seat publishes the rest", func(t *testing.T) {
		for _, it := range rank.Items {
			if it.Type == "cpu_affinity_or_cpuset" && math.Abs(it.EffectiveImpactMs-100) > 0.002 {
				t.Fatalf("restriction seat: %+v", it)
			}
			if it.Type == "runnable_wait" && it.ChainAnchorRemainderSeat {
				t.Fatalf("the emptied remainder must leave the board: %+v", it)
			}
		}
	})
	t.Run("detail of the restriction seat", func(t *testing.T) {
		if len(rank.AbsorbedItems) != 1 || !strings.HasPrefix(rank.AbsorbedItems[0].AbsorbedIntoFamily, "rank_pair:cpu_affinity_or_cpuset>runnable_wait|") {
			t.Fatalf("the emptied remainder rides as the restriction seat's detail, never the chain seat's: %+v", rank.AbsorbedItems)
		}
	})
}

// The restriction seat keeps the ◇ disclosure lane it rode as a whole-row
// demoted seat when it splits its instants (review-9 window 105 抖音极速版
// 终端重启 #5 udk-irq-7-76: with the board's eight remainder seats on the
// remainder lane, the seat — 2.693ms of its own restricted instants, the
// 3.913ms inside the walked windows left to its clipped chain seat — must
// not be the ninth remainder the lane cap drops).
func TestHMC377R11RestrictionSeatKeepsTheDemotedSideLane(t *testing.T) {
	var items []RootCauseRankItem
	for i := 0; i < rootCauseRankRemainderSideCap; i++ {
		items = append(items, RootCauseRankItem{Type: "runnable_wait", Source: "window_stats", Thread: ThreadRef{PID: 1000 + i},
			RunnableMs: float64(100 - i), EffectiveImpactMs: float64(100 - i), ChainAnchorRemainderSeat: true, ChainAnchoredMs: 1,
			ChainAnchorFullMs: float64(101 - i), ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain"})
	}
	seat := RootCauseRankItem{Type: "cpu_affinity_or_cpuset", Source: "window_stats.cpu_constraints", Thread: ThreadRef{PID: 76},
		RunnableMs: 2.693, EffectiveImpactMs: 2.693, ChainAnchorRemainderSeat: true, ChainAnchoredMs: 3.913, ChainAnchorFullMs: 6.606,
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain"}
	rspaHold(&seat, RootCauseShareHolderSubsetSeat, false, []foldInterval{{start: 1, end: 1.002693}}, 2.693)
	items = append(items, seat)
	out, _, _, _, _, sideTotal, sideEmitted := truncateRootCauseRankCandidatesAndSideRows(items, 12)
	kept := false
	for _, it := range out {
		if it.Thread.PID == 76 {
			kept = true
		}
	}
	if !kept || sideEmitted != sideTotal {
		t.Fatalf("the restriction seat must ride the demoted side lane beside a full remainder lane: kept=%t side %d/%d", kept, sideEmitted, sideTotal)
	}
}

// The plain remainder twin of an inversion window account whose several chain
// seats hold its walked share replaces the whole-account row on the demoted ◇
// lane and keeps that lane (review-9 window 105 蜂鸟 终端重启 TID 10566:
// binder:448_B-8516's twin must not take a remainder-lane slot from
// queued-work-loo-10586's 158.692ms remainder).
func TestHMC377R11SeveralSeatTwinKeepsTheDemotedSideLane(t *testing.T) {
	var items []RootCauseRankItem
	for i := 0; i < rootCauseRankRemainderSideCap; i++ {
		items = append(items, RootCauseRankItem{Type: "runnable_wait", Source: "window_stats", Thread: ThreadRef{PID: 1000 + i},
			RunnableMs: float64(100 - i), EffectiveImpactMs: float64(100 - i), ChainAnchorRemainderSeat: true, ChainAnchoredMs: 1,
			ChainAnchorFullMs: float64(101 - i), ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain"})
	}
	twin := RootCauseRankItem{Type: "runnable_wait", Source: "window_stats", Thread: ThreadRef{PID: 8516},
		RunnableMs: 164.064, EffectiveImpactMs: 164.064, ChainAnchorRemainderSeat: true, ChainAnchoredMs: 1.022, ChainAnchorFullMs: 165.226,
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain",
		inversionAccountRepresents: "inversion_account:pid=8516|lines=1-2|window=1.000000..2.000000"}
	items = append([]RootCauseRankItem{twin}, items...)
	out, _, _, _, _, sideTotal, sideEmitted := truncateRootCauseRankCandidatesAndSideRows(items, 12)
	if sideEmitted != sideTotal || len(out) != rootCauseRankRemainderSideCap+1 {
		t.Fatalf("the twin must ride the demoted lane, leaving the remainder lane its eight seats: side %d/%d, out %d", sideEmitted, sideTotal, len(out))
	}
}
