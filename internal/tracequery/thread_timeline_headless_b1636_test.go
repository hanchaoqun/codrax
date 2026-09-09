package tracequery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// B1636 extends the existing three-face headless-wakeup ruling to the
// focused timeline and its public Run target account. The trace begins
// before the first target event, so EOF completeness cannot prove the
// target's initial state.
func b1636HeadlessTrace(t *testing.T, duplicate bool) string {
	t.Helper()
	lines := []string{
		" idle-0 (0) [000] .... 1.000000: cpu_idle: state=0 cpu_id=0",
		" creator-7 (7) [000] .... 1.010000: sched_wakeup: comm=frag pid=55 prio=120 target_cpu=000",
	}
	if duplicate {
		lines = append(lines, " creator-7 (7) [000] .... 1.020000: sched_wakeup: comm=frag pid=55 prio=120 target_cpu=000")
	}
	lines = append(lines,
		" idle-0 (0) [000] .... 1.030000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=frag next_pid=55 next_prio=120",
		" frag-55 (55) [000] .... 1.045000: sched_switch: prev_comm=frag prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" creator-7 (7) [000] .... 1.060000: sched_wakeup: comm=frag pid=55 prio=120 target_cpu=000",
		" idle-0 (0) [000] .... 1.075000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=frag next_pid=55 next_prio=120",
		" frag-55 (55) [000] .... 1.090000: sched_switch: prev_comm=frag prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" idle-0 (0) [000] .... 1.100000: cpu_idle: state=0 cpu_id=0",
	)
	return writeSchedulerCarryTrace(t, "headless.ftrace", lines...)
}

func b1636Query() Query {
	return Query{PID: 55, TimeStart: 1, TimeEnd: 1.1, TimeStartSet: true, TimeEndSet: true, Limit: 1000, MinDurationMs: 0.000001}
}

func b1636EachIndex(t *testing.T, path string, start, end float64, run func(*testing.T, *Index)) {
	t.Helper()
	for _, windowed := range []bool{false, true} {
		name := "full"
		if windowed {
			name = "windowed"
		}
		t.Run(name, func(t *testing.T) {
			var idx *Index
			var err error
			if windowed {
				idx = buildSchedulerCarryWindow(t, path, start, end)
			} else {
				idx, err = BuildIndex(t.Context(), path)
				if err != nil {
					t.Fatal(err)
				}
			}
			before, err := json.Marshal(idx.Events)
			if err != nil {
				t.Fatal(err)
			}
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			run(t, idx)
			after, _ := json.Marshal(idx.Events)
			if !bytes.Equal(before, after) {
				t.Fatal("query mutated indexed source events")
			}
			afterSource, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(source, afterSource) {
				t.Fatalf("query mutated the source artifact: %v", err)
			}
		})
	}
}

// Values are [running, runnable, S, D, IO, known total], never a forced
// whole-window total. Use Run twice to exercise both public attachment paths.
func b1636AssertAccount(t *testing.T, idx *Index, q Query, want [6]float64) TimelineResult {
	t.Helper()
	q.View = "thread_timeline"
	r := Run(idx, q)
	if r.Timeline == nil || r.Timeline.IntegrityFailure != "" {
		t.Fatalf("missing usable public timeline: %+v", r)
	}
	tl := *r.Timeline
	var got [6]float64
	for _, iv := range tl.Intervals {
		switch iv.State {
		case StateRunning:
			got[0] += iv.DurationMs
		case StateRunnable:
			got[1] += iv.DurationMs
		case StateSSleep:
			got[2] += iv.DurationMs
		case StateDSleep:
			got[3] += iv.DurationMs
		case StateIOWait:
			got[4] += iv.DurationMs
		}
	}
	for i := 0; i < 5; i++ {
		got[5] += got[i]
	}
	for i := range want {
		if !near(got[i], want[i], 0.00001) {
			t.Errorf("public timeline lane %d = %.9f, want %.9f", i, got[i], want[i])
		}
	}
	q.View = "window_stats"
	r = Run(idx, q)
	a := r.TargetWindowStates
	if a == nil {
		t.Fatal("public target account missing")
	}
	got = [6]float64{a.RunningMs, a.RunnableMs, a.SleepMs, a.DStateMs, a.IOWaitMs, a.TotalMs}
	for i := range want {
		if !near(got[i], want[i], 0.00001) {
			t.Errorf("public account lane %d = %.9f, want %.9f", i, got[i], want[i])
		}
	}
	return tl
}

