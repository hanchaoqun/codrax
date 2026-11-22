package tracequery

// HMC377 round 11 — batch-owner ruling R-377-11 ① (2026-11-21): one carrier
// per instant across every share relation.
//
// Review 10 (lens holders) reproduced, in universes built with the engine's own
// typed internals (~/hmc-work/hmc377/fix10/review-holders/univ/
// zz_rv10r_universe_export.go, u_head.out, oracle10_univ.log, summary_table.txt):
//   - R10H-0: the resolver resolved a claim whole or not at all, so with a
//     multi-holder represented claim one withdrawn or cap-cut holder
//     unresolved the whole claim and the remaining holder's part had two ◎
//     carriers (UR1_w Σ2.756 / raw 1.903; UR1cut_c 2.953; UR2cut_c 3.953,
//     UR2cutI_c 3.756, UR2w_w 3.756 / raw 2.903);
//   - R10H-1/2: the LEVELMERGE-1 constituent/residual relation was decided a
//     second time by line span (levelMergeWithdrawClaimSeats) beside the
//     resolver (UR3c_w Σ4.500 / raw 3.000), and the residual kept its
//     pre-split chain-seat hold, so the satellite's claim resolved to a row
//     that does not count that time (UR3b_w Σ1.097 / 3.000; UR3d_c holder
//     word 「锚定份由链席[E3,E2]代表」);
//   - R10H-3: a ◇ remainder whose anchored holder the gate withdrew kept
//     only its remainder (UR4rem_w Σ5.000 / 6.903, UR4mix_w 5.853 / 6.903,
//     UR5both_w and UR5inv_w 1.000 / 2.903);
//   - R10H-4: the residual's withdrawn form asserted the constituent counts
//     the share while the side cap had cut the constituent (UR6_w Σ1.097 /
//     3.000).
// The universes are rebuilt here with the same internals; the LEVELMERGE-1
// pairs are minted by the engine split itself. The ◎ carrier model below is
// the published board's own: a represented satellite is off ◎ while its
// claims resolve whole, a constituent while its claim seat counts its share,
// a gate-withdrawn row always; every other row of the thread carries its
// published runnable account (a priority-inversion seat its gated runnable).
// Round-11 fields are read through rank_share_claims_segments_r11_shim_hmc377_test.go
// (the red run on b2325eba6 swaps in a shim with zero accessors; the
// sentences are literal bytes), so this same file runs on both.

import (
	"math"
	"strings"
	"testing"
)

var (
	hmc377R11Target = ThreadRef{Comm: "app", PID: 100}
	hmc377R11Dep    = ThreadRef{Comm: "binder:458_5", PID: 6326}
	hmc377R11Other  = ThreadRef{Comm: "other", PID: 6400}
)

func hmc377R11W1() []foldInterval { return []foldInterval{{start: 10.2, end: 10.20105}} }
func hmc377R11W2() []foldInterval { return []foldInterval{{start: 10.20105, end: 10.201903}} }
func hmc377R11W() []foldInterval  { return []foldInterval{{start: 10.2, end: 10.201903}} }
func hmc377R11U() []foldInterval  { return []foldInterval{{start: 10.3, end: 10.301}} }

func hmc377R11OtherSeat() RootCauseRankItem {
	it := RootCauseRankItem{Rank: 1, Tier: "primary", Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: hmc377R11Other,
		ImpactMs: 30, CumulativeImpactMs: 30, EffectiveImpactMs: 30, RunnableMs: 30, DominantState: "runnable",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.9, StartTs: 10.1, EndTs: 10.13, LineStart: 5, LineEnd: 6}
	rspaHold(&it, RootCauseShareHolderChainSeat, false, []foldInterval{{start: 10.1, end: 10.13}}, 30)
	return it
}

func hmc377R11Inv(ms float64, windows []foldInterval, l0, l1 int) RootCauseRankItem {
	it := RootCauseRankItem{Rank: 2, Tier: "secondary", Type: "priority_inversion_candidate", Source: "wakeup_chain.causal_impacts", Thread: hmc377R11Dep,
		ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, RunnableMs: ms, GatedRunnableMs: ms, DominantState: "runnable",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8,
		StartTs: windows[0].start, EndTs: windows[len(windows)-1].end, LineStart: l0, LineEnd: l1}
	rspaHold(&it, RootCauseShareHolderChainSeat, false, windows, ms)
	return it
}

