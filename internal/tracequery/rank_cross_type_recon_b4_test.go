package tracequery

import (
	"strings"
	"testing"
)

func b4RankPair() (RootCauseRankItem, RootCauseRankItem) {
	thread := ThreadRef{Comm: "app-100", PID: 100, TGID: 10}
	absorber := RootCauseRankItem{
		Type: "d_state_or_io_wait", Thread: thread,
		StartTs: 5.001000, EndTs: 5.002062,
		LineStart: 757, LineEnd: 758,
		StatsWindowStartTs: 5.000000, StatsWindowEndTs: 5.007000,
		Source: "window_stats", ChainRelevance: "on_chain",
		ImpactMs: 1.062, CumulativeImpactMs: 1.062, EffectiveImpactMs: 1.062,
		DStateMs: 1.062, DominantState: string(StateDSleep),
	}
	absorbed := RootCauseRankItem{
		Type: "io_burst_episode", Thread: thread,
		StartTs: 5.001000, EndTs: 5.002062,
		LineStart: 757, LineEnd: 758,
		StatsWindowStartTs: 5.000000, StatsWindowEndTs: 5.007000,
		Source: "window_stats.io_burst_episodes", ChainRelevance: "on_chain",
		ImpactMs: 1.062, CumulativeImpactMs: 1.062, EffectiveImpactMs: 1.062,
	}
	return absorber, absorbed
}

