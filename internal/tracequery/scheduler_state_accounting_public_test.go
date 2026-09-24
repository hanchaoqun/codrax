package tracequery

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// Real native public regression for the live handoff failure: the old
// window-accounted tail remains 1ms, but never claims an observed state end.
func TestSchedulerStateAccountingPublicOpenTail(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/hmosperf_scheduler_concurrency/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	r := Run(idx, Query{View: "window_stats", PID: 104, TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true})
	if r.WindowStats == nil {
		t.Fatal("no native window statistics")
	}
	for _, row := range r.WindowStats.RunnableTop {
		if row.Thread.PID != 104 {
			continue
		}
		if !near(row.DurationMs, 1, 1e-9) || row.LineEnd == 0 {
			t.Fatalf("fixture must keep old nonzero window-tail value and display fallback: %+v", row)
		}
		data, _ := json.Marshal(row)
		var wire map[string]json.RawMessage
		_ = json.Unmarshal(data, &wire)
		if len(wire["accounting"]) == 0 {
			t.Fatalf("native window-accounted open tail lost closure semantics: %s", data)
		}
		assertStateAccounting(t, row.Accounting, "runnable", 1, 0, 1, 0, 0, 1)
		for _, step := range r.WindowStats.StateDrilldownPlan {
			if step.Thread.PID == 104 && step.State == "runnable" {
				if !reflect.DeepEqual(step.Accounting, row.Accounting) || step.Accounting == row.Accounting {
					t.Fatal("drilldown must preserve an independent copy of the open-tail account")
				}
				return
			}
		}
		t.Fatal("open-tail drilldown missing")
		return
	}
	t.Fatal("missing real TID104 open-tail row")
}

func assertStateAccounting(t *testing.T, a *SchedulerStateAccounting, state string, segments, observed, open, unknown int, observedMs, openMs float64) {
	t.Helper()
	if a == nil || a.State != state || a.Caliber != "cumulative_segments" || a.SegmentCount != segments ||
		a.ObservedEndCount != observed || a.OpenTailCount != open || a.UnknownClosureCount != unknown ||
		!near(a.ObservedEndMs, observedMs, 1e-8) || !near(a.OpenTailMs, openMs, 1e-8) {
		t.Fatalf("unexpected state population: %+v; want %s segments=%d observed=%d/%.9g open=%d/%.9g unknown=%d", a, state, segments, observed, observedMs, open, openMs, unknown)
	}
}

func TestSchedulerStateAccountingPublicMixedStatesAndClipping(t *testing.T) {
	for _, tc := range []struct{ prev, state string }{{"R", "runnable"}, {"S", "s_sleep"}, {"D", "d_sleep"}, {"D", "io_wait"}} {
		t.Run(tc.state, func(t *testing.T) {
			rows := []string{concurrencyPublicSwitch(1, 0, 0, 41, "S"), concurrencyPublicSwitch(1.001, 0, 41, 0, tc.prev),
				concurrencyPublicSwitch(1.004, 0, 0, 41, "S"), concurrencyPublicSwitch(1.005, 0, 41, 0, tc.prev),
				"idle-0 (0) [000] .... 1.007000: cpu_frequency: state=1000000 cpu_id=0"}
			if tc.state == "io_wait" {
				for _, ts := range []float64{1.0025, 1.0055} {
					rows = append(rows, fmt.Sprintf("worker-41 (41) [000] .... %.9f: sched_blocked_reason: pid=41 iowait=1 caller=wait_on_page", ts))
				}
			}
			idx := buildTraceIndex(t, "states.systrace", concurrencyPublicTrace(rows...))
			q := Query{View: "window_stats", PID: 41, TimeStart: 1.002, TimeEnd: 1.006, TimeStartSet: true, TimeEndSet: true}
			r := Run(idx, q)
			var accounts *SchedulerStateAccounting
			var duration float64
			var selected []ThreadDuration
			switch tc.state {
			case "runnable":
				selected = r.WindowStats.RunnableTop
			case "s_sleep":
				selected = r.WindowStats.SleepTop
			case "d_sleep":
				selected = r.WindowStats.DStateTop
			case "io_wait":
				selected = r.WindowStats.IOWaitTop
			}
			for _, row := range selected {
				if row.Thread.PID != 41 {
					continue
				}
				duration += row.DurationMs
				if accounts == nil {
					accounts = cloneThreadDurationMeasurement(row).Accounting
				} else {
					accounts = mergeSchedulerStateAccounting(accounts, row.Accounting)
				}
			}
			assertStateAccounting(t, accounts, tc.state, 2, 1, 1, 0, 2, 1)
			if !near(duration, 3, 1e-8) || accounts.StartClippedCount != 1 || accounts.EndClippedCount != 0 {
				t.Fatalf("old cumulative value or clipping changed: duration=%g account=%+v", duration, accounts)
			}
		})
	}
}

