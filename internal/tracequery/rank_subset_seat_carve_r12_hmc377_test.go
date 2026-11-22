package tracequery

import (
	"math"
	"strings"
	"testing"
)

// HMC377 round 12 (batch-owner ruling R-377-12 ③ and ⑧, 2026-11-22): the
// restriction subset has its own fields, and every branch of the one-carrier
// pass (carveCPUConstraintReadyInstants) that changes a value, a carrier or a
// word is pinned by a shape a mutant of that branch fails.
//
// Round 11 wrote the restriction seat's split in the thread's same-source
// split fields (ChainAnchorFullMs / ChainAnchoredMs / ChainAnchorRemainderSeat
// and the three-way parts) with subset meaning, so every face read the subset
// as the thread's whole-window account and its ◇ pair partner. Real windows
// (review-population, fa2f01465): w54w1 nputmethod.anco-38135 restriction row
// 「全窗36.968ms=锚定7.413ms+本行其余29.555ms」 beside the thread's 105.717;
// n21w daily_control-26026 restriction row paired as the thread's ◇ seat,
// 「合计还原全窗账 182.107ms」 on a 196.790ms account (logs/final/fullword_*,
// out/new/head/head_n21w.fence_zh.txt).

const (
	hmc377R12PID   = 501
	hmc377R12Start = 9.9
	hmc377R12End   = 10.2
)

func hmc377R12Window(it *RootCauseRankItem) {
	it.StatsWindowStartTs, it.StatsWindowEndTs = hmc377R12Start, hmc377R12End
}

// hmc377R12Chain is a chain seat holding the walked share in windows w.
func hmc377R12Chain(w []foldInterval, ms float64, line int) RootCauseRankItem {
	it := RootCauseRankItem{Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: ThreadRef{PID: hmc377R12PID},
		DominantState: string(StateRunnable), RunnableMs: ms, EffectiveImpactMs: ms, ImpactMs: ms, CumulativeImpactMs: ms,
		ProjectedImpactMs: ms, Rank: 1, Causality: "on_wakeup_chain", ChainRelevance: "on_chain", LineStart: line, LineEnd: line + 5}
	hmc377R12Window(&it)
	rspaHold(&it, RootCauseShareHolderChainSeat, false, w, ms)
	return it
}

