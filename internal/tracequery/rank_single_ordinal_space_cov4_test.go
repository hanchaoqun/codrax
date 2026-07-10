package tracequery

// rank_single_ordinal_space_cov4_test.go — §29.27 ruling ① (COV-4, ledger
// docs/design/real_trace_campaign_20260705.md, user ruling 2026-07-11) 同板
// 查漏 pin: RUNNING-system candidates (converted compute-supply running,
// on-chain semantic classes) compete in the SAME single ordinal space as
// wait-chain candidates — one rankPos counter, ordered by published
// (converted) effective impact, no hidden demotion lane. The only Rank=0
// classes are the deliberate no-seat signals (data_gap diagnostics and
// target-self wait symptoms). This is a tripwire: reintroducing a running-
// only demotion arm (a second ordinal counter, a running filter, or a
// raw-instead-of-converted running sort key) reds it.

import "testing"

func TestCov4RunningCandidatesShareOneOrdinalSpace(t *testing.T) {
	items := []RootCauseRankItem{
		{
			// Wait-chain candidate.
			Type: "runnable_wait", Thread: ThreadRef{Comm: "waiter", PID: 100},
			ImpactMs: 26.392, CumulativeImpactMs: 26.392, EffectiveImpactMs: 26.392,
			RunnableMs: 26.392, DominantState: string(StateRunnable),
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", LineStart: 10,
		},
		{
			// RUNNING candidate — the published effective is the CONVERTED
			// supply-fold deficit (§20.2), and that converted value is its
			// ordinal key: raw running 109.270 would out-rank the waiter, the
			// converted 16.100 ranks below it.
			Type: "running", Thread: ThreadRef{Comm: "self-ui", PID: 61},
			ImpactMs: 109.270, CumulativeImpactMs: 109.270, EffectiveImpactMs: 16.100,
			RunningMs: 109.270, DominantState: string(StateRunning),
			SupplyFoldDeficitMs: 16.100, SupplyFoldIdealMs: 93.170,
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", LineStart: 20,
		},
		{
			// On-chain semantic class candidate (SEM-LEAD full-rights).
			Type: "trace_semantic_span", Thread: ThreadRef{Comm: "self-ui", PID: 61},
			SpanName: "VerifyClass Foo", SemanticClass: "class_verification",
			ImpactMs: 10.394, CumulativeImpactMs: 10.394, EffectiveImpactMs: 10.394,
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", LineStart: 30,
		},
		{
			// data_gap diagnostic: the deliberate no-seat class.
			Type: "trace_gap", Thread: ThreadRef{Comm: "blind", PID: 900},
			TraceGapKind:   "no_sched_data",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", LineStart: 40,
		},
	}
	sortRootCauseRankItems(items, true)
	assignRootCauseRanksAndTiers(items)

	byPIDType := func(pid int, typ string) *RootCauseRankItem {
		for i := range items {
			if items[i].Thread.PID == pid && items[i].Type == typ {
				return &items[i]
			}
		}
		return nil
	}
	waiter := byPIDType(100, "runnable_wait")
	running := byPIDType(61, "running")
	semantic := byPIDType(61, "trace_semantic_span")
	gap := byPIDType(900, "trace_gap")
	if waiter == nil || running == nil || semantic == nil || gap == nil {
		t.Fatalf("fixture drifted: %+v", items)
	}
	// One ordinal space, ordered by published (converted) effective impact:
	// waiter 26.392 → #1, running converted 16.100 → #2, semantic 10.394 → #3.
	if waiter.Rank != 1 || running.Rank != 2 || semantic.Rank != 3 {
		t.Fatalf("running/semantic candidates must share the wait board's ordinal space by converted eff: waiter=%d running=%d semantic=%d",
			waiter.Rank, running.Rank, semantic.Rank)
	}
	// The converted value is the running row's ordinal key — the raw 109.270
	// staying published on ImpactMs proves no channel was overwritten.
	if got := rootCauseEffectiveImpactMs(*running); got != 16.100 {
		t.Fatalf("the running ordinal key must be the converted deficit, got %.3f", got)
	}
	if running.ImpactMs != 109.270 {
		t.Fatalf("the raw running wall clock must stay published: %+v", running)
	}
	// The no-seat classes stay Rank=0 (badge negative gate upstream source).
	if gap.Rank != 0 {
		t.Fatalf("data_gap diagnostics never take an ordinal: %+v", gap)
	}
	// Ordinals are contiguous over the seated rows (§29.19 contiguity).
	seen := map[int]bool{}
	max := 0
	for _, item := range items {
		if item.Rank > 0 {
			if seen[item.Rank] {
				t.Fatalf("duplicate ordinal %d: %+v", item.Rank, items)
			}
			seen[item.Rank] = true
			if item.Rank > max {
				max = item.Rank
			}
		}
	}
	for i := 1; i <= max; i++ {
		if !seen[i] {
			t.Fatalf("ordinal hole at #%d (seats must be contiguous): %+v", i, items)
		}
	}
}