func TestB1636HeadlessWakeupPublicTimelineKeepsUnknownPrefix(t *testing.T) {
	for _, artifactWide := range []bool{false, true} {
		name := "explicit"
		if artifactWide {
			name = "artifact_wide"
		}
		t.Run(name, func(t *testing.T) {
			path := b1636HeadlessTrace(t, false)
			b1636EachIndex(t, path, 1, 1.1, func(t *testing.T, idx *Index) {
				q := b1636Query()
				if artifactWide {
					q.TimeStart, q.TimeEnd = 0, 0
					q.TimeStartSet, q.TimeEndSet = false, false
				}
				tl := b1636AssertAccount(t, idx, q, [6]float64{30, 35, 25, 0, 0, 90})
				if len(tl.Intervals) != 6 || tl.Intervals[0].State != StateRunnable ||
					!near(tl.Intervals[0].StartTs, 1.010, 0.0000001) || !near(tl.Intervals[0].EndTs, 1.030, 0.0000001) ||
					tl.Intervals[0].StartLine != 3 || tl.Intervals[0].EndLine != 4 {
					t.Fatalf("headless segment must retain actual wake/sched-in coordinates: %+v", tl.Intervals)
				}
				if idx.Windowed && (tl.HeadState == nil || tl.HeadState.Status != "unknown" ||
					!strings.Contains(strings.Join(tl.Caveats, "\n"), "scheduler_head_state_unknown=true")) {
					t.Fatalf("observed suffix must not erase unknown prefix: %+v", tl)
				}
				stats := ComputeWindowStats(idx, b1636Query())
				if stats.SchedulerHeadCoverage == nil || stats.SchedulerHeadCoverage.Status != "partial_unknown" {
					t.Fatalf("full/windowed known total must not imply a known prefix: %+v", stats.SchedulerHeadCoverage)
				}
				row := threadDurationForPID(stats.RunnableTop, 55)
				if row == nil || !near(row.DurationMs, 35, 0.00001) {
					t.Fatalf("existing indexed offCPU face changed: %+v", row)
				}
			})
			stream, err := StreamStateCluster(t.Context(), path, b1636Query(), 1000)
			if err != nil || stream.WindowStats == nil {
				t.Fatalf("stream failed: %v", err)
			}
			row := threadDurationForPID(stream.WindowStats.RunnableTop, 55)
			if row == nil || !near(row.DurationMs, 35, 0.00001) ||
				stream.WindowStats.SchedulerHeadCoverage == nil || stream.WindowStats.SchedulerHeadCoverage.Status != "partial_unknown" {
				t.Fatalf("existing stream runnable/prefix contract changed: %+v", stream.WindowStats)
			}
		})
	}
}

func TestB1636DonghuPublicAccountIncludesObservedHeadlessFortyMicroseconds(t *testing.T) {
	path := filepath.Join("..", "..", "eval", "fixtures", "real_traces", "donghu.ftrace")
	q := Query{PID: 2955, TimeStart: 13762.791708, TimeEnd: 13763.024898, TimeStartSet: true, TimeEndSet: true, Limit: 1000, MinDurationMs: 0.000001, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	b1636EachIndex(t, path, q.TimeStart, q.TimeEnd, func(t *testing.T, idx *Index) {
		tl := b1636AssertAccount(t, idx, q, [6]float64{74.915, 1.576, 118.586, 36.757, 0, 231.834})
		found := false
		for _, iv := range tl.Intervals {
			if iv.State == StateRunnable && iv.StartLine == 131 && iv.EndLine == 139 {
				found = near(iv.StartTs, 13762.793064, 0.0000001) && near(iv.EndTs, 13762.793104, 0.0000001) && near(iv.DurationMs, .040, .00001)
			}
		}
		if !found {
			t.Fatal("original capture wake line 131 → sched-in line 139 must supply precisely .040 ms")
		}
		latency := BuildSchedulerLatencyStats(idx, q)
		if !near(latency.MeanMs*float64(latency.Count), 1.576, .00001) {
			t.Fatalf("complete scheduler latency account changed: %+v", latency)
		}
	})
}

func TestB1636HeadlessWakeupLineBoundsDoNotBorrowIndexSeed(t *testing.T) {
	path := b1636HeadlessTrace(t, false)
	for _, tc := range []struct {
		name      string
		lineStart int
		start     float64
		runnable  float64
		total     float64
	}{
		{"include_wake", 3, 1, 35, 90},
		{"exclude_wake", 4, 1, 15, 70},
		{"include_prewindow_wake", 3, 1.020, 25, 80},
		{"exclude_prewindow_wake", 4, 1.020, 15, 70},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := b1636Query()
			q.LineStart, q.LineEnd, q.TimeStart = tc.lineStart, 9, tc.start
			b1636EachIndex(t, path, q.TimeStart, q.TimeEnd, func(t *testing.T, idx *Index) {
				// The physical wake remains in both indexes. Only the query's
				// line boundary, not index retention, determines admission.
				retained := false
				for _, ev := range idx.Events {
					retained = retained || ev.Line == 3 && ev.WakeePID == 55
				}
				if !retained {
					t.Fatal("fixture must retain the excluded wake in the index")
				}
				tl := b1636AssertAccount(t, idx, q, [6]float64{30, tc.runnable, 25, 0, 0, tc.total})
				if tl.HeadState != nil {
					t.Fatalf("line query must not borrow a time-head snapshot: %+v", tl.HeadState)
				}
				for _, iv := range tl.Intervals {
					if iv.StartLine < q.LineStart {
						t.Fatalf("excluded source became an interval: %+v", iv)
					}
				}
			})
			stream, err := StreamStateCluster(t.Context(), path, q, 1000)
			if err != nil || stream.WindowStats == nil {
				t.Fatalf("stream failed: %v", err)
			}
			row := threadDurationForPID(stream.WindowStats.RunnableTop, 55)
			if row == nil || !near(row.DurationMs, tc.runnable, .00001) {
				t.Fatalf("existing stream line scope changed: %+v; want %g", row, tc.runnable)
			}
		})
	}
}

