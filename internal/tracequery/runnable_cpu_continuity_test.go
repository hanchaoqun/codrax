package tracequery

import (
	"fmt"
	"strings"
	"testing"
)

func TestWakeupCPUContinuityMismatchRetainsRunnableAndWithdrawsCPULanes(t *testing.T) {
	idx := buildTraceIndex(t, "wakeup_cpu_mismatch.systrace", `
      rival-200 (200) [002] .... 1.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=rival next_pid=200 next_prio=20
      waker-300 (300) [003] .... 1.001000: sched_wakeup: comm=app pid=100 prio=60 target_cpu=001
	      rival-200 (200) [002] .... 1.006000: sched_switch: prev_comm=rival prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=app next_pid=100 next_prio=60
        app-100 (100) [002] .... 1.008000: sched_switch: prev_comm=app prev_pid=100 prev_prio=60 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
	`)
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.009, MinDurationMs: 0.001, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	stats := ComputeWindowStats(idx, q)

	var runnable *ThreadDuration
	for i := range stats.RunnableTop {
		if stats.RunnableTop[i].Thread.PID == 100 {
			runnable = &stats.RunnableTop[i]
			break
		}
	}
	if runnable == nil || !near(runnable.DurationMs, 5, 0.001) || runnable.CPU != -1 {
		t.Fatalf("thread-level runnable must survive on the unknown-CPU lane: %+v", stats.RunnableTop)
	}
	if stats.RunnableCPUContinuity == nil || stats.RunnableCPUContinuity.MismatchSegments != 1 ||
		stats.RunnableCPUContinuity.UnknownSegments != 1 || !near(stats.RunnableCPUContinuity.UnknownMs, 5, 0.001) {
		t.Fatalf("typed mismatch census missing: %+v", stats.RunnableCPUContinuity)
	}
	if len(stats.RunnableCPUContinuity.Witnesses) != 1 {
		t.Fatalf("bounded mismatch witness missing: %+v", stats.RunnableCPUContinuity)
	}
	witness := stats.RunnableCPUContinuity.Witnesses[0]
	if witness.Thread.PID != 100 || witness.ExpectedCPU != 1 || witness.ObservedCPU != 2 || witness.Reason != RunnableCPUContinuitySchedInMismatch {
		t.Fatalf("mismatch witness lost endpoint identity: %+v", witness)
	}
	for _, pressure := range stats.CPUPressure {
		for _, td := range pressure.TopRunnable {
			if td.Thread.PID == 100 {
				t.Fatalf("unproven runnable CPU leaked into pressure lane: %+v", stats.CPUPressure)
			}
		}
	}
	latency := BuildSchedulerLatencyStats(idx, q)
	if len(latency.Items) != 1 || latency.Items[0].CPU != -1 || !near(latency.Items[0].DurationMs, 5, 0.001) {
		t.Fatalf("scheduler latency must retain wall time without a CPU claim: %+v", latency.Items)
	}
	if len(latency.Items[0].SameCPUTopRunning) != 0 || latency.Items[0].WeightedFrequency != 0 || latency.Items[0].ObservedMaxFrequency != 0 {
		t.Fatalf("unknown-CPU latency minted CPU-specific context: %+v", latency.Items[0])
	}
	if !containsSubstring(stats.Caveats, "runnable_cpu_continuity_degraded=true") {
		t.Fatalf("customer-visible continuity disclosure missing: %+v", stats.Caveats)
	}

	rank := BuildRootCauseRank(idx, q)
	foundRunnable := false
	for _, item := range rank.Items {
		if item.Thread.PID == 100 && (item.Type == "runnable_wait" || item.Type == "scheduler_latency") && item.EffectiveImpactMs > 0 {
			foundRunnable = true
		}
		if item.Thread.PID == 100 && strings.Contains(item.Summary, "same_cpu_top_running=rival-200") {
			t.Fatalf("CPU-mismatched wait fabricated the rival as a same-CPU cause: %+v", item)
		}
	}
	if !foundRunnable {
		t.Fatalf("CPU fail-close must not erase the runnable root-cause account: %+v", rank.Items)
	}
}

func TestWakeupCPUContinuityExactAndMigratedSegmentsRemainCPUSpecific(t *testing.T) {
	idx := buildTraceIndex(t, "wakeup_cpu_migrate.systrace", `
      waker-300 (300) [003] .... 1.001000: sched_wakeup: comm=app pid=100 prio=60 target_cpu=000
      mover-400 (400) [000] .... 1.003000: sched_migrate_task: comm=app pid=100 prio=60 orig_cpu=0 dest_cpu=1
	        other-9 (9) [001] .... 1.006000: sched_switch: prev_comm=other prev_pid=9 prev_prio=120 prev_state=S ==> next_comm=app next_pid=100 next_prio=60
	`)
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.007, MinDurationMs: 0.001, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	stats := ComputeWindowStats(idx, q)
	byCPU := map[int]float64{}
	for _, td := range stats.RunnableTop {
		if td.Thread.PID == 100 {
			byCPU[td.CPU] += td.DurationMs
		}
	}
	if !near(byCPU[0], 2, 0.001) || !near(byCPU[1], 3, 0.001) || byCPU[-1] != 0 {
		t.Fatalf("exact migration must partition the CPU-specific wait: %+v", stats.RunnableTop)
	}
	continuity := stats.RunnableCPUContinuity
	if continuity == nil || continuity.VerifiedSegments != 2 || continuity.UnknownSegments != 0 || continuity.ExactMigrationSegments != 1 {
		t.Fatalf("exact migration continuity census drifted: %+v", continuity)
	}
	latency := BuildSchedulerLatencyStats(idx, q)
	latencyByCPU := map[int]float64{}
	for _, item := range latency.Items {
		if item.Thread.PID == 100 {
			latencyByCPU[item.CPU] += item.DurationMs
		}
	}
	if !near(latencyByCPU[0], 2, 0.001) || !near(latencyByCPU[1], 3, 0.001) {
		t.Fatalf("latency and runnable ledgers must share the migration partition: %+v", latency.Items)
	}
}

