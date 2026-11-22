package tracequery

// HMC377 round 10 — batch-owner ruling R-377-10 (2026-11-21): a row the
// trace_query publication gate withdraws holds nothing and claims nothing, and
// every share keeps exactly one carrier.
//
// The review-9 hand universes (fix9/review-holders/univ/zz_rv9_universe_test.go)
// are ported here with the engine's own typed internals (held shares, claim
// instants), which a tool-package hand universe cannot carry:
//   - U1: one represented claim held by TWO seats of the thread (the inversion
//     seat and a runnable chain seat holding the same 1.903 ms segment). R9-1
//     unresolved the whole claim when the inversion seat was withdrawn, so the
//     satellite returned to ◎ beside the runnable seat: two carriers (review 9
//     R9H-1, univ/oracle9_univ.log wt-head_U1_w ELIM-R-SUM-EXCEEDS-RAW 3.806 >
//     1.903). Now the remaining holder covers the share and the claim stays
//     resolved to it alone.
//   - U2: a three-way represented sentence (1.903 walked at the inversion seat
//     + 1.000 unwalked at the thread's own unwalked seat). With the inversion
//     seat withdrawn — or cut by the cap — it resolves only in part: the
//     satellite publishes its own 2.903 less the 1.000 its unwalked seat still
//     counts (R-377-10 ③; univ/oracle9_univ.log wt-head_U2_w Σ3.903 > 2.903).
//   - U5: a withdrawn holder shared by two claimants and a published holder
//     named by a third: only the published holder's key survives.
//   - F4/F5 (fault injection, perturb/out/nec_w11098.*): the withdrawn seat
//     keeps no split and no claim, and the claimant's sentence says the
//     holding row is published only as context.
//   - LEVELMERGE-1 (U4): every claim seat of a gated-share split withdrawn →
//     GatedShareClaimWithdrawn, and the pair's sentences stop saying that
//     seat counts the share.
// The assertions use literal sentence bytes and the shim accessors in
// rank_share_claims_withdrawal_shim_hmc377_test.go, so the same file runs on
// b71f82049 with that commit's signature (the red run).

import (
	"reflect"
	"strings"
	"testing"
)

var (
	hmc377R10Dep   = ThreadRef{Comm: "binder:458_5", PID: 6326}
	hmc377R10Other = ThreadRef{Comm: "other", PID: 6400}
)

func hmc377R10Walked() []foldInterval   { return []foldInterval{{start: 10.2, end: 10.201903}} }
func hmc377R10Unwalked() []foldInterval { return []foldInterval{{start: 10.3, end: 10.301}} }

func hmc377R10OtherSeat() RootCauseRankItem {
	seat := RootCauseRankItem{Rank: 1, Tier: "primary", Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: hmc377R10Other,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", DominantState: string(StateRunnable),
		RunnableMs: 30, ImpactMs: 30, CumulativeImpactMs: 30, EffectiveImpactMs: 30, LineStart: 5, LineEnd: 6}
	rspaHold(&seat, RootCauseShareHolderChainSeat, false, []foldInterval{{start: 10.1, end: 10.13}}, 30)
	return seat
}

// hmc377R10Inversion is the dependency thread's priority-inversion chain seat
// holding the walked 1.903 ms (the row the gate withdraws in these
// universes; lines 10..20).
func hmc377R10Inversion() RootCauseRankItem {
	inv := RootCauseRankItem{Rank: 2, Tier: "secondary", Type: "priority_inversion_candidate", Source: "wakeup_chain.causal_impacts", Thread: hmc377R10Dep,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", DominantState: string(StateRunnable),
		RunnableMs: 1.903, GatedRunnableMs: 1.903, ImpactMs: 1.903, CumulativeImpactMs: 1.903, EffectiveImpactMs: 1.903,
		LineStart: 10, LineEnd: 20}
	rspaHold(&inv, RootCauseShareHolderChainSeat, false, hmc377R10Walked(), 1.903)
	return inv
}

