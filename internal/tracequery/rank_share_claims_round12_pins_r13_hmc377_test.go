package tracequery

// HMC377 round 13 — batch-owner ruling R-377-13 ⑦ (2026-11-22): every
// round-12 branch that changes a value, a carrier or a word is pinned by a
// test its mutant fails. Review 12's surviving mutants with a reachable shape
// (fix12/review-pins/mutant/specs_tq.py, fix12/review-holders/mutant/mk.py;
// boards S4 / S6 of fix12/review-pins/probe/zz_reviewpins_probe_test.go):
//   - N25 / M23 — the successor-held part is measured less the instants a
//     seat of the claim's role still holds (S6);
//   - N33 — a share a withdrawn seat left whole with no successor takes the
//     context form (D6invOnly_w, D4invSolo_w);
//   - N50 — a carved share held in parts takes the see-detail form (S4);
//   - N28 — a claim without instants whose holder the gate withdrew is
//     withdrawn whole (a case-A' remainder's chain-account claim);
//   - N29 — only the claim's instants a withdrawn seat held are withdrawn (a
//     part the engine cap cut stays cut: the several-seat twin's detail);
//   - N51 — the LEVELMERGE-1 residual's share held in parts takes the
//     see-detail form (two claim seats, one withdrawn).
// The branches no reachable shape distinguishes are deleted in the same
// change (see the commit message).

import (
	"math"
	"strings"
	"testing"
)

