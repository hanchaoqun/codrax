package tracequery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the published engine output before referring to the proposed field:
// a missing source receipt must be a semantic failure, not a compilation error.
func b1638b2aOffCPUWireDomain(t *testing.T, td ThreadDuration) map[string]any {
	t.Helper()
	body, err := json.Marshal(td)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	domain, ok := object["measurement_domain"].(map[string]any)
	if !ok || domain["partition_id"] == "" {
		t.Fatalf("native OffCPU output has no measurement receipt: %s", body)
	}
	if domain["method"] != "off_cpu_sweep" || domain["status"] != "constructed_partition" ||
		domain["version"] != float64(1) || domain["target_tid"] != float64(td.Thread.PID) {
		t.Fatalf("receipt does not identify this native four-state sweep: %v", domain)
	}
	return domain
}

func b1638b2aNativeOffCPU(idx *Index, q Query) offCPUStatsResult {
	return computeOffCPUStats(idx, ensureQueryFlavor(idx, q), func(int) []Event { return nil }, map[int]*cpuPressureAcc{}, nil)
}

func b1638b2aTargetDuration(t *testing.T, rows []ThreadDuration, pid int) ThreadDuration {
	t.Helper()
	for _, td := range rows {
		if td.Thread.PID == pid {
			return td
		}
	}
	t.Fatalf("native fixture did not produce PID %d in %+v", pid, rows)
	return ThreadDuration{}
}

func TestB1638B2AOffCPURecordsFinalLaneAndPhysicalClosure(t *testing.T) {
	for _, tc := range []struct {
		name, state, close string
		io                 bool
	}{
		{"d_sched_in", "D", "sched_switch", false},
		{"io_sched_in", "D", "sched_switch", true},
		{"d_wake", "D", "sched_wakeup", false},
		{"io_waking", "D", "sched_waking", true},
		{"d_query_tail", "D", "window_end", false},
		{"io_query_tail", "D", "window_end", true},
		{"sleep_sched_in", "S", "sched_switch", false},
		{"sleep_wake", "S", "sched_wakeup", false},
		{"sleep_query_tail", "S", "window_end", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n"
			body += fmt.Sprintf("target-41 (41) [000] .... 1.100000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=%s ==> next_comm=waker next_pid=2 next_prio=120\n", tc.state)
			closeLine := 3
			if tc.state == "D" {
				io := 0
				if tc.io {
					io = 1
				}
				body += fmt.Sprintf("waker-2 (2) [000] .... 1.170000: sched_blocked_reason: pid=41 iowait=%d caller=io_schedule\n", io)
				closeLine++
			}
			switch tc.close {
			case "sched_switch":
				body += "waker-2 (2) [000] .... 1.200000: sched_switch: prev_comm=waker prev_pid=2 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n"
			case "window_end":
				closeLine = 0
			default:
				body += fmt.Sprintf("waker-2 (2) [000] .... 1.200000: %s: comm=target pid=41 prio=120 target_cpu=000\n", tc.close)
			}
			idx := buildTraceIndex(t, "one-close.ftrace", body)
			q := ensureQueryFlavor(idx, Query{TimeStart: 1.15, TimeEnd: 1.2, TimeStartSet: true, TimeEndSet: true})
			result := b1638b2aNativeOffCPU(idx, q)
			rows, state := result.dstateTop, StateDSleep
			if tc.io {
				rows, state = result.iowaitTop, StateIOWait
			} else if tc.state == "S" {
				rows, state = result.sleepTop, StateSSleep
			}
			td := b1638b2aTargetDuration(t, rows, 41)
			if math.Abs(td.DurationMs-50) > 1e-6 {
				t.Fatalf("receipt changed the original clipped value: %+v", td)
			}
			segment := schedulerMeasurementSegment{
				Thread: td.Thread, State: state, OriginalState: stateFromPrevState(tc.state),
				StartTs: 1.15, EndTs: 1.2, ActualStartTs: 1.1, ActualEndTs: 1.2, DurationMs: (1.2 - q.TimeStart) * 1000,
				StartLine: 2, EndLine: closeLine, CPU: 0, CPUKnown: true, Priority: 120,
				PriorityClass: classifyTracePriority(q.TraceFlavor, 120), CPUProvenance: runnableCPUProvenanceSchedSwitch,
				Closure: tc.close, IO: tc.io, IOMarked: tc.state == "D",
			}
			if tc.state == "D" {
				segment.Caller = "io_schedule"
			}
			recorder := newSchedulerMeasurementRecorder(q, td.Thread, "off_cpu_sweep", queryResultTimeWindow(q))
			recorder.add(segment)
			if got, want := td.MeasurementDomain, recorder.finish(); got == nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("receipt did not retain the booked lane/raw start/physical closing line and verdict: got=%+v want=%+v segment=%+v", got, want, segment)
			}
		})
	}
}