func hmc377R10Satellite(ms float64) RootCauseRankItem {
	return RootCauseRankItem{Rank: 4, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R10Dep,
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", DominantState: string(StateRunnable),
		RunnableMs: ms, ImpactMs: ms, ProjectedImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, Score: ms,
		ChainAnchorRepresentedByChainSeat: true, LineStart: 30, LineEnd: 40, Summary: "binder:458_5-6326 runnable satellite"}
}

// U1: the represented claim is held by the inversion seat AND a runnable
// chain seat of the thread (the C-1 dual-seat shape, one physical segment).
func hmc377R10U1Board() []RootCauseRankItem {
	run := RootCauseRankItem{Rank: 3, Tier: "secondary", Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: hmc377R10Dep,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", DominantState: string(StateRunnable),
		RunnableMs: 1.903, ImpactMs: 1.903, CumulativeImpactMs: 1.903, EffectiveImpactMs: 1.903, LineStart: 21, LineEnd: 25}
	rspaHold(&run, RootCauseShareHolderChainSeat, false, hmc377R10Walked(), 1.903)
	sat := hmc377R10Satellite(1.903)
	rspaAppendSentence(&sat, rspaRepresentedDemotionSummary(1.903, 1.903,
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R10Dep.PID, false, 1.903, hmc377R10Walked())))
	board := []RootCauseRankItem{hmc377R10OtherSeat(), hmc377R10Inversion(), run, sat}
	resolveRankShareClaims(board, nil)
	return board
}

// U2: a three-way represented sentence — 1.903 walked (inversion seat) +
// 1.000 inside the unwalked waits (the thread's own unwalked seat). cut=true
// leaves the inversion seat off the board (the engine cap cut it).
func hmc377R10U2Board(cut bool) []RootCauseRankItem {
	unw := RootCauseRankItem{Rank: 3, Tier: "secondary", Type: "runnable_wait", Source: "wakeup_chain.causal_impacts", Thread: hmc377R10Dep,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", DominantState: string(StateRunnable),
		RunnableMs: 1.0, ImpactMs: 1.0, CumulativeImpactMs: 1.0, EffectiveImpactMs: 1.0, ChainAnchorUnwalkedSeat: true, LineStart: 41, LineEnd: 45}
	rspaHold(&unw, RootCauseShareHolderUnwalkedSeat, false, hmc377R10Unwalked(), 1.0)
	sat := hmc377R10Satellite(2.903)
	claims := rspaRowClaims{
		anchored: rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R10Dep.PID, false, 1.903, hmc377R10Walked()),
		unwalked: rspaClaim(RootCauseShareClaimUnwalked, RootCauseShareHolderUnwalkedSeat, hmc377R10Dep.PID, false, 1.0, hmc377R10Unwalked()),
	}
	rspaAppendSentence(&sat, rspaRepresentedDemotionSummaryThreeWay(1.903, 1.0, 2.903, "the analysed thread's wait(s) that this thread's own wakeup ended", rspaRowScope{owned: 1.0}, claims))
	board := []RootCauseRankItem{hmc377R10OtherSeat(), hmc377R10Inversion(), unw, sat}
	if cut {
		board = []RootCauseRankItem{hmc377R10OtherSeat(), unw, sat}
	}
	resolveRankShareClaims(board, nil)
	return board
}

func hmc377R10Copy(board []RootCauseRankItem) []RootCauseRankItem {
	return append([]RootCauseRankItem(nil), board...)
}