func hmc377R11ChainRun(ms float64, windows []foldInterval, l0, l1 int) RootCauseRankItem {
	it := RootCauseRankItem{Rank: 3, Tier: "secondary", Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: hmc377R11Dep,
		ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, RunnableMs: ms, DominantState: "runnable",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8,
		StartTs: windows[0].start, EndTs: windows[len(windows)-1].end, LineStart: l0, LineEnd: l1}
	rspaHold(&it, RootCauseShareHolderChainSeat, false, windows, ms)
	return it
}

func hmc377R11UnwalkedSeat() RootCauseRankItem {
	unw := RootCauseRankItem{Rank: 3, Tier: "secondary", Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: hmc377R11Dep,
		ImpactMs: 1.0, CumulativeImpactMs: 1.0, EffectiveImpactMs: 1.0, RunnableMs: 1.0, DominantState: "runnable",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.3, EndTs: 10.301, LineStart: 41, LineEnd: 45,
		ChainAnchorUnwalkedSeat: true}
	rspaHold(&unw, RootCauseShareHolderUnwalkedSeat, false, hmc377R11U(), 1.0)
	return unw
}

func hmc377R11Satellite(ms, start, end float64, claims ...RootCauseShareClaim) RootCauseRankItem {
	sat := RootCauseRankItem{Rank: 4, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R11Dep,
		ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, ProjectedImpactMs: ms, RunnableMs: ms, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.78, StartTs: start, EndTs: end, LineStart: 30, LineEnd: 40,
		ChainAnchorRepresentedByChainSeat: true, Summary: "binder:458_5-6326 runnable satellite"}
	rspaAppendSentence(&sat, rspaRepresentedDemotionSummary(ms, ms, claims...))
	return sat
}

func hmc377R11Satellite1() RootCauseRankItem {
	return hmc377R11Satellite(1.903, 10.2, 10.201903,
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, hmc377R11W()))
}

func hmc377R11SatellitePart() RootCauseRankItem {
	return hmc377R11Satellite(1.5, 10.2, 10.2015,
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.5, []foldInterval{{start: 10.2, end: 10.2015}}))
}

func hmc377R11Satellite3() RootCauseRankItem {
	sat := RootCauseRankItem{Rank: 4, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R11Dep,
		ImpactMs: 2.903, CumulativeImpactMs: 2.903, EffectiveImpactMs: 2.903, ProjectedImpactMs: 2.903, RunnableMs: 2.903, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.78, StartTs: 10.2, EndTs: 10.301, LineStart: 30, LineEnd: 40,
		ChainAnchorRepresentedByChainSeat: true, Summary: "binder:458_5-6326 runnable satellite"}
	claims := rspaRowClaims{
		anchored: rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, hmc377R11W()),
		unwalked: rspaClaim(RootCauseShareClaimUnwalked, RootCauseShareHolderUnwalkedSeat, hmc377R11Dep.PID, false, 1.0, hmc377R11U()),
	}
	rspaAppendSentence(&sat, rspaRepresentedDemotionSummaryThreeWay(1.903, 1.0, 2.903,
		"the analysed thread's wait(s) that this thread's own wakeup ended and that the dependency chain did not walk", rspaRowScope{owned: 1.0}, claims))
	return sat
}

func hmc377R11Remainder(remainder, anchored, full, start float64, l0, l1 int) RootCauseRankItem {
	rem := RootCauseRankItem{Rank: 4, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R11Dep,
		ImpactMs: remainder, CumulativeImpactMs: remainder, EffectiveImpactMs: remainder, RunnableMs: remainder, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.76, StartTs: start, EndTs: start + remainder/1000, LineStart: l0, LineEnd: l1,
		ChainAnchoredMs: anchored, ChainAnchorFullMs: full, ChainAnchorRemainderSeat: true}
	rspaSetSentence(&rem, rspaRemainderSentence(hmc377R11Dep, "runnable (scheduling-pressure candidate)", remainder, anchored, full,
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, anchored, hmc377R11W())))
	rspaHold(&rem, RootCauseShareHolderRemainderSeat, false, []foldInterval{{start: start, end: start + remainder/1000}}, remainder)
	return rem
}

