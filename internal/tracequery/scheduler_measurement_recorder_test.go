package tracequery

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"
)

func b1638b2MeasurementSegment() schedulerMeasurementSegment {
	return schedulerMeasurementSegment{Thread: ThreadRef{PID: 55, TGID: 55, Comm: "worker"},
		State: StateIOWait, OriginalState: StateDSleep, StartTs: 1.1, EndTs: 1.2,
		ActualStartTs: 1.09, ActualEndTs: 1.21, DurationMs: 100, StartLine: 10, EndLine: 20,
		CPU: 0, CPUKnown: true, Priority: 120, PriorityClass: "normal", Closure: "sched_wakeup",
		IO: true, IOMarked: true, Caller: "wait_on_page"}
}

func TestB1638B2RecorderPreservesTypedContributionAxes(t *testing.T) {
	q := Query{TimeStart: 1, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true}
	window := queryResultTimeWindow(q)
	segment := b1638b2MeasurementSegment()
	makeDomain := func(q Query, window TimeWindow, method string, row schedulerMeasurementSegment) string {
		r := newSchedulerMeasurementRecorder(q, row.Thread, method, window)
		r.add(row)
		domain := r.finish()
		if domain == nil || domain.Status != "constructed_partition" {
			t.Fatal("valid native contribution must retain a bounded provenance receipt")
		}
		return domain.PartitionID
	}
	want := makeDomain(q, window, "off_cpu_sweep", segment)
	for name, mutate := range map[string]func(*schedulerMeasurementSegment){
		"thread":         func(r *schedulerMeasurementSegment) { r.Thread.Comm = "renamed" },
		"tgid":           func(r *schedulerMeasurementSegment) { r.Thread.TGID++ },
		"booked_state":   func(r *schedulerMeasurementSegment) { r.State = StateDSleep },
		"original_state": func(r *schedulerMeasurementSegment) { r.OriginalState = StateIOWait },
		"start":          func(r *schedulerMeasurementSegment) { r.StartTs = math.Nextafter(r.StartTs, 2) },
		"end":            func(r *schedulerMeasurementSegment) { r.EndTs = math.Nextafter(r.EndTs, 2) },
		"actual_start":   func(r *schedulerMeasurementSegment) { r.ActualStartTs = 1.08 },
		"actual_end":     func(r *schedulerMeasurementSegment) { r.ActualEndTs = 1.22 },
		"duration":       func(r *schedulerMeasurementSegment) { r.DurationMs++ },
		"start_line":     func(r *schedulerMeasurementSegment) { r.StartLine++ },
		"end_line":       func(r *schedulerMeasurementSegment) { r.EndLine = 0 },
		"cpu":            func(r *schedulerMeasurementSegment) { r.CPU++ },
		"cpu_unknown":    func(r *schedulerMeasurementSegment) { r.CPUKnown = false },
		"priority":       func(r *schedulerMeasurementSegment) { r.Priority++ },
		"priority_class": func(r *schedulerMeasurementSegment) { r.PriorityClass = "realtime" },
		"closure":        func(r *schedulerMeasurementSegment) { r.Closure = "window_end" },
		"cpu_origin":     func(r *schedulerMeasurementSegment) { r.CPUProvenance = "wake_target" },
		"cpu_reason":     func(r *schedulerMeasurementSegment) { r.CPUReason = "conflict" },
		"expected_cpu":   func(r *schedulerMeasurementSegment) { r.ExpectedCPU++ },
		"observed_cpu":   func(r *schedulerMeasurementSegment) { r.ObservedCPU++ },
		"observed_known": func(r *schedulerMeasurementSegment) { r.ObservedCPUKnown = true },
		"io":             func(r *schedulerMeasurementSegment) { r.IO = false },
		"io_marked":      func(r *schedulerMeasurementSegment) { r.IOMarked = false },
		"io_ambiguous":   func(r *schedulerMeasurementSegment) { r.IOAmbiguous = true },
		"caller":         func(r *schedulerMeasurementSegment) { r.Caller = "another_wait" },
	} {
		t.Run(name, func(t *testing.T) {
			row := segment
			mutate(&row)
			if makeDomain(q, window, "off_cpu_sweep", row) == want {
				t.Fatal("different native typed contribution acquired the same receipt")
			}
		})
	}
	for _, method := range []string{"cpu_running_sweep", "state_churn_sweep"} {
		if makeDomain(q, window, method, segment) == want {
			t.Fatal("different native algorithms became one measurement")
		}
	}
	q.View, q.Limit, q.MaxDepth, q.MaxBranches, q.MinDurationMs = "other_view", 1, 1, 1, 100
	if makeDomain(q, window, "off_cpu_sweep", segment) != want {
		t.Fatal("presentation/chain caps changed native contribution provenance")
	}
	q.LineEnd = 30
	if makeDomain(q, window, "off_cpu_sweep", segment) == want {
		t.Fatal("line-filtered source collapsed into an unfiltered source")
	}
	q.LineEnd, q.TimeEnd = 0, 3
	if makeDomain(q, window, "off_cpu_sweep", segment) == want {
		t.Fatal("actual accumulation window hid different requested pass bounds")
	}
}

