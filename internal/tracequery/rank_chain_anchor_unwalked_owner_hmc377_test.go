package tracequery

import (
	"fmt"
	"strings"
	"testing"
)

// HMC377 fix round 3: one owner for the unwalked share. The universe is
// hmc377UnwalkedChain (target 100; walked wait [1.000,1.030] and unwalked
// wait [1.050,1.060], both ended by 200's wakeup; 200's ready segments: 5 ms
// walked + 4 ms unwalked + 3 ms outside, full 12 ms).

func hmc377OwnerSatellite(typ string, intervals []foldInterval, ms float64) RootCauseRankItem {
	source := "scheduler_latency_stats"
	if typ == "low_frequency" {
		source = "window_stats.compute_supply"
	}
	return RootCauseRankItem{Type: typ, Thread: ThreadRef{Comm: "dep", PID: 200},
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
		RunnableMs: ms, ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, Source: source, Confidence: 0.78,
		familyMemberIntervals: intervals, MemberCount: len(intervals)}
}

func hmc377OwnerChurnTwin(ms float64) RootCauseRankItem {
	return RootCauseRankItem{Type: "fragmented_runnable_wait", Thread: ThreadRef{Comm: "dep", PID: 200},
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
		RunnableMs: ms, ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, Source: "window_stats.state_churn", Confidence: 0.7}
}

func hmc377OwnerAffinity(ms float64) RootCauseRankItem {
	return RootCauseRankItem{Type: "cpu_affinity_or_cpuset", Thread: ThreadRef{Comm: "dep", PID: 200},
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
		RunnableMs: ms, ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, Source: "window_stats.cpu_constraints", Confidence: 0.7}
}

// Review round 2, R1: chain seat + formal window seat + a satellite whose
// segments are the walked 5 ms and the unwalked 4 ms. The satellite's
// account is fully held by the chain seat (5) and the unwalked seat minted in
// the SAME pass (4); it must leave the chain lane (XLANE-1: a satellite never
// stands beside the seats that hold its account). Round 2 checked against a
// snapshot taken before the pass and kept 9 ms on the chain lane beside them
// (18 ms of on-chain ready time for 12 ms of raw ready time).
func TestHMC377SatelliteBesideTheUnwalkedSeatLeavesTheChainLane(t *testing.T) {
	chain, stats, segments := hmc377UnwalkedChain()
	items := hmc377UnwalkedBoard(chain, stats, []RootCauseRankItem{
		hmc377UnwalkedChainSeat(), hmc377UnwalkedWindowSeat(segments), hmc377OwnerSatellite("scheduler_latency", segments[:2], 9)})
	onChain := 0.0
	for _, it := range items {
		if it.Thread.PID == 200 && it.ChainRelevance == "on_chain" && it.DominantState == string(StateRunnable) {
			onChain += it.RunnableMs
		}
		if it.Type == "scheduler_latency" && it.ChainRelevance == "on_chain" {
			t.Fatalf("the satellite stays on the chain lane with %.3f ms beside the chain seat and the unwalked seat that hold the same instants", it.RunnableMs)
		}
		if it.Type == "scheduler_latency" && !it.ChainAnchorRepresentedByChainSeat {
			t.Fatalf("the satellite's whole account is held by the two seats — it rides as represented: %+v", it)
		}
	}
	if !hmc377UwNear(onChain, 9) {
		t.Fatalf("on-chain ready Σ for TID 200 = %.3f ms, its inside account is 9 ms", onChain)
	}
}