func TestWakeupCPUContinuityWarmCarryMismatchFailsCPULaneClosed(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "wakeup_cpu_warm_mismatch.systrace",
		" waker-300 (300) [003] .... 0.800000: sched_wakeup: comm=app pid=100 prio=60 target_cpu=000",
		"    other-9 (9) [001] .... 1.040000: sched_switch: prev_comm=other prev_pid=9 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=60",
	)
	idx := buildSchedulerCarryWindow(t, path, 1.0, 1.1)
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.1, MinDurationMs: 0.001}
	stats := ComputeWindowStats(idx, q)
	runnable := threadDurationForPID(stats.RunnableTop, 100)
	if runnable == nil || runnable.CPU != -1 || !near(runnable.DurationMs, 40, 0.001) {
		t.Fatalf("warm carry mismatch must retain only the thread-level wait: %+v", stats.RunnableTop)
	}
	if stats.RunnableCPUContinuity == nil || stats.RunnableCPUContinuity.MismatchSegments != 1 {
		t.Fatalf("warm carry mismatch disappeared across the head snapshot: %+v", stats.RunnableCPUContinuity)
	}
}

func TestWakeupCPUContinuityOpenEndDoesNotClaimTargetCPU(t *testing.T) {
	idx := buildTraceIndex(t, "wakeup_cpu_open.systrace", `
      waker-300 (300) [003] .... 1.001000: sched_wakeup: comm=app pid=100 prio=60 target_cpu=001
	`)
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.006, MinDurationMs: 0.001}
	stats := ComputeWindowStats(idx, q)
	runnable := threadDurationForPID(stats.RunnableTop, 100)
	if runnable == nil || runnable.CPU != -1 || !near(runnable.DurationMs, 5, 0.001) {
		t.Fatalf("open runnable wait has no terminal CPU witness: %+v", stats.RunnableTop)
	}
	if stats.RunnableCPUContinuity == nil || stats.RunnableCPUContinuity.OpenEndedSegments != 1 || stats.RunnableCPUContinuity.UnknownSegments != 1 {
		t.Fatalf("open-ended continuity census missing: %+v", stats.RunnableCPUContinuity)
	}
}

func TestWakeupCPUContinuityMigrationOriginMismatchFailsOnlyOldSegmentClosed(t *testing.T) {
	idx := buildTraceIndex(t, "wakeup_cpu_bad_migrate_origin.systrace", `
      waker-300 (300) [003] .... 1.001000: sched_wakeup: comm=app pid=100 prio=60 target_cpu=000
      mover-400 (400) [001] .... 1.003000: sched_migrate_task: comm=app pid=100 prio=60 orig_cpu=1 dest_cpu=2
	        other-9 (9) [002] .... 1.006000: sched_switch: prev_comm=other prev_pid=9 prev_prio=120 prev_state=S ==> next_comm=app next_pid=100 next_prio=60
	`)
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.007, MinDurationMs: 0.001}
	stats := ComputeWindowStats(idx, q)
	byCPU := map[int]float64{}
	for _, td := range stats.RunnableTop {
		if td.Thread.PID == 100 {
			byCPU[td.CPU] += td.DurationMs
		}
	}
	if !near(byCPU[-1], 2, 0.001) || !near(byCPU[2], 3, 0.001) || byCPU[0] != 0 || byCPU[1] != 0 {
		t.Fatalf("bad orig_cpu must withdraw only the pre-migration CPU claim: %+v", stats.RunnableTop)
	}
	continuity := stats.RunnableCPUContinuity
	if continuity == nil || continuity.MismatchSegments != 1 || continuity.ExactMigrationSegments != 1 ||
		continuity.CheckedBoundarySegments != 2 || continuity.VerifiedSegments != 1 || !near(continuity.MismatchRatio, 0.5, 0.000001) {
		t.Fatalf("migration-origin verdict census drifted: %+v", continuity)
	}
	if len(continuity.Witnesses) == 0 || continuity.Witnesses[0].Reason != RunnableCPUContinuityMigrationMismatch ||
		continuity.Witnesses[0].ExpectedCPU != 0 || continuity.Witnesses[0].ObservedCPU != 1 {
		t.Fatalf("migration mismatch lost its typed endpoints: %+v", continuity.Witnesses)
	}
}

func TestWakeupCPUContinuityMigrationMismatchWithoutSchedInHasDefinedRatio(t *testing.T) {
	idx := buildTraceIndex(t, "wakeup_cpu_bad_migrate_only.systrace", `
      waker-300 (300) [003] .... 1.001000: sched_wakeup: comm=app pid=100 prio=60 target_cpu=000
      mover-400 (400) [001] .... 1.003000: sched_migrate_task: comm=app pid=100 prio=60 orig_cpu=1 dest_cpu=2
	`)
	stats := ComputeWindowStats(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.003, MinDurationMs: 0.001})
	continuity := stats.RunnableCPUContinuity
	if continuity == nil || continuity.SchedInSegments != 0 || continuity.ExactMigrationSegments != 1 ||
		continuity.CheckedBoundarySegments != 1 || continuity.MismatchSegments != 1 || !near(continuity.MismatchRatio, 1, 0.000001) {
		t.Fatalf("migration-only mismatch ratio must use its checked boundary: %+v", continuity)
	}
}

