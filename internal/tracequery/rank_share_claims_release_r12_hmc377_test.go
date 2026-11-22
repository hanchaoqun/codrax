package tracequery

// HMC377 round 12 — batch-owner ruling R-377-12 ①② (2026-11-22).
//
// Review 11 (lens holders) reproduced, in universes built with the engine's
// own typed internals (~/hmc-work/hmc377/fix11/review-holders/univ/
// zz_rvh11_universe_export.go; evidence F1_*.txt, F2_*.txt, F3_*.txt,
// F4_*.txt; summary_H_table.txt):
//   - R11H-1: lossless-detail pairing was decided in the engine before the
//     publication gate, and the successor rule handed a withdrawn seat's
//     segments to an absorbed detail that holds no ◎ seat (tier 6), so those
//     instants had no carrier: D3cpuEmpty_w Σ1.500 / raw 2.903,
//     D5cpuEmptyNoW_w 1.000 / 2.903, D6invOnly_w no member / 1.903;
//   - R11H-2: a restriction seat all inside one chain-object row could not
//     take its instants back when that row was withdrawn (D1cpuIn_w no
//     member; D2cpuInRem_w: the ◇ remainder carried the restriction's 1.0 ms
//     while the absorbed restriction row still said the chain seat
//     "already publishes" it);
//   - R11H-3 / R11H-4: a share with several dispositions was worded as one —
//     the cap-cut rest called "published only as context" (E1satMix_w,
//     E3satMixW_w), a share held partly by its original holder and partly by
//     a successor called all-successor (C2part_w, H1lm_w, H1lmR_w).
// Ruling ①: every "rides as lossless detail" relation is a share relation of
// the one resolver, decided again on the gate's re-run; a detail row rides as
// detail only while its absorber is published and it owns no instant, and is
// never a successor. Ruling ②: a specific holder form only for one
// disposition; every mixed disposition takes the see-detail form.
//
// The gate's decision is the typed withdrawn flags (the tool's priority proof
// boundary sets them); the ◎ carrier model is the one of
// rank_share_claims_segments_r11_hmc377_test.go, over published board rows
// only (a lossless-detail row holds no ◎ seat). The publication is read through
// rank_share_claims_release_shim_hmc377_test.go (the red run on fa2f01465
// swaps in that commit's in-place signature).

import (
	"math"
	"strings"
	"testing"
)

// hmc377R12CPU is a cpu_affinity_or_cpuset seat of thread 6326 over the given
// restricted instants (the enrich pass's restriction seat).
func hmc377R12CPU(intervals []foldInterval, l0 int) RootCauseRankItem {
	ms := rspaFoldLengthMs(intervals)
	return RootCauseRankItem{Rank: 5, Tier: "tertiary", Type: "cpu_affinity_or_cpuset", Source: "window_stats.cpu_constraints", Thread: hmc377R11Dep,
		DominantState: string(StateRunnable), RunnableMs: ms, EffectiveImpactMs: ms, ImpactMs: ms, CumulativeImpactMs: ms,
		ProjectedImpactMs: ms, Confidence: 0.7, Summary: "binder:458_5-6326 runnable under a CPU affinity/cpuset restriction",
		cpuConstraintRestrictedIntervals: rspaFoldCopy(intervals),
		ChainCredentialLaneDemoted:       true, Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent",
		StartTs: intervals[0].start, EndTs: intervals[len(intervals)-1].end, LineStart: l0, LineEnd: l0 + 2}
}

// hmc377R12Carve runs the enrich pass's order on a hand board: the
// restriction carve, then the B4 reconcile (lossless details), then the one
// resolver.
func hmc377R12Carve(items []RootCauseRankItem) RootCauseRankResult {
	carveCPUConstraintReadyInstants(items)
	rank := RootCauseRankResult{Window: TimeWindow{StartTs: 10, EndTs: 11}, Items: items}
	reconcileExactCrossTypeRankSeats(&rank)
	resolveRankShareClaims(rank.Items, rank.AbsorbedItems)
	return rank
}