// Review round 2, R2: chain seat + a satellite, no formal window seat. Round
// 2 subtracted the satellite's 4 ms inside the unwalked wait and said it was
// "priced as this thread's own on-chain seat" — a seat nobody minted. The one
// owner now exists (minted by the ledger from the census) and the claim names
// it.
func TestHMC377SatelliteUnwalkedShareIsPricedByTheSeatItNames(t *testing.T) {
	chain, stats, segments := hmc377UnwalkedChain()
	items := hmc377UnwalkedBoard(chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), hmc377OwnerSatellite("scheduler_latency", segments, 12)})
	var owner *RootCauseRankItem
	for i := range items {
		if items[i].Thread.PID == 200 && items[i].OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState {
			if owner != nil {
				t.Fatalf("two seats own one share: %+v / %+v", *owner, items[i])
			}
			owner = &items[i]
		}
	}
	if owner == nil || !hmc377UwNear(owner.RunnableMs, 4) || !rootCauseItemIsOnChain(*owner) {
		t.Fatalf("the 4 ms inside the unwalked wait is priced by exactly one on-chain seat: %+v", owner)
	}
	for _, it := range items {
		if it.Type == "scheduler_latency" && it.ChainAnchorRemainderSeat {
			if !hmc377UwNear(it.RunnableMs, 3) || !strings.Contains(it.Summary, rspaSummaryUnwalkedOwnSeat) {
				t.Fatalf("the satellite remainder is the 3 ms outside and names the seat that prices the 4 ms: %+v", it)
			}
		}
	}
}

// hmc377OwnerUniverse is one arm configuration of the property test.
type hmc377OwnerUniverse struct {
	name  string
	build func() (ChainResult, WindowStats, []RootCauseRankItem)
}

func hmc377OwnerUniverses() []hmc377OwnerUniverse {
	base := func(mutate func(*ChainResult, *WindowStats)) (ChainResult, WindowStats, []foldInterval) {
		chain, stats, segments := hmc377UnwalkedChain()
		if mutate != nil {
			mutate(&chain, &stats)
		}
		return chain, stats, segments
	}
	inversionWindow := func(segments []foldInterval) RootCauseRankItem {
		inv := hmc377UnwalkedWindowSeat(segments)
		inv.Type = "priority_inversion_runnable_wait"
		inv.EffectiveImpactMs, inv.GatedRunnableMs, inv.PriorityRelationProvenLowerMs = 2.0, 2.0, 2.0
		return inv
	}
	inversionChain := func() RootCauseRankItem {
		seat := hmc377UnwalkedChainSeat()
		seat.Type = "priority_inversion_candidate"
		seat.GatedRunnableMs = 5.0
		return seat
	}
	return []hmc377OwnerUniverse{
		{"case A: every arm", func() (ChainResult, WindowStats, []RootCauseRankItem) {
			chain, stats, s := base(nil)
			return chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), hmc377UnwalkedWindowSeat(s),
				hmc377OwnerSatellite("scheduler_latency", s[:2], 9), hmc377OwnerSatellite("low_frequency", s[1:], 7),
				hmc377OwnerSatellite("low_frequency", nil, 12), hmc377OwnerChurnTwin(12), hmc377OwnerAffinity(12)}
		}},
		{"case A: inversion window account", func() (ChainResult, WindowStats, []RootCauseRankItem) {
			chain, stats, s := base(nil)
			return chain, stats, []RootCauseRankItem{inversionChain(), inversionWindow(s), hmc377OwnerSatellite("scheduler_latency", s, 12), hmc377OwnerChurnTwin(12)}
		}},
		{"case A without a formal seat", func() (ChainResult, WindowStats, []RootCauseRankItem) {
			chain, stats, s := base(nil)
			return chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), hmc377OwnerSatellite("scheduler_latency", s[1:2], 4),
				hmc377OwnerSatellite("scheduler_latency", s, 12), hmc377OwnerChurnTwin(12), hmc377OwnerAffinity(12)}
		}},
		{"case A': diverging chain seat", func() (ChainResult, WindowStats, []RootCauseRankItem) {
			chain, stats, s := base(func(c *ChainResult, _ *WindowStats) { c.CausalImpacts[0].RunnableMs = 4.5 })
			seat := hmc377UnwalkedChainSeat()
			seat.RunnableMs, seat.ImpactMs, seat.CumulativeImpactMs, seat.EffectiveImpactMs = 4.5, 4.5, 4.5, 4.5
			return chain, stats, []RootCauseRankItem{seat, hmc377UnwalkedWindowSeat(s), hmc377OwnerSatellite("scheduler_latency", s, 12), hmc377OwnerChurnTwin(12)}
		}},
		{"case B: no chain seat", func() (ChainResult, WindowStats, []RootCauseRankItem) {
			chain, stats, s := base(func(c *ChainResult, _ *WindowStats) { c.CausalImpacts = nil })
			return chain, stats, []RootCauseRankItem{hmc377UnwalkedWindowSeat(s), hmc377OwnerSatellite("scheduler_latency", s[:2], 9),
				hmc377OwnerSatellite("scheduler_latency", s, 12), hmc377OwnerChurnTwin(12), hmc377OwnerAffinity(12)}
		}},
		{"several chain seats: inversion demotion", func() (ChainResult, WindowStats, []RootCauseRankItem) {
			chain, stats, s := base(nil)
			a, b := hmc377UnwalkedChainSeat(), hmc377UnwalkedChainSeat()
			a.RunnableMs, a.ImpactMs, a.CumulativeImpactMs, a.EffectiveImpactMs, a.EndTs = 2, 2, 2, 2, 1.012
			b.RunnableMs, b.ImpactMs, b.CumulativeImpactMs, b.EffectiveImpactMs, b.StartTs = 3, 3, 3, 3, 1.012
			return chain, stats, []RootCauseRankItem{a, b, inversionWindow(s), hmc377OwnerSatellite("scheduler_latency", s, 12)}
		}},
	}
}