func TestB1636HeadlessDuplicateWakeKeepsFirstObservedStart(t *testing.T) {
	path := b1636HeadlessTrace(t, true)
	b1636EachIndex(t, path, 1, 1.1, func(t *testing.T, idx *Index) {
		tl := b1636AssertAccount(t, idx, b1636Query(), [6]float64{30, 35, 25, 0, 0, 90})
		first := tl.Intervals[0]
		if first.State != StateRunnable || first.StartLine != 3 || first.EndLine != 5 ||
			!near(first.ActualStartTs, 1.010, .0000001) || !near(first.DurationMs, 20, .00001) {
			t.Fatalf("duplicate wake shifted or duplicated the first runnable span: %+v", first)
		}
	})
}

func TestB1636OrdinaryWakeDoesNotReopenRunningAtHeadOrInsideWindow(t *testing.T) {
	for _, wakeAt := range []float64{1, 1.020} {
		t.Run(fmt.Sprintf("wake_%.3f", wakeAt), func(t *testing.T) {
			path := writeSchedulerCarryTrace(t, "running.ftrace",
				" idle-0 (0) [000] .... 0.900000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=frag next_pid=55 next_prio=120",
				fmt.Sprintf(" creator-7 (7) [000] .... %.6f: sched_wakeup: comm=frag pid=55 prio=120 target_cpu=000", wakeAt),
				" idle-0 (0) [001] .... 1.100000: cpu_idle: state=0 cpu_id=1",
			)
			b1636EachIndex(t, path, 1, 1.1, func(t *testing.T, idx *Index) {
				tl := b1636AssertAccount(t, idx, b1636Query(), [6]float64{100, 0, 0, 0, 0, 100})
				if len(tl.Intervals) != 1 || tl.Intervals[0].State != StateRunning ||
					!near(tl.Intervals[0].ActualStartTs, .900, .0000001) {
					t.Fatalf("ordinary wake displaced known running carry: %+v", tl.Intervals)
				}
			})
		})
	}
}

func TestB1636ExplicitZeroWakeKeepsRealZeroCoordinates(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "zero.ftrace",
		" creator-7 (7) [000] .... 0.000000: sched_wakeup: comm=frag pid=55 prio=120 target_cpu=000",
		" idle-0 (0) [000] .... 0.030000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=frag next_pid=55 next_prio=120",
		" frag-55 (55) [000] .... 0.090000: sched_switch: prev_comm=frag prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" idle-0 (0) [000] .... 0.100000: cpu_idle: state=0 cpu_id=0",
	)
	q := b1636Query()
	q.TimeStart, q.TimeEnd = 0, .1 // explicit flags remain true
	b1636EachIndex(t, path, 0, .1, func(t *testing.T, idx *Index) {
		tl := b1636AssertAccount(t, idx, q, [6]float64{60, 30, 10, 0, 0, 100})
		first := tl.Intervals[0]
		if first.State != StateRunnable || first.StartTs != 0 || first.ActualStartTs != 0 || first.StartLine != 2 || first.EndLine != 3 {
			t.Fatalf("explicit zero became missing/backfilled time: %+v", first)
		}
	})
}

