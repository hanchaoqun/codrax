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
	// readyInsideMs / dioInsideMs: thread 200's raw ready / D-IO inventory
	// inside its walked ∪ unwalked windows — the bound of its on-chain priced
	// time per family. fullMs: its formal ready account whose published parts
	// must add back to it (0 skips that check).
	readyInsideMs, dioInsideMs, fullMs float64
	// keep / additions (HMC377 fix round 4, cross-pass universes): the board
	// runs both rank passes the way a rank build does — keep is the build
	// cap, additions are the rows the enrich pass adds.
	keep      func(RootCauseRankItem) bool
	additions func() []RootCauseRankItem
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
	affinityWithConstraint := func() RootCauseRankItem {
		aff := hmc377OwnerAffinity(12)
		aff.CPUConstraintKind = "sched_switch_next_info"
		aff.CPUConstraintAllowedCPUs = []int{7}
		aff.FixDirection = "scheduling_supply"
		return aff
	}
	chainOnly := func(it RootCauseRankItem) bool { return strings.HasPrefix(it.Source, "wakeup_chain") }
	return []hmc377OwnerUniverse{
		{name: "case A: every arm", readyInsideMs: 9, fullMs: 12, build: func() (ChainResult, WindowStats, []RootCauseRankItem) {
			chain, stats, s := base(nil)
			return chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), hmc377UnwalkedWindowSeat(s),
				hmc377OwnerSatellite("scheduler_latency", s[:2], 9), hmc377OwnerSatellite("low_frequency", s[1:], 7),
				hmc377OwnerSatellite("low_frequency", nil, 12), hmc377OwnerChurnTwin(12), hmc377OwnerAffinity(12)}
		}},
		{name: "case A: inversion window account", readyInsideMs: 9, fullMs: 12, build: func() (ChainResult, WindowStats, []RootCauseRankItem) {
			chain, stats, s := base(nil)
			return chain, stats, []RootCauseRankItem{inversionChain(), inversionWindow(s), hmc377OwnerSatellite("scheduler_latency", s, 12), hmc377OwnerChurnTwin(12)}
		}},
		{name: "case A without a formal seat", readyInsideMs: 9, fullMs: 12, build: func() (ChainResult, WindowStats, []RootCauseRankItem) {
			chain, stats, s := base(nil)
			return chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), hmc377OwnerSatellite("scheduler_latency", s[1:2], 4),
				hmc377OwnerSatellite("scheduler_latency", s, 12), hmc377OwnerChurnTwin(12), hmc377OwnerAffinity(12)}
		}},
		{name: "case A': diverging chain seat", readyInsideMs: 9, fullMs: 12, build: func() (ChainResult, WindowStats, []RootCauseRankItem) {
			chain, stats, s := base(func(c *ChainResult, _ *WindowStats) { c.CausalImpacts[0].RunnableMs = 4.5 })
			seat := hmc377UnwalkedChainSeat()
			seat.RunnableMs, seat.ImpactMs, seat.CumulativeImpactMs, seat.EffectiveImpactMs = 4.5, 4.5, 4.5, 4.5
			return chain, stats, []RootCauseRankItem{seat, hmc377UnwalkedWindowSeat(s), hmc377OwnerSatellite("scheduler_latency", s, 12), hmc377OwnerChurnTwin(12)}
		}},
		{name: "case B: no chain seat", readyInsideMs: 9, fullMs: 12, build: func() (ChainResult, WindowStats, []RootCauseRankItem) {
			chain, stats, s := base(func(c *ChainResult, _ *WindowStats) { c.CausalImpacts = nil })
			return chain, stats, []RootCauseRankItem{hmc377UnwalkedWindowSeat(s), hmc377OwnerSatellite("scheduler_latency", s[:2], 9),
				hmc377OwnerSatellite("scheduler_latency", s, 12), hmc377OwnerChurnTwin(12), hmc377OwnerAffinity(12)}
		}},
		{name: "several chain seats: inversion demotion", readyInsideMs: 9, fullMs: 12, build: func() (ChainResult, WindowStats, []RootCauseRankItem) {
			chain, stats, s := base(nil)
			a, b := hmc377UnwalkedChainSeat(), hmc377UnwalkedChainSeat()
			a.RunnableMs, a.ImpactMs, a.CumulativeImpactMs, a.EffectiveImpactMs, a.EndTs = 2, 2, 2, 2, 1.012
			b.RunnableMs, b.ImpactMs, b.CumulativeImpactMs, b.EffectiveImpactMs, b.StartTs = 3, 3, 3, 3, 1.012
			return chain, stats, []RootCauseRankItem{a, b, inversionWindow(s), hmc377OwnerSatellite("scheduler_latency", s, 12)}
		}},
		// HMC377 fix round 4 — zero-walked owners: a formal seat whose own
		// walked share is zero gives the planner's ruling to the owner (round
		// 3's arm kept it and the residual lane priced it a second time).
		{name: "case A: zero-walked ready seat of a second census group", readyInsideMs: 9, fullMs: 12, build: hmc377R4TwoGroupUniverse},
		{name: "case A': zero-walked D/IO seat beside a diverging chain seat", dioInsideMs: 5, build: func() (ChainResult, WindowStats, []RootCauseRankItem) {
			return hmc377R4DIOUniverse(1.5)
		}},
		{name: "case A: zero-walked D/IO seat beside an identity chain seat", dioInsideMs: 5, build: func() (ChainResult, WindowStats, []RootCauseRankItem) {
			return hmc377R4DIOUniverse(2)
		}},
		{name: "case B: zero-walked D/IO seat holds its share as one object", dioInsideMs: 5, build: func() (ChainResult, WindowStats, []RootCauseRankItem) {
			chain, stats, items := hmc377R4DIOUniverse(0)
			chain.CausalImpacts[0].DStateMs = 2.0
			return chain, stats, items
		}},
		// HMC377 fix round 4 — cross-pass truncation: the build cap kills the
		// own seat, the three-way remainder and the formal seat (only the
		// chain seat survives) and the enrich pass adds the thread's
		// CPU-constraint row and a satellite; or the own seat survives.
		{name: "cross-pass: the build cap kills the own seat, its remainder and the formal seat", readyInsideMs: 9,
			build: func() (ChainResult, WindowStats, []RootCauseRankItem) {
				chain, stats, s := base(nil)
				return chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), hmc377UnwalkedWindowSeat(s)}
			}, keep: chainOnly,
			additions: func() []RootCauseRankItem {
				_, _, s := base(nil)
				return []RootCauseRankItem{affinityWithConstraint(), hmc377OwnerSatellite("scheduler_latency", s, 12)}
			}},
		{name: "cross-pass: the own seat survives the build cap", readyInsideMs: 9,
			build: func() (ChainResult, WindowStats, []RootCauseRankItem) {
				chain, stats, s := base(nil)
				return chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), hmc377UnwalkedWindowSeat(s)}
			}, keep: func(it RootCauseRankItem) bool { return chainOnly(it) || it.ChainAnchorUnwalkedSeat },
			additions: func() []RootCauseRankItem {
				_, _, s := base(nil)
				return []RootCauseRankItem{affinityWithConstraint(), hmc377OwnerSatellite("scheduler_latency", s[1:], 7)}
			}},
		// HMC377 fix round 4 — partial satellites: two unwalked waits; a
		// satellite whose inventory holds only one of them carries its own
		// part (4 of the thread's 6 ms) and says so.
		{name: "partial satellite: its inventory holds one of two unwalked waits", readyInsideMs: 11, fullMs: 14,
			build: func() (ChainResult, WindowStats, []RootCauseRankItem) {
				chain, stats, s := hmc377R4TwoUnwalkedChain()
				window := hmc377UnwalkedWindowSeat(s)
				window.RunnableMs, window.ImpactMs, window.CumulativeImpactMs, window.EffectiveImpactMs, window.EndTs = 14, 14, 14, 14, 1.084
				return chain, stats, []RootCauseRankItem{hmc377UnwalkedChainSeat(), window,
					hmc377OwnerSatellite("scheduler_latency", []foldInterval{s[0], s[1], s[2]}, 12),
					hmc377OwnerSatellite("low_frequency", []foldInterval{s[3]}, 2)}
			}},
		// HMC377 fix round 5 — review round 4's boards where a holder died at
		// a cap while the claim's family kept another seat (or the ledger's
		// planned support): every claim must resolve to a seat on the
		// published board, or speak its disclosure form.
		{name: "cross-pass: the case-B clipped seat dies at the build cap under an enrich satellite", readyInsideMs: 9,
			build: func() (ChainResult, WindowStats, []RootCauseRankItem) {
				chain, stats, s := base(func(c *ChainResult, _ *WindowStats) { c.CausalImpacts = nil })
				return chain, stats, []RootCauseRankItem{hmc377UnwalkedWindowSeat(s)}
			}, keep: func(it RootCauseRankItem) bool { return it.ChainAnchorRemainderSeat },
			additions: func() []RootCauseRankItem {
				_, _, s := base(nil)
				return []RootCauseRankItem{hmc377OwnerSatellite("scheduler_latency", s[:2], 9), hmc377OwnerChurnTwin(12)}
			}},
		{name: "cross-pass: a D/IO partition holder dies at the build cap", dioInsideMs: 5,
			build:     func() (ChainResult, WindowStats, []RootCauseRankItem) { return hmc377R4DIOUniverse(0) },
			keep:      func(it RootCauseRankItem) bool { return !(it.Type == "io_wait" && rootCauseItemIsOnChain(it)) },
			additions: func() []RootCauseRankItem { return nil }},
		{name: "cross-pass: the residual-lane seat owns the share of an enrich satellite", readyInsideMs: 4,
			build: hmc377R5ResidualOwnerUniverse, keep: func(RootCauseRankItem) bool { return true },
			additions: func() []RootCauseRankItem {
				_, _, s := base(nil)
				return []RootCauseRankItem{hmc377OwnerSatellite("scheduler_latency", s[1:], 7)}
			}},
	}
}

