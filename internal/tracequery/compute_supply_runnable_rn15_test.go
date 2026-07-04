package tracequery

import (
	"testing"
)

// RN-15 (§7.4 demand/supply separation ruling, §7.9) pins — the
// runnable×compute_supply double-publish fix.
//
// Customer live shape (cust_large_3s system supplement): the SAME per-thread
// runnable waits (2.661ms / 2.908ms) were published twice — once as
// type=runnable_wait, source=window_stats and once as type=compute_supply,
// source=window_stats.compute_supply — and the projection grew a phantom
// "影响点 compute_supply" row for a demand-side scheduling wait. The
// compute_supply causal token is reserved for the aggregate delivery-side
// ledger (supply ratio / low-frequency loss / idle mismatch / core-limited)
// and must never carry a concrete per-thread subject.

func rn15Stats(rows ...ComputeSupplySummary) WindowStats {
	return WindowStats{ComputeSupply: rows}
}

func countRankItems(rank RootCauseRankResult, match func(RootCauseRankItem) bool) int {
	n := 0
	for _, item := range rank.Items {
		if match(item) {
			n++
		}
	}
	return n
}

// TestEnrichRootCauseRank_PerThreadRunnableWaitPublishedOnceNotAsComputeSupply
// pins the customer shape end-state: after enrichment the 2.661/2.908ms
// runnable waits ride EXACTLY one candidate each (type=runnable_wait), and no
// compute_supply candidate with a concrete thread subject exists.
func TestEnrichRootCauseRank_PerThreadRunnableWaitPublishedOnceNotAsComputeSupply(t *testing.T) {
	ffrtA := ThreadRef{Comm: "OS_FFRT_2_3", PID: 49706}
	ffrtB := ThreadRef{Comm: "OS_FFRT_1_9", PID: 49691}
	q := Query{TimeStart: 0, TimeEnd: 3.0}

	// Seed the demand-side rows exactly as the window_stats rank producer
	// does (type=runnable_wait, source=window_stats).
	rank := RootCauseRankResult{Items: []RootCauseRankItem{
		rootCauseItem("runnable_wait", ffrtA, 2.661, 0.76, 100, 110, "window_stats", "OS_FFRT_2_3-49706 was runnable for 2.661ms"),
		rootCauseItem("runnable_wait", ffrtB, 2.908, 0.76, 120, 130, "window_stats", "OS_FFRT_1_9-49691 was runnable for 2.908ms"),
	}}
	// The per-thread compute-supply verdict rows for the SAME waits — the
	// pre-RN-15 second publication source.
	stats := rn15Stats(
		ComputeSupplySummary{Thread: ffrtA, State: "runnable", DurationMs: 2.661, Verdict: "cpu_pressure", Confidence: 0.76, LineStart: 100, LineEnd: 110},
		ComputeSupplySummary{Thread: ffrtB, State: "runnable", DurationMs: 2.908, Verdict: "cpu_pressure", Confidence: 0.76, LineStart: 120, LineEnd: 130},
	)

	out := enrichRootCauseRankWithScheduler(q, rank, SchedulerLatencyResult{}, stats, ChainResult{})

	if n := countRankItems(out, func(item RootCauseRankItem) bool {
		return item.Type == "compute_supply" && (item.Thread.PID > 0 || item.Thread.Comm != "")
	}); n != 0 {
		t.Fatalf("RN-15: a per-thread subject must never ride the compute_supply token, found %d such item(s): %+v", n, out.Items)
	}
	for _, tc := range []struct {
		thread ThreadRef
		ms     float64
	}{{ffrtA, 2.661}, {ffrtB, 2.908}} {
		if n := countRankItems(out, func(item RootCauseRankItem) bool {
			return item.Thread.PID == tc.thread.PID && item.CumulativeImpactMs == tc.ms
		}); n != 1 {
			t.Fatalf("RN-15: the %.3fms runnable wait of pid %d must be published exactly once (runnable_wait), got %d: %+v", tc.ms, tc.thread.PID, n, out.Items)
		}
		if n := countRankItems(out, func(item RootCauseRankItem) bool {
			return item.Thread.PID == tc.thread.PID && item.Type == "runnable_wait"
		}); n != 1 {
			t.Fatalf("RN-15: pid %d must keep its single runnable_wait candidate, got %d", tc.thread.PID, n)
		}
	}
}

