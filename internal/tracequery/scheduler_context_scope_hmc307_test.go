package tracequery

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// The checked-in source is also used by the natural business-IO eval. Query
// padding is deliberately not the business span or either runnable interval.
func TestHMC307PublicSchedulerSeparatesQueryBackgroundFromWait(t *testing.T) {
	idx := hmosperfEvalIndex(t, "hmosperf_business_io_chain")
	q := Query{View: "scheduler_latency_stats", PID: 100, TimeStart: .999, TimeEnd: 1.051, MinDurationMs: .05, Limit: 32}
	res := Run(idx, q)
	if res.SchedulerLatency == nil || len(res.SchedulerLatency.Items) != 1 {
		t.Fatalf("public scheduler result missing: %+v", res)
	}
	item := res.SchedulerLatency.Items[0]
	if item.ContextScope == nil || !item.ContextScope.QueryWindowKnown ||
		!near(item.ContextScope.QueryWindow.StartTs, .999, 1e-9) || !near(item.ContextScope.QueryWindow.EndTs, 1.051, 1e-9) ||
		!near(item.ContextScope.WaitInterval.StartTs, 1.045, 1e-9) || !near(item.ContextScope.WaitInterval.EndTs, 1.046, 1e-9) ||
		item.ContextScope.OtherCPUIdleUnit != "cpu_ms" {
		t.Fatalf("two different ranges or CPU-time unit lost: %+v", item.ContextScope)
	}
	wantWindowFields := []string{"same_cpu_busy_ms", "same_cpu_idle_ms", "other_cpu_idle_ms", "high_priority_running_ms", "system_or_kernel_running_ms"}
	if !reflect.DeepEqual(item.ContextScope.QueryWindowFields, wantWindowFields) {
		t.Fatalf("query totals were not classified exactly: %+v", item.ContextScope)
	}
	if !near(item.StartTs, 1.045, 1e-9) || !near(item.EndTs, 1.046, 1e-9) || !near(item.DurationMs, 1, 1e-6) ||
		!near(item.SameCPUBusyMs, 7, 1e-6) || !near(item.SameCPUIdleMs, 45, 1e-6) || !near(item.OtherCPUIdleMs, 37.5, 1e-6) {
		t.Fatalf("fixture changed; preserve original query totals and exact wait: %+v", item)
	}
	t.Logf("public wait %.6f..%.6f duration=%.3fms; original query %.6f..%.6f busy=%.3f idle=%.3f other_cpu_idle=%.3f; summary=%s", item.StartTs, item.EndTs, item.DurationMs, res.SchedulerLatency.Window.StartTs, res.SchedulerLatency.Window.EndTs, item.SameCPUBusyMs, item.SameCPUIdleMs, item.OtherCPUIdleMs, item.Summary)
	if !strings.Contains(item.Summary, "query_window_other_cpu_idle=") {
		t.Errorf("whole-query background is still presented as an unscoped wait attribute: %s", item.Summary)
	}
	q.View = "window_stats"
	stats := Run(idx, q).WindowStats
	if stats == nil {
		t.Fatal("public stats missing")
	}
	for _, ctx := range stats.RunnableContext {
		if ctx.Thread.PID == 100 {
			if ctx.ContextScope == nil || ctx.ContextScope.WaitIntervalFields[0] != "runnable_wait_ms" ||
				!reflect.DeepEqual(ctx.ContextScope.QueryWindowFields, wantWindowFields) ||
				ctx.ContextScope.QueryWindow != item.ContextScope.QueryWindow || ctx.ContextScope.WaitInterval != item.ContextScope.WaitInterval {
				t.Fatalf("scope changed at native context handoff: %+v", ctx.ContextScope)
			}
			if ctx.Verdict != "insufficient_signal" {
				t.Errorf("whole-query other CPU idle became a local scheduling verdict: %+v", ctx)
			}
			if !strings.Contains(ctx.Summary, "query_window_same_cpu_busy=") || !strings.Contains(ctx.Summary, "query_window_other_cpu_idle=") {
				t.Errorf("runnable handoff lost query background scope: %s", ctx.Summary)
			}
			return
		}
	}
	t.Fatal("main runnable context missing")
}

