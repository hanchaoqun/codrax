package tracequery

// HMC377 fix round 7 (R7-2, review round 6 finding 1, ruling R-377-8 value
// part) — the same wakeup causal impact under another record identity.
//
// A seat minted from one chain causal impact is that impact's account; the
// chain view publishes its own record of the same impact
// (wakeup_causal_impact:<position>). When the rank cap cuts the rank copy, a
// claim naming that seat does not resolve against the published board, while
// the projection may still draw the impact's chain-view record (105 蜂鸟
// container #1 TID 7912: pool priority_inversion_candidate of binder:458_5-6326
// = impact #31, drawn as ⇅ E27(+1); the ◇ E43 remainder's anchored 1.903 ms
// claim named no holder). Round 7: the impact's own seat carries the impact's
// position (causalImpactOrdinal), and a claim the published board does not
// hold, whose holders on the pre-truncation pool are all impacts' own seats,
// carries those positions (HolderCausalImpacts) — typed identity, never a
// value or name match. An inversion sub-seat whose impact record publishes
// the primary-state account instead carries no position (not the same
// account).

import (
	"reflect"
	"testing"
)

func TestHMC377R7ImpactOwnSeatsCarryTheirImpactPosition(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	runDep := ThreadRef{Comm: "binder:458_5", PID: 6326}
	dDep := ThreadRef{Comm: "io-dep", PID: 300}
	plainDep := ThreadRef{Comm: "plain-dep", PID: 400}
	inversion := func(impact WakeupCausalImpact) WakeupCausalImpact {
		impact.PriorityInversionCandidate = true
		impact.PriorityRelation = "lower_priority_dependency"
		impact.PriorityRelationCaliber = string(priorityCaliberClosedRangeStable)
		impact.PriorityRelationProvenLowerMs = 3
		impact.PriorityInversionGatedMs, impact.GatedRunnableMs, impact.GatedRunningDeficitMs = 2.4, 1.9, 0.5
		return impact
	}
	chain := ChainResult{
		Target: target, Window: TimeWindow{StartTs: 5.000, EndTs: 5.100},
		Nodes: []ChainNode{
			{ID: "target", Thread: target, Window: TimeWindow{StartTs: 5.000, EndTs: 5.100}},
			{ID: "run", Thread: runDep, Depth: 2, Window: TimeWindow{StartTs: 5.010, EndTs: 5.020}},
			{ID: "d", Thread: dDep, Depth: 1, Window: TimeWindow{StartTs: 5.030, EndTs: 5.040}},
			{ID: "plain", Thread: plainDep, Depth: 1, Window: TimeWindow{StartTs: 5.050, EndTs: 5.060}},
		},
		CausalImpacts: []WakeupCausalImpact{
			inversion(WakeupCausalImpact{Thread: runDep, ChainDepth: 2, DominantState: string(StateRunning), TotalMs: 4.6, DominantImpactMs: 2.7,
				RunningMs: 2.7, RunnableMs: 1.9, Window: TimeWindow{StartTs: 5.010, EndTs: 5.020}, LineStart: 10, LineEnd: 20, OnChain: true}),
			inversion(WakeupCausalImpact{Thread: dDep, ChainDepth: 1, DominantState: string(StateDSleep), TotalMs: 9, DominantImpactMs: 6,
				DStateMs: 6, RunnableMs: 1.9, RunningMs: 1.1, Window: TimeWindow{StartTs: 5.030, EndTs: 5.040}, LineStart: 30, LineEnd: 40, OnChain: true}),
			{Thread: plainDep, ChainDepth: 1, DominantState: string(StateRunnable), TotalMs: 3, DominantImpactMs: 3, RunnableMs: 3,
				Window: TimeWindow{StartTs: 5.050, EndTs: 5.060}, LineStart: 50, LineEnd: 60, OnChain: true},
		},
	}
	if !wakeupCausalImpactUsesCompositePrioritySeat(chain.CausalImpacts[0]) || !wakeupCausalImpactHasSeparatePrioritySeat(chain.CausalImpacts[1]) {
		t.Fatal("fixture: impact #1 is a composite inversion seat, impact #2 carries a separate inversion sub-seat")
	}
	rank := buildRootCauseRankFrom(nil, Query{TimeStart: 5.000, TimeEnd: 5.100, Limit: 12}, chain, WindowStats{})
	type minted struct {
		pid int
		typ string
	}
	got := map[minted]int{}
	for _, it := range rank.preTruncationItems {
		if it.Source != "wakeup_chain.causal_impacts" {
			continue
		}
		k := minted{it.Thread.PID, it.Type}
		if prev, seen := got[k]; seen && prev != it.causalImpactOrdinal {
			t.Fatalf("%+v carries two positions %d / %d", k, prev, it.causalImpactOrdinal)
		}
		got[k] = it.causalImpactOrdinal
	}
	want := map[minted]int{
		{runDep.PID, "priority_inversion_candidate"}: 1, // the composite seat IS the impact's account
		{dDep.PID, "priority_inversion_candidate"}:   0, // the sub-seat: the record publishes the primary state
		{plainDep.PID, "runnable_wait"}:              3,
	}
	for k, w := range want {
		g, ok := got[k]
		if !ok {
			t.Fatalf("fixture: no minted seat %+v (got %v)", k, got)
		}
		if g != w {
			t.Errorf("%+v: position %d, want %d", k, g, w)
		}
	}
	primary := 0
	for k, g := range got {
		if k.pid == dDep.PID && k.typ != "priority_inversion_candidate" {
			primary = g
		}
	}
	if primary != 2 {
		t.Errorf("the primary-state seat of impact #2 carries its position: got %d (%v)", primary, got)
	}
}