// hmc377R11Aggregate is 6326's (pid, runnable) chain aggregate: 3.000 ms over
// the occurrence windows W and [10.400,10.401097); holdW stamps the build
// pass's chain-seat hold (its segments inside the walked windows, at its
// pre-split account) as reanchorOnChainStateSeats does before the split.
func hmc377R11Aggregate(holdW bool) RootCauseRankItem {
	agg := RootCauseRankItem{Rank: 3, Tier: "secondary", Type: "runnable_wait", Source: "wakeup_chain.aggregated_impacts", Thread: hmc377R11Dep,
		ImpactMs: 3.0, CumulativeImpactMs: 3.0, EffectiveImpactMs: 3.0, ProjectedImpactMs: 3.0, RunnableMs: 3.0, DominantState: "runnable", ChainDepth: 2,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.2, EndTs: 10.401097, LineStart: 50, LineEnd: 60,
		OccurrenceWindows: []WakeupCausalOccurrence{{Window: TimeWindow{StartTs: 10.2, EndTs: 10.201903}}, {Window: TimeWindow{StartTs: 10.4, EndTs: 10.401097}}}}
	if holdW {
		rspaHold(&agg, RootCauseShareHolderChainSeat, false, hmc377R11W(), 3.0)
	}
	return agg
}

func hmc377R11Split(items []RootCauseRankItem) []RootCauseRankItem {
	return splitAggregateGatedRunnableShare(ChainResult{Target: hmc377R11Target}, items)
}

func hmc377R11WithoutConstituent(items []RootCauseRankItem) []RootCauseRankItem {
	var out []RootCauseRankItem
	for _, it := range items {
		if !it.GatedShareConstituentSeat {
			out = append(out, it)
		}
	}
	return out
}

// hmc377R11Universe is one review-10 universe: its board (engine-resolved),
// the indexes of the rows whose builder the gate withdraws, and the thread's
// raw ready time (the universe's own definition).
type hmc377R11Universe struct {
	board, detail   []RootCauseRankItem
	withdrawn       []bool
	withdrawnDetail []bool
	raw             float64
}

