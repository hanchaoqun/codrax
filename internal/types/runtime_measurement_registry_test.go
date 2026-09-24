package types

import (
	"encoding/json"
	"testing"
)

func TestRuntimeMeasurementPredicateViewRegistration(t *testing.T) {
	for _, predicate := range []string{"io_activity", "io_inflight", "scheduler_concurrency", "scheduler_concurrency_coverage", "io_activity_coverage", "model_measurement", ""} {
		for _, view := range []RuntimeMeasurementView{RuntimeMeasurementSummary, RuntimeMeasurementMembers, RuntimeMeasurementTimeline, RuntimeMeasurementDistribution, "invented"} {
			t.Run(predicate+"/"+string(view), func(t *testing.T) {
				r := measurementProviderRecord(t)
				p, ok := DecodeRuntimeMeasurementPublication(r)
				if !ok {
					t.Fatal("broken baseline")
				}
				r.Predicate = predicate
				p.Tables[0].View = view
				data, _ := json.Marshal(p)
				r.RichNotes = []string{TraceNoteKeyRuntimeMeasurement + "=" + string(data)}
				want := (predicate == "io_activity" || predicate == "io_inflight") && (view == RuntimeMeasurementSummary || view == RuntimeMeasurementTimeline) ||
					predicate == "io_activity" && view == RuntimeMeasurementDistribution || predicate == "io_inflight" && view == RuntimeMeasurementMembers ||
					predicate == "scheduler_concurrency" && (view == RuntimeMeasurementSummary || view == RuntimeMeasurementMembers || view == RuntimeMeasurementDistribution || view == RuntimeMeasurementTimeline)
				if _, got := DecodeRuntimeMeasurementPublication(r); got != want {
					t.Fatalf("registered predicate/view accepted=%v, want %v", got, want)
				}
				if got := BuildRuntimeMeasurementContract(measurementProviderInput(r)).Active(); got != want {
					t.Fatalf("active contract=%v, want %v", got, want)
				}
			})
		}
	}
}