func TestB1636HeadlessWakeDoesNotExtendPastPhysicalTail(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "tail.ftrace",
		" idle-0 (0) [000] .... 1.000000: cpu_idle: state=0 cpu_id=0",
		" creator-7 (7) [000] .... 1.010000: sched_wakeup: comm=frag pid=55 prio=120 target_cpu=000",
		" idle-0 (0) [000] .... 1.070000: cpu_idle: state=0 cpu_id=0",
	)
	b1636EachIndex(t, path, 1, 1.1, func(t *testing.T, idx *Index) {
		tl := b1636AssertAccount(t, idx, b1636Query(), [6]float64{0, 60, 0, 0, 0, 60})
		if len(tl.Intervals) != 1 || !near(tl.Intervals[0].EndTs, 1.070, .0000001) || tl.Intervals[0].EndLine != 0 ||
			!containsSubstring(tl.Caveats, "trace_artifact_tail_uncovered=true") {
			t.Fatalf("headless open state must retain the unmeasured tail boundary: %+v", tl)
		}
	})
}

func TestB1636NewIncarnationAndCrossGenerationGatesRemainDistinct(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "generation.ftrace",
		" idle-0 (0) [000] .... 0.700000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=55 next_prio=120",
		" old-55 (55) [000] .... 0.800000: sched_switch: prev_comm=old prev_pid=55 prev_prio=120 prev_state=X ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" creator-7 (7) [000] .... 1.010000: sched_wakeup_new: comm=frag pid=55 prio=120 target_cpu=000",
		" idle-0 (0) [000] .... 1.030000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=frag next_pid=55 next_prio=120",
		" frag-55 (55) [000] .... 1.090000: sched_switch: prev_comm=frag prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" idle-0 (0) [000] .... 1.100000: cpu_idle: state=0 cpu_id=0",
	)
	b1636EachIndex(t, path, 1, 1.1, func(t *testing.T, idx *Index) {
		tl := b1636AssertAccount(t, idx, b1636Query(), [6]float64{60, 20, 10, 0, 0, 90})
		if !containsSubstring(tl.Caveats, "thread_generation_boundary=true") {
			t.Fatalf("new-incarnation disclosure changed: %+v", tl.Caveats)
		}
	})
	b1636EachIndex(t, path, .7, 1.1, func(t *testing.T, idx *Index) {
		q := b1636Query()
		q.TimeStart, q.View = .7, "thread_timeline"
		r := Run(idx, q)
		if r.Timeline == nil || r.Timeline.IntegrityFailure != "thread_incarnation_conflict" || len(r.Timeline.Intervals) != 0 {
			t.Fatalf("cross-generation query gained a measurable timeline: %+v", r.Timeline)
		}
		q.View = "window_stats"
		if r = Run(idx, q); r.TargetWindowStates != nil {
			t.Fatalf("cross-generation query gained a target state account: %+v", r.TargetWindowStates)
		}
	})
}

func TestB1636TiebaCandidateRestoresOnlyClippedHeadlessSlice(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseTiebaFixture)
	q := Query{PID: 59843, TimeStart: 34579.472865, TimeEnd: 34579.475843, TimeStartSet: true, TimeEndSet: true, View: "thread_timeline", TraceFlavorHint: TraceFlavorHarmonyHitrace}
	r := Run(idx, q)
	if r.Timeline == nil || len(r.Timeline.Intervals) == 0 {
		t.Fatal("clipped dependency timeline missing")
	}
	iv := r.Timeline.Intervals[0]
	if iv.State != StateRunnable || iv.StartLine != 2776 || iv.EndLine != 2891 ||
		!near(iv.ActualStartTs, 34579.472841, 1e-7) || !near(iv.ActualEndTs, 34579.473581, 1e-7) ||
		!near(iv.ActualDurationMs, .740, 1e-5) || !near(iv.StartTs, q.TimeStart, 1e-7) || !near(iv.DurationMs, .716, 1e-5) {
		t.Fatalf("physical .740ms must contribute only its .716ms in-window slice: %+v", iv)
	}
	q.PID, q.TimeStart, q.TimeEnd = 59566, 34579.450627, 34579.520000
	rank := BuildRootCauseRank(idx, q)
	cand := evalcaseDHMFindItem(rank.Items, "priority_inversion_candidate", 59843)
	wait := evalcaseDHMFindItem(rank.Items, "priority_inversion_runnable_wait", 59843)
	if cand == nil || wait == nil || len(cand.OccurrenceWindows) != 3 {
		t.Fatal("original two inversion forms and three candidate occurrences must remain")
	}
	if !near(cand.ImpactMs, 12.12795295212336+.716, 1e-5) || !near(cand.RunnableMs, 12.813, 1e-5) ||
		!near(cand.RunningMs, 1.0779999938677065, 1e-5) || !near(cand.GatedRunningDeficitMs, .030952945944499766, 1e-7) ||
		!near(wait.ImpactMs, 19.372, .001) || !near(wait.EffectiveImpactMs, 1.847, .001) {
		t.Fatalf("headless correction changed a different ruler: candidate=%+v wait=%+v", cand, wait)
	}
}