// hmc377R12InversionDetail is an R-377-4 inversion window account of 6326
// (ready account full, anchored inside W) paired under absorber as its
// lossless detail (the B4 key the reconcile mints).
func hmc377R12InversionDetail(absorber *RootCauseRankItem, full float64, remainder []foldInterval) RootCauseRankItem {
	det := RootCauseRankItem{Type: "priority_inversion_runnable_wait", Source: "window_stats", Thread: hmc377R11Dep,
		ImpactMs: full, CumulativeImpactMs: full, EffectiveImpactMs: full, RunnableMs: full, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.7, StartTs: 10.2, EndTs: 10.501, LineStart: 30, LineEnd: 40,
		Summary: "binder:458_5-6326 same-CPU lower-priority overlap over the ready account"}
	var remainderClaim RootCauseShareClaim
	if len(remainder) > 0 {
		remainderClaim = rspaClaim(RootCauseShareClaimRemainder, RootCauseShareHolderRemainderSeat, hmc377R11Dep.PID, false, rspaFoldLengthMs(remainder), remainder)
	}
	rspaAppendSentence(&det, rspaInversionRepresentedSummary(1.903, full,
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, hmc377R11W()), remainderClaim))
	key := crossTypeRankSeatReconKey(*absorber, det, TimeWindow{StartTs: 10, EndTs: 11})
	det.Rank, det.BackgroundRank, det.Tier = 0, 0, RootCauseTierAbsorbed
	det.AbsorbedByRankFamily, det.AbsorbedIntoFamily = true, key
	absorber.RankFamilyKey, absorber.AbsorbedRankRows = key, 1
	return det
}

type hmc377R12Universe struct {
	rank RootCauseRankResult
	raw  float64
	// carriers: the published rows of 6326 the universe's hand oracle says
	// carry its ready time once the gate withdraws the inversion seat
	// (type → value).
	carriers map[string]float64
}

func hmc377R12Build(name string) hmc377R12Universe {
	r12Remainder := func(remainder float64, l0 int) RootCauseRankItem {
		return hmc377R11Remainder(remainder, 1.903, remainder+1.903, 10.5, l0, l0+10)
	}
	switch name {
	case "D1cpuIn":
		return hmc377R12Universe{rank: hmc377R12Carve([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20),
			hmc377R12CPU([]foldInterval{{start: 10.2, end: 10.201}}, 90)}), raw: 1.903,
			carriers: map[string]float64{"cpu_affinity_or_cpuset": 1.0}}
	case "D2cpuInRem":
		return hmc377R12Universe{rank: hmc377R12Carve([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20),
			hmc377R12CPU([]foldInterval{{start: 10.2, end: 10.201}}, 90), r12Remainder(1.0, 70)}), raw: 2.903,
			carriers: map[string]float64{"cpu_affinity_or_cpuset": 1.0, "runnable_wait": 1.903}}
	case "D3cpuEmpty":
		return hmc377R12Universe{rank: hmc377R12Carve([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20),
			hmc377R12CPU([]foldInterval{{start: 10.2, end: 10.2005}, {start: 10.5, end: 10.501}}, 90), r12Remainder(1.0, 70)}), raw: 2.903,
			carriers: map[string]float64{"cpu_affinity_or_cpuset": 1.5, "runnable_wait": 1.403}}
	case "D5cpuEmptyNoW":
		return hmc377R12Universe{rank: hmc377R12Carve([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20),
			hmc377R12CPU([]foldInterval{{start: 10.5, end: 10.501}}, 90), r12Remainder(1.0, 70)}), raw: 2.903,
			carriers: map[string]float64{"cpu_affinity_or_cpuset": 1.0, "runnable_wait": 1.903}}
	case "D6invOnly":
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		det := hmc377R12InversionDetail(&inv, 1.903, nil)
		rank := RootCauseRankResult{Window: TimeWindow{StartTs: 10, EndTs: 11}, Items: []RootCauseRankItem{hmc377R11OtherSeat(), inv}, AbsorbedItems: []RootCauseRankItem{det}}
		resolveRankShareClaims(rank.Items, rank.AbsorbedItems)
		return hmc377R12Universe{rank: rank, raw: 1.903, carriers: map[string]float64{"priority_inversion_runnable_wait": 1.903}}
	case "UR5inv":
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		rem := hmc377R11Remainder(1.0, 1.903, 2.903, 10.5, 70, 80)
		det := hmc377R12InversionDetail(&inv, 2.903, []foldInterval{{start: 10.5, end: 10.501}})
		rank := RootCauseRankResult{Window: TimeWindow{StartTs: 10, EndTs: 11}, Items: []RootCauseRankItem{hmc377R11OtherSeat(), inv, rem}, AbsorbedItems: []RootCauseRankItem{det}}
		resolveRankShareClaims(rank.Items, rank.AbsorbedItems)
		return hmc377R12Universe{rank: rank, raw: 2.903, carriers: map[string]float64{"runnable_wait": 2.903}}
	}
	return hmc377R12Universe{}
}

