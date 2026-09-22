package types

import "testing"

func TestJankSourceClockInventoryStates(t *testing.T) {
	for _, tc := range []struct {
		state string
		valid bool
	}{
		{"source_trace_clock", true}, {"unverified", true}, {"aligned", false}, {"", false}, {"UTC", false},
	} {
		t.Run(tc.state, func(t *testing.T) {
			r := testTraceEventSearchInventoryRecord()
			r.EventSearchInventory.Rows[0].JankEvent.TimeDomainStatus = tc.state
			if got := IsValidTraceEventSearchInventoryRecord(r); got != tc.valid {
				t.Fatalf("state %q validity=%v want=%v", tc.state, got, tc.valid)
			}
			clone := CloneTraceEventSearchInventory(r.EventSearchInventory)
			if clone.Rows[0].JankEvent.TimeDomainStatus != tc.state {
				t.Fatal("legacy or invalid clock silently upgraded")
			}
		})
	}
}
