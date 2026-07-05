package tracequery

import (
	"fmt"
	"strings"
	"testing"
)

// PTS-2 (#69 用户条件裁定 2026-07-06, 账本 real_trace_campaign_20260705.md §7.1):
// the engine aggregate top-8 trim folds its rank>8 overflow into ONE bounded
// synthetic member (ChainResult.AggregatedImpactsFold) instead of a
// caveat-only disclosure. These pins cover both adjudicated faces: >8 groups →
// fold present with correct count/range/roster (MAX never a sum — wall clock
// never sums across threads); ≤8 groups → nil field, zero emission
// (anti-noise), caveat lane byte-identical.

// buildPTS2AggregateOverflowChain builds a chain whose CausalImpacts collapse
// into exactly `groups` distinct (PID, dominant-state) aggregate pairs, each
// with 2 occurrences; group i's DominantImpactMs is groups-i (descending so
// the engine sort ranks group 0 first).
func buildPTS2AggregateOverflowChain(groups int) ChainResult {
	chain := ChainResult{Target: ThreadRef{PID: 9000, Comm: "target"}}
	for g := 0; g < groups; g++ {
		for occ := 0; occ < 2; occ++ {
			start := 100.0 + float64(g)*0.010 + float64(occ)*0.002
			chain.CausalImpacts = append(chain.CausalImpacts, WakeupCausalImpact{
				Thread:           ThreadRef{PID: 1000 + g, Comm: fmt.Sprintf("waker-%02d", g)},
				ChainDepth:       1,
				OnChain:          true,
				DominantState:    string(StateSSleep),
				DominantImpactMs: float64(groups - g),
				TotalMs:          float64(groups - g),
				SleepMs:          float64(groups - g),
				TargetBlockedMs:  float64(groups - g),
				FragmentCount:    1,
				Window:           TimeWindow{StartTs: start, EndTs: start + 0.001},
				LineStart:        1000 + g*10 + occ,
				LineEnd:          1001 + g*10 + occ,
			})
		}
	}
	return chain
}

func TestAggregateTopEightOverflowSynthesizesBoundedFoldMember(t *testing.T) {
	const groups = 12 // 4 beyond the top-8 trim
	chain := buildPTS2AggregateOverflowChain(groups)
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if len(aggregates) != 8 {
		t.Fatalf("top-8 trim must keep exactly 8 aggregates, got %d", len(aggregates))
	}
	fold := chain.AggregatedImpactsFold
	if fold == nil {
		t.Fatalf("rank>8 overflow must synthesize the bounded fold member")
	}
	if fold.Groups != groups-8 {
		t.Fatalf("fold must count the folded GROUPS: got %d want %d", fold.Groups, groups-8)
	}
	// Ranked by DominantImpactMs desc: each group's display value is its own
	// two occurrences summed (within-pair aggregation semantics), so the
	// folded groups (per-occ 4..1) carry 8/6/4/2 → range 2–8. The fold's
	// headline is the member MAX (8), never a sum across the folded groups
	// (a sum would claim 20ms of wall clock across four threads).
	if fold.MinImpactMs != 2 || fold.MaxImpactMs != 8 {
		t.Fatalf("fold range must be the members' min–max display values (MAX never a cross-member sum): %+v", fold)
	}
	if len(fold.Subjects) != groups-8 {
		t.Fatalf("roster must list every folded subject when ≤8: %+v", fold.Subjects)
	}
	for _, subject := range fold.Subjects {
		if !strings.HasPrefix(subject, "waker-") {
			t.Fatalf("roster entries must be member thread labels: %+v", fold.Subjects)
		}
	}
	if fold.LineStart <= 0 || fold.LineEnd < fold.LineStart || fold.FirstTs <= 0 || fold.LastTs < fold.FirstTs {
		t.Fatalf("fold envelope must span the members' line/ts extents: %+v", fold)
	}
	// The PTS top-8 caveat stays byte-identical next to the fold.
	var caveat string
	for _, c := range chain.Caveats {
		if strings.Contains(c, "aggregated_impacts kept top 8") {
			caveat = c
		}
	}
	want := fmt.Sprintf("aggregated_impacts kept top 8 of %d (derived view; per-hop causal impact rows remain complete)", groups)
	if caveat != want {
		t.Fatalf("top-8 caveat must stay byte-identical: got %q want %q", caveat, want)
	}
}