// hmc377R12Publish applies the gate's withdrawal of every inversion chain
// seat to a published copy.
func hmc377R12Publish(rank RootCauseRankResult) ([]RootCauseRankItem, []RootCauseRankItem, []bool) {
	board := append([]RootCauseRankItem(nil), rank.Items...)
	detail := append([]RootCauseRankItem(nil), rank.AbsorbedItems...)
	withdrawn := make([]bool, len(board))
	for i, it := range board {
		withdrawn[i] = it.Type == "priority_inversion_candidate"
	}
	board, detail = hmc377R12Withdraw(board, detail, withdrawn, nil)
	return board, detail, withdrawn
}

// hmc377R12Carriers are the published board rows of 6326 that are not the
// withdrawn seat, by type, and their ◎ carrier sum (a lossless-detail row
// holds no ◎ seat).
func hmc377R12Carriers(board []RootCauseRankItem, withdrawn []bool) (map[string]float64, float64) {
	out := map[string]float64{}
	sum := 0.0
	for i, it := range board {
		if it.Thread.PID != hmc377R11Dep.PID || (i < len(withdrawn) && withdrawn[i]) || it.AbsorbedByRankFamily {
			continue
		}
		switch {
		case it.ChainAnchorRepresentedByChainSeat:
			continue
		case it.Type == "priority_inversion_candidate":
			out[it.Type] += it.GatedRunnableMs
			sum += it.GatedRunnableMs
		default:
			out[it.Type] += it.RunnableMs
			sum += it.RunnableMs
		}
	}
	return out, sum
}