// hmc377OwnerValue is a row's published value in its family's caliber.
func hmc377OwnerValue(it RootCauseRankItem) float64 {
	switch {
	case strings.TrimSpace(it.Type) == "priority_inversion_candidate":
		return it.GatedRunnableMs
	case it.DominantState == string(StateRunnable):
		return it.RunnableMs
	case it.DominantState == string(StateDSleep) || it.DominantState == string(StateIOWait):
		return it.DStateMs + it.IOWaitMs
	}
	return 0
}

// Property (HMC377 fix rounds 3 and 4): per thread and state family, over
// every arm and across both rank passes, the on-chain priced time never
// exceeds the raw inventory inside the walked ∪ unwalked windows; the share
// has at most one owner per board (counting the pre-truncation pool of both
// passes); every typed split adds up to its full account; every unwalked part
// a typed split names is held by the one owner seat of its thread and family,
// found by identity, which publishes that part — or, when the row's account
// holds only part of the thread's share, more than it, and the row's sentence
// says which part it covers.
func TestHMC377UnwalkedShareOwnedOnceAcrossEveryArm(t *testing.T) {
	for _, u := range hmc377OwnerUniverses() {
		t.Run(u.name, func(t *testing.T) {
			chain, stats, items := u.build()
			var board []RootCauseRankItem
			ownSeats := map[bool]int{}
			countOwn := func(rows []RootCauseRankItem, sign int) {
				for _, it := range rows {
					if it.Thread.PID == 200 && it.ChainAnchorUnwalkedSeat {
						ownSeats[rspaUnwalkedFamilyRow(it, true)] += sign
					}
				}
			}
			if u.keep == nil {
				board = hmc377UnwalkedBoard(chain, stats, items)
				countOwn(board, 1)
			} else {
				var pass1, input2 []RootCauseRankItem
				pass1, input2, board = hmc377R4RankPasses(chain, stats, items, u.keep, u.additions())
				// Seats minted by either pass: the build pass's, plus the
				// enrich pass's own mints (its output minus its input).
				countOwn(pass1, 1)
				countOwn(board, 1)
				countOwn(input2, -1)
			}
			rspaPatchSummariesForTwinVisibility(board)
			dump := hmc377R4Dump(board)
			// HMC377 fix round 5: every published claim resolves to an
			// on-board holder of that exact share, or is in disclosure form.
			hmc377R5ClaimCheck(t, board, dump)
			ready, dio := 0.0, 0.0
			owners := map[bool]int{}
			for _, it := range board {
				if it.Thread.PID != 200 || !rootCauseItemIsOnChain(it) {
					continue
				}
				if rspaUnwalkedFamilyRow(it, true) {
					dio += hmc377OwnerValue(it)
				} else {
					ready += hmc377OwnerValue(it)
				}
				if it.ChainAnchorUnwalkedSeat || it.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState {
					owners[rspaUnwalkedFamilyRow(it, true)]++
				}
			}
			if ready > u.readyInsideMs+rspaAnchorIdentityTolMs {
				t.Fatalf("on-chain priced ready time %.3f ms exceeds the %.3f ms inside the walked and unwalked windows:\n%s", ready, u.readyInsideMs, dump)
			}
			if dio > u.dioInsideMs+rspaAnchorIdentityTolMs {
				t.Fatalf("on-chain priced D/IO time %.3f ms exceeds the %.3f ms inside the walked and unwalked windows:\n%s", dio, u.dioInsideMs, dump)
			}
			for family, n := range owners {
				if n > 1 {
					t.Fatalf("%d on-chain seats price the unwalked share (dio=%t):\n%s", n, family, dump)
				}
			}
			for family, n := range ownSeats {
				if n > 1 {
					t.Fatalf("%d own seats minted for one share across the rank passes (dio=%t):\n%s", n, family, dump)
				}
			}
			// A "(owned by the chain seat)" claim hands its anchored share to
			// the on-chain seats that hold the walked share (the chain seats
			// and the clipped window seats of the family, never the unwalked
			// share's own or residual seat): they publish at least that much
			// (round 3's case B handed the zero-walked io_wait seat's 3 ms to
			// clipped seats that did not hold it — the residual lane did).
			for _, it := range board {
				if it.Thread.PID != 200 || !it.ChainAnchorRemainderSeat || !strings.Contains(it.Summary, rspaSummaryOwnedByChainSeat) {
					continue
				}
				held := 0.0
				for _, o := range board {
					if o.Thread.PID == 200 && rootCauseItemIsOnChain(o) && !o.ChainAnchorRemainderSeat && !o.ChainAnchorUnwalkedSeat &&
						o.OnChainBasis != RootCauseOnChainBasisHostWakeupEdgeState && rspaUnwalkedFamilyRow(o, true) == rspaUnwalkedFamilyRow(it, true) {
						held += hmc377OwnerValue(o)
					}
				}
				if it.ChainAnchoredMs > held+rspaAnchorIdentityTolMs {
					t.Fatalf("a remainder hands %.3f ms to the chain seat(s), which hold %.3f ms: %q\n%s", it.ChainAnchoredMs, held, it.Summary, dump)
				}
			}
			for _, it := range board {
				if it.Thread.PID != 200 || it.ChainAnchorFullMs <= 0 {
					continue
				}
				value := hmc377OwnerValue(it)
				switch {
				case it.ChainAnchorUnwalkedMs > 0:
					outside := it.ChainAnchorOutsideMs
					if !hmc377UwNear(it.ChainAnchoredMs+it.ChainAnchorUnwalkedMs+outside, it.ChainAnchorFullMs) {
						t.Fatalf("three-way parts do not add up to the full account: %+v\n%s", it, dump)
					}
					if it.ChainAnchorRemainderSeat && !hmc377UwNear(value, outside) {
						t.Fatalf("a three-way remainder publishes its outside part: %+v", it)
					}
					if !it.ChainAnchorRemainderSeat && !hmc377UwNear(value, it.ChainAnchoredMs) {
						t.Fatalf("a three-way chain seat publishes its walked part: %+v", it)
					}
					// HMC377 fix round 5: the seat holding the unwalked part is
					// the one the row's typed claim resolved to on the board
					// (the own seat or the residual-lane seat), never a marker
					// search; a claim whose holder is off the board speaks its
					// disclosure form (checked by hmc377R5ClaimCheck) and names
					// no value.
					var owner *RootCauseRankItem
					unresolved := false
					for _, c := range it.ShareClaims {
						if c.Part != RootCauseShareClaimUnwalked {
							continue
						}
						if !c.Resolved {
							unresolved = true
						}
						for i := range board {
							for _, key := range c.HolderKeys {
								if board[i].ShareHolderKey == key {
									owner = &board[i]
								}
							}
						}
					}
					if unresolved {
						continue
					}
					if owner == nil {
						t.Fatalf("the unwalked part %.3f ms names no seat that holds it:\n%s", it.ChainAnchorUnwalkedMs, dump)
					}
					ownerMs := hmc377OwnerValue(*owner)
					partial := fmt.Sprintf("this row's %.3fms is part of the %.3fms that seat holds for this thread", it.ChainAnchorUnwalkedMs, ownerMs)
					switch {
					case hmc377UwNear(ownerMs, it.ChainAnchorUnwalkedMs):
						if strings.Contains(it.Summary, "is part of the") {
							t.Fatalf("a row holding the seat's whole share makes no partial claim: %q", it.Summary)
						}
					case ownerMs > it.ChainAnchorUnwalkedMs && it.Summary != "":
						if !strings.Contains(it.Summary, partial) {
							t.Fatalf("a row holding part of the seat's %.3f ms says which part it covers (%q): %q", ownerMs, partial, it.Summary)
						}
					case ownerMs < it.ChainAnchorUnwalkedMs:
						t.Fatalf("the row names %.3f ms inside the unwalked waits, more than the owner seat holds (%.3f):\n%s", it.ChainAnchorUnwalkedMs, ownerMs, dump)
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
			if u.fullMs <= 0 {
				return
			}
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
					parts += hmc377OwnerValue(it)
				case it.Source == "window_stats" && it.Type == "runnable_wait" && (it.ChainAnchorFullMs > 0 || it.rspaUnwalkedHeldMs > 0):
					parts += it.RunnableMs
					formalSplit = true
				}
			}
			if formalSplit && !divergent && !hmc377UwNear(parts, u.fullMs) {
				t.Fatalf("published parts Σ %.3f ms != the %.3f ms full account:\n%s", parts, u.fullMs, dump)
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
