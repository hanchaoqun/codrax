package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This is the public preparation/query -> current-source compiler -> common
// heading path, not a table assembled by the test or inferred from its notes.
func TestHMC321NativeIntervalEndpointsPublic(t *testing.T) {
	for _, tc := range []struct {
		name, fixture, body string
		start, end          float64
		pid                 int
		counts              map[string]int
	}{
		{name: "io", fixture: "hmosperf_io_inflight", start: 1, end: 1.01, counts: map[string]int{"io_inflight": 12}},
		{name: "business", fixture: "hmosperf_business_io_chain", start: .999, end: 1.051, counts: map[string]int{types.TraceBusinessSpanPredicate: 2}},
		{name: "timelines", start: 1, end: 2, pid: 41, counts: map[string]int{TraceThreadTimelinePredicate: 1, TraceCPUFrequencyTimelinePredicate: 1}, body: `# tracer: nop
idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1000000 cpu_id=0
idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=41 next_prio=120
worker-41 (41) [000] .... 2.000000: sched_switch: prev_comm=worker prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
idle-0 (0) [000] .... 2.100000: cpu_frequency: state=2000000 cpu_id=0
`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			if tc.fixture != "" {
				var err error
				body, err = os.ReadFile(filepath.Join("../../eval/fixtures", tc.fixture, "events.systrace"))
				if err != nil {
					t.Fatal(err)
				}
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "capture.systrace")
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("native interval facts")}
			args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": tc.pid, "time_start": tc.start, "time_end": tc.end})
			result, err := (&TraceQuery{}).Execute(ctx, args)
			if err != nil || !result.Success {
				t.Fatalf("public query failed: %v / %s", err, result.Summary)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			before, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
			contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(ctx, 0))
			seen := map[string]int{}
			for _, record := range result.Observations {
				if _, wanted := tc.counts[record.Predicate]; !wanted {
					continue
				}
				publication, ok := types.DecodeRuntimeMeasurementPublication(record)
				if !ok {
					t.Fatal("native producer did not publish its complete tables")
				}
				for _, table := range contract.Choices() {
					if table.ObservationID != record.ID {
						continue
					}
					seen[record.Predicate]++
					assertNativeWindowTitleHMC290(t, table, fmt.Sprintf("[%g,%g) s", tc.start, tc.end))
					if table.HasNativeRawFacts() || !table.HasNativeMeasurementFacts() {
						t.Fatal("endpoint display changed fixed/raw publication authority")
					}
					for _, native := range publication.Tables {
						if table.View == native.View && (!reflect.DeepEqual(table.Rows, native.Rows) || !reflect.DeepEqual(table.Columns, native.Columns)) {
							t.Fatal("common heading changed native values or member populations")
						}
					}
				}
			}
			if !reflect.DeepEqual(seen, tc.counts) {
				t.Fatalf("missing complete original publication: got %v want %v", seen, tc.counts)
			}
			after, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
			if string(before) != string(after) {
				t.Fatal("display compilation changed original native evidence")
			}
		})
	}
}
