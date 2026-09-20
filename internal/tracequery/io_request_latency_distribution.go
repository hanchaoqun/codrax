package tracequery

import (
	"math"
	"sort"
	"strconv"
)

// Total ordering prevents equal-size groups from changing their display
// membership with map iteration. It does not combine independent identities.
func storageLatencyGroupSortKey(item StorageLatencySummary) string {
	return encodePairingKey(item.SourcePath, item.Layer, item.Event, item.Dev, item.Inode, item.Operation, strconv.Itoa(item.Thread.PID))
}

const (
	IORequestLatencyQuantileMethod = "linear_interpolation_n_minus_1"
	IORequestLatencySamplePolicy   = "complete_pairs_intersecting_query"
	IORequestLatencyCaliber        = "full_request_start_to_completion"
)

// IORequestLatencyDistribution measures all admitted pairs in ONE storage
// summary group, before any display limit. It is request residence, not target
// blocking time, a cross-layer total, or proof of a causal dependency. Zero is
// a valid observation; no admitted samples is represented by a nil pointer.
type IORequestLatencyDistribution struct {
	SampleCount    int     `json:"sample_count"`
	MinMs          float64 `json:"min_ms"`
	MaxMs          float64 `json:"max_ms"`
	MeanMs         float64 `json:"mean_ms"`
	P50Ms          float64 `json:"p50_ms"`
	P90Ms          float64 `json:"p90_ms"`
	P95Ms          float64 `json:"p95_ms"`
	P99Ms          float64 `json:"p99_ms"`
	QuantileMethod string  `json:"quantile_method"`
	SamplePolicy   string  `json:"sample_policy"`
	LatencyCaliber string  `json:"latency_caliber"`
}

// requestLatencyDistribution owns the private accumulator slice and sorts it
// in place. Call only after the existing precise pairing admission. The helper
// does not infer pairs or silently filter invalid values into a smaller cohort.
func requestLatencyDistribution(samples []float64) *IORequestLatencyDistribution {
	if len(samples) == 0 {
		return nil
	}
	for _, sample := range samples {
		if sample < 0 || math.IsNaN(sample) || math.IsInf(sample, 0) {
			return nil
		}
	}
	sort.Float64s(samples)
	// The online mean avoids overflowing an intermediate sum of finite,
	// nonnegative durations. It does not alter the existing average field.
	var mean float64
	for i, sample := range samples {
		mean += (sample - mean) / float64(i+1)
	}
	quantile := func(p float64) float64 {
		position := p * float64(len(samples)-1)
		lo := int(position)
		hi := lo + 1
		if hi == len(samples) {
			return samples[lo]
		}
		return samples[lo] + (position-float64(lo))*(samples[hi]-samples[lo])
	}
	return &IORequestLatencyDistribution{
		SampleCount: len(samples), MinMs: samples[0], MaxMs: samples[len(samples)-1], MeanMs: mean,
		P50Ms: quantile(.50), P90Ms: quantile(.90), P95Ms: quantile(.95), P99Ms: quantile(.99),
		QuantileMethod: IORequestLatencyQuantileMethod, SamplePolicy: IORequestLatencySamplePolicy, LatencyCaliber: IORequestLatencyCaliber,
	}
}
