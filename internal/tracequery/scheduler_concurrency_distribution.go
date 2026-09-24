package tracequery

import (
	"math"
	"math/big"
	"sort"
	"strconv"
)

func prepareSchedulerConcurrencyBuckets(g *SchedulerConcurrencyGroup, window SchedulerConcurrencyWindow, ms float64) {
	n, spans, reason := boundedDecimalTimeBucketWindows(window.StartTs, window.EndTs, ms, SchedulerConcurrencyBucketLimit)
	g.BucketCount, g.BucketsUnavailableReason = n, reason
	if reason != "" {
		return
	}
	g.OmittedBuckets = n - uint64(len(spans))
	for _, span := range spans {
		g.Buckets = append(g.Buckets, SchedulerConcurrencyBucket{Window: SchedulerConcurrencyWindow{StartTs: span.start, EndTs: span.end}})
	}
}

// The full sweep feeds all displayed buckets, not the capped logical segments.
// At most 32 overlap checks per sweep span and no allocation proportional to
// a potentially immense bucket count are required.
func accumulateSchedulerConcurrencyBuckets(buckets []SchedulerConcurrencyBucket, start, end float64, depth int) {
	for i := range buckets {
		b := &buckets[i]
		left, right := math.Max(start, b.Window.StartTs), math.Min(end, b.Window.EndTs)
		if right <= left {
			continue
		}
		ms := (right - left) * 1000
		if depth > b.Values.PeakThreads {
			b.Values.PeakThreads = depth
		}
		b.Values.ThreadMs += float64(depth) * ms
		if depth > 0 {
			b.Values.BusyMs += ms
		}
	}
}

// One full ULP bounds shortest-decimal input rounding and each elementary
// float operation. The bound only selects an exact fallback; it NEVER changes
// a percentile by treating a near-boundary interval as equal. Thus a real 1ns
// difference remains distinguishable even when it lies inside this bound.
func schedulerConcurrencyULP(v float64) float64 {
	v = math.Abs(v)
	return math.Nextafter(v, math.Inf(1)) - v
}

func schedulerConcurrencySpanRoundoff(start, end float64) float64 {
	delta := end - start
	return 2 * ((schedulerConcurrencyULP(start)+schedulerConcurrencyULP(end)+schedulerConcurrencyULP(delta))*1000 + schedulerConcurrencyULP(delta*1000))
}

func schedulerConcurrencyDistribution(query Query, histogram map[int]float64, histogramRoundoff float64, window SchedulerConcurrencyWindow, points []float64, deltas map[float64]int) *SchedulerConcurrencyDistribution {
	windowMs := (window.EndTs - window.StartTs) * 1000
	depths := make([]int, 0, len(histogram))
	for depth := range histogram {
		depths = append(depths, depth)
	}
	sort.Ints(depths)
	d := &SchedulerConcurrencyDistribution{WindowMs: windowMs, DepthCount: len(depths)}
	quantiles := [...]float64{.5, .95, .99}
	values := [...]*int{&d.P50Threads, &d.P95Threads, &d.P99Threads}
	q, cumulative := 0, 0.0
	prefixRoundoff := 0.0
	needsExact := false
	for _, depth := range depths {
		ms := histogram[depth]
		cumulative += ms
		prefixRoundoff = math.Nextafter(prefixRoundoff+schedulerConcurrencyULP(cumulative), math.Inf(1))
		for _, p := range quantiles {
			threshold := p * windowMs
			bound := 2 * (histogramRoundoff + prefixRoundoff + schedulerConcurrencySpanRoundoff(window.StartTs, window.EndTs) + schedulerConcurrencyULP(p)*windowMs + schedulerConcurrencyULP(threshold))
			if math.Abs(cumulative-threshold) <= bound {
				needsExact = true
			}
		}
		for q < len(quantiles) && cumulative >= quantiles[q]*windowMs {
			*values[q] = depth
			q++
		}
		if len(d.Depths) < SchedulerConcurrencyDepthLimit {
			d.Depths = append(d.Depths, SchedulerConcurrencyDepthDuration{Threads: depth, DurationMs: ms, WindowShare: ms / windowMs})
		}
	}
	d.OmittedDepths = len(depths) - len(d.Depths)
	if needsExact && !schedulerConcurrencyExactQuantiles(query, points, deltas, depths, values) {
		return nil
	}
	return d
}

// Only ambiguous comparisons take this path. It reuses the existing full
// sweep boundary/delta inventory and a few Rat scratch values; no event-level
// heap carrier, member-copy, bucket expansion or per-TID rescan is introduced.
func schedulerConcurrencyExactQuantiles(q Query, points []float64, deltas map[float64]int, depths []int, values [3]*int) bool {
	exact := map[int]*big.Rat{}
	var left, right, span, total big.Rat
	left.SetString(strconv.FormatFloat(points[0], 'f', -1, 64))
	depth := 0
	for i := 0; i+1 < len(points); i++ {
		if q.runCancel.tick() {
			return false
		}
		depth += deltas[points[i]]
		right.SetString(strconv.FormatFloat(points[i+1], 'f', -1, 64))
		span.Sub(&right, &left)
		value := exact[depth]
		if value == nil {
			value = new(big.Rat)
			exact[depth] = value
		}
		value.Add(value, &span)
		total.Add(&total, &span)
		left.Set(&right)
	}
	var cumulative, scaled, threshold big.Rat
	percentiles := [...]int64{50, 95, 99}
	j := 0
	for _, depth := range depths {
		cumulative.Add(&cumulative, exact[depth])
		scaled.Mul(&cumulative, big.NewRat(100, 1))
		for j < len(percentiles) {
			threshold.Mul(&total, big.NewRat(percentiles[j], 1))
			if scaled.Cmp(&threshold) < 0 {
				break
			}
			*values[j] = depth
			j++
		}
	}
	return !q.runCancel.sample()
}