func TestB4ExactCrossTypeSeatReconciliation(t *testing.T) {
	absorber, absorbed := b4RankPair()
	other := RootCauseRankItem{
		Type: "running", Thread: ThreadRef{Comm: "worker-200", PID: 200},
		ImpactMs: 0.5, CumulativeImpactMs: 0.5, EffectiveImpactMs: 0.5,
		ChainRelevance: "on_chain",
	}
	rank := RootCauseRankResult{Items: []RootCauseRankItem{absorber, absorbed, other}}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 1 {
		t.Fatalf("one exact duplicate seat must move to the lossless carrier: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
	if rank.Items[0].Type != "d_state_or_io_wait" || rank.Items[0].AbsorbedRankRows != 1 || rank.Items[0].RankFamilyKey == "" {
		t.Fatalf("D-state row must own the seat and exact family key: %+v", rank.Items[0])
	}
	got := rank.AbsorbedItems[0]
	if got.Type != "io_burst_episode" || got.Tier != RootCauseTierAbsorbed || got.Rank != 0 ||
		!got.AbsorbedByRankFamily || got.AbsorbedIntoFamily != rank.Items[0].RankFamilyKey {
		t.Fatalf("IO-burst observation must retain typed absorption provenance: %+v", got)
	}
	if !strings.Contains(got.AbsorbedIntoFamily, "pid:100") ||
		!strings.Contains(got.AbsorbedIntoFamily, "interval:5.001000..5.002062") ||
		!strings.Contains(got.AbsorbedIntoFamily, "lines:757-758") {
		t.Fatalf("canonical key must expose the exact identity dimensions: %q", got.AbsorbedIntoFamily)
	}
	assignRootCauseRanksAndTiers(rank.Items)
	if rank.Items[0].Rank != 1 || rank.Items[1].Rank != 2 || rank.AbsorbedItems[0].Rank != 0 {
		t.Fatalf("absorbed row must consume no ordinal: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}

	// Reset-first idempotency: rejoin the lossless row, recompute, converge.
	key := rank.Items[0].RankFamilyKey
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 1 ||
		rank.Items[0].AbsorbedRankRows != 1 || rank.Items[0].RankFamilyKey != key ||
		rank.AbsorbedItems[0].AbsorbedIntoFamily != key {
		t.Fatalf("double reconcile must converge: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
}

func TestB4CrossTypeSeatNearMissesFailOpen(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RootCauseRankItem, *RootCauseRankItem)
	}{
		{"numeric thread differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.Thread.PID = 101 }},
		{"comm only is insufficient", func(a, b *RootCauseRankItem) { a.Thread.PID, b.Thread.PID = 0, 0 }},
		{"type pair not adjudicated", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.Type = "io_wait" }},
		{"absorber producer differs", func(a, _ *RootCauseRankItem) { a.Source = "wakeup_chain" }},
		{"absorbed producer differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.Source = "window_stats" }},
		{"query start differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.StatsWindowStartTs += 0.000001 }},
		{"query end differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.StatsWindowEndTs += 0.000001 }},
		{"query endpoints absent", func(a, b *RootCauseRankItem) {
			a.StatsWindowStartTs, b.StatsWindowStartTs = 0, 0
			a.StatsWindowEndTs, b.StatsWindowEndTs = 0, 0
		}},
		{"chain lane differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.ChainRelevance = "background" }},
		{"interval start differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.StartTs += 0.000001 }},
		{"interval end differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.EndTs += 0.000001 }},
		{"line start differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.LineStart++ }},
		{"line end differs", func(_ *RootCauseRankItem, b *RootCauseRankItem) { b.LineEnd++ }},
		{"line span absent", func(a, b *RootCauseRankItem) { a.LineStart, b.LineStart = 0, 0 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, b := b4RankPair()
			tc.mutate(&a, &b)
			rank := RootCauseRankResult{Items: []RootCauseRankItem{a, b}}
			reconcileExactCrossTypeRankSeats(&rank)
			if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 0 ||
				rank.Items[0].RankFamilyKey != "" || rank.Items[1].AbsorbedByRankFamily {
				t.Fatalf("near miss must keep both honest seats: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
			}
		})
	}
}

func TestB4ExactCrossTypeSeatAtZeroTraceEpoch(t *testing.T) {
	absorber, absorbed := b4RankPair()
	absorber.StatsWindowStartTs, absorber.StatsWindowEndTs = 0, 0.007
	absorbed.StatsWindowStartTs, absorbed.StatsWindowEndTs = 0, 0.007
	absorber.StartTs, absorber.EndTs = 0, 0.001062
	absorbed.StartTs, absorbed.EndTs = 0, 0.001062
	rank := RootCauseRankResult{Items: []RootCauseRankItem{absorber, absorbed}}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 1 || len(rank.AbsorbedItems) != 1 || rank.Items[0].Type != "d_state_or_io_wait" {
		t.Fatalf("a valid t=0 window/interval must still reconcile the exact physical seat: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
}

func TestB4ReconTypeUniversePinned(t *testing.T) {
	if len(crossTypeRankSeatReconPairs) != 9 || len(crossTypeRankSeatReconOrder) != 9 {
		t.Fatalf("cross-type rank-seat adjudication universe changed: %+v", crossTypeRankSeatReconPairs)
	}
	spec, ok := crossTypeRankSeatReconPairs["io_burst_episode"]
	if !ok || spec.absorbedType != "io_burst_episode" || len(spec.absorberTypes) != 2 ||
		spec.absorberTypes[0] != "d_state_or_io_wait" || spec.absorberTypes[1] != "io_wait" ||
		len(spec.absorberSources) != 2 || spec.absorbedSource != "window_stats.io_burst_episodes" {
		t.Fatalf("B4 exact pair changed without a ruling: %+v", crossTypeRankSeatReconPairs)
	}
	scheduler, ok := crossTypeRankSeatReconPairs["scheduler_latency"]
	if !ok || scheduler.absorbedType != "scheduler_latency" ||
		scheduler.absorberSource != "window_stats" || scheduler.absorbedSource != "scheduler_latency_stats" ||
		len(scheduler.absorberTypes) != 2 || scheduler.absorberTypes[0] != "runnable_wait" ||
		scheduler.absorberTypes[1] != "priority_inversion_runnable_wait" {
		t.Fatalf("runnable/scheduler exact pair changed without a ruling: %+v", crossTypeRankSeatReconPairs)
	}
	for _, typ := range []string{"fragmented_runnable_wait", "fragmented_d_state_or_io_wait"} {
		spec, ok := crossTypeRankSeatReconPairs[typ]
		if !ok || !spec.stateScalarMatch || spec.absorbedSource != "window_stats.state_churn" {
			t.Fatalf("state-churn single-seat adjudication changed for %s: %+v", typ, crossTypeRankSeatReconPairs)
		}
	}
}

func causalWindowRankPair(absorberType, candidateType, absorberSource, candidateSource string) (TimeWindow, RootCauseRankItem, RootCauseRankItem) {
	window := TimeWindow{StartTs: 5.000, EndTs: 5.010}
	thread := ThreadRef{Comm: "worker-200", PID: 200, TGID: 100}
	absorber := RootCauseRankItem{
		Type: absorberType, Thread: thread,
		StartTs: 5.001, EndTs: 5.006, LineStart: 100, LineEnd: 120,
		Source: absorberSource, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
		ImpactMs: 5, CumulativeImpactMs: 5, EffectiveImpactMs: 5,
	}
	candidate := RootCauseRankItem{
		Type: candidateType, Thread: thread,
		StartTs: absorber.StartTs, EndTs: absorber.EndTs,
		LineStart: absorber.LineStart, LineEnd: absorber.LineEnd,
		StatsWindowStartTs: window.StartTs, StatsWindowEndTs: window.EndTs,
		Source: candidateSource, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
		ImpactMs: 5, CumulativeImpactMs: 5, EffectiveImpactMs: 5,
	}
	stampState := func(item *RootCauseRankItem) {
		switch item.Type {
		case "runnable_wait", "priority_inversion_runnable_wait", "priority_inversion_candidate":
			item.DominantState = string(StateRunnable)
			item.RunnableMs = 5
			if item.Type == "priority_inversion_candidate" || item.Type == "priority_inversion_runnable_wait" {
				item.ImpactMs = 2
				item.EffectiveImpactMs = 2
				item.GatedRunnableMs = 2
			}
		case "io_wait":
			item.DominantState = string(StateIOWait)
			item.IOWaitMs = 5
		case "d_state_or_io_wait":
			item.DominantState = string(StateDSleep)
			item.DStateMs = 5
		}
	}
	stampState(&absorber)
	stampState(&candidate)
	return window, absorber, candidate
}

func TestCausalWindowExactSeatsReconcileAcrossProducers(t *testing.T) {
	tests := []struct {
		name            string
		absorberType    string
		candidateType   string
		absorberSource  string
		candidateSource string
		wantEffective   float64
	}{
		{"plain runnable causal", "runnable_wait", "runnable_wait", "wakeup_chain.causal_impacts", "window_stats", 5},
		{"plain runnable aggregate", "runnable_wait", "runnable_wait", "wakeup_chain.aggregated_impacts", "window_stats", 5},
		{"gated inversion projection", "priority_inversion_candidate", "priority_inversion_runnable_wait", "wakeup_chain.causal_impacts", "window_stats", 2},
		{"gated inversion over plain runnable", "priority_inversion_candidate", "runnable_wait", "wakeup_chain.aggregated_impacts", "window_stats", 2},
		{"IO", "io_wait", "io_wait", "wakeup_chain.causal_impacts", "window_stats.io_wait_top", 5},
		{"D state", "d_state_or_io_wait", "d_state_or_io_wait", "wakeup_chain.aggregated_impacts", "window_stats", 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			window, absorber, candidate := causalWindowRankPair(tc.absorberType, tc.candidateType, tc.absorberSource, tc.candidateSource)
			rank := RootCauseRankResult{Window: window, Items: []RootCauseRankItem{absorber, candidate}}
			reconcileExactCrossTypeRankSeats(&rank)
			if len(rank.Items) != 1 || len(rank.AbsorbedItems) != 1 {
				t.Fatalf("one exact physical account must own one seat: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
			}
			owner, supporting := rank.Items[0], rank.AbsorbedItems[0]
			if owner.Type != tc.absorberType || owner.Source != tc.absorberSource ||
				owner.AbsorbedRankRows != 1 || owner.RankFamilyKey == "" {
				t.Fatalf("causal producer must own the active seat with provenance: %+v", owner)
			}
			if supporting.Type != tc.candidateType || supporting.Source != tc.candidateSource ||
				supporting.Tier != RootCauseTierAbsorbed || !supporting.AbsorbedByRankFamily ||
				supporting.AbsorbedIntoFamily != owner.RankFamilyKey {
				t.Fatalf("window projection must remain lossless supporting evidence: %+v", supporting)
			}
			if got := rootCauseEffectiveImpactMs(owner); !near(got, tc.wantEffective, 0.000001) {
				t.Fatalf("reconciliation changed authoritative effective impact: got %.3f want %.3f owner=%+v", got, tc.wantEffective, owner)
			}
			if !strings.Contains(owner.RankFamilyKey, "window:5.000000..5.010000") ||
				strings.Contains(owner.RankFamilyKey, "window:0.000000..0.000000") {
				t.Fatalf("causal row must resolve the selected query window into the provenance key: %q", owner.RankFamilyKey)
			}

			// Reset-first recomputation must rejoin the supporting observation and
			// converge without multiplying markers or changing its owner.
			key := owner.RankFamilyKey
			reconcileExactCrossTypeRankSeats(&rank)
			if len(rank.Items) != 1 || len(rank.AbsorbedItems) != 1 ||
				rank.Items[0].AbsorbedRankRows != 1 || rank.Items[0].RankFamilyKey != key ||
				rank.AbsorbedItems[0].AbsorbedIntoFamily != key {
				t.Fatalf("causal/window reconciliation must be idempotent: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
			}
		})
	}
}

func TestCausalWindowNearMissesFailOpen(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RootCauseRankItem, *RootCauseRankItem)
	}{
		{"interval", func(_ *RootCauseRankItem, candidate *RootCauseRankItem) {
			candidate.StartTs += 0.000001
			candidate.EndTs += 0.000001
		}},
		{"line", func(_ *RootCauseRankItem, candidate *RootCauseRankItem) { candidate.LineEnd++ }},
		{"window", func(_ *RootCauseRankItem, candidate *RootCauseRankItem) { candidate.StatsWindowStartTs += 0.000001 }},
		{"lane", func(_ *RootCauseRankItem, candidate *RootCauseRankItem) {
			candidate.Causality = "background"
			candidate.ChainRelevance = "background"
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			window, absorber, candidate := causalWindowRankPair("runnable_wait", "runnable_wait", "wakeup_chain.causal_impacts", "window_stats")
			tc.mutate(&absorber, &candidate)
			rank := RootCauseRankResult{Window: window, Items: []RootCauseRankItem{absorber, candidate}}
			reconcileExactCrossTypeRankSeats(&rank)
			if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 0 {
				t.Fatalf("%s mismatch must fail open: active=%+v absorbed=%+v", tc.name, rank.Items, rank.AbsorbedItems)
			}
		})
	}
}

func TestCausalWindowAmbiguousOwnerFailsOpen(t *testing.T) {
	window, first, candidate := causalWindowRankPair("runnable_wait", "runnable_wait", "wakeup_chain.causal_impacts", "window_stats")
	second := first
	second.Source = "wakeup_chain.aggregated_impacts"
	second.MemberKey = "second-exact-owner"
	rank := RootCauseRankResult{Window: window, Items: []RootCauseRankItem{first, second, candidate}}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 3 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("two exact causal owners must keep all seats instead of electing arbitrarily: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
}

func TestBuildRootCauseRankCausalAndWindowRunnableOwnOneSeat(t *testing.T) {
	window := TimeWindow{StartTs: 5.000, EndTs: 5.010}
	target := ThreadRef{Comm: "app-100", PID: 100, TGID: 100}
	dependency := ThreadRef{Comm: "worker-200", PID: 200, TGID: 100}
	impact := WakeupCausalImpact{
		Thread: dependency, Window: TimeWindow{StartTs: 5.001, EndTs: 5.006},
		ActualWindow: TimeWindow{StartTs: 5.001, EndTs: 5.006},
		ChainDepth:   1, OnChain: true, DominantState: string(StateRunnable),
		DominantImpactMs: 5, ProjectedImpactMs: 5, ProjectedTotalMs: 5,
		ActualImpactMs: 5, ActualTotalMs: 5, TotalMs: 5,
		RunnableMs: 5, ActualRunnableMs: 5, TargetBlockedMs: 5,
		LineStart: 100, LineEnd: 120,
	}
	chain := ChainResult{
		Target: target,
		Nodes: []ChainNode{
			{ID: "target", Thread: target, Window: window},
			{ID: "dependency", Thread: dependency, Window: impact.Window, Impact: &impact, Depth: 1},
		},
		CausalImpacts: []WakeupCausalImpact{impact},
	}
	stats := WindowStats{
		Window: window,
		RunnableTop: []ThreadDuration{{
			Thread: dependency, DurationMs: 5, CPU: 2,
			StartTs: 5.001, EndTs: 5.006, LineStart: 100, LineEnd: 120,
		}},
	}
	rank := buildRootCauseRankFrom(nil, Query{PID: target.PID, TimeStart: window.StartTs, TimeEnd: window.EndTs, Limit: 12}, chain, stats)

	var active []RootCauseRankItem
	for _, item := range rank.Items {
		if item.Thread.PID == dependency.PID && item.Type == "runnable_wait" {
			active = append(active, item)
		}
	}
	if len(active) != 1 || active[0].Source != "wakeup_chain.causal_impacts" || active[0].AbsorbedRankRows != 1 {
		t.Fatalf("production rank construction must keep one causal runnable carrier across producers: %+v", rank.Items)
	}
	if len(rank.AbsorbedItems) != 1 || rank.AbsorbedItems[0].Thread.PID != dependency.PID ||
		rank.AbsorbedItems[0].Type != "runnable_wait" || rank.AbsorbedItems[0].Source != "window_stats" ||
		rank.AbsorbedItems[0].AbsorbedIntoFamily != active[0].RankFamilyKey {
		t.Fatalf("production window runnable must survive on the lossless carrier: %+v", rank.AbsorbedItems)
	}
	if !strings.Contains(active[0].RankFamilyKey, "window:5.000000..5.010000") {
		t.Fatalf("production causal carrier must publish its resolved nonzero query window: %q", active[0].RankFamilyKey)
	}
}

func schedulerLatencyRankPair(inversion bool) (RootCauseRankItem, RootCauseRankItem) {
	thread := ThreadRef{Comm: "app-100", PID: 100, TGID: 10}
	absorber := RootCauseRankItem{
		Type: "runnable_wait", Thread: thread,
		StartTs: 5.001, EndTs: 5.031, LineStart: 100, LineEnd: 120,
		StatsWindowStartTs: 5, StatsWindowEndTs: 5.040,
		Source: "window_stats", ChainRelevance: "on_chain",
		DominantState: string(StateRunnable), RunnableMs: 30,
		ImpactMs: 30, CumulativeImpactMs: 30, EffectiveImpactMs: 30,
	}
	if inversion {
		absorber.Type = "priority_inversion_runnable_wait"
		absorber.EffectiveImpactMs = 2
		absorber.GatedRunnableMs = 2
	}
	latency := RootCauseRankItem{
		Type: "scheduler_latency", Thread: thread,
		StartTs: absorber.StartTs, EndTs: absorber.EndTs,
		LineStart: absorber.LineStart, LineEnd: absorber.LineEnd,
		StatsWindowStartTs: absorber.StatsWindowStartTs, StatsWindowEndTs: absorber.StatsWindowEndTs,
		Source: "scheduler_latency_stats", ChainRelevance: "on_chain",
		DominantState: string(StateRunnable), RunnableMs: 30,
		ImpactMs: 30, CumulativeImpactMs: 30, EffectiveImpactMs: 30,
	}
	return absorber, latency
}

func TestExactRunnableStateOwnsSchedulerLatencySeat(t *testing.T) {
	for _, inversion := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "gated inversion"}[inversion], func(t *testing.T) {
			state, latency := schedulerLatencyRankPair(inversion)
			rank := RootCauseRankResult{Items: []RootCauseRankItem{state, latency}}
			reconcileExactCrossTypeRankSeats(&rank)
			if len(rank.Items) != 1 || len(rank.AbsorbedItems) != 1 ||
				rank.Items[0].Type != state.Type || rank.AbsorbedItems[0].Type != "scheduler_latency" {
				t.Fatalf("one exact runnable segment must own one seat: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
			}
			want := 30.0
			if inversion {
				want = 2
			}
			if got := rootCauseEffectiveImpactMs(rank.Items[0]); !near(got, want, 0.000001) {
				t.Fatalf("formal state caliber must remain authoritative, got %.3f want %.3f: %+v", got, want, rank.Items[0])
			}
		})
	}
}

func TestSchedulerLatencyNearMissKeepsIndependentSeat(t *testing.T) {
	state, latency := schedulerLatencyRankPair(true)
	latency.EndTs += 0.000001
	rank := RootCauseRankResult{Items: []RootCauseRankItem{state, latency}}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("non-identical intervals must fail open: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
}

func TestExactStateChurnProjectionDoesNotTakeSecondSeat(t *testing.T) {
	tests := []struct {
		name      string
		formal    RootCauseRankItem
		projected RootCauseRankItem
	}{
		{
			name: "runnable",
			formal: RootCauseRankItem{Type: "runnable_wait", Source: "window_stats",
				DominantState: string(StateRunnable), RunnableMs: 5},
			projected: RootCauseRankItem{Type: "fragmented_runnable_wait", Source: "window_stats.state_churn",
				DominantState: string(StateRunnable), RunnableMs: 5},
		},
		{
			name: "D IO",
			formal: RootCauseRankItem{Type: "io_wait", Source: "window_stats.io_wait_top",
				DominantState: string(StateIOWait), IOWaitMs: 5},
			projected: RootCauseRankItem{Type: "fragmented_d_state_or_io_wait", Source: "window_stats.state_churn",
				DominantState: string(StateIOWait), IOWaitMs: 5},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			thread := ThreadRef{PID: 100, Comm: "app"}
			for _, item := range []*RootCauseRankItem{&tc.formal, &tc.projected} {
				item.Thread = thread
				item.StatsWindowStartTs, item.StatsWindowEndTs = 5, 5.010
				item.ChainRelevance = "on_chain"
				item.LineStart, item.LineEnd = 10, 20
				item.ImpactMs, item.CumulativeImpactMs = 5, 5
			}
			tc.formal.StartTs, tc.formal.EndTs = 5.001, 5.006
			rank := RootCauseRankResult{Items: []RootCauseRankItem{tc.formal, tc.projected}}
			reconcileExactCrossTypeRankSeats(&rank)
			if len(rank.Items) != 1 || len(rank.AbsorbedItems) != 1 ||
				rank.AbsorbedItems[0].Type != tc.projected.Type {
				t.Fatalf("same typed state account must own one seat: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
			}

			// Exact scalar equality is load-bearing; a different projection may
			// cover additional fragments and must fail open.
			tc.projected.RunnableMs += 0.001
			tc.projected.IOWaitMs += 0.001
			rank = RootCauseRankResult{Items: []RootCauseRankItem{tc.formal, tc.projected}}
			reconcileExactCrossTypeRankSeats(&rank)
			if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 0 {
				t.Fatalf("state-scalar near miss must remain independent: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
			}
		})
	}
}

func TestCrossTypeAbsorbedOrderIsDeterministic(t *testing.T) {
	state, latency := schedulerLatencyRankPair(false)
	dstate, burst := b4RankPair()
	want := []string{"scheduler_latency", "io_burst_episode"}
	for run := 0; run < 20; run++ {
		rank := RootCauseRankResult{Items: []RootCauseRankItem{state, latency, dstate, burst}}
		reconcileExactCrossTypeRankSeats(&rank)
		if len(rank.AbsorbedItems) != len(want) {
			t.Fatalf("run %d absorbed cardinality drift: %+v", run, rank.AbsorbedItems)
		}
		for i, typ := range want {
			if rank.AbsorbedItems[i].Type != typ {
				t.Fatalf("run %d lossless order drift: got %+v want %v", run, rank.AbsorbedItems, want)
			}
		}
	}
}

func TestCrossTypeFamilyIntervalsRequireExactPhysicalSet(t *testing.T) {
	state, latency := schedulerLatencyRankPair(false)
	state.MemberCount, latency.MemberCount = 2, 2
	state.MemberFoldCaliber = RootCauseMemberFoldCaliberSumDisjoint
	latency.MemberFoldCaliber = RootCauseMemberFoldCaliberSumDisjoint
	state.familyMemberIntervals = []foldInterval{{start: 5.001, end: 5.010}, {start: 5.020, end: 5.031}}
	latency.familyMemberIntervals = append([]foldInterval(nil), state.familyMemberIntervals...)
	rank := RootCauseRankResult{Items: []RootCauseRankItem{state, latency}}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 1 || len(rank.AbsorbedItems) != 1 {
		t.Fatalf("equal family interval sets must reconcile: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}

	// Same hull and line envelope, different internal gap: never the same
	// physical set, so the exact gate must fail open.
	state, latency = schedulerLatencyRankPair(false)
	state.MemberCount, latency.MemberCount = 2, 2
	state.MemberFoldCaliber = RootCauseMemberFoldCaliberSumDisjoint
	latency.MemberFoldCaliber = RootCauseMemberFoldCaliberSumDisjoint
	state.familyMemberIntervals = []foldInterval{{start: 5.001, end: 5.010}, {start: 5.020, end: 5.031}}
	latency.familyMemberIntervals = []foldInterval{{start: 5.001, end: 5.015}, {start: 5.025, end: 5.031}}
	rank = RootCauseRankResult{Items: []RootCauseRankItem{state, latency}}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("same hull with different family gaps must fail open: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
}

func TestB4AmbiguousAbsorberFailsOpen(t *testing.T) {
	a, b := b4RankPair()
	secondOwner := a
	secondOwner.MemberKey = "second-owner"
	rank := RootCauseRankResult{Items: []RootCauseRankItem{a, secondOwner, b}}
	reconcileExactCrossTypeRankSeats(&rank)
	if len(rank.Items) != 3 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("two exact possible owners must not trigger arbitrary absorption: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
}

func TestB4FamilyKeySurvivesLaterG1Reconciliation(t *testing.T) {
	a, b := b4RankPair()
	rank := RootCauseRankResult{Items: []RootCauseRankItem{a, b}}
	reconcileExactCrossTypeRankSeats(&rank)
	b4Key := rank.Items[0].RankFamilyKey

	g1 := g1UnitFamily()
	rank.Items = append(rank.Items, g1)
	blocking := g1UnitBlocking(TimeWindow{StartTs: 10.0, EndTs: 11.0}, CriticalBlockingCandidate{
		Type: "io_latency", Thread: ThreadRef{Comm: "work", PID: 500},
		StartTs: 10.10, EndTs: 10.20, DurationMs: 100,
	})
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	if rank.Items[0].RankFamilyKey != b4Key || rank.Items[0].AbsorbedRankRows != 1 {
		t.Fatalf("G1 reset must preserve B4-owned marker: %+v", rank.Items[0])
	}
	if rank.Items[1].RankFamilyKey == "" || rank.Items[1].AbsorbedChainRows != 1 ||
		!blocking.Items[0].AbsorbedByRankFamily {
		t.Fatalf("G1 must still reconcile its own independent family: %+v / %+v", rank.Items[1], blocking.Items[0])
	}
}

func TestB4BuildRootCauseRankProductionShape(t *testing.T) {
	thread := ThreadRef{Comm: "app-100", PID: 100, TGID: 10}
	window := TimeWindow{StartTs: 5.000000, EndTs: 5.007000}
	stats := WindowStats{
		Window: window,
		DStateTop: []ThreadDuration{{
			Thread: thread, DurationMs: 1.062, CPU: 2,
			StartTs: 5.001000, EndTs: 5.002062, LineStart: 757, LineEnd: 758,
		}},
		IOBurstEpisodes: []IOBurstEpisodeSummary{{
			Thread: thread, DominantSignal: "d_state_or_io_wait",
			DurationMs: 1.062, DStateMs: 1.062,
			StartTs: 5.001000, EndTs: 5.002062, LineStart: 757, LineEnd: 758,
			Confidence: 0.74,
		}},
	}
	rank := buildRootCauseRankFrom(nil, Query{PID: 100, TimeStart: window.StartTs, TimeEnd: window.EndTs}, ChainResult{}, stats)
	activeD, activeBurst := 0, 0
	for _, item := range rank.Items {
		switch item.Type {
		case "d_state_or_io_wait":
			activeD++
		case "io_burst_episode":
			activeBurst++
		}
	}
	if activeD != 1 || activeBurst != 0 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("production rank path must seat the physical wait once: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
	if len(stats.IOBurstEpisodes) != 1 || stats.IOBurstEpisodes[0].DominantSignal != "d_state_or_io_wait" {
		t.Fatalf("scheduler-derived episode must remain available on WindowStats as a lossless observation: %+v", stats.IOBurstEpisodes)
	}
}
