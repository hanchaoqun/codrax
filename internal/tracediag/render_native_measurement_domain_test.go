package tracediag

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestB1638B2DiagActualNativeStreams(t *testing.T) {
	path := filepath.Join(t.TempDir(), "native.ftrace")
	const body = `idle-0 (0) [000] .... 6793224.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=55 next_prio=120
worker-55 (55) [000] .... 6793224.010000: sched_switch: prev_comm=worker prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
idle-0 (0) [000] .... 6793224.020000: sched_wakeup: comm=worker pid=55 prio=120 target_cpu=0
idle-0 (0) [000] .... 6793224.030000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=55 next_prio=120
worker-55 (55) [000] .... 6793224.040000: sched_switch: prev_comm=worker prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
idle-0 (0) [000] .... 6793224.050000: sched_wakeup: comm=worker pid=55 prio=120 target_cpu=0
idle-0 (0) [000] .... 6793224.060000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=55 next_prio=120
worker-55 (55) [000] .... 6793224.070000: sched_switch: prev_comm=worker prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"window_stats", "root_cause_rank", "frame_root_cause_bundle"} {
		t.Run(view, func(t *testing.T) {
			result := tracequery.Run(idx, tracequery.Query{View: view, PID: 55, TimeStart: 6793224, TimeEnd: 6793224.07})
			if result.WindowStats == nil || len(result.WindowStats.TopRunning) != 1 || len(result.WindowStats.StateChurn) != 1 {
				t.Fatal("fixture must construct native Running, OffCPU and eligible churn")
			}
			stats := result.WindowStats
			want := map[string]string{
				"window_stats.top_running[0].measurement_domain:":  "cpu_running_sweep",
				"window_stats.runnable_top[0].measurement_domain:": "off_cpu_sweep",
				"window_stats.sleep_top[0].measurement_domain:":    "off_cpu_sweep",
				"window_stats.state_churn[0].measurement_domain:":  "state_churn_sweep",
			}
			if stats.TopRunning[0].MeasurementDomain == nil || stats.StateChurn[0].MeasurementDomain == nil {
				t.Fatal("native receipts absent")
			}
			before, _ := json.Marshal(result)
			for _, policy := range []*detailRenderPolicy{nil, &nonEventDetailPolicy} {
				var lines []string
				renderResultDetailWithPolicy(&result, func(line string) { lines = append(lines, line) }, policy)
				for prefix, method := range want {
					found := 0
					for _, line := range lines {
						if !strings.Contains(line, prefix) {
							continue
						}
						found++
						for _, token := range []string{"version=1", "status=constructed_partition", "method=" + method, "target_tid=55", "window_start_ts=6793224.000000", "window_end_ts=6793224.070000", "query_line_start=0", "query_line_end=0", "partition_id=scheduler_partition:v1:"} {
							if !strings.Contains(line, token) {
								t.Errorf("missing %s: %s", token, line)
							}
						}
						if strings.Contains(line, "e+") || strings.Contains(line, "status=complete") {
							t.Errorf("wrong coordinate or completeness: %s", line)
						}
					}
					if found != 1 {
						t.Errorf("want one %s, got %d", prefix, found)
					}
				}
			}
			after, _ := json.Marshal(result)
			if !bytes.Equal(before, after) {
				t.Fatal("rendering changed native facts")
			}
			stats.TopRunning[0].MeasurementDomain = nil
			stats.StateChurn[0].MeasurementDomain = nil
			var legacy []string
			renderResultDetailWithPolicy(&result, func(line string) { legacy = append(legacy, line) }, nil)
			if strings.Contains(strings.Join(legacy, "\n"), "window_stats.top_running[0].measurement_domain:") || strings.Contains(strings.Join(legacy, "\n"), "window_stats.state_churn[0].measurement_domain:") {
				t.Fatal("legacy native rows acquired invented receipts")
			}
		})
	}
}
