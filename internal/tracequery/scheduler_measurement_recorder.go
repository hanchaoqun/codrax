package tracequery

import (
	"crypto/sha256"
	"encoding/hex"
	"math"

	"github.com/hanchaoqun/codrax/internal/types"
)

// schedulerMeasurementSegment is one contribution already accepted by a
// native accumulator. State is the booked lane, not necessarily OriginalState
// (a D start can be booked as I/O). EndLine/Closure retain the physical close,
// before any fallback-to-marker/source line used by display accounting.
// This is provenance only: no consumer may derive a new duration or causal
// permission from this record. Unknown dimensions retain their explicit flags.
type schedulerMeasurementSegment struct {
	Thread           ThreadRef
	State            ThreadState
	OriginalState    ThreadState
	StartTs          float64
	EndTs            float64
	ActualStartTs    float64
	ActualEndTs      float64
	DurationMs       float64
	StartLine        int
	EndLine          int
	CPU              int
	Priority         int
	CPUKnown         bool
	PriorityClass    string
	Closure          string
	CPUProvenance    string
	CPUReason        string
	ExpectedCPU      int
	ObservedCPU      int
	ObservedCPUKnown bool
	IO               bool
	IOMarked         bool
	IOAmbiguous      bool
	Caller           string
}

// One recorder belongs to ONE native accumulator stream, never an arbitrary
// union of view rows. Running uses a per-(TID,CPU) stream so map iteration of
// different CPU lanes cannot reorder a hash. OffCPU/churn use their own
// timestamp-ordered thread streams. Finish precedes presentation/roster caps.
type schedulerMeasurementRecorder struct {
	cancel             *runCancelState
	lineStart, lineEnd int
	thread             ThreadRef
	method             string
	window             TimeWindow
	writer             schedulerPartitionHash
	count              int
	invalid            bool
}

func newSchedulerMeasurementRecorder(q Query, thread ThreadRef, method string, window TimeWindow) *schedulerMeasurementRecorder {
	switch method {
	case "off_cpu_sweep", "cpu_running_sweep", "state_churn_sweep":
	default:
		return nil
	}
	if q.runCancel.sample() || thread.PID <= 0 ||
		!schedulerMeasurementFinite(q.TimeStart, q.TimeEnd, window.StartTs, window.EndTs) ||
		window.EndTs <= window.StartTs ||
		(window.StartTs == 0 && !window.StartSet) ||
		q.LineStart < 0 || q.LineEnd < 0 || (q.LineStart > 0 && q.LineEnd > 0 && q.LineEnd < q.LineStart) {
		return nil
	}
	r := &schedulerMeasurementRecorder{cancel: q.runCancel, lineStart: q.LineStart, lineEnd: q.LineEnd,
		thread: thread, method: method, window: window,
		writer: schedulerPartitionHash{digest: sha256.New()}}
	w := &r.writer
	w.text("scheduler_partition:v1:native_accumulator")
	w.text(method)
	w.thread(thread)
	// Requested/native-pass bounds and the actual accumulation window are
	// distinct, e.g. a busy sweep clipped at EOF. Never alter q to make a
	// receipt fit. The enclosing result still owns parent/capture/clock scope.
	w.number(q.TimeStart)
	w.number(q.TimeEnd)
	w.flag(q.TimeStartSet)
	w.flag(q.TimeEndSet)
	w.integer(q.LineStart)
	w.integer(q.LineEnd)
	w.number(window.StartTs)
	w.number(window.EndTs)
	return r
}

func (r *schedulerMeasurementRecorder) add(segment schedulerMeasurementSegment) {
	if r == nil || r.invalid {
		return
	}
	if r.cancel.tick() || segment.Thread.PID != r.thread.PID || segment.State == "" ||
		!schedulerMeasurementFinite(segment.StartTs, segment.EndTs, segment.ActualStartTs, segment.ActualEndTs, segment.DurationMs) ||
		segment.EndTs <= segment.StartTs || segment.DurationMs <= 0 || segment.ActualEndTs < segment.ActualStartTs ||
		segment.StartTs < r.window.StartTs || segment.EndTs > r.window.EndTs {
		r.invalid = true
		return
	}
	w := &r.writer
	w.text("contribution:v1")
	w.thread(segment.Thread)
	w.text(string(segment.State))
	w.text(string(segment.OriginalState))
	w.number(segment.StartTs)
	w.number(segment.EndTs)
	w.number(segment.ActualStartTs)
	w.number(segment.ActualEndTs)
	w.number(segment.DurationMs)
	w.integer(segment.StartLine)
	w.integer(segment.EndLine)
	w.integer(segment.CPU)
	w.integer(segment.Priority)
	w.flag(segment.CPUKnown)
	w.text(segment.PriorityClass)
	w.text(segment.Closure)
	w.text(segment.CPUProvenance)
	w.text(segment.CPUReason)
	w.integer(segment.ExpectedCPU)
	w.integer(segment.ObservedCPU)
	w.flag(segment.ObservedCPUKnown)
	w.flag(segment.IO)
	w.flag(segment.IOMarked)
	w.flag(segment.IOAmbiguous)
	w.text(segment.Caller)
	r.count++
}

func (r *schedulerMeasurementRecorder) finish() *types.TraceSchedulerMeasurementDomain {
	if r == nil || r.invalid || r.count == 0 || r.cancel.sample() {
		return nil
	}
	return &types.TraceSchedulerMeasurementDomain{
		Version: 1, Status: "constructed_partition", Method: r.method, TargetTID: r.thread.PID,
		WindowStartTs: r.window.StartTs, WindowEndTs: r.window.EndTs,
		QueryLineStart: r.lineStart, QueryLineEnd: r.lineEnd,
		PartitionID: "scheduler_partition:v1:" + hex.EncodeToString(r.writer.digest.Sum(nil)),
	}
}

func schedulerMeasurementFinite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}
