package types

import (
	"encoding/json"
	"math"
	"testing"
)

func TestProjectionStateOccupancyCarriesPublishedZeroAndRejectsMissingOrMalformed(t *testing.T) {
	for _, tc := range []struct {
		name, state, note string
		value             float64
		known             bool
	}{
		{"running", "running", "running=8.294", 8.294, true},
		{"runnable", "runnable", "runnable=0.109", .109, true},
		{"sleep", "s_sleep", "sleep=3.324", 3.324, true},
		{"D", "d_state", "d_state=3.598", 3.598, true},
		{"IO", "io_wait", "io_wait=1.759", 1.759, true},
		{"zero", "running", "running=0", 0, true},
		{"missing", "running", "runnable=3", 0, false},
		{"negative", "running", "running=-1", 0, false},
		{"nan", "running", "running=NaN", 0, false},
		{"infinity", "running", "running=+Inf", 0, false},
		{"ms_unit", "running", "running=8.294ms", 8.294, true},
		{"zero_ms_unit", "running", "running=0ms", 0, true},
		{"prose", "running", "running=8.294ms extra", 0, false},
		{"unknown", "future_state", "running=1", 0, false},
		{"device_latency", "io_latency", "io_wait=1", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := traceCausalProjectionNodeFromRecord("on_chain", ObservationRecord{Unit: "ms", Value: "7.405", RichNotes: []string{"dominant_state=" + tc.state, tc.note, "effective_impact_ms=7.405", "cumulative_impact_ms=19"}})
			value, known := node.PublishedStateOccupancy()
			if known != tc.known || (known && math.Abs(value-tc.value) > 1e-9) {
				t.Fatalf("got (%v,%v), want (%v,%v)", value, known, tc.value, tc.known)
			}
			if !known {
				return
			}
			encoded, err := json.Marshal(node)
			if err != nil {
				t.Fatal(err)
			}
			var restored TraceCausalProjectionNode
			if err := json.Unmarshal(encoded, &restored); err != nil {
				t.Fatal(err)
			}
			if got, ok := restored.PublishedStateOccupancy(); !ok || got != value {
				t.Fatalf("JSON roundtrip lost measurement: (%v,%v)", got, ok)
			}
			copy := node
			copy.RunningMS = 99
			if got, _ := node.PublishedStateOccupancy(); got != value {
				t.Fatal("value copy aliased original state measurement")
			}
			node.MergedCount = 2
			if _, ok := node.PublishedStateOccupancy(); ok {
				t.Fatal("retained seed measurement cannot describe a folded population")
			}
		})
	}
}

func TestProjectionMixedDIORequiresBothOriginalPartitionMembers(t *testing.T) {
	for _, tc := range []struct {
		note  string
		want  float64
		known bool
	}{
		{"io_wait=1.2", 4.8, true}, {"io_wait=0", 3.6, true}, {"sleep=1.2", 0, false},
	} {
		node := traceCausalProjectionNodeFromRecord("on_chain", ObservationRecord{Unit: "ms", Value: "12", RichNotes: []string{"dominant_state=d_state_or_io_wait", "d_state=3.6", tc.note}})
		value, known := node.PublishedStateOccupancy()
		if known != tc.known || (known && math.Abs(value-tc.want) > 1e-9) {
			t.Fatalf("%s: got (%v,%v), want (%v,%v)", tc.note, value, known, tc.want, tc.known)
		}
	}
}