func TestHMC377R13Round12BranchPins(t *testing.T) {
	t.Run("N25: a share a published seat of the claim's role still holds is not successor-held (S6)", func(t *testing.T) {
		// Two chain seats hold W (the inversion seat holds W and U, a runnable
		// chain seat W); the gate withdraws the inversion seat and the ◇
		// remainder takes W ∪ U as its successor; the satellite's W stays held
		// by the published chain seat.
		inv := hmc377R11Inv(2.903, []foldInterval{{start: 10.2, end: 10.201903}, {start: 10.3, end: 10.301}}, 10, 20)
		run := hmc377R11ChainRun(1.903, hmc377R11W(), 50, 55)
		rem := hmc377R11Remainder(1.0, 2.903, 3.903, 10.5, 70, 80)
		rspaSetSentence(&rem, rspaRemainderSentence(hmc377R11Dep, "runnable (scheduling-pressure candidate)", 1.0, 2.903, 3.903,
			rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 2.903, []foldInterval{{start: 10.2, end: 10.201903}, {start: 10.3, end: 10.301}})))
		items := []RootCauseRankItem{hmc377R11OtherSeat(), inv, run, rem, hmc377R11Satellite1()}
		resolveRankShareClaims(items, nil)
		board, _, _ := hmc377R12Publish(RootCauseRankResult{Items: items})
		sat := board[4]
		if !sat.ChainAnchorRepresentedByChainSeat || !strings.Contains(sat.Summary, "anchored share represented by the chain seat") ||
			strings.Contains(sat.Summary, rspaRepresentedSuccessor(1.903, 1.903)) {
			t.Errorf("represented=%t %q", sat.ChainAnchorRepresentedByChainSeat, sat.Summary)
		}
		if c := sat.ShareClaims[0]; c.heldSuccMs != 0 || math.Abs(c.heldOwnMs-1.903) > 0.0005 {
			t.Errorf("held own %.3f succ %.3f, want 1.903 / 0", c.heldOwnMs, c.heldSuccMs)
		}
	})
	t.Run("N33: a share its withdrawn holder left whole takes the context form (D6invOnly_w, D4invSolo_w)", func(t *testing.T) {
		for _, name := range []string{"D6invOnly", "D4invSolo"} {
			var rank RootCauseRankResult
			if name == "D6invOnly" {
				rank = hmc377R12Build("D6invOnly").rank
			} else {
				u := hmc377R12Build("UR5inv").rank
				for _, it := range u.Items {
					if !it.ChainAnchorRemainderSeat {
						rank.Items = append(rank.Items, it)
					}
				}
				rank.AbsorbedItems = u.AbsorbedItems
				resolveRankShareClaims(rank.Items, rank.AbsorbedItems)
			}
			board, _, _ := hmc377R12Publish(rank)
			det := board[len(board)-1]
			if det.Type != "priority_inversion_runnable_wait" || !strings.Contains(det.Summary, rspaInversionChainSeatContext) ||
				!strings.Contains(det.Summary, rspaInversionDetailHeadContext) || strings.Contains(det.Summary, hmc377R12SharePartsSeeDetail()) {
				t.Errorf("%s: %s %q", name, det.Type, det.Summary)
			}
		}
	})
	t.Run("N50: a carved share held in parts names no holder's tier (S4)", func(t *testing.T) {
		// A restriction seat straddles two chain seats of its thread; the
		// gate withdraws one: its carved share ends held in part by the
		// published chain seat and passed back in part.
		chainB := hmc377R11ChainRun(1.0, []foldInterval{{start: 10.3, end: 10.301}}, 50, 55)
		rank := hmc377R12Carve([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20), chainB,
			hmc377R12CPU([]foldInterval{{start: 10.2, end: 10.2005}, {start: 10.3, end: 10.3005}, {start: 10.6, end: 10.601}}, 90)})
		board, _, _ := hmc377R12Publish(rank)
		cpu := board[3]
		want := "; 1.000ms of it lies inside instants of this thread's chain seat, and " + hmc377R12SharePartsSeeDetail()
		if cpu.Type != "cpu_affinity_or_cpuset" || !strings.Contains(cpu.Summary, want) || strings.Contains(cpu.Summary, rspaHolderContextOnly) {
			t.Errorf("%s %q\nwant %q", cpu.Type, cpu.Summary, want)
		}
		if math.Abs(cpu.RunnableMs-2.0) > 0.0005 || math.Abs(cpu.ShareReturnedMs-0.5) > 0.0005 {
			t.Errorf("run=%.3f returned=%.3f, want 2.000 / 0.500", cpu.RunnableMs, cpu.ShareReturnedMs)
		}
	})
	t.Run("N28: a claim without instants whose holder the gate withdrew is withdrawn whole", func(t *testing.T) {
		// A case-A' remainder: its chain-account claim (no instants) names the
		// chain seat's own account; the gate withdraws that seat and no row
		// takes a value-only account — the context form, never the cap form.
		rem := hmc377R11Remainder(1.0, 1.903, 2.903, 10.5, 70, 80)
		rem.ChainAnchorOwnershipDivergent, rem.ChainAnchorChainLaneMs, rem.ChainAnchorCensusMs = true, 1.903, 1.903
		rspaSetSentence(&rem, rspaRemainderSentenceDivergent(hmc377R11Dep, "runnable (scheduling-pressure candidate)", 1.0, 1.903, 2.903, 1.903, 1.903,
			rspaClaim(RootCauseShareClaimChainAccount, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, nil)))
		items := []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20), rem}
		resolveRankShareClaims(items, nil)
		_, _, context := rspaChainAccountPhrases(1.903)
		if !items[2].ShareClaims[0].Resolved {
			t.Fatalf("precondition: the chain-account claim resolves to the chain seat on the engine run")
		}
		board, _, _ := hmc377R12Publish(RootCauseRankResult{Items: items})
		if s := board[2].Summary; !strings.Contains(s, context) {
			t.Errorf("%q\nwant %q", s, context)
		}
	})
	t.Run("N29: only the instants the withdrawn seat held are withdrawn (the cap-cut part stays cut)", func(t *testing.T) {
		// The several-seat twin's whole-account detail: the W2 seat was cut by
		// the engine cap; the gate withdraws the W1 seat and the ◇ twin takes
		// W1. The detail owns no instant — it keeps riding under the twin.
		inv := hmc377R11Inv(1.05, hmc377R11W1(), 10, 20)
		twin := hmc377R11Remainder(1.0, 1.903, 2.903, 10.5, 70, 80)
		det := hmc377R12InversionDetail(&twin, 2.903, []foldInterval{{start: 10.5, end: 10.501}})
		rank := RootCauseRankResult{Window: TimeWindow{StartTs: 10, EndTs: 11}, Items: []RootCauseRankItem{hmc377R11OtherSeat(), inv, twin}, AbsorbedItems: []RootCauseRankItem{det}}
		resolveRankShareClaims(rank.Items, rank.AbsorbedItems)
		board, detail, _ := hmc377R12Publish(rank)
		if len(detail) != 1 || detail[0].AbsorbedIntoFamily != board[2].RankFamilyKey || board[2].RankFamilyKey == "" {
			t.Fatalf("the detail left its published twin (detail rows %d, board rows %d)", len(detail), len(board))
		}
		if c := detail[0].ShareClaims[0]; c.restWithdrawnMs != 0 || math.Abs(c.heldSuccMs-1.05) > 0.0005 {
			t.Errorf("anchored claim: withdrawn rest %.3f succ %.3f, want 0 / 1.050 (W2 is cap-cut)", c.restWithdrawnMs, c.heldSuccMs)
		}
	})
	t.Run("N51: the LEVELMERGE-1 residual's share held in parts takes the see-detail form", func(t *testing.T) {
		agg := levelMergeTestAggregateSeat(200, 15, TimeWindow{StartTs: 10.000, EndTs: 10.010}, TimeWindow{StartTs: 10.020, EndTs: 10.030})
		inv1 := levelMergeTestInversionSeat(200, 3, 0, TimeWindow{StartTs: 10.002, EndTs: 10.005})
		inv2 := levelMergeTestInversionSeat(200, 3, 0, TimeWindow{StartTs: 10.022, EndTs: 10.025})
		inv2.LineStart, inv2.LineEnd = 400, 440
		for _, it := range []*RootCauseRankItem{&inv1, &inv2} {
			rspaHold(it, RootCauseShareHolderGatedShareSeat, false, []foldInterval{{start: it.StartTs, end: it.EndTs}}, 3)
		}
		items := splitAggregateGatedRunnableShare(ChainResult{Target: ThreadRef{PID: 100}}, []RootCauseRankItem{agg, inv1, inv2})
		resolveRankShareClaims(items, nil)
		withdrawn := make([]bool, len(items))
		residual := -1
		for i, it := range items {
			withdrawn[i] = it.Type == "priority_inversion_candidate" && it.LineStart == 300
			if it.GatedShareFullMs > 0 && !it.GatedShareConstituentSeat {
				residual = i
			}
		}
		if residual < 0 || items[residual].GatedShareClaimedMs <= 0 {
			t.Fatalf("precondition: the split ran (rows %d)", len(items))
		}
		board, _ := hmc377R12Withdraw(append([]RootCauseRankItem(nil), items...), nil, withdrawn, nil)
		if s := board[residual].Summary; !strings.Contains(s, "and "+hmc377R12SharePartsSeeDetail()) || strings.Contains(s, levelMergeSummaryClaimSeatContext) {
			t.Errorf("%q", s)
		}
	})
}