func TestWakeupCPUContinuityDuplicateWakeTargetsAreIdempotentOrFailClosed(t *testing.T) {
	t.Run("matching", func(t *testing.T) {
		idx := buildTraceIndex(t, "matching_wake_pair.systrace", `
 waker-300 (300) [000] .... 1.001000: sched_waking: comm=app pid=100 prio=52 target_cpu=001
 waker-300 (300) [000] .... 1.001100: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
 other-400 (400) [001] .... 1.003000: sched_switch: prev_comm=other prev_pid=400 prev_prio=52 prev_state=S ==> next_comm=app next_pid=100 next_prio=52
`)
		stats := ComputeWindowStats(idx, Query{PID: 100, TimeStart: 1, TimeEnd: 1.004, MinDurationMs: 0.001})
		continuity := stats.RunnableCPUContinuity
		if continuity == nil || continuity.TotalSegments != 1 || continuity.VerifiedSegments != 1 ||
			continuity.UnknownSegments != 0 || continuity.WakeTargetConflictSegments != 0 {
			t.Fatalf("matching waking/wakeup pair was not idempotent: %+v", continuity)
		}
	})

	t.Run("unknown_then_known", func(t *testing.T) {
		idx := buildTraceIndex(t, "fillable_wake_pair.systrace", `
 waker-300 (300) [000] .... 1.001000: sched_waking: comm=app pid=100 prio=52
 waker-300 (300) [000] .... 1.001100: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
 other-400 (400) [001] .... 1.003000: sched_switch: prev_comm=other prev_pid=400 prev_prio=52 prev_state=S ==> next_comm=app next_pid=100 next_prio=52
`)
		stats := ComputeWindowStats(idx, Query{PID: 100, TimeStart: 1, TimeEnd: 1.004, MinDurationMs: 0.001})
		continuity := stats.RunnableCPUContinuity
		if continuity == nil || continuity.TotalSegments != 1 || continuity.VerifiedSegments != 1 || continuity.UnknownSegments != 0 {
			t.Fatalf("later precise wake target did not fill an absent first target: %+v", continuity)
		}
	})

	t.Run("conflicting", func(t *testing.T) {
		idx := buildTraceIndex(t, "conflicting_wake_pair.systrace", `
 waker-300 (300) [000] .... 1.001000: sched_waking: comm=app pid=100 prio=52 target_cpu=001
 waker-300 (300) [000] .... 1.001100: sched_wakeup: comm=app pid=100 prio=52 target_cpu=002
 other-400 (400) [001] .... 1.003000: sched_switch: prev_comm=other prev_pid=400 prev_prio=52 prev_state=S ==> next_comm=app next_pid=100 next_prio=52
`)
		stats := ComputeWindowStats(idx, Query{PID: 100, TimeStart: 1, TimeEnd: 1.004, MinDurationMs: 0.001})
		continuity := stats.RunnableCPUContinuity
		if continuity == nil || continuity.TotalSegments != 1 || continuity.VerifiedSegments != 0 ||
			continuity.UnknownSegments != 1 || continuity.WakeTargetConflictSegments != 1 || len(continuity.Witnesses) != 1 {
			t.Fatalf("conflicting waking/wakeup targets did not fail closed: %+v", continuity)
		}
		witness := continuity.Witnesses[0]
		if witness.Reason != RunnableCPUContinuityWakeTargetConflict || witness.ExpectedCPU != 1 || witness.ObservedCPU != 2 {
			t.Fatalf("typed wake-target contradiction was lost: %+v", witness)
		}
		for _, pressure := range stats.CPUPressure {
			for _, runnable := range pressure.TopRunnable {
				if runnable.Thread.PID == 100 {
					t.Fatalf("contradictory wake target leaked into CPU pressure: %+v", stats.CPUPressure)
				}
			}
		}
	})
}

func TestWakeupCPUContinuityConflictHasFullAndWindowHeadParity(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "wakeup_target_conflict_prefix.systrace",
		" waker-300 (300) [000] .... 0.800000: sched_waking: comm=app pid=100 prio=52 target_cpu=001",
		" waker-300 (300) [000] .... 0.810000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=002",
		" other-400 (400) [001] .... 1.040000: sched_switch: prev_comm=other prev_pid=400 prev_prio=52 prev_state=S ==> next_comm=app next_pid=100 next_prio=52",
	)
	full, err := BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	warm := buildSchedulerCarryWindow(t, path, 1.0, 1.05)
	warmHead := schedulerHeadForQuery(warm, Query{TimeStart: 1.0, TimeEnd: 1.05})
	if warmHead == nil || !warmHead.Complete {
		t.Fatalf("expected complete window-head state: %+v", warmHead)
	}
	headState, ok := warmHead.Threads[100]
	if !ok || headState.State != StateRunnable || headState.CPUKnown || headState.CPU != -1 ||
		headState.CPUProvenance != runnableCPUProvenanceWakeTargetConflict ||
		headState.CPUConflictExpected != 1 || headState.CPUConflictObserved != 2 {
		t.Fatalf("prefix scan swallowed the duplicate wake-target conflict: %+v", headState)
	}

	fullStats := ComputeWindowStats(full, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.05, MinDurationMs: 0.001})
	warmStats := ComputeWindowStats(warm, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.05, MinDurationMs: 0.001})
	assertConflict := func(label string, stats WindowStats, wantMs float64) {
		t.Helper()
		runnable := threadDurationForPID(stats.RunnableTop, 100)
		if runnable == nil || runnable.CPU != -1 || !near(runnable.DurationMs, wantMs, 0.001) {
			t.Fatalf("%s lost thread wall time or minted a CPU claim: %+v", label, stats.RunnableTop)
		}
		continuity := stats.RunnableCPUContinuity
		if continuity == nil || continuity.TotalSegments != 1 || continuity.UnknownSegments != 1 ||
			continuity.VerifiedSegments != 0 || continuity.WakeTargetConflictSegments != 1 || len(continuity.Witnesses) != 1 {
			t.Fatalf("%s lost the typed conflict verdict: %+v", label, continuity)
		}
		witness := continuity.Witnesses[0]
		if witness.Reason != RunnableCPUContinuityWakeTargetConflict || witness.ExpectedCPU != 1 || witness.ObservedCPU != 2 {
			t.Fatalf("%s conflict endpoints drifted: %+v", label, witness)
		}
		for _, pressure := range stats.CPUPressure {
			for _, item := range pressure.TopRunnable {
				if item.Thread.PID == 100 {
					t.Fatalf("%s contradictory carry leaked into CPU pressure: %+v", label, stats.CPUPressure)
				}
			}
		}
	}
	assertConflict("full-index", fullStats, 40)
	assertConflict("window-head", warmStats, 40)
}

