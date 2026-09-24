package tracequery

import (
	"strings"
	"testing"
)

func TestIOActivityCapabilityKeepsEndpointAndPairContractsSeparate(t *testing.T) {
	for _, view := range []string{"window_stats", "evidence_pack", "frame_root_cause_bundle"} {
		catalog, err := TraceCapabilities(view, true)
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]MetricCapability{}
		for _, metric := range catalog.Metrics {
			found[metric.ID] = metric
		}
		for _, id := range []string{"io_activity", "io_inflight", "io_request_latency"} {
			if _, ok := found[id]; !ok {
				t.Errorf("%s must retain composable %s contract", view, id)
			}
		}
		metric := found["io_activity"]
		text := metric.Summary + " " + strings.Join(metric.Requirements.Conditions, " ") + " " + strings.Join(metric.Limitations, " ")
		for _, want := range []string{"independently of pairing", "RQ/BIO", "full-wire MMC", "six existing strict F2FS", "query_pid is context, not a filter", "line bounds take precedence", "not unique logical request counts", "known-size subset", "Size does not prove random/sequential", "No causal"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s endpoint contract lost %q", view, want)
			}
		}
		for _, output := range metric.Outputs {
			if !strings.HasPrefix(output.Section, "window_stats.io_activity") {
				t.Errorf("endpoint metric borrowed another population: %+v", output)
			}
		}
	}
	if _, err := TraceCapabilities("io_activity", true); err == nil {
		t.Fatal("an output section must not acquire a callable-view name")
	}
}
