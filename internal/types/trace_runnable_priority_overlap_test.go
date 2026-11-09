package types

import (
	"math"
	"testing"
)

func TestHMC370NativePriorityWitnessBinding(t *testing.T) {
	ready := TraceThreadCoordinate{TID: 42, TGID: 40, Comm: "query alias"}
	w := TraceRunnablePriorityOverlap{SourcePath: "/trace/a.ftrace", Ready: ready,
		Running: TraceThreadCoordinate{TID: 43, TGID: 40, Comm: "running"}, CPU: 0,
		StartTs: 0, EndTs: .002, ReadyStartLine: 15, ReadyEndLine: 21, RunningStartLine: 13, RunningEndLine: 21}
	if got := w.ForRankMember("/trace/a.ftrace", TraceThreadCoordinate{TID: 42, TGID: 40, Comm: "native alias"}, 0, .003); got == nil || got == &w || got.CPU != 0 {
		t.Fatalf("zero-origin exact witness did not survive the alias-safe bind: %+v", got)
	}
	for _, v := range []struct {
		path       string
		member     TraceThreadCoordinate
		start, end float64
	}{
		{"/trace/b.ftrace", ready, 0, .003},
		{"/trace/a.ftrace", TraceThreadCoordinate{TID: 43, TGID: 40}, 0, .003},
		{"/trace/a.ftrace", TraceThreadCoordinate{TID: 42, TGID: 41}, 0, .003},
		{"/trace/a.ftrace", ready, .001, .003},
		{"/trace/a.ftrace", ready, 0, .001},
		{"/trace/a.ftrace", ready, 0, math.Inf(1)},
	} {
		if got := w.ForRankMember(v.path, v.member, v.start, v.end); got != nil {
			t.Errorf("witness escaped its physical member/source/interval: %+v", v)
		}
	}
	for _, corrupt := range []func(*TraceRunnablePriorityOverlap){
		func(w *TraceRunnablePriorityOverlap) { w.Running.TID = w.Ready.TID },
		func(w *TraceRunnablePriorityOverlap) { w.CPU = -1 },
		func(w *TraceRunnablePriorityOverlap) { w.EndTs = math.NaN() },
		func(w *TraceRunnablePriorityOverlap) { w.ReadyEndLine = w.ReadyStartLine },
	} {
		other := w
		corrupt(&other)
		if other.ReaderDescription(true) != "" || other.ReaderDescription(false) != "" {
			t.Errorf("invalid witness was rendered: %+v", other)
		}
	}
}
