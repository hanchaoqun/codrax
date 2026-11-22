package tracequery

import (
	"math"
	"strings"
	"testing"
)

// HMC377 round-12 merge (R-377-12 ① × ③). Two round-12 lanes meet on the
// successor rule's tier 2 (rspaSuccessorTier): the holder lane re-decides
// every share relation on the publication gate's re-run and types tier 2 by
// the restriction seat's restricted instants (its own inventory), for an
// anchored and an unwalked claim alike; the carve lane gives the restriction
// seat its own subset fields and typed tier 2 by "carved anchored claim". The
// textual merge kept the holder lane's typing (it subsumes the carve lane's:
// only a restriction seat has restricted instants, and its carved claims are
// anchored or unwalked). The hand boards below pin what the merged head does
// when the gate withdraws a chain-object row that holds part of a restriction
// seat (the seat straddles it) or all of it (the seat rides as its lossless
// detail): every instant has one carrier, and the subset fields the faces
// print agree with the published values.
//
// Mutants of the merged tier-2 line (fix12/merge/resolution_mutant/):
//   - RM1 (the carve lane's "anchored && carved"): the released all-inside
//     restriction row no longer takes its instants back and the ◇ remainder
//     takes them too (D2-like board: 3.903 for a 2.903 ms account);
//   - RM2 (anchored only): the three-way seat does not take back the part an
//     unwalked seat the gate withdrew held; nothing names its carrier;
//   - RM3 (no tier 2): the remainder takes the restriction seat's instants.

// hmc377M12Restricted reads the restriction subset fields (carve shim).
func hmc377M12Restricted(it RootCauseRankItem) (account, elsewhere, here float64) {
	return hmc377R12Restricted(it)
}

func hmc377M12Sum(t *testing.T, board []RootCauseRankItem, withdrawn []bool, pid int, raw float64) {
	t.Helper()
	sum := 0.0
	for i, it := range board {
		if it.Thread.PID != pid || it.AbsorbedByRankFamily || (i < len(withdrawn) && withdrawn[i]) {
			continue
		}
		sum += it.RunnableMs
	}
	if math.Abs(sum-raw) > 0.002 {
		t.Fatalf("published carriers of the thread sum to %.3f, its ready account is %.3f", sum, raw)
	}
}

// hmc377M12ThreeWay is a thread whose ready account is 106 ms: a chain seat
// on walked W (4 ms), its own unwalked seat U (2 ms), the ◇ remainder (100 ms
// outside both) and a restriction seat of 24 ms — 2 ms inside W, all of U,
// 20 ms of the remainder's instants.
func hmc377M12ThreeWay() []RootCauseRankItem {
	walked := []foldInterval{{start: 10.000, end: 10.004}}
	unwalked := []foldInterval{{start: 10.004, end: 10.006}}
	rest := []foldInterval{{start: 10.010, end: 10.110}}
	own := RootCauseRankItem{Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: ThreadRef{PID: hmc377R12PID},
		DominantState: string(StateRunnable), RunnableMs: 2, EffectiveImpactMs: 2, ImpactMs: 2, Rank: 4,
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainAnchorUnwalkedSeat: true, LineStart: 60, LineEnd: 61}
	hmc377R12Window(&own)
	rspaHold(&own, RootCauseShareHolderUnwalkedSeat, false, unwalked, 2)
	rem := hmc377R12Remainder(rest, 100, 6)
	rem.ChainAnchorUnwalkedMs, rem.ChainAnchorOutsideMs = 2, 100
	rspaSetSentence(&rem, rspaSentence{claims: []RootCauseShareClaim{
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R12PID, false, 4, walked),
		rspaClaim(RootCauseShareClaimUnwalked, RootCauseShareHolderUnwalkedSeat, hmc377R12PID, false, 2, unwalked)}})
	restricted := []foldInterval{{start: 10.002, end: 10.006}, {start: 10.050, end: 10.070}}
	items := []RootCauseRankItem{hmc377R12Chain(walked, 4, 10), own, rem, hmc377R12Seat(restricted)}
	carveCPUConstraintReadyInstants(items)
	resolveRankShareClaims(items, nil)
	return items
}