func TestB1638B2AOffCPUFullStreamBeforeCapsAndSourceUnchanged(t *testing.T) {
	const count = 399 // each D/IO lane exceeds its separate credential prefix cap
	body := b1607ManySleepsTrace(count)
	idx := buildTraceIndex(t, "full-stream.ftrace", body)
	before, _ := json.Marshal(idx.Events)
	q := Query{TimeStart: 10, TimeEnd: 10.401, TimeStartSet: true, TimeEndSet: true}
	result := b1638b2aNativeOffCPU(idx, q)
	io := b1638b2aTargetDuration(t, result.iowaitTop, 41)
	d := b1638b2aTargetDuration(t, result.dstateTop, 41)
	if !io.dioIntervalsOverflow || !d.dioIntervalsOverflow || len(io.dioIntervals) != CriticalBlockingCredentialSegmentCap ||
		len(d.dioIntervals) != CriticalBlockingCredentialSegmentCap {
		t.Fatal("fixture must exceed, and preserve, both existing credential caps")
	}
	if io.MeasurementDomain == nil || !reflect.DeepEqual(io.MeasurementDomain, d.MeasurementDomain) || io.MeasurementDomain == d.MeasurementDomain {
		t.Fatal("each bucket must carry an independent clone of its full native TID source")
	}
	last := strings.LastIndex(body, "caller=io_schedule")
	changedBody := body[:last] + strings.Replace(body[last:], "caller=io_schedule", "caller=late_wait", 1)
	changed := b1638b2aNativeOffCPU(buildTraceIndex(t, "late-change.ftrace", changedBody), q)
	changedIO := b1638b2aTargetDuration(t, changed.iowaitTop, 41)
	if changedIO.DurationMs != io.DurationMs || !reflect.DeepEqual(changedIO.dioIntervals, io.dioIntervals) ||
		changedIO.MeasurementDomain == nil || changedIO.MeasurementDomain.PartitionID == io.MeasurementDomain.PartitionID {
		t.Fatal("an uncapped late contribution must change native identity without changing duration or the old prefix")
	}
	filteredQ := q
	filteredQ.LineStart, filteredQ.LineEnd = 1, 20
	filtered := b1638b2aTargetDuration(t, b1638b2aNativeOffCPU(idx, filteredQ).iowaitTop, 41)
	if filtered.MeasurementDomain == nil || filtered.MeasurementDomain.PartitionID == io.MeasurementDomain.PartitionID ||
		filtered.MeasurementDomain.QueryLineEnd != 20 {
		t.Fatal("same time range must not merge a different source-line measurement")
	}
	for _, pressure := range result.pressureCensus {
		for _, row := range pressure.TopRunnable {
			if row.MeasurementDomain != nil {
				t.Fatal("pressure mirrors are not the native four-bucket source")
			}
		}
	}
	after, _ := json.Marshal(idx.Events)
	if !bytes.Equal(before, after) {
		t.Fatal("receipt construction mutated native source events")
	}
	var legacy ThreadDuration
	if err := json.Unmarshal([]byte(`{"thread":{"pid":41},"duration_ms":1,"cpu":0}`), &legacy); err != nil || legacy.MeasurementDomain != nil {
		t.Fatal("legacy duration must not acquire invented provenance")
	}
	copy := types.CloneTraceSchedulerMeasurementDomain(io.MeasurementDomain)
	io.MeasurementDomain.PartitionID = "test mutation"
	if !reflect.DeepEqual(d.MeasurementDomain, copy) {
		t.Fatal("independent bucket clone was mutated through another bucket")
	}
}

