package tracequery

import "testing"

func TestRootCauseEffectiveMatrixUsesTypedCalibers(t *testing.T) {
	tests := []struct {
		name string
		item RootCauseRankItem
		want float64
	}{
		{
			name: "runnable uses runnable lane not total",
			item: RootCauseRankItem{Type: "runnable_wait", DominantState: string(StateRunnable),
				RunnableMs: 7, RunningMs: 30, SleepMs: 20, CumulativeImpactMs: 57},
			want: 7,
		},
		{
			name: "fragmented runnable excludes mixed states",
			item: RootCauseRankItem{Type: "fragmented_runnable_wait", DominantState: string(StateRunnable),
				RunnableMs: 9, RunningMs: 8, SleepMs: 7, CumulativeImpactMs: 24, EffectiveImpactMs: 20},
			want: 9,
		},
		{
			name: "D and IO participate in full",
			item: RootCauseRankItem{Type: "fragmented_d_state_or_io_wait", DominantState: string(StateIOWait),
				DStateMs: 3, IOWaitMs: 4, RunningMs: 20, CumulativeImpactMs: 27, EffectiveImpactMs: 18},
			want: 7,
		},
		{
			name: "running uses CAP deficit",
			item: RootCauseRankItem{Type: "running", DominantState: string(StateRunning),
				RunningMs: 26.392, CumulativeImpactMs: 26.392, EffectiveImpactMs: 6.125,
				SupplyFoldDeficitMs: 6.125, SupplyFoldBasis: &SupplyFoldBasis{KnownMs: 26.392}},
			want: 6.125,
		},
		{
			name: "unfolded running is zero",
			item: RootCauseRankItem{Type: "fragmented_running", DominantState: string(StateRunning),
				RunningMs: 26.392, CumulativeImpactMs: 26.392},
			want: 0,
		},
		{
			name: "ordinary sleep is context only",
			item: RootCauseRankItem{Type: "sleep_wait", DominantState: string(StateSSleep),
				SleepMs: 20, CumulativeImpactMs: 20, EffectiveImpactMs: 20},
			want: 0,
		},
		{
			name: "unknown churn is context only",
			item: RootCauseRankItem{Type: "state_churn", DominantState: "unknown",
				CumulativeImpactMs: 20, EffectiveImpactMs: 20},
			want: 0,
		},
		{
			name: "semantic running uses measured intersection",
			item: RootCauseRankItem{Type: "class_verification", SemanticClass: "class_verification",
				DominantState: string(StateRunning), RunningMs: 5.3, EffectiveImpactMs: 5.3, CumulativeImpactMs: 12},
			want: 5.3,
		},
		{
			name: "periodic source uses VS1 discounted effective",
			item: RootCauseRankItem{Type: "sleep_wait", PeriodicSource: true,
				SleepMs: 30, RunnableMs: 2, CumulativeImpactMs: 32, EffectiveImpactMs: 3.5},
			want: 3.5,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := rootCauseEffectiveImpactMs(tc.item); !near(got, tc.want, 0.000001) {
				t.Fatalf("effective=%.6f want %.6f for %+v", got, tc.want, tc.item)
			}
		})
	}
}

