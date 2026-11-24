package tracequery

// HMC377 round 15 — batch-owner ruling R-377-15 ②–⑥ (2026-11-24), the closing
// round: where a row the publication gate withdrew leaves its account, mostly
// in the form without instants (no segment inventory reproduced the accounts,
// so holds and claims carry values only).
//
// Review 14 (lens engine, ~/hmc-work/hmc377/fix14/review-engine/univ/
// head.e14.out; universes Y2censusVO, Y2roleVO, Y3mix2, Y3chainRest,
// Y4seatVO) reproduced, on boards built with the engine's own typed
// internals:
//   - R14E-1: a withdrawn row that holds nothing (a census inversion account
//     the chain seat represents) had no roles, so the row that took the chain
//     seat's share through a hold without instants was not seen as carrying
//     its account and its claimless twin was published as a second priced row
//     (Y2censusVO 4.806 ms against raw 2.903 ms);
//   - R14E-2: a hold passed without instants carried every withdrawn role of
//     the thread's family, so it counted for a withdrawn seat whose share it
//     did not take (Y2roleVO 0.500 ms against raw 2.403 ms);
//   - R14E-6: the held part of a claim read only the holder's instants when
//     it had any, so a claim resolved through a hold without instants found
//     no host and the released detail stayed a zero-valued row (Y3mix2);
//   - R14E-5: a detail was said to own instants for the rest of a claim that
//     only names the chain seat's own diverging account, which neither the
//     row's value nor the placement counts: the detail ended as a lossless
//     detail with zeroed values and a second publication differed
//     (Y3chainRest);
//   - R14E-4 / R14E-7: two branches of the placement no test told apart (a
//     hold without instants in the account's own role that no successor rule
//     passed; the whole share against the unheld rest of a claim).
//
// Every board is a hand board run through the production enrich order
// (hmc377R13Enrich) and published through the gate's typed decision
// (hmc377R13Gate), as every round-13 and round-14 universe is. Where a row is
// constructed rather than a row the engine mints, the builder says so.

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// hmc377R15WithoutInstants re-stamps a row's hold in role without a segment
// inventory (the form the engine mints when no inventory reproduces the
// row's account: the hold carries its value only).
func hmc377R15WithoutInstants(it RootCauseRankItem, role RootCauseShareHolderRole) RootCauseRankItem {
	ms := it.RunnableMs
	it.rspaHeld = nil
	rspaHold(&it, role, false, nil, ms)
	return it
}

// hmc377R15ClaimWithoutInstants is an anchored claim of 6326 on role that
// carries its value only.
func hmc377R15ClaimWithoutInstants(role RootCauseShareHolderRole, ms float64) RootCauseShareClaim {
	return rspaClaim(RootCauseShareClaimAnchored, role, hmc377R11Dep.PID, false, ms, nil)
}

// hmc377R15Rows prints every row of a publication: what a second publication
// must reproduce.
func hmc377R15Rows(pub RootCauseRankResult) string {
	var b strings.Builder
	for _, rows := range [][]RootCauseRankItem{pub.Items, pub.AbsorbedItems} {
		for _, it := range rows {
			fmt.Fprintf(&b, "[%s/%s rank=%d tier=%s run=%.3f eff=%.3f rows=%d key=%q into=%q absorbed=%t %q]\n", it.Type, it.Source, it.Rank, it.Tier,
				it.RunnableMs, it.EffectiveImpactMs, it.AbsorbedRankRows, it.RankFamilyKey, it.AbsorbedIntoFamily, it.AbsorbedByRankFamily, it.Summary)
		}
		b.WriteString("--\n")
	}
	return b.String()
}

