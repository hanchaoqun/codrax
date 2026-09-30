package types

import "testing"

func TestRuntimeThreadLookupSnapshotDoesNotMutateAuthority(t *testing.T) {
	rm := RequestModel{RuntimeThreadLookups: []RuntimeThreadLookup{{PID: 10, SourceQuote: "thread 10"}}}
	m := NewMutableState("thread 10")
	m.SetRequestModel(rm)
	rm.RuntimeThreadLookups[0].PID = 11
	copy := m.RequestModel()
	if copy.RuntimeThreadLookups[0].PID != 10 {
		t.Fatal("set retained mutable authority slice")
	}
	copy.RuntimeThreadLookups[0].PID = 12
	if m.RequestModel().RuntimeThreadLookups[0].PID != 10 {
		t.Fatal("snapshot mutated stored authority")
	}
	if len(m.RequestModel().RuntimeTargets) != 0 {
		t.Fatal("lookup gained focus authority")
	}
}