func TestB1638B2AOffCPUUnknownWindowCancelAndPoison(t *testing.T) {
	idx := buildTraceIndex(t, "known.ftrace", b1607ManySleepsTrace(3))
	unknown := b1638b2aNativeOffCPU(idx, Query{})
	row := b1638b2aTargetDuration(t, unknown.sleepTop, 41)
	if row.DurationMs <= 0 || row.MeasurementDomain != nil {
		t.Fatal("unbounded native values remain usable but cannot invent a measured source window")
	}
	q := Query{View: "window_stats", PID: 41, TimeStart: 10, TimeEnd: 10.005, TimeStartSet: true, TimeEndSet: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := b1638b2aNativeOffCPU(idx, q.WithRunContext(ctx))
	for _, rows := range [][]ThreadDuration{canceled.runnableTop, canceled.sleepTop, canceled.dstateTop, canceled.iowaitTop} {
		for _, td := range rows {
			if td.MeasurementDomain != nil {
				t.Fatal("canceled native pass acquired a finished receipt")
			}
		}
	}
	poison := buildTraceIndex(t, "reused.ftrace", b1607ManySleepsTrace(3)+
		"creator-7 (7) [000] .... 10.004500: sched_wakeup_new: comm=new pid=41 prio=120 target_cpu=000\n")
	result := Run(poison, q)
	if result.WindowStats != nil {
		for _, rows := range [][]ThreadDuration{result.WindowStats.RunnableTop, result.WindowStats.SleepTop, result.WindowStats.DStateTop, result.WindowStats.IOWaitTop} {
			for _, td := range rows {
				if td.Thread.PID == 41 {
					t.Fatal("existing incarnation guard must still remove unsafe target values and receipts")
				}
			}
		}
	}
}

func TestB1638B2AActualRunOffCPUCPUContinuity(t *testing.T) {
	for _, tc := range []struct {
		name, middle, close string
		cpus                []int
		ms                  []float64
		reason              string
	}{
		{
			name: "sched_in_migrated", cpus: []int{1}, ms: []float64{100},
			close:  "idle-0 (0) [001] .... 1.200000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n",
			reason: RunnableCPUContinuitySchedInMigrated,
		},
		{
			name: "migration_two_cpu_buckets", cpus: []int{0, 1}, ms: []float64{50, 50},
			middle: "mover-2 (2) [000] .... 1.150000: sched_migrate_task: comm=target pid=41 prio=120 orig_cpu=0 dest_cpu=1\n",
			close:  "idle-0 (0) [001] .... 1.200000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n",
			reason: RunnableCPUContinuityVerified,
		},
		{
			name: "conflicting_repeated_wake", cpus: []int{-1}, ms: []float64{100},
			middle: "waker-2 (2) [000] .... 1.150000: sched_wakeup: comm=target pid=41 prio=120 target_cpu=001\n",
			close:  "idle-0 (0) [001] .... 1.200000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n",
			reason: RunnableCPUContinuityWakeTargetConflict,
		},
		{
			name: "open_tail_cpu_unknown", cpus: []int{-1}, ms: []float64{200},
			reason: RunnableCPUContinuityOpenEnded,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1000000 cpu_id=0\n" +
				"waker-2 (2) [000] .... 1.100000: sched_wakeup: comm=target pid=41 prio=120 target_cpu=000\n" + tc.middle + tc.close +
				"idle-0 (0) [000] .... 1.300000: cpu_frequency: state=1000000 cpu_id=0\n"
			idx := buildTraceIndex(t, "continuity.ftrace", body)
			q := Query{View: "window_stats", PID: 41, TimeStart: 1, TimeEnd: 1.3, TimeStartSet: true, TimeEndSet: true}
			public := Run(idx, q)
			if public.WindowStats == nil {
				t.Fatal("fixture lost actual window stats")
			}
			native := b1638b2aNativeOffCPU(idx, q)
			var first *types.TraceSchedulerMeasurementDomain
			for i, cpu := range tc.cpus {
				found := false
				for _, row := range public.WindowStats.RunnableTop {
					if row.Thread.PID != 41 || row.CPU != cpu {
						continue
					}
					found = true
					if math.Abs(row.DurationMs-tc.ms[i]) > 1e-6 || row.MeasurementDomain == nil {
						t.Fatalf("native continuity value/source changed: %+v", row)
					}
					if first == nil {
						first = row.MeasurementDomain
					} else if first == row.MeasurementDomain || !reflect.DeepEqual(first, row.MeasurementDomain) {
						t.Fatal("distinct CPU buckets need independent copies of one complete TID stream")
					}
				}
				if !found {
					t.Fatalf("missing expected native CPU bucket %d: %+v", cpu, public.WindowStats.RunnableTop)
				}
			}
			for _, segment := range native.runnableSegments {
				if segment.thread.PID == 41 && segment.cpuContinuity != tc.reason {
					t.Fatalf("source-only addition changed original continuity: %+v", segment)
				}
			}
			// Reconstruct this pass's typed native receipt, retaining the exact
			// attribution verdict rather than converting unknown CPU to CPU 0.
			thread := b1638b2aTargetDuration(t, native.runnableTop, 41).Thread
			recorder := newSchedulerMeasurementRecorder(ensureQueryFlavor(idx, q), thread, "off_cpu_sweep", queryResultTimeWindow(q))
			for _, segment := range native.runnableSegments {
				if segment.thread.PID != 41 {
					continue
				}
				closure, provenance, expectedCPU, observedCPU, observedKnown := runnableCPUContinuityBoundarySchedIn, runnableCPUProvenanceWakeTarget, 0, 1, true
				if tc.name == "migration_two_cpu_buckets" {
					if segment.cpu == 0 {
						closure, observedCPU = runnableCPUContinuityBoundaryMigration, 0
					} else {
						provenance, expectedCPU = runnableCPUProvenanceMigration, 1
					}
				} else if tc.name == "open_tail_cpu_unknown" {
					closure, observedCPU, observedKnown = runnableCPUContinuityBoundaryWindowEnd, -1, false
				} else if tc.name == "conflicting_repeated_wake" {
					provenance = runnableCPUProvenanceWakeTargetConflict
				}
				endLine := segment.endLine
				if closure == runnableCPUContinuityBoundaryWindowEnd {
					endLine = 0 // source endpoint, not the old display fallback
				}
				recorder.add(schedulerMeasurementSegment{
					Thread: thread, State: StateRunnable, OriginalState: StateRunnable,
					StartTs: segment.startTs, EndTs: segment.endTs, ActualStartTs: segment.startTs, ActualEndTs: segment.endTs,
					DurationMs: segment.durationMs, StartLine: segment.startLine, EndLine: endLine,
					CPU: segment.cpu, CPUKnown: segment.cpuKnown, CPUReason: segment.cpuContinuity,
					Priority: segment.priority, PriorityClass: segment.priorityClass, Closure: closure,
					CPUProvenance: provenance, ExpectedCPU: expectedCPU, ObservedCPU: observedCPU, ObservedCPUKnown: observedKnown,
				})
			}
			if !reflect.DeepEqual(first, recorder.finish()) {
				t.Fatalf("actual native receipt lost exact continuity/closure input: got=%+v want=%+v", first, recorder.finish())
			}
		})
	}
}

