package tracequery

// rank_chain_anchor_runsplit_test.go — RUNSPLIT-1 pin family (§29.209 user
// ruling, 2026-07-22; background = CHAINGUARD audit probe F2, §29.204.1 伴生洞
// F2: a chain member's runnable window seat crowned its FULL 36ms while the
// engine's own anchored overlap was 0.999ms).
//
// Mechanical-map verdict this file pins (评估后收窄实施, §29.209 出口):
//   件1/件2 — the anchored-share valuation + ◇ remainder counter-seat for the
//   ledger-covered runnable member family are ALREADY the shipped RSPA
//   bipartition (RNB-1 T1 ledger stamp + reanchorOnChainStateSeats runnable
//   arms, one wording single-point shared with the D/IO lane). The probe F2
//   shape is pinned end-to-end here so the form can never regress to the
//   full-window crown: ⛓ seat value = ledger anchored share (eff/sort follow
//   the true value, census credential tier = interval_proven via
//   ChainAnchoredMs>0), ◇ remainder twin = full − anchored on the adjacent
//   lane, OUT of the direction-conservation population, additive back to the
//   full account through the shared bipartition sentences.
//   件3 — a seat with NO usable ledger record keeps its FULL value with its
//   existing credential word (禁估算), and the fallback population count is
//   disclosed at the report level (rspaRunnableLedgerFallbackCaveat).

import (
	"math"
	"strings"
	"testing"
)

// runsplitF2ProbeTrace is the CHAINGUARD probe F2 topology (§29.204.1): the
// target app-100 sleeps [1.020, 1.0999] and is woken by worker-200 (the L1
// chain member; anchor window = the depth-1 node window [1.020, 1.0999]).
// Inside the anchor window worker-200 holds D 4ms [1.030, 1.034] plus 2ms of
// runnable ([1.021, 1.022] + [1.034, 1.035]); OUTSIDE it worker-200 is
// runnable [1.110, 1.145] = 35ms, woken by other-300 AFTER the target already
// resumed running. Census: full 37ms = 2ms anchored + 35ms remainder.
func runsplitF2ProbeTrace(t *testing.T) *Index {
	t.Helper()
	return buildTraceIndex(t, "runsplit_f2_probe.systrace", `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
     worker-200 (100) [002] .... 1.000500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.020000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.021000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.022000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.030000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.034000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.035000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.099900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 1.100000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.101000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
      other-300 (300) [003] .... 1.110000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.145000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.146000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.190000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
	`)
}