func TestAggregateFoldRosterBoundedAtEightSubjects(t *testing.T) {
	const groups = 20 // 12 beyond the trim — roster must cap at 8
	chain := buildPTS2AggregateOverflowChain(groups)
	if got := aggregateWakeupCausalImpacts(&chain); len(got) != 8 {
		t.Fatalf("top-8 trim must keep exactly 8 aggregates, got %d", len(got))
	}
	fold := chain.AggregatedImpactsFold
	if fold == nil || fold.Groups != groups-8 {
		t.Fatalf("fold must count all folded groups: %+v", fold)
	}
	if len(fold.Subjects) != 8 {
		t.Fatalf("roster is bounded at 8 labels (O(1) memory): got %d", len(fold.Subjects))
	}
}

// PTS-2 F2 pin (复核 2026-07-06): aggregate groups are keyed (PID, state) —
// ONE thread overflowing with TWO dominant states counts 2 folded groups but
// occupies exactly ONE roster slot (dedupe mirrors the wire-cap fold).
func TestAggregateFoldRosterDedupesSameThreadAcrossStates(t *testing.T) {
	chain := buildPTS2AggregateOverflowChain(8) // 8 leading groups fill the cap
	for _, state := range []ThreadState{StateSSleep, StateRunnable} {
		for occ := 0; occ < 2; occ++ {
			start := 200.0 + float64(occ)*0.002
			chain.CausalImpacts = append(chain.CausalImpacts, WakeupCausalImpact{
				Thread:           ThreadRef{PID: 2000, Comm: "dual-state"},
				ChainDepth:       1,
				OnChain:          true,
				DominantState:    string(state),
				DominantImpactMs: 0.25,
				TotalMs:          0.25,
				TargetBlockedMs:  0.25,
				FragmentCount:    1,
				Window:           TimeWindow{StartTs: start, EndTs: start + 0.001},
				LineStart:        5000 + occ,
				LineEnd:          5001 + occ,
			})
		}
	}
	if got := aggregateWakeupCausalImpacts(&chain); len(got) != 8 {
		t.Fatalf("top-8 trim must keep exactly 8 aggregates, got %d", len(got))
	}
	fold := chain.AggregatedImpactsFold
	if fold == nil || fold.Groups != 2 {
		t.Fatalf("both (PID,state) groups of the dual-state thread must fold: %+v", fold)
	}
	if len(fold.Subjects) != 1 || fold.Subjects[0] != "dual-state-2000" {
		t.Fatalf("one thread across two states must occupy exactly one roster slot: %+v", fold.Subjects)
	}
}

func TestAggregateAtOrUnderCapEmitsNoFoldMember(t *testing.T) {
	// 突变形态: exactly at the cap (8 groups) and under it (5 groups) the
	// field stays nil — 零发射反噪音 — and no fold caveat appears.
	for _, groups := range []int{5, 8} {
		chain := buildPTS2AggregateOverflowChain(groups)
		aggregates := aggregateWakeupCausalImpacts(&chain)
		if len(aggregates) != groups {
			t.Fatalf("groups=%d: aggregates must all survive, got %d", groups, len(aggregates))
		}
		if chain.AggregatedImpactsFold != nil {
			t.Fatalf("groups=%d: ≤8 groups must not synthesize a fold member: %+v", groups, chain.AggregatedImpactsFold)
		}
		for _, c := range chain.Caveats {
			if strings.Contains(c, "kept top 8") {
				t.Fatalf("groups=%d: no trim caveat may appear under the cap: %q", groups, c)
			}
		}
	}
}
