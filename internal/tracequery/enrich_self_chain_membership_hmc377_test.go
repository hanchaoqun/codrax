package tracequery

import "testing"

// R8 (§29.93): the analysed thread is always on its own chain. A single-thread
// chain whose published rows wear no chain token still has the target on it:
// the target's own scheduler rows stay on the chain and every other thread's
// rows are background. Integration witness (HMC377): with the own-work fix
// (the target's own span no longer wears a fabricated chain overlap) and the
// dependency-credit bound (a host-edge seat outside the wait its wakeup ended
// leaves the chain) the gallery board had no token-wearing row left; the
// fallback set then fell empty and a background thread's 76.827 ms
// CPU-restriction row ranked first with no chain lane while the target's own
// 1.845 ms restriction row lost its on-chain identity.
func TestHMC377TargetStaysOnItsOwnChainWithoutAnyChainToken(t *testing.T) {
	target := ThreadRef{Comm: "app.main", PID: 100}
	other := ThreadRef{Comm: "HeapTaskDaemon", PID: 200}
	constraint := func(thread ThreadRef, ms float64, line int) CPUConstraintSummary {
		return CPUConstraintSummary{
			Thread:               thread,
			AllowedCPUs:          []int{0, 1},
			AllowedCPUsAuthority: CPUConstraintAllowedCPUsAuthorityKernelNextInfo,
			RestrictionProof:     CPUConstraintRestrictionProofAllowedMaskExcludesUniverse,
			ExcludedCPUs:         []int{4},
			RunnableWaitMs:       ms,
			LineStart:            line,
			LineEnd:              line + 1,
			Summary:              thread.Comm + " restricted",
		}
	}
	census := []CPUConstraintSummary{constraint(other, 76.827, 10), constraint(target, 1.845, 20)}
	stats := WindowStats{Window: TimeWindow{StartTs: 1, EndTs: 1.1}, CPUConstraints: census, cpuConstraintCensus: census}
	chain := ChainResult{Target: target, Nodes: []ChainNode{{ID: "t", Thread: target, Window: TimeWindow{StartTs: 1, EndTs: 1.1}}}}
	// The board's only other row is the target's own work on a self token.
	own := rootCauseItem("gc_pause", target, 75.656, 0.8, 30, 40, "window_stats.trace_spans", "own collection")
	own.Causality = RootCauseCausalitySelfDeterministic
	rank := enrichRootCauseRankWithScheduler(Query{PID: 100, TimeStart: 1, TimeEnd: 1.1, Limit: 16},
		RootCauseRankResult{Target: target, Items: []RootCauseRankItem{own}}, SchedulerLatencyResult{}, stats, chain)
	seen := 0
	for _, item := range rank.Items {
		if item.Type != "cpu_affinity_or_cpuset" {
			continue
		}
		seen++
		switch item.Thread.PID {
		case target.PID:
			if item.ChainRelevance != "on_chain" {
				t.Fatalf("the target's own CPU-restriction row must stay on its own chain: %+v", item)
			}
		case other.PID:
			if item.ChainRelevance != "background" {
				t.Fatalf("another thread's row is background when only the target is on the chain: %+v", item)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("fixture: both restriction rows must mint, got %d: %+v", seen, rank.Items)
	}
}