// TestRUNSPLITProbeF2FormBisectsToAnchoredShareAndRemainder — the §29.209 件1
// + 件2 regression pin on the probe F2 shape: the 36ms-full-crown form is
// extinct — the chain member's window runnable seat publishes the LEDGER
// anchored share on the ⛓ lane and its remainder rides the ◇ counter-seat.
func TestRUNSPLITProbeF2FormBisectsToAnchoredShareAndRemainder(t *testing.T) {
	idx := runsplitF2ProbeTrace(t)
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.2, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 16}
	chain := BuildWakeupChain(idx, q)
	anchors := chainAnchorWindowsByPID(chain)
	if len(anchors[200]) == 0 {
		t.Fatal("fixture drifted: worker-200 must hold a typed anchor window")
	}
	var anchorCeiling float64
	for _, w := range anchors[200] {
		anchorCeiling += (w.EndTs - w.StartTs) * 1000
	}
	rank := BuildRootCauseRank(idx, q)
	var clipped, remainder *RootCauseRankItem
	for i := range rank.Items {
		it := &rank.Items[i]
		if it.Thread.PID != 200 || it.Type != "runnable_wait" || it.Source != "window_stats" {
			continue
		}
		if it.ChainAnchorRemainderSeat {
			remainder = it
		} else {
			clipped = it
		}
	}
	if clipped == nil || remainder == nil {
		t.Fatalf("F2 form must publish the bipartition pair (⛓ anchored + ◇ remainder), got clipped=%v remainder=%v", clipped, remainder)
	}
	// 件1 — ⛓ seat value = the ledger anchored share; eff and the ordinal
	// follow the TRUE value; the census credential tier is the interval tier
	// (ChainAnchoredMs>0 arm), never a bare membership word.
	if math.Abs(clipped.RunnableMs-2.0) > rspaAnchorIdentityTolMs ||
		math.Abs(clipped.EffectiveImpactMs-2.0) > rspaAnchorIdentityTolMs {
		t.Fatalf("⛓ seat must publish the anchored share 2.000 (probe disease: full 37.000): %+v", clipped)
	}
	if !clipped.ledgerAnchorStamped || math.Abs(clipped.ledgerAnchoredRunnableMs-clipped.ChainAnchoredMs) > rspaAnchorIdentityTolMs {
		t.Fatalf("⛓ seat value must come from the RNB-1 T1 ledger stamp: %+v", clipped)
	}
	if clipped.ChainRelevance != "on_chain" || clipped.Rank <= 0 {
		t.Fatalf("⛓ seat must stay ranked on the chain lane: %+v", clipped)
	}
	if clipped.ChainCredentialCensus != RootCauseChainCredentialCensusIntervalProven {
		t.Fatalf("件1 census 凭证档随升: the anchored-share seat must carry interval_proven, got %q", clipped.ChainCredentialCensus)
	}
	if clipped.EffectiveImpactMs > anchorCeiling+rspaAnchorIdentityTolMs {
		t.Fatalf("⛓ seat exceeds its anchor-window ceiling %.3f: %+v", anchorCeiling, clipped)
	}
	// 件2 — ◇ remainder counter-seat: full − anchored on the adjacent lane,
	// additive back to the full account, OUT of the direction-conservation
	// population (条件可消上界不入守恒).
	if math.Abs(remainder.RunnableMs-35.0) > rspaAnchorIdentityTolMs ||
		math.Abs(remainder.EffectiveImpactMs-35.0) > rspaAnchorIdentityTolMs {
		t.Fatalf("◇ remainder must publish full − anchored = 35.000: %+v", remainder)
	}
	if remainder.ChainRelevance != "adjacent" || remainder.Causality != "adjacent_to_wakeup_chain" {
		t.Fatalf("◇ remainder must ride the adjacent lane: %+v", remainder)
	}
	if rootCauseItemDirectionPopulationEligible(remainder) {
		t.Fatalf("◇ remainder must stay OUT of the direction-conservation population: %+v", remainder)
	}
	if math.Abs((remainder.ChainAnchoredMs+remainder.RunnableMs)-remainder.ChainAnchorFullMs) > 2*rspaAnchorIdentityTolMs ||
		math.Abs(remainder.ChainAnchorFullMs-37.0) > rspaAnchorIdentityTolMs {
		t.Fatalf("bipartition must restore the full-window account (anchored %.3f + remainder %.3f = full %.3f): %+v",
			remainder.ChainAnchoredMs, remainder.RunnableMs, remainder.ChainAnchorFullMs, remainder)
	}
	// 件2 word face — both halves speak the SHARED bipartition sentences (the
	// E28/E38 word single-point: rspaAnchoredSummary / rspaRemainderSummary,
	// same emitters as the D/IO lane — 禁二抄).
	if !strings.Contains(clipped.Summary, "anchored inside its typed wakeup-dependency windows") ||
		!strings.Contains(clipped.Summary, rspaSummaryRemainderTwinPublished) {
		t.Fatalf("⛓ seat must speak the shared anchored-half sentence with the counter-seat pointer: %q", clipped.Summary)
	}
	if !strings.Contains(remainder.Summary, "outside its wakeup-dependency windows (no chain credential for these segments)") ||
		!strings.Contains(remainder.Summary, "additive back to the full account") {
		t.Fatalf("◇ remainder must speak the shared remainder-half sentence: %q", remainder.Summary)
	}
	// The probe disease predicate (RNB acceptance form): NO on-chain runnable
	// seat of the member family may publish more than the anchor ceiling
	// without carrying the bipartition decomposition.
	for _, it := range rank.Items {
		if it.Thread.PID != 200 || it.AbsorbedByRankFamily || !rootCauseItemIsOnChain(it) {
			continue
		}
		if it.DominantState != string(StateRunnable) {
			continue
		}
		if !strings.HasPrefix(it.Source, "wakeup_chain") && it.ChainAnchorFullMs == 0 &&
			it.EffectiveImpactMs > anchorCeiling+rspaAnchorIdentityTolMs {
			t.Errorf("F2 DISEASE: on-chain runnable seat publishes %.3fms full (> anchor ceiling %.3fms): type=%s src=%s",
				it.EffectiveImpactMs, anchorCeiling, it.Type, it.Source)
		}
	}
	// 件3 negative arm: a fully-ledgered board discloses NO fallback.
	if hasRunnableLedgerFallbackCaveat(rank.Caveats) {
		t.Fatalf("ledger-covered board must not mint the fallback disclosure: %v", rank.Caveats)
	}
}