// hmc377R15Publish runs a hand board through the production enrich order and
// the gate's typed decision, and checks what every publication keeps: no two
// rows wear one family key, no withdrawn row wears one, every family counts
// the rows riding under it, no released row is published at zero, the engine
// result is unchanged, and a second publication equals the first. The second
// publication applies the gate's own rule to the published copy: a
// priority-inversion row is published as a cause only with a finite positive
// effective value.
func hmc377R15Publish(t *testing.T, name string, items []RootCauseRankItem, board func(RootCauseRankItem) bool) (rank, pub RootCauseRankResult) {
	t.Helper()
	rank = hmc377R13Enrich(items)
	before := hmc377R15Rows(rank)
	pub, withdrawn := hmc377R13Gate(rank, board, nil)
	for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
		t.Errorf("%s: %s", name, f)
	}
	if after := hmc377R15Rows(rank); after != before {
		t.Errorf("%s: the publication changed the engine result", name)
	}
	unproven := func(it RootCauseRankItem) bool {
		return types.TraceRootCauseTypeIsPriorityInversion(it.Type) && rootCauseEffectiveImpactMs(it) <= 0
	}
	again, _ := hmc377R13Gate(pub, unproven, unproven)
	if first, second := hmc377R15Rows(pub), hmc377R15Rows(again); first != second {
		t.Errorf("%s: a second publication differs from the first:\nfirst\n%ssecond\n%s", name, first, second)
	}
	return rank, pub
}

// hmc377R15Carried checks the priced carriers of 6326 against its raw ready
// time (the interval union the board is built from).
func hmc377R15Carried(t *testing.T, name string, pub RootCauseRankResult, raw float64) {
	t.Helper()
	if got := hmc377R13SeatCarriers(pub.Items); math.Abs(got-raw) > 0.0015 {
		t.Errorf("%s: Σ carriers %.3f, want raw %.3f\n%s", name, got, raw, hmc377R15Rows(pub))
	}
}

// hmc377R15TwinHost is the board row the scheduler-latency twin of the given
// first line rides under (ok=false: the twin does not ride as a detail).
func hmc377R15TwinHost(pub RootCauseRankResult, line int) (RootCauseRankItem, bool) {
	for _, d := range pub.AbsorbedItems {
		if d.Type == "scheduler_latency" && d.LineStart == line {
			return hmc377R14HostOfRow(pub, d)
		}
	}
	return RootCauseRankItem{}, false
}

// hmc377R15TwinOnBoard is the scheduler-latency twin of the given first line
// when it is published as its own priced row.
func hmc377R15TwinOnBoard(pub RootCauseRankResult, line int) (RootCauseRankItem, bool) {
	for _, it := range hmc377R14Priced(pub) {
		if it.Type == "scheduler_latency" && it.LineStart == line {
			return it, true
		}
	}
	return RootCauseRankItem{}, false
}

