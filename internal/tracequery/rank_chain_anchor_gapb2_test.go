package tracequery

import (
	"math"
	"strings"
	"testing"
)

// rank_chain_anchor_gapb2_test.go — S14-A / AUD-03 (§14.4, colleague audit
// 2026-07-25) pins: when the RSPA one-seat closure suppresses a PERIODIC
// D∧timer chain seat (identityHolds), the canonical single window owner must
// inherit the discount and the timer credential — otherwise the µs-identical
// wall clock already proven normal timer cadence resurrects at FULL value on
// the final board (the "值通道正确、canonical owner 错" escape).

func gapB2RSPAFixture(t *testing.T) (ChainResult, WindowStats, []RootCauseRankItem) {
	t.Helper()
	chain := buildPeriodicTimerChain(gapB2TimerIntervalsMs, gapB2TimerDStateMs, gapB2TimerRunnableMs,
		[]string{"timerfd_read+0x74/0x120"})
	chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
	if len(chain.AggregatedImpacts) != 1 || !chain.AggregatedImpacts[0].PeriodicTimerWait {
		t.Fatalf("fixture precondition: the D∧timer aggregate must be periodic: %+v", chain.AggregatedImpacts)
	}
	agg := chain.AggregatedImpacts[0]
	fullD := agg.DStateMs
	stats := WindowStats{
		chainAnchorsByPID:      chainAnchorWindowsByPID(chain),
		offCPUProducerDisjoint: true,
		dstateCensus: map[string]ThreadDuration{
			"610|0": {Thread: ThreadRef{PID: 610, Comm: "TimerDispatcher"}, DurationMs: fullD, anchoredMs: fullD},
		},
	}
	// The pid's ONE window_stats D seat, fully anchored (Case B): the RSPA
	// mint-time closure already suppressed the chain periodic seat, so items
	// carry only the window owner (production shape).
	items := []RootCauseRankItem{{
		Thread: ThreadRef{PID: 610, Comm: "TimerDispatcher"}, Type: "d_state_or_io_wait",
		Source: "window_stats", DStateMs: fullD, CumulativeImpactMs: fullD, ImpactMs: fullD,
		DominantState: string(StateDSleep),
		// Production shape: window seats of chain-present pids carry the
		// enriched chain relevance (chainContextForCandidate) — the reanchor
		// pass only visits on-chain rows.
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
		ledgerAnchorStamped: true, ledgerAnchoredDMs: fullD,
	}}
	return chain, stats, items
}

func TestRSPACaseBWindowOwnerInheritsPeriodicTimerDiscount(t *testing.T) {
	chain, stats, items := gapB2RSPAFixture(t)
	// Fixture sanity: the decision must mint with the case-A identity (the
	// suppression premise) — otherwise this test pins nothing.
	_, dio := buildRSPAFamilyDecisions(chain, stats)
	decision, ok := dio[610]
	if !ok || !decision.migrate || !decision.identityHolds {
		t.Fatalf("fixture precondition: identity-holding DIO decision must mint: %+v", dio)
	}
	out := reanchorOnChainStateSeats(chain, stats, items)
	var owner *RootCauseRankItem
	for i := range out {
		if out[i].Thread.PID == 610 && out[i].Source == "window_stats" {
			owner = &out[i]
		}
	}
	if owner == nil {
		t.Fatalf("the window owner must survive: %+v", out)
	}
	if !owner.PeriodicSource || !owner.PeriodicTimerWait || owner.PeriodicTimerCaller != "timerfd_read" {
		t.Fatalf("AUD-03: the canonical owner must inherit the periodic timer credential: %+v", owner)
	}
	if !near(owner.EffectiveImpactMs, chain.AggregatedImpacts[0].EffectivePeriodicImpactMs, 0.0001) {
		t.Fatalf("AUD-03: the owner's effective must be the DISCOUNTED chain value (%.6f), got %.6f",
			chain.AggregatedImpacts[0].EffectivePeriodicImpactMs, owner.EffectiveImpactMs)
	}
	if !near(owner.DStateMs, chain.AggregatedImpacts[0].DStateMs, 0.001) {
		t.Fatalf("raw D account stays lossless on the owner: %+v", owner)
	}
	// 复核修 (wf_8fe3fe39 finding #5): §7.30 S1 — the sort key re-derives
	// with the published value; a stale full-value Score beside the
	// discounted effective is the q4 rank1-score dissonance class.
	wantScore := owner.EffectiveImpactMs * owner.Confidence * rootCauseItemScoreWeight(*owner)
	if !near(owner.Score, wantScore, 0.0001) {
		t.Fatalf("the owner's Score must re-derive from the discounted effective: got %.6f want %.6f", owner.Score, wantScore)
	}
}

