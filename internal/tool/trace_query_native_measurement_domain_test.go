package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1638B2ActualQuerySavesNativeStreamsWithoutChangingModelContract(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "native-streams.ftrace")
	var trace strings.Builder
	for i := 0; i < 9; i++ {
		start := 1 + float64(i)*.03
		state := "S"
		if i%3 != 0 {
			state = "D"
		}
		fmt.Fprintf(&trace, "idle-0 (0) [000] .... %.6f: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=55 next_prio=120\n", start)
		fmt.Fprintf(&trace, "worker-55 (55) [000] .... %.6f: sched_switch: prev_comm=worker prev_pid=55 prev_prio=120 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120\n", start+.01, state)
		if state == "D" {
			fmt.Fprintf(&trace, "idle-0 (0) [000] .... %.6f: sched_blocked_reason: pid=55 iowait=%d caller=io_schedule\n", start+.015, i%3-1)
		}
		fmt.Fprintf(&trace, "idle-0 (0) [000] .... %.6f: sched_wakeup: comm=worker pid=55 prio=120 target_cpu=0\n", start+.02)
	}
	fmt.Fprintln(&trace, "idle-0 (0) [000] .... 1.270000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=55 next_prio=120")
	if err := os.WriteFile(path, []byte(trace.String()), 0600); err != nil {
		t.Fatal(err)
	}
	var err error
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	var reference map[string]string
	for _, view := range []string{"window_stats", "root_cause_rank", "frame_root_cause_bundle"} {
		t.Run(view, func(t *testing.T) {
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": 55, "time_start": 1, "time_end": 1.27})
			published, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !published.Success {
				t.Fatalf("actual query failed: %v %+v", err, published)
			}
			var source types.ObservationSourceRef
			for _, row := range published.Observations {
				if row.Predicate == "target_window_states" {
					source = row.SourceRef
					break
				}
			}
			if source.Path != path || source.PayloadRef == "" || source.QueryScopeID == "" {
				t.Fatalf("saved native streams lost enclosing source/receipt: %+v", source)
			}
			payload, err := os.ReadFile(source.PayloadRef)
			if err != nil {
				t.Fatal(err)
			}
			var full tracequery.Result
			if err := json.Unmarshal(payload, &full); err != nil || full.WindowStats == nil {
				t.Fatalf("saved JSON lacks native window statistics: %v", err)
			}
			stats := full.WindowStats
			seen := map[string]string{}
			for _, lane := range []struct {
				name, method string
				rows         []tracequery.ThreadDuration
			}{{"running", "cpu_running_sweep", stats.TopRunning}, {"runnable", "off_cpu_sweep", stats.RunnableTop},
				{"sleep", "off_cpu_sweep", stats.SleepTop}, {"D", "off_cpu_sweep", stats.DStateTop}, {"IO", "off_cpu_sweep", stats.IOWaitTop}} {
				for _, row := range lane.rows {
					if row.Thread.PID != 55 {
						continue
					}
					domain := row.MeasurementDomain
					if row.DurationMs <= 0 || domain == nil || domain.Method != lane.method || domain.TargetTID != 55 ||
						domain.Status != "constructed_partition" || domain.WindowStartTs != 1 || domain.WindowEndTs != 1.27 || domain.PartitionID == "" {
						t.Fatalf("%s lost native value/source: %+v", lane.name, row)
					}
					seen[lane.name] = domain.PartitionID
				}
			}
			for _, row := range stats.StateChurn {
				if row.Thread.PID == 55 && row.TotalMs > 0 && row.FragmentCount > 8 && row.MeasurementDomain != nil && row.MeasurementDomain.Method == "state_churn_sweep" {
					seen["churn"] = row.MeasurementDomain.PartitionID
				}
			}
			if len(seen) != 6 || seen["runnable"] != seen["sleep"] || seen["sleep"] != seen["D"] || seen["D"] != seen["IO"] ||
				seen["running"] == seen["IO"] || seen["churn"] == seen["IO"] || seen["churn"] == seen["running"] {
				t.Fatalf("native methods must stay distinct; four OffCPU buckets share only their source: %+v", seen)
			}
			if reference == nil {
				reference = seen
			} else if !reflect.DeepEqual(reference, seen) {
				t.Fatalf("view selection changed a native stream: got=%v want=%v", seen, reference)
			}
			q := tracequery.Query{View: view, PID: 55, TimeStart: 1, TimeEnd: 1.27}
			with := traceQueryTypedObservations(full, path, source.PayloadRef, source.RawRef, "", time.Unix(1, 0), q)
			// Remove only the new metadata from a saved-payload round trip. The
			// existing observation compiler must remain byte/structure invariant.
			var wire any
			if err := json.Unmarshal(payload, &wire); err != nil {
				t.Fatal(err)
			}
			b1638b2RemoveMeasurementMetadata(wire)
			legacyJSON, _ := json.Marshal(wire)
			var legacy tracequery.Result
			if err := json.Unmarshal(legacyJSON, &legacy); err != nil {
				t.Fatal(err)
			}
			without := traceQueryTypedObservations(legacy, path, source.PayloadRef, source.RawRef, "", time.Unix(1, 0), q)
			if !reflect.DeepEqual(with, without) || strings.Contains(published.Summary, "constructed_partition") || strings.Contains(published.Summary, "scheduler_partition:v1:") {
				t.Fatal("native metadata changed model guidance, observations or causal permissions")
			}
		})
	}
}

func b1638b2RemoveMeasurementMetadata(value any) {
	switch v := value.(type) {
	case map[string]any:
		delete(v, "measurement_domain")
		for _, child := range v {
			b1638b2RemoveMeasurementMetadata(child)
		}
	case []any:
		for _, child := range v {
			b1638b2RemoveMeasurementMetadata(child)
		}
	}
}
