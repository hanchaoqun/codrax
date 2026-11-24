package tracequery

// HMC377 round 14 — batch-owner ruling R-377-14 ⑤ (2026-11-24), finding R13H-1
// of review 13 (lens holders, universes Z1bNoHolder / Z1bNoHolder_H).
//
// When the publication gate withdraws a row, a claimless detail it absorbed
// (the scheduler-latency twin, the state-churn twin) follows the withdrawn
// row's account: it rides under the published row carrying the largest part
// of the instants that row held and claimed. Round 13 measured "carries" from
// holds alone. A claim-bearing detail of the same withdrawn row that returns
// to the board with an unheld rest — the share the withdrawn seat held that no
// published successor took — counts those instants in its own value without
// holding them, so the placement saw no carrier and released the twin as a
// second priced row over the same instants (3.806 ms against raw 1.903 ms).
// Ruling ⑤: the returned claim-bearing row carries the unheld rest and the
// claimless twin rides under it — one share, one priced carrier.
//
// The universes are built with the engine's own typed internals through the
// production enrich order (hmc377R13Enrich) and published through the gate's
// typed decision (hmc377R13Gate), as every round-13 universe is.

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// hmc377R14Describe prints the rows a failure message names.
func hmc377R14Describe(rows []RootCauseRankItem) string {
	var b strings.Builder
	for _, it := range rows {
		fmt.Fprintf(&b, "[%s/%s tier=%s run=%.3f eff=%.3f rows=%d key=%q into=%q]", it.Type, it.Source, it.Tier, it.RunnableMs, it.EffectiveImpactMs, it.AbsorbedRankRows, it.RankFamilyKey, it.AbsorbedIntoFamily)
	}
	return b.String()
}

// hmc377R14Det is an R-377-4 inversion window account of 6326 with a typed
// stats window, so its state-churn twin pairs with it.
func hmc377R14Det(full float64, anchored, rest []foldInterval, key string, l0 int) RootCauseRankItem {
	det := hmc377R13InvDetail(full, anchored, rest, key, l0)
	det.StatsWindowStartTs, det.StatsWindowEndTs = 10, 11
	det.StartTs, det.EndTs = anchored[0].start, anchored[len(anchored)-1].end
	if len(rest) > 0 {
		det.EndTs = rest[len(rest)-1].end
	}
	return det
}

// hmc377R14Priced lists the priced board rows of 6326 on a published copy (a
// context row, a lossless detail and a represented row are no carriers).
func hmc377R14Priced(pub RootCauseRankResult) []RootCauseRankItem {
	var out []RootCauseRankItem
	for _, it := range pub.Items {
		if it.Thread.PID != hmc377R11Dep.PID || it.Tier == RootCauseTierContextOnly || it.AbsorbedByRankFamily || it.ChainAnchorRepresentedByChainSeat {
			continue
		}
		out = append(out, it)
	}
	return out
}

// hmc377R14HostOfRow returns the board row a given detail row rides under.
func hmc377R14HostOfRow(pub RootCauseRankResult, d RootCauseRankItem) (RootCauseRankItem, bool) {
	for _, it := range pub.Items {
		if it.RankFamilyKey != "" && it.RankFamilyKey == d.AbsorbedIntoFamily {
			return it, true
		}
	}
	return RootCauseRankItem{}, false
}

