package tracequery

// HMC377 round 13 — batch-owner ruling R-377-13 ⑤⑥⑦ (2026-11-22).
//
// Review 12 (lens holders, ~/hmc-work/hmc377/fix12/review-holders/evidence/
// F1_*.txt … F5_*.txt; boards S1/S4/S6 of fix12/review-pins/probe) reproduced,
// in universes built with the engine's own typed internals:
//   - R12H-1: a gate-withdrawn absorber kept its claimless detail and its
//     family key; the re-paired detail's new host got the same key, so two
//     rows wore one key and the face hosted both details under the
//     context-only row (Q1key, Q1cChurn, Q1bTwin, Q6census, Q6bcensus);
//   - R12N-2: a released detail owning nothing, held by two rows, stayed a
//     zero-valued context row whose sentence named one successor (S1, Q2two,
//     Q8absSucc);
//   - R12H-2: the per-round reset of the entry holds was unpinned (Q9round2:
//     the ◇ remainder published 0.853 instead of 1.903 without it);
//   - R12H-3 / R12H-4: a share two successor rows hold took the
//     one-successor form (Q3succ2), and the R-377-4 detail head called the
//     chain seat context-only while a published chain seat held part (Q5b).
// Ruling ⑤: a withdrawn row absorbs nothing — it ends with no family key and
// no absorbed rows; claim-bearing details are decided again by the one
// resolver, claimless ones follow their B4 owner; a released row owning no
// instants rides as the lossless detail of the published row holding the
// largest part of its account (equal parts: the earlier board row), never a
// zero-valued second object. Ruling ⑥: one disposition means one disposition
// and one holder row. Ruling ⑦: every round-12/13 branch that changes a value,
// a carrier or a word is pinned (or deleted when no shape reaches it).
//
// The publication is applied through rank_share_claims_publication_shim_r13_
// hmc377_test.go (the red run on 532dc208d swaps in that commit's
// return-value signature and the round-12 phrase bytes).

import (
	"math"
	"strings"
	"testing"
)

// hmc377R13InvDetail is an R-377-4 inversion window account of 6326 (whole
// ready account full; anchored inside anchored; rest outside), paired with
// the row carrying the same represents key (the B4 reconcile absorbs it).
func hmc377R13InvDetail(full float64, anchored, rest []foldInterval, key string, l0 int) RootCauseRankItem {
	det := RootCauseRankItem{Type: "priority_inversion_runnable_wait", Source: "window_stats", Thread: hmc377R11Dep,
		ImpactMs: full, CumulativeImpactMs: full, EffectiveImpactMs: full, RunnableMs: full, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.7, StartTs: 10.2, EndTs: 10.501, LineStart: l0, LineEnd: l0 + 10,
		Summary: "binder:458_5-6326 same-CPU lower-priority overlap over the ready account"}
	var remainder RootCauseShareClaim
	if len(rest) > 0 {
		remainder = rspaClaim(RootCauseShareClaimRemainder, RootCauseShareHolderRemainderSeat, hmc377R11Dep.PID, false, rspaFoldLengthMs(rest), rest)
	}
	a := rspaFoldLengthMs(anchored)
	rspaAppendSentence(&det, rspaInversionRepresentedSummary(a, full,
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, a, anchored), remainder))
	det.inversionAccountRepresentedBy = key
	return det
}

// hmc377R13SchedTwin is a scheduler_latency diagnostic projection that is an
// exact interval twin of seat (the B4 scheduler pairs): no claim of its own.
func hmc377R13SchedTwin(seat RootCauseRankItem) RootCauseRankItem {
	ms := seat.RunnableMs
	return RootCauseRankItem{Rank: 6, Tier: "tertiary", Type: "scheduler_latency", Source: "scheduler_latency_stats", Thread: hmc377R11Dep,
		ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, ProjectedImpactMs: ms, RunnableMs: ms, DominantState: "runnable",
		ChainRelevance: seat.ChainRelevance, Causality: seat.Causality, Confidence: 0.6,
		StatsWindowStartTs: 10, StatsWindowEndTs: 11,
		StartTs: seat.StartTs, EndTs: seat.EndTs, LineStart: seat.LineStart, LineEnd: seat.LineEnd,
		Summary: "binder:458_5-6326 scheduler latency"}
}

// hmc377R13Rem is the thread's ◇ remainder with its rest at [10.5, 10.5+rest)
// and an anchored claim over anchored.
func hmc377R13Rem(rest float64, anchored []foldInterval, l0 int) RootCauseRankItem {
	a := rspaFoldLengthMs(anchored)
	rem := hmc377R11Remainder(rest, a, rest+a, 10.5, l0, l0+10)
	rspaSetSentence(&rem, rspaRemainderSentence(hmc377R11Dep, "runnable (scheduling-pressure candidate)", rest, a, rest+a,
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, a, anchored)))
	return rem
}

