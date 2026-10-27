package tracequery

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC299WakeupMeasuredWaitNativeTolerance(t *testing.T) {
	idx := buildTraceIndex(t, "boundary.systrace", `app-100 (100) [001] .... 1.020000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
worker-200 (100) [002] .... 1.030001: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
app-100 (100) [001] .... 1.030020: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 1.020, TimeEnd: 1.030, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	if len(chain.Edges) != 1 {
		t.Fatalf("original edge lost: %+v", chain)
	}
	e := chain.Edges[0]
	w := e.MeasuredWait
	if w == nil || w.StartTs != 1.020 || w.EndTs != 1.030001 || w.SegmentEndTs != 1.030 || w.SegmentState != "s_sleep" || e.LatencyMs != (e.WakeupTs-w.StartTs)*1000 {
		t.Fatalf("native segment operands lost: %+v / %+v", e, w)
	}
	display := types.TraceWakeupMeasuredWaitDisplay(w)
	for _, want := range []string{"measured_wait=[1.020000000,1.030001000)s", "selected_state_interval=[1.020000000,1.030000000)s", "wakeup_matches_selected_state_end=false"} {
		if !strings.Contains(display, want) {
			t.Errorf("tolerance tail mislabeled as scheduler state: missing %q in %s", want, display)
		}
	}
}

func TestHMC299WakeupMeasuredWaitUnknownIsNotZeroOrState(t *testing.T) {
	for _, state := range []ThreadState{StateRunning, StateRunnable, StateStopped, StateDead, StateUnknown} {
		if got := wakeupMeasuredWait(Interval{State: state, StartTs: 1, EndTs: 2, StartLine: 3}, 2); got != nil {
			t.Errorf("nonblocking segment %s became wait: %+v", state, got)
		}
	}
	if got := wakeupMeasuredWait(Interval{State: StateSSleep, StartTs: 1, EndTs: 2}, 2); got != nil {
		t.Fatal("missing start evidence became exact measured interval")
	}
	w := wakeupMeasuredWait(Interval{State: StateSSleep, StartTs: 0, EndTs: 2, StartLine: 3}, 2)
	if w == nil || w.ActualStartTs != nil || !strings.Contains(types.TraceWakeupMeasuredWaitDisplay(w), "actual_segment_start=unknown") {
		t.Fatalf("unknown actual start borrowed selected zero: %+v", w)
	}
}