func TestTypedZeroRowsNeverTakeRankCapacityOrKeepRawScore(t *testing.T) {
	items := []RootCauseRankItem{
		{Type: "running", DominantState: string(StateRunning), RunningMs: 100,
			ImpactMs: 100, CumulativeImpactMs: 100, Score: 1000,
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "state_churn", DominantState: "unknown", ImpactMs: 80,
			CumulativeImpactMs: 80, EffectiveImpactMs: 80, Score: 900,
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "runnable_wait", DominantState: string(StateRunnable), RunnableMs: 2,
			ImpactMs: 2, CumulativeImpactMs: 2, Score: 1,
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
	}
	normalizeRootCauseEffectiveImpact(items)
	sortRootCauseRankItems(items, true)
	assignRootCauseRanksAndTiers(items)
	if items[0].Type != "runnable_wait" || items[0].Rank != 1 || items[0].Tier != "primary" {
		t.Fatalf("the only positive typed impact must own the crown: %+v", items)
	}
	for _, item := range items[1:] {
		if item.Rank != 0 || item.Tier != RootCauseTierContextOnly || item.Score != 0 || rootCauseEffectiveImpactMs(item) != 0 {
			t.Fatalf("typed-zero rows stay bounded rank-0 context with score=0: %+v", item)
		}
	}
	trimmed, _, _, candidateTotal, candidateEmitted, _, _ := truncateRootCauseRankCandidatesAndSideRows(items, 1)
	if candidateTotal != 1 || candidateEmitted != 1 || len(trimmed) < 1 || trimmed[0].Type != "runnable_wait" {
		t.Fatalf("rank-0 context must not consume candidate capacity: totals=%d/%d items=%+v", candidateEmitted, candidateTotal, trimmed)
	}
}

func TestWakeupCausalEffectiveMatrixNeverFallsBackToTotal(t *testing.T) {
	tests := []struct {
		name   string
		impact WakeupCausalImpact
		want   float64
	}{
		{
			name: "runnable",
			impact: WakeupCausalImpact{DominantState: string(StateRunnable), RunnableMs: 5,
				RunningMs: 30, SleepMs: 20, TotalMs: 55},
			want: 5,
		},
		{
			name: "D IO",
			impact: WakeupCausalImpact{DominantState: string(StateDSleep), DStateMs: 2,
				IOWaitMs: 3, RunningMs: 40, TotalMs: 45},
			want: 5,
		},
		{
			name: "running missing fold",
			impact: WakeupCausalImpact{DominantState: string(StateRunning), RunningMs: 40,
				TotalMs: 50},
			want: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := WakeupCausalImpactEffectiveImpactMs(tc.impact); !near(got, tc.want, 0.000001) {
				t.Fatalf("effective=%.6f want %.6f for %+v", got, tc.want, tc.impact)
			}
		})
	}
}

func TestDIOAggregateKeepsFullBlockingCaliberInsteadOfInversionGate(t *testing.T) {
	aggregate := WakeupCausalAggregate{
		Thread:                   ThreadRef{PID: 200, Comm: "io-worker"},
		DominantState:            string(StateIOWait),
		DominantImpactMs:         9,
		DStateMs:                 2,
		IOWaitMs:                 7,
		TotalMs:                  12,
		PriorityInversion:        true,
		PriorityInversionGatedMs: 3,
		GatedRunnableMs:          2,
		GatedRunningDeficitMs:    1,
		ChainDepth:               1,
	}
	if WakeupCausalAggregateInversionTyped(aggregate) {
		t.Fatalf("D/IO-dominant aggregate must retain its IO identity, not scheduling-inversion gating: %+v", aggregate)
	}
	item := rootCauseItemFromCausalAggregate(aggregate)
	if item.Type != "io_wait" || !near(rootCauseEffectiveImpactMs(item), 9, 0.000001) {
		t.Fatalf("D/IO aggregate must participate by DStateMs+IOWaitMs=9, got %+v effective=%.6f", item, rootCauseEffectiveImpactMs(item))
	}
	if item.Confidence != 0.82 {
		t.Fatalf("raw inversion census bit must not raise non-inversion D/IO confidence: %+v", item)
	}
}

func TestRawAggregateInversionFlagCannotSuppressPlainRunningDeficit(t *testing.T) {
	aggregate := WakeupCausalAggregate{
		Thread: ThreadRef{PID: 200, Comm: "worker"}, DominantState: string(StateRunning),
		DominantImpactMs: 10, RunningMs: 10, TotalMs: 10,
		PriorityInversion: true, PriorityInversionGatedMs: 0,
		SupplyFoldDeficitMs: 4, SupplyFoldIdealMs: 10,
		SupplyFoldBasis: &SupplyFoldBasis{KnownMs: 10}, ChainDepth: 1,
	}
	if WakeupCausalAggregateInversionTyped(aggregate) {
		t.Fatalf("gated zero cannot type an aggregate as inversion: %+v", aggregate)
	}
	item := rootCauseItemFromCausalAggregate(aggregate)
	if item.Type != "running" || !near(rootCauseEffectiveImpactMs(item), 4, 0.000001) || item.Score <= 0 || item.Confidence != 0.82 {
		t.Fatalf("plain running CAP deficit must survive the weak raw flag: %+v effective=%.3f", item, rootCauseEffectiveImpactMs(item))
	}
}

