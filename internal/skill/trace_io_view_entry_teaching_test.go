package skill

import (
	"strings"
	"testing"
)

func TestTraceIODistributionTeachingNamesCallableParentView(t *testing.T) {
	windowStats := ""
	for _, row := range TraceQueryViewTeachings() {
		if row.View == "window_stats" {
			windowStats = row.When
		}
	}
	for name, text := range map[string]string{
		"shared IO teaching":   TraceIORequestLatencyDistributionTeaching,
		"parent view row":      windowStats,
		"rendered view matrix": RenderTraceQueryViewMatrix(),
	} {
		for _, want := range []string{`call trace_query with view="window_stats"`, "window_stats.storage_latency_by_layer[].request_latency_distribution", "output section, not a view"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s lacks callable-view/output-section boundary %q", name, want)
			}
		}
	}
}