// hmc377R13Census is a census priority-inversion ready account on the board,
// represented by the chain seat on windows (a successor claimant, tier 5).
func hmc377R13Census(windows []foldInterval, l0 int) RootCauseRankItem {
	ms := rspaFoldLengthMs(windows)
	p := RootCauseRankItem{Rank: 4, Tier: "tertiary", Type: "priority_inversion_runnable_wait", Source: "window_stats", Thread: hmc377R11Dep,
		ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, ProjectedImpactMs: ms, RunnableMs: ms, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.7, StatsWindowStartTs: 10, StatsWindowEndTs: 11,
		StartTs: windows[0].start, EndTs: windows[len(windows)-1].end, LineStart: l0, LineEnd: l0 + 5,
		ChainAnchorRepresentedByChainSeat: true, Summary: "binder:458_5-6326 same-CPU lower-priority overlap"}
	rspaAppendSentence(&p, rspaRepresentedDemotionSummary(ms, ms,
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, ms, windows)))
	return p
}

// hmc377R13Enrich is the production enrich order on a hand board: the
// restriction carve, the LEVELMERGE-1 split, the B4 reconcile (lossless
// details) and the one resolver.
func hmc377R13Enrich(items []RootCauseRankItem) RootCauseRankResult {
	carveCPUConstraintReadyInstants(items)
	items = splitAggregateGatedRunnableShare(ChainResult{Target: hmc377R11Target}, items)
	rank := RootCauseRankResult{Window: TimeWindow{StartTs: 10, EndTs: 11}, Items: items}
	reconcileExactCrossTypeRankSeats(&rank)
	resolveRankShareClaims(rank.Items, rank.AbsorbedItems)
	return rank
}

// hmc377R13Gate is the trace_query publication gate's decision on a copy of
// rank: every row the predicate names is published as context only (its
// type and rank seat go, as the tool's priority proof boundary writes them),
// then the decision is applied (WithdrawRankShareHolders).
func hmc377R13Gate(rank RootCauseRankResult, board, detail func(RootCauseRankItem) bool) (RootCauseRankResult, []bool) {
	pub := rank
	pub.Items = append([]RootCauseRankItem(nil), rank.Items...)
	pub.AbsorbedItems = append([]RootCauseRankItem(nil), rank.AbsorbedItems...)
	gate := func(rows []RootCauseRankItem, pick func(RootCauseRankItem) bool) []bool {
		out := make([]bool, len(rows))
		for i := range rows {
			if pick == nil || !pick(rows[i]) {
				continue
			}
			out[i] = true
			rows[i].Rank, rows[i].Tier, rows[i].Type, rows[i].Score = 0, RootCauseTierContextOnly, "unknown_state", 0
			rows[i].EffectiveImpactMs, rows[i].GatedRunnableMs, rows[i].GatedRunningDeficitMs = 0, 0, 0
			rows[i].Summary = "scheduler state retained as context"
		}
		return out
	}
	wb := gate(pub.Items, board)
	wd := gate(pub.AbsorbedItems, detail)
	hmc377R13Withdraw(&pub, wb, wd)
	return pub, wb
}

func hmc377R13IsChainInversion(it RootCauseRankItem) bool {
	return it.Type == "priority_inversion_candidate"
}

func hmc377R13IsAnyInversion(it RootCauseRankItem) bool {
	return it.Type == "priority_inversion_candidate" || it.Type == "priority_inversion_runnable_wait"
}