func TestSchedulerStateAccountingPublicRunningClosureAndQueryCuts(t *testing.T) {
	for _, closed := range []bool{true, false} {
		t.Run(fmt.Sprint(closed), func(t *testing.T) {
			rows := []string{concurrencyPublicSwitch(1, 0, 0, 41, "S"), "idle-0 (0) [001] .... 1.010000: cpu_frequency: state=1000000 cpu_id=1"}
			if closed {
				rows = append(rows, concurrencyPublicSwitch(1.009, 0, 41, 0, "S"))
			}
			idx := buildTraceIndex(t, "running.systrace", concurrencyPublicTrace(rows...))
			r := Run(idx, Query{View: "window_stats", PID: 41, TimeStart: 1.002, TimeEnd: 1.006, TimeStartSet: true, TimeEndSet: true})
			row := b1638b2aTargetDuration(t, r.WindowStats.TopRunning, 41)
			if closed {
				assertStateAccounting(t, row.Accounting, "running", 1, 1, 0, 0, 4, 0)
			} else {
				assertStateAccounting(t, row.Accounting, "running", 1, 0, 1, 0, 0, 4)
			}
			if row.Accounting.StartClippedCount != 1 || row.Accounting.EndClippedCount != boolToAccountingCount(closed) || !near(row.DurationMs, 4, 1e-8) {
				t.Fatalf("clipped start/end must not become physical endpoints: %+v", row)
			}
		})
	}
}

func boolToAccountingCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestSchedulerStateAccountingPublicChurnPerStateAndGaps(t *testing.T) {
	idx := b1638b2aRunningChurnIndex(t)
	r := Run(idx, Query{View: "window_stats", PID: 300, TimeStart: 5, TimeEnd: 5.07, TimeStartSet: true, TimeEndSet: true, MinDurationMs: .000001})
	if len(r.WindowStats.StateChurn) != 1 {
		t.Fatal("churn fixture absent")
	}
	churn := r.WindowStats.StateChurn[0]
	values := map[string]float64{"running": churn.RunningMs, "runnable": churn.RunnableMs, "s_sleep": churn.SleepMs, "d_sleep": churn.DStateMs, "io_wait": churn.IOWaitMs}
	for _, account := range churn.StateAccounting {
		if !near(account.ObservedEndMs+account.OpenTailMs+account.UnknownClosureMs, values[account.State], 1e-8) {
			t.Fatalf("churn state borrowed another population: %+v values=%v", account, values)
		}
	}
	for _, step := range r.WindowStats.StateDrilldownPlan {
		if step.Thread.PID != 300 || step.Source != "state_churn" {
			continue
		}
		want := schedulerStateAccountingForState(churn.StateAccounting, step.State)
		if !reflect.DeepEqual(step.Accounting, want) || step.Accounting == nil || step.Accounting.SegmentCount < 2 || !near(step.ImpactMs, step.Accounting.ObservedEndMs+step.Accounting.OpenTailMs, 1e-8) {
			t.Fatalf("non-contiguous dominant account lost its own population: %+v want=%+v", step, want)
		}
		return
	}
	t.Fatal("native churn drilldown missing")
}