// hmc377R7ImpactHolder is a chain seat minted from impact #position that
// holds the runnable share over windows.
func hmc377R7ImpactHolder(pid, position int, windows []foldInterval, ms float64) RootCauseRankItem {
	return RootCauseRankItem{Type: "priority_inversion_candidate", Source: "wakeup_chain.causal_impacts", Thread: ThreadRef{Comm: "dep", PID: pid},
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunning), GatedRunnableMs: ms, RunnableMs: ms,
		EffectiveImpactMs: ms, causalImpactOrdinal: position,
		rspaHeld: []rspaHeldShare{{role: RootCauseShareHolderChainSeat, windows: rspaFoldCopy(windows), ms: ms}}}
}

func hmc377R7Remainder(pid int, claims ...RootCauseShareClaim) RootCauseRankItem {
	return RootCauseRankItem{Type: "runnable_wait", Source: "window_stats", Thread: ThreadRef{Comm: "dep", PID: pid},
		Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent", DominantState: string(StateRunnable),
		RunnableMs: 90, ChainAnchoredMs: 1.9, ChainAnchorFullMs: 91.9, ChainAnchorRemainderSeat: true, ShareClaims: claims}
}

func TestHMC377R7UnheldClaimNamesTheCausalImpactWhoseOwnSeatHoldsIt(t *testing.T) {
	w := []foldInterval{{start: 5.010, end: 5.0119}}
	w2 := []foldInterval{{start: 5.030, end: 5.031}}
	anchored := rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, 200, false, 1.9, w)
	both := rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, 200, false, 2.9, append(append([]foldInterval(nil), w...), w2...))
	cases := []struct {
		name  string
		board []RootCauseRankItem
		pool  []RootCauseRankItem
		want  []int
	}{
		{"the impact's own seat was cut by the cap", []RootCauseRankItem{hmc377R7Remainder(200, anchored)},
			[]RootCauseRankItem{hmc377R7ImpactHolder(200, 31, w, 1.9), hmc377R7ImpactHolder(200, 31, w, 1.9)}, []int{31}},
		{"two impacts' own seats hold the share together", []RootCauseRankItem{hmc377R7Remainder(200, both)},
			[]RootCauseRankItem{hmc377R7ImpactHolder(200, 32, w2, 1), hmc377R7ImpactHolder(200, 31, w, 1.9)}, []int{31, 32}},
		{"the published board holds it (resolved, no impact named)", []RootCauseRankItem{hmc377R7Remainder(200, anchored), hmc377R7ImpactHolder(200, 31, w, 1.9)},
			[]RootCauseRankItem{hmc377R7ImpactHolder(200, 31, w, 1.9)}, nil},
		{"the cut holder is no impact's own seat", []RootCauseRankItem{hmc377R7Remainder(200, anchored)},
			[]RootCauseRankItem{hmc377R7ImpactHolder(200, 0, w, 1.9)}, nil},
		{"an impact's own seat covers only part (the rest held by a non-impact seat)", []RootCauseRankItem{hmc377R7Remainder(200, both)},
			[]RootCauseRankItem{hmc377R7ImpactHolder(200, 31, w, 1.9), hmc377R7ImpactHolder(200, 0, w2, 1)}, nil},
		{"another thread's impact seat", []RootCauseRankItem{hmc377R7Remainder(200, anchored)},
			[]RootCauseRankItem{hmc377R7ImpactHolder(201, 31, w, 1.9)}, nil},
		{"the impact seat holds other instants", []RootCauseRankItem{hmc377R7Remainder(200, anchored)},
			[]RootCauseRankItem{hmc377R7ImpactHolder(200, 31, w2, 1.9)}, nil},
	}
	for _, c := range cases {
		board := append([]RootCauseRankItem(nil), c.board...)
		resolveRankShareClaims(board, nil)
		resolveRankShareClaimHolderImpacts(board, nil, c.pool)
		claim := board[0].ShareClaims[0]
		if c.want == nil && claim.HolderCausalImpacts != nil || c.want != nil && !reflect.DeepEqual(claim.HolderCausalImpacts, c.want) {
			t.Errorf("[%s] holder impacts %v, want %v (resolved=%t)", c.name, claim.HolderCausalImpacts, c.want, claim.Resolved)
		}
		if c.want != nil && claim.Resolved {
			t.Errorf("[%s] the rank board does not hold the share — the claim stays unresolved", c.name)
		}
	}
	// A copy that becomes another seat is no impact's own account.
	copied := hmc377R7ImpactHolder(200, 31, w, 1.9)
	rspaClearClaims(&copied)
	if copied.causalImpactOrdinal != 0 {
		t.Fatal("rspaClearClaims keeps the impact position on a copy")
	}
}