func TestHMC307PublicSchedulerPreservesOnlyRealCompetition(t *testing.T) {
	for _, priority := range []struct{ name, value string }{{"normal", "1"}, {"rt", "80"}, {"system", "160"}} {
		for _, overlap := range []bool{false, true} {
			t.Run(priority.name+map[bool]string{false: "/outside", true: "/overlap"}[overlap], func(t *testing.T) {
				wake := "1.008000"
				if overlap {
					wake = "1.007000"
				}
				idx := buildTraceIndex(t, "competition.systrace", strings.NewReplacer("PRIORITY", priority.value, "WAKE", wake).Replace(`
other-200 (200) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=200 next_prio=PRIORITY
waker-300 (300) [000] .... WAKE: sched_wakeup: comm=app pid=100 prio=120 target_cpu=000
other-200 (200) [000] .... 1.008000: sched_switch: prev_comm=other prev_pid=200 prev_prio=PRIORITY prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
app-100 (100) [000] .... 1.009000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
app-100 (100) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
`))
				res := Run(idx, Query{View: "window_stats", PID: 100, TimeStart: 1, TimeEnd: 1.01, MinDurationMs: .05, Limit: 32, TraceFlavorHint: TraceFlavorHarmonyHitrace})
				if res.WindowStats == nil || len(res.WindowStats.RunnableContext) != 1 {
					t.Fatalf("context absent: %+v", res.WindowStats)
				}
				ctx := res.WindowStats.RunnableContext[0]
				want := "insufficient_signal"
				if overlap {
					want = "cpu_pressure"
				}
				if ctx.Verdict != want {
					t.Fatalf("want=%s got=%+v", want, ctx)
				}
				if priority.name == "rt" && !near(ctx.HighPriorityRunningMs, 9, 1e-6) {
					t.Fatalf("full-window RT total changed: %+v", ctx)
				}
				if priority.name == "system" && !near(ctx.SystemOrKernelRunningMs, 8, 1e-6) {
					t.Fatalf("full-window system total changed: %+v", ctx)
				}
				if overlap && (len(ctx.SameCPUTopRunning) != 1 || ctx.SameCPUTopRunning[0].Thread.PID != 200 || !near(ctx.SameCPUTopRunning[0].DurationMs, 1, 1e-6)) {
					t.Fatalf("exact competitor lost: %+v", ctx)
				}
			})
		}
	}
}

func TestHMC307SchedulerScopeUnknownAndCloneIsolation(t *testing.T) {
	segment := runnableWaitSegment{startTs: 0, endTs: .001}
	unknown := schedulerContextScope(Query{LineStart: 5, LineEnd: 9}, segment, CPUStats{}, -1)
	if unknown.QueryWindowKnown || unknown.CPUBusyIdleStatus != CPUBusyIdleStatusUnavailable || unknown.CPUBusyIdleReason != "wait_cpu_not_proven" || !strings.Contains(SchedulerContextScopeSummary(unknown), "query_window=unknown") {
		t.Fatalf("missing bounds or CPU became known: %+v", unknown)
	}
	zero := schedulerContextScope(Query{TimeStartSet: true, TimeEnd: .010}, segment, CPUStats{BusyIdleStatus: CPUBusyIdleStatusMeasured}, 0)
	if !zero.QueryWindowKnown || zero.QueryWindow.StartTs != 0 || zero.QueryWindow.EndTs != .01 || zero.CPUBusyIdleStatus != CPUBusyIdleStatusMeasured {
		t.Fatalf("known zero boundary lost: %+v", zero)
	}
	for _, scope := range []*SchedulerContextScope{unknown, zero} {
		raw, err := json.Marshal(scope)
		if err != nil {
			t.Fatal(err)
		}
		var restored SchedulerContextScope
		if err := json.Unmarshal(raw, &restored); err != nil {
			t.Fatal(err)
		}
		if SchedulerContextScopeSummary(&restored) != SchedulerContextScopeSummary(scope) {
			t.Fatalf("scope changed on persisted roundtrip: %s", raw)
		}
	}
	items := []SchedulerLatencyItem{{ContextScope: zero}}
	clone := cloneSchedulerLatencyItemsForAccounting(items)
	clone[0].ContextScope.QueryWindowFields[0] = "changed"
	contexts := computeRunnableContextSummaries(items, nil, nil, nil, 0)
	contexts[0].ContextScope.QueryWindowFields[1] = "changed"
	display := capRunnableContextDisplay(contexts, 1)
	display[0].ContextScope.WaitIntervalFields[0] = "changed"
	if zero.QueryWindowFields[0] != "same_cpu_busy_ms" || zero.QueryWindowFields[1] != "same_cpu_idle_ms" || zero.WaitIntervalFields[0] != "duration_ms" || contexts[0].ContextScope.WaitIntervalFields[0] != "runnable_wait_ms" {
		t.Fatal("display mutation aliased original scope")
	}
	if !strings.Contains(SchedulerContextScopeSummary(nil), "unknown") {
		t.Fatal("legacy missing scope invented window")
	}
}

func TestHMC307PublicSchedulerOutsideWaitRunningIsNotLocalPressure(t *testing.T) {
	idx := buildTraceIndex(t, "scheduler_scope.systrace", `
other-200 (200) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=200 next_prio=120
other-200 (200) [000] .... 1.008000: sched_switch: prev_comm=other prev_pid=200 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
waker-300 (300) [000] .... 1.008000: sched_wakeup: comm=app pid=100 prio=120 target_cpu=000
app-100 (100) [000] .... 1.009000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
app-100 (100) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
`)
	res := Run(idx, Query{View: "window_stats", PID: 100, TimeStart: 1, TimeEnd: 1.010, MinDurationMs: .05, Limit: 32})
	if res.WindowStats == nil || len(res.WindowStats.RunnableContext) != 1 {
		t.Fatalf("public context missing: %+v", res)
	}
	ctx := res.WindowStats.RunnableContext[0]
	if !near(ctx.RunnableWaitMs, 1, 1e-6) || !near(ctx.SameCPUBusyMs, 9, 1e-6) || !near(ctx.SameCPUIdleMs, 1, 1e-6) || len(ctx.SameCPUTopRunning) != 0 {
		t.Fatalf("test must isolate query-total activity entirely outside the wait: %+v", ctx)
	}
	if ctx.Verdict != "insufficient_signal" {
		t.Fatalf("outside-wait activity minted local CPU pressure: %+v", ctx)
	}
}