func TestWakeupCPUContinuityFullAndWindowHeadCarryParity(t *testing.T) {
	tests := []struct {
		name       string
		lines      []string
		wantCPU    int
		wantMs     float64
		wantReason string
	}{
		{
			name: "wake_to_sched_in",
			lines: []string{
				" waker-300 (300) [000] .... 0.800000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001",
				" other-400 (400) [001] .... 1.040000: sched_switch: prev_comm=other prev_pid=400 prev_prio=52 prev_state=S ==> next_comm=app next_pid=100 next_prio=52",
			},
			wantCPU: 1, wantMs: 40, wantReason: RunnableCPUContinuityVerified,
		},
		{
			name: "open_end",
			lines: []string{
				" waker-300 (300) [000] .... 0.800000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001",
				" marker-900 (900) [003] .... 1.050000: tracing_mark_write: C|900|fixture_clock|1",
			},
			wantCPU: -1, wantMs: 50, wantReason: RunnableCPUContinuityOpenEnded,
		},
		{
			name: "pre_window_migration",
			lines: []string{
				" waker-300 (300) [000] .... 0.800000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=000",
				" mover-500 (500) [000] .... 0.900000: sched_migrate_task: comm=app pid=100 prio=52 orig_cpu=0 dest_cpu=1",
				" other-400 (400) [001] .... 1.040000: sched_switch: prev_comm=other prev_pid=400 prev_prio=52 prev_state=S ==> next_comm=app next_pid=100 next_prio=52",
			},
			wantCPU: 1, wantMs: 40, wantReason: RunnableCPUContinuityVerified,
		},
		{
			name: "migration_at_exact_boundary",
			lines: []string{
				" waker-300 (300) [000] .... 0.800000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=000",
				" mover-500 (500) [000] .... 1.000000: sched_migrate_task: comm=app pid=100 prio=52 orig_cpu=0 dest_cpu=1",
				" other-400 (400) [001] .... 1.040000: sched_switch: prev_comm=other prev_pid=400 prev_prio=52 prev_state=S ==> next_comm=app next_pid=100 next_prio=52",
			},
			wantCPU: 1, wantMs: 40, wantReason: RunnableCPUContinuityVerified,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeSchedulerCarryTrace(t, tc.name+".systrace", tc.lines...)
			full, err := BuildIndex(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			warm := buildSchedulerCarryWindow(t, path, 1.0, 1.05)
			q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.05, MinDurationMs: 0.001}
			for label, stats := range map[string]WindowStats{
				"full-index":  ComputeWindowStats(full, q),
				"window-head": ComputeWindowStats(warm, q),
			} {
				runnable := threadDurationForPID(stats.RunnableTop, 100)
				if runnable == nil || runnable.CPU != tc.wantCPU || !near(runnable.DurationMs, tc.wantMs, 0.001) {
					t.Fatalf("%s carry account drifted: %+v", label, stats.RunnableTop)
				}
				continuity := stats.RunnableCPUContinuity
				if continuity == nil || continuity.TotalSegments != 1 {
					t.Fatalf("%s continuity census missing: %+v", label, continuity)
				}
				if tc.wantReason == RunnableCPUContinuityVerified {
					if continuity.VerifiedSegments != 1 || continuity.UnknownSegments != 0 || len(continuity.Witnesses) != 0 {
						t.Fatalf("%s verified endpoint drifted: %+v", label, continuity)
					}
				} else if continuity.UnknownSegments != 1 || len(continuity.Witnesses) != 1 || continuity.Witnesses[0].Reason != tc.wantReason {
					t.Fatalf("%s open-end verdict drifted: %+v", label, continuity)
				}
			}
		})
	}
}

func TestWakeupCPUContinuityPressureAccountingIsUncapped(t *testing.T) {
	var trace strings.Builder
	for cpu := 0; cpu < 9; cpu++ {
		pid := 100 + cpu
		wake := 1.001 + float64(cpu)*0.001
		run := wake + 0.0005
		fmt.Fprintf(&trace, "waker-%d (%d) [%03d] .... %.6f: sched_wakeup: comm=app%d pid=%d prio=60 target_cpu=%03d\n", 300+cpu, 300+cpu, cpu, wake, cpu, pid, cpu)
		fmt.Fprintf(&trace, "other-%d (%d) [%03d] .... %.6f: sched_switch: prev_comm=other prev_pid=%d prev_prio=120 prev_state=S ==> next_comm=app%d next_pid=%d next_prio=60\n", 400+cpu, 400+cpu, cpu, run, 400+cpu, cpu, pid)
	}
	idx := buildTraceIndex(t, "wakeup_cpu_nine_cpus.systrace", trace.String())
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.02, MinDurationMs: 0.001})
	if len(stats.CPUPressure) != 8 {
		t.Fatalf("public pressure face must remain display-capped: %d %+v", len(stats.CPUPressure), stats.CPUPressure)
	}
	if len(stats.cpuPressureCensus) != 9 {
		t.Fatalf("accounting census must retain the ninth verified CPU: %d %+v", len(stats.cpuPressureCensus), stats.cpuPressureCensus)
	}
	foundCPU8 := false
	for _, pressure := range stats.cpuPressureCensus {
		if pressure.CPU == 8 && near(pressure.RunnableWaitMs, 0.5, 0.001) {
			foundCPU8 = true
		}
	}
	if !foundCPU8 {
		t.Fatalf("verified CPU 8 was lost behind the display cap: %+v", stats.cpuPressureCensus)
	}
	supply := stats.SupplyPressureSummary
	if supply == nil || !near(supply.RunnableWaitMs, 4.5, 0.001) ||
		!near(supply.CPUAttributedRunnableWaitMs, 4.5, 0.001) || !near(supply.CPUUnattributedRunnableWaitMs, 0, 0.001) {
		t.Fatalf("display-capped CPUs must not be relabelled unattributed: %+v", supply)
	}
}