func TestHMC377R10WithdrawnHolderLeavesTheRemainingHolderResolved(t *testing.T) {
	board := hmc377R10U1Board()
	if c := board[3].ShareClaims[0]; !c.Resolved || !reflect.DeepEqual(c.HolderKeys, []string{"seat-2", "seat-3"}) {
		t.Fatalf("precondition: the engine resolves the claim to both holders: %+v", c)
	}
	published := hmc377R10Copy(board)
	hmc377R10Withdraw(published, nil, []bool{false, true, false, false}, nil)
	inv, run, sat := published[1], published[2], published[3]
	if c := sat.ShareClaims[0]; !c.Resolved || !c.Exact || !reflect.DeepEqual(c.HolderKeys, []string{"seat-3"}) {
		t.Errorf("the remaining holder covers the share: the claim must stay resolved to it alone: %+v", c)
	}
	if !sat.ChainAnchorRepresentedByChainSeat || hmc377R10Held(sat) != 0 || sat.EffectiveImpactMs != 1.903 {
		t.Errorf("the satellite stays represented whole (one carrier: the runnable seat): marker=%t held=%.3f eff=%.3f",
			sat.ChainAnchorRepresentedByChainSeat, hmc377R10Held(sat), sat.EffectiveImpactMs)
	}
	if !strings.Contains(sat.Summary, "; anchored share represented by the chain seat: this satellite's whole account (1.903ms of 1.903ms)") {
		t.Errorf("the represented sentence keeps its held form: %q", sat.Summary)
	}
	if inv.ShareHolderKey != "" || run.ShareHolderKey != "seat-3" {
		t.Errorf("holder keys: withdrawn %q (want none), runnable %q (want seat-3)", inv.ShareHolderKey, run.ShareHolderKey)
	}
	if c := board[3].ShareClaims[0]; !c.Resolved || len(c.HolderKeys) != 2 || board[1].ShareHolderKey != "seat-2" {
		t.Errorf("publication mutated the engine result: %+v key=%q", c, board[1].ShareHolderKey)
	}
}

func TestHMC377R10PartlyResolvedRepresentedSentencePublishesTheRestOnly(t *testing.T) {
	const partialWithdrawn = "; this satellite's whole account (2.903ms of 2.903ms) lies inside the thread's dependency windows: 1.000ms of it is held by a seat of this thread on the published board, which counts it, and the seat holding the other 1.903ms is published only as context, tier context_only — this diagnostic projection rides the adjacent lane and its published values count only those 1.903ms"
	const partialCut = "; this satellite's whole account (2.903ms of 2.903ms) lies inside the thread's dependency windows: 1.000ms of it is held by a seat of this thread on the published board, which counts it, and the seat holding the other 1.903ms is not on the published board (see the compaction disclosure) — this diagnostic projection rides the adjacent lane and its published values count only those 1.903ms"
	// HMC377 round 11 (register R10T-4): check reports on the subtest's own t
	// (the round-10 closure captured the parent t, so a failing subtest was
	// reported as passing — review-pins/mutant/tq_RM5.log).
	check := func(t *testing.T, name string, sat RootCauseRankItem, want string) {
		t.Helper()
		if sat.ChainAnchorRepresentedByChainSeat {
			t.Errorf("%s: a partly resolved sentence does not represent the row whole", name)
		}
		if got := hmc377R10Held(sat); got < 0.9995 || got > 1.0005 {
			t.Errorf("%s: held share %.3f, want the 1.000 its unwalked seat counts", name, got)
		}
		for label, v := range map[string]float64{"runnable": sat.RunnableMs, "impact": sat.ImpactMs, "projected": sat.ProjectedImpactMs,
			"cumulative": sat.CumulativeImpactMs, "effective": sat.EffectiveImpactMs} {
			if v < 1.9025 || v > 1.9035 {
				t.Errorf("%s: published %s %.4f, want 2.903 less the 1.000 counted at the unwalked seat", name, label, v)
			}
		}
		if !strings.Contains(sat.Summary, want) {
			t.Errorf("%s: summary %q\nwant the partial form %q", name, sat.Summary, want)
		}
		if c := sat.ShareClaims[1]; !c.Resolved || !reflect.DeepEqual(c.HolderKeys, []string{"seat-3"}) && !reflect.DeepEqual(c.HolderKeys, []string{"seat-2"}) {
			t.Errorf("%s: the unwalked claim stays resolved to its seat: %+v", name, c)
		}
	}
	t.Run("gate withdrawal", func(t *testing.T) {
		board := hmc377R10U2Board(false)
		if !board[3].ChainAnchorRepresentedByChainSeat || board[3].EffectiveImpactMs != 2.903 {
			t.Fatalf("precondition: the engine represents the satellite whole")
		}
		published := hmc377R10Copy(board)
		hmc377R10Withdraw(published, nil, []bool{false, true, false, false}, nil)
		check(t, "withdrawn", published[3], partialWithdrawn)
		if board[3].EffectiveImpactMs != 2.903 || !board[3].ChainAnchorRepresentedByChainSeat {
			t.Errorf("publication mutated the engine row")
		}
		again := hmc377R10Copy(published)
		hmc377R10Withdraw(again, nil, []bool{false, true, false, false}, nil)
		if !reflect.DeepEqual(again, published) {
			t.Errorf("the withdrawal is not idempotent:\n%+v\n%+v", again[3], published[3])
		}
	})
	t.Run("engine cap cut", func(t *testing.T) {
		board := hmc377R10U2Board(true)
		check(t, "cut", board[2], partialCut)
	})
	t.Run("nothing resolves: own value and the unchanged compaction form", func(t *testing.T) {
		board := hmc377R10U2Board(true)
		published := hmc377R10Copy(board)
		hmc377R10Withdraw(published, nil, []bool{false, true, false}, nil)
		sat := published[2]
		if sat.ChainAnchorRepresentedByChainSeat || hmc377R10Held(sat) != 0 || sat.EffectiveImpactMs != 2.903 || sat.RunnableMs != 2.903 {
			t.Errorf("an unresolved sentence keeps the satellite's own account: marker=%t held=%.3f eff=%.3f run=%.3f",
				sat.ChainAnchorRepresentedByChainSeat, hmc377R10Held(sat), sat.EffectiveImpactMs, sat.RunnableMs)
		}
		if !strings.Contains(sat.Summary, "; this satellite's whole account (2.903ms of 2.903ms) lies inside the thread's dependency windows, but the seat holding that time is not on the published board (see the compaction disclosure) — this diagnostic projection keeps its own value on the adjacent lane") {
			t.Errorf("the cap-cut compaction wording changed: %q", sat.Summary)
		}
	})
}

