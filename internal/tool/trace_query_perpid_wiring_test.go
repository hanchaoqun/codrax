package tool

// WIRENOTE (P3-2, 2026-07-25) — the per-PID narrowing caveat wording
// (suppressed_pids roster) gets its tool-face wiring pin: the narrowing
// batch (c5923758e/fcc465c75) only pinned the engine lane, so deleting the
// caveat plumbing between engine Result and the model-visible summary would
// have left every test green (the RUNSPLIT-1 M4 lesson). Full chain: trace
// text → (&TraceQuery{}).Execute → summary bytes.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQuerySummaryCarriesPerPIDNarrowingCaveat(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "perpid_narrowing.systrace")
	trace := strings.Join([]string{
		`worker-100 ( 100) [000] .... 0.990000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=100 next_prio=20`,
		`old-900 ( 900) [001] .... 0.995000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=900 next_prio=20`,
		`creator-7 (   7) [001] .... 1.005000: sched_wakeup_new: comm=new pid=900 prio=20 target_cpu=001`,
		`worker-100 ( 100) [000] .... 1.010000: sched_switch: prev_comm=worker prev_pid=100 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
		`swapper-1 (   0) [001] .... 1.015000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=new next_pid=900 next_prio=20`,
		`new-900 ( 900) [001] .... 1.030000: sched_switch: prev_comm=new prev_pid=900 prev_prio=20 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`swapper-0 (   0) [000] .... 1.020000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=200 next_prio=20`,
		`other-200 ( 200) [000] .... 1.040000: sched_switch: prev_comm=other prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("perpid wiring")}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "perpid_narrowing.systrace",
		"view":       "window_stats",
		"pid":        100,
		"time_start": 1.0,
		"time_end":   1.05,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("window_stats failed: res=%+v err=%v", res, err)
	}
	for _, want := range []string{
		"thread_identity_per_pid_filtered=true",
		"suppressed_pids=[900]",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("per-pid narrowing caveat wording missing %q from the tool face:\n%s", want, res.Summary)
		}
	}
}
