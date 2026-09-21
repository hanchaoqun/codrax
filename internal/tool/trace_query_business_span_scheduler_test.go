package tool

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryBusinessSpanSchedulerPublicExactMarkerRulers(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("business intervals")}
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "root_cause_rank", "pid": 100, "time_start": .999, "time_end": 1.051})
	if !result.Success {
		t.Fatal(result.Summary)
	}
	var payloadPath string
	for _, record := range result.Observations {
		if record.SourceRef.PayloadRef != "" {
			payloadPath = record.SourceRef.PayloadRef
			break
		}
	}
	data, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	// Decode the public wire shape: this assertion was RED before the native
	// carrier existed, rather than depending on a new test-only constructor.
	var wire struct {
		TargetWindowStates struct {
			Running float64 `json:"running_ms"`
			Total   float64 `json:"total_ms"`
		} `json:"target_window_states"`
		Stats struct {
			Spans []struct {
				Name      string `json:"name"`
				Scheduler *struct {
					Coverage  string  `json:"coverage"`
					Running   float64 `json:"running_ms"`
					Runnable  float64 `json:"runnable_ms"`
					Sleep     float64 `json:"sleep_ms"`
					Accounted float64 `json:"accounted_ms"`
				} `json:"scheduler_states"`
			} `json:"trace_spans"`
		} `json:"window_stats"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if math.Abs(wire.TargetWindowStates.Running-7) > 1e-6 || math.Abs(wire.TargetWindowStates.Total-52) > 1e-6 {
		t.Fatalf("broad target ruler was replaced: %+v", wire.TargetWindowStates)
	}
	for name, want := range map[string][]float64{"OpenDocument": {5, 1, 44, 50}, "LoadDocumentIndex": {8, 1, 31, 40}} {
		found := false
		for _, span := range wire.Stats.Spans {
			if span.Name != name {
				continue
			}
			found = true
			if span.Scheduler == nil {
				t.Errorf("public query lost marker-local scheduler account for %s", name)
				continue
			}
			got := []float64{span.Scheduler.Running, span.Scheduler.Runnable, span.Scheduler.Sleep, span.Scheduler.Accounted}
			for i := range want {
				if math.Abs(got[i]-want[i]) > 1e-6 {
					t.Errorf("%s marker ruler[%d]=%g want %g", name, i, got[i], want[i])
				}
			}
			if span.Scheduler.Coverage != "complete" {
				t.Errorf("complete marker partition lost coverage: %+v", span.Scheduler)
			}
		}
		if !found {
			t.Errorf("missing marker %s", name)
		}
		published := false
		for _, observation := range result.Observations {
			if observation.Predicate == types.TraceBusinessSpanPredicate && observation.Object == name &&
				strings.Contains(strings.Join(observation.RichNotes, "\n"), "business_span_scheduler_states=") {
				published = true
				row, ok := types.TraceNoteKeyLookup("business_span_scheduler_states")
				if !ok || row.Family != "business_span" || row.Carrier != types.TraceNoteCarrierSoftConsumer {
					t.Errorf("public emitted scheduler note bypassed the factual-context registry: %+v", row)
				}
			}
		}
		if !published {
			t.Errorf("native %s scheduler account did not reach typed public observation", name)
		}
	}
}

func businessSpanSchedulerPublicPayload(t *testing.T, result types.ToolResult) tracequery.Result {
	t.Helper()
	if !result.Success {
		t.Fatal(result.Summary)
	}
	for _, record := range result.Observations {
		if record.SourceRef.PayloadRef == "" {
			continue
		}
		data, err := os.ReadFile(record.SourceRef.PayloadRef)
		if err != nil {
			t.Fatal(err)
		}
		var payload tracequery.Result
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}
		return payload
	}
	t.Fatal("public query omitted payload reference")
	return tracequery.Result{}
}

func TestTraceQueryBusinessSpanSchedulerPublicClipping(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("clipped business intervals")}
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "time_start": 1.01, "time_end": 1.02})
	payload := businessSpanSchedulerPublicPayload(t, result)
	if payload.WindowStats == nil || len(payload.WindowStats.TraceSpans) != 2 {
		t.Fatalf("missing clipped business spans: %+v", payload.WindowStats)
	}
	for _, span := range payload.WindowStats.TraceSpans {
		states := span.SchedulerStates
		if states == nil || states.Coverage != "complete" || states.Window.StartTs != 1.01 || states.Window.EndTs != 1.02 ||
			math.Abs(states.SleepMs-10) > 1e-6 || states.RunningMs != 0 || states.RunnableMs != 0 || span.ActualDurationMs <= span.DurationMs {
			t.Errorf("clipped marker borrowed full or broader-query occupancy: %+v", span)
		}
	}
}

func TestTraceQueryBusinessSpanSchedulerPublicNestedOwnershipAndAsync(t *testing.T) {
	ctx, path := businessRefTestContext(t, `# tracer: nop
task-700 (600) [003] .... 8.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=700 next_prio=120
task-700 (600) [003] .... 8.001000: tracing_mark_write: B|600|UncataloguedOuter
task-700 (600) [003] .... 8.002000: tracing_mark_write: S|600|UncataloguedAsync|41
task-700 (600) [003] .... 8.003000: tracing_mark_write: B|600|UncataloguedInner
task-700 (600) [003] .... 8.004000: sched_switch: prev_comm=task prev_pid=700 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
irq-80 (2) [003] .... 8.008000: sched_wakeup: comm=task pid=700 prio=120 target_cpu=003
task-700 (600) [003] .... 8.009000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=700 next_prio=120
task-700 (600) [003] .... 8.010000: tracing_mark_write: E|600
task-700 (600) [003] .... 8.011000: tracing_mark_write: F|600|UncataloguedAsync|41
task-700 (600) [003] .... 8.012000: tracing_mark_write: E|600
task-700 (600) [003] .... 8.013000: sched_switch: prev_comm=task prev_pid=700 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`)
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "time_start": 8, "time_end": 8.013})
	payload := businessSpanSchedulerPublicPayload(t, result)
	if payload.WindowStats == nil || len(payload.WindowStats.TraceSpans) != 3 {
		t.Fatalf("lost nested or async facts: %+v", payload.WindowStats)
	}
	for _, span := range payload.WindowStats.TraceSpans {
		if span.Kind == "async" {
			if span.SchedulerStates != nil {
				t.Fatal("async ownership was equated with synchronous thread occupancy")
			}
			continue
		}
		wantRunning := 6.0
		if span.Name == "UncataloguedInner" {
			wantRunning = 2
		}
		states := span.SchedulerStates
		if states == nil || states.Thread.PID != 700 || states.Coverage != "complete" ||
			math.Abs(states.RunningMs-wantRunning) > 1e-6 || math.Abs(states.RunnableMs-1) > 1e-6 || math.Abs(states.SleepMs-4) > 1e-6 {
			t.Errorf("nested marker lost its own interval or borrowed payload PID: %+v", span)
		}
	}
	for _, record := range result.Observations {
		if record.Predicate == types.TraceBusinessSpanPredicate && record.Object == "UncataloguedAsync" &&
			strings.Contains(strings.Join(record.RichNotes, "\n"), "business_span_scheduler_states=") {
			t.Fatal("async observation acquired an owner-thread partition")
		}
	}
}

func TestTraceQueryBusinessSpanSchedulerPublicMissingCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, trace, coverage string
		running, runnable     float64
	}{
		{"partial", `# tracer: nop
task-700 (600) [003] .... 9.000000: tracing_mark_write: B|600|OtherWork
irq-80 (2) [003] .... 9.005000: sched_wakeup: comm=task pid=700 prio=120 target_cpu=003
task-700 (600) [003] .... 9.006000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=task next_pid=700 next_prio=120
task-700 (600) [003] .... 9.010000: tracing_mark_write: E|600
`, "partial", 4, 1},
		{"unavailable", `# tracer: nop
task-700 (600) [003] .... 9.000000: tracing_mark_write: B|600|OtherWork
task-700 (600) [003] .... 9.010000: tracing_mark_write: E|600
`, "unavailable", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, path := businessRefTestContext(t, tc.trace)
			result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "time_start": 9, "time_end": 9.01})
			payload := businessSpanSchedulerPublicPayload(t, result)
			if payload.WindowStats == nil || len(payload.WindowStats.TraceSpans) != 1 {
				t.Fatalf("missing marker: %+v", payload.WindowStats)
			}
			states := payload.WindowStats.TraceSpans[0].SchedulerStates
			if states == nil || states.Coverage != tc.coverage || math.Abs(states.RunningMs-tc.running) > 1e-6 || math.Abs(states.RunnableMs-tc.runnable) > 1e-6 {
				t.Fatalf("unknown time silently became a complete/zero state account: %+v", states)
			}
		})
	}
}