func TestHMC377R14ClaimlessTwinRidesUnderTheRowCarryingTheUnheldRest(t *testing.T) {
	t.Run("Z1bNoHolder: the returned inversion detail is the one priced carrier", func(t *testing.T) {
		for _, withChurn := range []bool{true, false} {
			// The withdrawn seat's only claimant is its R-377-4 detail (whole
			// account inside W, no ◇ remainder): the whole share returns to the
			// detail, which is published with its own value. No published row
			// HOLDS W. The seat's claimless scheduler-latency twin follows the
			// withdrawn account to that row.
			inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
			inv.inversionAccountRepresents = "inversion_account:z1b"
			det := hmc377R14Det(1.903, hmc377R11W(), nil, "inversion_account:z1b", 30)
			items := []RootCauseRankItem{hmc377R11OtherSeat(), inv, hmc377R13SchedTwin(inv), det}
			want := 1
			if withChurn {
				items = append(items, hmc377R13ChurnOf(det))
				want = 2
			}
			rank := hmc377R13Enrich(items)
			if len(rank.AbsorbedItems) != want+1 {
				t.Fatalf("withChurn=%t precondition: the chain seat absorbs %d details, got %d", withChurn, want+1, len(rank.AbsorbedItems))
			}
			pub, withdrawn := hmc377R13Gate(rank, hmc377R13IsChainInversion, nil)
			for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
				t.Errorf("withChurn=%t: %s", withChurn, f)
			}
			priced := hmc377R14Priced(pub)
			if len(priced) != 1 || priced[0].Type != "priority_inversion_runnable_wait" || math.Abs(priced[0].RunnableMs-1.903) > 0.0005 {
				t.Fatalf("withChurn=%t: priced rows of 6326 %s, want the returned inversion detail alone at 1.903", withChurn, hmc377R14Describe(priced))
			}
			if priced[0].AbsorbedRankRows != want {
				t.Errorf("withChurn=%t: the returned detail hosts %d rows, want %d", withChurn, priced[0].AbsorbedRankRows, want)
			}
			if len(pub.AbsorbedItems) != want {
				t.Fatalf("withChurn=%t: %d lossless details, want %d", withChurn, len(pub.AbsorbedItems), want)
			}
			for _, d := range pub.AbsorbedItems {
				if host, ok := hmc377R14HostOfRow(pub, d); !ok || host.Type != "priority_inversion_runnable_wait" || !d.AbsorbedByRankFamily {
					t.Errorf("withChurn=%t: detail %s rides under %q, want the returned inversion detail", withChurn, d.Type, host.Type)
				}
				if math.Abs(d.RunnableMs-1.903) > 0.0005 {
					t.Errorf("withChurn=%t: lossless detail %s keeps its minted value, got %.3f", withChurn, d.Type, d.RunnableMs)
				}
			}
			if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-1.903) > 0.0015 {
				t.Errorf("withChurn=%t: Σ carriers %.3f, want raw 1.903", withChurn, got)
			}
		}
	})
	t.Run("the larger of a successor's part and the unheld rest hosts the twin", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			sat      []foldInterval
			wantType string
			wantSrc  string
		}{
			// Z1dSatPart: a represented satellite takes W1 (1.050) as the
			// seat's successor; the returned detail keeps W2 (0.853).
			{"the successor holds more", hmc377R11W1(), "runnable_wait", "window_stats"},
			// The satellite takes W2 (0.853); the returned detail keeps W1.
			{"the unheld rest is more", hmc377R11W2(), "priority_inversion_runnable_wait", "window_stats"},
		} {
			inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
			inv.inversionAccountRepresents = "inversion_account:z1d"
			det := hmc377R14Det(1.903, hmc377R11W(), nil, "inversion_account:z1d", 30)
			ms := rspaFoldLengthMs(tc.sat)
			sat := hmc377R11Satellite(ms, tc.sat[0].start, tc.sat[0].end,
				rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, ms, tc.sat))
			sat.LineStart, sat.LineEnd = 60, 70
			pub, withdrawn := hmc377R13Gate(hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), inv, sat, hmc377R13SchedTwin(inv), det}), hmc377R13IsChainInversion, nil)
			for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
				t.Errorf("%s: %s", tc.name, f)
			}
			host, ok := hmc377R13HostOf(pub, "scheduler_latency")
			if !ok || host.Type != tc.wantType || host.Source != tc.wantSrc || host.Tier == RootCauseTierContextOnly {
				t.Errorf("%s: the twin rides under %q/%q (found %t), want %s", tc.name, host.Type, host.Source, ok, tc.wantType)
			}
			if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-1.903) > 0.0015 {
				t.Errorf("%s: Σ carriers %.3f, want raw 1.903", tc.name, got)
			}
		}
	})
	t.Run("the unheld rest is measured inside the withdrawn seat's own instants", func(t *testing.T) {
		// Two withdrawn chain seats: a over W1 (1.050) and b over W2 (0.853),
		// each with its claimless twin. The R-377-4 detail of a claims W. A
		// represented satellite takes W2 as b's successor; the detail returns
		// to the board with W1 unheld (1.050). a's twin follows W1 to the
		// detail; b's twin follows W2 to the satellite — the detail's larger
		// rest lies outside b's instants.
		a := hmc377R11Inv(1.05, hmc377R11W1(), 10, 15)
		b := hmc377R11Inv(0.853, hmc377R11W2(), 16, 20)
		a.inversionAccountRepresents = "inversion_account:z8"
		det := hmc377R14Det(1.903, hmc377R11W(), nil, "inversion_account:z8", 30)
		sat := hmc377R11Satellite(0.853, 10.20105, 10.201903,
			rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 0.853, hmc377R11W2()))
		sat.LineStart, sat.LineEnd = 60, 70
		ta, tb := hmc377R13SchedTwin(a), hmc377R13SchedTwin(b)
		rank := hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), a, b, sat, ta, tb, det})
		if len(rank.AbsorbedItems) != 3 {
			t.Fatalf("precondition: a absorbs its twin and the detail, b its twin; got %d details", len(rank.AbsorbedItems))
		}
		pub, withdrawn := hmc377R13Gate(rank, hmc377R13IsChainInversion, nil)
		for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
			t.Errorf("%s", f)
		}
		twins := 0
		for _, d := range pub.AbsorbedItems {
			if d.Type != "scheduler_latency" {
				continue
			}
			twins++
			host, ok := hmc377R14HostOfRow(pub, d)
			want := "priority_inversion_runnable_wait"
			if d.LineStart == tb.LineStart {
				want = "runnable_wait"
			}
			if !ok || host.Type != want {
				t.Errorf("the twin of lines %d-%d rides under %q, want %s", d.LineStart, d.LineEnd, host.Type, want)
			}
		}
		if twins != 2 {
			t.Errorf("%d twins ride as lossless detail, want both", twins)
		}
		if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-1.903) > 0.0015 {
			t.Errorf("Σ carriers %.3f, want raw 1.903", got)
		}
	})
	t.Run("an unheld rest without instants is the withdrawn seat's share by its role", func(t *testing.T) {
		// No segment inventory reproduced the accounts: the withdrawn seat's
		// hold and the detail's claim carry no instants. The detail returns to
		// the board with the whole share unheld; the twin rides under it.
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		inv.rspaHeld = nil
		rspaHold(&inv, RootCauseShareHolderChainSeat, false, nil, 1.903)
		inv.inversionAccountRepresents = "inversion_account:vo"
		det := RootCauseRankItem{Type: "priority_inversion_runnable_wait", Source: "window_stats", Thread: hmc377R11Dep,
			ImpactMs: 1.903, CumulativeImpactMs: 1.903, EffectiveImpactMs: 1.903, RunnableMs: 1.903, DominantState: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.7, StartTs: 10.2, EndTs: 10.201903, LineStart: 30, LineEnd: 40,
			Summary: "binder:458_5-6326 same-CPU lower-priority overlap over the ready account"}
		rspaAppendSentence(&det, rspaInversionRepresentedSummary(1.903, 1.903,
			rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, nil), RootCauseShareClaim{}))
		det.inversionAccountRepresentedBy = "inversion_account:vo"
		pub, withdrawn := hmc377R13Gate(hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), inv, hmc377R13SchedTwin(inv), det}), hmc377R13IsChainInversion, nil)
		for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
			t.Errorf("%s", f)
		}
		if host, ok := hmc377R13HostOf(pub, "scheduler_latency"); !ok || host.Type != "priority_inversion_runnable_wait" {
			t.Errorf("the twin rides under %q (found %t), want the returned inversion detail", host.Type, ok)
		}
		if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-1.903) > 0.0015 {
			t.Errorf("Σ carriers %.3f, want raw 1.903", got)
		}
	})
	t.Run("Z1cTwinOnly: a twin no published row carries is its own priced row", func(t *testing.T) {
		// Nobody claims W: the twin is the one carrier of the withdrawn seat's
		// ready time (R-377-14 ⑤: a released twin that is the only carrier is
		// its own row).
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		pub, withdrawn := hmc377R13Gate(hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), inv, hmc377R13SchedTwin(inv)}), hmc377R13IsChainInversion, nil)
		priced := hmc377R14Priced(pub)
		if len(pub.AbsorbedItems) != 0 || len(priced) != 1 || priced[0].Type != "scheduler_latency" || math.Abs(priced[0].RunnableMs-1.903) > 0.0005 {
			t.Errorf("priced rows %s, details %d; want the twin alone at 1.903", hmc377R14Describe(priced), len(pub.AbsorbedItems))
		}
		for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
			t.Errorf("%s", f)
		}
	})
}