func hmc377R11Build(name string) hmc377R11Universe {
	var u hmc377R11Universe
	withdrawInv := func(items []RootCauseRankItem) []bool {
		out := make([]bool, len(items))
		for i, it := range items {
			out[i] = it.Type == "priority_inversion_candidate"
		}
		return out
	}
	switch name {
	case "UR1":
		u.board, u.raw = []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.05, hmc377R11W1(), 10, 20), hmc377R11ChainRun(0.853, hmc377R11W2(), 21, 25), hmc377R11Satellite1()}, 1.903
	case "UR1cut":
		u.board, u.raw = []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.05, hmc377R11W1(), 10, 20), hmc377R11Satellite1()}, 1.903
	case "UR2cut":
		u.board, u.raw = []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.05, hmc377R11W1(), 10, 20), hmc377R11UnwalkedSeat(), hmc377R11Satellite3()}, 2.903
	case "UR2cutI":
		u.board, u.raw = []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11ChainRun(0.853, hmc377R11W2(), 21, 25), hmc377R11UnwalkedSeat(), hmc377R11Satellite3()}, 2.903
	case "UR2w":
		u.board, u.raw = []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.05, hmc377R11W1(), 10, 20), hmc377R11ChainRun(0.853, hmc377R11W2(), 21, 25), hmc377R11UnwalkedSeat(), hmc377R11Satellite3()}, 2.903
	case "UR3a", "UR3b":
		u.board, u.raw = hmc377R11Split([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20), hmc377R11Aggregate(name == "UR3b"), hmc377R11Satellite1()}), 3.0
	case "UR3c", "UR3d":
		u.board, u.raw = hmc377R11Split([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20), hmc377R11Aggregate(name == "UR3d"), hmc377R11SatellitePart()}), 3.0
	case "UR6":
		u.board, u.raw = hmc377R11WithoutConstituent(hmc377R11Split([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20), hmc377R11Aggregate(false)})), 3.0
	case "UR4":
		u.board, u.raw = []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.05, hmc377R11W1(), 10, 20), hmc377R11Inv(0.853, hmc377R11W2(), 21, 25), hmc377R11Satellite1()}, 1.903
	case "UR4rem":
		u.board, u.raw = []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.05, hmc377R11W1(), 10, 20), hmc377R11Inv(0.853, hmc377R11W2(), 21, 25), hmc377R11Remainder(5, 1.903, 6.903, 10.5, 60, 90)}, 6.903
	case "UR4mix":
		u.board, u.raw = []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.05, hmc377R11W1(), 10, 20), hmc377R11ChainRun(0.853, hmc377R11W2(), 21, 25), hmc377R11Remainder(5, 1.903, 6.903, 10.5, 60, 90)}, 6.903
	case "UR5both", "UR5inv":
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		rem := hmc377R11Remainder(1.0, 1.903, 2.903, 10.5, 70, 80)
		det := RootCauseRankItem{Type: "priority_inversion_runnable_wait", Source: "window_stats", Thread: hmc377R11Dep,
			ImpactMs: 2.903, CumulativeImpactMs: 2.903, EffectiveImpactMs: 2.903, RunnableMs: 2.903, DominantState: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.7, StartTs: 10.2, EndTs: 10.501, LineStart: 30, LineEnd: 40,
			Summary: "binder:458_5-6326 same-CPU lower-priority overlap over the ready account"}
		rspaAppendSentence(&det, rspaInversionRepresentedSummary(1.903, 2.903,
			rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, hmc377R11W()),
			rspaClaim(RootCauseShareClaimRemainder, RootCauseShareHolderRemainderSeat, hmc377R11Dep.PID, false, 1.0, []foldInterval{{start: 10.5, end: 10.501}})))
		key := crossTypeRankSeatReconKey(inv, det, TimeWindow{StartTs: 10, EndTs: 11})
		det.Rank, det.BackgroundRank, det.Tier = 0, 0, RootCauseTierAbsorbed
		det.AbsorbedByRankFamily, det.AbsorbedIntoFamily = true, key
		inv.RankFamilyKey, inv.AbsorbedRankRows = key, 1
		u.board, u.detail, u.raw = []RootCauseRankItem{hmc377R11OtherSeat(), inv, rem}, []RootCauseRankItem{det}, 2.903
		u.withdrawnDetail = []bool{name == "UR5both"}
	}
	resolveRankShareClaims(u.board, u.detail)
	u.withdrawn = withdrawInv(u.board)
	return u
}

// hmc377R11Publish applies the gate's decision to a published copy (withdraw
// false: the engine cap alone shaped the board).
func hmc377R11Publish(u hmc377R11Universe, withdraw bool) ([]RootCauseRankItem, []RootCauseRankItem) {
	board := append([]RootCauseRankItem(nil), u.board...)
	detail := append([]RootCauseRankItem(nil), u.detail...)
	if withdraw {
		// HMC377 round 12 (R-377-12 ①): the publication returns the rows —
		// a lossless-detail row can return to the board.
		board, detail = hmc377R12Withdraw(board, detail, u.withdrawn, u.withdrawnDetail)
	}
	return board, detail
}

// hmc377R11Carriers sums the ◎ carriers of the dependency thread on a
// published board (see the file header).
func hmc377R11Carriers(board []RootCauseRankItem, withdrawn []bool, withdraw bool) float64 {
	sum := 0.0
	for i, it := range board {
		if it.Thread.PID != hmc377R11Dep.PID || (withdraw && withdrawn[i]) {
			continue
		}
		switch {
		case it.ChainAnchorRepresentedByChainSeat:
		case it.GatedShareConstituentSeat && !it.GatedShareClaimWithdrawn:
		case it.Type == "priority_inversion_candidate":
			sum += it.GatedRunnableMs
		default:
			sum += it.RunnableMs
		}
	}
	return sum
}