func TestHMC377R12DetailPairingIsDecidedAgainOnTheGatesReRun(t *testing.T) {
	for _, name := range []string{"D1cpuIn", "D2cpuInRem", "D3cpuEmpty", "D5cpuEmptyNoW", "D6invOnly", "UR5inv"} {
		t.Run(name+"_c: the engine's pairing stands on a board the gate leaves whole", func(t *testing.T) {
			u := hmc377R12Build(name)
			if len(u.rank.AbsorbedItems) != 1 {
				t.Fatalf("precondition: one lossless-detail row, got %d", len(u.rank.AbsorbedItems))
			}
			board := append([]RootCauseRankItem(nil), u.rank.Items...)
			detail := append([]RootCauseRankItem(nil), u.rank.AbsorbedItems...)
			gotBoard, gotDetail := hmc377R12Withdraw(board, detail, make([]bool, len(board)), nil)
			if len(gotBoard) != len(u.rank.Items) || len(gotDetail) != 1 || gotDetail[0].Summary != u.rank.AbsorbedItems[0].Summary {
				t.Errorf("no withdrawal: board %d detail %d (want %d / 1, detail untouched)", len(gotBoard), len(gotDetail), len(u.rank.Items))
			}
		})
		t.Run(name+"_w: one carrier per instant after the withdrawal", func(t *testing.T) {
			u := hmc377R12Build(name)
			board, detail, withdrawn := hmc377R12Publish(u.rank)
			got, sum := hmc377R12Carriers(board, withdrawn)
			for typ, want := range u.carriers {
				if math.Abs(got[typ]-want) > 0.0015 {
					t.Errorf("carrier %s = %.3f, want %.3f (all carriers %v)", typ, got[typ], want, got)
				}
			}
			want := 0.0
			for _, v := range u.carriers {
				want += v
			}
			if math.Abs(sum-want) > 0.0015 {
				t.Errorf("Σ carriers %.3f, want %.3f (raw %.3f): %v", sum, want, u.raw, got)
			}
			for _, d := range detail {
				for _, h := range d.rspaHeld {
					if h.successor {
						t.Errorf("a lossless-detail row is never a successor: %s holds %+v", d.Type, h)
					}
				}
			}
		})
	}
	t.Run("D1cpuIn_w: the restriction row returns with its own words", func(t *testing.T) {
		board, detail, _ := hmc377R12Publish(hmc377R12Build("D1cpuIn").rank)
		if len(detail) != 0 {
			t.Fatalf("the restriction seat whose absorber the gate withdrew still rides as detail")
		}
		cpu := board[len(board)-1]
		if cpu.Type != "cpu_affinity_or_cpuset" || cpu.AbsorbedByRankFamily || cpu.Tier == RootCauseTierAbsorbed {
			t.Fatalf("released row %s tier %q absorbed=%t", cpu.Type, cpu.Tier, cpu.AbsorbedByRankFamily)
		}
		if want := "; all 1.000ms of this ready time under the CPU restriction lies inside instants of this thread's on-chain seat, which is published only as context, tier context_only, so this row counts them with its own value"; !strings.Contains(cpu.Summary, want) {
			t.Errorf("summary %q\nwant %q", cpu.Summary, want)
		}
		if strings.Contains(cpu.Summary, "already publishes") {
			t.Errorf("the released row still says the withdrawn seat publishes its instants: %q", cpu.Summary)
		}
	})
	t.Run("D2cpuInRem_w: the cause-specific seat keeps its instants, the ◇ the rest", func(t *testing.T) {
		board, _, _ := hmc377R12Publish(hmc377R12Build("D2cpuInRem").rank)
		for _, it := range board {
			if it.Thread.PID == hmc377R11Dep.PID && it.ChainAnchorRemainderSeat && it.Type == "runnable_wait" &&
				!strings.Contains(it.Summary, "("+hmc377R12SharePartsSeeDetail()+")") {
				t.Errorf("the ◇ anchored share is held in part by the restriction seat (successor) and in part by the ◇ itself: %q", it.Summary)
			}
		}
	})
	t.Run("D6invOnly_w: the released inversion detail says it counts on its own row", func(t *testing.T) {
		board, _, _ := hmc377R12Publish(hmc377R12Build("D6invOnly").rank)
		det := board[len(board)-1]
		if det.Type != "priority_inversion_runnable_wait" || !strings.Contains(det.Summary, hmc377R12TailReleased()) ||
			strings.Contains(det.Summary, rspaInversionDetailTailContext) {
			t.Errorf("released inversion detail %s: %q", det.Type, det.Summary)
		}
	})
	t.Run("UR5inv_w: a detail every instant of which another published row holds rides under that row", func(t *testing.T) {
		board, detail, _ := hmc377R12Publish(hmc377R12Build("UR5inv").rank)
		if len(detail) != 1 {
			t.Fatalf("detail rows %d, want the inversion detail", len(detail))
		}
		var rem *RootCauseRankItem
		for i := range board {
			if board[i].ChainAnchorRemainderSeat {
				rem = &board[i]
			}
		}
		if rem == nil || rem.RankFamilyKey == "" || detail[0].AbsorbedIntoFamily != rem.RankFamilyKey || !detail[0].AbsorbedByRankFamily {
			t.Fatalf("the detail must ride under the published ◇ row that holds its account: into=%q ◇ key=%+v", detail[0].AbsorbedIntoFamily, rem)
		}
		if detail[0].EffectiveImpactMs != 2.903 || !strings.Contains(detail[0].Summary, rspaInversionDetailTailContext) {
			t.Errorf("a lossless detail keeps every value and says it rides as detail: eff=%.3f %q", detail[0].EffectiveImpactMs, detail[0].Summary)
		}
		again, againDetail := hmc377R12Withdraw(append([]RootCauseRankItem(nil), board...), append([]RootCauseRankItem(nil), detail...), make([]bool, len(board)), nil)
		if len(again) != len(board) || len(againDetail) != 1 || againDetail[0].Summary != detail[0].Summary {
			t.Errorf("a second publication is not a no-op")
		}
	})
}