// TestRUNSPLITLedgerFallbackDisclosureCounts — 件3 (§29.209 ③): seats with no
// usable ledger record keep FULL values (no estimated split is ever minted)
// and the population is disclosed once at the report level. Direct-fixture
// shapes (the RNB unit-pin style):
//
//	pid 500 — chain member with NO typed jump window (decision absent): its
//	         window seat + churn twin both walk the fallback;
//	pid 600 — stamp-less legacy seat whose value diverges from the census
//	         full (the dispatch default arm);
//	pid 200 — ledger-covered control: bisects, never counted.
func TestRUNSPLITLedgerFallbackDisclosureCounts(t *testing.T) {
	chain := ChainResult{
		Target: ThreadRef{PID: 100},
		Nodes: []ChainNode{
			{Thread: ThreadRef{PID: 200}, Depth: 1, Window: TimeWindow{StartTs: 1.0, EndTs: 1.03}},
		},
	}
	stats := WindowStats{
		chainAnchorsByPID:      chainAnchorWindowsByPID(chain),
		offCPUProducerDisjoint: true,
		runnableCensus: map[string]ThreadDuration{
			"200|0": {Thread: ThreadRef{PID: 200}, DurationMs: 8.0, anchoredMs: 5.0},
			"600|0": {Thread: ThreadRef{PID: 600}, DurationMs: 9.0, anchoredMs: 0.0},
		},
	}
	// pid 600 is census-known but holds no jump window → no decision may mint
	// for it either; give it one so the stamp-less mismatch arm is what fires.
	chain.Nodes = append(chain.Nodes, ChainNode{Thread: ThreadRef{PID: 600}, Depth: 1, Window: TimeWindow{StartTs: 1.05, EndTs: 1.06}})
	stats.chainAnchorsByPID = chainAnchorWindowsByPID(chain)
	items := []RootCauseRankItem{
		{Type: "runnable_wait", Thread: ThreadRef{PID: 500, Comm: "noledger"}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 12.0, ImpactMs: 12.0, CumulativeImpactMs: 12.0, EffectiveImpactMs: 12.0,
			Source: "window_stats", Confidence: 0.76},
		{Type: "fragmented_runnable_wait", Thread: ThreadRef{PID: 500, Comm: "noledger"}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 12.0, ImpactMs: 12.0, CumulativeImpactMs: 12.0, EffectiveImpactMs: 12.0,
			Source: "window_stats.state_churn", Confidence: 0.7},
		{Type: "runnable_wait", Thread: ThreadRef{PID: 600, Comm: "legacyform"}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 4.0, ImpactMs: 4.0, CumulativeImpactMs: 4.0, EffectiveImpactMs: 4.0,
			Source: "window_stats", Confidence: 0.76},
		{Type: "runnable_wait", Thread: ThreadRef{PID: 200, Comm: "ledgered"}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 8.0, ImpactMs: 8.0, CumulativeImpactMs: 8.0, EffectiveImpactMs: 8.0,
			Source: "window_stats", Confidence: 0.76,
			ledgerAnchorStamped: true, ledgerAnchoredRunnableMs: 5.0},
	}
	items = reanchorOnChainStateSeats(chain, stats, items)
	// 禁估算: the fallback seats' values and lanes moved ZERO.
	for _, it := range items {
		if it.Thread.PID != 500 && it.Thread.PID != 600 {
			continue
		}
		if it.ChainAnchorFullMs != 0 || it.ChainAnchorRemainderSeat || it.ChainRelevance != "on_chain" {
			t.Fatalf("no-ledger seat must keep its full account untouched (禁估算): %+v", it)
		}
	}
	caveat := rspaRunnableLedgerFallbackCaveat(chain, stats, items)
	if caveat == "" || !strings.HasPrefix(caveat, rspaRunnableLedgerFallbackCaveatPrefix) {
		t.Fatalf("armed board with fallback seats must disclose, got %q", caveat)
	}
	if !strings.Contains(caveat, "3 on-chain runnable seat(s)") {
		t.Fatalf("fallback population must count exactly the three no-ledger seats: %q", caveat)
	}
	if !strings.Contains(caveat, "noledger-500") || !strings.Contains(caveat, "legacyform-600") {
		t.Fatalf("disclosure must name the fallback seats: %q", caveat)
	}
	if strings.Contains(caveat, "ledgered-200") {
		t.Fatalf("the ledger-covered control seat must never be counted: %q", caveat)
	}
	if !hasRunnableLedgerFallbackCaveat([]string{caveat}) {
		t.Fatal("sentinel-prefix dedupe helper must recognise its own sentence")
	}
}