// hmc377R13Build builds the review-12 universes of R12H-1 / R12N-2 (hand raw
// ready time of 6326 in raw).
func hmc377R13Build(name string) (rank RootCauseRankResult, raw float64, board, detail func(RootCauseRankItem) bool) {
	rest := []foldInterval{{start: 10.5, end: 10.501}}
	board = hmc377R13IsChainInversion
	switch name {
	case "Q1key", "Q1bTwin":
		// A chain inversion seat absorbs an R-377-4 detail (Q1key only, claims)
		// and a claimless scheduler-latency exact twin; the ◇ remainder holds
		// the rest and claims W.
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		items := []RootCauseRankItem{hmc377R11OtherSeat(), inv, hmc377R13Rem(1.0, hmc377R11W(), 70), hmc377R13SchedTwin(inv)}
		if name == "Q1key" {
			items[1].inversionAccountRepresents = "inversion_account:q1"
			items = append(items, hmc377R13InvDetail(2.903, hmc377R11W(), rest, "inversion_account:q1", 30))
		}
		return hmc377R13Enrich(items), 2.903, board, nil
	case "Q1cChurn":
		// Q1key with the claimless detail being the state-churn twin of the
		// R-377-4 inversion detail (transitive B4 ownership: churn → detail →
		// chain seat).
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		inv.inversionAccountRepresents = "inversion_account:q1c"
		det := hmc377R13InvDetail(2.903, hmc377R11W(), rest, "inversion_account:q1c", 30)
		det.StatsWindowStartTs, det.StatsWindowEndTs = 10, 11
		churn := RootCauseRankItem{Rank: 7, Tier: "tertiary", Type: "fragmented_runnable_wait", Source: "window_stats.state_churn", Thread: hmc377R11Dep,
			ImpactMs: 2.903, CumulativeImpactMs: 2.903, EffectiveImpactMs: 2.903, ProjectedImpactMs: 2.903, RunnableMs: 2.903, DominantState: "runnable",
			ChainRelevance: det.ChainRelevance, Causality: det.Causality, Confidence: 0.5, StatsWindowStartTs: 10, StatsWindowEndTs: 11,
			StartTs: 10.2, EndTs: 10.501, LineStart: 200, LineEnd: 210, Summary: "binder:458_5-6326 fragmented runnable wait"}
		return hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), inv, hmc377R13Rem(1.0, hmc377R11W(), 70), det, churn}), 2.903, board, nil
	case "Q6census", "Q6bcensus":
		// A census inversion ready account on the board (represented by the
		// chain inversion seat) absorbs its claimless scheduler-latency exact
		// twin. Q6census: the gate withdraws the census row and the chain
		// seat, ◇ remainder present; Q6bcensus: only the census row, no ◇.
		p := hmc377R13Census(hmc377R11W(), 40)
		items := []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20), p, hmc377R13SchedTwin(p)}
		if name == "Q6census" {
			return hmc377R13Enrich(append(items, hmc377R13Rem(1.0, hmc377R11W(), 70))), 2.903, hmc377R13IsAnyInversion, nil
		}
		return hmc377R13Enrich(items), 1.903, func(it RootCauseRankItem) bool { return it.Type == "priority_inversion_runnable_wait" }, nil
	case "UR5both":
		// The R-377-4 detail's own priority proof fails too: the gate withdraws
		// the chain seat and its detail (whose claims go with its seat).
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		inv.inversionAccountRepresents = "inversion_account:u5"
		items := []RootCauseRankItem{hmc377R11OtherSeat(), inv, hmc377R13Rem(1.0, hmc377R11W(), 70),
			hmc377R13InvDetail(2.903, hmc377R11W(), rest, "inversion_account:u5", 30)}
		return hmc377R13Enrich(items), 2.903, board, func(it RootCauseRankItem) bool { return it.Type == "priority_inversion_runnable_wait" }
	case "S1", "Q2two":
		// The R-377-4 detail's anchored instants pass, when the gate withdraws
		// its absorber, in parts to a CPU-restriction seat (successor tier 2)
		// and the ◇ remainder (tier 3): its claims resolve to two rows.
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		inv.inversionAccountRepresents = "inversion_account:s1"
		cpu := hmc377R12CPU([]foldInterval{{start: 10.2, end: 10.2005}, {start: 10.6, end: 10.601}}, 90)
		raw := 3.903
		if name == "Q2two" {
			cpu, raw = hmc377R12CPU([]foldInterval{{start: 10.2, end: 10.201}}, 90), 2.903
		}
		return hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), inv, cpu, hmc377R13Rem(1.0, hmc377R11W(), 70),
			hmc377R13InvDetail(2.903, hmc377R11W(), rest, "inversion_account:s1", 30)}), raw, board, nil
	}
	return RootCauseRankResult{}, 0, nil, nil
}

// hmc377R13Carriers sums the ◎ carriers of 6326 on a published board (rows
// the gate withdrew and lossless details carry nothing; a represented row
// carries nothing while it is represented).
func hmc377R13Carriers(board []RootCauseRankItem, withdrawn []bool) float64 {
	_, sum := hmc377R12Carriers(board, withdrawn)
	return sum
}

// hmc377R13FamilyFaults lists every way the published copy breaks R-377-13 ⑤:
// a withdrawn row wearing a family key or absorbed rows, two board rows
// wearing one key, a detail riding under a key no board row wears, an
// absorbed-row count that differs from the detail rows riding, a released
// row publishing zero value.
func hmc377R13FamilyFaults(pub RootCauseRankResult, withdrawn []bool) []string {
	var faults []string
	keys := map[string]int{}
	for i, it := range pub.Items {
		if i < len(withdrawn) && withdrawn[i] && (it.RankFamilyKey != "" || it.AbsorbedRankRows != 0) {
			faults = append(faults, "withdrawn row "+it.Type+" wears family key "+it.RankFamilyKey)
		}
		if it.RankFamilyKey != "" {
			keys[it.RankFamilyKey]++
		}
		if it.Thread.PID == hmc377R11Dep.PID && i >= len(withdrawn) && it.RunnableMs <= rspaAnchorIdentityTolMs && rootCauseEffectiveImpactMs(it) <= rspaAnchorIdentityTolMs {
			faults = append(faults, "zero-valued released row "+it.Type)
		}
	}
	for key, n := range keys {
		if n > 1 {
			faults = append(faults, "two rows wear one family key "+key)
		}
	}
	riding := map[string]int{}
	for _, d := range pub.AbsorbedItems {
		if keys[d.AbsorbedIntoFamily] == 0 {
			faults = append(faults, "detail "+d.Type+" rides under a key no board row wears: "+d.AbsorbedIntoFamily)
		}
		riding[d.AbsorbedIntoFamily]++
	}
	for _, it := range pub.Items {
		if it.RankFamilyKey != "" && it.AbsorbedRankRows != riding[it.RankFamilyKey] && it.AbsorbedRankRows > 0 {
			faults = append(faults, "absorbed-row count differs from the riding details on "+it.Type)
		}
	}
	return faults
}

// hostOf returns the board row a detail of the given type rides under.
func hmc377R13HostOf(pub RootCauseRankResult, typ string) (RootCauseRankItem, bool) {
	for _, d := range pub.AbsorbedItems {
		if d.Type != typ {
			continue
		}
		for _, it := range pub.Items {
			if it.RankFamilyKey != "" && it.RankFamilyKey == d.AbsorbedIntoFamily {
				return it, true
			}
		}
	}
	return RootCauseRankItem{}, false
}

