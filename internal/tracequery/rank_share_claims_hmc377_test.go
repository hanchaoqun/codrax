package tracequery

// HMC377 fix round 5 — one mechanism for every sentence that says a share of
// a row's account is held or represented by another row: the claim is typed
// where the sentence is written, resolved once against the published board,
// and written in its compaction-disclosure form when its holder is not there.
// Review round 4 reproduced three boards where the former checks (family
// presence, verbatim anchors, the ledger's owner support) disagreed with the
// board; each is pinned here, plus the claim-resolution property every board
// of TestHMC377UnwalkedShareOwnedOnceAcrossEveryArm checks.

import (
	"strings"
	"testing"
)

// hmc377R5ClaimCheck is the claim-resolution property of one published board:
// every span of every row is in its held form only when each claim it governs
// resolved to seats ON this board that hold that exact share (the claim's
// role, thread and family, its instants — or its value without instants),
// and in its disclosure form otherwise; and every holder phrase the text
// carries names a seat of the right kind on the board (the reviewer's
// phrase-level rules, independent of the typed claims).
func hmc377R5ClaimCheck(t *testing.T, board []RootCauseRankItem, dump string) {
	t.Helper()
	byKey := map[string]RootCauseRankItem{}
	for _, it := range board {
		if it.ShareHolderKey != "" {
			byKey[it.ShareHolderKey] = it
		}
	}
	for _, it := range board {
		for _, sp := range it.rspaClaimSpans {
			if !strings.Contains(it.Summary, sp.current) {
				t.Fatalf("a claim span is not in the row's summary (%q): %q\n%s", sp.current, it.Summary, dump)
			}
			resolved := true
			for _, ci := range sp.claims {
				c := it.ShareClaims[ci]
				if !c.Resolved {
					resolved = false
					continue
				}
				var heldWindows []foldInterval
				heldMs := 0.0
				for _, key := range c.HolderKeys {
					holder, ok := byKey[key]
					if !ok {
						t.Fatalf("a resolved claim names a seat that is not on the board (%q): %+v\n%s", key, c, dump)
					}
					if holder.Thread.PID != c.pid || (c.Role != RootCauseShareHolderRemainderSeat && !rootCauseItemIsOnChain(holder)) {
						t.Fatalf("a resolved claim names a seat of another thread or off the chain tier: %+v -> %s/%s\n%s", c, holder.Type, holder.Source, dump)
					}
					found := false
					for _, h := range holder.rspaHeld {
						if h.role == c.Role && h.dio == c.dio {
							found = true
							heldWindows = append(heldWindows, h.windows...)
							heldMs += h.ms
						}
					}
					if !found {
						t.Fatalf("a resolved claim names a seat that holds no share of its role %q: %s/%s\n%s", c.Role, holder.Type, holder.Source, dump)
					}
				}
				if len(c.windows) > 0 {
					if covered := foldIntervalIntersectionMs(rspaFoldCopy(heldWindows), c.windows); covered < rspaFoldLengthMs(c.windows)-0.002 {
						t.Fatalf("a resolved claim's holders hold %.3f ms of its %.3f ms instants: %+v\n%s", covered, rspaFoldLengthMs(c.windows), c, dump)
					}
				} else if heldMs < c.ShareMs-0.002 {
					t.Fatalf("a resolved claim's holders hold %.3f ms of its %.3f ms share: %+v\n%s", heldMs, c.ShareMs, c, dump)
				}
			}
			want := sp.compact
			if resolved {
				want = sp.held
			}
			if sp.current != want {
				t.Fatalf("a span is written in the wrong form (resolved=%t): %q\n%s", resolved, sp.current, dump)
			}
			if sp.represented && it.ChainAnchorRepresentedByChainSeat != resolved {
				t.Fatalf("a row wears the represented form (%t) although its holder resolved=%t: %q\n%s", it.ChainAnchorRepresentedByChainSeat, resolved, it.Summary, dump)
			}
		}
	}
	// Phrase-level rules (what the reader is told, per thread and family).
	type fam struct {
		pid int
		dio bool
	}
	chainHeld, ownSeat, clippedHeld, remainders := map[fam]float64{}, map[fam]bool{}, map[fam]bool{}, map[fam]bool{}
	for _, it := range board {
		f := fam{it.Thread.PID, rspaUnwalkedFamilyRow(it, true)}
		if it.ChainAnchorRemainderSeat {
			remainders[f] = true
		}
		if !rootCauseItemIsOnChain(it) {
			continue
		}
		switch {
		case it.ChainAnchorUnwalkedSeat || it.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState:
			ownSeat[f] = true
		default:
			chainHeld[f] += hmc377OwnerValue(it)
			if it.rspaUnwalkedHeldMs > 0 {
				clippedHeld[f] = true
			}
		}
	}
	for _, it := range board {
		f := fam{it.Thread.PID, rspaUnwalkedFamilyRow(it, true)}
		if strings.Contains(it.Summary, rspaSummaryOwnedByChainSeat) && it.ChainAnchoredMs > chainHeld[f]+0.002 {
			t.Fatalf("%s/%s hands %.3f ms %q but the chain/clipped seats on the board hold %.3f:\n%s", it.Type, it.Source, it.ChainAnchoredMs, rspaSummaryOwnedByChainSeat, chainHeld[f], dump)
		}
		if strings.Contains(it.Summary, rspaSummaryUnwalkedOwnSeat) && !ownSeat[f] {
			t.Fatalf("%s/%s says %q but no own on-chain seat of the family is on the board:\n%s", it.Type, it.Source, rspaSummaryUnwalkedOwnSeat, dump)
		}
		if (strings.Contains(it.Summary, "one seat holds both") || strings.Contains(it.Summary, "all held by that one seat")) && !clippedHeld[f] {
			t.Fatalf("%s/%s names the clipped seat holding walked + unwalked, which is not on the board:\n%s", it.Type, it.Source, dump)
		}
		if strings.Contains(it.Summary, rspaSummaryRemainderTwinPublished) && !remainders[f] {
			t.Fatalf("%s/%s says its remainder is %q, but no remainder seat of the family is on the board:\n%s", it.Type, it.Source, rspaSummaryRemainderTwinPublished, dump)
		}
	}
}