// R-377-15 ② (R14E-1, universes Y2censusVO / Y2censusWin): a withdrawn
// account's roles come from its holds and from its own claims.
func TestHMC377R15WithdrawnAccountWithoutHoldsReadsItsClaimRoles(t *testing.T) {
	for _, instants := range []bool{false, true} {
		name := "Y2censusVO"
		if instants {
			name = "Y2censusWin"
		}
		// The chain inversion seat holds 1.903. A census inversion ready
		// account of the same time is represented by that seat: it holds
		// nothing, claims the chain seat's share and owns its claimless
		// scheduler-latency twin. The gate withdraws both. The thread's ◇
		// remainder (own rest 1.000) takes the chain seat's share as its
		// successor, so it carries the census account too: the twin rides under
		// it and 1.903 has one priced carrier.
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		p := hmc377R13Census(hmc377R11W(), 40)
		rem := hmc377R13Rem(1.0, hmc377R11W(), 70)
		if !instants {
			inv = hmc377R15WithoutInstants(inv, RootCauseShareHolderChainSeat)
			p.ShareClaims, p.rspaClaimSpans = nil, nil
			p.Summary = "binder:458_5-6326 same-CPU lower-priority overlap"
			rspaAppendSentence(&p, rspaRepresentedDemotionSummary(1.903, 1.903, hmc377R15ClaimWithoutInstants(RootCauseShareHolderChainSeat, 1.903)))
			rspaSetSentence(&rem, rspaRemainderSentence(hmc377R11Dep, "runnable (scheduling-pressure candidate)", 1.0, 1.903, 2.903,
				hmc377R15ClaimWithoutInstants(RootCauseShareHolderChainSeat, 1.903)))
			rem = hmc377R15WithoutInstants(rem, RootCauseShareHolderRemainderSeat)
		}
		twin := hmc377R13SchedTwin(p)
		rank, pub := hmc377R15Publish(t, name, []RootCauseRankItem{hmc377R11OtherSeat(), inv, p, twin, rem}, hmc377R13IsAnyInversion)
		if len(rank.AbsorbedItems) != 1 || len(rank.Items[2].rspaHeld) != 0 || rank.Items[2].RankFamilyKey == "" {
			t.Fatalf("%s precondition: the census row holds nothing and owns its twin (details %d, holds %d, key %q)",
				name, len(rank.AbsorbedItems), len(rank.Items[2].rspaHeld), rank.Items[2].RankFamilyKey)
		}
		priced := hmc377R14Priced(pub)
		if len(priced) != 1 || priced[0].Type != "runnable_wait" || math.Abs(priced[0].RunnableMs-2.903) > 0.0005 {
			t.Errorf("%s: priced rows of 6326 %s, want the ◇ remainder alone at 2.903", name, hmc377R14Describe(priced))
		}
		if host, ok := hmc377R15TwinHost(pub, twin.LineStart); !ok || host.Type != "runnable_wait" || host.Tier == RootCauseTierContextOnly {
			t.Errorf("%s: the census row's twin rides under %q (found %t), want the ◇ remainder that took the chain seat's share\n%s", name, host.Type, ok, hmc377R15Rows(pub))
		}
		hmc377R15Carried(t, name, pub, 2.903)
	}
}

// R-377-15 ③ (R14E-2, universes Y2roleVO / Y2roleWin): a hold passed without
// instants carries only the role of the claim it was passed for. This is also
// the default-suite pin of the role test of the placement (review-13 mutant
// M09 / A15f, "any successor hold without instants carries the account"),
// which round 14 could only record as indistinguishable while such a hold
// carried every withdrawn role of the family.
func TestHMC377R15HoldPassedWithoutInstantsCarriesItsClaimsRoleOnly(t *testing.T) {
	for _, instants := range []bool{false, true} {
		name := "Y2roleVO"
		if instants {
			name = "Y2roleWin"
		}
		// Two withdrawn seats of 6326: x holds 1.903 as the chain seat, y holds
		// 0.500 as the clipped seat; each owns its scheduler-latency twin. A
		// represented satellite claims the clipped seat's 0.500 and takes it as
		// y's successor. Nobody takes x's 1.903: x's twin is its only carrier
		// and is published with its own value; y's twin rides under the
		// satellite.
		yw := []foldInterval{{start: 10.3, end: 10.3005}}
		x := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		y := hmc377R11Inv(0.5, yw, 21, 25)
		y.rspaHeld = nil
		rspaHold(&y, RootCauseShareHolderClippedSeat, false, yw, 0.5)
		claim := rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderClippedSeat, hmc377R11Dep.PID, false, 0.5, yw)
		if !instants {
			x = hmc377R15WithoutInstants(x, RootCauseShareHolderChainSeat)
			y = hmc377R15WithoutInstants(y, RootCauseShareHolderClippedSeat)
			claim = hmc377R15ClaimWithoutInstants(RootCauseShareHolderClippedSeat, 0.5)
		}
		sat := hmc377R11Satellite(0.5, 10.3, 10.3005, claim)
		sat.LineStart, sat.LineEnd = 60, 65
		tx, ty := hmc377R13SchedTwin(x), hmc377R13SchedTwin(y)
		_, pub := hmc377R15Publish(t, name, []RootCauseRankItem{hmc377R11OtherSeat(), x, y, sat, tx, ty}, hmc377R13IsChainInversion)
		if own, ok := hmc377R15TwinOnBoard(pub, tx.LineStart); !ok || math.Abs(own.RunnableMs-1.903) > 0.0005 {
			t.Errorf("%s: the chain seat's twin is not published with its own value 1.903 (found %t)\n%s", name, ok, hmc377R15Rows(pub))
		}
		if host, ok := hmc377R15TwinHost(pub, ty.LineStart); !ok || host.Type != "runnable_wait" || host.LineStart != sat.LineStart {
			t.Errorf("%s: the clipped seat's twin rides under %q lines %d (found %t), want the satellite that took its share\n%s", name, host.Type, host.LineStart, ok, hmc377R15Rows(pub))
		}
		hmc377R15Carried(t, name, pub, 2.403)
	}
}

