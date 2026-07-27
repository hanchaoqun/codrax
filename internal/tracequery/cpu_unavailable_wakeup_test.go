package tracequery

import (
	"strings"
	"testing"
)

func TestCPUUnavailableWakeupWireRoundTrip(t *testing.T) {
	wakeup := CPUUnavailableWakeup{
		TimestampNS:    1_234_567_890,
		EventName:      "sched_wakeup",
		WakerTID:       200,
		WakerTGID:      201,
		WakeeTID:       100,
		TargetCPU:      7,
		Priority:       42,
		PrioritySource: WakeePrioritySourceInferredNextSchedSlice,
		WakerComm:      "waker 空格",
		WakeeComm:      "app",
		Reason:         SchedulerEmitterCPUReasonUnknown,
	}
	line, err := FormatCPUUnavailableWakeup(wakeup)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(line, "[000]") || !strings.HasPrefix(line, cpuUnavailableWakeupPrefix+" ") {
		t.Fatalf("wire row fabricated a CPU or lost its exact prefix: %q", line)
	}
	if ts, ok := ParseLineTimestampNS(line); !ok || ts != wakeup.TimestampNS {
		t.Fatalf("timestamp parse=(%d,%t), want (%d,true)", ts, ok, wakeup.TimestampNS)
	}
	event, ok := ParseLine(9, line, newStringInterner())
	if !ok || event.Line != 9 || event.Type != EventSchedWakeup ||
		event.Name != wakeup.EventName || event.CPU != -1 ||
		event.PID != wakeup.WakerTID || event.TGID != wakeup.WakerTGID ||
		event.Comm != wakeup.WakerComm || event.WakeePID != wakeup.WakeeTID ||
		event.WakeeComm != wakeup.WakeeComm || event.WakeePrio != wakeup.Priority ||
		event.WakeePrioritySource() != WakeePrioritySourceInferredNextSchedSlice ||
		event.TargetCPU != wakeup.TargetCPU || !event.TargetCPUValid ||
		event.PluginFields == nil ||
		event.PluginFields.SchedulerEmitterCPUStatus != SchedulerEmitterCPUStatusUnavailable ||
		event.PluginFields.SchedulerEmitterCPUReason != wakeup.Reason {
		t.Fatalf("typed wakeup round-trip drifted: %+v", event)
	}
}

func TestCPUUnavailableWakeupUnknownPriorityAndClosedWire(t *testing.T) {
	base, err := FormatCPUUnavailableWakeup(CPUUnavailableWakeup{
		TimestampNS:    10,
		EventName:      "sched_waking",
		WakerTID:       20,
		WakerTGID:      21,
		WakeeTID:       30,
		TargetCPU:      0,
		PrioritySource: WakeePrioritySourceUnknown,
		WakerComm:      "waker",
		WakeeComm:      "wakee",
		Reason:         SchedulerEmitterCPUReasonSourceTainted,
	})
	if err != nil {
		t.Fatal(err)
	}
	event, ok := ParseLine(1, base, newStringInterner())
	if !ok || event.Type != EventSchedWaking || event.CPU != -1 ||
		event.WakeePrio != 0 || event.WakeePrioritySource() != WakeePrioritySourceUnknown ||
		event.PluginFields == nil ||
		event.PluginFields.SchedulerEmitterCPUReason != SchedulerEmitterCPUReasonSourceTainted {
		t.Fatalf("unknown-priority wakeup drifted: %+v", event)
	}
	for _, line := range []string{
		strings.Replace(base, "priority=~", "priority=42", 1),
		strings.Replace(base, "event=sched_waking", "event=future_wakeup", 1),
		strings.Replace(base, "target_cpu=0", "target_cpu=4096", 1),
		strings.Replace(base, "reason="+SchedulerEmitterCPUReasonSourceTainted, "reason=future", 1),
		strings.Replace(base, " waker_tid=20", "  waker_tid=20", 1),
		base + " extra=x",
	} {
		if _, ok := ParseLine(1, line, newStringInterner()); ok {
			t.Fatalf("accepted non-canonical CPU-unavailable wakeup: %q", line)
		}
	}
}

func TestCPUUnavailableWakeupRemainsAvailableToCausalChain(t *testing.T) {
	line, err := FormatCPUUnavailableWakeup(CPUUnavailableWakeup{
		TimestampNS:    5_005_000_000,
		EventName:      "sched_wakeup",
		WakerTID:       200,
		WakerTGID:      200,
		WakeeTID:       100,
		TargetCPU:      1,
		PrioritySource: WakeePrioritySourceUnknown,
		WakerComm:      "worker",
		WakeeComm:      "app",
		Reason:         SchedulerEmitterCPUReasonUnknown,
	})
	if err != nil {
		t.Fatal(err)
	}
	trace := `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.000100: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
` + line + `
     worker-200 (200) [002] .... 5.005100: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.006000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
        app-100 (100) [001] .... 5.007000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`
	index := buildTraceIndex(t, "cpu-unavailable-wakeup.systrace", trace)
	chain := BuildWakeupChain(index, Query{
		PID: 100, TimeStart: 5.0, TimeEnd: 5.007, MaxDepth: 4,
		MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace,
	})
	for _, edge := range chain.Edges {
		if edge.Waker.PID == 200 && edge.Wakee.PID == 100 &&
			edge.WakeePrioritySource == WakeePrioritySourceUnknown {
			return
		}
	}
	t.Fatalf("CPU-unavailable wakeup was absent from the causal chain: %+v", chain)
}