func TestWakeupCPUContinuityPressureDisplayKeepsNestedCaps(t *testing.T) {
	pressure := map[int]*cpuPressureAcc{}
	for cpu := 0; cpu < 9; cpu++ {
		acc := cpuPressure(pressure, cpu)
		for member := 0; member < 9; member++ {
			thread := ThreadRef{Comm: fmt.Sprintf("cpu%d-thread%d", cpu, member), PID: 1000 + cpu*100 + member}
			accumulateThreadDuration(acc.runnable, thread, float64(20-member), cpu, 0, 1, 1.001, member+1, 60, "ohos_cfs")
			accumulateThreadDuration(acc.running, thread, float64(10-member)/2, cpu, 0, 1, 1.001, member+1, 60, "ohos_cfs")
			acc.runnableWaitMs += float64(20 - member)
			acc.runningMs += float64(10-member) / 2
		}
	}
	full := buildCPUPressureStats(pressure, 0)
	for i := range full {
		for member := 0; member < 9; member++ {
			full[i].OverlapCompetitors = append(full[i].OverlapCompetitors, ThreadDuration{
				Thread: ThreadRef{Comm: fmt.Sprintf("overlap-%d-%d", i, member), PID: 9000 + i*100 + member},
				CPU:    full[i].CPU, DurationMs: float64(9 - member),
			})
		}
	}
	public := capCPUPressureDisplay(full, 8, 8)
	if len(full) != 9 || len(full[0].TopRunnable) != 9 || len(full[0].TopRunning) != 9 || len(full[0].OverlapCompetitors) != 9 {
		t.Fatalf("private pressure census lost full accounting inventory: %+v", full)
	}
	if len(public) != 8 {
		t.Fatalf("public pressure CPU rows lost the outer cap: %d", len(public))
	}
	for _, item := range public {
		if len(item.TopRunnable) > 8 || len(item.TopRunning) > 8 || len(item.OverlapCompetitors) > 8 {
			t.Fatalf("public pressure nested roster exceeded its display cap: %+v", item)
		}
	}
	public[0].TopRunnable[0].DurationMs = -1
	if full[0].TopRunnable[0].DurationMs < 0 {
		t.Fatal("public display roster aliases and mutates the private census")
	}
	public[0].OverlapCompetitors[0].DurationMs = -1
	if full[0].OverlapCompetitors[0].DurationMs < 0 {
		t.Fatal("public overlap roster aliases and mutates the private census")
	}
}

func TestWakeupCPUContinuityNinthCPUFallbackCanMintRankSeat(t *testing.T) {
	full := make([]CPUPressureStats, 9)
	for cpu := range full {
		full[cpu] = CPUPressureStats{
			CPU: cpu, RunnableWaitMs: float64(20 - cpu), RunningMs: 1,
			TopRunnable: []ThreadDuration{{Thread: ThreadRef{Comm: fmt.Sprintf("waiter%d", cpu), PID: 100 + cpu}, CPU: cpu, LineStart: cpu + 1}},
		}
	}
	stats := WindowStats{CPUPressure: append([]CPUPressureStats(nil), full[:8]...)}
	stats.cpuPressureCensus = full
	rank := buildRootCauseRankFrom(nil, Query{TimeStart: 1, TimeEnd: 1.1}, ChainResult{}, stats)
	foundCPU8 := false
	count := 0
	for _, item := range rank.preTruncationItems {
		if item.Type != "cpu_pressure" {
			continue
		}
		count++
		if strings.Contains(item.Summary, "cpu=8 had") {
			foundCPU8 = true
		}
	}
	if count != 9 || !foundCPU8 {
		t.Fatalf("fallback rank mint treated public top-8 as full accounting: count=%d pre_truncation=%+v public=%+v", count, rank.preTruncationItems, rank.Items)
	}
}

func TestWakeupCPUContinuityAllZeroTargetCensusIsAdvisoryOnly(t *testing.T) {
	var trace strings.Builder
	for i := 0; i < wakeupTargetCPUDegradedFloor; i++ {
		pid := 1000 + i
		wake := 1.001 + float64(i)*0.00001
		run := wake + 0.000001
		headerCPU := 1 + i%2
		fmt.Fprintf(&trace, "waker-%d (%d) [%03d] .... %.6f: sched_wakeup: comm=app%d pid=%d prio=60 target_cpu=000\n", 3000+i, 3000+i, headerCPU, wake, i, pid)
		schedCPU := 0
		if i == wakeupTargetCPUDegradedFloor-1 {
			schedCPU = 1
		}
		fmt.Fprintf(&trace, "other-%d (%d) [%03d] .... %.6f: sched_switch: prev_comm=other prev_pid=%d prev_prio=120 prev_state=S ==> next_comm=app%d next_pid=%d next_prio=60\n", 4000+i, 4000+i, schedCPU, run, 4000+i, i, pid)
	}
	idx := buildTraceIndex(t, "wakeup_target_all_zero.systrace", trace.String())
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 1.01, MinDurationMs: 0.0001})
	continuity := stats.RunnableCPUContinuity
	if continuity == nil || continuity.TotalSegments != wakeupTargetCPUDegradedFloor ||
		continuity.VerifiedSegments != wakeupTargetCPUDegradedFloor-1 || continuity.UnknownSegments != 1 || continuity.MismatchSegments != 1 {
		t.Fatalf("all-zero heuristic overrode exact sched-in endpoints: %+v", continuity)
	}
	if !containsSubstring(stats.Caveats, "wakeup_target_cpu_degraded=true") {
		t.Fatalf("all-zero census disclosure missing: %+v", stats.Caveats)
	}
	var cpu0Wait float64
	for _, pressure := range stats.CPUPressure {
		if pressure.CPU == 0 {
			cpu0Wait = pressure.RunnableWaitMs
		}
		if pressure.CPU == 1 && pressure.RunnableWaitMs > 0 {
			t.Fatalf("the one endpoint mismatch fabricated CPU1 pressure: %+v", stats.CPUPressure)
		}
	}
	if cpu0Wait <= 0 {
		t.Fatalf("verified CPU0 segments were deleted by an advisory heuristic: %+v", stats.CPUPressure)
	}
}