// hmc377R12Remainder is the thread's ◇ remainder holding the rest in w.
func hmc377R12Remainder(w []foldInterval, rest, anchored float64) RootCauseRankItem {
	it := RootCauseRankItem{Type: "runnable_wait", Source: "window_stats", Thread: ThreadRef{PID: hmc377R12PID}, DominantState: string(StateRunnable),
		RunnableMs: rest, EffectiveImpactMs: rest, ImpactMs: rest, CumulativeImpactMs: rest, ProjectedImpactMs: rest, Score: rest / 10, Rank: 2,
		ChainAnchorRemainderSeat: true, ChainAnchoredMs: anchored, ChainAnchorFullMs: rest + anchored,
		Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent", LineStart: 30, LineEnd: 40}
	hmc377R12Window(&it)
	rspaHold(&it, RootCauseShareHolderRemainderSeat, false, w, rest)
	return it
}

// hmc377R12Seat is the thread's CPU-restriction seat over its restricted
// instants, demoted whole to ◇ by the RSPA lane arm.
func hmc377R12Seat(restricted []foldInterval) RootCauseRankItem {
	ms := rspaFoldLengthMs(rspaFoldCopy(restricted))
	it := RootCauseRankItem{Type: "cpu_affinity_or_cpuset", Source: "window_stats.cpu_constraints", Thread: ThreadRef{PID: hmc377R12PID},
		DominantState: string(StateRunnable), RunnableMs: ms, EffectiveImpactMs: ms, ImpactMs: ms, CumulativeImpactMs: ms,
		ProjectedImpactMs: ms, Score: ms / 20, Rank: 3, Summary: "restriction", cpuConstraintRestrictedIntervals: restricted,
		ChainCredentialLaneDemoted: true, Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent", LineStart: 50, LineEnd: 50}
	hmc377R12Window(&it)
	return it
}

func hmc377R12Near(got, want float64) bool { return math.Abs(got-want) <= 0.002 }

// The restriction seat carries its own typed subset fields; the thread's
// same-source split fields keep the thread-account meaning (they stay on the
// thread's ◇ remainder and never appear on the restriction seat).
func TestHMC377R12RestrictionSeatCarriesItsOwnSubsetFields(t *testing.T) {
	walked := []foldInterval{{start: 10.000, end: 10.004}}
	rest := []foldInterval{{start: 10.010, end: 10.110}}
	// 2ms of the restriction inside the walked window, 20ms in the rest.
	restricted := []foldInterval{{start: 10.002, end: 10.004}, {start: 10.050, end: 10.070}}
	t.Run("two-way", func(t *testing.T) {
		items := []RootCauseRankItem{hmc377R12Chain(walked, 4, 10), hmc377R12Remainder(rest, 100, 4), hmc377R12Seat(restricted)}
		carveCPUConstraintReadyInstants(items)
		resolveRankShareClaims(items, nil)
		seat, rem := items[2], items[1]
		t.Run("restriction seat", func(t *testing.T) {
			account, elsewhere, here := hmc377R12Restricted(seat)
			if !hmc377R12Near(account, 22) || !hmc377R12Near(elsewhere, 2) || !hmc377R12Near(here, 20) || !hmc377R12Near(seat.RunnableMs, 20) {
				t.Fatalf("restriction seat subset fields: account=%.3f elsewhere=%.3f here=%.3f value=%.3f", account, elsewhere, here, seat.RunnableMs)
			}
			if seat.ChainAnchorRemainderSeat || seat.ChainAnchoredMs != 0 || seat.ChainAnchorFullMs != 0 ||
				seat.ChainAnchorUnwalkedMs != 0 || seat.ChainAnchorOutsideMs != 0 || seat.ChainAnchorOwnershipDivergent {
				t.Fatalf("the restriction seat must never carry the thread's same-source split fields: %+v", seat)
			}
		})
		t.Run("remainder", func(t *testing.T) {
			_, elsewhere, here := hmc377R12Restricted(rem)
			if !hmc377R12Near(rem.RunnableMs, 80) || !hmc377R12Near(elsewhere, 20) || !hmc377R12Near(here, 80) {
				t.Fatalf("remainder: value=%.3f elsewhere=%.3f here=%.3f", rem.RunnableMs, elsewhere, here)
			}
			if !rem.ChainAnchorRemainderSeat || !hmc377R12Near(rem.ChainAnchorFullMs, 104) || !hmc377R12Near(rem.ChainAnchoredMs, 4) {
				t.Fatalf("the remainder keeps the thread's split: %+v", rem)
			}
		})
	})
	t.Run("three-way", func(t *testing.T) {
		// The thread's own seat inside the waits the chain did not walk holds
		// [10.004, 10.006]; the restriction covers 2ms of it as well.
		unwalked := []foldInterval{{start: 10.004, end: 10.006}}
		own := RootCauseRankItem{Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: ThreadRef{PID: hmc377R12PID},
			DominantState: string(StateRunnable), RunnableMs: 2, EffectiveImpactMs: 2, ImpactMs: 2, Rank: 4,
			Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainAnchorUnwalkedSeat: true, LineStart: 60, LineEnd: 61}
		hmc377R12Window(&own)
		rspaHold(&own, RootCauseShareHolderUnwalkedSeat, false, unwalked, 2)
		threeWay := append(append([]foldInterval(nil), restricted...), unwalked...)
		items := []RootCauseRankItem{hmc377R12Chain(walked, 4, 10), own, hmc377R12Remainder(rest, 100, 4), hmc377R12Seat(threeWay)}
		carveCPUConstraintReadyInstants(items)
		resolveRankShareClaims(items, nil)
		seat := items[3]
		account, elsewhere, here := hmc377R12Restricted(seat)
		if !hmc377R12Near(account, 24) || !hmc377R12Near(elsewhere, 4) || !hmc377R12Near(here, 20) || !hmc377R12Near(seat.RunnableMs, 20) {
			t.Fatalf("three-way restriction seat: account=%.3f elsewhere=%.3f here=%.3f value=%.3f", account, elsewhere, here, seat.RunnableMs)
		}
		// Round 11 wrote the three-way parts on the seat
		// (ChainAnchorUnwalkedMs 2 / ChainAnchorOutsideMs 20) — the thread's
		// three-way split fields, never the subset's (review-11 mutant P26
		// dropped them unnoticed).
		if seat.ChainAnchorUnwalkedMs != 0 || seat.ChainAnchorOutsideMs != 0 || seat.ChainAnchorFullMs != 0 {
			t.Fatalf("the restriction seat must not carry the thread's three-way split: %+v", seat)
		}
		parts := map[RootCauseShareClaimPart]float64{}
		for _, c := range seat.ShareClaims {
			if c.Resolved {
				parts[c.Part] += c.ShareMs
			}
		}
		if !hmc377R12Near(parts[RootCauseShareClaimAnchored], 2) || !hmc377R12Near(parts[RootCauseShareClaimUnwalked], 2) {
			t.Fatalf("each part counted elsewhere is a resolved claim of its own: %+v", seat.ShareClaims)
		}
		t.Run("one holder cut", func(t *testing.T) {
			// The cap cuts the unwalked seat: its part returns to the
			// restriction seat; only the walked part is counted elsewhere.
			board := []RootCauseRankItem{items[0], items[2], items[3]}
			resolveRankShareClaims(board, nil)
			cut := board[2]
			account, elsewhere, here := hmc377R12Restricted(cut)
			if !hmc377R12Near(account, 24) || !hmc377R12Near(elsewhere, 2) || !hmc377R12Near(here, 22) || !hmc377R12Near(cut.RunnableMs, 22) {
				t.Fatalf("restriction seat with its unwalked holder cut: account=%.3f elsewhere=%.3f here=%.3f value=%.3f", account, elsewhere, here, cut.RunnableMs)
			}
		})
	})
	t.Run("donghu", func(t *testing.T) {
		rank := hmc377R11Board(t, hmc377R11DonghuFixture, Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
			MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
		rank2955 := hmc377R11Board(t, hmc377R11DonghuFixture, Query{PID: 2955, TimeStart: 13762.791708, TimeEnd: 13763.024898,
			MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
		find := func(items []RootCauseRankItem, pid int, typ string, remainder bool) *RootCauseRankItem {
			for i := range items {
				if items[i].Thread.PID == pid && items[i].Type == typ && items[i].ChainAnchorRemainderSeat == remainder {
					return &items[i]
				}
			}
			return nil
		}
		for _, tc := range []struct {
			name                     string
			items                    []RootCauseRankItem
			pid                      int
			account, elsewhere, here float64
		}{
			{"JankManager-9655", rank.Items, 9655, 27.507, 1.759, 25.748},
			{"logd.writer-9163", rank2955.Items, 9163, 48.519, 0.033, 48.486},
			{"hilogcat-9503", rank2955.Items, 9503, 17.292, 0.026, 17.266},
		} {
			t.Run(tc.name, func(t *testing.T) {
				seat := find(tc.items, tc.pid, "cpu_affinity_or_cpuset", false)
				if seat == nil {
					t.Fatalf("%s restriction seat not on the board (or still wearing the remainder marker)", tc.name)
				}
				account, elsewhere, here := hmc377R12Restricted(*seat)
				if !hmc377R12Near(account, tc.account) || !hmc377R12Near(elsewhere, tc.elsewhere) || !hmc377R12Near(here, tc.here) ||
					!hmc377R12Near(seat.EffectiveImpactMs, tc.here) {
					t.Fatalf("%s: account=%.3f elsewhere=%.3f here=%.3f value=%.3f", tc.name, account, elsewhere, here, seat.EffectiveImpactMs)
				}
				if seat.ChainAnchoredMs != 0 || seat.ChainAnchorFullMs != 0 || seat.ChainAnchorUnwalkedMs != 0 || seat.ChainAnchorOutsideMs != 0 {
					t.Fatalf("%s: the restriction seat carries the thread's split fields: %+v", tc.name, seat)
				}
			})
		}
		t.Run("JankManager-9655/remainder", func(t *testing.T) {
			rem := find(rank.Items, 9655, "runnable_wait", true)
			if rem == nil {
				t.Fatal("remainder missing")
			}
			_, elsewhere, here := hmc377R12Restricted(*rem)
			if !hmc377R12Near(elsewhere, 25.748) || !hmc377R12Near(here, 3.684) ||
				!hmc377R12Near(rem.ChainAnchorFullMs, 31.191) || !hmc377R12Near(rem.ChainAnchoredMs, 1.759) {
				t.Fatalf("remainder: elsewhere=%.3f here=%.3f %+v", elsewhere, here, rem)
			}
		})
		t.Run("JankManager-9655/the chain seat's rest resolves to both rows that carry it", func(t *testing.T) {
			// Review-11 mutant P34 (the restriction seat keeps no hold of the
			// instants it takes from the remainder) left this claim unresolved
			// with the remainder as its only holder
			// (review-pins/probe/p34_probe.log).
			rem := find(rank.Items, 9655, "runnable_wait", true)
			seat := find(rank.Items, 9655, "cpu_affinity_or_cpuset", false)
			if rem == nil || seat == nil {
				t.Fatal("witness rows missing")
			}
			var claim *RootCauseShareClaim
			for i := range rank.Items {
				it := &rank.Items[i]
				if it.Thread.PID != 9655 || it.Type != "priority_inversion_candidate" {
					continue
				}
				for j := range it.ShareClaims {
					if it.ShareClaims[j].Part == RootCauseShareClaimRemainder {
						claim = &it.ShareClaims[j]
					}
				}
			}
			if claim == nil || !claim.Resolved || !hmc377R12Near(claim.ShareMs, 29.432) {
				t.Fatalf("the ⛓ seat's remainder claim must resolve: %+v", claim)
			}
			holders := strings.Join(claim.HolderKeys, ",")
			if !strings.Contains(","+holders+",", ","+rem.ShareHolderKey+",") || !strings.Contains(","+holders+",", ","+seat.ShareHolderKey+",") {
				t.Fatalf("the rest is held by the remainder %q and the restriction seat %q: holders %v", rem.ShareHolderKey, seat.ShareHolderKey, claim.HolderKeys)
			}
		})
	})
}

// The split's typed numbers follow the one resolver: a share whose holder is
// not on the published board returns to the row that handed it over, and the
// row then says nothing is counted elsewhere (R11T-6 / review-11 mutant T02:
// the display counted an unresolved share as 另行计入).
func TestHMC377R12CountedElsewhereFollowsTheResolution(t *testing.T) {
	rank := hmc377R11Board(t, hmc377R11DonghuFixture, Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	remIdx, seatIdx, chainIdx := -1, -1, -1
	for i, it := range rank.Items {
		if it.Thread.PID != 9655 {
			continue
		}
		switch {
		case it.Type == "runnable_wait" && it.ChainAnchorRemainderSeat:
			remIdx = i
		case it.Type == "cpu_affinity_or_cpuset":
			seatIdx = i
		case it.Type == "priority_inversion_candidate":
			chainIdx = i
		}
	}
	if remIdx < 0 || seatIdx < 0 || chainIdx < 0 {
		t.Fatalf("witness rows missing: remainder=%d restriction=%d chain=%d", remIdx, seatIdx, chainIdx)
	}
	without := func(cut int) []RootCauseRankItem {
		board := make([]RootCauseRankItem, 0, len(rank.Items)-1)
		for i, it := range rank.Items {
			if i != cut {
				board = append(board, it)
			}
		}
		resolveRankShareClaims(board, nil)
		return board
	}
	pick := func(board []RootCauseRankItem, typ string) *RootCauseRankItem {
		for i := range board {
			if board[i].Thread.PID == 9655 && board[i].Type == typ && (typ != "runnable_wait" || board[i].ChainAnchorRemainderSeat) {
				return &board[i]
			}
		}
		return nil
	}
	t.Run("restriction seat cut: the remainder counts its whole rest", func(t *testing.T) {
		rem := pick(without(seatIdx), "runnable_wait")
		if rem == nil {
			t.Fatal("remainder missing")
		}
		_, elsewhere, here := hmc377R12Restricted(*rem)
		if !hmc377R12Near(rem.RunnableMs, 29.432) || elsewhere != 0 || here != 0 {
			t.Fatalf("remainder: value=%.3f elsewhere=%.3f here=%.3f", rem.RunnableMs, elsewhere, here)
		}
	})
	t.Run("chain seat cut: the restriction seat counts its whole restricted account", func(t *testing.T) {
		seat := pick(without(chainIdx), "cpu_affinity_or_cpuset")
		if seat == nil {
			t.Fatal("restriction seat missing")
		}
		account, elsewhere, here := hmc377R12Restricted(*seat)
		if !hmc377R12Near(seat.RunnableMs, 27.507) || !hmc377R12Near(account, 27.507) || elsewhere != 0 || here != 0 {
			t.Fatalf("restriction seat: value=%.3f account=%.3f elsewhere=%.3f here=%.3f", seat.RunnableMs, account, elsewhere, here)
		}
	})
	t.Run("gate withdrawal of the chain seat: the restriction seat takes its instants back", func(t *testing.T) {
		board := append([]RootCauseRankItem(nil), rank.Items...)
		withdrawn := make([]bool, len(board))
		withdrawn[chainIdx] = true
		WithdrawRankShareHolders(board, nil, withdrawn, nil)
		account, elsewhere, here := hmc377R12Restricted(board[seatIdx])
		if !hmc377R12Near(board[seatIdx].RunnableMs, 27.507) || !hmc377R12Near(account, 27.507) || elsewhere != 0 || here != 0 {
			t.Fatalf("restriction seat: value=%.3f account=%.3f elsewhere=%.3f here=%.3f", board[seatIdx].RunnableMs, account, elsewhere, here)
		}
	})
}

// Every guard of the pass that decides which seat is carved and against which
// holds (review-11 mutants P22, P23, P24, P25, P32 survived every suite).
func TestHMC377R12CarveGuardsArePinned(t *testing.T) {
	walked := []foldInterval{{start: 10.000, end: 10.004}}
	rest := []foldInterval{{start: 10.010, end: 10.110}}
	restricted := []foldInterval{{start: 10.002, end: 10.004}, {start: 10.050, end: 10.070}}
	untouched := func(t *testing.T, items []RootCauseRankItem, seatAt, remAt int) {
		t.Helper()
		seat := items[seatAt]
		account, elsewhere, _ := hmc377R12Restricted(seat)
		if !hmc377R12Near(seat.RunnableMs, 22) || len(seat.ShareClaims) != 0 || account != 0 || elsewhere != 0 ||
			!strings.HasPrefix(seat.Summary, "restriction") || strings.Contains(seat.Summary, "under this CPU restriction") {
			t.Fatalf("the seat must be left as minted: %+v", seat)
		}
		if remAt >= 0 && (!hmc377R12Near(items[remAt].RunnableMs, 100) || len(items[remAt].ShareClaims) != 0) {
			t.Fatalf("the remainder must keep its instants: %+v", items[remAt])
		}
	}
	t.Run("background seat is never carved (P22)", func(t *testing.T) {
		// A background-lane seat publishes a capped impact on a lane ◎ does
		// not count: it is never a carrier another seat could leave
		// instants to.
		seat := hmc377R12Seat(restricted)
		seat.ChainRelevance = "background"
		items := []RootCauseRankItem{hmc377R12Chain(walked, 4, 10), hmc377R12Remainder(rest, 100, 4), seat}
		carveCPUConstraintReadyInstants(items)
		resolveRankShareClaims(items, nil)
		untouched(t, items, 2, 1)
	})
	t.Run("a seat whose impact is not its instants is never carved (P23)", func(t *testing.T) {
		seat := hmc377R12Seat(restricted)
		seat.ImpactMs = 11
		items := []RootCauseRankItem{hmc377R12Chain(walked, 4, 10), hmc377R12Remainder(rest, 100, 4), seat}
		carveCPUConstraintReadyInstants(items)
		resolveRankShareClaims(items, nil)
		untouched(t, items, 2, 1)
	})
	t.Run("a thread whose holds carry no instants is left as minted (P25)", func(t *testing.T) {
		// The chain seat's account has no segment inventory (a windowless
		// hold): the pass cannot tell which restricted instants it holds,
		// so it touches neither the seat nor the remainder (fail open).
		chain := hmc377R12Chain(nil, 4, 10)
		items := []RootCauseRankItem{chain, hmc377R12Remainder(rest, 100, 4), hmc377R12Seat(restricted)}
		carveCPUConstraintReadyInstants(items)
		resolveRankShareClaims(items, nil)
		untouched(t, items, 2, 1)
	})
	t.Run("D/IO and semantic holds are not ready time (P32)", func(t *testing.T) {
		// The thread's only holds over the restricted instants are a D/IO
		// family hold and a semantic span hold: neither is ready time, so
		// the seat keeps every instant.
		dio := RootCauseRankItem{Type: "d_state_or_io_wait", Source: "wakeup_chain.causal_impacts", Thread: ThreadRef{PID: hmc377R12PID},
			DStateMs: 30, EffectiveImpactMs: 30, ImpactMs: 30, Rank: 1, Causality: "on_wakeup_chain", ChainRelevance: "on_chain", LineStart: 70, LineEnd: 71}
		hmc377R12Window(&dio)
		rspaHold(&dio, RootCauseShareHolderChainSeat, true, []foldInterval{{start: 10.000, end: 10.080}}, 30)
		sem := RootCauseRankItem{Type: "jit_compile", Source: "wakeup_chain.causal_impacts", Thread: ThreadRef{PID: hmc377R12PID},
			EffectiveImpactMs: 30, ImpactMs: 30, Rank: 2, Causality: "on_wakeup_chain", ChainRelevance: "on_chain", LineStart: 80, LineEnd: 81}
		hmc377R12Window(&sem)
		sem.rspaHeld = []rspaHeldShare{{role: RootCauseShareHolderResidualSeat, semantic: true, windows: []foldInterval{{start: 10.000, end: 10.080}}, ms: 30}}
		items := []RootCauseRankItem{dio, sem, hmc377R12Seat(restricted)}
		carveCPUConstraintReadyInstants(items)
		resolveRankShareClaims(items, nil)
		untouched(t, items, 2, -1)
	})
	t.Run("a seat inside two chain-object rows hands each its share, never rides as one row's detail (P24)", func(t *testing.T) {
		// Every restricted instant lies inside the chain object, but in two
		// rows (the chain seat and the thread's own unwalked seat): no one
		// row represents the seat, so it hands each row its share and keeps
		// no instant of its own. Riding as one row's detail would leave the
		// other row's share on two carriers.
		unwalked := []foldInterval{{start: 10.020, end: 10.024}}
		own := RootCauseRankItem{Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: ThreadRef{PID: hmc377R12PID},
			DominantState: string(StateRunnable), RunnableMs: 4, EffectiveImpactMs: 4, ImpactMs: 4, Rank: 4,
			Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainAnchorUnwalkedSeat: true, LineStart: 60, LineEnd: 61}
		hmc377R12Window(&own)
		rspaHold(&own, RootCauseShareHolderUnwalkedSeat, false, unwalked, 4)
		inside := []foldInterval{{start: 10.000, end: 10.004}, {start: 10.020, end: 10.024}}
		rank := RootCauseRankResult{Items: []RootCauseRankItem{hmc377R12Chain(walked, 4, 10), own, hmc377R12Seat(inside)},
			Window: TimeWindow{StartTs: hmc377R12Start, EndTs: hmc377R12End}}
		carveCPUConstraintReadyInstants(rank.Items)
		reconcileExactCrossTypeRankSeats(&rank)
		resolveRankShareClaims(rank.Items, rank.AbsorbedItems)
		var seat *RootCauseRankItem
		for i := range rank.Items {
			if rank.Items[i].Type == "cpu_affinity_or_cpuset" {
				seat = &rank.Items[i]
			}
		}
		if seat == nil {
			t.Fatalf("the seat must stay on the board: %+v", rank.AbsorbedItems)
		}
		if seat.RunnableMs > 0.002 || seat.EffectiveImpactMs > 0.002 || strings.Contains(seat.Summary, "lossless detail") {
			t.Fatalf("the seat hands both rows their shares (0ms of its own): value=%.3f %s", seat.RunnableMs, seat.Summary)
		}
		resolved := 0
		for _, c := range seat.ShareClaims {
			if c.Resolved && hmc377R12Near(c.ShareMs, 4) {
				resolved++
			}
		}
		if resolved != 2 {
			t.Fatalf("one resolved claim per holding row: %+v", seat.ShareClaims)
		}
	})
}

// The RSPA lane arm's words for a CPU-restriction seat on the in-repo donghu
// board (review-11 mutants P41 and P42 changed them there:
// review-pins/probe/cpu_sentence_probe.log).
func TestHMC377R12RestrictionLaneWordsOnTheRealBoard(t *testing.T) {
	rank := hmc377R11Board(t, hmc377R11DonghuFixture, Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	seat := func(pid int) *RootCauseRankItem {
		for i := range rank.Items {
			if rank.Items[i].Thread.PID == pid && rank.Items[i].Type == "cpu_affinity_or_cpuset" {
				return &rank.Items[i]
			}
		}
		return nil
	}
	t.Run("a split seat speaks its split on its own base, never the whole-row demotion (P42)", func(t *testing.T) {
		jank := seat(9655)
		if jank == nil {
			t.Fatal("JankManager-9655 restriction seat missing")
		}
		if strings.Contains(jank.Summary, "no chain credential for the full account") ||
			strings.Contains(jank.Summary, "the whole row rides the adjacent lane with every published value unchanged") ||
			!strings.Contains(jank.Summary, "this row counts the ready time under this CPU restriction that no on-chain seat of this thread publishes (27.507ms under the restriction in all)") {
			t.Fatalf("restriction seat sentence: %s", jank.Summary)
		}
	})
	t.Run("a seat the pass leaves whole says how much of it lies inside the chain windows (P41)", func(t *testing.T) {
		// udk-irq-12-92: no seat of the thread on this board holds its
		// instants, so the pass leaves the row whole on the adjacent lane;
		// the lane sentence still measures its own restricted instants
		// inside the dependency windows.
		irq := seat(92)
		if irq == nil {
			t.Fatal("udk-irq-12-92 restriction seat missing")
		}
		if !strings.Contains(irq.Summary, "0.252ms of this affinity/cpuset satellite's own restricted instants lie inside them") {
			t.Fatalf("lane sentence: %s", irq.Summary)
		}
	})
}
