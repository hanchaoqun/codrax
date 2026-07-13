package tracequery

// wakeup_aggregate_fold_tie_p21_test.go — P2-1 pins (第四跨线程取最大点,
// §29.6 G12-ENG batch, real_trace_campaign_20260705.md, 2026-07-09): the
// engine aggregate top-8 trim fold retains a µs-tie member roster
// ((label, line-range), cap 4, strict types.TraceCausalProjectionSameValueTieMS
// band — the same ruler as the three DIAG-A1 take-MAX points), so a ×2
// same-value trim shape is disclosable instead of unverifiable.
//
// MUTATION self-checks:
//   - dropping the wakeupCausalAggregateFoldTies call reds
//     TestP21AggregateFoldTieRosterOnSameValueOverflow;
//   - loosening the ≥2-tie discipline reds
//     TestP21AggregateFoldNoRosterOnDistinctValues (a lone max would mint).

import "testing"

// buildP21TieOverflowChain: 8 leading groups (occurrence sums 40..26ms) fill
// the top-8 board; two folded groups tie at 4.577ms to the µs; one more
// folded group carries a distinct 1.9ms.
func buildP21TieOverflowChain() ChainResult {
	chain := ChainResult{Target: ThreadRef{PID: 9000, Comm: "target"}}
	for g := 0; g < 8; g++ {
		for occ := 0; occ < 2; occ++ {
			start := 100.0 + float64(g)*0.010 + float64(occ)*0.002
			v := float64(20 - g)
			chain.CausalImpacts = append(chain.CausalImpacts, WakeupCausalImpact{
				Thread:           ThreadRef{PID: 1000 + g, Comm: "lead-waker"},
				ChainDepth:       1,
				OnChain:          true,
				DominantState:    string(StateSSleep),
				DominantImpactMs: v,
				TotalMs:          v,
				SleepMs:          v,
				TargetBlockedMs:  v,
				FragmentCount:    1,
				Window:           TimeWindow{StartTs: start, EndTs: start + 0.001},
				LineStart:        1000 + g*10 + occ,
				LineEnd:          1001 + g*10 + occ,
			})
		}
	}
	for g, v := range map[int]float64{100: 4.577, 101: 4.577, 102: 1.9} {
		for occ := 0; occ < 2; occ++ {
			start := 200.0 + float64(g)*0.010 + float64(occ)*0.002
			chain.CausalImpacts = append(chain.CausalImpacts, WakeupCausalImpact{
				Thread:           ThreadRef{PID: 2000 + g, Comm: "tie-waker"},
				ChainDepth:       1,
				OnChain:          true,
				DominantState:    string(StateSSleep),
				DominantImpactMs: v / 2,
				TotalMs:          v / 2,
				SleepMs:          v / 2,
				TargetBlockedMs:  v / 2,
				FragmentCount:    1,
				Window:           TimeWindow{StartTs: start, EndTs: start + 0.001},
				LineStart:        5000 + g*10 + occ,
				LineEnd:          5001 + g*10 + occ,
			})
		}
	}
	// The leading groups' per-occurrence values are 12..5 → group sums 24..10,
	// all above 4.577, so the three tie/distinct groups land in the overflow.
	return chain
}

func TestP21AggregateFoldTieRosterOnSameValueOverflow(t *testing.T) {
	chain := buildP21TieOverflowChain()
	if got := aggregateWakeupCausalImpacts(&chain); len(got) != 8 {
		t.Fatalf("top-8 trim expected, got %d aggregates", len(got))
	}
	fold := chain.AggregatedImpactsFold
	if fold == nil || fold.Groups != 3 {
		t.Fatalf("three overflow groups expected on the fold: %+v", fold)
	}
	if fold.MaxImpactMs != 4.577 {
		t.Fatalf("fold max must be the tied member value: %.3f", fold.MaxImpactMs)
	}
	if len(fold.SameValueMembers) != 2 {
		t.Fatalf("n=2 same-value trim shape must disclose BOTH tied members: %+v", fold.SameValueMembers)
	}
	for _, tie := range fold.SameValueMembers {
		if tie.Label == "" || tie.LineStart <= 0 || tie.LineEnd < tie.LineStart {
			t.Fatalf("each tie entry carries (label, line-range): %+v", tie)
		}
	}
	// Zero weight: the published fold accounting is untouched by the roster.
	if fold.MinImpactMs != 1.9 || fold.MaxImpactMs != 4.577 || fold.Groups != 3 {
		t.Fatalf("tie roster must not edit the fold values: %+v", fold)
	}
}

// TestP21TieMicroValueBandGuard (复核 P2-3): the v<=0 guard's ONE load-bearing
// corner is the micro-value band — with max below the tie band
// (max=0.0003 < 0.0005) a ZERO-value member sits |0−max| inside the band and
// would enter the roster as a fabricated tie witness. The guard keeps it out;
// the two REAL 0.0003 members still disclose. Deleting the v<=0 guard reds
// this pin (mutation-verified).
func TestP21TieMicroValueBandGuard(t *testing.T) {
	overflow := []WakeupCausalAggregate{
		{Thread: ThreadRef{PID: 1, Comm: "micro-a"}, DominantImpactMs: 0.0003, LineStart: 10, LineEnd: 11},
		{Thread: ThreadRef{PID: 2, Comm: "micro-b"}, DominantImpactMs: 0.0003, LineStart: 20, LineEnd: 21},
		{Thread: ThreadRef{PID: 3, Comm: "zero-c"}, DominantImpactMs: 0, LineStart: 30, LineEnd: 31},
	}
	ties := wakeupCausalAggregateFoldTies(overflow, 0.0003)
	if len(ties) != 2 {
		t.Fatalf("exactly the two real 0.0003 members tie, got %+v", ties)
	}
	for _, tie := range ties {
		if tie.LineStart == 30 {
			t.Fatalf("a zero-value member must never enter the micro-band roster: %+v", ties)
		}
	}
}

func TestP21AggregateFoldNoRosterOnDistinctValues(t *testing.T) {
	const groups = 12
	chain := buildPTS2AggregateOverflowChain(groups)
	if got := aggregateWakeupCausalImpacts(&chain); len(got) != 8 {
		t.Fatalf("top-8 trim expected, got %d aggregates", len(got))
	}
	fold := chain.AggregatedImpactsFold
	if fold == nil {
		t.Fatal("fold expected")
	}
	if fold.SameValueMembers != nil {
		t.Fatalf("distinct member values must mint NO tie roster (a lone max is just the max): %+v", fold.SameValueMembers)
	}
}