// R-377-12 ②: one disposition → its specific holder form; a mix → the
// see-detail form, which names no holder (values unchanged).
func TestHMC377R12MixedDispositionTakesTheSeeDetailForm(t *testing.T) {
	sat := func(name string, ms float64, windows []foldInterval) RootCauseRankItem {
		s := hmc377R11Satellite(ms, windows[0].start, windows[len(windows)-1].end,
			rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, ms, windows))
		s.LineStart, s.LineEnd = 30, 40
		return s
	}
	w13 := []foldInterval{{start: 10.2, end: 10.202903}}
	publish := func(items []RootCauseRankItem) []RootCauseRankItem {
		resolveRankShareClaims(items, nil)
		board, _, _ := hmc377R12Publish(RootCauseRankResult{Items: items})
		return board
	}
	t.Run("E1satMix_w: held + passed back + cap-cut", func(t *testing.T) {
		board := publish([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.05, hmc377R11W1(), 10, 20), hmc377R11ChainRun(0.853, hmc377R11W2(), 21, 25), sat("E1", 2.903, w13)})
		s := board[3]
		if want := hmc377R12RepresentedSeeDetail(2.903, 2.903, 0.853); !strings.Contains(s.Summary, want) || math.Abs(s.EffectiveImpactMs-2.05) > 0.0005 {
			t.Errorf("eff=%.3f summary %q\nwant %q", s.EffectiveImpactMs, s.Summary, want)
		}
		if strings.Contains(s.Summary, rspaHolderContextOnly) {
			t.Errorf("the cap-cut rest is never worded as published only as context: %q", s.Summary)
		}
	})
	t.Run("E3satMixW_w: passed back + cap-cut", func(t *testing.T) {
		board := publish([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.05, hmc377R11W1(), 10, 20), sat("E3", 2.903, w13)})
		s := board[2]
		if want := hmc377R12RepresentedSeeDetail(2.903, 2.903, 0); !strings.Contains(s.Summary, want) || math.Abs(s.EffectiveImpactMs-2.903) > 0.0005 {
			t.Errorf("eff=%.3f summary %q\nwant %q", s.EffectiveImpactMs, s.Summary, want)
		}
	})
	t.Run("UR4rem_w: all passed back keeps the returned form", func(t *testing.T) {
		board := publish([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.05, hmc377R11W1(), 10, 20), hmc377R11Inv(0.853, hmc377R11W2(), 21, 25),
			hmc377R11Remainder(5, 1.903, 6.903, 10.5, 60, 90)})
		if s := board[3].Summary; !strings.Contains(s, rspaSummaryOwnedReturned) {
			t.Errorf("%q", s)
		}
	})
	t.Run("UR4mix_w: held + passed back on a ◇ owner phrase", func(t *testing.T) {
		board := publish([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.05, hmc377R11W1(), 10, 20), hmc377R11ChainRun(0.853, hmc377R11W2(), 21, 25),
			hmc377R11Remainder(5, 1.903, 6.903, 10.5, 60, 90)})
		if s := board[3].Summary; !strings.Contains(s, hmc377R12OwnedSeeDetail()) || math.Abs(board[3].EffectiveImpactMs-6.05) > 0.0005 {
			t.Errorf("eff=%.3f %q", board[3].EffectiveImpactMs, s)
		}
	})
	t.Run("T1ab_w: all held by one successor keeps the successor form", func(t *testing.T) {
		a := sat("A", 1.903, hmc377R11W())
		b := sat("B", 1.903, hmc377R11W())
		b.Type, b.LineStart, b.LineEnd = "scheduler_latency", 36, 46
		board := publish([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20), a, b})
		if s := board[3].Summary; !strings.Contains(s, rspaRepresentedSuccessor(1.903, 1.903)) {
			t.Errorf("%q", s)
		}
	})
	t.Run("C2part_w: held by the chain seat + held by a successor", func(t *testing.T) {
		board := publish([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.05, hmc377R11W1(), 10, 20), hmc377R11ChainRun(0.853, hmc377R11W2(), 21, 25),
			sat("C2", 1.903, hmc377R11W()), hmc377R11Remainder(5, 1.903, 6.903, 10.5, 70, 80)})
		s := board[3]
		if want := hmc377R12RepresentedSeeDetail(1.903, 1.903, 1.903); !strings.Contains(s.Summary, want) || !s.ChainAnchorRepresentedByChainSeat {
			t.Errorf("represented=%t %q\nwant %q", s.ChainAnchorRepresentedByChainSeat, s.Summary, want)
		}
	})
	t.Run("UR1cut_c: the engine cap alone keeps the partial form", func(t *testing.T) {
		items := []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.05, hmc377R11W1(), 10, 20), sat("UR1", 1.903, hmc377R11W())}
		resolveRankShareClaims(items, nil)
		if want := hmc377R12RepresentedPartial(1.903, 1.903, 1.05, 0.853); !strings.Contains(items[2].Summary, want) {
			t.Errorf("%q\nwant %q", items[2].Summary, want)
		}
	})
}

