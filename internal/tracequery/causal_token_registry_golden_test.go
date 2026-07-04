package tracequery

import (
	"fmt"
	"strings"
	"testing"
)

// TestCausalTokenRegistryGoldenSnapshot pins the ENTIRE causal-token registry
// (RN-16 §7.9, modeled on the 69-kind analysis golden): every token with its
// lane / additivity / subject / row-token flag / zh-label ref, one line each,
// sorted. Any registry edit — adding a token, moving a lane, changing
// additivity or subject — shows up as a golden diff and forces a conscious
// review against the §7.4 demand/supply ruling and the §7.5 supply_pressure
// final ruling (read the registry header before updating this list).
func TestCausalTokenRegistryGoldenSnapshot(t *testing.T) {
	golden := []string{
		"binder_wait|wakeup_chain|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"block_io_by_inode|io_blocking|wall_clock_per_thread|either|row|runtimeTraceRootCauseTypeZHLabel",
		"blocked_reason|io_blocking|count|per_thread|row|",
		"blocking_span|lock_contention|wall_clock_per_thread|per_thread|row|",
		"class_verification|cpu_work|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"compute_supply|compute_delivery|cross_thread_cpu_ms|aggregate_only|row|runtimeTraceRootCauseTypeZHLabel",
		"compute_supply_balance|compute_delivery|cross_thread_cpu_ms|aggregate_only|observation_only|",
		"cpu_affinity_or_cpuset|scheduling_demand|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"cpu_frequency_limit|compute_delivery|cross_thread_cpu_ms|aggregate_only|row|runtimeTraceRootCauseTypeZHLabel",
		"cpu_pressure|scheduling_demand|cross_thread_cpu_ms|aggregate_only|row|runtimeTraceRootCauseTypeZHLabel",
		"d_state_or_io_wait|io_blocking|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"dma_fence_activity|cpu_work|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"file_io_hot_inode|io_blocking|count|either|row|runtimeTraceRootCauseTypeZHLabel",
		"fragmented_d_state_or_io_wait|io_blocking|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"fragmented_runnable_wait|scheduling_demand|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"fragmented_running|cpu_work|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"fragmented_sleep_wait|wakeup_chain|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"io_burst_episode|io_blocking|wall_clock_per_thread|either|row|runtimeTraceRootCauseTypeZHLabel",
		"io_latency|io_blocking|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"io_pressure|io_blocking|cross_thread_cpu_ms|either|row|runtimeTraceRootCauseTypeZHLabel",
		"io_wait|io_blocking|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"ipi_activity|irq_aggregate|cross_thread_cpu_ms|aggregate_only|row|runtimeTraceRootCauseTypeZHLabel",
		"irq_activity|irq_aggregate|cross_thread_cpu_ms|aggregate_only|row|runtimeTraceRootCauseTypeZHLabel",
		"irq_burst|irq_aggregate|cross_thread_cpu_ms|aggregate_only|row|runtimeTraceRootCauseTypeZHLabel",
		"jit_compile|cpu_work|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"lock_contention|lock_contention|wall_clock_per_thread|per_thread|observation_only|",
		"low_frequency|compute_delivery|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"memory_gc|memory_pressure|count|aggregate_only|row|",
		"memory_page_fault|memory_pressure|count|aggregate_only|row|",
		"memory_reclaim|memory_pressure|count|aggregate_only|row|",
		"missing_wakeup|wakeup_chain|wall_clock_per_thread|per_thread|row|",
		"monitor_contention|lock_contention|wall_clock_per_thread|per_thread|observation_only|",
		"page_cache_churn|memory_pressure|count|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"priority_inversion_candidate|scheduling_demand|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"priority_inversion_runnable_wait|scheduling_demand|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"runnable_occupancy|scheduling_demand|wall_clock_per_thread|per_thread|observation_only|",
		"runnable_wait|scheduling_demand|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"running|cpu_work|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"runtime_compile|cpu_work|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"sched_stat_accounting|diagnostic|wall_clock_per_thread|per_thread|row|",
		"scheduler_latency|scheduling_demand|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"shader_compile|cpu_work|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"sleep_wait|wakeup_chain|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"state_churn|diagnostic|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		// VS-2 §7.10: per-thread wall-clock decomposition of the subject's OWN
		// running time — the delivery lane's legal per-thread form, distinct
		// from the compute_supply aggregate ledger (§7.10 (5)).
		"supply_fold_deficit|compute_delivery|wall_clock_per_thread|per_thread|observation_only|",
		"supply_pressure|scheduling_demand|cross_thread_cpu_ms|aggregate_only|row|runtimeTraceSupplyPressureDisplayLabel",
		"trace_gap|diagnostic|count|per_thread|row|",
		"trace_span|cpu_work|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
		"unknown_state|diagnostic|wall_clock_per_thread|per_thread|row|",
		"workqueue_activity|cpu_work|wall_clock_per_thread|per_thread|row|runtimeTraceRootCauseTypeZHLabel",
	}

	var got []string
	for _, token := range CausalTokenUniverse() {
		spec, _ := CausalTokenSpecFor(token)
		row := "row"
		if !spec.RowToken {
			row = "observation_only"
		}
		got = append(got, fmt.Sprintf("%s|%s|%s|%s|%s|%s", token, spec.Lane, spec.Additivity, spec.Subject, row, spec.LabelZhRef))
	}

	if len(got) != len(golden) {
		t.Errorf("registry has %d token(s), golden has %d — full current registry:\n%s", len(got), len(golden), strings.Join(got, "\n"))
	}
	for i := 0; i < len(got) && i < len(golden); i++ {
		if got[i] != golden[i] {
			t.Errorf("registry line %d drifted:\n  got:    %s\n  golden: %s", i, got[i], golden[i])
		}
	}
	if t.Failed() {
		t.Fatalf("causal-token registry golden mismatch — if the change is INTENDED, re-read the §7.4 demand/supply ruling and the §7.5 supply_pressure final ruling in docs/design/customer_dead_session_audit_20260703.md (see the registry file header), then update this golden")
	}
}