func TestHMC377R10WithdrawnSeatHoldsNothingAndClaimsNothing(t *testing.T) {
	// U5: the withdrawn inversion seat holds the share of a represented
	// satellite AND of a ◇ remainder; another thread's remainder names a
	// published holder (seat-1).
	other := hmc377R10OtherSeat()
	inv := hmc377R10Inversion()
	// F5: the withdrawn seat itself carries its split stamp and claims its ◇
	// remainder twin.
	inv.ChainAnchoredMs, inv.ChainAnchorFullMs = 1.903, 6.903
	rspaSetSentence(&inv, rspaAnchoredSentence(hmc377R10Dep, "runnable", 1.903, 6.903,
		rspaClaim(RootCauseShareClaimRemainder, RootCauseShareHolderRemainderSeat, hmc377R10Dep.PID, false, 5.0, []foldInterval{{start: 10.5, end: 10.505}})))
	sat := hmc377R10Satellite(1.903)
	rspaAppendSentence(&sat, rspaRepresentedDemotionSummary(1.903, 1.903,
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R10Dep.PID, false, 1.903, hmc377R10Walked())))
	rem := RootCauseRankItem{Rank: 5, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R10Dep,
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", DominantState: string(StateRunnable), RunnableMs: 5.0, EffectiveImpactMs: 5.0,
		ChainAnchoredMs: 1.903, ChainAnchorFullMs: 6.903, ChainAnchorRemainderSeat: true, LineStart: 60, LineEnd: 90}
	rspaSetSentence(&rem, rspaRemainderSentence(hmc377R10Dep, "runnable (scheduling-pressure candidate)", 5.0, 1.903, 6.903,
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R10Dep.PID, false, 1.903, hmc377R10Walked())))
	rspaHold(&rem, RootCauseShareHolderRemainderSeat, false, []foldInterval{{start: 10.5, end: 10.505}}, 5.0)
	otherRem := RootCauseRankItem{Rank: 6, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R10Other,
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", DominantState: string(StateRunnable), RunnableMs: 4.0, EffectiveImpactMs: 4.0,
		ChainAnchoredMs: 30, ChainAnchorFullMs: 34, ChainAnchorRemainderSeat: true, LineStart: 100, LineEnd: 120}
	rspaSetSentence(&otherRem, rspaRemainderSentence(hmc377R10Other, "runnable (scheduling-pressure candidate)", 4.0, 30, 34,
		rspaClaim(RootCauseShareClaimAnchored, RootCauseShareHolderChainSeat, hmc377R10Other.PID, false, 30, []foldInterval{{start: 10.1, end: 10.13}})))
	board := []RootCauseRankItem{other, inv, sat, rem, otherRem}
	resolveRankShareClaims(board, nil)
	if !board[2].ShareClaims[0].Resolved || !board[3].ShareClaims[0].Resolved || !board[1].ShareClaims[0].Resolved || board[3].ShareHolderKey != "seat-4" {
		t.Fatalf("precondition: the engine resolves every claim (inversion seat's twin claim on the remainder seat-4)")
	}
	published := hmc377R10Copy(board)
	hmc377R10Withdraw(published, nil, []bool{false, true, false, false, false}, nil)
	pInv, pSat, pRem, pOtherRem := published[1], published[2], published[3], published[4]
	if len(pInv.ShareClaims) != 0 || pInv.ChainAnchoredMs != 0 || pInv.ChainAnchorFullMs != 0 || pInv.ShareHolderKey != "" {
		t.Errorf("F5: the withdrawn seat still claims or prints its split: claims=%+v anchored=%.3f full=%.3f key=%q",
			pInv.ShareClaims, pInv.ChainAnchoredMs, pInv.ChainAnchorFullMs, pInv.ShareHolderKey)
	}
	// EVOLUTION RECORD (HMC377 round 11, batch-owner ruling R-377-11 ①,
	// 2026-11-21): old → new. Old (round 10): both claimants of the withdrawn
	// seat's walked 1.903 ms kept their own value unresolved — the satellite
	// 1.903 on ◎, the ◇ remainder 5.000 — and the remainder held no key.
	// New: the withdrawn segments pass to ONE published successor in typed
	// order, and the ◇ row's anchored claim precedes the claimant itself: the
	// remainder carries its whole 6.903 (its span says it counts the share),
	// the satellite's claim resolves to the remainder (represented whole by a
	// row that is not a chain seat — the successor form) and the remainder's
	// key now names it as that claim's holder, not as the withdrawn seat's
	// twin. Σ = raw 6.903 with one carrier either way; the order is the
	// ruling's (review 10 R10H-3, ~/hmc-work/hmc377/fix10/review-holders/univ/
	// faces/head_UR4rem_w.zh.f shows the round-10 Σ5.000 of raw 6.903 for the
	// remainder-only shape).
	if pRem.ShareHolderKey != "seat-4" || pInv.ShareHolderKey != "" {
		t.Errorf("F5: keys: remainder %q (want seat-4, the satellite claim's successor holder), withdrawn %q", pRem.ShareHolderKey, pInv.ShareHolderKey)
	}
	if c := pRem.ShareClaims[0]; c.Resolved || len(c.HolderKeys) != 0 || !near(c.ReturnedMs, 1.903, 0.0005) {
		t.Errorf("U5 remainder: the withdrawn seat's segments pass back to it: %+v", c)
	}
	if !near(pRem.RunnableMs, 6.903, 0.0005) || !near(pRem.EffectiveImpactMs, 6.903, 0.0005) || !near(pRem.ShareReturnedMs, 1.903, 0.0005) {
		t.Errorf("U5: the remainder carries its whole account: run=%.3f eff=%.3f returned=%.3f", pRem.RunnableMs, pRem.EffectiveImpactMs, pRem.ShareReturnedMs)
	}
	if c := pSat.ShareClaims[0]; !c.Resolved || !c.Successor || !reflect.DeepEqual(c.HolderKeys, []string{"seat-4"}) {
		t.Errorf("U5 satellite: the claim resolves to the successor: %+v", c)
	}
	if !pSat.ChainAnchorRepresentedByChainSeat || pSat.EffectiveImpactMs != 1.903 {
		t.Errorf("U5: the satellite is represented whole by the remainder: marker=%t eff=%.3f", pSat.ChainAnchorRepresentedByChainSeat, pSat.EffectiveImpactMs)
	}
	// F4 (R-377-10 ④): the holding row is published only as context — not
	// absent, not owned by the chain seat; round 11 names who counts it.
	if !strings.Contains(pRem.Summary, "anchored inside typed dependency windows (the owning row is published only as context, tier context_only, so this row counts that share with its own value) + this remainder") ||
		strings.Contains(pRem.Summary, "not on the published board") || strings.Contains(pRem.Summary, "(owned by the chain seat)") {
		t.Errorf("F4: remainder summary %q", pRem.Summary)
	}
	if !strings.Contains(pSat.Summary, "; anchored share counted by another row of this thread: this satellite's whole account (1.903ms of 1.903ms) lies inside the thread's dependency windows; the row that held that time is published only as context, tier context_only, and another row of this thread on the published board counts it") ||
		strings.Contains(pSat.Summary, "represented by the chain seat") {
		t.Errorf("F4: satellite summary %q", pSat.Summary)
	}
	if c := pOtherRem.ShareClaims[0]; !c.Resolved || !reflect.DeepEqual(c.HolderKeys, []string{"seat-1"}) || published[0].ShareHolderKey != "seat-1" {
		t.Errorf("U5: the published holder's key and the claim naming it must survive: %+v key=%q", c, published[0].ShareHolderKey)
	}
	if !strings.Contains(pOtherRem.Summary, "(owned by the chain seat)") {
		t.Errorf("U5: the published holder keeps the held form: %q", pOtherRem.Summary)
	}
	if !board[1].ShareClaims[0].Resolved || board[1].ChainAnchorFullMs != 6.903 || board[3].ShareHolderKey != "seat-4" {
		t.Errorf("publication mutated the engine rows")
	}
}