func TestAggregateGatedInversionIgnoresNonCandidateMemberFields(t *testing.T) {
	chain := ChainResult{CausalImpacts: []WakeupCausalImpact{
		{PriorityInversionCandidate: true, PriorityRelationCaliber: string(priorityCaliberClosedRangeStable),
			PriorityRelationProvenLowerMs: 2, PriorityInversionGatedMs: 2, GatedRunnableMs: 2,
			Window: TimeWindow{StartTs: 1.0, EndTs: 1.002}},
		{PriorityInversionCandidate: false, PriorityInversionGatedMs: 20, GatedRunnableMs: 20,
			Window: TimeWindow{StartTs: 1.010, EndTs: 1.030}},
	}}
	aggregate := WakeupCausalAggregate{}
	applyAggregateGatedInversion(&chain, &aggregate, []int{0, 1})
	if !near(aggregate.PriorityInversionGatedMs, 2, 0.000001) ||
		!near(aggregate.GatedRunnableMs, 2, 0.000001) || aggregate.GatedRunningDeficitMs != 0 {
		t.Fatalf("non-candidate gated-looking fields must never enter the aggregate: %+v", aggregate)
	}
}

func TestRootEvidenceOnlyRunnableAndDIOKeepTypedParticipation(t *testing.T) {
	target := ThreadRef{PID: 100, Comm: "app"}
	for _, tc := range []struct {
		typ string
	}{
		{typ: "runnable_wait"},
		{typ: "io_wait"},
		{typ: "d_state_or_io_wait"},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			chain := ChainResult{Target: target, RootEvidence: []RootEvidence{{
				Type: tc.typ, Thread: target, DurationMs: 5, LineStart: 10, LineEnd: 11, Confidence: 0.8,
			}}}
			rank := buildRootCauseRankFrom(nil, Query{PID: 100, TimeStart: 1, TimeEnd: 1.010}, chain,
				WindowStats{Window: TimeWindow{StartTs: 1, EndTs: 1.010}})
			if len(rank.Items) == 0 || rank.Items[0].Type != tc.typ ||
				!near(rootCauseEffectiveImpactMs(rank.Items[0]), 5, 0.000001) || rank.Items[0].Rank != 1 {
				t.Fatalf("RootEvidence-only exact state must preserve its typed caliber: %+v", rank.Items)
			}
		})
	}
}

func TestIOBurstIOWaitRefinementDoesNotAliasDStateDuration(t *testing.T) {
	thread := ThreadRef{PID: 200, Comm: "io-worker"}
	episodes := computeIOBurstEpisodes(WindowStats{
		IOWaitTop: []ThreadDuration{{Thread: thread, DurationMs: 1.062, StartTs: 5.001, EndTs: 5.002062, LineStart: 10, LineEnd: 11}},
	}, 8)
	if len(episodes) != 1 {
		t.Fatalf("expected one refined IO burst: %+v", episodes)
	}
	got := episodes[0]
	if got.DominantSignal != "scheduler_iowait" || got.DStateMs != 0 ||
		!near(got.IOWaitMs, 1.062, 0.000001) || !near(got.DStateMs+got.IOWaitMs, got.DurationMs, 0.000001) {
		t.Fatalf("IO-wait refines, rather than duplicates, the physical D-state interval: %+v", got)
	}
}