func TestHMC377R12MergeRestrictionSeatUnderAWithdrawnChainObject(t *testing.T) {
	const tol = 0.002
	near := func(got, want float64) bool { return math.Abs(got-want) <= tol }
	t.Run("straddling seat, chain seat withdrawn", func(t *testing.T) {
		for _, tc := range []struct {
			name           string
			remainderFirst bool
		}{{"restriction seat first", false}, {"remainder first", true}} {
			t.Run(tc.name, func(t *testing.T) {
				board := hmc377MergeCarveBoard(tc.remainderFirst)
				carveCPUConstraintReadyInstants(board)
				resolveRankShareClaims(board, nil)
				withdrawn := make([]bool, len(board))
				for i, it := range board {
					withdrawn[i] = it.Type == "priority_inversion_candidate"
				}
				published, _ := hmc377R12Withdraw(append([]RootCauseRankItem(nil), board...), nil, withdrawn, nil)
				hmc377M12Sum(t, published, withdrawn, hmc377MergeCarvePID, 50)
				seat := hmc377MergeCarveRow(t, published, "cpu_affinity_or_cpuset")
				rem := hmc377MergeCarveRow(t, published, "runnable_wait")
				t.Run("restriction seat counts its whole subset", func(t *testing.T) {
					account, elsewhere, here := hmc377M12Restricted(seat)
					if !near(seat.RunnableMs, 25) || !near(account, 25) || elsewhere != 0 || here != 0 {
						t.Fatalf("restriction seat: value=%.3f account=%.3f elsewhere=%.3f here=%.3f (want 25 / 25 / nothing counted elsewhere)",
							seat.RunnableMs, account, elsewhere, here)
					}
				})
				t.Run("remainder counts its rest less the subset and the share passed back", func(t *testing.T) {
					// The rest (40) less the restriction seat's 20 is what the
					// subset clause counts here; the 5 ms of W outside the
					// restriction passed back to the remainder (ShareReturnedMs)
					// and is outside that split.
					_, elsewhere, here := hmc377M12Restricted(rem)
					if !near(rem.RunnableMs, 25) || !near(elsewhere, 20) || !near(here, 20) || !near(rem.ShareReturnedMs, 5) {
						t.Fatalf("remainder: value=%.3f elsewhere=%.3f here=%.3f returned=%.3f (want 25 / 20 / 20 / 5)",
							rem.RunnableMs, elsewhere, here, rem.ShareReturnedMs)
					}
				})
			})
		}
	})
	t.Run("three-way seat", func(t *testing.T) {
		engine := hmc377M12ThreeWay()
		for _, tc := range []struct {
			name                        string
			cut                         int
			chainPart, unwalkedResolves bool
			remValue, remReturned       float64
		}{
			{"chain seat withdrawn", 0, false, true, 82, 2},
			{"unwalked seat withdrawn", 1, true, false, 80, 0},
		} {
			t.Run(tc.name, func(t *testing.T) {
				withdrawn := make([]bool, len(engine))
				withdrawn[tc.cut] = true
				published, _ := hmc377R12Withdraw(append([]RootCauseRankItem(nil), engine...), nil, withdrawn, nil)
				hmc377M12Sum(t, published, withdrawn, hmc377R12PID, 106)
				seat, rem := published[3], published[2]
				account, elsewhere, here := hmc377M12Restricted(seat)
				if !near(seat.RunnableMs, 22) || !near(account, 24) || !near(elsewhere, 2) || !near(here, 22) {
					t.Fatalf("restriction seat: value=%.3f account=%.3f elsewhere=%.3f here=%.3f (want 22 / 24 / 2 / 22)", seat.RunnableMs, account, elsewhere, here)
				}
				for _, c := range seat.ShareClaims {
					wantResolved := (c.Part == RootCauseShareClaimAnchored) == tc.chainPart
					if c.Resolved != wantResolved {
						t.Fatalf("restriction seat %s claim resolved=%v, want %v: %+v", c.Part, c.Resolved, wantResolved, c)
					}
					if !c.Resolved && !near(c.ReturnedMs, 2) {
						t.Fatalf("the part the withdrawn seat held passes back to the restriction seat: %s claim returned %.3f, want 2", c.Part, c.ReturnedMs)
					}
				}
				if !strings.Contains(seat.Summary, "so this row counts them with its own value") {
					t.Fatalf("the restriction seat says it counts the part passed back:\n%s", seat.Summary)
				}
				_, remElsewhere, remHere := hmc377M12Restricted(rem)
				if !near(rem.RunnableMs, tc.remValue) || !near(rem.ShareReturnedMs, tc.remReturned) || !near(remElsewhere, 20) || !near(remHere, 80) {
					t.Fatalf("remainder: value=%.3f returned=%.3f elsewhere=%.3f here=%.3f", rem.RunnableMs, rem.ShareReturnedMs, remElsewhere, remHere)
				}
				// The remainder's claim on the withdrawn seat's part: the
				// restriction seat holds its restricted 2 ms as the successor.
				part := RootCauseShareClaimAnchored
				if !tc.unwalkedResolves {
					part = RootCauseShareClaimUnwalked
				}
				for _, c := range rem.ShareClaims {
					if c.Part != part {
						continue
					}
					if !near(c.ResolvedMs, 2) || len(c.HolderKeys) != 1 || c.HolderKeys[0] != seat.ShareHolderKey {
						t.Fatalf("remainder %s claim: the restriction seat %q holds its 2 ms as the successor, got resolved %.3f holders %v",
							part, seat.ShareHolderKey, c.ResolvedMs, c.HolderKeys)
					}
				}
			})
		}
	})
	t.Run("seat all inside the chain object, chain object withdrawn", func(t *testing.T) {
		// Thread ready account 3 ms: a chain seat on W (2 ms), the ◇
		// remainder (1 ms outside W) and a restriction seat of 1 ms all inside
		// W — it rides as the chain seat's lossless detail until the gate
		// withdraws that seat.
		walked := []foldInterval{{start: 10.000, end: 10.002}}
		items := []RootCauseRankItem{hmc377R12Chain(walked, 2, 10),
			hmc377R12Remainder([]foldInterval{{start: 10.010, end: 10.011}}, 1, 2),
			hmc377R12Seat([]foldInterval{{start: 10.0005, end: 10.0015}})}
		items[0].Type = "priority_inversion_candidate"
		rspaSetSentence(&items[1], rspaSentence{claims: []RootCauseShareClaim{
			rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R12PID, false, 2, walked)}})
		carveCPUConstraintReadyInstants(items)
		rank := RootCauseRankResult{Window: TimeWindow{StartTs: hmc377R12Start, EndTs: hmc377R12End}, Items: items}
		reconcileExactCrossTypeRankSeats(&rank)
		resolveRankShareClaims(rank.Items, rank.AbsorbedItems)
		if len(rank.AbsorbedItems) != 1 || rank.AbsorbedItems[0].Type != "cpu_affinity_or_cpuset" {
			t.Fatalf("engine: the restriction seat rides as the chain seat's lossless detail, absorbed=%d", len(rank.AbsorbedItems))
		}
		withdrawn := make([]bool, len(rank.Items))
		for i, it := range rank.Items {
			withdrawn[i] = it.Type == "priority_inversion_candidate"
		}
		board, detail := hmc377R12Withdraw(append([]RootCauseRankItem(nil), rank.Items...), append([]RootCauseRankItem(nil), rank.AbsorbedItems...), withdrawn, nil)
		if len(detail) != 0 {
			t.Fatalf("the restriction row returns to the board once its absorber is withdrawn: %d detail rows left", len(detail))
		}
		hmc377M12Sum(t, board, withdrawn, hmc377R12PID, 3)
		var seat, rem *RootCauseRankItem
		for i := range board {
			switch {
			case board[i].Type == "cpu_affinity_or_cpuset":
				seat = &board[i]
			case board[i].ChainAnchorRemainderSeat:
				rem = &board[i]
			}
		}
		if seat == nil || rem == nil {
			t.Fatalf("published board misses the restriction seat or the remainder")
		}
		account, elsewhere, here := hmc377M12Restricted(*seat)
		if !near(seat.RunnableMs, 1) || account != 0 || elsewhere != 0 || here != 0 {
			t.Fatalf("released restriction row: value=%.3f account=%.3f elsewhere=%.3f here=%.3f (want its own 1 ms, no subset split)", seat.RunnableMs, account, elsewhere, here)
		}
		if !near(rem.RunnableMs, 2) || !near(rem.ShareReturnedMs, 1) {
			t.Fatalf("remainder: value=%.3f returned=%.3f (want its 1 ms + the 1 ms of W outside the restriction)", rem.RunnableMs, rem.ShareReturnedMs)
		}
	})
}
