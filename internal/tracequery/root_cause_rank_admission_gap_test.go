package tracequery

import (
	"math"
	"testing"
)

// These pins cover rank-admission invariants at the production construction
// funnels.  They intentionally avoid hand-constructing RootCauseRankItem rows:
// every row below is minted by buildRootCauseRankFrom/BuildRootCauseRank, just
// as it is in a real query.

func TestRootCauseRankExactRootEvidenceTwinOwnsOneSeat(t *testing.T) {
	dep := ThreadRef{Comm: "dependency", PID: 200}
	impact := WakeupCausalImpact{
		Thread:            dep,
		Window:            TimeWindow{StartTs: 5.001, EndTs: 5.009},
		ActualWindow:      TimeWindow{StartTs: 5.001, EndTs: 5.009},
		ChainDepth:        1,
		OnChain:           true,
		DominantState:     string(StateRunnable),
		DominantImpactMs:  8,
		ProjectedImpactMs: 8,
		ProjectedTotalMs:  8,
		ActualImpactMs:    8,
		ActualTotalMs:     8,
		TotalMs:           8,
		RunnableMs:        8,
		ActualRunnableMs:  8,
		TargetBlockedMs:   8,
		LineStart:         20,
		LineEnd:           30,
		Summary:           "dependency runnable occurrence",
	}
	chain := ChainResult{
		Target: ThreadRef{Comm: "app", PID: 100},
		Nodes: []ChainNode{
			{ID: "target", Thread: ThreadRef{Comm: "app", PID: 100}, Window: TimeWindow{StartTs: 5.000, EndTs: 5.010}},
			{ID: "dep", Thread: dep, Window: impact.Window, Impact: &impact, Depth: 1},
		},
		CausalImpacts: []WakeupCausalImpact{impact},
		// This is the exact production expandChain shape: the evidence row is
		// derived from the very same WakeupCausalImpact object.
		RootEvidence: []RootEvidence{
			rootEvidenceFromCausalImpact(impact, "dependency runnable occurrence", 0.8),
		},
	}

	rank := buildRootCauseRankFrom(nil, Query{TimeStart: 5.000, TimeEnd: 5.010, Limit: 12}, chain, WindowStats{})
	var carriers []RootCauseRankItem
	for _, item := range rank.Items {
		if item.Thread.PID == dep.PID && item.LineStart == impact.LineStart && item.LineEnd == impact.LineEnd &&
			(item.Type == "runnable_wait" || item.Type == "priority_inversion_runnable_wait" || item.Type == "priority_inversion_candidate") {
			carriers = append(carriers, item)
		}
	}
	if len(carriers) != 1 {
		t.Fatalf("one physical wakeup-chain occurrence must own one active rank seat; got %d carriers: %+v", len(carriers), carriers)
	}
}

func TestRootCauseRankOverlappingAggregateCannotExceedOccurrenceWindowUnion(t *testing.T) {
	dep := ThreadRef{Comm: "dependency", PID: 200}
	occurrence := func(start, end float64, line int) WakeupCausalImpact {
		ms := (end - start) * 1000
		return WakeupCausalImpact{
			Thread:            dep,
			Window:            TimeWindow{StartTs: start, EndTs: end},
			ActualWindow:      TimeWindow{StartTs: start, EndTs: end},
			ChainDepth:        1,
			OnChain:           true,
			DominantState:     string(StateRunnable),
			DominantImpactMs:  ms,
			ProjectedImpactMs: ms,
			ProjectedTotalMs:  ms,
			ActualImpactMs:    ms,
			ActualTotalMs:     ms,
			TotalMs:           ms,
			RunnableMs:        ms,
			ActualRunnableMs:  ms,
			TargetBlockedMs:   ms,
			LineStart:         line,
			LineEnd:           line + 1,
		}
	}
	// Two 10ms full-occupancy occurrences overlap by 5ms.  Their physical
	// window union is 15ms, so a 20ms aggregate rank value is impossible.
	first := occurrence(10.000, 10.010, 20)
	second := occurrence(10.005, 10.015, 40)
	chain := ChainResult{
		Target:        ThreadRef{Comm: "app", PID: 100},
		Nodes:         []ChainNode{{ID: "target", Thread: ThreadRef{Comm: "app", PID: 100}, Window: TimeWindow{StartTs: 10.000, EndTs: 10.015}}, {ID: "dep", Thread: dep, Window: TimeWindow{StartTs: 10.000, EndTs: 10.015}, Depth: 1}},
		CausalImpacts: []WakeupCausalImpact{first, second},
	}
	chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
	rank := buildRootCauseRankFrom(nil, Query{TimeStart: 10.000, TimeEnd: 10.015, Limit: 12}, chain, WindowStats{})

	var aggregate *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Thread.PID == dep.PID && rank.Items[i].Source == "wakeup_chain.aggregated_impacts" {
			aggregate = &rank.Items[i]
			break
		}
	}
	if aggregate == nil {
		t.Fatalf("fixture must mint the production aggregate rank carrier: %+v", rank.Items)
	}
	const unionMs = 15.0
	if aggregate.ImpactMs > unionMs+0.001 || aggregate.CumulativeImpactMs > unionMs+0.001 || rootCauseEffectiveImpactMs(*aggregate) > unionMs+0.001 {
		t.Fatalf("overlapping occurrence aggregate must not exceed the 15ms physical window union: %+v", aggregate)
	}
	if got := chain.rankAggregateCensus[0].AggregationCaliber; got != WakeupAggregateCaliberOverlapSafe {
		t.Fatalf("overlap-safe aggregate must disclose its union/MAX caliber, got %q", got)
	}
}