// Register R11T-2 (review-pins mutants P04, P06, P51, P13) and R11T-3 (P20):
// each pin fails under its mutant.
func TestHMC377R12SuccessorRuleValuePins(t *testing.T) {
	t.Run("P04: a share without instants passes whole to one successor only", func(t *testing.T) {
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		inv.rspaHeld = nil
		rspaHold(&inv, RootCauseShareHolderChainSeat, false, nil, 1.903)
		rem := hmc377R11Remainder(1.0, 1.903, 2.903, 10.5, 70, 80)
		rspaSetSentence(&rem, rspaRemainderSentence(hmc377R11Dep, "runnable (scheduling-pressure candidate)", 1.0, 1.903, 2.903,
			rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, nil)))
		s := hmc377R11Satellite(1.903, 10.2, 10.201903, rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, nil))
		items := []RootCauseRankItem{hmc377R11OtherSeat(), inv, rem, s}
		resolveRankShareClaims(items, nil)
		board, _, withdrawn := hmc377R12Publish(RootCauseRankResult{Items: items})
		if _, sum := hmc377R12Carriers(board, withdrawn); math.Abs(sum-2.903) > 0.0015 {
			t.Errorf("Σ carriers %.3f, want 2.903 (the ◇ takes the 1.903 once; the satellite resolves to it)", sum)
		}
	})
	t.Run("P06: a share another published seat still holds is counted once (the returned share is clamped)", func(t *testing.T) {
		items := []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20), hmc377R11ChainRun(0.853, hmc377R11W2(), 21, 25),
			hmc377R11Remainder(5, 1.903, 6.903, 10.5, 70, 80)}
		resolveRankShareClaims(items, nil)
		board, _, withdrawn := hmc377R12Publish(RootCauseRankResult{Items: items})
		if _, sum := hmc377R12Carriers(board, withdrawn); math.Abs(sum-6.903) > 0.0015 {
			t.Errorf("Σ carriers %.3f, want 6.903 (the chain seat keeps W2; the ◇ takes W1 only)", sum)
		}
		if got := board[3].ShareReturnedMs; math.Abs(got-1.05) > 0.0005 {
			t.Errorf("◇ took back %.3f, want 1.050", got)
		}
	})
	disclosure := func() []RootCauseRankItem {
		agg := levelMergeTestAggregateSeat(200, 15, TimeWindow{StartTs: 10.000, EndTs: 10.010}, TimeWindow{StartTs: 10.020, EndTs: 10.030})
		agg.PeriodicSource, agg.DetectedPeriodMs, agg.LatenessMs, agg.EffectiveImpactMs = true, 16.6, 4, 19
		inv := levelMergeTestInversionSeat(200, 6, 2, TimeWindow{StartTs: 10.005, EndTs: 10.025})
		items := splitAggregateGatedRunnableShare(ChainResult{Target: ThreadRef{PID: 100}}, []RootCauseRankItem{agg, inv})
		resolveRankShareClaims(items, nil)
		return items
	}
	t.Run("P51: the fail-open overlap disclosure moves no value at the gate", func(t *testing.T) {
		items := disclosure()
		if items[0].GatedShareOverlapDisclosureMs <= 0 {
			t.Fatalf("precondition: the periodic aggregate takes the disclosure arm")
		}
		board, _ := hmc377R12Withdraw(append([]RootCauseRankItem(nil), items...), nil, []bool{false, true}, nil)
		d := board[0]
		if d.EffectiveImpactMs != 19 || d.RunnableMs != 15 || d.ImpactMs != 15 || d.ShareReturnedMs != 0 {
			t.Errorf("disclosure seat eff=%.3f run=%.3f impact=%.3f returned=%.3f, want 19/15/15/0 (value-free claim)", d.EffectiveImpactMs, d.RunnableMs, d.ImpactMs, d.ShareReturnedMs)
		}
	})
	t.Run("P13: the disclosure seat says its claim seat is context only", func(t *testing.T) {
		board, _ := hmc377R12Withdraw(disclosure(), nil, []bool{false, true}, nil)
		if !board[0].GatedShareClaimWithdrawn || !strings.Contains(board[0].Summary, levelMergeSummaryClaimSeatContext) {
			t.Errorf("withdrawn=%t %q", board[0].GatedShareClaimWithdrawn, board[0].Summary)
		}
	})
}