func TestRSPADiscountTransferFailsClosedOnSigmaMismatch(t *testing.T) {
	chain, stats, items := gapB2RSPAFixture(t)
	// 不等值 fail-close: the owner's own D account diverges from the chain
	// periodic account beyond tolerance — no transfer (the frozen §14.4
	// direction forbids copying a discount onto a non-identical account).
	items[0].DStateMs += 5.0
	items[0].ledgerAnchoredDMs = items[0].DStateMs
	stats.dstateCensus["610|0"] = ThreadDuration{
		Thread:     ThreadRef{PID: 610, Comm: "TimerDispatcher"},
		DurationMs: items[0].DStateMs, anchoredMs: items[0].DStateMs,
	}
	out := reanchorOnChainStateSeats(chain, stats, items)
	for i := range out {
		if out[i].Thread.PID == 610 && out[i].Source == "window_stats" && out[i].PeriodicSource {
			t.Fatalf("Σ mismatch must fail the transfer closed: %+v", out[i])
		}
	}
	// No resurrection either (S14-A2): the Σ divergence breaks the decision
	// identity, so the mint-time suppression never fired and the chain seat
	// is still alive in production carrying its own discount — resurrecting
	// here would double-mint it.
	for i := range out {
		if out[i].Thread.PID == 610 && out[i].Source != "window_stats" && out[i].PeriodicSource {
			t.Fatalf("divergent-identity pids must not resurrect (chain seat never suppressed): %+v", out[i])
		}
	}
}