func TestHMC377R11EveryUniverseHasOneCarrierPerInstant(t *testing.T) {
	cases := []struct {
		name     string
		withdraw bool
	}{
		{"UR1", false}, {"UR1", true}, {"UR1cut", false}, {"UR2cut", false}, {"UR2cut", true}, {"UR2cutI", false},
		{"UR2w", false}, {"UR2w", true}, {"UR3a", false}, {"UR3a", true}, {"UR3b", false}, {"UR3b", true},
		{"UR3c", false}, {"UR3c", true}, {"UR3d", false}, {"UR3d", true}, {"UR6", false}, {"UR6", true},
		{"UR4", false}, {"UR4", true}, {"UR4rem", false}, {"UR4rem", true}, {"UR4mix", false}, {"UR4mix", true},
		{"UR5both", false}, {"UR5both", true}, {"UR5inv", false}, {"UR5inv", true},
	}
	for _, tc := range cases {
		t.Run(map[bool]string{false: tc.name + "_c", true: tc.name + "_w"}[tc.withdraw], func(t *testing.T) {
			u := hmc377R11Build(tc.name)
			board, _ := hmc377R11Publish(u, tc.withdraw)
			if got := hmc377R11Carriers(board, u.withdrawn, tc.withdraw); math.Abs(got-u.raw) > 0.0015 {
				for i, it := range board {
					t.Logf("row %d type=%s run=%.3f gated=%.3f repr=%t constituent=%t claimWithdrawn=%t held=%.3f returned=%.3f claims=%+v", i, it.Type, it.RunnableMs, it.GatedRunnableMs,
						it.ChainAnchorRepresentedByChainSeat, it.GatedShareConstituentSeat, it.GatedShareClaimWithdrawn, it.ChainAnchorRepresentedHeldMs, hmc377R11Returned(it), it.ShareClaims)
				}
				t.Errorf("Σ carriers %.3f != raw %.3f (one carrier per instant)", got, u.raw)
			}
		})
	}
}

