package tracequery

// HMC377 round 13, continuation — batch-owner ruling R-377-13 ⑤ applied to a
// lossless detail the publication gate itself withdrew, and ⑦ pins of two
// round-13 branches a second mutation pass found unpinned
// (~/hmc-work/hmc377/fix13/hold/mutant/cont/: X05, X06, X07).
//
// A detail the gate withdrew makes no claim (its claims go with its seat), so
// the re-run places it with the claimless details. Commit a6be6a95e then
// treated it like every other placed row: returned to the board it took the
// "tertiary" tier of a seat — the tool published a row the gate had made
// context as an adjacent carrier of its whole ready account
// (~/hmc-work/hmc377/fix13/hold/cont/exp/tool_head.out: `E3
// root_cause_adjacent val=1.903`, none on 532dc208d) —, riding under a new
// host it took the absorbed tier (the context tier the gate wrote was lost),
// and standing on the board it hosted the detail it owned (a withdrawn row
// wearing a family). The detail is a withdrawn row too: it keeps the context
// tier wherever it lands and it hosts nothing.

import (
	"math"
	"strings"
	"testing"
)

func hmc377R13IsInversionAccount(it RootCauseRankItem) bool {
	return it.Type == "priority_inversion_runnable_wait"
}

// hmc377R13ChurnOf is the claimless state-churn twin of det (the B4 reconcile
// pairs it with det: transitive ownership churn → det → chain seat).
func hmc377R13ChurnOf(det RootCauseRankItem) RootCauseRankItem {
	ms := det.RunnableMs
	return RootCauseRankItem{Rank: 7, Tier: "tertiary", Type: "fragmented_runnable_wait", Source: "window_stats.state_churn", Thread: hmc377R11Dep,
		ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, ProjectedImpactMs: ms, RunnableMs: ms, DominantState: "runnable",
		ChainRelevance: det.ChainRelevance, Causality: det.Causality, Confidence: 0.5, StatsWindowStartTs: 10, StatsWindowEndTs: 11,
		StartTs: det.StartTs, EndTs: det.EndTs, LineStart: 200, LineEnd: 210, Summary: "binder:458_5-6326 fragmented runnable wait"}
}

// hmc377R13SeatCarriers sums what the seats of 6326 on a published board
// carry: a context-tier row, a lossless detail and a represented row carry
// nothing.
func hmc377R13SeatCarriers(board []RootCauseRankItem) float64 {
	sum := 0.0
	for _, it := range board {
		if it.Thread.PID != hmc377R11Dep.PID || it.Tier == RootCauseTierContextOnly || it.AbsorbedByRankFamily || it.ChainAnchorRepresentedByChainSeat {
			continue
		}
		if it.Type == "priority_inversion_candidate" {
			sum += it.GatedRunnableMs
			continue
		}
		sum += it.RunnableMs
	}
	return sum
}