func TestHMC377R13WithdrawnRowAbsorbsNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		// host: the type of the published row each named detail rides under.
		host map[string]string
	}{
		{"Q1key", map[string]string{"scheduler_latency": "runnable_wait", "priority_inversion_runnable_wait": "runnable_wait"}},
		{"Q1bTwin", map[string]string{"scheduler_latency": "runnable_wait"}},
		{"Q1cChurn", map[string]string{"fragmented_runnable_wait": "runnable_wait", "priority_inversion_runnable_wait": "runnable_wait"}},
		{"Q6census", map[string]string{"scheduler_latency": "runnable_wait"}},
		{"Q6bcensus", map[string]string{"scheduler_latency": "priority_inversion_candidate"}},
		{"UR5both", map[string]string{"unknown_state": "runnable_wait"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rank, raw, board, detail := hmc377R13Build(tc.name)
			if len(rank.AbsorbedItems) == 0 {
				t.Fatalf("precondition: the universe has lossless-detail rows")
			}
			pub, withdrawn := hmc377R13Gate(rank, board, detail)
			for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
				t.Errorf("%s", f)
			}
			for typ, want := range tc.host {
				host, ok := hmc377R13HostOf(pub, typ)
				if !ok || host.Type != want || host.Tier == RootCauseTierContextOnly {
					t.Errorf("detail %s rides under %q (tier %q), want the published %s row holding its account", typ, host.Type, host.Tier, want)
				}
			}
			if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-raw) > 0.0015 {
				t.Errorf("Σ carriers %.3f, want raw %.3f", got, raw)
			}
		})
	}
	t.Run("Q1cChurn: the state-churn twin follows the inversion detail that owns it", func(t *testing.T) {
		rank, _, board, detail := hmc377R13Build("Q1cChurn")
		pub, _ := hmc377R13Gate(rank, board, detail)
		var det, churn string
		for _, d := range pub.AbsorbedItems {
			switch d.Type {
			case "priority_inversion_runnable_wait":
				det = d.AbsorbedIntoFamily
			case "fragmented_runnable_wait":
				churn = d.AbsorbedIntoFamily
			}
		}
		if det == "" || churn != det {
			t.Errorf("the churn twin rides under %q, its owner under %q", churn, det)
		}
	})
	t.Run("the state-churn twin follows its owner, not the withdrawn seat's account", func(t *testing.T) {
		// The chain seat's W passes in parts: W1 to a restriction seat
		// (successor tier 2), W2 to the ◇ remainder (tier 3), whose own rest is
		// 2.000. The inversion detail rides under the ◇ (it holds the larger
		// part of the detail's account), while the restriction seat holds the
		// larger part of the withdrawn seat's own account: the churn twin of
		// the detail follows the detail.
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		inv.inversionAccountRepresents = "inversion_account:q1d"
		det := hmc377R13InvDetail(3.903, hmc377R11W(), []foldInterval{{start: 10.5, end: 10.502}}, "inversion_account:q1d", 30)
		det.StatsWindowStartTs, det.StatsWindowEndTs = 10, 11
		churn := RootCauseRankItem{Rank: 7, Tier: "tertiary", Type: "fragmented_runnable_wait", Source: "window_stats.state_churn", Thread: hmc377R11Dep,
			ImpactMs: 3.903, CumulativeImpactMs: 3.903, EffectiveImpactMs: 3.903, ProjectedImpactMs: 3.903, RunnableMs: 3.903, DominantState: "runnable",
			ChainRelevance: det.ChainRelevance, Causality: det.Causality, Confidence: 0.5, StatsWindowStartTs: 10, StatsWindowEndTs: 11,
			StartTs: 10.2, EndTs: 10.502, LineStart: 200, LineEnd: 210, Summary: "binder:458_5-6326 fragmented runnable wait"}
		cpu := hmc377R12CPU(rspaFoldUnion(hmc377R11W1(), []foldInterval{{start: 10.6, end: 10.601}}), 90)
		pub, withdrawn := hmc377R13Gate(hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), inv, cpu, hmc377R13Rem(2.0, hmc377R11W(), 70), det, churn}), hmc377R13IsChainInversion, nil)
		for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
			t.Errorf("%s", f)
		}
		detHost, ok1 := hmc377R13HostOf(pub, "priority_inversion_runnable_wait")
		churnHost, ok2 := hmc377R13HostOf(pub, "fragmented_runnable_wait")
		if !ok1 || !ok2 || !detHost.ChainAnchorRemainderSeat || churnHost.RankFamilyKey != detHost.RankFamilyKey {
			t.Errorf("detail rides under %q, its churn twin under %q (want both under the ◇ remainder)", detHost.Type, churnHost.Type)
		}
	})
	t.Run("a claimless twin follows a withdrawn owner's account passed whole without instants", func(t *testing.T) {
		// The withdrawn seat's hold carries no instants: the ◇ remainder's
		// value-only claim takes it whole; the twin rides under the ◇.
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		inv.rspaHeld = nil
		rspaHold(&inv, RootCauseShareHolderChainSeat, false, nil, 1.903)
		rem := hmc377R11Remainder(1.0, 1.903, 2.903, 10.5, 70, 80)
		rspaSetSentence(&rem, rspaRemainderSentence(hmc377R11Dep, "runnable (scheduling-pressure candidate)", 1.0, 1.903, 2.903,
			rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, nil)))
		pub, withdrawn := hmc377R13Gate(hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), inv, rem, hmc377R13SchedTwin(inv)}), hmc377R13IsChainInversion, nil)
		for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
			t.Errorf("%s", f)
		}
		if host, ok := hmc377R13HostOf(pub, "scheduler_latency"); !ok || !host.ChainAnchorRemainderSeat {
			t.Errorf("the twin rides under %q, want the ◇ remainder that took the withdrawn seat's account", host.Type)
		}
		if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-2.903) > 0.0015 {
			t.Errorf("Σ carriers %.3f, want raw 2.903", got)
		}
	})
	t.Run("another reconciliation lane's family key is not read as a B4 family", func(t *testing.T) {
		// A G1 family owner wears its family key with no B4 absorbed-row
		// count; the re-run's family count leaves it as it is.
		rank, _, board, detail := hmc377R13Build("Q1key")
		io := RootCauseRankItem{Rank: 8, Tier: "tertiary", Type: "io_latency", Source: "window_stats", Thread: hmc377R11Other,
			ImpactMs: 2, CumulativeImpactMs: 2, EffectiveImpactMs: 2, Confidence: 0.6, StartTs: 10.7, EndTs: 10.702, LineStart: 800, LineEnd: 810,
			RankFamilyKey: "family:io_latency|pid:6400"}
		rank.Items = append(rank.Items, io)
		pub, _ := hmc377R13Gate(rank, board, detail)
		for _, it := range pub.Items {
			if it.Type == "io_latency" && it.RankFamilyKey != "family:io_latency|pid:6400" {
				t.Errorf("the G1 family key changed: %q", it.RankFamilyKey)
			}
		}
	})
	t.Run("another thread's hold on the same instants is another account", func(t *testing.T) {
		// Another thread's seat, listed first, holds instants over W (and
		// more): the twin still rides under the ◇ of its own thread.
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		other := hmc377R11OtherSeat()
		other.rspaHeld = nil
		rspaHold(&other, RootCauseShareHolderChainSeat, false, []foldInterval{{start: 10.1, end: 10.3}}, 200)
		pub, withdrawn := hmc377R13Gate(hmc377R13Enrich([]RootCauseRankItem{other, inv, hmc377R13Rem(1.0, hmc377R11W(), 70), hmc377R13SchedTwin(inv)}), hmc377R13IsChainInversion, nil)
		for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
			t.Errorf("%s", f)
		}
		if host, ok := hmc377R13HostOf(pub, "scheduler_latency"); !ok || host.Thread.PID != hmc377R11Dep.PID || !host.ChainAnchorRemainderSeat {
			t.Errorf("the twin rides under %s-%d, want its own thread's ◇ remainder", host.Type, host.Thread.PID)
		}
	})
	t.Run("a semantic span's hold is another account than the withdrawn seat's ready time", func(t *testing.T) {
		// A same-thread host-edge semantic seat holds a span over the
		// withdrawn seat's W (and more), listed before the ◇: the twin still
		// rides under the ◇ that took W, never under the semantic seat.
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		sem := RootCauseRankItem{Rank: 3, Tier: "secondary", Type: "semantic_work", Source: "wakeup_chain.causal_impacts", Thread: hmc377R11Dep,
			ImpactMs: 4, CumulativeImpactMs: 4, EffectiveImpactMs: 4, ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8,
			StartTs: 10.199, EndTs: 10.203, LineStart: 300, LineEnd: 310}
		sem.rspaHeld = []rspaHeldShare{{role: RootCauseShareHolderResidualSeat, semantic: true, windows: []foldInterval{{start: 10.199, end: 10.203}}, ms: 4}}
		pub, withdrawn := hmc377R13Gate(hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), inv, sem, hmc377R13Rem(1.0, hmc377R11W(), 70), hmc377R13SchedTwin(inv)}), hmc377R13IsChainInversion, nil)
		for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
			t.Errorf("%s", f)
		}
		if host, ok := hmc377R13HostOf(pub, "scheduler_latency"); !ok || !host.ChainAnchorRemainderSeat {
			t.Errorf("the twin rides under %q, want the ◇ remainder", host.Type)
		}
	})
	t.Run("a claimless detail owned by a claimless detail waits for its owner", func(t *testing.T) {
		// churn (state-churn twin) → x (window ready account, exact interval
		// twin of the chain seat) → inv: the gate withdraws inv and no
		// published row holds its account, so x is published with its own
		// value and churn rides as x's detail — listed before x, it waits for
		// x to be placed (otherwise both would carry the same instants).
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		x := RootCauseRankItem{Rank: 6, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R11Dep,
			ImpactMs: 1.903, CumulativeImpactMs: 1.903, EffectiveImpactMs: 1.903, ProjectedImpactMs: 1.903, RunnableMs: 1.903, DominantState: "runnable",
			ChainRelevance: inv.ChainRelevance, Causality: inv.Causality, Confidence: 0.6, StatsWindowStartTs: 10, StatsWindowEndTs: 11,
			StartTs: inv.StartTs, EndTs: inv.EndTs, LineStart: inv.LineStart, LineEnd: inv.LineEnd, Summary: "binder:458_5-6326 runnable"}
		churn := x
		churn.Type, churn.Source, churn.LineStart, churn.LineEnd, churn.Summary = "fragmented_runnable_wait", "window_stats.state_churn", 200, 210, "binder:458_5-6326 fragmented runnable wait"
		rank := hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), inv, churn, x})
		if len(rank.AbsorbedItems) != 2 || rank.AbsorbedItems[0].Type != "fragmented_runnable_wait" {
			t.Fatalf("precondition: churn then x ride as the chain seat's details: %d", len(rank.AbsorbedItems))
		}
		pub, withdrawn := hmc377R13Gate(rank, hmc377R13IsChainInversion, nil)
		for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
			t.Errorf("%s", f)
		}
		if host, ok := hmc377R13HostOf(pub, "fragmented_runnable_wait"); !ok || host.Type != "runnable_wait" || host.Source != "window_stats" {
			t.Errorf("the churn twin rides under %q/%q, want its owner x on the board", host.Type, host.Source)
		}
		if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-1.903) > 0.0015 {
			t.Errorf("Σ carriers %.3f, want raw 1.903", got)
		}
	})
	t.Run("a claimless twin whose withdrawn owner's account no published row holds is published with its own value", func(t *testing.T) {
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		pub, withdrawn := hmc377R13Gate(hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), inv, hmc377R13SchedTwin(inv)}), hmc377R13IsChainInversion, nil)
		if len(pub.AbsorbedItems) != 0 {
			t.Fatalf("the twin still rides as detail: %+v", pub.AbsorbedItems)
		}
		for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
			t.Errorf("%s", f)
		}
		if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-1.903) > 0.0015 {
			t.Errorf("Σ carriers %.3f, want raw 1.903 (the twin carries the withdrawn seat's ready time once)", got)
		}
	})
}