// Property (HMC377 fix round 3): per thread, over every arm, the on-chain
// priced ready time never exceeds the raw inventory inside the walked ∪
// unwalked windows (9 ms), every typed split adds up to its full account, and
// every part a typed split names is held by a seat on the board that
// publishes exactly that part.
func TestHMC377UnwalkedShareOwnedOnceAcrossEveryArm(t *testing.T) {
	const insideMs, fullMs = 9.0, 12.0
	for _, u := range hmc377OwnerUniverses() {
		t.Run(u.name, func(t *testing.T) {
			chain, stats, items := u.build()
			board := hmc377UnwalkedBoard(chain, stats, items)
			var dump strings.Builder
			onChain, owners := 0.0, 0
			for _, it := range board {
				if it.Thread.PID != 200 {
					continue
				}
				fmt.Fprintf(&dump, "  %s src=%s rel=%s basis=%s run=%.3f gated=%.3f anchored=%.3f unwalked=%.3f outside=%.3f full=%.3f rem=%t own=%t\n",
					it.Type, it.Source, it.ChainRelevance, it.OnChainBasis, it.RunnableMs, it.GatedRunnableMs, it.ChainAnchoredMs,
					it.ChainAnchorUnwalkedMs, it.ChainAnchorOutsideMs, it.ChainAnchorFullMs, it.ChainAnchorRemainderSeat, it.ChainAnchorUnwalkedSeat)
				if !rootCauseItemIsOnChain(it) {
					continue
				}
				switch {
				case strings.TrimSpace(it.Type) == "priority_inversion_candidate":
					onChain += it.GatedRunnableMs
				case it.DominantState == string(StateRunnable):
					onChain += it.RunnableMs
				}
				if it.ChainAnchorUnwalkedSeat {
					owners++
				}
			}
			if onChain > insideMs+rspaAnchorIdentityTolMs {
				t.Fatalf("on-chain priced ready time %.3f ms exceeds the 9 ms inside the walked and unwalked windows:\n%s", onChain, dump.String())
			}
			if owners > 1 {
				t.Fatalf("%d seats own the unwalked share:\n%s", owners, dump.String())
			}
			for _, it := range board {
				if it.Thread.PID != 200 || it.ChainAnchorFullMs <= 0 {
					continue
				}
				value := it.RunnableMs
				if strings.TrimSpace(it.Type) == "priority_inversion_candidate" {
					value = it.GatedRunnableMs
				}
				switch {
				case it.ChainAnchorUnwalkedMs > 0:
					outside := it.ChainAnchorOutsideMs
					if !hmc377UwNear(it.ChainAnchoredMs+it.ChainAnchorUnwalkedMs+outside, it.ChainAnchorFullMs) {
						t.Fatalf("three-way parts do not add up to the full account: %+v\n%s", it, dump.String())
					}
					if it.ChainAnchorRemainderSeat && !hmc377UwNear(value, outside) {
						t.Fatalf("a three-way remainder publishes its outside part: %+v", it)
					}
					if !it.ChainAnchorRemainderSeat && !hmc377UwNear(value, it.ChainAnchoredMs) {
						t.Fatalf("a three-way chain seat publishes its walked part: %+v", it)
					}
					unwalkedHeld := false
					for _, o := range board {
						if o.Thread.PID == 200 && o.ChainAnchorUnwalkedSeat && rootCauseItemIsOnChain(o) && hmc377UwNear(o.RunnableMs, it.ChainAnchorUnwalkedMs) {
							unwalkedHeld = true
						}
					}
					if !unwalkedHeld {
						t.Fatalf("the unwalked part %.3f ms names no seat that holds it:\n%s", it.ChainAnchorUnwalkedMs, dump.String())
					}
				case it.ChainAnchorRemainderSeat:
					if !hmc377UwNear(it.ChainAnchoredMs+value, it.ChainAnchorFullMs) {
						t.Fatalf("two-way parts do not add up to the full account: %+v", it)
					}
				}
			}
			// Σ of the formal account's published parts = the full account
			// (a diverging chain seat publishes its own account and is never
			// added — the double-account form says so).
			parts := 0.0
			formalSplit, divergent := false, false
			for _, it := range board {
				if it.Thread.PID != 200 {
					continue
				}
				divergent = divergent || it.ChainAnchorOwnershipDivergent
				switch {
				case it.ChainAnchorUnwalkedSeat && rootCauseItemIsOnChain(it):
					parts += it.RunnableMs
				case strings.HasPrefix(it.Source, "wakeup_chain") && rootCauseItemIsOnChain(it):
					if strings.TrimSpace(it.Type) == "priority_inversion_candidate" {
						parts += it.GatedRunnableMs
					} else {
						parts += it.RunnableMs
					}
				case it.Source == "window_stats" && it.Type == "runnable_wait" && (it.ChainAnchorFullMs > 0 || it.rspaUnwalkedHeldMs > 0):
					parts += it.RunnableMs
					formalSplit = true
				}
			}
			if formalSplit && !divergent && !hmc377UwNear(parts, fullMs) {
				t.Fatalf("published parts Σ %.3f ms != the 12 ms full account:\n%s", parts, dump.String())
			}
		})
	}
}