func TestHMC377R12CarvedClaimHeldByASuccessorNamesNoHolder(t *testing.T) {
	// P20: a restriction seat's carved claim on instants a withdrawn
	// inversion seat held, which the LEVELMERGE-1 constituent (successor tier
	// 1) took first: the claim is held by that successor — the successor
	// form, never the held form ("… already publishes").
	agg := hmc377R11Aggregate(true)
	agg.OccurrenceWindows = []WakeupCausalOccurrence{{Window: TimeWindow{StartTs: 10.2, EndTs: 10.20105}}, {Window: TimeWindow{StartTs: 10.4, EndTs: 10.401097}}}
	agg.ImpactMs, agg.CumulativeImpactMs, agg.EffectiveImpactMs, agg.ProjectedImpactMs, agg.RunnableMs = 2.147, 2.147, 2.147, 2.147, 2.147
	agg.rspaHeld = nil
	rspaHold(&agg, RootCauseShareHolderChainSeat, false, rspaFoldUnion(hmc377R11W1(), []foldInterval{{start: 10.4, end: 10.401097}}), 2.147)
	items := []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.05, hmc377R11W1(), 10, 20), agg,
		hmc377R12CPU([]foldInterval{{start: 10.2, end: 10.2005}, {start: 10.6, end: 10.601}}, 90)}
	carveCPUConstraintReadyInstants(items)
	items = splitAggregateGatedRunnableShare(ChainResult{Target: hmc377R11Target}, items)
	rank := RootCauseRankResult{Window: TimeWindow{StartTs: 10, EndTs: 11}, Items: items}
	reconcileExactCrossTypeRankSeats(&rank)
	resolveRankShareClaims(rank.Items, rank.AbsorbedItems)
	var cpu *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "cpu_affinity_or_cpuset" {
			cpu = &rank.Items[i]
		}
	}
	if cpu == nil || !strings.Contains(cpu.Summary, "already publishes") {
		t.Fatalf("precondition: the restriction seat's carved claim is held by the inversion seat: %+v", cpu)
	}
	board, _, _ := hmc377R12Publish(rank)
	for _, it := range board {
		if it.Type != "cpu_affinity_or_cpuset" {
			continue
		}
		want := "; 0.500ms of it lies inside instants of this thread's chain seat, which is published only as context, tier context_only; another row of this thread on the published board counts them (not here)"
		if !strings.Contains(it.Summary, want) {
			t.Errorf("summary %q\nwant %q", it.Summary, want)
		}
	}
}