func TestAggregateChainRunnableCensusPreservesThreadTotalAndFailsCPUMixedClosed(t *testing.T) {
	census := map[string]ThreadDuration{
		"unknown": {
			Thread: ThreadRef{PID: 100, TGID: 10, Comm: "app-old"}, CPU: -1, DurationMs: 18.537, anchoredMs: 12.895,
			StartTs: 1, EndTs: 1.028537,
			segCount: 2, segMinMs: 2, segMaxMs: 16.537,
			runnableIntervals: []foldInterval{{start: 1, end: 1.010, valueMs: 10}, {start: 1.020, end: 1.028537, valueMs: 8.537}},
		},
		"known": {
			Thread: ThreadRef{PID: 100, TGID: 10, Comm: "app-new"}, CPU: 0, DurationMs: 1.805, anchoredMs: 1.805,
			StartTs: 1.030, EndTs: 1.031805,
			Frequency: 1000, freqKnownMs: 1.805, freqWeightKHzMs: 1805,
			runnableIntervals: []foldInterval{{start: 1.030, end: 1.031805, valueMs: 1.805}},
		},
	}
	got := aggregateChainRunnableCensusByThread(census, map[int]bool{100: true}, 0)
	if len(got) != 1 || !near(got[0].DurationMs, 20.342, 0.001) || !near(got[0].anchoredMs, 14.700, 0.001) {
		t.Fatalf("formal chain account must conserve full and anchored totals: %+v", got)
	}
	if got[0].CPU != -1 || got[0].Frequency != 0 || got[0].freqKnownMs != 0 || got[0].freqWeightKHzMs != 0 {
		t.Fatalf("mixed known/unknown CPU account retained CPU-specific metadata: %+v", got[0])
	}
	if got[0].Thread.Comm != "app-new" {
		t.Fatalf("thread rename across CPU buckets kept stale display identity: %+v", got[0].Thread)
	}
	intervalTotal := 0.0
	for _, interval := range got[0].runnableIntervals {
		intervalTotal += interval.valueMs
	}
	if len(got[0].runnableIntervals) != 3 || !near(intervalTotal, 20.342, 0.001) {
		t.Fatalf("first runnable bucket was duplicated during thread aggregation: %+v", got[0].runnableIntervals)
	}
	sameCPU := map[string]ThreadDuration{
		"only": {Thread: ThreadRef{PID: 100}, CPU: 4, DurationMs: 3, anchoredMs: 2, Frequency: 1200},
	}
	got = aggregateChainRunnableCensusByThread(sameCPU, map[int]bool{100: true}, 0)
	if len(got) != 1 || got[0].CPU != 4 || got[0].Frequency != 1200 {
		t.Fatalf("one verified CPU should retain its exact scope: %+v", got)
	}
}

func TestChainRunnableMixedCPUKeepsOneAccountAndVerifiedInversion(t *testing.T) {
	worker := ThreadRef{Comm: "worker", PID: 200}
	app := ThreadRef{Comm: "app", PID: 100}
	cpu0 := ThreadDuration{
		Thread: worker, CPU: 0, DurationMs: 4, Priority: 52, PriorityClass: "ohos_cfs",
		StartTs: 1.000, EndTs: 1.004, LineStart: 10, LineEnd: 20,
		runnableIntervals: []foldInterval{{start: 1.000, end: 1.004, valueMs: 4}},
	}
	cpu1 := ThreadDuration{
		Thread: worker, CPU: 1, DurationMs: 6, Priority: 52, PriorityClass: "ohos_cfs",
		StartTs: 1.010, EndTs: 1.016, LineStart: 30, LineEnd: 40,
		runnableIntervals: []foldInterval{{start: 1.010, end: 1.016, valueMs: 6}},
	}
	unknown := ThreadDuration{
		Thread: worker, CPU: -1, DurationMs: 2, Priority: 52, PriorityClass: "ohos_cfs",
		StartTs: 1.020, EndTs: 1.022, LineStart: 50, LineEnd: 60,
		runnableIntervals: []foldInterval{{start: 1.020, end: 1.022, valueMs: 2}},
	}
	lower := ThreadDuration{
		Thread: ThreadRef{Comm: "lower", PID: 300}, CPU: 1, DurationMs: 3,
		Priority: 20, PriorityClass: "ohos_cfs", StartTs: 1.011, EndTs: 1.014,
		LineStart: 31, LineEnd: 39,
	}
	stats := WindowStats{
		Window: TimeWindow{StartTs: 1, EndTs: 1.03},
		runnableCensus: map[string]ThreadDuration{
			"cpu0": cpu0, "cpu1": cpu1, "unknown": unknown,
		},
		// The public top-N deliberately lacks the valid CPU1 context. Admission
		// must consume the private full census, not the display cap.
		RunnableContext: []RunnableContextSummary{{
			Thread: worker, CPU: 2, RunnableWaitMs: 12, Priority: 52,
			SameCPUTopRunning: []ThreadDuration{{Thread: lower.Thread, CPU: 2, DurationMs: 12, Priority: 20}},
		}},
		runnableContextCensus: []RunnableContextSummary{
			{Thread: worker, CPU: 1, RunnableWaitMs: 6, Priority: 52, PriorityClass: "ohos_cfs", SameCPUTopRunning: []ThreadDuration{lower}},
			{Thread: worker, CPU: -1, RunnableWaitMs: 2, Priority: 52, SameCPUTopRunning: []ThreadDuration{{Thread: lower.Thread, CPU: -1, DurationMs: 2, Priority: 20}}},
			{Thread: worker, CPU: 2, RunnableWaitMs: 12, Priority: 52, SameCPUTopRunning: []ThreadDuration{{Thread: lower.Thread, CPU: 2, DurationMs: 12, Priority: 20}}},
		},
	}
	chain := ChainResult{
		Target: app,
		Nodes: []ChainNode{
			{ID: "target", Thread: app, Depth: 0, Window: TimeWindow{StartTs: 1, EndTs: 1.03}},
			{ID: "worker", Thread: worker, Depth: 1, Window: TimeWindow{StartTs: 1, EndTs: 1.03}},
		},
	}
	rank := buildRootCauseRankFrom(nil, Query{TraceFlavor: TraceFlavorHarmonyHitrace, TimeStart: 1, TimeEnd: 1.03}, chain, stats)
	var got []RootCauseRankItem
	for _, item := range rank.Items {
		if sameThreadRef(item.Thread, worker) && rootCauseTypeIsPriorityInversion(item.Type) {
			got = append(got, item)
		}
	}
	if len(got) != 1 {
		t.Fatalf("mixed-CPU chain thread must own exactly one inversion rank seat: %+v", rank.Items)
	}
	item := got[0]
	if !near(item.ImpactMs, 12, 0.001) || !near(item.CumulativeImpactMs, 12, 0.001) || !near(item.RunnableMs, 12, 0.001) {
		t.Fatalf("thread-level raw runnable account was not conserved: %+v", item)
	}
	if !near(item.EffectiveImpactMs, 3, 0.001) || !near(item.GatedRunnableMs, 3, 0.001) {
		t.Fatalf("only the verified CPU1 inversion overlap may participate: %+v", item)
	}
	if item.runnableCPUKnown || item.MemberKey != "cpu=unknown" {
		t.Fatalf("aggregate row fabricated a concrete CPU identity: %+v", item)
	}
}

