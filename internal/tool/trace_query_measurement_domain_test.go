package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// b1 deliberately publishes provenance in the saved result only. It does not
// silently grant measurement equivalence to nodes or change model guidance.
func TestB1638B1ActualQueryPayloadPreservesMeasurementDomain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partition.ftrace")
	const body = `<idle>-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=55 next_prio=120
worker-55 (55) [000] .... 1.010000: sched_switch: prev_comm=worker prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
<idle>-0 (-----) [000] .... 1.020000: sched_wakeup: comm=worker pid=55 prio=120 target_cpu=0
<idle>-0 (-----) [000] .... 1.030000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=55 next_prio=120
worker-55 (55) [000] .... 1.040000: sched_switch: prev_comm=worker prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	// The real source resolver canonicalizes macOS /var -> /private/var.
	// Compare the exact canonical locator, not a test-directory alias.
	var err error
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	var mainDomain *types.TraceSchedulerMeasurementDomain
	for _, tc := range []struct {
		view    string
		lineEnd int
	}{{"thread_timeline", 0}, {"window_stats", 0}, {"root_cause_rank", 0}, {"frame_root_cause_bundle", 0}, {"root_cause_rank", 2}} {
		t.Run(tc.view+"/"+strconv.Itoa(tc.lineEnd), func(t *testing.T) {
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": tc.view, "pid": 55, "time_start": 1, "time_end": 1.04, "line_end": tc.lineEnd})
			result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !result.Success {
				t.Fatalf("actual query failed: %v %+v", err, result)
			}
			var source types.ObservationSourceRef
			for _, row := range result.Observations {
				if row.Predicate == "target_window_states" {
					source = row.SourceRef
					break
				}
			}
			if source.PayloadRef == "" || source.Path != path || source.QueryScopeID == "" {
				t.Fatalf("receipt must remain bound to the actual capture and published result: source=%+v path=%s", source, path)
			}
			payload, err := os.ReadFile(source.PayloadRef)
			if err != nil {
				t.Fatal(err)
			}
			var full tracequery.Result
			if err := json.Unmarshal(payload, &full); err != nil {
				t.Fatal(err)
			}
			account := full.TargetWindowStates
			if full.FrameRootCauseBundle != nil {
				account = full.FrameRootCauseBundle.TargetWindowStates
			}
			if account == nil || account.MeasurementDomain == nil {
				t.Fatal("saved JSON lost native measurement provenance")
			}
			domain := account.MeasurementDomain
			if domain.Version != 1 || domain.Status != "constructed_partition" || domain.Method != "thread_timeline" || domain.TargetTID != 55 ||
				domain.WindowStartTs != 1 || domain.WindowEndTs != 1.04 || domain.QueryLineEnd != tc.lineEnd || domain.PartitionID == "" {
				t.Fatalf("wrong measurement boundary: %+v", domain)
			}
			if tc.lineEnd == 0 {
				if mainDomain == nil {
					mainDomain = types.CloneTraceSchedulerMeasurementDomain(domain)
				} else if !reflect.DeepEqual(mainDomain, domain) {
					t.Fatal("same constructed partition must retain its identity across views")
				}
			} else if mainDomain == nil || mainDomain.PartitionID == domain.PartitionID {
				t.Fatal("the line-filtered measurement must not borrow the main partition")
			}
			q := tracequery.Query{View: tc.view, PID: 55, TimeStart: 1, TimeEnd: 1.04, LineEnd: tc.lineEnd}
			with := traceQueryTypedObservations(full, path, source.PayloadRef, source.RawRef, "", time.Unix(1, 0), q)
			account.MeasurementDomain = nil
			if full.Timeline != nil {
				full.Timeline.MeasurementDomain = nil
			}
			without := traceQueryTypedObservations(full, path, source.PayloadRef, source.RawRef, "", time.Unix(1, 0), q)
			// EVOLUTION RECORD B1638b2b: native receipts now deliberately
			// propagate through MeasurementSources. Exclude only this new
			// metadata; all earlier value, evidence, authority and guidance
			// fields must still remain identical to the B1 publication.
			for i := range with {
				with[i].MeasurementSources = nil
			}
			for i := range without {
				without[i].MeasurementSources = nil
			}
			if !reflect.DeepEqual(with, without) || strings.Contains(result.Summary, "constructed_partition") {
				t.Fatal("b1 must not change observations, priorities, or inject an opaque receipt into model teaching")
			}
		})
	}
}
