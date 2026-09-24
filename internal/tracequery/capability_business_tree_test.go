package tracequery

import (
	"strings"
	"testing"
)

func TestBusinessTreeCapabilityKeepsInstanceAndMeasurementBoundaries(t *testing.T) {
	catalog, err := TraceCapabilities("window_stats", true)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, metric := range catalog.Metrics {
		if metric.ID != "business_tree" {
			continue
		}
		count++
		text := strings.Join(metric.Requirements.Conditions, " ") + " " + strings.Join(metric.Limitations, " ")
		for _, want := range []string{"same-physical-source/emitter-thread B/E stack", "unclosed/invalidated", "missing ancestors", "inclusive costs are not additive", "async intervals", "Unknown scheduler time", "root cause"} {
			if !strings.Contains(text, want) {
				t.Errorf("business tree capability lost %q: %s", want, text)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected one business-tree metric under window_stats, got %d", count)
	}
	span, err := TraceCapabilities("span_window", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, metric := range span.Metrics {
		if metric.ID == "business_tree" {
			t.Fatal("span-window locator must not advertise a tree it does not produce")
		}
	}
}

func TestBusinessTreeCapacityIsIndependentFromLegacySpans(t *testing.T) {
	if TraceMarkerTreeNodeLimit != 32 || TraceMarkerTreeSegmentLimit != 16 {
		t.Fatal("tree display budgets require explicit review")
	}
	if spanWindowFloorLimit != 8 || sharedDefaultResultLimit != 40 {
		t.Fatal("tree must not alter established query/locator budgets")
	}
}
