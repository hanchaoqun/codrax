package tracequery

import "testing"

func TestWakeupAggregateOverlapKeepsCoupledSupplyVectorFromOneOccurrence(t *testing.T) {
	thread := ThreadRef{PID: 200, Comm: "worker"}
	member := func(start, end float64, line, fragments, switches int, known, unknown, ideal, deficit float64) WakeupCausalImpact {
		total := (end - start) * 1000
		return WakeupCausalImpact{
			Thread:              thread,
			ChainDepth:          1,
			DominantState:       string(StateRunning),
			DominantImpactMs:    total,
			TotalMs:             total,
			ProjectedTotalMs:    total,
			RunningMs:           total,
			FragmentCount:       fragments,
			StateSwitches:       switches,
			Window:              TimeWindow{StartTs: start, EndTs: end},
			LineStart:           line,
			LineEnd:             line + 1,
			SupplyFoldIdealMs:   ideal,
			SupplyFoldDeficitMs: deficit,
			SupplyFoldBasis: &SupplyFoldBasis{
				KnownMs:          known,
				UnknownMs:        unknown,
				CapabilitySource: CoreCapabilitySourceDefault,
			},
		}
	}
	chain := ChainResult{CausalImpacts: []WakeupCausalImpact{
		// This 10ms member is the strongest representative of the overlap
		// cohort. Its duration, counts and supply vector must travel together.
		member(10.000, 10.010, 10, 2, 1, 6, 4, 8, 2),
		member(10.005, 10.013, 20, 9, 7, 8, 0, 6, 2),
		// A disjoint component remains additive.
		member(10.020, 10.024, 30, 1, 0, 3, 1, 3, 1),
	}}
	chain.CausalImpacts[0].MaxSegmentMs = 7
	chain.CausalImpacts[1].MaxSegmentMs = 8
	chain.CausalImpacts[2].MaxSegmentMs = 3
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if len(aggregates) != 1 {
		t.Fatalf("expected one aggregate, got %+v", aggregates)
	}
	agg := aggregates[0]
	if agg.AggregationCaliber != WakeupAggregateCaliberOverlapSafe {
		t.Fatalf("overlap must be disclosed: %+v", agg)
	}
	if !near(agg.TotalMs, 14, 0.001) || !near(agg.RunningMs, 14, 0.001) {
		t.Fatalf("selected 10ms vector + disjoint 4ms vector expected: %+v", agg)
	}
	if agg.FragmentCount != 3 || agg.StateSwitches != 1 {
		t.Fatalf("counts must come from the same selected vectors, not independent maxima: %+v", agg)
	}
	if !near(agg.MaxSegmentMs, 7, 0.001) {
		t.Fatalf("max segment must be recomputed from selected vectors, not retained from an excluded member: %+v", agg)
	}
	if agg.SupplyFoldBasis == nil {
		t.Fatalf("selected folded vectors must retain their basis: %+v", agg)
	}
	if !near(agg.SupplyFoldBasis.KnownMs, 9, 0.001) || !near(agg.SupplyFoldBasis.UnknownMs, 5, 0.001) ||
		!near(agg.SupplyFoldIdealMs, 11, 0.001) || !near(agg.SupplyFoldDeficitMs, 3, 0.001) {
		t.Fatalf("supply vector must be selected and added as a unit: %+v basis=%+v", agg, agg.SupplyFoldBasis)
	}
	if !near(agg.SupplyFoldBasis.KnownMs+agg.SupplyFoldBasis.UnknownMs, agg.RunningMs, 0.001) ||
		!near(agg.SupplyFoldIdealMs+agg.SupplyFoldDeficitMs, agg.RunningMs, 0.001) {
		t.Fatalf("aggregate supply identities must remain mechanically true: %+v basis=%+v", agg, agg.SupplyFoldBasis)
	}
}

func TestWakeupAggregateExactUnionKeepsProjectedAndActualStateBucketsSeparate(t *testing.T) {
	thread := ThreadRef{PID: 200, Comm: "worker"}
	member := func(line int) WakeupCausalImpact {
		return WakeupCausalImpact{
			Thread: thread, ChainDepth: 1,
			Window: TimeWindow{StartTs: 20.000, EndTs: 20.010}, ActualWindow: TimeWindow{StartTs: 20.000, EndTs: 20.010},
			DominantState: string(StateRunning), DominantImpactMs: 10,
			TotalMs: 10, ProjectedTotalMs: 10, RunningMs: 10,
			ActualTotalMs: 10, ActualRunnableMs: 10, MaxSegmentMs: 10,
			LineStart: line, LineEnd: line + 1,
		}
	}
	chain := ChainResult{CausalImpacts: []WakeupCausalImpact{member(10), member(20)}}
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if len(aggregates) != 1 {
		t.Fatalf("expected one aggregate, got %+v", aggregates)
	}
	agg := aggregates[0]
	if !near(agg.RunningMs, 10, 0.001) || agg.RunnableMs != 0 {
		t.Fatalf("projected union must remain in the projected running bucket: %+v", agg)
	}
	if !near(agg.ActualRunnableMs, 10, 0.001) || agg.ActualRunningMs != 0 {
		t.Fatalf("actual union must use its own typed runnable state, not projected dominant_state: %+v", agg)
	}
	if !near(agg.MaxSegmentMs, 10, 0.001) {
		t.Fatalf("exact connected union must publish its continuous segment extent: %+v", agg)
	}
}

func TestWakeupAggregateMissingWindowFallbackCannotBuildFrankensteinVector(t *testing.T) {
	thread := ThreadRef{PID: 200, Comm: "worker"}
	chain := ChainResult{CausalImpacts: []WakeupCausalImpact{
		{
			Thread: thread, ChainDepth: 1, DominantState: string(StateRunning),
			DominantImpactMs: 10, TotalMs: 10, RunningMs: 10,
			FragmentCount: 1, StateSwitches: 1, LineStart: 10,
		},
		{
			Thread: thread, ChainDepth: 1, DominantState: string(StateRunning),
			DominantImpactMs: 8, TotalMs: 8, RunningMs: 8,
			FragmentCount: 9, StateSwitches: 7, LineStart: 20,
		},
	}}
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if len(aggregates) != 1 {
		t.Fatalf("expected one aggregate, got %+v", aggregates)
	}
	agg := aggregates[0]
	if agg.AggregationCaliber != GatedCaliberMaxOverlapFallback {
		t.Fatalf("missing typed windows must use the global representative fallback: %+v", agg)
	}
	if agg.TotalMs != 10 || agg.RunningMs != 10 || agg.FragmentCount != 1 || agg.StateSwitches != 1 {
		t.Fatalf("fallback must copy one strongest complete vector, not fieldwise maxima: %+v", agg)
	}
}