func TestMixedDIOPublishesOneFullFormalFamilyAndAbsorbsChurnTwin(t *testing.T) {
	thread := ThreadRef{PID: 200, Comm: "io-worker"}
	stats := WindowStats{
		Window: TimeWindow{StartTs: 5, EndTs: 5.010},
		DStateTop: []ThreadDuration{{Thread: thread, DurationMs: 3, CPU: 1,
			StartTs: 5.001, EndTs: 5.004, LineStart: 10, LineEnd: 11}},
		IOWaitTop: []ThreadDuration{{Thread: thread, DurationMs: 2, CPU: 1,
			StartTs: 5.005, EndTs: 5.007, LineStart: 20, LineEnd: 21}},
		StateChurn: []ThreadStateChurnSummary{{Thread: thread, DominantState: string(StateIOWait),
			TotalMs: 5, DominantImpactMs: 2, DStateMs: 3, IOWaitMs: 2,
			LineStart: 10, LineEnd: 21, Confidence: 0.8}},
	}
	rank := buildRootCauseRankFrom(&Index{TimestampOrder: TraceTimestampOrderMonotonic}, Query{TimeStart: 5, TimeEnd: 5.010}, ChainResult{}, stats)
	if len(rank.Items) != 1 {
		t.Fatalf("mixed D/IO must own one formal active seat: active=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
	item := rank.Items[0]
	if item.Type != "d_state_or_io_wait" || !near(item.DStateMs, 3, 0.000001) ||
		!near(item.IOWaitMs, 2, 0.000001) || !near(rootCauseEffectiveImpactMs(item), 5, 0.000001) ||
		item.Rank != 1 || item.MemberCount != 2 || item.MemberFoldCaliber != RootCauseMemberFoldCaliberSumDisjoint {
		t.Fatalf("formal D/IO family must participate by the full mutually-exclusive sum: %+v", item)
	}
	if len(rank.AbsorbedItems) != 1 || rank.AbsorbedItems[0].Type != "fragmented_d_state_or_io_wait" {
		t.Fatalf("the exact StateChurn projection must remain lossless without a second vote: %+v", rank.AbsorbedItems)
	}

	unproven := stats
	unproven.StateChurn = nil
	rank = buildRootCauseRankFrom(&Index{TimestampOrder: TraceTimestampOrderUnknown}, Query{TimeStart: 5, TimeEnd: 5.010}, ChainResult{}, unproven)
	if len(rank.Items) != 1 || !near(rootCauseEffectiveImpactMs(rank.Items[0]), 3, 0.000001) ||
		rank.Items[0].MemberFoldCaliber != RootCauseMemberFoldCaliberMaxOverlapFallback ||
		!near(rank.Items[0].MemberSumMs, 5, 0.000001) {
		t.Fatalf("unknown timestamp order cannot prove D/IO member additivity: %+v", rank.Items)
	}
}

func TestRootCauseSortAndTierUseEffectiveNotRawOrScore(t *testing.T) {
	zeroRunning := RootCauseRankItem{
		Type: "running", Thread: ThreadRef{PID: 200, Comm: "renamed-worker"},
		DominantState: "running", RunningMs: 100, ImpactMs: 100, CumulativeImpactMs: 100,
		Score: 1000, ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
	}
	runnable := RootCauseRankItem{
		Type: "runnable_wait", Thread: ThreadRef{PID: 300, Comm: "app"},
		DominantState: "runnable", RunnableMs: 2, ImpactMs: 2, CumulativeImpactMs: 2,
		Score: 1, ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
	}
	items := []RootCauseRankItem{zeroRunning, runnable}
	sortRootCauseRankItems(items, true)
	if items[0].Thread.PID != 300 {
		t.Fatalf("positive effective must outrank raw/Score-heavy zero running: %+v", items)
	}
	assignRootCauseRanksAndTiers(items)
	if items[0].Tier != "primary" || items[0].Rank != 1 {
		t.Fatalf("positive on-chain runnable must win the strict positional election: %+v", items)
	}
	if items[1].Tier != RootCauseTierContextOnly || items[1].Rank != 0 {
		t.Fatalf("effective=0 running must stay rank-0 context: %+v", items)
	}
}

func TestRunningLowFrequencyVerdictDoesNotMintRawDurationSeat(t *testing.T) {
	stats := WindowStats{
		Window: TimeWindow{StartTs: 5, EndTs: 5.1},
		ComputeSupply: []ComputeSupplySummary{
			{Thread: ThreadRef{PID: 200, Comm: "worker-old"}, State: "running", DurationMs: 40,
				Verdict: "low_frequency_signal", Confidence: 0.7},
			{Thread: ThreadRef{PID: 300, Comm: "waiter"}, State: "runnable", DurationMs: 4,
				Verdict: "low_frequency_signal", Confidence: 0.7},
		},
	}
	rank := enrichRootCauseRankWithScheduler(Query{TimeStart: 5, TimeEnd: 5.1}, RootCauseRankResult{}, SchedulerLatencyResult{}, stats, ChainResult{})
	for _, item := range rank.Items {
		if item.Type == "low_frequency" && item.Thread.PID == 200 {
			t.Fatalf("running low_frequency raw DurationMs must remain diagnostic-only, got rank seat %+v", item)
		}
	}
	foundRunnable := false
	for _, item := range rank.Items {
		if item.Type == "low_frequency" && item.Thread.PID == 300 {
			foundRunnable = true
			if got := rootCauseEffectiveImpactMs(item); !near(got, 4, 0.000001) {
				t.Fatalf("runnable low_frequency uses its runnable caliber, got %.6f: %+v", got, item)
			}
		}
	}
	if !foundRunnable {
		t.Fatalf("control runnable verdict must remain visible: %+v", rank.Items)
	}
}
