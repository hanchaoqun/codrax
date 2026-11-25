package tracequery

// HMC377 ruling R-377-18 ③ (2026-11-25): pins of round-15 lines that the
// round-15 engine verifier found unguarded by the default suite. Test only;
// no product line changes.
//
// The verifier's mutants of rank_share_claims.go changed a published value on
// its universes (~/hmc-work/hmc377/fix15/verify-engine/newuniv/tq/
// zz_ve15_universe_export.go) and still passed every default test:
//   - E1 "claim roles only for claims without instants" (rspaWithdrawnAccountOf
//     reads the role of a claim only when the claim carries no instants):
//     V9censusMix, withdraw=true, 4.806 ms against raw 2.903 ms;
//   - E4 "only anchored claims own instants" (rspaDetailOwnsInstants counts
//     the returned and unheld rest of an anchored claim only): V10remRest,
//     withdraw=true, 1.903 ms against raw 2.903 ms.
// Both shapes are built here with the in-repo builders of the round-11..15
// tests and run through the production enrich order and the gate's typed
// decision; hmc377R15Publish also checks the family keys, the riding counts
// and that a second publication equals the first.
//
// E3 "no clamp" (rspaClaimHeldParts keeps a holder's value without instants
// whole instead of at most the claim's share): the held parts are read by one
// consumer only, rspaReleasedDetailHost, which picks the row a released
// detail rides under (the largest part; equal parts: the earlier board row).
// The detail it places owns no instant after the re-run, so it rides as a
// lossless detail (its minted value is kept and never counted) whatever its
// host: the clamp cannot change a published value in ms (every row keeps its
// run and effective values under either host; checked on the shape below).
// It can change which row hosts the detail — the family key the detail rides
// under and the hosts' riding counts — when two rows hold the claim: an
// earlier row whose instants cover it whole and a later row whose instants
// miss it and whose hold without instants in the claim's role exceeds the
// share. With the clamp both hold the whole share (equal parts: the earlier
// row hosts); without it the later row's value wins. That shape is built
// below (the later row is a successor of two withdrawn seats, one with
// instants and one without, as in Y3mix2: constructed by hand, since no row
// the engine mints holds one share with instants and one without) and pins
// the host.

import (
	"math"
	"testing"
)

// R-377-18 ③a (E1, universe V9censusMix): a withdrawn census inversion
// account whose claim carries instants is carried by the row that took the
// chain seat's share through a hold without instants. The roles of a
// withdrawn account come from all its claims, with or without instants.
func TestHMC377R18WithdrawnAccountReadsTheRoleOfAClaimWithInstants(t *testing.T) {
	// The chain inversion seat holds 1.903 without instants. A census
	// inversion ready account of the same time is represented by that seat:
	// it holds nothing and its claim names the chain seat's share WITH its
	// instants (W). The gate withdraws both. The thread's ◇ remainder (own
	// rest 1.000, held without instants) takes the chain seat's share by
	// value, so it carries the census account: the census row's scheduler
	// twin rides under it and the thread's 2.903 has one priced carrier.
	inv := hmc377R15WithoutInstants(hmc377R11Inv(1.903, hmc377R11W(), 10, 20), RootCauseShareHolderChainSeat)
	p := hmc377R13Census(hmc377R11W(), 40)
	rem := hmc377R11Remainder(1.0, 1.903, 2.903, 10.5, 70, 80)
	rspaSetSentence(&rem, rspaRemainderSentence(hmc377R11Dep, "runnable (scheduling-pressure candidate)", 1.0, 1.903, 2.903,
		hmc377R15ClaimWithoutInstants(RootCauseShareHolderChainSeat, 1.903)))
	rem = hmc377R15WithoutInstants(rem, RootCauseShareHolderRemainderSeat)
	twin := hmc377R13SchedTwin(p)
	const name = "V9censusMix"
	rank, pub := hmc377R15Publish(t, name, []RootCauseRankItem{hmc377R11OtherSeat(), inv, p, twin, rem}, hmc377R13IsAnyInversion)
	census := rank.Items[2]
	if census.Type != "priority_inversion_runnable_wait" || len(census.rspaHeld) != 0 || len(census.ShareClaims) != 1 || len(census.ShareClaims[0].windows) == 0 || census.RankFamilyKey == "" {
		t.Fatalf("%s precondition: the census row holds nothing, claims with instants and owns its twin\n%s", name, hmc377R15Rows(rank))
	}
	if len(rank.Items[1].rspaHeld) != 1 || len(rank.Items[1].rspaHeld[0].windows) != 0 {
		t.Fatalf("%s precondition: the chain seat holds its share without instants\n%s", name, hmc377R15Rows(rank))
	}
	priced := hmc377R14Priced(pub)
	if len(priced) != 1 || priced[0].Type != "runnable_wait" || math.Abs(priced[0].RunnableMs-2.903) > 0.0005 {
		t.Errorf("%s: priced rows of 6326 %s, want the ◇ remainder alone at 2.903", name, hmc377R14Describe(priced))
	}
	if _, onBoard := hmc377R15TwinOnBoard(pub, twin.LineStart); onBoard {
		t.Errorf("%s: the census row's twin is published as a second priced row\n%s", name, hmc377R15Rows(pub))
	}
	if host, ok := hmc377R15TwinHost(pub, twin.LineStart); !ok || host.LineStart != rem.LineStart || host.Tier == RootCauseTierContextOnly {
		t.Errorf("%s: the census row's twin rides under lines %d (found %t), want the ◇ remainder that took the chain seat's share (lines %d)\n%s",
			name, host.LineStart, ok, rem.LineStart, hmc377R15Rows(pub))
	}
	hmc377R15Carried(t, name, pub, 2.903)
}

