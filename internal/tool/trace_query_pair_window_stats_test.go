package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPairWindowStatsReusesCurrentNativeResultAcrossDispatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.ftrace")
	const trace = `<idle>-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=55 next_prio=120
worker-55 (55) [000] .... 1.010000: tracing_mark_write: B|55|work
worker-55 (55) [000] .... 1.020000: tracing_mark_write: E|55
worker-55 (55) [000] .... 1.030000: sched_switch: prev_comm=worker prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
<idle>-0 (-----) [000] .... 1.040000: sched_wakeup: comm=worker pid=55 prio=120 target_cpu=0
`
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("compare")}
	query := func(end float64) types.ToolResult {
		t.Helper()
		raw, err := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats",
			"pid": 55, "time_start": 1, "time_end": end})
		if err != nil {
			t.Fatal(err)
		}
		result, err := (&TraceQuery{pairWindowStatsSelector: "baseline"}).Execute(bus, raw)
		if err != nil || !result.Success || len(result.Observations) == 0 || result.ReusedFromRunMemo {
			t.Fatalf("expected a native query result: %v %s", err, result.Summary)
		}
		return result
	}
	first := query(1.03)
	bus.Mutable.ResetDispatchToolResults()
	if again := query(1.03); !first.Timestamp.Equal(again.Timestamp) {
		t.Fatal("identical window did not reuse the original native result after a dispatch reset")
	}
	if changed := query(1.04); first.Timestamp.Equal(changed.Timestamp) {
		t.Fatal("a different window reused the original native result")
	}
	bus.Mutable.ResetTurnAArtifacts()
	if fresh := query(1.03); first.Timestamp.Equal(fresh.Timestamp) {
		t.Fatal("a new investigation reused a retired native result")
	}
}