// TestRUNSPLITFallbackDisclosureScopeBoundaries — 件3 scope negatives: a board
// with no anchor sweep at all (the §29.61.10 documented fail-open boundary)
// and credentialed/self/satellite seats never enter the disclosure population.
func TestRUNSPLITFallbackDisclosureScopeBoundaries(t *testing.T) {
	chain := ChainResult{
		Target: ThreadRef{PID: 100},
		Nodes: []ChainNode{
			{Thread: ThreadRef{PID: 200}, Depth: 1, Window: TimeWindow{StartTs: 1.0, EndTs: 1.03}},
		},
	}
	fallbackSeat := RootCauseRankItem{Type: "runnable_wait", Thread: ThreadRef{PID: 500, Comm: "noledger"},
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
		RunnableMs: 12.0, ImpactMs: 12.0, EffectiveImpactMs: 12.0, Source: "window_stats", Confidence: 0.76}
	// (a) no ledger infrastructure → silent, byte-identically.
	if caveat := rspaRunnableLedgerFallbackCaveat(chain, WindowStats{}, []RootCauseRankItem{fallbackSeat}); caveat != "" {
		t.Fatalf("anchor-less board is the documented boundary, never a per-seat ledger miss: %q", caveat)
	}
	stats := WindowStats{chainAnchorsByPID: chainAnchorWindowsByPID(chain), offCPUProducerDisjoint: true}
	// (b) typed-basis seats ride their own credential lanes.
	edgeSeat := fallbackSeat
	edgeSeat.OnChainBasis = RootCauseOnChainBasisHostWakeupEdgeState
	// (c) the target's own seats are self-exempt (R8 自身恒链上).
	selfSeat := fallbackSeat
	selfSeat.Thread = ThreadRef{PID: 100, Comm: "app"}
	// (d) satellites carry their own R4 lane arms.
	satellite := fallbackSeat
	satellite.Type = "scheduler_latency"
	satellite.Source = "scheduler_latency_stats"
	if caveat := rspaRunnableLedgerFallbackCaveat(chain, stats, []RootCauseRankItem{edgeSeat, selfSeat, satellite}); caveat != "" {
		t.Fatalf("credentialed/self/satellite seats must never enter the fallback population: %q", caveat)
	}
	// The basis-less member seat on the same armed board DOES disclose.
	if caveat := rspaRunnableLedgerFallbackCaveat(chain, stats, []RootCauseRankItem{fallbackSeat}); !strings.Contains(caveat, "1 on-chain runnable seat(s)") {
		t.Fatalf("armed-board basis-less member seat must disclose: %q", caveat)
	}
}
