package tracequery

import (
	"strings"
	"testing"
)

// RN-11 (§7.9, cust_runnable.txt 2026-07-04): a runnable-dominant
// state_drilldown row must NOT require a wakeup-chain drilldown — the
// customer's model anchor (OS_FFRT_2_3-49706, runnable 2528ms of a 3000ms
// window, sleep=0, no wakeup edge) was forced into view=wakeup_chain in
// exploration round 10 and the model correctly pushed back. Runnable rows
// recommend the CPU-competition surfaces (scheduler_latency_stats /
// root_cause_rank / window_stats occupancy) and stay recursive; sleep and
// D-state rows keep their wakeup-chain requirement byte-for-byte.
func TestStateDrilldownRunnableDropsWakeupChainRequirement(t *testing.T) {
	stats := WindowStats{
		Window: TimeWindow{StartTs: 100.0, EndTs: 103.0}, // 3000ms customer window
		RunnableTop: []ThreadDuration{{
			Thread: ThreadRef{Comm: "OS_FFRT_2_3", PID: 49706}, DurationMs: 2528.000,
			LineStart: 10, LineEnd: 90,
		}},
		SleepTop: []ThreadDuration{{
			Thread: ThreadRef{Comm: "render_service", PID: 411}, DurationMs: 1200.000,
			LineStart: 11, LineEnd: 40,
		}},
		DStateTop: []ThreadDuration{{
			Thread: ThreadRef{Comm: "f2fs_flush", PID: 300}, DurationMs: 400.000,
			LineStart: 12, LineEnd: 30,
		}},
	}
	plan, _ := buildStateDrilldownPlan(stats, 12)

	runnable := findStateDrilldownStepForTest(plan, 49706, "top_runnable", string(StateRunnable))
	if runnable == nil {
		t.Fatalf("runnable drilldown step missing: %+v", plan)
	}
	if runnable.ChainRequired {
		t.Fatalf("RN-11: runnable-dominant row must not require a wakeup chain: %+v", runnable)
	}
	if !runnable.Recursive {
		t.Fatalf("RN-11: runnable row must stay a recursive root-cause candidate: %+v", runnable)
	}
	if got := strings.Join(runnable.RecommendedViews, ","); got != "scheduler_latency_stats,root_cause_rank,window_stats" {
		t.Fatalf("RN-11: runnable recommended views must be the CPU-competition surfaces, got %q", got)
	}
	// Pin the published row text (the LLM-facing gate/hint surface): the
	// runnable row says chain_required=false and names the occupancy surface.
	if !strings.Contains(runnable.Summary, "chain_required=false") ||
		!strings.Contains(runnable.Summary, "recommended_views=scheduler_latency_stats,root_cause_rank,window_stats") {
		t.Fatalf("RN-11: runnable row text must publish the no-chain requirement: %s", runnable.Summary)
	}

	// Sleep row unchanged: wakeup chain still required, same views as before.
	sleep := findStateDrilldownStepForTest(plan, 411, "top_sleep", string(StateSSleep))
	if sleep == nil {
		t.Fatalf("sleep drilldown step missing: %+v", plan)
	}
	if !sleep.ChainRequired || !sleep.Recursive {
		t.Fatalf("sleep row behavior must be unchanged (chain_required=true, recursive=true): %+v", sleep)
	}
	if got := strings.Join(sleep.RecommendedViews, ","); got != "wakeup_chain,root_cause_rank" {
		t.Fatalf("sleep recommended views must be unchanged, got %q", got)
	}
	if !strings.Contains(sleep.Summary, "chain_required=true") {
		t.Fatalf("sleep row text must keep chain_required=true: %s", sleep.Summary)
	}

	// D-state row unchanged: chain requirement intact.
	dstate := findStateDrilldownStepForTest(plan, 300, "top_d_state", string(StateDSleep))
	if dstate == nil {
		t.Fatalf("d_state drilldown step missing: %+v", plan)
	}
	if !dstate.ChainRequired || !dstate.Recursive {
		t.Fatalf("d_state row behavior must be unchanged: %+v", dstate)
	}
}

// RN-11: churn-derived runnable rows follow the same rule — no wakeup-chain
// requirement, still recursive.
func TestStateDrilldownRunnableChurnDropsWakeupChainRequirement(t *testing.T) {
	stats := WindowStats{
		Window: TimeWindow{StartTs: 100.0, EndTs: 103.0},
		StateChurn: []ThreadStateChurnSummary{{
			Thread:           ThreadRef{Comm: "OS_FFRT_2_3", PID: 49706},
			DominantState:    string(StateRunnable),
			DominantImpactMs: 900.000,
			TotalMs:          1400.000,
			RunnableMs:       900.000,
			FragmentCount:    12,
			StateSwitches:    11,
			MaxSegmentMs:     120.000,
		}},
	}
	plan, _ := buildStateDrilldownPlan(stats, 12)
	step := findStateDrilldownStepForTest(plan, 49706, "state_churn", string(StateRunnable))
	if step == nil {
		t.Fatalf("churn runnable step missing: %+v", plan)
	}
	if step.ChainRequired {
		t.Fatalf("RN-11: churn runnable row must not require a wakeup chain: %+v", step)
	}
	if !step.Recursive {
		t.Fatalf("RN-11: churn runnable row must stay recursive: %+v", step)
	}
}