func TestSchedulerStateAccountingLegacyUnknownAndMergeOwnership(t *testing.T) {
	legacy := ThreadDuration{Thread: ThreadRef{PID: 41}, CPU: 0, DurationMs: 1, StartTs: 1, EndTs: 2, LineStart: 1, LineEnd: 9}
	steps, _ := buildStateDrilldownPlan(WindowStats{RunnableTop: []ThreadDuration{legacy}}, 8)
	if len(steps) != 1 || steps[0].Accounting != nil {
		t.Fatal("legacy fallback endpoints fabricated closure metadata")
	}
	known := legacy
	known.Accounting = &SchedulerStateAccounting{State: "runnable", Caliber: "cumulative_segments", SegmentCount: 1, OpenTailCount: 1, OpenTailMs: 1}
	for _, rows := range [][]ThreadDuration{{known, legacy, known}, {legacy, known}} {
		dst := map[string]ThreadDuration{}
		for _, row := range rows {
			streamStateAccumulateDuration(dst, row, false)
		}
		if dst[threadKey(legacy.Thread)].Accounting != nil {
			t.Fatal("known sibling repaired missing legacy membership")
		}
	}
	copy := cloneThreadDurationMeasurement(known)
	copy.Accounting.OpenTailMs = 9
	if known.Accounting.OpenTailMs != 1 {
		t.Fatal("derived row mutated native accounting")
	}
}

func TestSchedulerStateAccountingPublicMigrationAndUnknownRunningEnd(t *testing.T) {
	idx := buildTraceIndex(t, "migration.systrace", concurrencyPublicTrace(concurrencyPublicWake(1, 41),
		"worker-41 (41) [000] .... 1.002000: sched_migrate_task: comm=worker pid=41 prio=120 orig_cpu=0 dest_cpu=1",
		concurrencyPublicSwitch(1.004, 1, 0, 41, "S"), concurrencyPublicSwitch(1.005, 1, 41, 0, "S")))
	r := Run(idx, Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.006, TimeStartSet: true, TimeEndSet: true})
	var merged *SchedulerStateAccounting
	for _, row := range r.WindowStats.RunnableTop {
		if row.Thread.PID != 41 {
			continue
		}
		if merged == nil {
			merged = cloneThreadDurationMeasurement(row).Accounting
		} else {
			merged = mergeSchedulerStateAccounting(merged, row.Accounting)
		}
	}
	assertStateAccounting(t, merged, "runnable", 2, 2, 0, 0, 4, 0)
	if merged.BoundaryContinuationCount != 1 {
		t.Fatalf("migration misrepresented as state termination: %+v", merged)
	}
	// A real following CPU switch without this thread as prev_pid is not
	// proof of its physical close. Preserve old CPU-accounted milliseconds.
	idx = buildTraceIndex(t, "mismatch.systrace", concurrencyPublicTrace(concurrencyPublicSwitch(1, 0, 0, 41, "S"), concurrencyPublicSwitch(1.004, 0, 99, 0, "S")))
	r = Run(idx, Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.006, TimeStartSet: true, TimeEndSet: true})
	row := b1638b2aTargetDuration(t, r.WindowStats.TopRunning, 41)
	assertStateAccounting(t, row.Accounting, "running", 1, 0, 0, 1, 0, 0)
	if !near(row.Accounting.UnknownClosureMs, row.DurationMs, 1e-8) {
		t.Fatal("unknown physical close erased native value")
	}
}