// hmc377R5ResidualOwnerUniverse — review round 4 R1: thread 200 has chain
// windows but no ready time inside them; its 4 ms inside the wait its own
// wakeup ended is priced by the STATERES-1 residual seat in the build pass.
func hmc377R5ResidualOwnerUniverse() (ChainResult, WindowStats, []RootCauseRankItem) {
	chain, stats, s := hmc377UnwalkedChain()
	chain.CausalImpacts = nil
	inventory := []foldInterval{s[1], s[2]}
	stats.runnableCensus = map[string]ThreadDuration{"200|0": {Thread: ThreadRef{Comm: "dep", PID: 200}, DurationMs: 7, anchoredMs: 0, runnableIntervals: inventory}}
	seat := hmc377UnwalkedWindowSeat(inventory)
	seat.RunnableMs, seat.ImpactMs, seat.CumulativeImpactMs, seat.EffectiveImpactMs = 7, 7, 7, 7
	seat.ledgerAnchoredRunnableMs = 0
	seat.StartTs, seat.EndTs = s[1].start, s[2].end
	return chain, stats, []RootCauseRankItem{seat}
}

// Review round 4 F2 (TestRv4CaseBClippedSeatDiesAtTheBuildCap): the case-B
// clipped seat holding walked 5 + unwalked 4 dies at the build cap; the enrich
// pass adds a satellite over exactly those 9 ms. The satellite may not claim
// the time is "all held by that one seat" (nor wear the represented form):
// it keeps its own value on the adjacent lane and says the holding seat is not
// on the published board.
func TestHMC377ClaimCaseBClippedSeatDiesAtTheBuildCap(t *testing.T) {
	for _, keepTwin := range []bool{true, false} {
		chain, stats, s := hmc377UnwalkedChain()
		chain.CausalImpacts = nil
		_, _, board := hmc377R4RankPasses(chain, stats, []RootCauseRankItem{hmc377UnwalkedWindowSeat(s)},
			func(it RootCauseRankItem) bool { return keepTwin && it.ChainAnchorRemainderSeat },
			[]RootCauseRankItem{hmc377OwnerSatellite("scheduler_latency", s[:2], 9)})
		rspaPatchSummariesForTwinVisibility(board)
		var sat *RootCauseRankItem
		for i := range board {
			if board[i].Type == "scheduler_latency" {
				sat = &board[i]
			}
		}
		if sat == nil {
			t.Fatalf("keepTwin=%t: the satellite is on the board:\n%s", keepTwin, hmc377R4Dump(board))
		}
		if strings.Contains(sat.Summary, "all held by that one seat") || sat.ChainAnchorRepresentedByChainSeat {
			t.Fatalf("keepTwin=%t: the satellite is represented by a seat the build cap removed: %q (represented=%t)", keepTwin, sat.Summary, sat.ChainAnchorRepresentedByChainSeat)
		}
		if !strings.Contains(sat.Summary, "is not on the published board") {
			t.Fatalf("keepTwin=%t: the satellite says the seat holding its time is not on the published board: %q", keepTwin, sat.Summary)
		}
		if sat.ChainRelevance != "adjacent" || !hmc377UwNear(sat.RunnableMs, 9) {
			t.Fatalf("keepTwin=%t: the satellite keeps its own value on the adjacent lane (never a second chain seat): %+v", keepTwin, *sat)
		}
		hmc377R5ClaimCheck(t, board, hmc377R4Dump(board))
	}
}

