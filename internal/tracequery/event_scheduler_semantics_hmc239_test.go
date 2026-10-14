package tracequery

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func schedulerField239(t *testing.T, value *types.TraceEventSemantics, key string) types.TraceEventSemanticField {
	t.Helper()
	if value != nil {
		for _, field := range value.Fields {
			if field.Key == key {
				return field
			}
		}
	}
	t.Fatalf("missing %s: %+v", key, value)
	return types.TraceEventSemanticField{}
}

func TestSchedulerIdentityTypedProjection239(t *testing.T) {
	event := Event{Type: EventSchedWakeup, PID: 11, TGID: 10, Comm: "record", WakeePID: 42, WakeeComm: "target"}
	before := event
	got := ProjectTraceEventSemantics(event)
	if !types.ValidateTraceEventSemantics(got) || got == nil {
		t.Fatal("raw-free typed wake event has no display semantics")
	}
	for key, want := range map[string]string{"scheduler.wakee.comm": "target", "scheduler.wakee.tid": "42"} {
		f := schedulerField239(t, got, key)
		if f.Status != "known" || f.Value == nil || *f.Value != want {
			t.Fatalf("%s: %+v", key, f)
		}
	}
	if f := schedulerField239(t, got, "scheduler.wakee.tgid"); f.Status != "unavailable" || f.Value != nil {
		t.Fatal("unrelated emitter TGID leaked to wakee")
	}
	if !reflect.DeepEqual(before, event) {
		t.Fatal("display mutated native event")
	}
	for _, kind := range []EventType{EventSchedWakeup, EventSchedWaking} {
		event.Type, event.WakeePID = kind, 11
		if f := schedulerField239(t, ProjectTraceEventSemantics(event), "scheduler.wakee.tgid"); f.Value == nil || *f.Value != "10" {
			t.Fatalf("same-row exact thread lost independently known TGID: %+v", f)
		}
	}
	event.WakeeComm, event.TGID = "", 0
	got = ProjectTraceEventSemantics(event)
	if f := schedulerField239(t, got, "scheduler.wakee.tid"); f.Value == nil || *f.Value != "11" {
		t.Fatal("missing name discarded exact TID")
	}
	if f := schedulerField239(t, got, "scheduler.wakee.tgid"); f.Status != "unavailable" {
		t.Fatal("missing TGID became main-thread process")
	}
	event.TGID = 10
	event.Type, event.PrevPID, event.PrevComm, event.NextPID, event.NextComm = EventSchedSwitch, 11, "record", 42, "target"
	got = ProjectTraceEventSemantics(event)
	for key, want := range map[string]string{"scheduler.prev.tid": "11", "scheduler.prev.tgid": "10", "scheduler.next.tid": "42"} {
		if f := schedulerField239(t, got, key); f.Value == nil || *f.Value != want {
			t.Fatalf("%s: %+v", key, f)
		}
	}
	if f := schedulerField239(t, got, "scheduler.next.tgid"); f.Status != "unavailable" {
		t.Fatal("next TGID inferred")
	}
	// No presence bit survives in the historical Event transport for endpoint
	// TID zero. The display cannot distinguish idle zero from a default zero.
	for _, value := range []int{0, -1, 2147483648} {
		event.NextPID = value
		if f := schedulerField239(t, ProjectTraceEventSemantics(event), "scheduler.next.tid"); f.Status != "unavailable" || f.Value != nil {
			t.Fatalf("unproven endpoint acquired TID: %+v", f)
		}
	}
	event.Type = EventUnknown
	event.FieldText = "sched_wakeup: comm=target pid=42"
	if got := ProjectTraceEventSemantics(event); got != nil {
		t.Fatal("raw resurrected rejected scheduler identity")
	}
}

func TestSchedulerIdentityParserPresenceBoundary239(t *testing.T) {
	valid := "worker-11 (10) [000] .... 1.001000: sched_switch: prev_comm=worker prev_pid=11 prev_prio=120 prev_state=S ==> next_comm=swapper next_pid=0 next_prio=120"
	event, ok := ParseLine(1, valid, newStringInterner())
	if !ok || event.Type != EventSchedSwitch || event.NextPID != 0 {
		t.Fatal("fixture is not a genuine admitted idle endpoint")
	}
	if f := schedulerField239(t, ProjectTraceEventSemantics(event), "scheduler.next.tid"); f.Status != "unavailable" {
		t.Fatal("transient parser presence revived through an unmarked Event zero")
	}
	for _, line := range []string{
		"worker-11 (10) [000] .... 1.001000: sched_switch: prev_comm=worker prev_pid=11 prev_prio=120 prev_state=S ==> next_comm=target next_prio=120",
		"worker-11 (10) [000] .... 1.001000: sched_wakeup: comm=target pid=42 pid=43 prio=120 target_cpu=0",
	} {
		if bad, accepted := ParseLine(1, line, newStringInterner()); accepted && ProjectTraceEventSemantics(bad) != nil {
			t.Fatal("invalid or conflicting identity acquired endpoint fields")
		}
	}
}