// EVOLUTION RECORD (HMC377 round 11, batch-owner ruling R-377-11 ①,
// 2026-11-21): old → new. Old (round 10): the pair was hand-built (summary
// strings, GatedShare* fields, no claim) and the withdrawal was decided by
// line span (levelMergeWithdrawClaimSeats). New: the LEVELMERGE-1 relation is
// a typed claim of the one resolver, so the pair is built by the engine split
// itself; the constituent takes back the carved share as the withdrawn seat's
// first successor and the residual's claim resolves to it — the residual's
// sentence now names "another row of this thread on the published board"
// (round 10: "this thread's split constituent row", which the one resolver
// cannot prove for every successor, e.g. a ◇ remainder when the constituent is
// cut). The constituent bytes are unchanged.
func TestHMC377R10GatedShareSplitWithdrawnClaimSeat(t *testing.T) {
	mk := func(second bool) []RootCauseRankItem {
		inv := hmc377R10Inversion()
		agg := RootCauseRankItem{Rank: 4, Tier: "secondary", Type: "runnable_wait", Source: "wakeup_chain.aggregated_impacts", Thread: hmc377R10Dep,
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", DominantState: string(StateRunnable), ChainDepth: 2, Confidence: 0.8,
			RunnableMs: 3.0, ImpactMs: 3.0, ProjectedImpactMs: 3.0, CumulativeImpactMs: 3.0, EffectiveImpactMs: 3.0, LineStart: 50, LineEnd: 60,
			StartTs: 10.2, EndTs: 10.401097,
			OccurrenceWindows: []WakeupCausalOccurrence{{Window: TimeWindow{StartTs: 10.2, EndTs: 10.201903}}, {Window: TimeWindow{StartTs: 10.4, EndTs: 10.401097}}}}
		inv.StartTs, inv.EndTs = 10.2, 10.201903
		items := []RootCauseRankItem{hmc377R10OtherSeat(), inv}
		if second {
			inv2 := inv
			inv2.Rank, inv2.LineStart, inv2.LineEnd = 3, 70, 80
			items = append(items, inv2)
		}
		items = splitAggregateGatedRunnableShare(ChainResult{Target: ThreadRef{PID: 100}}, append(items, agg))
		resolveRankShareClaims(items, nil)
		return items
	}
	board := mk(false)
	pb, pa := board[2], board[3]
	if !pa.GatedShareConstituentSeat || pb.GatedShareFullMs != 3.0 || !pa.ShareClaims[0].Resolved || !pb.ShareClaims[0].Resolved {
		t.Fatalf("precondition: the engine split resolves both claims to the inversion seat: %+v / %+v", pb.ShareClaims, pa.ShareClaims)
	}
	if pb.Summary != levelMergeResidualSentenceForTest(pb) {
		t.Errorf("the residual's mint sentence changed: %q", pb.Summary)
	}
	published := hmc377R10Copy(board)
	hmc377R10Withdraw(published, nil, []bool{false, true, false, false}, nil)
	pb, pa = published[2], published[3]
	if !hmc377R10ClaimWithdrawn(pb) || !hmc377R10ClaimWithdrawn(pa) {
		t.Errorf("the split whose claim seat is withdrawn must carry the typed decision on both rows")
	}
	if pa.Summary != "binder:458_5-6326 runnable share 1.903ms of the 3.000ms aggregate account overlapping the same thread's priority-inversion seat branch window(s) (branch-window overlap measure) (the claiming inversion seat is published only as context, tier context_only) — constituent share: it rides the adjacent lane and counts this share with its own value" {
		t.Errorf("constituent summary %q", pa.Summary)
	}
	if pb.Summary != "binder:458_5-6326 runnable aggregate residual 1.097ms after the interval-accounting split: 1.903ms of the 3.000ms scheduling-demand account lies inside the same thread's priority-inversion seat branch window(s) and is counted by another row of this thread on the published board (the claiming inversion seat is published only as context, tier context_only) — claimed + residual = full account (one segment set, two disjoint shares, additive back)" {
		t.Errorf("residual summary %q", pb.Summary)
	}
	if c := pb.ShareClaims[0]; !c.Resolved || !c.Successor || len(c.HolderKeys) != 1 || c.HolderKeys[0] != pa.ShareHolderKey {
		t.Errorf("the residual's claim resolves to the constituent (successor): %+v key=%q", c, pa.ShareHolderKey)
	}
	if !near(pa.EffectiveImpactMs, 1.903, 0.0005) || !near(pb.EffectiveImpactMs, 1.097, 0.0005) {
		t.Errorf("values: constituent %.3f residual %.3f (Σ = raw 3.000)", pa.EffectiveImpactMs, pb.EffectiveImpactMs)
	}
	if hmc377R10ClaimWithdrawn(board[2]) || board[2].Summary != levelMergeResidualSentenceForTest(board[2]) {
		t.Errorf("publication mutated the engine rows")
	}
	// One of two claim seats withdrawn: the other still holds the carved
	// share — the split keeps its form (no typed decision).
	partial := mk(true)
	pp := hmc377R10Copy(partial)
	hmc377R10Withdraw(pp, nil, []bool{false, true, false, false, false}, nil)
	if hmc377R10ClaimWithdrawn(pp[3]) || hmc377R10ClaimWithdrawn(pp[4]) || pp[4].Summary != partial[4].Summary || pp[3].Summary != partial[3].Summary {
		t.Errorf("a split with a published claim seat keeps its form:\n%q\n%q", pp[3].Summary, pp[4].Summary)
	}
}

// levelMergeResidualSentenceForTest is the residual's mint sentence at
// b71f82049 (literal bytes; the composer must reproduce them).
func levelMergeResidualSentenceForTest(b RootCauseRankItem) string {
	return "binder:458_5-6326 runnable aggregate residual 1.097ms after the interval-accounting split: 1.903ms of the 3.000ms scheduling-demand account lies inside the same thread's priority-inversion seat branch window(s) and is already counted by that seat's gated composite (the claiming inversion seat is published on this board) — claimed + residual = full account (one segment set, two disjoint shares, additive back)"
}