// The unheld rest is carried only by the row whose own value counts it, and
// only for the shares that row counts.
func TestHMC377R14UnheldRestIsCarriedOnlyWhereItIsCounted(t *testing.T) {
	t.Run("a share without instants left by another withdrawn seat's role is not this account", func(t *testing.T) {
		// Two withdrawn seats of 6326: a holds 1.903 as the chain seat (no
		// instants) and absorbs its twin and the R-377-4 detail, whose claim
		// names the chain seat; b holds 0.500 as the clipped seat and absorbs
		// its own twin. The detail returns with a's whole share unheld: a's
		// twin rides under it; b's account has no carrier on the board, so b's
		// twin is published with its own value.
		a := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		a.rspaHeld = nil
		rspaHold(&a, RootCauseShareHolderChainSeat, false, nil, 1.903)
		a.inversionAccountRepresents = "inversion_account:two-roles"
		bw := []foldInterval{{start: 10.3, end: 10.3005}}
		b := hmc377R11Inv(0.5, bw, 21, 25)
		b.rspaHeld = nil
		rspaHold(&b, RootCauseShareHolderClippedSeat, false, bw, 0.5)
		det := RootCauseRankItem{Type: "priority_inversion_runnable_wait", Source: "window_stats", Thread: hmc377R11Dep,
			ImpactMs: 1.903, CumulativeImpactMs: 1.903, EffectiveImpactMs: 1.903, RunnableMs: 1.903, DominantState: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.7, StartTs: 10.2, EndTs: 10.201903, LineStart: 30, LineEnd: 40,
			Summary: "binder:458_5-6326 same-CPU lower-priority overlap over the ready account"}
		rspaAppendSentence(&det, rspaInversionRepresentedSummary(1.903, 1.903,
			rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, nil), RootCauseShareClaim{}))
		det.inversionAccountRepresentedBy = "inversion_account:two-roles"
		ta, tb := hmc377R13SchedTwin(a), hmc377R13SchedTwin(b)
		rank := hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), a, b, ta, tb, det})
		if len(rank.AbsorbedItems) != 3 {
			t.Fatalf("precondition: a absorbs its twin and the detail, b its twin; got %d details", len(rank.AbsorbedItems))
		}
		pub, withdrawn := hmc377R13Gate(rank, hmc377R13IsChainInversion, nil)
		for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
			t.Errorf("%s", f)
		}
		priced := hmc377R14Priced(pub)
		if len(priced) != 2 || len(pub.AbsorbedItems) != 1 || pub.AbsorbedItems[0].LineStart != ta.LineStart {
			t.Fatalf("priced %s details %s; want the returned detail (hosting a's twin) and b's twin on its own row",
				hmc377R14Describe(priced), hmc377R14Describe(pub.AbsorbedItems))
		}
		if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-2.403) > 0.0015 {
			t.Errorf("Σ carriers %.3f, want raw 2.403", got)
		}
	})
	t.Run("the chain seat's own account the returned row only names is not carried by it", func(t *testing.T) {
		// Z1dSatPart: the satellite takes W1 (1.050) as the seat's successor,
		// the returned detail keeps W2 (0.853) — and also names the chain
		// seat's own diverging account (case A'), a claim that never moved a
		// value of its own row. That claim's unheld rest is not part of the
		// row's account: the twin still rides under the satellite.
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		inv.inversionAccountRepresents = "inversion_account:uncounted"
		det := hmc377R14Det(1.903, hmc377R11W(), nil, "inversion_account:uncounted", 30)
		det.ShareClaims = append(append([]RootCauseShareClaim(nil), det.ShareClaims...),
			rspaClaim(RootCauseShareClaimChainAccount, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, nil))
		sat := hmc377R11Satellite(1.05, 10.2, 10.20105,
			rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.05, hmc377R11W1()))
		sat.LineStart, sat.LineEnd = 60, 70
		pub, withdrawn := hmc377R13Gate(hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), inv, sat, hmc377R13SchedTwin(inv), det}), hmc377R13IsChainInversion, nil)
		for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
			t.Errorf("%s", f)
		}
		if host, ok := hmc377R13HostOf(pub, "scheduler_latency"); !ok || host.Type != "runnable_wait" || host.Source != "window_stats" {
			t.Errorf("the twin rides under %q/%q (found %t), want the satellite holding the larger part", host.Type, host.Source, ok)
		}
		if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-1.903) > 0.0015 {
			t.Errorf("Σ carriers %.3f, want raw 1.903", got)
		}
	})
	t.Run("a row that stayed on the board does not count its claim's unheld rest", func(t *testing.T) {
		// The withdrawn row x holds the thread's remainder R (1.000) and
		// absorbs its claimless twin; the published chain seat s holds W and
		// only NAMES x as the seat publishing R (a remainder claim). After the
		// withdrawal that claim has an unheld rest, but s publishes W alone:
		// R has no carrier on the board, so x's twin is published with its own
		// value.
		r := []foldInterval{{start: 10.5, end: 10.501}}
		x := RootCauseRankItem{Rank: 4, Tier: "tertiary", Type: "priority_inversion_runnable_wait", Source: "window_stats", Thread: hmc377R11Dep,
			ImpactMs: 1, CumulativeImpactMs: 1, EffectiveImpactMs: 1, ProjectedImpactMs: 1, RunnableMs: 1, DominantState: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.7, StatsWindowStartTs: 10, StatsWindowEndTs: 11,
			StartTs: 10.5, EndTs: 10.501, LineStart: 70, LineEnd: 80, ChainAnchorRemainderSeat: true, Summary: "binder:458_5-6326 remainder"}
		rspaHold(&x, RootCauseShareHolderRemainderSeat, false, r, 1)
		s := hmc377R11ChainRun(1.903, hmc377R11W(), 10, 20)
		s.ShareClaims = []RootCauseShareClaim{rspaClaim(RootCauseShareClaimRemainder, RootCauseShareHolderRemainderSeat, hmc377R11Dep.PID, false, 1, r)}
		rank := hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), s, x, hmc377R13SchedTwin(x)})
		if len(rank.AbsorbedItems) != 1 || !rank.Items[1].ShareClaims[0].Resolved {
			t.Fatalf("precondition: x absorbs its twin (%d details) and holds the chain seat's remainder claim", len(rank.AbsorbedItems))
		}
		pub, withdrawn := hmc377R13Gate(rank, hmc377R13IsInversionAccount, nil)
		for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
			t.Errorf("%s", f)
		}
		if got := pub.Items[1].ShareClaims[0].restWithdrawnMs; math.Abs(got-1) > 0.0005 {
			t.Fatalf("precondition: the chain seat's remainder claim has an unheld rest of 1.000, got %.3f", got)
		}
		priced := hmc377R14Priced(pub)
		if len(pub.AbsorbedItems) != 0 || len(priced) != 2 || priced[1].Type != "scheduler_latency" {
			t.Errorf("priced %s details %s; want the chain seat and x's twin on its own row", hmc377R14Describe(priced), hmc377R14Describe(pub.AbsorbedItems))
		}
		if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-2.903) > 0.0015 {
			t.Errorf("Σ carriers %.3f, want raw 2.903", got)
		}
	})
}

