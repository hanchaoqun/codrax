package skill

import (
	"strings"
	"testing"
)

func TestTraceIOActivityTeachingUsesExistingComposableView(t *testing.T) {
	var windowStats string
	for _, row := range TraceQueryViewTeachings() {
		if row.View == "window_stats" {
			windowStats = row.When
		} else if strings.Contains(row.When, TraceIOActivityTeaching) {
			t.Fatalf("IO activity teaching duplicated into %q", row.View)
		}
		if row.View == "io_activity" {
			t.Fatal("an output section must not become a callable view")
		}
	}
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"window view": windowStats,
		"matrix":      RenderTraceQueryViewMatrix(),
		"explorer":    allWorkflowBodies(sk),
	} {
		if count := strings.Count(text, TraceIOActivityTeaching); count != 1 {
			t.Errorf("%s must reuse exactly one shared activity contract, got %d", name, count)
		}
	}
}

func TestTraceIOActivityTeachingKeepsDistinctMeasurementPopulations(t *testing.T) {
	for _, want := range []string{
		`view="window_stats", section io_activity (not a new view)`,
		"independently admitted endpoint events, including unpaired endpoints",
		"io_inflight occupancy and request latency require complete pairs",
		"source/layer/endpoint family/phase/device/byte caliber",
		"observed event counts are not unique request counts",
		"RQ/BIO/filesystem layers must not be added",
		"known-byte denominators, missing-size counts and coverage",
		"full selected time window",
		"Explicit positive-width windows are half-open",
		"capture endpoint is included only as disclosed by end_inclusive",
		"line-only and point selections have no rate denominator",
		"idle buckets and the actual short-tail width",
		"Line bounds take precedence",
		"unavailable, not zero; known zero is valid",
		"size alone does not prove random/sequential",
		"not target blocking time or root-cause evidence",
	} {
		if !strings.Contains(TraceIOActivityTeaching, want) {
			t.Errorf("activity teaching lost %q", want)
		}
	}
}