// R-377-15 ③, the same rule with one claimant per withdrawn seat: a value
// passes per role of the thread's family. The hold passed for one role no
// longer stands for the other, so the other role's value passes to its own
// claimant — otherwise that claimant publishes its account with nothing
// passed to it, the withdrawn seat's twin finds no carrier and is published as
// well. (Until round 15 one value passed per family with every role on it:
// the second claimant either stayed represented by a row that never added its
// share — 0.500 lost on the first three boards — or, when its share was the
// larger one, published it while both twins rode under the first.)
func TestHMC377R15EachRoleWithoutInstantsPassesToItsOwnClaimant(t *testing.T) {
	const remLine, satXLine, satYLine = 70, 50, 60
	for _, tc := range []struct {
		name                 string
		raw                  float64
		xHost, yHost         int // first line of the row each seat's twin rides under (0: no twins)
		remainder, satXFirst bool
	}{
		// The chain seat's 1.903 is claimed by the thread's ◇ remainder (rest
		// 1.000) or by a represented satellite; the clipped seat's 0.500 by
		// another represented satellite.
		{name: "Y2roleVO2rem", raw: 3.403, xHost: remLine, yHost: satYLine, remainder: true},
		{name: "Y2roleVO2remNoTwins", raw: 3.403, remainder: true},
		{name: "Y2roleVO2sat", raw: 2.403, xHost: satXLine, yHost: satYLine, satXFirst: true},
		{name: "Y2roleVO2satRev", raw: 2.403, xHost: satXLine, yHost: satYLine},
	} {
		x := hmc377R15WithoutInstants(hmc377R11Inv(1.903, hmc377R11W(), 10, 20), RootCauseShareHolderChainSeat)
		y := hmc377R15WithoutInstants(hmc377R11Inv(0.5, []foldInterval{{start: 10.3, end: 10.3005}}, 21, 25), RootCauseShareHolderClippedSeat)
		satY := hmc377R11Satellite(0.5, 10.3, 10.3005, hmc377R15ClaimWithoutInstants(RootCauseShareHolderClippedSeat, 0.5))
		satY.LineStart, satY.LineEnd = satYLine, satYLine+5
		items := []RootCauseRankItem{hmc377R11OtherSeat(), x, y}
		switch {
		case tc.remainder:
			rem := hmc377R13Rem(1.0, hmc377R11W(), remLine)
			rspaSetSentence(&rem, rspaRemainderSentence(hmc377R11Dep, "runnable (scheduling-pressure candidate)", 1.0, 1.903, 2.903,
				hmc377R15ClaimWithoutInstants(RootCauseShareHolderChainSeat, 1.903)))
			items = append(items, satY, hmc377R15WithoutInstants(rem, RootCauseShareHolderRemainderSeat))
		default:
			satX := hmc377R11Satellite(1.903, 10.2, 10.201903, hmc377R15ClaimWithoutInstants(RootCauseShareHolderChainSeat, 1.903))
			satX.LineStart, satX.LineEnd = satXLine, satXLine+5
			if tc.satXFirst {
				items = append(items, satX, satY)
			} else {
				items = append(items, satY, satX)
			}
		}
		tx, ty := hmc377R13SchedTwin(x), hmc377R13SchedTwin(y)
		if tc.xHost != 0 {
			items = append(items, tx, ty)
		}
		_, pub := hmc377R15Publish(t, tc.name, items, hmc377R13IsChainInversion)
		hmc377R15Carried(t, tc.name, pub, tc.raw)
		if tc.xHost == 0 {
			continue
		}
		if host, ok := hmc377R15TwinHost(pub, tx.LineStart); !ok || host.LineStart != tc.xHost {
			t.Errorf("%s: the chain seat's twin rides under lines %d (found %t), want the row that took its 1.903 (lines %d)\n%s", tc.name, host.LineStart, ok, tc.xHost, hmc377R15Rows(pub))
		}
		if host, ok := hmc377R15TwinHost(pub, ty.LineStart); !ok || host.LineStart != tc.yHost {
			t.Errorf("%s: the clipped seat's twin rides under lines %d (found %t), want the satellite that took its 0.500 (lines %d)\n%s", tc.name, host.LineStart, ok, tc.yHost, hmc377R15Rows(pub))
		}
	}
}

