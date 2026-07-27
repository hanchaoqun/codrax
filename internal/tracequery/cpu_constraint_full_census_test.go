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

// 批己 (§15.12 PIN-HYG): the remaining two census consumers gain judgment
// power — mutating either back to the capped public slice must go red.

func TestCPUConstraintFullCensusReachesDrillUniverse(t *testing.T) {
	public := []CPUConstraintSummary{{Thread: ThreadRef{Comm: "d1", PID: 1}}}
	ninth := CPUConstraintSummary{Thread: ThreadRef{Comm: "beyond", PID: 909}}
	stats := WindowStats{
		CPUConstraints:      public,
		cpuConstraintCensus: append(append([]CPUConstraintSummary(nil), public...), ninth),
	}
	universe := buildDrillSubjectUniverse(nil, &stats)
	if !universe.pids[909] {
		t.Fatalf("a census constraint beyond the display cap must stay drilled: %+v", universe.pids)
	}
}

func TestCPUConstraintFullCensusReachesRunnableContext(t *testing.T) {
	// Direct wiring-shape pin: the runnable-context join consumes the FULL
	// census (the ComputeWindowStats call site passes
	// cpuConstraintAccounting(stats)); a thread whose constraint sits beyond
	// the public display cap must still receive its constraint evidence.
	items := []SchedulerLatencyItem{{Thread: ThreadRef{Comm: "beyond", PID: 909}, DurationMs: 9}}
	public := []CPUConstraintSummary{{Thread: ThreadRef{Comm: "d1", PID: 1}}}
	ninth := CPUConstraintSummary{Thread: ThreadRef{Comm: "beyond", PID: 909}, RunnableWaitMs: 9, AllowedCPUs: []int{0, 1}}
	stats := WindowStats{
		CPUConstraints:      public,
		cpuConstraintCensus: append(append([]CPUConstraintSummary(nil), public...), ninth),
	}
	contexts := computeRunnableContextSummaries(items, nil, nil, cpuConstraintAccounting(stats), 0)
	if len(contexts) != 1 || contexts[0].CPUConstraint == nil || contexts[0].CPUConstraint.Thread.PID != 909 {
		t.Fatalf("the census constraint beyond the display cap must join the runnable context: %+v", contexts)
	}
}