// R10H-0: per held segment — the remaining holder keeps its part, the rest
// returns to the claimant (one carrier per instant), at the gate and at the
// cap alike.
func TestHMC377R11ClaimResolvesPerHeldSegment(t *testing.T) {
	t.Run("UR1_w: withdrawn holder of one segment", func(t *testing.T) {
		u := hmc377R11Build("UR1")
		board, _ := hmc377R11Publish(u, true)
		sat := board[3]
		c := sat.ShareClaims[0]
		if c.Resolved || math.Abs(hmc377R11ResolvedMs(c)-0.853) > 0.0005 || math.Abs(hmc377R11ReturnedMs(c)-1.05) > 0.0005 || len(c.HolderKeys) != 1 || c.HolderKeys[0] != board[2].ShareHolderKey {
			t.Fatalf("the claim resolves its W2 segment to the chain seat and takes W1 back: %+v (run key %q)", c, board[2].ShareHolderKey)
		}
		if sat.ChainAnchorRepresentedByChainSeat || math.Abs(sat.ChainAnchorRepresentedHeldMs-0.853) > 0.0005 || math.Abs(sat.EffectiveImpactMs-1.05) > 0.0005 {
			t.Errorf("the satellite counts only the rest: marker=%t held=%.3f eff=%.3f", sat.ChainAnchorRepresentedByChainSeat, sat.ChainAnchorRepresentedHeldMs, sat.EffectiveImpactMs)
		}
		// EVOLUTION RECORD (HMC377 round 12, R-377-12 ②): the share has two
		// dispositions (0.853 held by the chain seat on the board, 1.050
		// passed back here), so the sentence takes the see-detail form.
		//   old: "0.853ms of it is held by a seat of this thread on the
		//        published board, which counts it, and the seat holding the
		//        other 1.050ms is published only as context, tier context_only"
		//   new: "its parts are counted by different rows; see the detail
		//        index for each part — … count only the 1.050ms no other row on
		//        the published board holds" (values unchanged)
		if want := "and its parts are counted by different rows; see the detail index for each part — this diagnostic projection rides the adjacent lane and its published values count only the 1.050ms no other row on the published board holds"; !strings.Contains(sat.Summary, want) {
			t.Errorf("summary %q\nwant %q", sat.Summary, want)
		}
	})
	t.Run("UR1cut_c: the cap cut one holder", func(t *testing.T) {
		u := hmc377R11Build("UR1cut")
		sat := u.board[2]
		c := sat.ShareClaims[0]
		if c.Resolved || math.Abs(hmc377R11ResolvedMs(c)-1.05) > 0.0005 || hmc377R11ReturnedMs(c) != 0 {
			t.Fatalf("engine claim: %+v", c)
		}
		if math.Abs(sat.EffectiveImpactMs-0.853) > 0.0005 || math.Abs(sat.ChainAnchorRepresentedHeldMs-1.05) > 0.0005 {
			t.Errorf("the satellite counts the 0.853 its cut holder held: eff=%.3f held=%.3f", sat.EffectiveImpactMs, sat.ChainAnchorRepresentedHeldMs)
		}
		if want := "the seat holding the other 0.853ms is not on the published board (see the compaction disclosure)"; !strings.Contains(sat.Summary, want) {
			t.Errorf("the cap-cut part keeps the compaction wording: %q", sat.Summary)
		}
	})
	t.Run("UR2cut_c: three-way, cap cut the plain seat", func(t *testing.T) {
		u := hmc377R11Build("UR2cut")
		sat := u.board[3]
		if math.Abs(sat.ChainAnchorRepresentedHeldMs-2.05) > 0.0005 || math.Abs(sat.EffectiveImpactMs-0.853) > 0.0005 {
			t.Errorf("held=%.3f eff=%.3f (want 1.050 + 1.000 held, 0.853 counted here)", sat.ChainAnchorRepresentedHeldMs, sat.EffectiveImpactMs)
		}
	})
	t.Run("idempotent: a second publication of the same board is a no-op", func(t *testing.T) {
		u := hmc377R11Build("UR2w")
		board, _ := hmc377R11Publish(u, true)
		again := append([]RootCauseRankItem(nil), board...)
		WithdrawRankShareHolders(again, nil, u.withdrawn, nil)
		for i := range board {
			if again[i].Summary != board[i].Summary || again[i].EffectiveImpactMs != board[i].EffectiveImpactMs || hmc377R11Returned(again[i]) != hmc377R11Returned(board[i]) {
				t.Errorf("row %d changed on a second publication: %q %.3f / %q %.3f", i, board[i].Summary, board[i].EffectiveImpactMs, again[i].Summary, again[i].EffectiveImpactMs)
			}
		}
		if u.board[4].EffectiveImpactMs != 2.903 || !u.board[4].ChainAnchorRepresentedByChainSeat {
			t.Errorf("publication mutated the engine row")
		}
	})
}