func TestRootCauseRankZeroRankSideRowsDoNotConsumeCandidateLimit(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	chain := ChainResult{Target: target}
	chain.Nodes = append(chain.Nodes, ChainNode{ID: "target", Thread: target, Window: TimeWindow{StartTs: 1, EndTs: 2}})
	// High-valued self wait symptoms sort before the true candidates, but they
	// are Rank=0 side-lane disclosures and therefore must not spend the three
	// candidate seats requested below.
	chain.RootEvidence = append(chain.RootEvidence,
		RootEvidence{Type: "binder_wait", Thread: target, DurationMs: 100, LineStart: 10, LineEnd: 11, Confidence: 0.9},
		RootEvidence{Type: "binder_wait", Thread: target, DurationMs: 90, LineStart: 12, LineEnd: 13, Confidence: 0.9},
	)
	for i, ms := range []float64{80, 70, 60, 50} {
		peer := ThreadRef{Comm: "peer", PID: 200 + i}
		chain.Nodes = append(chain.Nodes, ChainNode{ID: "peer", Thread: peer, Window: TimeWindow{StartTs: 1, EndTs: 2}, Depth: 1})
		chain.RootEvidence = append(chain.RootEvidence, RootEvidence{
			Type: "binder_wait", Thread: peer, DurationMs: ms,
			LineStart: 20 + i*2, LineEnd: 21 + i*2, Confidence: 0.9,
		})
	}

	rank := buildRootCauseRankFrom(nil, Query{TimeStart: 1, TimeEnd: 2, Limit: 3}, chain, WindowStats{})
	competing := 0
	for _, item := range rank.Items {
		if item.Rank > 0 {
			competing++
		}
	}
	if competing != 3 {
		t.Fatalf("Rank=0 side rows must not consume candidate limit=3; got %d competing rows: %+v", competing, rank.Items)
	}
}

func TestRootCauseRankBlockIOCarryInRanksOnlyWindowProjection(t *testing.T) {
	idx := buildTraceIndex(t, "block_io_carry_in_projection.systrace", `
 io-40 (40) [003] .... 4.900000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
irq-2 (2) [003] .... 5.005000: block_rq_complete: 8,0 R () 123 + 8 [0]
`)
	rank := BuildRootCauseRank(idx, Query{TimeStart: 5.000, TimeEnd: 5.010, Limit: 12})
	var ioRow *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "io_latency" {
			ioRow = &rank.Items[i]
			break
		}
	}
	if ioRow == nil {
		t.Fatalf("carry-in issue completed in-window must remain visible: %+v", rank.Items)
	}
	// Physical latency is 105ms, but only [5.000,5.005] intersects the selected
	// window.  The rank participation/projected channels must use that 5ms.
	if math.Abs(ioRow.ImpactMs-5) > 0.001 || math.Abs(ioRow.ProjectedImpactMs-5) > 0.001 || math.Abs(rootCauseEffectiveImpactMs(*ioRow)-5) > 0.001 {
		t.Fatalf("block IO carry-in must rank by its 5ms in-window projection, not the 105ms physical extent: %+v", ioRow)
	}
}