func TestHMC377R13GateWithdrawnDetailStaysContext(t *testing.T) {
	rest := []foldInterval{{start: 10.5, end: 10.501}}
	for _, tc := range []struct {
		name string
		full float64
		rest []foldInterval
	}{
		{"its whole account inside the withdrawn seat's windows", 1.903, nil},
		{"with a rest outside them", 2.903, rest},
	} {
		t.Run("no published row holds its withdrawn owner's account: "+tc.name, func(t *testing.T) {
			inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
			inv.inversionAccountRepresents = "inversion_account:wd"
			rank := hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), inv, hmc377R13InvDetail(tc.full, hmc377R11W(), tc.rest, "inversion_account:wd", 30)})
			if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 1 {
				t.Fatalf("precondition: the chain seat absorbs the inversion detail: board %d detail %d", len(rank.Items), len(rank.AbsorbedItems))
			}
			pub, withdrawn := hmc377R13Gate(rank, hmc377R13IsChainInversion, hmc377R13IsInversionAccount)
			for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
				t.Errorf("%s", f)
			}
			if len(pub.AbsorbedItems) != 0 || len(pub.Items) != 3 {
				t.Fatalf("published board %d detail %d, want the withdrawn detail on the board (a withdrawn row absorbs nothing)", len(pub.Items), len(pub.AbsorbedItems))
			}
			det := pub.Items[2]
			if det.Tier != RootCauseTierContextOnly || det.Rank != 0 || det.BackgroundRank != 0 || det.AbsorbedByRankFamily || len(det.ShareClaims) != 0 {
				t.Errorf("the withdrawn detail is published as tier %q rank %d (claims %d), want the context row the gate wrote", det.Tier, det.Rank, len(det.ShareClaims))
			}
			if det.RankFamilyKey != "" || det.AbsorbedRankRows != 0 {
				t.Errorf("the withdrawn detail wears a family: key=%q rows=%d", det.RankFamilyKey, det.AbsorbedRankRows)
			}
			if got := hmc377R13SeatCarriers(pub.Items); got > 0.0005 {
				t.Errorf("Σ carriers %.3f, want none (the gate published both rows of the thread as context)", got)
			}
		})
	}
	t.Run("riding under the published row that holds its withdrawn owner's account it keeps the context tier", func(t *testing.T) {
		rank, raw, board, detail := hmc377R13Build("UR5both")
		pub, withdrawn := hmc377R13Gate(rank, board, detail)
		host, ok := hmc377R13HostOf(pub, "unknown_state")
		if !ok || !host.ChainAnchorRemainderSeat {
			t.Fatalf("the withdrawn detail rides under %q, want the ◇ remainder that took its withdrawn owner's account", host.Type)
		}
		if len(pub.AbsorbedItems) != 1 || pub.AbsorbedItems[0].Tier != RootCauseTierContextOnly || !pub.AbsorbedItems[0].AbsorbedByRankFamily {
			t.Errorf("the withdrawn detail rides as tier %q, want the context tier the gate wrote", pub.AbsorbedItems[0].Tier)
		}
		if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-raw) > 0.0015 {
			t.Errorf("Σ carriers %.3f, want raw %.3f", got, raw)
		}
	})
	t.Run("the detail it owns rides beside it under its host", func(t *testing.T) {
		// Q1cChurn with the inversion detail's own priority proof failing too:
		// the ◇ remainder takes the withdrawn chain seat's W; the withdrawn
		// detail rides under it as context and its state-churn twin beside it.
		rank, raw, board, _ := hmc377R13Build("Q1cChurn")
		pub, withdrawn := hmc377R13Gate(rank, board, hmc377R13IsInversionAccount)
		for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
			t.Errorf("%s", f)
		}
		detHost, ok1 := hmc377R13HostOf(pub, "unknown_state")
		churnHost, ok2 := hmc377R13HostOf(pub, "fragmented_runnable_wait")
		if !ok1 || !ok2 || !detHost.ChainAnchorRemainderSeat || churnHost.RankFamilyKey != detHost.RankFamilyKey {
			t.Fatalf("withdrawn detail under %q, its churn twin under %q (want both under the ◇ remainder)", detHost.Type, churnHost.Type)
		}
		for _, d := range pub.AbsorbedItems {
			want := RootCauseTierAbsorbed
			if d.Type == "unknown_state" {
				want = RootCauseTierContextOnly
			}
			if d.Tier != want {
				t.Errorf("detail %s rides as tier %q, want %q", d.Type, d.Tier, want)
			}
		}
		if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-raw) > 0.0015 {
			t.Errorf("Σ carriers %.3f, want raw %.3f", got, raw)
		}
	})
	t.Run("standing on the board it hosts nothing", func(t *testing.T) {
		// The withdrawn chain seat's account passes to no published row: the
		// withdrawn detail stands on the board as context. Its state-churn
		// twin does not ride under a withdrawn row — it follows the withdrawn
		// seat's account and, no row holding it, is published with its own
		// value (the thread's ready time keeps one carrier), whichever of the
		// two the re-run places first.
		for _, churnFirst := range []bool{false, true} {
			inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
			inv.inversionAccountRepresents = "inversion_account:wh"
			det := hmc377R13InvDetail(2.903, hmc377R11W(), rest, "inversion_account:wh", 30)
			det.StatsWindowStartTs, det.StatsWindowEndTs = 10, 11
			items := []RootCauseRankItem{hmc377R11OtherSeat(), inv, det, hmc377R13ChurnOf(det)}
			if churnFirst {
				items[2], items[3] = items[3], items[2]
			}
			rank := hmc377R13Enrich(items)
			if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 2 || (rank.AbsorbedItems[0].Type == "fragmented_runnable_wait") != churnFirst {
				t.Fatalf("churnFirst=%t precondition: the chain seat absorbs the detail and its churn twin in the listed order: board %d detail %d", churnFirst, len(rank.Items), len(rank.AbsorbedItems))
			}
			pub, withdrawn := hmc377R13Gate(rank, hmc377R13IsChainInversion, hmc377R13IsInversionAccount)
			for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
				t.Errorf("churnFirst=%t: %s", churnFirst, f)
			}
			if len(pub.AbsorbedItems) != 0 || len(pub.Items) != 4 {
				t.Fatalf("churnFirst=%t: published board %d detail %d, want both former details on the board", churnFirst, len(pub.Items), len(pub.AbsorbedItems))
			}
			for _, it := range pub.Items[2:] {
				switch it.Type {
				case "unknown_state":
					if it.Tier != RootCauseTierContextOnly || it.RankFamilyKey != "" || it.AbsorbedRankRows != 0 {
						t.Errorf("churnFirst=%t: the withdrawn detail tier %q key %q rows %d, want a context row that hosts nothing", churnFirst, it.Tier, it.RankFamilyKey, it.AbsorbedRankRows)
					}
				case "fragmented_runnable_wait":
					if it.Tier != "tertiary" || it.AbsorbedByRankFamily || math.Abs(it.RunnableMs-2.903) > 0.0005 {
						t.Errorf("churnFirst=%t: the churn twin tier %q run %.3f, want its own seat carrying 2.903", churnFirst, it.Tier, it.RunnableMs)
					}
				default:
					t.Errorf("churnFirst=%t: unexpected row %s", churnFirst, it.Type)
				}
			}
			if got := hmc377R13SeatCarriers(pub.Items); math.Abs(got-2.903) > 0.0015 {
				t.Errorf("churnFirst=%t: Σ carriers %.3f, want raw 2.903 once", churnFirst, got)
			}
		}
	})
}