func TestSchedulerStateAccountingPublicZeroWindowedAndStream(t *testing.T) {
	idx := buildTraceIndex(t, "zero.systrace", concurrencyPublicTrace(concurrencyPublicSwitch(0, 0, 0, 41, "S"), concurrencyPublicSwitch(.000000001, 0, 41, 0, "S")))
	r := Run(idx, Query{View: "window_stats", TimeStartSet: true, TimeEndSet: true, TimeEnd: .000000002})
	row := b1638b2aTargetDuration(t, r.WindowStats.TopRunning, 41)
	assertStateAccounting(t, row.Accounting, "running", 1, 1, 0, 0, .000001, 0)
	if row.Accounting.StartClippedCount != 0 || row.Accounting.EndClippedCount != 0 {
		t.Fatal("real zero or nanosecond confused with missing bounds")
	}
	path := writeSchedulerCarryTrace(t, "carry.systrace", concurrencyPublicSwitch(.2, 0, 0, 41, "S"), concurrencyPublicSwitch(1.04, 0, 41, 0, "S"))
	windowed := buildSchedulerCarryWindow(t, path, 1, 1.05)
	q := Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.05, TimeStartSet: true, TimeEndSet: true}
	r = Run(windowed, q)
	row = b1638b2aTargetDuration(t, r.WindowStats.TopRunning, 41)
	assertStateAccounting(t, row.Accounting, "running", 1, 1, 0, 0, 40, 0)
	if row.Accounting.StartClippedCount != 1 {
		t.Fatalf("bounded head lost physical origin: %+v", row.Accounting)
	}
	stream, err := StreamStateCluster(context.Background(), "../../eval/fixtures/hmosperf_scheduler_concurrency/events.systrace", Query{TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true}, 16)
	if err != nil {
		t.Fatal(err)
	}
	row = b1638b2aTargetDuration(t, stream.WindowStats.RunnableTop, 104)
	assertStateAccounting(t, row.Accounting, "runnable", 1, 0, 1, 0, 0, 1)
	for _, churn := range stream.WindowStats.StateChurn {
		if churn.Thread.PID != 104 {
			continue
		}
		assertStateAccounting(t, schedulerStateAccountingForState(churn.StateAccounting, "runnable"), "runnable", 1, 0, 1, 0, 0, 1)
	}
}

func TestSchedulerStateAccountingPublicFiveStateChurn(t *testing.T) {
	idx := buildTraceIndex(t, "churn.systrace", concurrencyPublicTrace(
		concurrencyPublicSwitch(1, 0, 0, 41, "S"), concurrencyPublicSwitch(1.001, 0, 41, 0, "R"),
		concurrencyPublicSwitch(1.002, 0, 0, 41, "S"), concurrencyPublicSwitch(1.003, 0, 41, 0, "S"),
		concurrencyPublicWake(1.004, 41), concurrencyPublicSwitch(1.005, 0, 0, 41, "S"),
		concurrencyPublicSwitch(1.006, 0, 41, 0, "D"), "worker-41 (41) [000] .... 1.006500: sched_blocked_reason: pid=41 iowait=1 caller=wait_on_page",
		concurrencyPublicSwitch(1.008, 0, 0, 41, "S"), concurrencyPublicSwitch(1.009, 0, 41, 0, "D"),
		"worker-41 (41) [000] .... 1.009500: sched_blocked_reason: pid=41 iowait=0 caller=dma_fence_wait",
		concurrencyPublicSwitch(1.011, 0, 0, 41, "S"), concurrencyPublicSwitch(1.012, 0, 41, 0, "S"),
		"idle-0 (0) [000] .... 1.015000: cpu_frequency: state=1000000 cpu_id=0"))
	q := Query{View: "window_stats", PID: 41, TimeStart: 1, TimeEnd: 1.014, TimeStartSet: true, TimeEndSet: true}
	r := Run(idx, q)
	if len(r.WindowStats.StateChurn) != 1 {
		t.Fatal("five-state native churn fixture missing")
	}
	churn := r.WindowStats.StateChurn[0]
	if len(churn.StateAccounting) != 5 {
		t.Fatalf("lost state partitions: %+v", churn.StateAccounting)
	}
	wants := map[string]struct {
		n  int
		ms float64
	}{"running": {5, 5}, "runnable": {2, 2}, "s_sleep": {2, 3}, "d_sleep": {1, 2}, "io_wait": {1, 2}}
	for _, a := range churn.StateAccounting {
		want := wants[a.State]
		open, openMs := 0, 0.0
		if a.State == "s_sleep" {
			open, openMs = 1, 2
		}
		assertStateAccounting(t, &a, a.State, want.n, want.n-open, open, 0, want.ms-openMs, openMs)
	}
	base, _ := json.Marshal(r)
	ctx, cancel := context.WithCancel(context.Background())
	armed, _ := json.Marshal(Run(idx, q.WithRunContext(ctx)))
	if string(base) != string(armed) {
		t.Fatal("armed context changed complete state accounts")
	}
	cancel()
	if canceled := Run(idx, q.WithRunContext(ctx)); canceled.WindowStats != nil || canceled.ViewCancellation == nil {
		t.Fatal("cancellation published partial state account")
	}
}