func TestHMC377R13ReleasedRowOwningNothingRidesUnderTheLargestHolder(t *testing.T) {
	for _, name := range []string{"S1", "Q2two"} {
		t.Run(name, func(t *testing.T) {
			rank, raw, board, detail := hmc377R13Build(name)
			pub, withdrawn := hmc377R13Gate(rank, board, detail)
			for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
				t.Errorf("%s", f)
			}
			host, ok := hmc377R13HostOf(pub, "priority_inversion_runnable_wait")
			if !ok || !host.ChainAnchorRemainderSeat {
				t.Fatalf("the inversion detail rides under %+v, want the ◇ remainder (it holds the largest part of its account)", host.Type)
			}
			var det RootCauseRankItem
			for _, d := range pub.AbsorbedItems {
				if d.Type == "priority_inversion_runnable_wait" {
					det = d
				}
			}
			if math.Abs(det.RunnableMs-2.903) > 0.0005 || math.Abs(det.EffectiveImpactMs-2.903) > 0.0005 {
				t.Errorf("a lossless detail keeps its minted values: run=%.3f eff=%.3f", det.RunnableMs, det.EffectiveImpactMs)
			}
			// More than one row holds its parts: the words name none of them.
			for _, want := range []string{hmc377R13InversionHeadSeeDetail(), "of which " + hmc377R12SharePartsSeeDetail(), rspaInversionDetailTailContext} {
				if !strings.Contains(det.Summary, want) {
					t.Errorf("detail words %q\nwant %q", det.Summary, want)
				}
			}
			for _, banned := range []string{rspaInversionDetailHeadContext, rspaInversionChainSeatSuccessor, rspaInversionDetailTailHeld} {
				if strings.Contains(det.Summary, banned) {
					t.Errorf("detail words name one holder: %q", det.Summary)
				}
			}
			if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-raw) > 0.0015 {
				t.Errorf("Σ carriers %.3f, want raw %.3f", got, raw)
			}
		})
	}
	t.Run("equal parts go to the earlier row of the published board", func(t *testing.T) {
		half := []foldInterval{{start: 10.2, end: 10.2009515}}
		for _, cpuFirst := range []bool{true, false} {
			inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
			inv.inversionAccountRepresents = "inversion_account:tie"
			cpu := hmc377R12CPU(rspaFoldUnion(half, []foldInterval{{start: 10.6, end: 10.601}}), 90)
			rem := hmc377R13Rem(0.9515, hmc377R11W(), 70)
			items := []RootCauseRankItem{hmc377R11OtherSeat(), inv, cpu, rem}
			if !cpuFirst {
				items = []RootCauseRankItem{hmc377R11OtherSeat(), inv, rem, cpu}
			}
			items = append(items, hmc377R13InvDetail(1.903, hmc377R11W(), nil, "inversion_account:tie", 30))
			pub, _ := hmc377R13Gate(hmc377R13Enrich(items), hmc377R13IsChainInversion, nil)
			host, ok := hmc377R13HostOf(pub, "priority_inversion_runnable_wait")
			want := "cpu_affinity_or_cpuset"
			if !cpuFirst {
				want = "runnable_wait"
			}
			if !ok || host.Type != want {
				t.Errorf("cpuFirst=%t: the detail rides under %q, want the earlier row %s", cpuFirst, host.Type, want)
			}
		}
	})
	t.Run("a share without instants held by its successor still carries the released row's account", func(t *testing.T) {
		// The chain seat's hold and the inversion detail's claim carry no
		// instants (no segment inventory reproduced the accounts); the ◇
		// remainder's value-only claim takes the withdrawn seat's share whole,
		// so the detail owns nothing and rides under the ◇ by value.
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		inv.rspaHeld = nil
		rspaHold(&inv, RootCauseShareHolderChainSeat, false, nil, 1.903)
		inv.inversionAccountRepresents = "inversion_account:vo"
		rem := hmc377R11Remainder(1.0, 1.903, 2.903, 10.5, 70, 80)
		rspaSetSentence(&rem, rspaRemainderSentence(hmc377R11Dep, "runnable (scheduling-pressure candidate)", 1.0, 1.903, 2.903,
			rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, nil)))
		det := RootCauseRankItem{Type: "priority_inversion_runnable_wait", Source: "window_stats", Thread: hmc377R11Dep,
			ImpactMs: 1.903, CumulativeImpactMs: 1.903, EffectiveImpactMs: 1.903, RunnableMs: 1.903, DominantState: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.7, StartTs: 10.2, EndTs: 10.201903, LineStart: 30, LineEnd: 40,
			Summary: "binder:458_5-6326 same-CPU lower-priority overlap over the ready account"}
		rspaAppendSentence(&det, rspaInversionRepresentedSummary(1.903, 1.903,
			rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.903, nil), RootCauseShareClaim{}))
		det.inversionAccountRepresentedBy = "inversion_account:vo"
		pub, withdrawn := hmc377R13Gate(hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), inv, rem, det}), hmc377R13IsChainInversion, nil)
		for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
			t.Errorf("%s", f)
		}
		if host, ok := hmc377R13HostOf(pub, "priority_inversion_runnable_wait"); !ok || !host.ChainAnchorRemainderSeat {
			t.Errorf("the detail rides under %q, want the ◇ remainder holding its whole account by value", host.Type)
		}
	})
	t.Run("a released row whose account its claims do not cover stays on the board with the rest", func(t *testing.T) {
		// The detail's whole account is 2.903 but it claims only W (1.903);
		// the satellite (successor tier 5) takes W. The detail owns the other
		// 1.000 — never a detail that drops it.
		inv := hmc377R11Inv(1.903, hmc377R11W(), 10, 20)
		inv.inversionAccountRepresents = "inversion_account:open"
		items := []RootCauseRankItem{hmc377R11OtherSeat(), inv, hmc377R11Satellite1(),
			hmc377R13InvDetail(2.903, hmc377R11W(), nil, "inversion_account:open", 30)}
		pub, withdrawn := hmc377R13Gate(hmc377R13Enrich(items), hmc377R13IsChainInversion, nil)
		if len(pub.AbsorbedItems) != 0 {
			t.Fatalf("the detail rides as detail although it carries an account no row holds")
		}
		det := pub.Items[len(pub.Items)-1]
		if det.Type != "priority_inversion_runnable_wait" || math.Abs(det.RunnableMs-1.0) > 0.0005 {
			t.Errorf("released row %s run=%.3f, want its unclaimed 1.000", det.Type, det.RunnableMs)
		}
		if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-2.903) > 0.0015 {
			t.Errorf("Σ carriers %.3f, want raw 2.903", got)
		}
	})
}