// Delivered-face agreement, engine half: the ⛓ chain seat's stamp and the ◇
// remainder state the same walked part (the chain seat holds only the walked
// share; round 2 told the remainder walked + unwalked and the chain seat
// walked only, so the two rows named different parts of one account).
func TestHMC377ChainSeatAndRemainderStateTheSameParts(t *testing.T) {
	chain, stats, segments := hmc377UnwalkedChain()
	seat := hmc377UnwalkedChainSeat()
	seat.Type = "priority_inversion_candidate"
	seat.GatedRunnableMs = 5.0
	board := hmc377UnwalkedBoard(chain, stats, []RootCauseRankItem{seat, hmc377UnwalkedWindowSeat(segments)})
	var stamp, rem *RootCauseRankItem
	for i := range board {
		switch {
		case board[i].Type == "priority_inversion_candidate":
			stamp = &board[i]
		case board[i].ChainAnchorRemainderSeat:
			rem = &board[i]
		}
	}
	if stamp == nil || rem == nil || stamp.ChainAnchorFullMs <= 0 {
		t.Fatalf("the chain seat carries the split stamp and a remainder exists: %+v / %+v", stamp, rem)
	}
	if !hmc377UwNear(stamp.ChainAnchoredMs, rem.ChainAnchoredMs) || !hmc377UwNear(stamp.ChainAnchoredMs, 5) {
		t.Fatalf("both rows name the walked part held by the chain seat (5 ms): chain %.3f, remainder %.3f", stamp.ChainAnchoredMs, rem.ChainAnchoredMs)
	}
	if !hmc377UwNear(stamp.ChainAnchorUnwalkedMs, 4) || !hmc377UwNear(rem.ChainAnchorUnwalkedMs, 4) ||
		!hmc377UwNear(stamp.ChainAnchorOutsideMs, rem.RunnableMs) || !hmc377UwNear(stamp.ChainAnchorFullMs, rem.ChainAnchorFullMs) {
		t.Fatalf("the typed three parts agree on both rows: chain %+v remainder %+v", stamp, rem)
	}
	if !hmc377UwNear(stamp.GatedRunnableMs, 5) {
		t.Fatalf("the inversion estimate of the walked hop never absorbs the unwalked time: gated %.3f", stamp.GatedRunnableMs)
	}
}