// R10H-1/2/4: the LEVELMERGE-1 relation is a claim of the one resolver —
// the constituent is the withdrawn claim seat's first successor, the split
// re-stamps the residual's holds, and the residual takes the carved share
// back only when neither its claim seat nor its constituent is published.
func TestHMC377R11LevelMergeIsAClaimOfTheOneResolver(t *testing.T) {
	t.Run("split mints the claims and re-stamps the residual's hold", func(t *testing.T) {
		u := hmc377R11Build("UR3d")
		inv, b, sat, a := u.board[1], u.board[2], u.board[3], u.board[4]
		if !a.GatedShareConstituentSeat || len(a.ShareClaims) != 1 || len(b.ShareClaims) != 1 || !hmc377R11IsGatedShareClaim(a.ShareClaims[0]) {
			t.Fatalf("the split pair must carry its gated-share claim: a=%+v b=%+v", a.ShareClaims, b.ShareClaims)
		}
		if !hmc377R11HoldsGatedShare(inv) {
			t.Fatalf("the claiming inversion seat holds its branch windows in the gated-share role")
		}
		for _, h := range b.rspaHeld {
			if h.role == RootCauseShareHolderChainSeat && foldIntervalIntersectionMs(h.windows, hmc377R11W()) > 0 {
				t.Fatalf("the residual still holds the carved instants: %+v", h)
			}
		}
		// UR3d_c: the satellite's holder is the inversion seat alone (round
		// 10 named the residual too: 「锚定份由链席[E3,E2]代表」).
		if c := sat.ShareClaims[0]; !c.Resolved || len(c.HolderKeys) != 1 || c.HolderKeys[0] != inv.ShareHolderKey {
			t.Errorf("the satellite's claim resolves to the inversion seat alone: %+v (inv %q, b %q)", c, inv.ShareHolderKey, b.ShareHolderKey)
		}
	})
	t.Run("UR3b_w: the constituent is the successor", func(t *testing.T) {
		u := hmc377R11Build("UR3b")
		board, _ := hmc377R11Publish(u, true)
		b, sat, a := board[2], board[3], board[4]
		if !a.GatedShareClaimWithdrawn || math.Abs(hmc377R11Returned(a)-1.903) > 0.0005 || math.Abs(a.EffectiveImpactMs-1.903) > 0.0005 {
			t.Errorf("the constituent carries its share: withdrawn=%t returned=%.3f eff=%.3f", a.GatedShareClaimWithdrawn, hmc377R11Returned(a), a.EffectiveImpactMs)
		}
		for name, c := range map[string]RootCauseShareClaim{"residual": b.ShareClaims[0], "satellite": sat.ShareClaims[0]} {
			if !c.Resolved || !hmc377R11Successor(c) || len(c.HolderKeys) != 1 || c.HolderKeys[0] != a.ShareHolderKey {
				t.Errorf("%s: the claim resolves to the constituent successor: %+v (a %q)", name, c, a.ShareHolderKey)
			}
		}
		if !sat.ChainAnchorRepresentedByChainSeat || !b.GatedShareClaimWithdrawn || math.Abs(b.EffectiveImpactMs-1.097) > 0.0005 {
			t.Errorf("satellite represented=%t, residual withdrawn=%t eff=%.3f", sat.ChainAnchorRepresentedByChainSeat, b.GatedShareClaimWithdrawn, b.EffectiveImpactMs)
		}
		if strings.Contains(sat.Summary, "represented by the chain seat") || !strings.Contains(sat.Summary, "anchored share counted by another row of this thread") {
			t.Errorf("the satellite names the successor, never a chain seat: %q", sat.Summary)
		}
	})
	t.Run("UR6_w: neither claim seat nor constituent published — the residual takes it back", func(t *testing.T) {
		u := hmc377R11Build("UR6")
		board, _ := hmc377R11Publish(u, true)
		b := board[2]
		if !b.GatedShareClaimWithdrawn || math.Abs(hmc377R11Returned(b)-1.903) > 0.0005 || math.Abs(b.EffectiveImpactMs-3.0) > 0.0005 || math.Abs(b.RunnableMs-3.0) > 0.0005 {
			t.Errorf("the residual takes back the carved share: withdrawn=%t returned=%.3f eff=%.3f run=%.3f", b.GatedShareClaimWithdrawn, hmc377R11Returned(b), b.EffectiveImpactMs, b.RunnableMs)
		}
		if b.CumulativeImpactMs != 3.0 {
			t.Errorf("the residual's cumulative channel already is its full account: %.3f", b.CumulativeImpactMs)
		}
		if !strings.Contains(b.Summary, "and this seat counts it with its own value (the claiming inversion seat is published only as context, tier context_only)") ||
			strings.Contains(b.Summary, "split constituent row") {
			t.Errorf("the residual's sentence: %q", b.Summary)
		}
	})
}

