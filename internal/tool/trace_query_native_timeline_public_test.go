package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func nativeTimelinePublicQuery(t *testing.T, fixture, view string, start, end float64) (types.ToolResult, tracequery.Result) {
	t.Helper()
	path, err := filepath.Abs("../../eval/fixtures/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": 41, "time_start": start, "time_end": end, "include_window_stats": true})
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("public native query: %v / %s", err, result.Summary)
	}
	for _, r := range result.Observations {
		if r.SourceRef.PayloadRef == "" {
			continue
		}
		data, err := os.ReadFile(r.SourceRef.PayloadRef)
		var native tracequery.Result
		if err == nil && json.Unmarshal(data, &native) == nil {
			return result, native
		}
	}
	t.Fatal("native result payload absent")
	return result, tracequery.Result{}
}

func nativeTimelinePublicTable(t *testing.T, result types.ToolResult, predicate string) types.RuntimeMeasurementTable {
	t.Helper()
	for _, r := range result.Observations {
		if r.Predicate != predicate {
			continue
		}
		publication, ok := types.DecodeRuntimeMeasurementPublication(r)
		if !ok || len(publication.Tables) != 1 || publication.Tables[0].View != types.RuntimeMeasurementTimeline {
			t.Fatalf("invalid native timeline publication: %+v", r)
		}
		if !publication.Tables[0].DefaultPresentation || publication.Tables[0].MemberSet != nil || r.ProvenanceLane != types.ObservationProvenanceArtifactSpan || r.Role != types.AnswerAggregateRoleSupportingCoverage {
			t.Fatal("native display changed completeness/causal authority")
		}
		return publication.Tables[0]
	}
	t.Fatalf("native timeline publication %q absent", predicate)
	return types.RuntimeMeasurementTable{}
}

func TestNativeTimelinePublicThreadFactsSeparateWakeAndRunning(t *testing.T) {
	for _, tc := range []struct {
		fixture               string
		start, end, wake, run float64
		queue                 string
	}{
		{"hmosperf_dual_source_windows/baseline.systrace", 1, 1.01, 1.002, 1.006, "4"},
		{"hmosperf_dual_source_windows/current.systrace", 4, 4.02, 4.004, 4.012, "8"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			result, native := nativeTimelinePublicQuery(t, tc.fixture, "thread_timeline", tc.start, tc.end)
			table := nativeTimelinePublicTable(t, result, "thread_timeline_observation")
			if native.Timeline == nil || len(table.Rows) != len(native.Timeline.Intervals) {
				t.Fatal("native intervals lost or reconstructed")
			}
			queue, running := false, false
			for _, row := range table.Rows {
				if row[1] == "可运行" && row[2] == traceQueryDisplaySeconds(tc.wake) && row[3] == traceQueryDisplaySeconds(tc.run) && row[4] == tc.queue {
					queue = true
				}
				if row[1] == "运行" && row[2] == traceQueryDisplaySeconds(tc.run) && row[5] == "CPU0" {
					running = true
				}
			}
			if !queue || !running {
				t.Fatalf("wake point and actual sched-in not independently represented: %+v", table.Rows)
			}
			last := table.Rows[len(table.Rows)-1]
			if tc.start == 1 && (last[7] != "3.999000" || last[9] != "16") {
				t.Fatalf("out-of-window closing witness lost: %+v", last)
			}
			if tc.start == 4 && (last[7] != "未知" || last[9] != "未知") {
				t.Fatalf("open tail invented a closing event: %+v", last)
			}
		})
	}
}

func TestNativeTimelinePublicWakeupQueryRetainsAllFrequencySegments(t *testing.T) {
	for _, tc := range []struct {
		fixture    string
		start, end float64
		want       [][]string
	}{
		{"hmosperf_dual_source_windows/baseline.systrace", 1, 1.01, [][]string{{"CPU0", "1.000000", "1.005000", "1000000", "5", "2", "8"}, {"CPU0", "1.005000", "1.010000", "2000000", "5", "8", "8"}}},
		{"hmosperf_dual_source_windows/current.systrace", 4, 4.02, [][]string{{"CPU0", "4.000000", "4.010000", "1500000", "10", "12", "18"}, {"CPU0", "4.010000", "4.020000", "750000", "10", "18", "18"}}},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			result, native := nativeTimelinePublicQuery(t, tc.fixture, "wakeup_chain", tc.start, tc.end)
			if native.WindowStats == nil || len(native.WindowStats.CPU) != 1 || len(native.WindowStats.CPU[0].FrequencyResidency) != 2 {
				t.Fatalf("fixture lacks independent native frequency spans: %+v", native.WindowStats)
			}
			table := nativeTimelinePublicTable(t, result, "cpu_frequency_timeline_observation")
			if !reflect.DeepEqual(table.Rows, tc.want) {
				t.Fatalf("frequency transition or source window lost: got %+v want %+v", table.Rows, tc.want)
			}
		})
	}
}

func TestNativeTimelinePublicBlockedReasonDoesNotCloseOpenWait(t *testing.T) {
	for _, ioWait := range []int{0, 1} {
		t.Run(fmt.Sprint(ioWait), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "open-wait.systrace")
			body := fmt.Sprintf("idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=41 next_prio=120\nworker-41 (41) [000] .... 1.001000: sched_switch: prev_comm=worker prev_pid=41 prev_prio=120 prev_state=D ==> next_comm=idle next_pid=0 next_prio=120\nirq-2 (2) [000] .... 1.001100: sched_blocked_reason: pid=41 iowait=%d caller=io_schedule\nidle-0 (0) [000] .... 1.020000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120\n", ioWait)
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, _, _ := hmc17NamedPathContext(t)
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "thread_timeline", "pid": 41, "time_start": 1, "time_end": 1.01})
			result, err := (&TraceQuery{}).Execute(ctx, params)
			if err != nil || !result.Success {
				t.Fatalf("public open-wait query: %v %s", err, result.Summary)
			}
			table := nativeTimelinePublicTable(t, result, "thread_timeline_observation")
			last := table.Rows[len(table.Rows)-1]
			state := "D态等待"
			if ioWait == 1 {
				state = "I/O等待"
			}
			if last[1] != state || last[4] != "9" || last[7] != "未知" || last[9] != "3" || last[11] != "io_schedule（行3）" {
				t.Fatalf("blocked-reason marker became a wait completion: %+v", last)
			}
		})
	}
}
