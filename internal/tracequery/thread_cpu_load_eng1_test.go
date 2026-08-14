package tracequery

import (
	"context"
	"math"
	"os"
	"testing"
)

// thread_cpu_load_eng1_test.go — ENG-1 pins (复核冷读 F2-1, 2026-07-12):
// thread_cpu_load totals are FULL-window all-core sums; the display caps
// limit only the per-slice/roster rows, never the arithmetic.

// donghuWitnessTracePath is the customer witness trace (ledger §29.42.5
// registers it as the eval gold sample). The full-window donghu assertion
// below runs wherever the file exists and skips elsewhere — the hermetic
// half of ENG-1 is pinned by TestProcessDomainCensusDonghuShapePin's
// 17.0-vs-6.0 totals flip on the constructed b3 census trace.
const donghuWitnessTracePath = "/Users/han/opt/donghu/donghu.ftrace"

// TestENG1ThreadCPULoadFullWindowRunningTotalDonghu — the production
// witness: .ugc.aweme.lite-17267's window running is 157.248ms across all
// cores (cold-read full sched_switch state machine, identity closes at
// 157.248+70.338+5.604=233.190); the pre-ENG-1 face published 132.041
// (cpu12 96.081 + cpu4 35.960 — the only two slices surviving the global
// top-8 cut). Runnable likewise: full total 5.604 vs the 1.193 top-slice.
func TestENG1ThreadCPULoadFullWindowRunningTotalDonghu(t *testing.T) {
	if _, err := os.Stat(donghuWitnessTracePath); err != nil {
		t.Skipf("witness trace unavailable: %v", err)
	}
	idx, err := BuildIndex(context.Background(), donghuWitnessTracePath)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	stats := ComputeWindowStats(idx, Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898})
	var load *ThreadCPULoadSummary
	for i := range stats.ThreadCPULoad {
		if stats.ThreadCPULoad[i].Thread.PID == 17267 {
			load = &stats.ThreadCPULoad[i]
			break
		}
	}
	if load == nil {
		t.Fatalf("expected the target's thread_cpu_load row: %+v", stats.ThreadCPULoad)
	}
	if math.Abs(load.RunningMs-157.248) > 0.05 {
		t.Fatalf("running total must be the full-window all-core sum 157.248ms, got %.3f (132.041 = the pre-ENG-1 two-slice partial sum)", load.RunningMs)
	}
	if math.Abs(load.RunnableWaitMs-5.604) > 0.05 {
		t.Fatalf("runnable total must be the full-window sum 5.604ms, got %.3f (1.193 = the pre-ENG-1 top-slice partial)", load.RunnableWaitMs)
	}
	if load.RunningScope != ThreadCPULoadRunningScope ||
		load.RunnableScope != ThreadCPULoadRunnableScope ||
		load.ValueScope != ThreadCPULoadValueScope ||
		load.CPUScope != ThreadCPULoadCPUScope {
		t.Fatalf("thread CPU-load JSON product must carry its value/CPU calibers: %+v", load)
	}
	// B766 production witness: the focused target account must publish every
	// exact CPU bucket from its own timeline, independent of the global
	// top-running cap that exposed only CPU12+CPU4 (=132.041ms).
	res := Run(idx, Query{View: "window_stats", PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898})
	account := res.TargetWindowStates
	if account == nil || account.RunningCPURosterStatus != "complete" ||
		account.RunningCPUAssignmentStatus != "complete" || account.RunningCPURosterTotal <= 2 {
		t.Fatalf("target-owned per-CPU running roster must expose the CPUs hidden by global top-8: %+v", account)
	}
	var rosterSum float64
	var cpu4, cpu12 float64
	for _, row := range account.RunningByCPU {
		rosterSum += row.RunningMs
		switch row.CPU {
		case 4:
			cpu4 = row.RunningMs
		case 12:
			cpu12 = row.RunningMs
		}
	}
	if math.Abs(rosterSum-account.RunningMs) > 0.001 || math.Abs(account.RunningMs-157.248) > 0.05 {
		t.Fatalf("complete target CPU roster must conserve 157.248ms: roster=%.3f account=%+v", rosterSum, account)
	}
	if math.Abs(cpu4-35.960) > 0.05 || math.Abs(cpu12-96.081) > 0.05 {
		t.Fatalf("known customer buckets drifted while completing the roster: cpu4=%.3f cpu12=%.3f roster=%+v", cpu4, cpu12, account.RunningByCPU)
	}
}