func TestB1638B2AActualRunOffCPUReceipt(t *testing.T) {
	idx := buildTraceIndex(t, "offcpu.ftrace", b1607ManySleepsTrace(39))
	q := Query{View: "window_stats", PID: 41, TimeStart: 10, TimeEnd: 10.041, TimeStartSet: true, TimeEndSet: true, MinDurationMs: 0.001}
	result := Run(idx, q)
	if result.WindowStats == nil {
		t.Fatal("fixture must publish actual window statistics")
	}
	stats := result.WindowStats
	var want map[string]any
	for _, lane := range []struct {
		name string
		rows []ThreadDuration
		ms   float64
	}{
		{"runnable", stats.RunnableTop, 3.9},
		{"sleep", stats.SleepTop, 2.6},
		{"dstate", stats.DStateTop, 2.6},
		{"iowait", stats.IOWaitTop, 2.6},
	} {
		t.Run(lane.name, func(t *testing.T) {
			found := false
			for _, td := range lane.rows {
				if td.Thread.PID != 41 {
					continue
				}
				found = true
				if math.Abs(td.DurationMs-lane.ms) > 1e-6 {
					t.Fatalf("native measurement changed: got %.9f want %.9f", td.DurationMs, lane.ms)
				}
				got := b1638b2aOffCPUWireDomain(t, td)
				if want == nil {
					want = got
				} else if !reflect.DeepEqual(got, want) {
					t.Fatalf("one TID's four buckets must retain the same complete native close stream: got=%v want=%v", got, want)
				}
			}
			if !found {
				t.Fatal("fixture did not exercise its positive native bucket")
			}
		})
	}
}