func TestHMC377R12PublicationReRunIsStable(t *testing.T) {
	t.Run("the same withdrawal applied again leaves the published rows as they are", func(t *testing.T) {
		for _, name := range []string{"D3cpuEmpty", "D2cpuInRem", "UR5inv"} {
			board, detail, withdrawn := hmc377R12Publish(hmc377R12Build(name).rank)
			again, againDetail := hmc377R12Withdraw(append([]RootCauseRankItem(nil), board...), append([]RootCauseRankItem(nil), detail...),
				append([]bool(nil), withdrawn...), nil)
			if len(again) != len(board) || len(againDetail) != len(detail) {
				t.Fatalf("%s: rows moved on a second run: board %d→%d detail %d→%d", name, len(board), len(again), len(detail), len(againDetail))
			}
			for i := range board {
				if again[i].Summary != board[i].Summary || again[i].EffectiveImpactMs != board[i].EffectiveImpactMs || again[i].ShareReturnedMs != board[i].ShareReturnedMs {
					t.Errorf("%s row %d changed on a second run: %.3f %q / %.3f %q", name, i, board[i].EffectiveImpactMs, board[i].Summary, again[i].EffectiveImpactMs, again[i].Summary)
				}
			}
		}
	})
	t.Run("a detail row that rides as detail again keeps its position", func(t *testing.T) {
		u := hmc377R12Build("UR5inv")
		other := RootCauseRankItem{Type: "io_burst_episode", Source: "window_stats.io_burst_episodes", Thread: hmc377R11Other, Tier: RootCauseTierAbsorbed,
			AbsorbedByRankFamily: true, AbsorbedIntoFamily: "rank_pair:other", ImpactMs: 1, LineStart: 900, LineEnd: 901}
		u.rank.AbsorbedItems = append(u.rank.AbsorbedItems, other)
		_, detail, _ := hmc377R12Publish(u.rank)
		if len(detail) != 2 || detail[0].Type != "priority_inversion_runnable_wait" || detail[1].Type != "io_burst_episode" {
			t.Errorf("detail order changed: %v", []string{detail[0].Type, detail[1].Type})
		}
	})
}

// The several-seat twin's whole-account detail (R-377-11 ② severalOwners arm):
// the whole-account inversion row rides as the ◇ twin's lossless detail. When
// the gate withdraws one of the chain seats holding its anchored share, the
// twin (successor tier 3) takes that seat's segments, the detail owns no
// instant and its absorber is published — it keeps riding as detail; its
// anchored share is now held in parts (the other chain seat, the twin), so
// that phrase takes the see-detail form.
func TestHMC377R12SeveralSeatTwinDetailStaysWhileItsAbsorberCarriesIt(t *testing.T) {
	inv := hmc377R11Inv(1.05, hmc377R11W1(), 10, 20)
	run := hmc377R11ChainRun(0.853, hmc377R11W2(), 21, 25)
	twin := hmc377R11Remainder(1.0, 1.903, 2.903, 10.5, 70, 80)
	det := hmc377R12InversionDetail(&twin, 2.903, []foldInterval{{start: 10.5, end: 10.501}})
	rank := RootCauseRankResult{Window: TimeWindow{StartTs: 10, EndTs: 11}, Items: []RootCauseRankItem{hmc377R11OtherSeat(), inv, run, twin}, AbsorbedItems: []RootCauseRankItem{det}}
	resolveRankShareClaims(rank.Items, rank.AbsorbedItems)
	if !rank.AbsorbedItems[0].ShareClaims[0].Resolved {
		t.Fatalf("precondition: the detail's anchored share is held by the two chain seats")
	}
	board, detail, withdrawn := hmc377R12Publish(rank)
	if len(detail) != 1 || !detail[0].AbsorbedByRankFamily || detail[0].AbsorbedIntoFamily != board[3].RankFamilyKey {
		t.Fatalf("the detail rides under its published twin: detail %d into=%q twin key=%q", len(detail), detail[0].AbsorbedIntoFamily, board[3].RankFamilyKey)
	}
	if _, sum := hmc377R12Carriers(board, withdrawn); math.Abs(sum-2.903) > 0.0015 {
		t.Errorf("Σ carriers %.3f, want 2.903 (chain seat 0.853 + twin 1.000 + the withdrawn seat's 1.050)", sum)
	}
	if !strings.Contains(detail[0].Summary, "of which "+hmc377R12SharePartsSeeDetail()) || !strings.Contains(detail[0].Summary, rspaInversionDetailTailContext) {
		t.Errorf("detail words: %q", detail[0].Summary)
	}
}