// TestEnrichRootCauseRank_AggregateComputeSupplyRetainedPerThreadLowFrequencyKept
// pins the two survivors: (1) an aggregate-level (no thread subject)
// compute_supply verdict row still publishes its delivery-side candidate;
// (2) a per-thread low-frequency verdict keeps its own low_frequency token —
// the RN-15 guard removes only the per-thread compute_supply double-publish.
func TestEnrichRootCauseRank_AggregateComputeSupplyRetainedPerThreadLowFrequencyKept(t *testing.T) {
	q := Query{TimeStart: 0, TimeEnd: 3.0}
	ffrt := ThreadRef{Comm: "OS_FFRT_2_3", PID: 49706}
	stats := rn15Stats(
		// Aggregate delivery-side row: no concrete thread subject.
		ComputeSupplySummary{State: "runnable", DurationMs: 40, Verdict: "cpu_pressure", Confidence: 0.76, LineStart: 10, LineEnd: 20},
		// Per-thread low-frequency verdict: keeps the low_frequency token.
		ComputeSupplySummary{Thread: ffrt, State: "runnable", DurationMs: 2.661, Verdict: "low_frequency_signal", Confidence: 0.68, LineStart: 100, LineEnd: 110},
	)
	out := enrichRootCauseRankWithScheduler(q, RootCauseRankResult{}, SchedulerLatencyResult{}, stats, ChainResult{})

	if n := countRankItems(out, func(item RootCauseRankItem) bool {
		return item.Type == "compute_supply" && item.Thread.PID == 0 && item.Thread.Comm == ""
	}); n != 1 {
		t.Fatalf("aggregate-level compute_supply delivery verdict must be retained, got %d: %+v", n, out.Items)
	}
	if n := countRankItems(out, func(item RootCauseRankItem) bool {
		return item.Type == "low_frequency" && item.Thread.PID == 49706
	}); n != 1 {
		t.Fatalf("per-thread low_frequency verdict keeps its own token, got %d: %+v", n, out.Items)
	}
}

// TestComputeSupplyCauseSubjectAllowed_RejectsConcreteThreadSubjects pins the
// guard itself as the single precise signal: pid or comm present → rejected.
func TestComputeSupplyCauseSubjectAllowed_RejectsConcreteThreadSubjects(t *testing.T) {
	cases := []struct {
		thread ThreadRef
		want   bool
	}{
		{ThreadRef{}, true},
		{ThreadRef{Comm: "  "}, true},
		{ThreadRef{PID: 49706}, false},
		{ThreadRef{Comm: "OS_FFRT_2_3"}, false},
		{ThreadRef{Comm: "OS_FFRT_2_3", PID: 49706}, false},
	}
	for _, tc := range cases {
		if got := computeSupplyCauseSubjectAllowed(tc.thread); got != tc.want {
			t.Fatalf("computeSupplyCauseSubjectAllowed(%+v) = %v, want %v", tc.thread, got, tc.want)
		}
	}
}

// TestEvidenceFromStats_NoPerThreadComputeSupplyFactRunnableWaitSingleCopy
// pins the evidence-fact surface: per-thread ComputeSupply verdict rows are
// not republished as compute_supply facts; the wait stays a single
// runnable_wait fact.
func TestEvidenceFromStats_NoPerThreadComputeSupplyFactRunnableWaitSingleCopy(t *testing.T) {
	ffrt := ThreadRef{Comm: "OS_FFRT_2_3", PID: 49706}
	stats := WindowStats{
		RunnableTop: []ThreadDuration{{Thread: ffrt, DurationMs: 2.661, LineStart: 100, LineEnd: 110}},
		ComputeSupply: []ComputeSupplySummary{
			{Thread: ffrt, State: "runnable", DurationMs: 2.661, Verdict: "cpu_pressure", Confidence: 0.76, LineStart: 100, LineEnd: 110},
		},
	}
	facts := evidenceFromStats(stats)
	runnable, supply := 0, 0
	for _, fact := range facts {
		switch fact.Predicate {
		case "runnable_wait":
			runnable++
		case "compute_supply":
			supply++
		}
	}
	if supply != 0 {
		t.Fatalf("RN-15: per-thread compute_supply evidence facts must not be published, got %d: %+v", supply, facts)
	}
	if runnable != 1 {
		t.Fatalf("the runnable wait must keep exactly one runnable_wait fact, got %d: %+v", runnable, facts)
	}
}
