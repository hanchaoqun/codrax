package tracequery

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestRequestLatencyDistributionLinearInterpolation(t *testing.T) {
	d := requestLatencyDistribution([]float64{10, 0})
	if d == nil || d.SampleCount != 2 || d.MinMs != 0 || d.MaxMs != 10 || d.MeanMs != 5 || d.P50Ms != 5 || d.P90Ms != 9 || d.P95Ms != 9.5 || d.P99Ms != 9.9 {
		t.Fatalf("expected n-1 linear interpolation: %+v", d)
	}
	if d.QuantileMethod != IORequestLatencyQuantileMethod || d.SamplePolicy != IORequestLatencySamplePolicy || d.LatencyCaliber != IORequestLatencyCaliber {
		t.Fatalf("measurement contract missing: %+v", d)
	}
}

func TestRequestLatencyDistributionEmptyZeroAndInvalid(t *testing.T) {
	for _, samples := range [][]float64{nil, {}, {-1}, {0, math.Inf(1)}, {1, math.NaN()}} {
		if got := requestLatencyDistribution(samples); got != nil {
			t.Fatalf("empty/invalid population must not manufacture measurements: %+v", got)
		}
	}
	d := requestLatencyDistribution([]float64{0})
	if d == nil || d.SampleCount != 1 {
		t.Fatalf("real zero observation lost: %+v", d)
	}
	body, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"min_ms", "max_ms", "mean_ms", "p50_ms", "p90_ms", "p95_ms", "p99_ms"} {
		if !strings.Contains(string(body), `"`+key+`":0`) {
			t.Fatalf("required measured zero missing for %s: %s", key, body)
		}
	}
	if d := requestLatencyDistribution([]float64{math.MaxFloat64, math.MaxFloat64}); d == nil || math.IsInf(d.MeanMs, 0) || d.MeanMs != math.MaxFloat64 {
		t.Fatalf("finite samples overflowed mean: %+v", d)
	}
}

func TestRequestLatencyDistributionSummaryRetainsScope(t *testing.T) {
	row := StorageLatencySummary{Layer: "block", Event: blockEndpointFamilyRQ, PairedCount: 2, RequestLatencyDistribution: requestLatencyDistribution([]float64{0, 10})}
	summary := storageLatencySummaryText(row)
	for _, token := range []string{"samples=2", "p50=5.000", "p99=9.900", "in this group", "complete intersecting pairs", "not target blocking time"} {
		if !strings.Contains(summary, token) {
			t.Fatalf("fact summary missing %s: %s", token, summary)
		}
	}
	row.RequestLatencyDistribution = nil
	if strings.Contains(storageLatencySummaryText(row), "p99=") {
		t.Fatal("missing measurements rendered as percentiles")
	}
}
