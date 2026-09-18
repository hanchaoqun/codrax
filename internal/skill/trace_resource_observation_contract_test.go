package skill

import (
	"strings"
	"testing"
)

// These are teaching pins, not runtime scans of user requests or model answers.
// The observed failure was interpretation of intact resource metadata as a
// lifetime, release proof, or a count mixing operation and counter rows.
func TestTraceResourceObservationTeachingContract(t *testing.T) {
	var teaching string
	for _, row := range TraceQueryViewTeachings() {
		if row.View == "event_search" {
			teaching = row.When
		}
	}
	for _, want := range []string{
		"Native resource observation contract:",
		"I markers are resource observations, not execution spans",
		"Keep I resource-operation row counts separate from C counter-observation counts",
		"only report counts supplied by deterministic evidence",
		"per-event source quantity",
		"HeapSize and MmapSize are independent cumulative snapshot series",
		"not per-operation increments or decrements",
		"0/-1 are not assigned stack or function meanings",
		"NULL/0 do not prove release",
		"MaxInt64 and other positive values are not presumed sentinels",
		"do not subtract them to claim an execution duration",
		"Preserve decimal integers exactly",
		"do not infer a floating-point encoding",
		"Unresolved callchain keys do not identify functions",
		"resource lifetime is not execution time",
	} {
		if !strings.Contains(teaching, want) {
			t.Errorf("resource observation teaching missing %q", want)
		}
	}
}

func TestTraceResourceObservationTeachingUsesSharedContract(t *testing.T) {
	for _, corpus := range []string{RenderTraceQueryViewMatrix(), func() string {
		var out strings.Builder
		for _, row := range TraceQueryViewTeachings() {
			out.WriteString(row.When)
		}
		return out.String()
	}()} {
		if count := strings.Count(corpus, TraceResourceObservationContract); count != 1 {
			t.Errorf("shared resource contract must be emitted once per teaching table, got %d", count)
		}
	}
}
