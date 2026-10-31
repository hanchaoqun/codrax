package tracequery

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type hmc331WakeBoundaryCase struct {
	name       string
	wakeTimes  []float64
	wakeBefore bool
	waitMS     float64
}

// These are recorded transition-order contracts, not a claim that equal
// timestamps from different CPUs establish a positive physical sleep latency.
var hmc331WakeBoundaryCases = []hmc331WakeBoundaryCase{
	{"equal_after_out", []float64{1.010}, false, 0},
	{"equal_before_out", []float64{1.010}, true, 10},
	{"earlier_than_out", []float64{1.009}, true, 10},
	{"later_than_out", []float64{1.011}, false, 1},
	{"at_switch_in", []float64{1.020}, false, 10},
	{"repeat_after_first_transition", []float64{1.012, 1.016}, false, 2},
	{"equal_then_repeat", []float64{1.010, 1.016}, false, 0},
}

func hmc331WakeBoundarySource(t *testing.T, state string, wakeCPU int, tc hmc331WakeBoundaryCase) string {
	t.Helper()
	lines := []string{
		"idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120 next_info=1,0,0,0,0,0,0",
	}
	out := fmt.Sprintf("target-41 (41) [000] .... 1.010000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120", state)
	wakes := make([]string, 0, len(tc.wakeTimes))
	for _, ts := range tc.wakeTimes {
		wakes = append(wakes, fmt.Sprintf("waker-2 (2) [%03d] .... %.6f: sched_wakeup: comm=target pid=41 prio=120 target_cpu=000", wakeCPU, ts))
	}
	if tc.wakeBefore {
		lines = append(lines, wakes...)
		lines = append(lines, out)
	} else {
		lines = append(lines, out)
		lines = append(lines, wakes...)
	}
	lines = append(lines,
		"idle-0 (0) [000] .... 1.020000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120 next_info=1,0,0,0,0,0,0",
		"target-41 (41) [000] .... 1.040000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120",
	)
	path := filepath.Join(t.TempDir(), "recorded-boundary.ftrace")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func hmc331WakeBoundaryQuery(view string, start, end float64) Query {
	return Query{View: view, PID: 41, TargetScope: "thread", TimeStart: start, TimeEnd: end,
		TimeStartSet: true, TimeEndSet: true, Limit: 32, MinDurationMs: 0.001, MaxDepth: 4}
}

func hmc331WakeBoundaryNear(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s: got %.9f ms, want %.9f ms", label, got, want)
	}
}

func TestHMC331AcceptedWakeBoundaryStateConsumersAgree(t *testing.T) {
	for _, state := range []string{"S", "D"} {
		for _, wakeCPU := range []int{0, 1} {
			for _, tc := range hmc331WakeBoundaryCases {
				t.Run(fmt.Sprintf("%s/cpu%d/%s", state, wakeCPU, tc.name), func(t *testing.T) {
					path := hmc331WakeBoundarySource(t, state, wakeCPU, tc)
					idx, err := BuildIndex(t.Context(), path)
					if err != nil {
						t.Fatal(err)
					}
					r := Run(idx, hmc331WakeBoundaryQuery("window_stats", 1, 1.04))
					if r.WindowStats == nil || r.TargetWindowStates == nil || r.TargetWindowStates.Thread.PID != 41 {
						t.Fatalf("public native fixture lacks exact target state/statistics: %+v", r)
					}
					a := r.TargetWindowStates
					hmc331WakeBoundaryNear(t, "target total", a.TotalMs, 40)
					hmc331WakeBoundaryNear(t, "target running", a.RunningMs, 30)
					hmc331WakeBoundaryNear(t, "target runnable", a.RunnableMs, 10-tc.waitMS)
					wantS, wantD := 0.0, 0.0
					if state == "S" {
						wantS = tc.waitMS
					} else {
						wantD = tc.waitMS
					}
					hmc331WakeBoundaryNear(t, "target S", a.SleepMs, wantS)
					hmc331WakeBoundaryNear(t, "target unmarked D", a.DStateMs, wantD)
					hmc331WakeBoundaryNear(t, "no I/O marker may invent I/O", a.IOWaitMs, 0)
					var loadMS float64
					var loadRows, constraintRows int
					for _, row := range r.WindowStats.ThreadCPULoad {
						if row.Thread.PID == 41 {
							loadRows++
							loadMS += row.RunnableWaitMs
						}
					}
					if loadRows == 0 {
						t.Fatal("public fixture did not publish the existing thread CPU load face")
					}
					hmc331WakeBoundaryNear(t, "existing thread CPU load", loadMS, 10-tc.waitMS)
					hmc331WakeBoundaryNear(t, "same source/query state vs load", a.RunnableMs, loadMS)
					for _, row := range r.WindowStats.CPUConstraints {
						if row.Thread.PID == 41 {
							constraintRows++
							hmc331WakeBoundaryNear(t, "existing CPU constraint runnable context", row.RunnableWaitMs, 10-tc.waitMS)
						}
					}
					if constraintRows == 0 {
						t.Fatal("public fixture did not publish the next_info constraint context")
					}
					// A state transition at one timestamp is not a positive sleep
					// instance. The all-waits inventory remains measurement-only.
					inv := a.SleepInventory
					if inv == nil || inv.StateClosureStatus != "not_assessed" || inv.CausalAttributionStatus != "not_assessed" {
						t.Fatalf("measurement roster gained/lost closure authority: %+v", inv)
					}
					wantWaits := 1
					if tc.waitMS == 0 {
						wantWaits = 0
					}
					if inv.Total != wantWaits {
						t.Errorf("positive wait instance count got %d, want %d", inv.Total, wantWaits)
					}
					for _, occurrence := range inv.Occurrences {
						if occurrence.EndTs <= occurrence.StartTs {
							t.Errorf("zero-length transition minted a wait occurrence: %+v", occurrence)
						}
					}
				})
			}
		}
	}
}

