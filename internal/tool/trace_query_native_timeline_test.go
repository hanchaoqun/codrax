package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func nativeTimelineTestResult() (tracequery.Result, types.ObservationSourceRef) {
	thread := tracequery.ThreadRef{PID: 41, Comm: "worker"}
	result := tracequery.Result{SourcePath: "/capture.trace", Timeline: &tracequery.TimelineResult{Thread: thread,
		Window: tracequery.TimeWindow{StartTs: 1, EndTs: 1.01}, Intervals: []tracequery.Interval{{Thread: thread, State: tracequery.StateIOWait,
			StartTs: 1, EndTs: 1.01, DurationMs: 10, ActualStartTs: .99, ActualEndTs: 1.02, ActualDurationMs: 30,
			StartLine: 4, EndLine: 16, WakeupLine: 14, BlockedReasonCaller: "io_schedule", BlockedReasonLine: 5}}},
		WindowStats: &tracequery.WindowStats{Window: tracequery.TimeWindow{StartTs: 1, EndTs: 1.01}, CPU: []tracequery.CPUStats{{CPU: 3,
			Frequency: 2000000, FrequencyResidency: []tracequery.CPUFrequencyResidency{{Frequency: 1000000, StartTs: 1.002, EndTs: 1.01, DurationMs: 8, LineStart: 6, LineEnd: 6}}}}}}
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: result.SourcePath, PayloadRef: "payload", QueryScopeID: "scope",
		QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 1.01, QueryTargetPID: 41}
	return result, ref
}

func TestNativeTimelineRejectsInvalidFamilyWithoutDiscardingIndependentFacts(t *testing.T) {
	for _, tc := range []struct {
		name              string
		mutate            func(*tracequery.Result, *types.ObservationSourceRef)
		thread, frequency bool
	}{
		{"native", func(*tracequery.Result, *types.ObservationSourceRef) {}, true, true},
		{"source_mismatch", func(_ *tracequery.Result, r *types.ObservationSourceRef) { r.Path += ".other" }, false, false},
		{"missing_payload", func(_ *tracequery.Result, r *types.ObservationSourceRef) { r.PayloadRef = "" }, false, false},
		{"unknown_window", func(_ *tracequery.Result, r *types.ObservationSourceRef) { r.QueryWindowKnown = false }, false, false},
		{"line_scope", func(_ *tracequery.Result, r *types.ObservationSourceRef) {
			r.QueryLineRangeKnown, r.QueryLineStart = true, 5
		}, false, false},
		{"wrong_thread_window", func(r *tracequery.Result, _ *types.ObservationSourceRef) { r.Timeline.Window.StartTs = .99 }, false, true},
		{"wrong_frequency_window", func(r *tracequery.Result, _ *types.ObservationSourceRef) { r.WindowStats.Window.EndTs = 2 }, true, false},
		{"wrong_target", func(r *tracequery.Result, _ *types.ObservationSourceRef) { r.Timeline.Thread.PID = 42 }, false, true},
		{"wrong_member", func(r *tracequery.Result, _ *types.ObservationSourceRef) { r.Timeline.Intervals[0].Thread.PID = 42 }, false, true},
		{"integrity_failure", func(r *tracequery.Result, _ *types.ObservationSourceRef) {
			r.Timeline.IntegrityFailure = "thread_incarnation_conflict"
		}, false, true},
		{"invalid_state", func(r *tracequery.Result, _ *types.ObservationSourceRef) {
			r.Timeline.Intervals[0].State = "probably-running"
		}, false, true},
		{"bad_duration", func(r *tracequery.Result, _ *types.ObservationSourceRef) { r.Timeline.Intervals[0].DurationMs = 100 }, false, true},
		{"invalid_hidden_tail", func(r *tracequery.Result, _ *types.ObservationSourceRef) {
			r.Timeline.Intervals = append(r.Timeline.Intervals, r.Timeline.Intervals[0])
		}, false, true},
		{"nan", func(r *tracequery.Result, _ *types.ObservationSourceRef) {
			r.Timeline.Intervals[0].ActualStartTs = math.NaN()
		}, false, true},
		{"inverted_actual", func(r *tracequery.Result, _ *types.ObservationSourceRef) {
			r.Timeline.Intervals[0].ActualEndTs = .98
		}, false, true},
		{"invalid_cpu_frequency", func(r *tracequery.Result, _ *types.ObservationSourceRef) {
			r.WindowStats.CPU[0].FrequencyResidency[0].Frequency = 0
		}, true, false},
		{"overlapping_frequency", func(r *tracequery.Result, _ *types.ObservationSourceRef) {
			r.WindowStats.CPU[0].FrequencyResidency = append(r.WindowStats.CPU[0].FrequencyResidency, r.WindowStats.CPU[0].FrequencyResidency[0])
		}, true, false},
		{"no_frequency_spans", func(r *tracequery.Result, _ *types.ObservationSourceRef) {
			r.WindowStats.CPU[0].FrequencyResidency = nil
		}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, ref := nativeTimelineTestResult()
			tc.mutate(&r, &ref)
			found := map[string]bool{}
			for _, row := range traceQueryNativeTimelineObservations(r, ref, "scope", "now") {
				if _, ok := types.DecodeRuntimeMeasurementPublication(row); !ok {
					t.Fatal("producer emitted an invalid publication")
				}
				found[row.Predicate] = true
			}
			if found[TraceThreadTimelinePredicate] != tc.thread || found[TraceCPUFrequencyTimelinePredicate] != tc.frequency {
				t.Fatalf("wrong family qualification: %+v", found)
			}
		})
	}
}

