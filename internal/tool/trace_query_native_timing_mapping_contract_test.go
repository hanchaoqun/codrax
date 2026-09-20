package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// This is a source-identity mapping, not the cross-thread aggregate family:
// three admitted types are per-thread, while aggregate pressure/supply and
// irq_burst are excluded. Pin every type/source pair against the full shared
// token universe so registering this distinct dispatch in UXG-1 cannot widen
// duration publication to the larger aggregate family.
func TestNativeTimingProducerMappingClosedPairs(t *testing.T) {
	want := map[string]string{
		"io_latency":         "window_stats",
		"irq_activity":       "window_stats.irq_activity",
		"ipi_activity":       "window_stats.ipi_activity",
		"workqueue_activity": "window_stats.workqueue_activity",
		"dma_fence_activity": "window_stats.dma_fence_activity",
	}
	tokens := append(tracequery.CausalTokenUniverse(), "", "unknown_native_duration")
	sources := []string{"", "unknown_source", "window_stats.io_facet_family"}
	for token, source := range want {
		tokens = append(tokens, " "+token, token+" ", strings.ToUpper(token))
		sources = append(sources, source, " "+source, source+" ", strings.ToUpper(source))
	}
	accepted := 0
	for _, token := range tokens {
		for _, source := range sources {
			measured, ok := traceQueryBackgroundNativeDurationMeasurement(tracequery.RootCauseRankItem{
				Type: token, Source: source, CumulativeImpactMs: 23,
			})
			expectedSource, registered := want[token]
			expected := registered && source == expectedSource
			if ok != expected || (ok && measured != 23) {
				t.Errorf("type=%q source=%q: got measured=%v ok=%t, want accepted=%t", token, source, measured, ok, expected)
			}
			if ok {
				accepted++
			}
		}
	}
	if accepted != len(want) {
		t.Fatalf("closed producer mapping admitted %d pairs, want exactly %d", accepted, len(want))
	}
}