// Review round 4 F3 (TestRv4CaseBPartitionHolderDiesAtTheBuildCap, here with
// the IO partition holding the unwalked share): two case-B partition seats;
// the cap kills the IO clipped seat that holds the 3 ms inside the unwalked
// wait. The IO remainder may not hand those 3 ms "(owned by the chain seat)"
// because the D partition's clipped seat (other instants) survived.
func TestHMC377ClaimPartitionHolderDiesAtTheBuildCap(t *testing.T) {
	chain, stats, items := hmc377R4DIOUniverse(0)
	_, _, board := hmc377R4RankPasses(chain, stats, items,
		func(it RootCauseRankItem) bool { return !(it.Type == "io_wait" && rootCauseItemIsOnChain(it)) }, nil)
	rspaPatchSummariesForTwinVisibility(board)
	sawRemainder := false
	for _, it := range board {
		if it.Type != "io_wait" || !it.ChainAnchorRemainderSeat {
			continue
		}
		sawRemainder = true
		if strings.Contains(it.Summary, rspaSummaryOwnedByChainSeat) || !strings.Contains(it.Summary, rspaSummaryOwnedByChainSeatUnpublished) {
			t.Fatalf("the IO remainder hands its 3 ms to the IO clipped seat the cap removed, and must say so: %q", it.Summary)
		}
	}
	if !sawRemainder {
		t.Fatalf("the IO remainder is on the board:\n%s", hmc377R4Dump(board))
	}
	hmc377R5ClaimCheck(t, board, hmc377R4Dump(board))
}

// Review round 4 R1, engine half (TestRv4ResidualOwnerThreeWaySatellite): the
// satellite's 4 ms inside the unwalked wait is owned by the STATERES-1
// residual seat; its claim resolves to THAT seat by identity (no marker), so
// the reader can be pointed at it.
func TestHMC377ClaimResidualOwnerResolvesToTheResidualSeat(t *testing.T) {
	chain, stats, items := hmc377R5ResidualOwnerUniverse()
	_, _, board := hmc377R4RankPasses(chain, stats, items, func(RootCauseRankItem) bool { return true },
		[]RootCauseRankItem{hmc377OwnerSatellite("scheduler_latency", []foldInterval{{start: 1.052, end: 1.056, valueMs: 4}, {start: 1.070, end: 1.073, valueMs: 3}}, 7)})
	rspaPatchSummariesForTwinVisibility(board)
	var sat, residual *RootCauseRankItem
	for i := range board {
		switch {
		case board[i].Type == "scheduler_latency":
			sat = &board[i]
		case board[i].OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState && rootCauseItemIsOnChain(board[i]):
			residual = &board[i]
		}
	}
	if sat == nil || residual == nil {
		t.Fatalf("the satellite and the residual seat are on the board:\n%s", hmc377R4Dump(board))
	}
	var claim *RootCauseShareClaim
	for i := range sat.ShareClaims {
		if sat.ShareClaims[i].Part == RootCauseShareClaimUnwalked {
			claim = &sat.ShareClaims[i]
		}
	}
	if claim == nil || claim.Role != RootCauseShareHolderResidualSeat || !claim.Resolved ||
		len(claim.HolderKeys) != 1 || claim.HolderKeys[0] != residual.ShareHolderKey || !claim.Exact {
		t.Fatalf("the satellite's unwalked share resolves to the residual seat %q: %+v", residual.ShareHolderKey, sat.ShareClaims)
	}
	if !strings.Contains(sat.Summary, rspaSummaryUnwalkedOwnSeat) {
		t.Fatalf("the satellite names the seat that holds its 4 ms: %q", sat.Summary)
	}
	hmc377R5ClaimCheck(t, board, hmc377R4Dump(board))
}

// The resolution pass runs once on the final board; running it again on the
// same board changes nothing, and a later board re-resolves from the claims
// (a holder that comes back turns the disclosure back into the held form).
func TestHMC377ClaimResolutionIsIdempotentAndReResolves(t *testing.T) {
	chain, stats, s := hmc377UnwalkedChain()
	chain.CausalImpacts = nil
	board := hmc377UnwalkedBoard(chain, stats, []RootCauseRankItem{hmc377UnwalkedWindowSeat(s)})
	var clipped, twin RootCauseRankItem
	for _, it := range board {
		if it.ChainAnchorRemainderSeat {
			twin = it
		} else if it.ChainAnchorFullMs > 0 {
			clipped = it
		}
	}
	alone := []RootCauseRankItem{twin}
	rspaPatchSummariesForTwinVisibility(alone)
	first := alone[0].Summary
	rspaPatchSummariesForTwinVisibility(alone)
	if alone[0].Summary != first || !strings.Contains(first, rspaSummaryOwnedByChainSeatUnpublished) {
		t.Fatalf("a twin without its holder speaks the disclosure form, idempotently: %q / %q", first, alone[0].Summary)
	}
	both := []RootCauseRankItem{clipped, alone[0]}
	rspaPatchSummariesForTwinVisibility(both)
	if !strings.Contains(both[1].Summary, rspaSummaryOwnedByChainSeat) || !strings.Contains(both[0].Summary, rspaSummaryRemainderTwinPublished) {
		t.Fatalf("with both seats on the board both claims resolve: %q / %q", both[0].Summary, both[1].Summary)
	}
}
