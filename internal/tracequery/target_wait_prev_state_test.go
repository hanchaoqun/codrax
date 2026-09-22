package tracequery

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTargetWaitPrevStateCopiesOnlyNativeOccurrenceMetadata(t *testing.T) {
	intervals := []Interval{
		{State: StateIOWait, PrevStateRaw: "D", StartTs: 1, EndTs: 1.001, DurationMs: 1, BlockedReasonIOWaitKnown: true, BlockedReasonIOWait: 1},
		{State: StateDSleep, PrevStateRaw: "D|K", StartTs: 2, EndTs: 2.002, DurationMs: 2},
		{State: StateSSleep, PrevStateRaw: "S", StartTs: 3, EndTs: 3.003, DurationMs: 3, BlockedReasonIOWaitKnown: true, BlockedReasonIOWait: 1},
		{State: StateIOWait, StartTs: 4, EndTs: 4.004, DurationMs: 4, BlockedReasonIOWaitKnown: true, BlockedReasonIOWait: 1},
		{State: StateSSleep, PrevStateRaw: "I", StartTs: 5, EndTs: 5.005, DurationMs: 5, BlockedReasonIOWaitKnown: true, BlockedReasonIOWait: 1},
		{State: StateRunnable, PrevStateRaw: "D", StartTs: 6, EndTs: 6.006, DurationMs: 6},
	}
	before := append([]Interval(nil), intervals...)
	rows := targetWindowWaitOccurrences(intervals)
	if len(rows) != 5 || !reflect.DeepEqual(before, intervals) {
		t.Fatal("metadata transport changed roster admission or native input")
	}
	for i, want := range []string{"D", "D|K", "S", "", "I"} {
		data, _ := json.Marshal(rows[i])
		var fields map[string]any
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		got, present := fields["prev_state_raw"]
		if (want != "" && got != want) || (want == "" && present) {
			t.Errorf("row %d native raw state lost or inferred: json=%s want=%q", i, data, want)
		}
		if rows[i].State != intervals[i].State || rows[i].DurationMs != intervals[i].DurationMs || rows[i].Ordinal != i+1 {
			t.Fatal("transport changed category, duration or ordinal")
		}
	}
}