// R-377-18 ③b (E4, universe V10remRest): the unheld rest of a detail's
// REMAINDER claim is the detail's own: when the gate withdraws the ◇
// remainder row (an inversion row of the R4 lane arm) and nobody takes its
// rest, the R-377-4 inversion account returns to the board with that rest.
func TestHMC377R18RemainderClaimRestReturnsTheDetail(t *testing.T) {
	// The chain inversion seat stands (1.903 at W) and hosts the thread's
	// inversion account (2.903: W anchored, rest R = [10.500, 10.501) on its
	// remainder claim). The ◇ remainder holds R and is an inversion row the
	// gate withdraws; no published row holds R.
	inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
	const key = "inversion_account:pid=6326|r18v10"
	inv.inversionAccountRepresents = key
	q := hmc377R13Rem(1.0, hmc377R11W(), 70)
	q.Type = "priority_inversion_runnable_wait"
	rest := []foldInterval{{start: 10.5, end: 10.501}}
	det := hmc377R14Det(2.903, hmc377R11W(), rest, key, 30)
	const name = "V10remRest"
	rank, pub := hmc377R15Publish(t, name, []RootCauseRankItem{hmc377R11OtherSeat(), inv, q, det}, hmc377R13IsInversionAccount)
	if len(rank.AbsorbedItems) != 1 || rank.AbsorbedItems[0].LineStart != det.LineStart {
		t.Fatalf("%s precondition: the inversion account rides as the chain seat's detail\n%s", name, hmc377R15Rows(rank))
	}
	var remClaim *RootCauseShareClaim
	for i := range rank.AbsorbedItems[0].ShareClaims {
		if c := &rank.AbsorbedItems[0].ShareClaims[i]; c.Part == RootCauseShareClaimRemainder {
			remClaim = c
		}
	}
	if remClaim == nil || len(remClaim.windows) == 0 {
		t.Fatalf("%s precondition: the inversion account claims its rest on a remainder claim with instants\n%s", name, hmc377R15Rows(rank))
	}
	var returned *RootCauseRankItem
	for i := range pub.Items {
		if pub.Items[i].Type == "priority_inversion_runnable_wait" && pub.Items[i].LineStart == det.LineStart {
			returned = &pub.Items[i]
		}
	}
	if returned == nil || returned.Tier == RootCauseTierContextOnly || returned.AbsorbedByRankFamily || math.Abs(returned.RunnableMs-1.0) > 0.0005 {
		t.Errorf("%s: the inversion account is not returned to the board with its unheld rest 1.000\n%s", name, hmc377R15Rows(pub))
	}
	priced := hmc377R14Priced(pub)
	if len(priced) != 2 {
		t.Errorf("%s: priced rows of 6326 %s, want the chain seat (1.903) and the returned inversion account (1.000)", name, hmc377R14Describe(priced))
	}
	hmc377R15Carried(t, name, pub, 2.903)
}

// R-377-18 ③c (E3): a holder holds at most the claim's share. Two rows hold
// a released detail's claim whole: an earlier row whose instants cover it and
// a later row whose instants miss it and whose hold without instants (2.000)
// exceeds the share (1.000). Equal parts: the detail rides under the earlier
// row. Its values stay those of a lossless detail.
func TestHMC377R18HolderHoldsAtMostTheShare(t *testing.T) {
	// Two withdrawn chain seats of 6326: x with instants (W, 1.903) and y
	// without (2.000); y owns an R-377-4 inversion account whose claim
	// carries U (1.000). A standing on-chain seat holds U with its instants
	// and is earlier on the board. The ◇ remainder is the successor of both
	// withdrawn seats (built by hand as in Y3mix2: it holds W with instants
	// and y's 2.000 without), so its instants miss U.
	u := hmc377R11ChainRun(1.0, hmc377R11U(), 41, 45)
	x := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
	y := hmc377R15WithoutInstants(hmc377R11Inv(2.0, hmc377R11U(), 21, 25), RootCauseShareHolderChainSeat)
	y.inversionAccountRepresents = "inversion_account:r18e3"
	det := hmc377R14Det(1.0, hmc377R11U(), nil, "inversion_account:r18e3", 30)
	rem := hmc377R13Rem(1.0, hmc377R11W(), 70)
	rspaAppendSentence(&rem, rspaRepresentedDemotionSummary(2.0, 2.0, hmc377R15ClaimWithoutInstants(RootCauseShareHolderChainSeat, 2.0)))
	const name = "R18heldAtMostShare"
	_, pub := hmc377R15Publish(t, name, []RootCauseRankItem{hmc377R11OtherSeat(), u, x, y, rem, hmc377R13SchedTwin(x), hmc377R13SchedTwin(y), det}, hmc377R13IsChainInversion)
	var d *RootCauseRankItem
	for i := range pub.AbsorbedItems {
		if pub.AbsorbedItems[i].Type == "priority_inversion_runnable_wait" && pub.AbsorbedItems[i].LineStart == det.LineStart {
			d = &pub.AbsorbedItems[i]
		}
	}
	if d == nil {
		t.Fatalf("%s: the inversion account does not ride as a lossless detail\n%s", name, hmc377R15Rows(pub))
	}
	if host, ok := hmc377R14HostOfRow(pub, *d); !ok || host.LineStart != u.LineStart {
		t.Errorf("%s: the inversion account rides under lines %d (found %t), want the earlier row holding the whole share (lines %d)\n%s",
			name, host.LineStart, ok, u.LineStart, hmc377R15Rows(pub))
	}
	if math.Abs(d.RunnableMs-1.0) > 0.0005 {
		t.Errorf("%s: the lossless detail keeps its minted value 1.000, got %.3f", name, d.RunnableMs)
	}
}