func TestB1638B2RecorderUnknownCancelAndOutputCopies(t *testing.T) {
	q := Query{TimeStart: 1, TimeEnd: 2}
	row := b1638b2MeasurementSegment()
	for name, mutate := range map[string]func(*schedulerMeasurementSegment){
		"different_tid":   func(r *schedulerMeasurementSegment) { r.Thread.PID++ },
		"nan":             func(r *schedulerMeasurementSegment) { r.StartTs = math.NaN() },
		"infinite":        func(r *schedulerMeasurementSegment) { r.DurationMs = math.Inf(1) },
		"zero":            func(r *schedulerMeasurementSegment) { r.DurationMs = 0 },
		"out_of_window":   func(r *schedulerMeasurementSegment) { r.EndTs = 3 },
		"reversed_actual": func(r *schedulerMeasurementSegment) { r.ActualEndTs = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			r := newSchedulerMeasurementRecorder(q, row.Thread, "off_cpu_sweep", queryResultTimeWindow(q))
			bad := row
			mutate(&bad)
			r.add(row)
			r.add(bad)
			r.add(row)
			if r.finish() != nil {
				t.Fatal("invalid contribution must withdraw the whole receipt, not retain a seemingly complete prefix")
			}
		})
	}
	var absent *schedulerMeasurementRecorder
	absent.add(row)
	if absent.finish() != nil {
		t.Fatal("nil recorder fabricated provenance")
	}
	r := newSchedulerMeasurementRecorder(q, row.Thread, "off_cpu_sweep", queryResultTimeWindow(q))
	if r.finish() != nil {
		t.Fatal("empty native stream fabricated provenance")
	}
	r.add(row)
	a, b := r.finish(), r.finish()
	if a == nil || b == nil || a == b || !reflect.DeepEqual(a, b) {
		t.Fatal("finish must be stable and independently owned")
	}
	b.PartitionID = "changed"
	if r.finish().PartitionID != a.PartitionID {
		t.Fatal("output mutation changed native recorder")
	}
	ctx, cancel := context.WithCancel(t.Context())
	r = newSchedulerMeasurementRecorder(q.WithRunContext(ctx), row.Thread, "off_cpu_sweep", queryResultTimeWindow(q))
	r.add(row)
	cancel()
	if r.finish() != nil {
		t.Fatal("canceled native stream published a partial receipt")
	}
	for _, window := range []TimeWindow{{StartTs: 0, EndTs: 2}, {StartTs: 1, EndTs: 0}, {StartTs: 2, EndTs: 1}} {
		if newSchedulerMeasurementRecorder(q, row.Thread, "off_cpu_sweep", window) != nil {
			t.Fatal("ambiguous/unbounded window acquired a receipt")
		}
	}
	zero := TimeWindow{StartTs: 0, EndTs: 2, StartSet: true}
	if newSchedulerMeasurementRecorder(q, row.Thread, "off_cpu_sweep", zero) == nil {
		t.Fatal("explicit rebased zero must remain available")
	}
}

func TestB1638B2RecorderFieldCensus(t *testing.T) {
	typ := reflect.TypeOf(schedulerMeasurementSegment{})
	var fields []string
	for i := 0; i < typ.NumField(); i++ {
		fields = append(fields, typ.Field(i).Name)
	}
	const want = "Thread State OriginalState StartTs EndTs ActualStartTs ActualEndTs DurationMs StartLine EndLine CPU Priority CPUKnown PriorityClass Closure CPUProvenance CPUReason ExpectedCPU ObservedCPU ObservedCPUKnown IO IOMarked IOAmbiguous Caller"
	if strings.Join(fields, " ") != want {
		t.Fatalf("new contribution fields require explicit hash-input review: %v", fields)
	}
}

func TestB1638B2RecorderMemoryDoesNotGrowPerSegment(t *testing.T) {
	q := Query{TimeStart: 1, TimeEnd: 2}
	row := b1638b2MeasurementSegment()
	measure := func(n int) float64 {
		return testing.AllocsPerRun(3, func() {
			r := newSchedulerMeasurementRecorder(q, row.Thread, "off_cpu_sweep", queryResultTimeWindow(q))
			for i := 0; i < n; i++ {
				r.add(row)
			}
			if r.finish() == nil {
				t.Fatal("full native stream lost provenance")
			}
		})
	}
	if small, large := measure(1), measure(4096); large > small+1 {
		t.Fatalf("per-segment allocations: small=%g large=%g", small, large)
	}
}