// hmc377R14Sat is a represented satellite of 6326 over windows.
func hmc377R14Sat(windows []foldInterval, l0 int) RootCauseRankItem {
	ms := rspaFoldLengthMs(windows)
	sat := hmc377R11Satellite(ms, windows[0].start, windows[len(windows)-1].end,
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, ms, windows))
	sat.LineStart, sat.LineEnd = l0, l0+5
	return sat
}

// hmc377R14Universe builds the universes review 13 (lens holders) added: a
// withdrawn absorber whose claim-bearing and claimless details land on
// different rows, shares split with ties, two withdrawn absorbers, a host that
// already owns a family. raw is the hand raw ready time of 6326; detail names
// the lossless details whose own priority proof fails the gate too.
func hmc377R14Universe(name string) (rank RootCauseRankResult, raw float64, detail func(RootCauseRankItem) bool) {
	rest := []foldInterval{{start: 10.5, end: 10.501}}
	a1 := []foldInterval{{start: 10.2, end: 10.201}}
	b1 := []foldInterval{{start: 10.201, end: 10.202}}
	c1 := []foldInterval{{start: 10.202, end: 10.203}}
	ab := []foldInterval{{start: 10.2, end: 10.202}}
	abc := []foldInterval{{start: 10.2, end: 10.203}}
	w, w1, w2 := hmc377R11W(), hmc377R11W1(), hmc377R11W2()
	inv := func(ms float64, windows []foldInterval, key string) RootCauseRankItem {
		it := hmc377R11Inv(ms, windows, 10, 20)
		it.inversionAccountRepresents = key
		return it
	}
	switch name {
	case "Z1own2":
		i := inv(1.903, w, "inversion_account:z1")
		det := hmc377R14Det(2.903, w, rest, "inversion_account:z1", 30)
		return hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), i, hmc377R12CPU(w1, 90), hmc377R13Rem(1.0, w, 70), hmc377R13SchedTwin(i), det, hmc377R13ChurnOf(det)}), 2.903, nil
	case "Z1bNoHolder":
		i := inv(1.903, w, "inversion_account:z1b")
		det := hmc377R14Det(1.903, w, nil, "inversion_account:z1b", 30)
		return hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), i, hmc377R13SchedTwin(i), det, hmc377R13ChurnOf(det)}), 1.903, nil
	case "Z1cTwinOnly":
		i := hmc377R11Inv(1.903, w, 10, 20)
		return hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), i, hmc377R13SchedTwin(i)}), 1.903, nil
	case "Z1dSatPart":
		i := inv(1.903, w, "inversion_account:z1d")
		det := hmc377R14Det(1.903, w, nil, "inversion_account:z1d", 30)
		return hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), i, hmc377R14Sat(w1, 60), hmc377R13SchedTwin(i), det, hmc377R13ChurnOf(det)}), 1.903, nil
	case "Z2tie3a", "Z2tie3b", "Z2tie3c":
		i := inv(3.0, abc, "inversion_account:z2")
		det := hmc377R14Det(3.0, abc, nil, "inversion_account:z2", 30)
		sat, rem, cpu := hmc377R14Sat(b1, 60), hmc377R13Rem(1.0, c1, 70), hmc377R12CPU(a1, 90)
		items := []RootCauseRankItem{hmc377R11OtherSeat(), i, sat, rem, cpu, det}
		switch name {
		case "Z2tie3b":
			items = []RootCauseRankItem{hmc377R11OtherSeat(), i, rem, sat, cpu, det}
		case "Z2tie3c":
			items = []RootCauseRankItem{hmc377R11OtherSeat(), cpu, rem, i, det, sat}
		}
		return hmc377R13Enrich(items), 4.0, nil
	case "Z4eq2a", "Z4eq2b":
		i := inv(2.0, ab, "inversion_account:z4")
		det := hmc377R14Det(2.0, ab, nil, "inversion_account:z4", 30)
		items := []RootCauseRankItem{hmc377R11OtherSeat(), i, hmc377R13Rem(1.0, b1, 70), hmc377R12CPU(a1, 90), det}
		if name == "Z4eq2b" {
			items = []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R12CPU(a1, 90), det, hmc377R13Rem(1.0, b1, 70), i}
		}
		return hmc377R13Enrich(items), 3.0, nil
	case "Z4eq2sat":
		return hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(2.0, ab, 10, 20),
			hmc377R12CPU(rspaFoldUnion(a1, []foldInterval{{start: 10.5, end: 10.5005}}), 90), hmc377R13Rem(1.0, b1, 70), hmc377R14Sat(ab, 30)}), 3.0, nil
	case "Z7bothRem":
		i := inv(1.903, w, "inversion_account:z7")
		det := hmc377R14Det(2.903, w, rest, "inversion_account:z7", 30)
		return hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), i, hmc377R13Rem(1.0, w, 70), det, hmc377R13ChurnOf(det)}), 2.903, hmc377R13IsInversionAccount
	case "Z7bothSolo":
		i := inv(1.903, w, "inversion_account:z7")
		det := hmc377R14Det(1.903, w, nil, "inversion_account:z7", 30)
		return hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), i, det, hmc377R13ChurnOf(det)}), 1.903, hmc377R13IsInversionAccount
	case "Z8twoAbs", "Z8twoAbsCpu":
		a := hmc377R11Inv(1.05, w1, 10, 15)
		b := hmc377R11Inv(0.853, w2, 16, 20)
		items := []RootCauseRankItem{hmc377R11OtherSeat(), a, b, hmc377R13Rem(1.0, w, 70), hmc377R13SchedTwin(a), hmc377R13SchedTwin(b)}
		if name == "Z8twoAbsCpu" {
			items = append(items, hmc377R12CPU(rspaFoldUnion(w1, []foldInterval{{start: 10.5, end: 10.5005}}), 90))
		}
		return hmc377R13Enrich(items), 2.903, nil
	case "Z9hostHasFamily":
		a := hmc377R11Inv(1.05, w1, 10, 15)
		b := hmc377R11Inv(0.853, w2, 16, 20)
		twin := hmc377R13Rem(1.0, w, 70)
		twin.inversionAccountRepresents = "inversion_account:z9"
		return hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), a, b, twin,
			hmc377R13InvDetail(2.903, w, rest, "inversion_account:z9", 30), hmc377R13SchedTwin(a), hmc377R13SchedTwin(b)}), 2.903, nil
	}
	return RootCauseRankResult{}, 0, nil
}