func hmc331WakeBoundaryProjection(t *testing.T, r Result, start, end float64) map[ThreadState]float64 {
	t.Helper()
	if r.Timeline == nil || r.Timeline.IntegrityFailure != "" {
		t.Fatalf("public timeline unavailable: %+v", r.Timeline)
	}
	out := map[ThreadState]float64{}
	for _, interval := range r.Timeline.Intervals {
		lo, hi := math.Max(start, interval.StartTs), math.Min(end, interval.EndTs)
		if hi > lo {
			out[interval.State] += (hi - lo) * 1000
		}
	}
	return out
}

func TestHMC331WakeBoundaryFullAndWindowedOverlapAgree(t *testing.T) {
	for _, state := range []string{"S", "D"} {
		for _, wakeCPU := range []int{0, 1} {
			for _, tc := range hmc331WakeBoundaryCases {
				t.Run(fmt.Sprintf("%s/cpu%d/%s", state, wakeCPU, tc.name), func(t *testing.T) {
					path := hmc331WakeBoundarySource(t, state, wakeCPU, tc)
					full, err := BuildIndex(t.Context(), path)
					if err != nil {
						t.Fatal(err)
					}
					const start, end = 1.014, 1.018
					windowed, err := BuildIndexWithOptions(t.Context(), path, BuildOptions{
						AllowWindowedParse: true, TimeStart: start, TimeEnd: end, TimeStartSet: true, TimeEndSet: true,
						TimePaddingBefore: 0.05, TimePaddingAfter: 0.05,
					})
					if err != nil || !windowed.Windowed || full.Path != windowed.Path {
						t.Fatalf("same-source public windowed fixture failed: err=%v", err)
					}
					wide := hmc331WakeBoundaryProjection(t, Run(full, hmc331WakeBoundaryQuery("thread_timeline", 1, 1.04)), start, end)
					cold := hmc331WakeBoundaryProjection(t, Run(full, hmc331WakeBoundaryQuery("thread_timeline", start, end)), start, end)
					warm := hmc331WakeBoundaryProjection(t, Run(windowed, hmc331WakeBoundaryQuery("thread_timeline", start, end)), start, end)
					for _, bucket := range []ThreadState{StateRunning, StateRunnable, StateSSleep, StateDSleep, StateIOWait} {
						hmc331WakeBoundaryNear(t, "wide projection vs full narrow "+string(bucket), wide[bucket], cold[bucket])
						hmc331WakeBoundaryNear(t, "full vs windowed narrow "+string(bucket), cold[bucket], warm[bucket])
					}
				})
			}
		}
	}
}

func TestHMC331WakeBoundaryMeasurementDoesNotInventClosedSleep(t *testing.T) {
	for _, tc := range []hmc331WakeBoundaryCase{hmc331WakeBoundaryCases[0], hmc331WakeBoundaryCases[3], hmc331WakeBoundaryCases[5]} {
		t.Run(tc.name, func(t *testing.T) {
			path := hmc331WakeBoundarySource(t, "S", 1, tc)
			idx, err := BuildIndex(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			r := Run(idx, hmc331WakeBoundaryQuery("wakeup_chain", 1, 1.04))
			if r.WakeupChain == nil {
				t.Fatal("public wakeup chain missing")
			}
			found := false
			for _, edge := range r.WakeupChain.Edges {
				wait := edge.MeasuredWait
				if wait == nil {
					continue
				}
				if wait.EndTs <= wait.StartTs {
					t.Errorf("transition without positive elapsed wait gained measured closure: %+v", wait)
				}
				if math.Abs(wait.StartTs-1.01) < 1e-9 {
					found = true
					hmc331WakeBoundaryNear(t, "closed sleep retains its first ending wake", (wait.EndTs-wait.StartTs)*1000, tc.waitMS)
				}
			}
			if tc.waitMS > 0 && !found {
				t.Fatal("the observed positive closed sleep instance was lost")
			}
			if tc.waitMS == 0 && found {
				t.Fatal("equal-timestamp transition gained a closed sleep instance")
			}
		})
	}
}
