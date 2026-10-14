package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSchedulerIdentityRegistryAndAuthority239(t *testing.T) {
	keys := []string{"scheduler.wakee.comm", "scheduler.wakee.tid", "scheduler.wakee.tgid", "scheduler.prev.comm", "scheduler.prev.tid", "scheduler.prev.tgid", "scheduler.next.comm", "scheduler.next.tid", "scheduler.next.tgid"}
	var registered []string
	for _, d := range traceEventSemanticDescriptors {
		if d.Family == "scheduler" {
			registered = append(registered, d.Key)
			if d.Unit != "" || d.Label == "" {
				t.Fatalf("incomplete endpoint descriptor: %+v", d)
			}
		}
	}
	if !reflect.DeepEqual(keys, registered) {
		t.Fatalf("scheduler display field census changed: %v", registered)
	}
	for _, key := range keys {
		d, _ := LookupTraceEventSemanticDescriptor(key)
		f := TraceEventSemanticField{Key: key, Type: d.Type, Status: "known", Value: semanticTestValue("42")}
		s := &TraceEventSemantics{SchemaVersion: 1, Fields: []TraceEventSemanticField{f}}
		if !ValidateTraceEventSemantics(s) {
			t.Fatal("valid identity display rejected")
		}
		for _, eventType := range []string{"sched_wakeup", "sched_waking", "sched_switch"} {
			want := strings.HasPrefix(key, "scheduler.wakee.") == (eventType != "sched_switch")
			if traceEventSemanticsMatchEventType(s, eventType) != want {
				t.Fatalf("%s role qualification changed for %s", key, eventType)
			}
		}
		for _, unrelated := range []string{"trace_mark", "measure_interval", "process_measure_interval", "perf_sample", "unknown"} {
			if traceEventSemanticsMatchEventType(s, unrelated) {
				t.Fatalf("%s acquired %s", unrelated, key)
			}
		}
		if d.Type == "int64" {
			for _, bad := range []string{"0", "-1", "2147483648", "42.0", "042", "+42"} {
				s.Fields[0].Value = semanticTestValue(bad)
				if ValidateTraceEventSemantics(s) {
					t.Fatalf("%s accepted %q", key, bad)
				}
			}
		}
	}
	r := testTraceEventSearchInventoryRecord()
	r.EventSearchInventory.Rows[0].EventType = "sched_wakeup"
	r.EventSearchInventory.Rows[0].Semantics = &TraceEventSemantics{SchemaVersion: 1, Fields: []TraceEventSemanticField{{Key: keys[1], Type: "int64", Status: "known", Value: semanticTestValue("42")}}}
	if !IsValidTraceEventSearchInventoryRecord(r) {
		t.Fatal("scheduler inventory rejected")
	}
	before := CompileTraceCausalProjectionSet(ObservationLedger{Records: []ObservationRecord{r}})
	clone := CloneTraceEventSearchInventory(r.EventSearchInventory)
	*clone.Rows[0].Semantics.Fields[0].Value = "99"
	if *r.EventSearchInventory.Rows[0].Semantics.Fields[0].Value != "42" {
		t.Fatal("clone aliased fields")
	}
	serialized, _ := json.Marshal(r)
	var restored ObservationRecord
	if err := json.Unmarshal(serialized, &restored); err != nil || !IsValidTraceEventSearchInventoryRecord(restored) {
		t.Fatalf("persistence lost precise fields: %v", err)
	}
	r.EventSearchInventory.Rows[0].Semantics = nil
	if !reflect.DeepEqual(before, CompileTraceCausalProjectionSet(ObservationLedger{Records: []ObservationRecord{r}})) {
		t.Fatal("display fields became causal authority")
	}
}