func TestB1636TiebaRestoredOverlapKeepsFamilyMembersAndOtherCauses(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseTiebaFixture)
	q := Query{PID: 60560, TimeStart: 34579.450627, TimeEnd: 34579.595184, TimeStartSet: true, TimeEndSet: true, View: "thread_timeline", TraceFlavorHint: TraceFlavorHarmonyHitrace}
	r := Run(idx, q)
	if r.Timeline == nil || len(r.Timeline.Intervals) == 0 {
		t.Fatal("target timeline missing")
	}
	iv := r.Timeline.Intervals[0]
	if iv.State != StateRunnable || iv.StartLine != 2659 || iv.EndLine != 4310 ||
		!near(iv.StartTs, 34579.472126, 1e-7) || !near(iv.EndTs, 34579.482908, 1e-7) || !near(iv.DurationMs, 10.782, 1e-5) {
		t.Fatalf("restored self overlap lacks its original physical interval: %+v", iv)
	}
	rank := BuildRootCauseRank(idx, q)
	var own []RootCauseRankItem
	other := map[string]int{}
	for _, item := range rank.Items {
		if item.Thread.PID != q.PID {
			other[fmt.Sprintf("%d/%s", item.Thread.PID, item.Type)]++
		} else if item.Type == "priority_inversion_runnable_wait" {
			own = append(own, item)
		}
		if item.P3MCounterfactualInvalidMs != 0 {
			t.Fatalf("restoring an interval changed the existing no-conviction boundary: %+v", item)
		}
	}
	if len(own) != 1 {
		t.Fatalf("same-basis CPU members must use the existing single family fold: %+v", own)
	}
	family := own[0]
	if family.MemberCount != 3 || family.MemberFoldCaliber != "sum_disjoint" ||
		!near(family.ImpactMs, 10.816+5.450+3.037, 1e-5) || !near(family.EffectiveImpactMs, 12.173, 1e-5) ||
		family.P3MDisposition != p3mDispositionSelfRuled || family.Causality != "on_wakeup_chain" ||
		family.OnChainBasis != "" || !family.SubjectIsAnalysisTarget {
		t.Fatalf("three observed CPU members changed value or proof basis: %+v", family)
	}
	for _, member := range []string{"cpu=4 10.816ms", "cpu=0 5.450ms", "cpu=2 3.037ms"} {
		if !containsSubstring(family.MemberRoster, member) {
			t.Fatalf("family lost original CPU member %q: %v", member, family.MemberRoster)
		}
	}
	// Before/after source audit: all fourteen non-target rows remain. The
	// one-row census reduction is a same-target merge, not a cap eviction.
	wantOther := map[string]int{
		"60595/priority_inversion_candidate": 2, "897/runnable_wait": 1, "59566/io_wait": 1,
		"60559/priority_inversion_candidate": 1, "60555/d_state_or_io_wait": 1, "61238/running": 1,
		"9388/trace_span": 1, "7640/trace_span": 1, "0/supply_pressure": 1,
		"897/priority_inversion_runnable_wait": 1, "918/priority_inversion_runnable_wait": 1,
		"919/priority_inversion_runnable_wait": 1, "60595/priority_inversion_runnable_wait": 1,
	}
	gotJSON, _ := json.Marshal(other)
	wantJSON, _ := json.Marshal(wantOther)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("other cause population changed: got=%s want=%s", gotJSON, wantJSON)
	}
	sep := evalcaseDHMFindItem(rank.Items, "running", 61238)
	if sep == nil || !near(sep.ImpactMs, .175, .001) || !near(sep.P3MCounterfactualValidMs, 65.514, .001) ||
		!near(sep.P3MEdgeWitnessedMs, .077, .001) || sep.P3MDisposition != p3mDispositionEdgeTerminatedWindow {
		t.Fatalf("independent Compositor two-ruler evidence changed: %+v", sep)
	}
}