// R12H-2 (mutants M06 / N04): each round of the successor rule starts from
// the holds the rows carried on entry — a hold an earlier round passed to a
// row that is a lossless detail again (or to a lower-tier claimant) is not
// kept when the round decides the withdrawn seat's segments again.
func TestHMC377R13EachRoundStartsFromTheEntryHolds(t *testing.T) {
	rest := []foldInterval{{start: 10.5, end: 10.501}}
	for _, name := range []string{"Q9round2", "Q9bround2"} {
		t.Run(name, func(t *testing.T) {
			// A ◇ remainder emptied into a restriction seat (its lossless
			// detail) claims W; a represented satellite claims W1 only. Round
			// 1 hands W1 to the satellite and releases the ◇ (it then owns
			// W2); round 2 decides W again with the ◇ (tier 3) on the board.
			items := []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20),
				hmc377R13Rem(1.0, hmc377R11W(), 70), hmc377R12CPU(rest, 90),
				hmc377R11Satellite(1.05, 10.2, 10.20105, rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R11Dep.PID, false, 1.05, hmc377R11W1()))}
			if name == "Q9bround2" {
				items[2], items[3], items[4] = items[3], items[4], items[2]
			}
			rank := hmc377R13Enrich(items)
			if len(rank.AbsorbedItems) != 1 || !rank.AbsorbedItems[0].ChainAnchorRemainderSeat {
				t.Fatalf("precondition: the emptied ◇ rides as the restriction seat's detail")
			}
			pub, withdrawn := hmc377R13Gate(rank, hmc377R13IsChainInversion, nil)
			// The restriction seat the ◇ no longer rides under owns no family.
			for _, f := range hmc377R13FamilyFaults(pub, withdrawn) {
				t.Errorf("%s", f)
			}
			for _, it := range pub.Items {
				if it.Type == "cpu_affinity_or_cpuset" && (it.RankFamilyKey != "" || it.AbsorbedRankRows != 0) {
					t.Errorf("the restriction seat still wears the released ◇'s family: %q rows=%d", it.RankFamilyKey, it.AbsorbedRankRows)
				}
			}
			var rem, sat *RootCauseRankItem
			for i := range pub.Items {
				switch {
				case pub.Items[i].ChainAnchorRemainderSeat:
					rem = &pub.Items[i]
				case pub.Items[i].Type == "runnable_wait" && pub.Items[i].Source == "window_stats":
					sat = &pub.Items[i]
				}
			}
			if rem == nil || sat == nil {
				t.Fatalf("published rows: ◇ %v satellite %v", rem != nil, sat != nil)
			}
			if math.Abs(rem.EffectiveImpactMs-1.903) > 0.0005 || math.Abs(rem.ShareReturnedMs-1.903) > 0.0005 {
				t.Errorf("◇ eff=%.3f returned=%.3f, want 1.903 / 1.903 (it takes all of W in round 2)", rem.EffectiveImpactMs, rem.ShareReturnedMs)
			}
			if !sat.ChainAnchorRepresentedByChainSeat || !sat.ShareClaims[0].Resolved {
				t.Errorf("the satellite's W1 resolves to the ◇ (represented=%t): %+v", sat.ChainAnchorRepresentedByChainSeat, sat.ShareClaims)
			}
			if got := hmc377R13Carriers(pub.Items, withdrawn); math.Abs(got-2.903) > 0.0015 {
				t.Errorf("Σ carriers %.3f, want raw 2.903", got)
			}
		})
	}
}

