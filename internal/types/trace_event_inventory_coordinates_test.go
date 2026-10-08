package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTraceEventInventoryCoordinatesValidationCloneAndLegacy(t *testing.T) {
	ptr := func(value bool) *bool { return &value }
	good := testTraceEventSearchInventoryRecord()
	r := &good.EventSearchInventory.Rows[0]
	r.CPU, r.CPUKnown = 0, ptr(true)
	r.EmitterTID, r.EmitterTIDKnown = 0, ptr(true)
	r.EmitterTGID, r.EmitterTGIDKnown = -1, ptr(false)
	if !IsValidTraceEventSearchInventoryRecord(good) {
		t.Fatal("real CPU0/idle TID0 rejected")
	}
	for name, mutate := range map[string]func(*TraceEventSearchInventoryRow){
		"unknown CPU retained zero":  func(r *TraceEventSearchInventoryRow) { r.CPUKnown = ptr(false); r.CPUUnknownReason = "not_recorded" },
		"known CPU negative":         func(r *TraceEventSearchInventoryRow) { r.CPU = -1 },
		"known CPU has reason":       func(r *TraceEventSearchInventoryRow) { r.CPUUnknownReason = "not_recorded" },
		"unknown CPU missing reason": func(r *TraceEventSearchInventoryRow) { r.CPU, r.CPUKnown = -1, ptr(false) },
		"unknown CPU prose reason": func(r *TraceEventSearchInventoryRow) {
			r.CPU, r.CPUKnown, r.CPUUnknownReason = -1, ptr(false), "looks unknown"
		},
		"legacy CPU upgraded reason": func(r *TraceEventSearchInventoryRow) { r.CPUKnown = nil; r.CPUUnknownReason = "not_recorded" },
		"unknown TID retained idle":  func(r *TraceEventSearchInventoryRow) { r.EmitterTIDKnown = ptr(false) },
		"known TID negative":         func(r *TraceEventSearchInventoryRow) { r.EmitterTID = -1 },
		"unknown TGID retained zero": func(r *TraceEventSearchInventoryRow) { r.EmitterTGID = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := good
			bad.EventSearchInventory = CloneTraceEventSearchInventory(good.EventSearchInventory)
			mutate(&bad.EventSearchInventory.Rows[0])
			if IsValidTraceEventSearchInventoryRecord(bad) {
				t.Fatal("contradictory coordinates admitted")
			}
		})
	}
	clone := CloneTraceEventSearchInventory(good.EventSearchInventory)
	*clone.Rows[0].CPUKnown = false
	*clone.Rows[0].EmitterTIDKnown = false
	*clone.Rows[0].EmitterTGIDKnown = true
	if !*r.CPUKnown || !*r.EmitterTIDKnown || *r.EmitterTGIDKnown {
		t.Fatal("known bits alias producer")
	}
	r.CPU, r.CPUKnown, r.CPUUnknownReason = -1, ptr(false), "no_trusted_running_slice_at_start"
	encoded, err := json.Marshal(good)
	var restored ObservationRecord
	if err != nil || json.Unmarshal(encoded, &restored) != nil || !IsValidTraceEventSearchInventoryRecord(restored) || !reflect.DeepEqual(good, restored) {
		t.Fatal("known/unknown JSON roundtrip changed values")
	}
	legacy := testTraceEventSearchInventoryRecord()
	legacy.EventSearchInventory.Rows[0].CPU = -1
	encoded, _ = json.Marshal(legacy)
	restored = ObservationRecord{}
	if json.Unmarshal(encoded, &restored) != nil || !IsValidTraceEventSearchInventoryRecord(restored) || restored.EventSearchInventory.Rows[0].CPUKnown != nil {
		t.Fatal("legacy unknown upgraded or rejected")
	}
}