func TestNativeTimelineKeepsUnknownsBoundariesAndWaitLocation(t *testing.T) {
	r, ref := nativeTimelineTestResult()
	rows := traceQueryNativeTimelineObservations(r, ref, "scope", "now")
	thread, _ := types.DecodeRuntimeMeasurementPublication(rows[0])
	if got := thread.Tables[0].Rows[0]; got[1] != "I/O等待" || got[6] != "0.990000" || got[7] != "1.020000" || got[11] != "io_schedule（行5）" {
		t.Fatalf("wait or actual witness lost: %+v", got)
	}
	frequency, _ := types.DecodeRuntimeMeasurementPublication(rows[1])
	if got := frequency.Tables[0].Rows; len(got) != 1 || got[0][1] != "1.002000" || got[0][3] != "1000000" {
		t.Fatalf("unknown head backfilled from representative frequency: %+v", got)
	}
	r.Timeline.Intervals[0].State = tracequery.StateRunning
	r.Timeline.Intervals[0].EndLine = 0
	r.Timeline.HeadState = &tracequery.TimelineHeadState{Status: "unknown", BoundaryTs: 1}
	rows = traceQueryNativeTimelineObservations(r, ref, "scope", "now")
	thread, _ = types.DecodeRuntimeMeasurementPublication(rows[0])
	if got := thread.Tables[0].Rows[0]; got[5] != "未知" || got[7] != "未知" || got[9] != "未知" {
		t.Fatalf("CPU0 or closing event invented: %+v", got)
	}
	if !strings.Contains(strings.Join(thread.Tables[0].Notes, " "), "窗口开头状态未知") {
		t.Fatal("unknown head disclosure lost")
	}
	// Nonzero source lines alone cannot supply missing actual timestamps.
	r.Timeline.Intervals[0].ActualStartTs, r.Timeline.Intervals[0].ActualEndTs = 0, 0
	r.Timeline.Intervals[0].EndLine = 16
	rows = traceQueryNativeTimelineObservations(r, ref, "scope", "now")
	thread, _ = types.DecodeRuntimeMeasurementPublication(rows[0])
	if got := thread.Tables[0].Rows[0]; got[6] != "未知" || got[7] != "未知" {
		t.Fatalf("source lines manufactured actual boundary timestamps: %+v", got)
	}
}

func TestNativeTimelinePublicRetainsRowsBeyondPromptPreview(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 24; i++ {
		ts := 1 + float64(i)/1000
		fmt.Fprintf(&body, "idle-0 (0) [000] .... %.6f: cpu_frequency: state=%d cpu_id=0\n", ts, 1000000+i)
		prev, next, prevName, nextName := 0, 41, "idle", "worker"
		if i%2 == 1 {
			prev, next, prevName, nextName = 41, 0, "worker", "idle"
		}
		fmt.Fprintf(&body, "idle-0 (0) [000] .... %.6f: sched_switch: prev_comm=%s prev_pid=%d prev_prio=120 prev_state=R ==> next_comm=%s next_pid=%d next_prio=120\n", ts, prevName, prev, nextName, next)
	}
	body.WriteString("idle-0 (0) [000] .... 1.025000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120\n")
	path := filepath.Join(t.TempDir(), "many-intervals.systrace")
	if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 41, "time_start": 1, "time_end": 1.024})
	r, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !r.Success {
		t.Fatalf("public long query failed: %v %s", err, r.Summary)
	}
	for _, predicate := range []string{TraceThreadTimelinePredicate, TraceCPUFrequencyTimelinePredicate} {
		table := nativeTimelinePublicTable(t, r, predicate)
		if len(table.Rows) != 24 {
			t.Fatalf("%s table inherited a preview cap: %d", predicate, len(table.Rows))
		}
	}
}