// R12H-3 (R-377-13 ⑥): a share two successor rows hold is two holders — the
// see-detail form, never the one-successor form.
func TestHMC377R13TwoSuccessorRowsTakeTheSeeDetailForm(t *testing.T) {
	for _, satFirst := range []bool{false, true} {
		// A represented satellite over W whose holder (inv) the gate
		// withdraws; a restriction seat (tier 2) takes W1, the ◇ (tier 3)
		// takes W2.
		items := []RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20),
			hmc377R12CPU(rspaFoldUnion(hmc377R11W1(), []foldInterval{{start: 10.5, end: 10.501}}), 90), hmc377R13Rem(2.0, hmc377R11W(), 70), hmc377R11Satellite1()}
		if satFirst {
			items[3], items[4] = items[4], items[3]
		}
		pub, _ := hmc377R13Gate(hmc377R13Enrich(items), hmc377R13IsChainInversion, nil)
		for _, it := range pub.Items {
			if !strings.Contains(it.Summary, "runnable satellite") {
				continue
			}
			if want := hmc377R12RepresentedSeeDetail(1.903, 1.903, 1.903); !strings.Contains(it.Summary, want) ||
				strings.Contains(it.Summary, rspaRepresentedSuccessor(1.903, 1.903)) {
				t.Errorf("satFirst=%t: %q\nwant %q", satFirst, it.Summary, want)
			}
		}
	}
	t.Run("one successor row keeps the successor form", func(t *testing.T) {
		pub, _ := hmc377R13Gate(hmc377R13Enrich([]RootCauseRankItem{hmc377R11OtherSeat(), hmc377R11Inv(1.903, hmc377R11W(), 10, 20),
			hmc377R13Rem(2.0, hmc377R11W(), 70), hmc377R11Satellite1()}), hmc377R13IsChainInversion, nil)
		if s := pub.Items[3].Summary; !strings.Contains(s, rspaRepresentedSuccessor(1.903, 1.903)) {
			t.Errorf("%q", s)
		}
	})
}