// Claims follow the published board: a remainder that hands its unwalked part
// to the thread's own seat says so only while that seat is on the board; and
// "(owned by the chain seat)" is verified against the walked owner alone —
// the surviving unwalked seat never stands in for a chain seat that died at
// the cap (round 2 counted it as the walked owner).
func hmc377ClaimBoard(t *testing.T) ([]RootCauseRankItem, func(func(RootCauseRankItem) bool) []RootCauseRankItem, func([]RootCauseRankItem) RootCauseRankItem) {
	t.Helper()
	chain, stats, segments := hmc377UnwalkedChain()
	board := hmc377UnwalkedBoard(chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), hmc377UnwalkedWindowSeat(segments)})
	drop := func(keep func(RootCauseRankItem) bool) []RootCauseRankItem {
		var out []RootCauseRankItem
		for _, it := range board {
			if keep(it) {
				out = append(out, it)
			}
		}
		return out
	}
	remainderOf := func(items []RootCauseRankItem) RootCauseRankItem {
		for _, it := range items {
			if it.ChainAnchorRemainderSeat {
				return it
			}
		}
		t.Fatalf("no remainder on the board")
		return RootCauseRankItem{}
	}
	return board, drop, remainderOf
}

func TestHMC377UnwalkedClaimFollowsItsSeatOntoTheBoard(t *testing.T) {
	_, drop, remainderOf := hmc377ClaimBoard(t)
	ownSeatDied := drop(func(it RootCauseRankItem) bool { return it.OnChainBasis != RootCauseOnChainBasisHostWakeupEdgeState })
	rspaPatchSummariesForTwinVisibility(ownSeatDied)
	rem := remainderOf(ownSeatDied)
	if strings.Contains(rem.Summary, "(priced as this thread's own on-chain seat)") || !strings.Contains(rem.Summary, "not on the published board") {
		t.Fatalf("the unwalked seat is off the board, the claim must say so: %q", rem.Summary)
	}
	if !strings.Contains(rem.Summary, rspaSummaryOwnedByChainSeat) {
		t.Fatalf("the chain seat is still on the board — its claim stays: %q", rem.Summary)
	}
}

func TestHMC377WalkedClaimIsNeverHeldByTheUnwalkedSeat(t *testing.T) {
	_, drop, remainderOf := hmc377ClaimBoard(t)
	chainSeatDied := drop(func(it RootCauseRankItem) bool { return !strings.HasPrefix(it.Source, "wakeup_chain") })
	rspaPatchSummariesForTwinVisibility(chainSeatDied)
	rem := remainderOf(chainSeatDied)
	if strings.Contains(rem.Summary, rspaSummaryOwnedByChainSeat) {
		t.Fatalf("the chain seat died at the cap; the surviving unwalked seat must not keep \"(owned by the chain seat)\": %q", rem.Summary)
	}
	if !strings.Contains(rem.Summary, "(priced as this thread's own on-chain seat)") {
		t.Fatalf("the unwalked seat survived — its claim stays: %q", rem.Summary)
	}
}
