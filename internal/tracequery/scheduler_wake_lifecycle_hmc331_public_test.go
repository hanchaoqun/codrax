package tracequery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hmc331WriteTransitionSource(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "transition.ftrace")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHMC331WakeBoundaryActuallyPrunedPrefixRetainsSameMeasure(t *testing.T) {
	for _, state := range []string{"S", "D"} {
		t.Run(state, func(t *testing.T) {
			path := hmc331WriteTransitionSource(t, fmt.Sprintf(`idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120
target-41 (41) [000] .... 1.010000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120
waker-2 (2) [001] .... 1.012000: sched_wakeup: comm=target pid=41 prio=120 target_cpu=000
observer-3 (3) [001] .... 1.014000: tracing_mark_write: C|3|observation|1
waker-2 (2) [001] .... 1.016000: sched_wakeup: comm=target pid=41 prio=120 target_cpu=000
observer-3 (3) [001] .... 1.018000: tracing_mark_write: C|3|observation|2
idle-0 (0) [000] .... 1.020000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120
target-41 (41) [000] .... 1.040000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`, state))
			full, err := BuildIndex(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			const start, end = 1.014, 1.018
			cut, err := BuildIndexWithOptions(t.Context(), path, BuildOptions{
				AllowWindowedParse: true, TimeStart: start, TimeEnd: end, TimeStartSet: true, TimeEndSet: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !cut.Windowed || cut.IndexTimeStart != start || cut.IndexTimeEnd != end || len(cut.Events) >= len(full.Events) {
				t.Fatalf("fixture must genuinely remove prefix/suffix: cut=%d full=%d bounds=%g..%g", len(cut.Events), len(full.Events), cut.IndexTimeStart, cut.IndexTimeEnd)
			}
			for _, event := range cut.Events {
				if event.Ts < start || event.Ts > end {
					t.Fatalf("prefix/suffix row survived strict public index: %+v", event)
				}
			}
			q := hmc331WakeBoundaryQuery("thread_timeline", start, end)
			fullResult, cutResult := Run(full, q), Run(cut, q)
			if cutResult.Timeline == nil || cutResult.Timeline.HeadState == nil || cutResult.Timeline.HeadState.Status != "recovered" {
				t.Fatalf("real clipped index did not recover its producer head: %+v", cutResult.Timeline)
			}
			fullBuckets := hmc331WakeBoundaryProjection(t, fullResult, start, end)
			cutBuckets := hmc331WakeBoundaryProjection(t, cutResult, start, end)
			hmc331WakeBoundaryNear(t, "full replay ready suffix", fullBuckets[StateRunnable], 4)
			hmc331WakeBoundaryNear(t, "strict prefix recovery ready suffix", cutBuckets[StateRunnable], 4)
			for _, bucket := range []ThreadState{StateRunning, StateRunnable, StateSSleep, StateDSleep, StateIOWait} {
				hmc331WakeBoundaryNear(t, "strict prefix recovery preserves "+string(bucket), fullBuckets[bucket], cutBuckets[bucket])
			}
		})
	}
}

func TestHMC331WakeBoundaryAcceptedKindsKeepExistingMeasurePolicy(t *testing.T) {
	for _, kinds := range [][2]string{{"sched_waking", "sched_wakeup"}, {"sched_wakeup", "sched_waking"}, {"sched_waking", "sched_waking"}} {
		t.Run(strings.Join(kinds[:], "_then_"), func(t *testing.T) {
			path := hmc331WakeBoundarySource(t, "S", 1, hmc331WakeBoundaryCases[5])
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			parts := strings.Split(string(body), "sched_wakeup:")
			if len(parts) != 3 {
				t.Fatal("fixture needs exactly two accepted wake rows")
			}
			path = hmc331WriteTransitionSource(t, parts[0]+kinds[0]+":"+parts[1]+kinds[1]+":"+parts[2])
			idx, err := BuildIndex(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			r := Run(idx, hmc331WakeBoundaryQuery("window_stats", 1, 1.04))
			if r.WindowStats == nil || r.TargetWindowStates == nil {
				t.Fatal("native state/load fixture missing")
			}
			var legacy float64
			for _, row := range r.WindowStats.ThreadCPULoad {
				if row.Thread.PID == 41 {
					legacy += row.RunnableWaitMs
				}
			}
			// This pins the existing measured-state admission; it does not
			// equate a waking attempt with a new causal completion certificate.
			hmc331WakeBoundaryNear(t, "existing accepted-kind measure", legacy, 8)
			hmc331WakeBoundaryNear(t, "native state uses same admission", r.TargetWindowStates.RunnableMs, legacy)
			hmc331WakeBoundaryNear(t, "native first state end", r.TargetWindowStates.SleepMs, 2)
		})
	}
}

func TestHMC331WakeBoundaryNewIncarnationKeepsOldTailButNotOldAuthority(t *testing.T) {
	for _, state := range []string{"S", "D"} {
		t.Run(state, func(t *testing.T) {
			path := hmc331WriteTransitionSource(t, fmt.Sprintf(`idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=41 next_prio=120
old-41 (41) [000] .... 1.010000: sched_switch: prev_comm=old prev_pid=41 prev_prio=120 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120
waker-2 (2) [001] .... 1.012000: sched_wakeup: comm=old pid=41 prio=120 target_cpu=000
creator-9 (9) [001] .... 1.030000: sched_wakeup_new: comm=new pid=41 prio=100 target_cpu=000
idle-0 (0) [000] .... 1.032000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=new next_pid=41 next_prio=100
new-41 (91) [000] .... 1.040000: sched_switch: prev_comm=new prev_pid=41 prev_prio=100 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`, state))
			idx, err := BuildIndex(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			old := Run(idx, hmc331WakeBoundaryQuery("thread_timeline", 1, 1.02))
			if old.TargetWindowStates == nil {
				t.Fatalf("old-only query must not be poisoned by a later reset: %+v", old.Timeline)
			}
			hmc331WakeBoundaryNear(t, "old ready tail before future reset", old.TargetWindowStates.RunnableMs, 8)
			hmc331WakeBoundaryNear(t, "old positive wait before future reset", old.TargetWindowStates.SleepMs+old.TargetWindowStates.DStateMs, 2)
			fresh := Run(idx, hmc331WakeBoundaryQuery("thread_timeline", 1.03, 1.04))
			if fresh.TargetWindowStates == nil {
				t.Fatalf("fresh generation must remain independently queryable: %+v", fresh.Timeline)
			}
			hmc331WakeBoundaryNear(t, "new generation starts ready", fresh.TargetWindowStates.RunnableMs, 2)
			hmc331WakeBoundaryNear(t, "new generation running", fresh.TargetWindowStates.RunningMs, 8)
			hmc331WakeBoundaryNear(t, "new generation cannot inherit old sleep", fresh.TargetWindowStates.SleepMs+fresh.TargetWindowStates.DStateMs, 0)
			for _, interval := range fresh.Timeline.Intervals {
				if interval.StartLine < 4 || interval.WakeupLine == 3 {
					t.Errorf("old generation provenance entered new account: %+v", interval)
				}
			}
			mixed := Run(idx, hmc331WakeBoundaryQuery("thread_timeline", 1, 1.04))
			if mixed.Timeline == nil || mixed.Timeline.IntegrityFailure != "thread_incarnation_conflict" || mixed.TargetWindowStates != nil || len(mixed.Timeline.Intervals) != 0 {
				t.Fatalf("mixed generations gained a unified measured account: %+v", mixed)
			}
			chain := Run(idx, hmc331WakeBoundaryQuery("wakeup_chain", 1, 1.04))
			if chain.WakeupChain != nil && len(chain.WakeupChain.Edges) != 0 {
				t.Fatalf("new incarnation became a closing wake for old identity: %+v", chain.WakeupChain.Edges)
			}
		})
	}
}
