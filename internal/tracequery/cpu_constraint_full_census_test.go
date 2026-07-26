package tracequery

import "testing"

// NIAUTH-04 (§15.2/§15.4 batch E1): CPUConstraints is a display Top-N.
// A proven constrained runnable thread outside that roster must still reach
// the rank board through the full internal census.
func TestCPUConstraintRootRankReadsFullCensusBeyondDisplayCap(t *testing.T) {
	decoys := make([]CPUConstraintSummary, 8)
	for i := range decoys {
		decoys[i] = CPUConstraintSummary{
			Thread:          ThreadRef{PID: 100 + i},
			ConstraintCount: 100 - i,
		}
	}
	target := CPUConstraintSummary{
		Thread:               ThreadRef{PID: 999, Comm: "late-but-important"},
		AllowedCPUs:          []int{0, 1},
		AllowedCPUsAuthority: CPUConstraintAllowedCPUsAuthorityKernelNextInfo,
		RestrictionProof:     CPUConstraintRestrictionProofAllowedMaskExcludesUniverse,
		ExcludedCPUs:         []int{4},
		RunnableWaitMs:       40,
		LineStart:            90,
		LineEnd:              91,
		Summary:              "full-census constraint",
	}
	full := append(append([]CPUConstraintSummary(nil), decoys...), target)
	stats := WindowStats{
		Window:              TimeWindow{StartTs: 1, EndTs: 1.1},
		CPUConstraints:      capCPUConstraintDisplay(full, 8),
		cpuConstraintCensus: full,
	}
	if len(stats.CPUConstraints) != 8 || len(cpuConstraintAccounting(stats)) != 9 {
		t.Fatalf("display/accounting split drifted: display=%d census=%d", len(stats.CPUConstraints), len(cpuConstraintAccounting(stats)))
	}
	rank := enrichRootCauseRankWithScheduler(
		Query{TimeStart: 1, TimeEnd: 1.1, Limit: 16},
		RootCauseRankResult{Target: ThreadRef{PID: 1}},
		SchedulerLatencyResult{},
		stats,
		ChainResult{},
	)
	for _, item := range rank.Items {
		if item.Type == "cpu_affinity_or_cpuset" && item.Thread.PID == target.Thread.PID {
			if item.RunnableMs != target.RunnableWaitMs ||
				len(item.CPUConstraintExcludedCPUs) != 1 || item.CPUConstraintExcludedCPUs[0] != 4 {
				t.Fatalf("full-census seat lost its typed value/proof: %+v", item)
			}
			return
		}
	}
	t.Fatalf("the ninth proven constraint was lost behind the display cap: %+v", rank.Items)
}

func TestCPUConstraintAccountingLegacyFixtureFallsBackToPublicRows(t *testing.T) {
	public := []CPUConstraintSummary{{Thread: ThreadRef{PID: 7}, RunnableWaitMs: 3}}
	stats := WindowStats{CPUConstraints: public}
	got := cpuConstraintAccounting(stats)
	if len(got) != 1 || got[0].Thread.PID != 7 {
		t.Fatalf("nil internal census must preserve legacy/direct-literal fixtures: %+v", got)
	}
}