// R12H-4 (R-377-13 ⑥): the R-377-4 detail head names the chain seat's tier
// only for one disposition; parts held by different rows (one of them a
// published chain seat) take the see-detail head.
func TestHMC377R13InversionDetailHeadSeeDetail(t *testing.T) {
	for _, withCPU := range []bool{false, true} {
		// The several-seat twin: two chain seats hold W1 / W2, the ◇ twin
		// represents the whole-account inversion row as its lossless detail;
		// the gate withdraws the W1 seat (Q5b; Q5c adds a restriction seat).
		a := hmc377R11Inv(1.05, hmc377R11W1(), 10, 15)
		b := hmc377R11Inv(0.853, hmc377R11W2(), 16, 20)
		b.Type = "runnable_wait"
		twin := hmc377R13Rem(1.0, hmc377R11W(), 70)
		twin.inversionAccountRepresents = "inversion_account:q5"
		items := []RootCauseRankItem{hmc377R11OtherSeat(), a, b, twin, hmc377R13InvDetail(2.903, hmc377R11W(), []foldInterval{{start: 10.5, end: 10.501}}, "inversion_account:q5", 30)}
		if withCPU {
			items = append(items, hmc377R12CPU(rspaFoldUnion(hmc377R11W1(), []foldInterval{{start: 10.5, end: 10.5005}}), 90))
		}
		pub, _ := hmc377R13Gate(hmc377R13Enrich(items), hmc377R13IsChainInversion, nil)
		found := false
		for _, rows := range [][]RootCauseRankItem{pub.Items, pub.AbsorbedItems} {
			for _, it := range rows {
				if it.Type != "priority_inversion_runnable_wait" {
					continue
				}
				found = true
				if !strings.Contains(it.Summary, hmc377R13InversionHeadSeeDetail()) || strings.Contains(it.Summary, rspaInversionDetailHeadContext) {
					t.Errorf("withCPU=%t: %q", withCPU, it.Summary)
				}
			}
		}
		if !found {
			t.Fatalf("withCPU=%t: no inversion detail published", withCPU)
		}
	}
}