// R12N-1 (R-377-13 ⑤): the re-run owns the publication's rows — it writes the
// board and detail rows back into the publication it is given, so a detail
// the re-run returns to the board is published there without any caller
// assignment.
func TestHMC377R13WithdrawalWritesThePublicationBack(t *testing.T) {
	rank := hmc377R12Build("D1cpuIn").rank
	pub := rank
	pub.Items = append([]RootCauseRankItem(nil), rank.Items...)
	pub.AbsorbedItems = append([]RootCauseRankItem(nil), rank.AbsorbedItems...)
	withdrawn := make([]bool, len(pub.Items))
	for i, it := range pub.Items {
		withdrawn[i] = it.Type == "priority_inversion_candidate"
	}
	hmc377R13Withdraw(&pub, withdrawn, nil)
	if len(pub.AbsorbedItems) != 0 || len(pub.Items) != len(rank.Items)+1 || pub.Items[len(pub.Items)-1].Type != "cpu_affinity_or_cpuset" {
		t.Fatalf("publication rows: board %d detail %d (want %d / 0: the restriction row is published on the board)", len(pub.Items), len(pub.AbsorbedItems), len(rank.Items)+1)
	}
	if len(rank.AbsorbedItems) != 1 || rank.AbsorbedItems[0].AbsorbedIntoFamily == "" {
		t.Errorf("the engine result changed")
	}
}