// R10H-3: a ◇ remainder (or R-377-4 inversion detail) whose anchored holder
// the gate withdraws carries its whole account; the engine cap-cut remainder
// keeps its compaction wording (N17).
func TestHMC377R11RemainderCarriesTheWithdrawnHoldersShare(t *testing.T) {
	t.Run("UR4rem_w", func(t *testing.T) {
		u := hmc377R11Build("UR4rem")
		board, _ := hmc377R11Publish(u, true)
		rem := board[3]
		if math.Abs(rem.EffectiveImpactMs-6.903) > 0.0005 || math.Abs(rem.RunnableMs-6.903) > 0.0005 || math.Abs(hmc377R11Returned(rem)-1.903) > 0.0005 {
			t.Errorf("remainder eff=%.3f run=%.3f returned=%.3f, want 6.903 whole", rem.EffectiveImpactMs, rem.RunnableMs, hmc377R11Returned(rem))
		}
		if !strings.Contains(rem.Summary, "(the owning row is published only as context, tier context_only, so this row counts that share with its own value)") {
			t.Errorf("remainder summary %q", rem.Summary)
		}
	})
	t.Run("UR4mix_w", func(t *testing.T) {
		u := hmc377R11Build("UR4mix")
		board, _ := hmc377R11Publish(u, true)
		rem := board[3]
		if math.Abs(rem.EffectiveImpactMs-6.05) > 0.0005 || math.Abs(hmc377R11Returned(rem)-1.05) > 0.0005 {
			t.Errorf("remainder eff=%.3f returned=%.3f, want 5.000 + the withdrawn holder's 1.050", rem.EffectiveImpactMs, hmc377R11Returned(rem))
		}
		// EVOLUTION RECORD (HMC377 round 12, R-377-12 ②): the computed mixed
		// owner form (rspaOwnerMixedForm) is removed — a share with two
		// dispositions takes the see-detail form; the value is unchanged.
		//   old: "(0.853ms of it owned by the chain seat; 1.050ms whose owning
		//        row is published only as context, tier context_only, so this
		//        row counts them with its own value)"
		//   new: "(its parts are counted by different rows; see the detail
		//        index for each part)"
		if want := "(its parts are counted by different rows; see the detail index for each part)"; !strings.Contains(rem.Summary, want) {
			t.Errorf("remainder summary %q\nwant %q", rem.Summary, want)
		}
	})
	t.Run("UR5inv_w: the ◇ row precedes the inversion detail", func(t *testing.T) {
		u := hmc377R11Build("UR5inv")
		board, detail := hmc377R11Publish(u, true)
		rem, det := board[2], detail[0]
		if math.Abs(rem.EffectiveImpactMs-2.903) > 0.0005 {
			t.Errorf("remainder eff=%.3f, want 2.903", rem.EffectiveImpactMs)
		}
		if c := det.ShareClaims[0]; !c.Resolved || !hmc377R11Successor(c) || c.HolderKeys[0] != rem.ShareHolderKey {
			t.Errorf("the detail's anchored claim resolves to the remainder successor: %+v", c)
		}
		if !strings.Contains(det.Summary, "which another row of this thread on the published board counts (the row that held it is published only as context, tier context_only)") || strings.Contains(det.Summary, "which that chain seat already publishes") {
			t.Errorf("detail summary %q", det.Summary)
		}
		if strings.Contains(det.Summary, "represented by this thread's chain seat") || strings.Contains(det.Summary, "rides as that seat's lossless detail") ||
			!strings.Contains(det.Summary, "; this thread's chain seat is published only as context, tier context_only: 1.903ms of this 2.903ms ready-time account") {
			t.Errorf("the detail must not say the withdrawn chain seat represents it: %q", det.Summary)
		}
	})
	t.Run("engine cap cut: the remainder keeps the compaction form", func(t *testing.T) {
		rem := hmc377R11Remainder(5, 1.903, 6.903, 10.5, 60, 90)
		board := []RootCauseRankItem{hmc377R11OtherSeat(), rem}
		resolveRankShareClaims(board, nil)
		if board[1].EffectiveImpactMs != 5 || hmc377R11Returned(board[1]) != 0 || !strings.Contains(board[1].Summary, "(the owning seat is not on the published board; see the compaction disclosure)") {
			t.Errorf("cap-cut remainder: eff=%.3f returned=%.3f %q", board[1].EffectiveImpactMs, hmc377R11Returned(board[1]), board[1].Summary)
		}
	})
}