// R-377-15 ④ (R14E-6, universe Y3mix2): the held part of a claim reads the
// holder's hold without instants when its instants cover none of the claim.
func TestHMC377R15HeldPartReadsTheHoldWithoutInstants(t *testing.T) {
	// Two withdrawn chain seats of 6326: x with instants (W, 1.903) and y
	// without (1.000 at U); y owns an R-377-4 inversion account whose claim
	// carries U. One published row is the successor of both, so it holds W
	// with instants and y's share without. The inversion account's claim
	// resolves to that row through the hold without instants: it owns no
	// instant and rides as that row's lossless detail.
	//
	// Constructed: no row the engine mints makes one claim with instants and
	// one without (a row's inventory reproduces its account or it does not),
	// so the successor row is built by hand in both forms below.
	build := func(successor RootCauseRankItem) []RootCauseRankItem {
		x := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		y := hmc377R15WithoutInstants(hmc377R11Inv(1.0, hmc377R11U(), 21, 25), RootCauseShareHolderChainSeat)
		y.inversionAccountRepresents = "inversion_account:y3mix"
		det := hmc377R14Det(1.0, hmc377R11U(), nil, "inversion_account:y3mix", 30)
		return []RootCauseRankItem{hmc377R11OtherSeat(), x, y, successor, hmc377R13SchedTwin(x), hmc377R13SchedTwin(y), det}
	}
	check := func(t *testing.T, name string, pub RootCauseRankResult, hostLine int) {
		t.Helper()
		var det *RootCauseRankItem
		for i := range pub.AbsorbedItems {
			if pub.AbsorbedItems[i].Type == "priority_inversion_runnable_wait" {
				det = &pub.AbsorbedItems[i]
			}
		}
		if det == nil {
			t.Fatalf("%s: the inversion account does not ride as a lossless detail\n%s", name, hmc377R15Rows(pub))
		}
		if host, ok := hmc377R14HostOfRow(pub, *det); !ok || host.LineStart != hostLine || host.Tier == RootCauseTierContextOnly {
			t.Errorf("%s: the inversion account rides under lines %d (found %t), want the successor row\n%s", name, host.LineStart, ok, hmc377R15Rows(pub))
		}
		if math.Abs(det.RunnableMs-1.0) > 0.0005 {
			t.Errorf("%s: the lossless detail keeps its minted value 1.000, got %.3f", name, det.RunnableMs)
		}
	}
	t.Run("Y3mix2: the reviewer's successor row", func(t *testing.T) {
		// The ◇ remainder of the thread (rest 1.000, anchored claim over W)
		// with a second, represented claim without instants appended by hand.
		// A hybrid of two row kinds: its own published value is no evidence
		// (review 14), only the placement of the inversion account is.
		rem := hmc377R13Rem(1.0, hmc377R11W(), 70)
		rspaAppendSentence(&rem, rspaRepresentedDemotionSummary(1.0, 1.0, hmc377R15ClaimWithoutInstants(RootCauseShareHolderChainSeat, 1.0)))
		_, pub := hmc377R15Publish(t, "Y3mix2", build(rem), hmc377R13IsChainInversion)
		check(t, "Y3mix2", pub, rem.LineStart)
	})
	t.Run("a represented satellite inside both seats' time", func(t *testing.T) {
		// The same placement on a row whose value is its own account: a
		// represented satellite of 2.903 = W (claimed with instants) + 1.000
		// (claimed without). Both shares pass back to it, so it is the one
		// priced carrier of the thread's 2.903.
		sat := hmc377R11Satellite(2.903, 10.2, 10.301,
			rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, hmc377R11W()),
			hmc377R15ClaimWithoutInstants(RootCauseShareHolderChainSeat, 1.0))
		sat.LineStart, sat.LineEnd = 60, 65
		_, pub := hmc377R15Publish(t, "Y3mix2sat", build(sat), hmc377R13IsChainInversion)
		check(t, "Y3mix2sat", pub, sat.LineStart)
		priced := hmc377R14Priced(pub)
		if len(priced) != 1 || math.Abs(priced[0].RunnableMs-2.903) > 0.0005 {
			t.Errorf("Y3mix2sat: priced rows %s, want the satellite alone at 2.903", hmc377R14Describe(priced))
		}
		hmc377R15Carried(t, "Y3mix2sat", pub, 2.903)
	})
	t.Run("instants that cover the claim still measure the part", func(t *testing.T) {
		// The windowed part is not zero: the holder's instants decide, and its
		// hold without instants adds nothing to that part.
		c := rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, hmc377R11W())
		holder := hmc377R11ChainRun(1.05, hmc377R11W1(), 41, 45)
		holder.rspaHeld = append(holder.rspaHeld, rspaHeldShare{role: RootCauseShareHolderChainSeat, ms: 5})
		c.holderIdx = []int{0}
		parts := rspaClaimHeldParts(c, []RootCauseRankItem{holder})
		if math.Abs(parts[0]-1.05) > 0.0005 {
			t.Errorf("held part %.3f, want the instants the holder covers (1.050)", parts[0])
		}
	})
	t.Run("a holder with instants only keeps its measured part", func(t *testing.T) {
		// No hold without instants: the part is what the instants measure,
		// whatever its size (here the holder's 1.000 meets the claim for half
		// the identity tolerance) — a board with instants everywhere reads
		// exactly as before.
		c := rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, hmc377R11W())
		end := hmc377R11W()[0].end
		sliver := rspaAnchorIdentityTolMs / 2
		holder := hmc377R11ChainRun(1.0, []foldInterval{{start: end - sliver/1000, end: end - sliver/1000 + 0.001}}, 41, 45)
		c.holderIdx = []int{0}
		parts := rspaClaimHeldParts(c, []RootCauseRankItem{holder})
		if math.Abs(parts[0]-sliver) > 1e-7 {
			t.Errorf("held part %.9f, want the instants the holder covers (%.9f)", parts[0], sliver)
		}
	})
	t.Run("instants outside the claim are no part of it", func(t *testing.T) {
		// The windowed part is zero: the part is the value of the hold without
		// instants alone — the holder's instants elsewhere (1.000 at U) hold
		// nothing of this claim.
		c := rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, hmc377R11W())
		holder := hmc377R11ChainRun(1.0, hmc377R11U(), 41, 45)
		holder.rspaHeld = append(holder.rspaHeld, rspaHeldShare{role: RootCauseShareHolderChainSeat, ms: 0.5})
		c.holderIdx = []int{0}
		parts := rspaClaimHeldParts(c, []RootCauseRankItem{holder})
		if math.Abs(parts[0]-0.5) > 0.0005 {
			t.Errorf("held part %.3f, want the value of the hold without instants (0.500)", parts[0])
		}
	})
}