// Every universe of review 13 keeps one carrier per instant on the published
// board, with the gate leaving the board whole and with the gate withdrawing
// every chain inversion seat: Σ of the carriers of 6326 equals its raw ready
// time, no two rows wear one family key, no withdrawn row wears one, every
// family counts the rows riding under it, and the engine result is unchanged.
func TestHMC377R14ReviewUniversesKeepOneCarrierPerInstant(t *testing.T) {
	for _, name := range []string{"Z1own2", "Z1bNoHolder", "Z1cTwinOnly", "Z1dSatPart", "Z2tie3a", "Z2tie3b", "Z2tie3c",
		"Z4eq2a", "Z4eq2b", "Z4eq2sat", "Z7bothRem", "Z7bothSolo", "Z8twoAbs", "Z8twoAbsCpu", "Z9hostHasFamily"} {
		for _, withdraw := range []bool{false, true} {
			rank, raw, detail := hmc377R14Universe(name)
			if len(rank.Items) == 0 {
				t.Fatalf("%s: no universe", name)
			}
			before := hmc377R14Describe(rank.Items) + hmc377R14Describe(rank.AbsorbedItems)
			var board func(RootCauseRankItem) bool
			if withdraw {
				board = hmc377R13IsChainInversion
			} else {
				detail = nil
			}
			pub, withdrawn := hmc377R13Gate(rank, board, detail)
			for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
				t.Errorf("%s withdraw=%t: %s", name, withdraw, f)
			}
			if got := hmc377R13SeatCarriers(pub.Items); math.Abs(got-raw) > 0.0015 {
				t.Errorf("%s withdraw=%t: Σ carriers %.3f, want raw %.3f\n%s", name, withdraw, got, raw, hmc377R14Describe(pub.Items))
			}
			if after := hmc377R14Describe(rank.Items) + hmc377R14Describe(rank.AbsorbedItems); after != before {
				t.Errorf("%s withdraw=%t: the publication changed the engine result", name, withdraw)
			}
		}
	}
}