// R-377-13 ⑥ pin (continuation mutant X06): one disposition and one holder
// row is read from the final run of the resolver. When a released row that
// owns nothing rides as detail again, the board rows behind it move up; the
// successor row a claim named on the earlier run is not kept beside the
// position it holds now (two rows would make a one-successor share take the
// see-detail form).
func TestHMC377R13SuccessorRowsAreThoseOfTheFinalRun(t *testing.T) {
	for _, detFirst := range []bool{true, false} {
		// The withdrawn chain seat absorbs an inversion detail (its whole
		// account inside W) and a restriction seat over W; a represented
		// satellite claims W. The restriction seat returns to the board and
		// takes W (successor tier 2); the detail owns nothing and rides under
		// it — listed first it leaves the board from a position before the
		// restriction seat's.
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		inv.inversionAccountRepresents = "inversion_account:x6"
		det := hmc377R13InvDetail(1.903, hmc377R11W(), nil, "inversion_account:x6", 30)
		cpu := hmc377R12CPU(hmc377R11W(), 90)
		sat := hmc377R11Satellite1()
		sat.LineStart, sat.LineEnd = 300, 310
		items := []RootCauseRankItem{hmc377R11OtherSeat(), inv, sat, det, cpu}
		if !detFirst {
			items[3], items[4] = items[4], items[3]
		}
		rank := hmc377R13Enrich(items)
		if len(rank.AbsorbedItems) != 2 || (rank.AbsorbedItems[0].Type == "priority_inversion_runnable_wait") != detFirst {
			t.Fatalf("detFirst=%t precondition: the chain seat absorbs the detail and the restriction seat in the listed order: %d", detFirst, len(rank.AbsorbedItems))
		}
		pub, withdrawn := hmc377R13Gate(rank, hmc377R13IsChainInversion, nil)
		for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
			t.Errorf("detFirst=%t: %s", detFirst, f)
		}
		if host, ok := hmc377R13HostOf(pub, "priority_inversion_runnable_wait"); !ok || host.Type != "cpu_affinity_or_cpuset" {
			t.Fatalf("detFirst=%t: the detail rides under %q, want the restriction seat that took W", detFirst, host.Type)
		}
		found := false
		for _, it := range pub.Items {
			if !strings.Contains(it.Summary, "runnable satellite") {
				continue
			}
			found = true
			if !strings.Contains(it.Summary, rspaRepresentedSuccessor(1.903, 1.903)) || strings.Contains(it.Summary, hmc377R12RepresentedSeeDetail(1.903, 1.903, 1.903)) {
				t.Errorf("detFirst=%t: one successor row holds the satellite's share, want the successor form: %q", detFirst, it.Summary)
			}
		}
		if !found {
			t.Fatalf("detFirst=%t: no satellite published", detFirst)
		}
		if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-1.903) > 0.0015 {
			t.Errorf("detFirst=%t: Σ carriers %.3f, want raw 1.903", detFirst, got)
		}
	}
}

// R-377-13 ⑤ pin (continuation mutant X07): the part of a released row's
// account a holder holds is the claim's instants it holds, never its value. A
// published chain seat holds W2 (0.853) of the detail's W inside a 3.853
// account; the ◇ remainder took W1 (1.050) from the withdrawn seat — the
// detail rides under the ◇.
func TestHMC377R13AHoldersPartIsTheClaimsInstantsItHolds(t *testing.T) {
	a := hmc377R11Inv(1.05, hmc377R11W1(), 10, 15)
	a.inversionAccountRepresents = "inversion_account:x7"
	b := hmc377R11ChainRun(3.853, rspaFoldUnion(hmc377R11W2(), []foldInterval{{start: 10.25, end: 10.253}}), 16, 20)
	rank := hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), a, b, hmc377R13Rem(1.0, hmc377R11W(), 70),
		hmc377R13InvDetail(1.903, hmc377R11W(), nil, "inversion_account:x7", 30)})
	if len(rank.AbsorbedItems) != 1 {
		t.Fatalf("precondition: the W1 chain seat absorbs the inversion detail: %d", len(rank.AbsorbedItems))
	}
	pub, withdrawn := hmc377R13Gate(rank, hmc377R13IsChainInversion, nil)
	for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
		t.Errorf("%s", f)
	}
	host, ok := hmc377R13HostOf(pub, "priority_inversion_runnable_wait")
	if !ok || !host.ChainAnchorRemainderSeat {
		t.Errorf("the detail rides under %q (%s), want the ◇ remainder holding 1.050 of its 1.903 (the chain seat holds 0.853 of it)", host.Type, host.Source)
	}
	if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-5.903) > 0.0015 {
		t.Errorf("Σ carriers %.3f, want raw 5.903", got)
	}
}