func TestRSPADiscountTransferFailsClosedOnMultiSeat(t *testing.T) {
	chain, stats, items := gapB2RSPAFixture(t)
	// 多分区 fail-close (S14-A2 / RSPA-MP1): two real partitions whose typed
	// ledger-anchor Σ equals the suppressed chain census. One aggregate
	// discount must never be COPIED onto them; the chain periodic seat
	// resurrects and B4 makes it the sole active value owner while retaining
	// both raw partitions losslessly.
	fullD := items[0].DStateMs
	firstD := fullD * 0.4
	secondD := fullD - firstD
	items[0].DStateMs = firstD
	items[0].CumulativeImpactMs = firstD
	items[0].ImpactMs = firstD
	items[0].EffectiveImpactMs = firstD
	items[0].ledgerAnchoredDMs = firstD
	second := items[0]
	second.DStateMs = secondD
	second.CumulativeImpactMs = secondD
	second.ImpactMs = secondD
	second.EffectiveImpactMs = secondD
	second.ledgerAnchoredDMs = secondD
	items = append(items, second)
	stats.dstateCensus = map[string]ThreadDuration{
		"610|0": {Thread: ThreadRef{PID: 610, Comm: "TimerDispatcher"}, DurationMs: firstD, anchoredMs: firstD},
		"610|1": {Thread: ThreadRef{PID: 610, Comm: "TimerDispatcher"}, DurationMs: secondD, anchoredMs: secondD},
	}
	out := reanchorOnChainStateSeats(chain, stats, items)
	var resurrected *RootCauseRankItem
	for i := range out {
		if out[i].Thread.PID != 610 {
			continue
		}
		if out[i].Source == "window_stats" {
			if out[i].PeriodicSource {
				t.Fatalf("window partition seats must not receive the discount copy: %+v", out[i])
			}
			continue
		}
		if out[i].PeriodicSource {
			resurrected = &out[i]
		}
	}
	if resurrected == nil {
		t.Fatalf("S14-A2: the chain periodic seat must resurrect on the multi-seat fail-close: %+v", out)
	}
	if !resurrected.PeriodicTimerWait || resurrected.PeriodicTimerCaller != "timerfd_read" {
		t.Fatalf("the resurrected seat carries the timer credential: %+v", resurrected)
	}
	if !near(resurrected.EffectiveImpactMs, chain.AggregatedImpacts[0].EffectivePeriodicImpactMs, 0.0001) {
		t.Fatalf("the resurrected seat publishes the DISCOUNTED value: %+v", resurrected)
	}
	if !resurrected.ChainAnchorOwnershipDivergent || resurrected.ChainAnchorCensusMs <= 0 {
		t.Fatalf("the resurrected seat must wear the typed dual-account disclosure: %+v", resurrected)
	}
	if !strings.Contains(resurrected.Summary, "dual account") {
		t.Fatalf("the dual-account clause must render: %q", resurrected.Summary)
	}
	// §15.12 批戊: at reanchor time the absorption is UNDECIDED — the
	// sole-owner claim may only be written by the committed reconciliation.
	if strings.Contains(resurrected.Summary, "sole rank-value owner") {
		t.Fatalf("reanchor must not pre-claim absorption success: %q", resurrected.Summary)
	}
	rank := RootCauseRankResult{
		Window: TimeWindow{StartTs: 1, EndTs: 2},
		Items:  out,
	}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 1 || !rank.Items[0].PeriodicSource {
		t.Fatalf("RSPA-MP1: only the discounted periodic owner may remain active: %+v", rank.Items)
	}
	if rank.Items[0].AbsorbedRankRows != 2 || rank.Items[0].RankFamilyKey == "" {
		t.Fatalf("the owner must disclose both absorbed raw partitions: %+v", rank.Items[0])
	}
	if !strings.Contains(rank.Items[0].Summary, "sole rank-value owner") ||
		!strings.Contains(rank.Items[0].Summary, "2 raw partition row(s) absorbed") {
		t.Fatalf("the COMMITTED absorption must claim sole ownership on the owner face: %q", rank.Items[0].Summary)
	}
	if len(rank.AbsorbedItems) != 2 {
		t.Fatalf("both raw partitions must remain losslessly visible: %+v", rank.AbsorbedItems)
	}
	absorbedSum := 0.0
	for _, detail := range rank.AbsorbedItems {
		if !detail.AbsorbedByRankFamily || detail.AbsorbedIntoFamily != rank.Items[0].RankFamilyKey ||
			detail.Tier != RootCauseTierAbsorbed || detail.Rank != 0 {
			t.Fatalf("absorbed partition must point at the unique owner: %+v", detail)
		}
		absorbedSum += detail.DStateMs + detail.IOWaitMs
	}
	if !rspaWithinTol(absorbedSum, rank.Items[0].ChainAnchorCensusMs) {
		t.Fatalf("lossless partition Σ %.6f must equal owner census %.6f", absorbedSum, rank.Items[0].ChainAnchorCensusMs)
	}
	normalizeRootCauseCumulativeImpact(rank.Items)
	normalizeRootCauseEffectiveImpact(rank.Items)
	sortRootCauseRankItems(rank.Items, true)
	if len(rank.Items) != 1 || !near(rank.Items[0].EffectiveImpactMs, chain.AggregatedImpacts[0].EffectivePeriodicImpactMs, 0.0001) {
		t.Fatalf("sort must see only the discounted periodic owner: %+v", rank.Items)
	}
	// Idempotence across the double reanchor pass: the resurrected seat's
	// PeriodicSource reads as carried — no second copy.
	out2 := reanchorOnChainStateSeats(chain, stats, out)
	periodicSeats := 0
	for i := range out2 {
		if out2[i].Thread.PID == 610 && out2[i].PeriodicSource {
			periodicSeats++
		}
	}
	if periodicSeats != 1 {
		t.Fatalf("the second reanchor pass must not mint a second periodic seat: %d", periodicSeats)
	}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 1 || len(rank.AbsorbedItems) != 2 || rank.Items[0].AbsorbedRankRows != 2 {
		t.Fatalf("B4 reset-first recomputation must be idempotent: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
}

func TestRSPAMultiPartitionAbsorptionFailsOpenOnCensusMismatch(t *testing.T) {
	chain, stats, items := gapB2RSPAFixture(t)
	fullD := items[0].DStateMs
	first := items[0]
	first.DStateMs = fullD * 0.4
	first.CumulativeImpactMs = first.DStateMs
	first.ImpactMs = first.DStateMs
	first.ledgerAnchoredDMs = first.DStateMs
	second := first
	second.DStateMs = fullD * 0.5 // raw Σ is deliberately 10% short.
	second.CumulativeImpactMs = second.DStateMs
	second.ImpactMs = second.DStateMs
	second.ledgerAnchoredDMs = second.DStateMs
	items = []RootCauseRankItem{first, second}
	stats.dstateCensus = map[string]ThreadDuration{
		"610|0": {Thread: first.Thread, DurationMs: fullD * 0.4, anchoredMs: fullD * 0.4},
		"610|1": {Thread: first.Thread, DurationMs: fullD * 0.6, anchoredMs: fullD * 0.6},
	}
	out := reanchorOnChainStateSeats(chain, stats, items)
	rank := RootCauseRankResult{Window: TimeWindow{StartTs: 1, EndTs: 2}, Items: out}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 3 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("raw Σ != owner census must keep the honest dual publication: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
	// §15.12 批戊: on the fail-open arm the raw rows are ACTIVE competing
	// seats — no face may claim they were absorbed or that any seat is the
	// sole rank-value owner.
	for _, item := range rank.Items {
		if strings.Contains(item.Summary, "sole rank-value owner") ||
			strings.Contains(item.Summary, "absorbed as lossless detail") {
			t.Fatalf("fail-open board must not carry an absorption-success claim: %q", item.Summary)
		}
	}
}

// §15.12 批戊 + X-cross P3: the per-row proof arms fail open too — one raw
// partition missing its ledger-anchor stamp (or only partially anchored)
// poisons the pid's whole absorption, keeping the honest dual publication.
func TestRSPAMultiPartitionAbsorptionFailsOpenOnRowProof(t *testing.T) {
	build := func(mutate func(*RootCauseRankItem)) RootCauseRankResult {
		chain, stats, items := gapB2RSPAFixture(t)
		fullD := items[0].DStateMs
		first := items[0]
		first.DStateMs = fullD * 0.4
		first.CumulativeImpactMs = first.DStateMs
		first.ImpactMs = first.DStateMs
		first.ledgerAnchoredDMs = first.DStateMs
		second := first
		second.DStateMs = fullD - first.DStateMs
		second.CumulativeImpactMs = second.DStateMs
		second.ImpactMs = second.DStateMs
		second.ledgerAnchoredDMs = second.DStateMs
		mutate(&second)
		items = []RootCauseRankItem{first, second}
		stats.dstateCensus = map[string]ThreadDuration{
			"610|0": {Thread: first.Thread, DurationMs: first.DStateMs, anchoredMs: first.DStateMs},
			"610|1": {Thread: first.Thread, DurationMs: second.DStateMs, anchoredMs: second.DStateMs},
		}
		out := reanchorOnChainStateSeats(chain, stats, items)
		rank := RootCauseRankResult{Window: TimeWindow{StartTs: 1, EndTs: 2}, Items: out}
		reconcileExactCrossTypeRankSeats(&rank)
		return rank
	}
	arms := map[string]func(*RootCauseRankItem){
		"missing stamp":    func(item *RootCauseRankItem) { item.ledgerAnchorStamped = false },
		"partial anchored": func(item *RootCauseRankItem) { item.ledgerAnchoredDMs = item.DStateMs * 0.5 },
	}
	for name, mutate := range arms {
		rank := build(mutate)
		// The poisoned row may legitimately spawn a remainder seat via the
		// anchored-decomposition lane — the judgment is ZERO absorption
		// with every raw account still active.
		if len(rank.Items) < 3 || len(rank.AbsorbedItems) != 0 {
			t.Fatalf("%s: one poisoned row must fail the pid's whole absorption open: active=%d absorbed=%d",
				name, len(rank.Items), len(rank.AbsorbedItems))
		}
		for _, item := range rank.Items {
			if strings.Contains(item.Summary, "sole rank-value owner") {
				t.Fatalf("%s: fail-open board must not claim sole ownership: %q", name, item.Summary)
			}
		}
	}
}

// TestApplyPacingTimerDiscountsSubMinOccurrenceShape — S14-A / AUD-04(a)
// (§14.5): a d_sleep timer wait below wakeupPeriodicMinOccurrences never
// reaches the VS-1 detector, but its pacing verdict (waker-side typed VS-1
// period + segment fit + timer caller) must reach the same physical impact's
// canonical rank value — a periodic_idle context row beside a full-value D
// candidate is a self-contradicting board.
func TestApplyPacingTimerDiscountsSubMinOccurrenceShape(t *testing.T) {
	chain := buildPeriodicTimerChain(
		gapB2TimerIntervalsMs[:2], gapB2TimerDStateMs[:3], gapB2TimerRunnableMs[:3],
		[]string{"timerfd_read+0x74/0x120"})
	chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
	if len(chain.AggregatedImpacts) != 1 || chain.AggregatedImpacts[0].PeriodicSource {
		t.Fatalf("fixture precondition: 3 occurrences stay below the VS-1 min-occurrence gate: %+v", chain.AggregatedImpacts)
	}
	period := 8.302
	for i := range chain.CausalImpacts {
		imp := chain.CausalImpacts[i]
		chain.PacingIdles = append(chain.PacingIdles, PacingIdleSummary{
			Thread:          imp.Thread,
			WindowStartTs:   imp.Window.StartTs,
			WindowEndTs:     imp.Window.EndTs,
			DurationMs:      imp.DStateMs,
			FramePeriodMs:   period,
			PeriodSource:    binderPacingPeriodSourceAggregate,
			Kind:            binderIdleKindPeriodic,
			TimerWaitCaller: "timerfd_read",
		})
	}
	applyPacingTimerDiscounts(&chain)
	for i, imp := range chain.CausalImpacts {
		if !imp.PeriodicSource || !imp.PeriodicTimerWait {
			t.Fatalf("member %d must carry the pacing-credential discount: %+v", i, imp)
		}
		wantLateness := math.Max(0, imp.TargetBlockedMs-period)
		if !near(imp.LatenessMs, wantLateness, 0.0001) {
			t.Fatalf("member %d lateness must be the blocked caliber: got %.6f want %.6f", i, imp.LatenessMs, wantLateness)
		}
		if !strings.Contains(imp.Summary, "periodic_source=true") || !strings.Contains(imp.Summary, "timer_wait_caller=timerfd_read") {
			t.Fatalf("member %d summary must republish the discount: %q", i, imp.Summary)
		}
	}
	agg := chain.AggregatedImpacts[0]
	if !agg.PeriodicSource || !agg.PeriodicTimerWait || agg.PeriodicTimerCaller != "timerfd_read" {
		t.Fatalf("the sub-min aggregate must be reconciled from its fully-stamped members: %+v", agg)
	}
	if !near(agg.DStateMs, gapB2TimerDStateMs[0]+gapB2TimerDStateMs[1]+gapB2TimerDStateMs[2], 0.001) {
		t.Fatalf("raw aggregate account stays lossless: %+v", agg)
	}
	if agg.EffectivePeriodicImpactMs >= aggregateBlockingMs(agg) {
		t.Fatalf("the aggregate effective must be the discounted value: %+v", agg)
	}
}

// TestApplyPacingTimerDiscountsFailClosedArms: frame-chain two-tick cadence
// (period source 2) is NOT a discount authority, and one unstamped member
// fails the whole aggregate reconcile closed.
func TestApplyPacingTimerDiscountsFailClosedArms(t *testing.T) {
	chain := buildPeriodicTimerChain(
		gapB2TimerIntervalsMs[:2], gapB2TimerDStateMs[:3], gapB2TimerRunnableMs[:3],
		[]string{"timerfd_read+0x74/0x120"})
	chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
	// Cadence-source verdict: no discount (weaker evidence tier).
	chain.PacingIdles = []PacingIdleSummary{{
		Thread:          chain.CausalImpacts[0].Thread,
		WindowStartTs:   chain.CausalImpacts[0].Window.StartTs,
		WindowEndTs:     chain.CausalImpacts[0].Window.EndTs,
		FramePeriodMs:   8.302,
		PeriodSource:    binderPacingPeriodSourceCadence,
		TimerWaitCaller: "timerfd_read",
	}}
	applyPacingTimerDiscounts(&chain)
	if chain.CausalImpacts[0].PeriodicSource {
		t.Fatalf("two-tick cadence evidence must not mint the discount: %+v", chain.CausalImpacts[0])
	}
	// One member stamped, the others not → aggregate stays undiscounted.
	chain.PacingIdles[0].PeriodSource = binderPacingPeriodSourceAggregate
	applyPacingTimerDiscounts(&chain)
	if !chain.CausalImpacts[0].PeriodicSource {
		t.Fatalf("the verdict-carrying member itself is discounted: %+v", chain.CausalImpacts[0])
	}
	if chain.AggregatedImpacts[0].PeriodicSource {
		t.Fatalf("an aggregate with unstamped members must stay undiscounted (fail-close): %+v", chain.AggregatedImpacts[0])
	}
}

// TestPacingIdleIOWaitNodeNeverEligible — AUD-04(b): io_wait segments never
// reach the idle-cadence lane, not even through the legacy binder write-off
// route (single-source D admission; S keeps the direct route).
func TestPacingIdleIOWaitNodeNeverEligible(t *testing.T) {
	chain := buildPeriodicTimerChain(gapB2TimerIntervalsMs, gapB2TimerDStateMs, gapB2TimerRunnableMs,
		[]string{"timerfd_read+0x74/0x120"})
	for i := range chain.CausalImpacts {
		chain.CausalImpacts[i].DominantState = string(StateIOWait)
		chain.CausalImpacts[i].IOWaitMs = chain.CausalImpacts[i].DStateMs
		chain.CausalImpacts[i].DStateMs = 0
	}
	for i := range chain.Nodes {
		chain.Nodes[i].Dominant = StateIOWait
	}
	chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
	waker := ThreadRef{PID: 777, Comm: "TimerTick"}
	chain.AggregatedImpacts = append(chain.AggregatedImpacts, WakeupCausalAggregate{
		Thread: waker, PeriodicSource: true, DetectedPeriodMs: 8.302,
	})
	for i := range chain.Nodes {
		chain.Nodes[i].DurationMs = 8.302
		chain.Edges = append(chain.Edges, WakeupEdge{
			Waker: waker, Wakee: chain.Nodes[i].Thread,
			WakeupTs: chain.Nodes[i].Window.EndTs, WakeupLine: 900 + i,
		})
	}
	_, pacing, _ := findBinderWaitsForChain(&Index{}, chain, nil, nil)
	if len(pacing) != 0 {
		t.Fatalf("io_wait nodes must never mint idle-cadence rows: %+v", pacing)
	}
}

// TestPacingWriteOffBypassClosedForDState — 复核修 (wf_8fe3fe39 finding #9,
// 2026-07-25): the earlier io_wait negative ran with edges=nil, so
// rejectedTxns never armed and the test had ZERO judgment power against the
// legacy write-off bypass (empirically: re-adding the bypass stayed green).
// This pin arms the REAL write-off machinery — the donghu P9 fixture whose
// txn 12145963 is written off (reply completed before the segment) — and
// flips the sleeper's segment to D state with NO timer blocked_reason row:
// pre-AUD-04(b) the write-off route minted the pacing row for this D
// segment; now a D segment without the timer credential stays off the idle
// lane even with armed write-offs.
func TestPacingWriteOffBypassClosedForDState(t *testing.T) {
	trace := strings.Replace(donghuP9FalseAttributionTrace,
		"13762.992415: sched_switch: prev_comm=.ugc.aweme.lite prev_pid=17267 prev_prio=53 prev_state=S",
		"13762.992415: sched_switch: prev_comm=.ugc.aweme.lite prev_pid=17267 prev_prio=53 prev_state=D", 1)
	if trace == donghuP9FalseAttributionTrace {
		t.Fatal("fixture surgery failed: sleeper switch-out line not found")
	}
	idx := buildTraceIndex(t, "donghu_p9_dstate_bypass.ftrace", trace)
	q := Query{PID: 17267, TimeStart: 13762.894, TimeEnd: 13763.010, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 1}
	chain := BuildWakeupChain(idx, q)
	// Precondition: the write-off machinery is armed on this fixture (the
	// chain-level caveat proves rejectedTxns was non-empty for the segment).
	if !containsSubstring(chain.Caveats, "wrote off") {
		t.Fatalf("fixture precondition: the binder write-off must arm: %+v", chain.Caveats)
	}
	for _, p := range chain.PacingIdles {
		if p.Thread.PID == 17267 {
			t.Fatalf("a D segment without the timer credential must stay off the idle-cadence lane even with armed write-offs: %+v", p)
		}
	}
}

// TestApplyPacingTimerDiscountsLatenessAndCapArms — 复核修 (finding #10):
// the sub-min pin never exercised the lateness/cap arithmetic (every fixture
// wait sat below the period). One member's target wait exceeds the period —
// exact blocked-caliber lateness — and the aggregate rides the overlap-
// reconciled sum with the F1(c) cap still bounding.
func TestApplyPacingTimerDiscountsLatenessAndCapArms(t *testing.T) {
	chain := buildPeriodicTimerChain(
		gapB2TimerIntervalsMs[:2], gapB2TimerDStateMs[:3], gapB2TimerRunnableMs[:3],
		[]string{"timerfd_read+0x74/0x120"})
	chain.CausalImpacts[1].TargetBlockedMs = 12.0 // > period 8.302 → lateness 3.698
	chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
	period := 8.302
	for i := range chain.CausalImpacts {
		imp := chain.CausalImpacts[i]
		chain.PacingIdles = append(chain.PacingIdles, PacingIdleSummary{
			Thread: imp.Thread, WindowStartTs: imp.Window.StartTs, WindowEndTs: imp.Window.EndTs,
			FramePeriodMs: period, PeriodSource: binderPacingPeriodSourceAggregate,
			TimerWaitCaller: "timerfd_read",
		})
	}
	applyPacingTimerDiscounts(&chain)
	if !near(chain.CausalImpacts[1].LatenessMs, 12.0-period, 0.0001) {
		t.Fatalf("blocked-caliber lateness must bind: %+v", chain.CausalImpacts[1])
	}
	if !near(chain.CausalImpacts[0].LatenessMs, 0, 0.0001) {
		t.Fatalf("sub-period member carries zero lateness: %+v", chain.CausalImpacts[0])
	}
	agg := chain.rankAggregateCensus[0]
	if !agg.PeriodicSource || !near(agg.LatenessMs, 12.0-period, 0.0001) {
		t.Fatalf("aggregate lateness must be the reconciled disjoint sum (one late member): %+v", agg)
	}
	wantEff := agg.RunnableMs + (12.0 - period)
	if !near(agg.EffectivePeriodicImpactMs, wantEff, 0.0001) {
		t.Fatalf("aggregate effective must be runnable+lateness: got %.6f want %.6f", agg.EffectivePeriodicImpactMs, wantEff)
	}
}

// TestApplyPacingTimerDiscountsOverlapReconciledLateness — 复核修 (finding
// #1, runtime-reproduced): two branch projections of ONE physical wait
// (identical windows) must not double-count lateness — the aggregate rides
// the same overlap-cohort authority as the VS-1 lane.
func TestApplyPacingTimerDiscountsOverlapReconciledLateness(t *testing.T) {
	chain := ChainResult{Target: ThreadRef{PID: 4144, Comm: "main"}}
	period := 8.302
	for branch := 1; branch <= 2; branch++ {
		chain.CausalImpacts = append(chain.CausalImpacts, WakeupCausalImpact{
			Thread: ThreadRef{PID: 610, Comm: "TimerDispatcher"}, ChainDepth: 1, ChainBranch: branch,
			DominantState: string(StateDSleep), DominantImpactMs: 20, TotalMs: 20,
			DStateMs: 20, TargetBlockedMs: 20, FragmentCount: 1,
			Window:       TimeWindow{StartTs: 4520.1, EndTs: 4520.12},
			ActualWindow: TimeWindow{StartTs: 4520.1, EndTs: 4520.12},
			LineStart:    100, LineEnd: 105,
			DFamilyBlockedCaller: "timerfd_read+0x74/0x120",
		})
	}
	chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
	if len(chain.rankAggregateCensus) != 1 {
		t.Fatalf("fixture precondition: one aggregate: %+v", chain.rankAggregateCensus)
	}
	chain.PacingIdles = []PacingIdleSummary{{
		Thread:        ThreadRef{PID: 610, Comm: "TimerDispatcher"},
		WindowStartTs: 4520.1, WindowEndTs: 4520.12,
		FramePeriodMs: period, PeriodSource: binderPacingPeriodSourceAggregate,
		TimerWaitCaller: "timerfd_read",
	}}
	applyPacingTimerDiscounts(&chain)
	agg := chain.rankAggregateCensus[0]
	if !agg.PeriodicSource {
		t.Fatalf("both branch projections stamped → aggregate must reconcile: %+v", agg)
	}
	// One physical wait: lateness = 20 − 8.302 ONCE, never ×2 (nor clamped
	// silently to raw blocking, which would erase the discount).
	if !near(agg.LatenessMs, 20-period, 0.0001) {
		t.Fatalf("overlap cohorts must count one representative lateness: got %.6f want %.6f", agg.LatenessMs, 20-period)
	}
	if agg.EffectivePeriodicImpactMs >= aggregateBlockingMs(agg)-0.0001 {
		t.Fatalf("the discount must survive the overlap shape: %+v", agg)
	}
}

// TestApplyPacingTimerDiscountsCensusBeyondDisplayTrim — 复核修 (finding #2):
// the reconcile iterates the FULL rank census — a 9th+ aggregate (outside
// the AggregatedImpacts display trim) still gets its discount.
func TestApplyPacingTimerDiscountsCensusBeyondDisplayTrim(t *testing.T) {
	chain := buildPeriodicTimerChain(
		gapB2TimerIntervalsMs[:2], gapB2TimerDStateMs[:3], gapB2TimerRunnableMs[:3],
		[]string{"timerfd_read+0x74/0x120"})
	chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
	// Simulate the display trim dropping the group: the census keeps it,
	// the trimmed view does not (backing arrays diverge exactly like the
	// production out[:8] reslice on a 9th+ entry).
	chain.rankAggregateCensus = append([]WakeupCausalAggregate(nil), chain.AggregatedImpacts...)
	chain.AggregatedImpacts = nil
	period := 8.302
	for i := range chain.CausalImpacts {
		imp := chain.CausalImpacts[i]
		chain.PacingIdles = append(chain.PacingIdles, PacingIdleSummary{
			Thread: imp.Thread, WindowStartTs: imp.Window.StartTs, WindowEndTs: imp.Window.EndTs,
			FramePeriodMs: period, PeriodSource: binderPacingPeriodSourceAggregate,
			TimerWaitCaller: "timerfd_read",
		})
	}
	applyPacingTimerDiscounts(&chain)
	if !chain.rankAggregateCensus[0].PeriodicSource {
		t.Fatalf("the census entry beyond the display trim must still reconcile: %+v", chain.rankAggregateCensus[0])
	}
}

// TestApplyPacingTimerDiscountsMixedPeriodFailsClosed — 复核修 (finding #3):
// members paced by two different proven periods publish NO single-period
// aggregate claim (fail-close; member stamps stay).
func TestApplyPacingTimerDiscountsMixedPeriodFailsClosed(t *testing.T) {
	chain := buildPeriodicTimerChain(
		gapB2TimerIntervalsMs[:2], gapB2TimerDStateMs[:3], gapB2TimerRunnableMs[:3],
		[]string{"timerfd_read+0x74/0x120"})
	chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
	periods := []float64{8.302, 16.6, 8.302}
	for i := range chain.CausalImpacts {
		imp := chain.CausalImpacts[i]
		chain.PacingIdles = append(chain.PacingIdles, PacingIdleSummary{
			Thread: imp.Thread, WindowStartTs: imp.Window.StartTs, WindowEndTs: imp.Window.EndTs,
			FramePeriodMs: periods[i], PeriodSource: binderPacingPeriodSourceAggregate,
			TimerWaitCaller: "timerfd_read",
		})
	}
	applyPacingTimerDiscounts(&chain)
	for i, imp := range chain.CausalImpacts {
		if !imp.PeriodicSource {
			t.Fatalf("member %d keeps its own internally consistent stamp: %+v", i, imp)
		}
	}
	if chain.rankAggregateCensus[0].PeriodicSource {
		t.Fatalf("mixed-period group must not claim one aggregate cadence: %+v", chain.rankAggregateCensus[0])
	}
}