func TestOnChainDependencyRunnableInversionUsesProductionExactWiring(t *testing.T) {
	idx := buildTraceIndex(t, "on_chain_dependency_inversion.systrace", `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 1.000500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=52 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-400 (400) [000] .... 1.001000: sched_wakeup: comm=worker pid=200 prio=52 target_cpu=002
       idle-0 (0) [002] .... 1.002000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=lower next_pid=300 next_prio=20
      lower-300 (300) [002] .... 1.027000: sched_switch: prev_comm=lower prev_pid=300 prev_prio=20 prev_state=S ==> next_comm=worker next_pid=200 next_prio=52
     worker-200 (100) [002] .... 1.029000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 1.030000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=52 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        idle-0 (0) [001] .... 1.031000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	q := Query{
		View: "root_cause_rank", PID: 100, TimeStart: 1.0, TimeEnd: 1.04,
		MaxDepth: 4, MinDurationMs: 0.001, Limit: 16,
		TraceFlavorHint: TraceFlavorHarmonyHitrace,
	}
	res := Run(idx, q)
	if res.RootCauseRank == nil || res.SchedulerLatency == nil || res.WindowStats == nil || res.WakeupChain == nil {
		t.Fatalf("production rank lane did not publish its required faces: %+v", res)
	}
	for _, item := range res.SchedulerLatency.Items {
		if item.Thread.PID != 100 {
			t.Fatalf("public scheduler-latency face escaped the selected target: %+v", res.SchedulerLatency.Items)
		}
	}
	for _, ctx := range res.WindowStats.RunnableContext {
		if ctx.Thread.PID != 100 {
			t.Fatalf("public runnable-context face escaped the selected target: %+v", res.WindowStats.RunnableContext)
		}
	}
	var workerRows []RootCauseRankItem
	for _, item := range append(append([]RootCauseRankItem{}, res.RootCauseRank.Items...), res.RootCauseRank.AbsorbedItems...) {
		if item.Thread.PID == 200 && item.Type == "priority_inversion_runnable_wait" {
			workerRows = append(workerRows, item)
		}
	}
	if len(workerRows) != 1 {
		t.Fatalf("on-chain worker must own one exact inversion account: rank=%+v absorbed=%+v chain=%+v",
			res.RootCauseRank.Items, res.RootCauseRank.AbsorbedItems, res.WakeupChain)
	}
	worker := workerRows[0]
	if !near(worker.RunnableMs, 26, 0.001) || !near(worker.EffectiveImpactMs, 25, 0.001) ||
		!strings.Contains(worker.Summary, "same_cpu_competitor=lower") {
		t.Fatalf("dependency inversion lost raw/effective separation or competitor identity: %+v", worker)
	}
}

func TestRunnableInversionUsesFullExactCompetitorInventory(t *testing.T) {
	waiter := ThreadRef{Comm: "waiter", PID: 200}
	running := make([]pressureSegment, 0, 9)
	for i := 0; i < 9; i++ {
		prio := 52
		if i == 8 {
			prio = 20
		}
		running = append(running, pressureSegment{
			thread: ThreadRef{Comm: fmt.Sprintf("competitor-%d", i), PID: 300 + i},
			cpu:    1, startTs: 1 + float64(i)*0.001, endTs: 1 + float64(i+1)*0.001,
			line: 10 + i, priority: prio, priorityClass: "ohos_cfs",
		})
	}
	stats := WindowStats{
		Window: TimeWindow{StartTs: 1, EndTs: 1.009},
		CPU:    []CPUStats{{CPU: 1, BusyMs: 9}},
		cpuPressureCensus: []CPUPressureStats{{
			CPU: 1, RunningMs: 9, runningSegments: running,
		}},
		runnableSegments: []runnableWaitSegment{{
			thread: waiter, startTs: 1, endTs: 1.009, durationMs: 9,
			startLine: 1, endLine: 30, cpu: 1, cpuKnown: true,
			cpuContinuity: RunnableCPUContinuityVerified, priority: 52, priorityClass: "ohos_cfs",
		}},
	}
	idx := &Index{TimestampOrder: TraceTimestampOrderMonotonic, FirstTs: 1, LastTs: 1.009}
	latency := buildSchedulerLatencyStatsFromStats(idx, Query{TraceFlavor: TraceFlavorHarmonyHitrace, TimeStart: 1, TimeEnd: 1.009}, stats)
	if len(latency.itemsCensus) != 1 || len(latency.itemsCensus[0].SameCPUTopRunning) != 9 || len(latency.itemsCensus[0].sameCPURunningSegments) != 9 {
		t.Fatalf("private scheduler context was capped before correctness consumption: %+v", latency.itemsCensus)
	}
	if len(latency.Items) != 1 || len(latency.Items[0].SameCPUTopRunning) != 8 || latency.Items[0].sameCPURunningSegments != nil {
		t.Fatalf("public scheduler context did not keep its deep top-8 cap: %+v", latency.Items)
	}
	fullContexts := computeRunnableContextSummaries(latency.itemsCensus, nil, nil, nil, 0)
	publicContexts := capRunnableContextDisplay(fullContexts, 8)
	if len(fullContexts) != 1 || len(fullContexts[0].SameCPUTopRunning) != 9 || len(fullContexts[0].sameCPURunningSegments) != 9 {
		t.Fatalf("private runnable context lost the ninth exact competitor: %+v", fullContexts)
	}
	if len(publicContexts) != 1 || len(publicContexts[0].SameCPUTopRunning) != 8 || publicContexts[0].sameCPURunningSegments != nil {
		t.Fatalf("public runnable context leaked its private competitor inventory: %+v", publicContexts)
	}
	stats.runnableContextCensus = fullContexts
	stats.RunnableContext = publicContexts
	td := ThreadDuration{Thread: waiter, CPU: 1, DurationMs: 9, StartTs: 1, EndTs: 1.009, Priority: 52}
	item := rootCauseItem("runnable_wait", waiter, 9, 0.76, 1, 30, "window_stats", "waiter runnable")
	applyRunnableTopPriorityInversion(idx, Query{TraceFlavor: TraceFlavorHarmonyHitrace}, stats, td, &item)
	if item.Type != "priority_inversion_runnable_wait" || !near(item.EffectiveImpactMs, 1, 0.001) {
		t.Fatalf("ninth lower-priority exact competitor did not participate: %+v", item)
	}
}

func TestRunnableInversionUsesPriorityAtEachExactSegment(t *testing.T) {
	waiter := ThreadRef{Comm: "waiter", PID: 200}
	competitor := ThreadRef{Comm: "dynamic", PID: 300}
	ctx := RunnableContextSummary{
		Thread: waiter, CPU: 1, RunnableWaitMs: 5, Priority: 52,
		// The thread aggregate ends at equal priority; using this display field
		// for the hard gate would erase the earlier 1ms lower-priority segment.
		SameCPUTopRunning: []ThreadDuration{{Thread: competitor, CPU: 1, DurationMs: 5, Priority: 52, StartTs: 1, EndTs: 1.005}},
		sameCPURunningSegments: []ThreadDuration{
			{Thread: competitor, CPU: 1, DurationMs: 1, Priority: 20, StartTs: 1, EndTs: 1.001},
			{Thread: competitor, CPU: 1, DurationMs: 4, Priority: 52, StartTs: 1.001, EndTs: 1.005},
		},
	}
	stats := WindowStats{runnableContextCensus: []RunnableContextSummary{ctx}}
	td := ThreadDuration{Thread: waiter, CPU: 1, DurationMs: 5, StartTs: 1, EndTs: 1.005, Priority: 52}
	item := rootCauseItem("runnable_wait", waiter, 5, 0.76, 1, 10, "window_stats", "waiter runnable")
	applyRunnableTopPriorityInversion(nil, Query{TraceFlavor: TraceFlavorHarmonyHitrace}, stats, td, &item)
	if item.Type != "priority_inversion_runnable_wait" || !near(item.EffectiveImpactMs, 1, 0.001) {
		t.Fatalf("dynamic priority was flattened to the aggregate's last value: %+v", item)
	}
}

func TestStateChurnMixedCPUContinuityCannotClaimOneCPU(t *testing.T) {
	thread := ThreadRef{Comm: "churn", PID: 200}
	items := []ThreadStateChurnSummary{{
		Thread: thread, DominantState: string(StateRunnable), RunnableMs: 10,
		NextStep: "generic", Summary: "generic",
	}}
	pressures := []CPUPressureStats{{
		CPU: 1, CoreClass: "big",
		TopRunnable: []ThreadDuration{{Thread: thread, CPU: 1, DurationMs: 3}},
	}}
	census := map[string]ThreadDuration{
		"known":   {Thread: thread, CPU: 1, DurationMs: 3},
		"unknown": {Thread: thread, CPU: -1, DurationMs: 7},
	}
	got := enrichStateChurnWithCPUPressure(items, pressures, census)
	if len(got) != 1 || got[0].RunnableMs != 10 || got[0].RunnableCPUKnown || got[0].TopCompetitor != "" || strings.Contains(got[0].Summary, "same_cpu=") {
		t.Fatalf("mixed known/unknown churn account leaked onto one CPU: %+v", got)
	}
}

func TestRunnableContextJoinCannotCrossKnownUnknownCPULanes(t *testing.T) {
	known := RunnableContextSummary{
		Thread: ThreadRef{PID: 100}, CPU: 2, LineStart: 20, LineEnd: 30,
		CoreClass: "big", SameCPUTopRunning: []ThreadDuration{{Thread: ThreadRef{PID: 200}, DurationMs: 3}},
	}
	unknown := RunnableContextSummary{Thread: ThreadRef{PID: 100}, CPU: -1, LineStart: 40, LineEnd: 50}
	contexts := []RunnableContextSummary{known, unknown}

	ctx, ok := runnableContextForScope(ThreadRef{PID: 100}, -1, false, 40, 50, contexts)
	if !ok || ctx.CPU != -1 || ctx.CoreClass != "" || len(ctx.SameCPUTopRunning) != 0 {
		t.Fatalf("unknown segment inherited a known-CPU context: ok=%t %+v", ok, ctx)
	}
	ctx, ok = runnableContextForScope(ThreadRef{PID: 100}, 2, true, 20, 30, contexts)
	if !ok || ctx.CPU != 2 || ctx.CoreClass != "big" || len(ctx.SameCPUTopRunning) != 1 {
		t.Fatalf("exact known segment lost its matching context: ok=%t %+v", ok, ctx)
	}
	if _, ok = runnableContextForScope(ThreadRef{PID: 100}, -1, false, 1, 99,
		[]RunnableContextSummary{unknown, {Thread: ThreadRef{PID: 100}, CPU: -1, LineStart: 60, LineEnd: 70}}); ok {
		t.Fatal("an aggregate spanning multiple unknown contexts must fail closed")
	}
}

func TestUnknownCPURootCauseMemberLabelsNeverExposeNegativeSentinel(t *testing.T) {
	if got := rootCauseCPUMemberKey(-1); got != "cpu=unknown" {
		t.Fatalf("unknown CPU member key leaked its numeric sentinel: %q", got)
	}
	thread := ThreadRef{Comm: "blocked", PID: 42}
	stats := WindowStats{
		DStateTop: []ThreadDuration{{
			Thread: thread, CPU: -1, DurationMs: 2, StartTs: 1, EndTs: 1.002,
			segCount: 1, segMinMs: 2, segMaxMs: 2,
		}},
		IOWaitTop: []ThreadDuration{{
			Thread: thread, CPU: -1, DurationMs: 3, StartTs: 1.002, EndTs: 1.005,
			segCount: 1, segMinMs: 3, segMaxMs: 3,
		}},
	}
	items := rootCauseDIOStateFamilyItems(Query{TimeStart: 1, TimeEnd: 1.01}, stats, nil, false, true)
	if len(items) != 1 || len(items[0].MemberRoster) != 2 {
		t.Fatalf("fixture must mint one two-member D/IO family: %+v", items)
	}
	roster := strings.Join(items[0].MemberRoster, " | ")
	if strings.Contains(roster, "cpu=-1") || strings.Count(roster, "cpu=unknown") != 2 {
		t.Fatalf("D/IO roster exposed a negative CPU sentinel: %q", roster)
	}
}
