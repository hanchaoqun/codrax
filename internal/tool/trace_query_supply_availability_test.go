package tool

// SUPPLYAVAIL (2026-07-24) — the compute_supply render face must not print a
// withdrawn (unavailable) CPU's zero busy/idle as a measured 0.000ms (the
// masquerade the CPU-row and core_class faces already closed in
// 3bcfa33af/c1c7eba13; this is the third face of the same family).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQuerySummaryComputeSupplyBusyIdleAvailability(t *testing.T) {
	row := func(status, reason string, busy, idle float64) tracequery.Result {
		return tracequery.Result{WindowStats: &tracequery.WindowStats{
			ComputeSupply: []tracequery.ComputeSupplySummary{{
				Thread:            tracequery.ThreadRef{Comm: "worker", PID: 7},
				State:             "runnable",
				CPU:               2,
				CoreClass:         "big",
				DurationMs:        5,
				CPUBusyMs:         busy,
				CPUIdleMs:         idle,
				CPUBusyIdleStatus: status,
				CPUBusyIdleReason: reason,
				Verdict:           "insufficient_evidence",
			}},
		}}
	}
	unavailable := traceQuerySummary(row(tracequery.CPUBusyIdleStatusUnavailable, "no_sched_switch_observation", 0, 0), traceQueryParams{}, "src", "")
	if !strings.Contains(unavailable, "busy=unavailable idle=unavailable busy_idle_status=unavailable busy_idle_reason=no_sched_switch_observation") {
		t.Fatalf("unavailable supply row must render the typed withdrawal:\n%s", unavailable)
	}
	if strings.Contains(unavailable, "busy=0.000ms") {
		t.Fatalf("unavailable supply row leaked a measured-zero masquerade:\n%s", unavailable)
	}
	partial := traceQuerySummary(row(tracequery.CPUBusyIdleStatusPartial, "", 3, 2), traceQueryParams{}, "src", "")
	if !strings.Contains(partial, "busy=3.000ms idle=2.000ms busy_idle_status=partial") {
		t.Fatalf("partial supply row must keep numbers and disclose status:\n%s", partial)
	}
	// Legacy rows (empty status) keep their numeric bytes untouched.
	legacy := traceQuerySummary(row("", "", 3, 2), traceQueryParams{}, "src", "")
	if !strings.Contains(legacy, "busy=3.000ms idle=2.000ms runnable_wait=") {
		t.Fatalf("legacy supply row bytes drifted:\n%s", legacy)
	}
}

func TestTraceQuerySummaryOmitsEmptyBusyIdleReasonToken(t *testing.T) {
	// P3-SMALLS ③ (2026-07-24): measured per-CPU / core_class rows carry no
	// reason — the render must omit the key instead of printing an empty
	// busy_idle_reason= token.
	result := tracequery.Result{WindowStats: &tracequery.WindowStats{
		CPU: []tracequery.CPUStats{{
			CPU: 1, CoreClass: "small", BusyMs: 4, IdleMs: 6,
			BusyIdleStatus: tracequery.CPUBusyIdleStatusMeasured,
		}},
		CoreTopology: []tracequery.CoreClassStats{{
			Class: "small", CPUs: []int{1}, BusyMs: 4, IdleMs: 6,
			BusyIdleStatus: tracequery.CPUBusyIdleStatusMeasured,
		}},
	}}
	summary := traceQuerySummary(result, traceQueryParams{}, "src", "")
	if strings.Contains(summary, "busy_idle_reason= ") || strings.HasSuffix(summary, "busy_idle_reason=") {
		t.Fatalf("measured rows must omit the empty reason token:\n%s", summary)
	}
	for _, want := range []string{
		"cpu=1 core_class=small busy=4.000ms idle=6.000ms busy_idle_status=measured freq=0",
		"core_class=small cpus=[1] busy=4.000ms idle=6.000ms busy_idle_status=measured runnable_wait=",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("measured row bytes missing %q:\n%s", want, summary)
		}
	}
	// A populated reason still renders on both faces.
	result.WindowStats.CPU[0].BusyIdleStatus = tracequery.CPUBusyIdleStatusPartial
	result.WindowStats.CPU[0].BusyIdleReason = "partial_cpu_busy_idle_coverage"
	result.WindowStats.CoreTopology[0].BusyIdleStatus = tracequery.CPUBusyIdleStatusPartial
	result.WindowStats.CoreTopology[0].BusyIdleReason = "partial_cpu_busy_idle_coverage"
	summary = traceQuerySummary(result, traceQueryParams{}, "src", "")
	if strings.Count(summary, "busy_idle_status=partial busy_idle_reason=partial_cpu_busy_idle_coverage") != 2 {
		t.Fatalf("populated reason must render on both faces:\n%s", summary)
	}
}
