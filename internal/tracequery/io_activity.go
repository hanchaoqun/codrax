package tracequery

import (
	"math"
	"sort"
)

type ioActivityGroupKey struct {
	source, layer, family, phase, dev, caliber string
}

type ioActivityGroupAccumulator struct {
	population ioActivityPopulationAccumulator
	// Only nonempty displayed-prefix buckets allocate storage. Idle buckets
	// are synthesized after the complete group population is accumulated.
	buckets map[int]*ioActivityPopulationAccumulator
}

func computeIOActivity(idx *Index, q Query) *IOActivityStats {
	if idx == nil || q.runCancel.sample() {
		return nil
	}
	out := &IOActivityStats{
		Population: IOActivityPopulationEndpointEvents, IssuerScope: IOInFlightIssuerScopeAll,
		QueryPID: q.PID, LineStart: q.LineStart, LineEnd: q.LineEnd, BucketMs: ioActivityBucketMs(q.BucketMs),
	}
	window := queryResultTimeWindow(q)
	if q.LineStart > 0 || q.LineEnd > 0 {
		out.WindowUnavailableReason = "line_bounds_take_precedence"
	} else if window.StartDetermined() && ioInFlightFinite(window.StartTs) && ioInFlightFinite(window.EndTs) && window.EndTs > window.StartTs && ioInFlightFinite(window.EndTs-window.StartTs) {
		out.Window = &IOActivityWindow{StartTs: window.StartTs, EndTs: window.EndTs, EndInclusive: q.timeEndBackfilled}
	} else {
		out.WindowUnavailableReason = "finite_positive_time_window_not_determined"
	}
	if idx.Windowed {
		out.Coverage.Reasons = append(out.Coverage.Reasons, "windowed_index_observed_endpoint_population_only")
	}
	if idx.PaddingTruncated {
		out.Coverage.Reasons = append(out.Coverage.Reasons, "index_padding_truncated")
	}
	count, windows, bucketReason := ioActivityBucketWindows(out.Window, out.BucketMs)
	groups := make(map[ioActivityGroupKey]*ioActivityGroupAccumulator)
	hasSupportedEndpoint := false
	for _, ev := range idx.Events {
		if q.runCancel.tick() {
			return nil
		}
		item, supported := ioActivityFromEvent(ev)
		if !supported {
			continue
		}
		hasSupportedEndpoint = true
		if !ioActivityEventSelected(ev, q) {
			continue
		}
		if !item.admitted {
			out.Coverage.RejectedEndpointCount++
			continue
		}
		source, ok := tracePairingSourceIdentity(idx, ev)
		if _, lineOK := ioInFlightMemberSourceLine(idx, source, ev.Line); !ok || !lineOK {
			out.Coverage.UnresolvedSourceCount++
			continue
		}
		out.Coverage.SupportedEndpointCount++
		key := ioActivityGroupKey{source, item.layer, item.family, item.phase, item.dev, item.caliber}
		acc := groups[key]
		if acc == nil {
			acc = &ioActivityGroupAccumulator{}
			groups[key] = acc
		}
		acc.population.add(&item)
		// Binary search only the bounded shared physical bucket boundaries;
		// no per-event scan through a potentially enormous query duration.
		bucket := sort.Search(len(windows), func(i int) bool {
			return ev.Ts < windows[i].EndTs || windows[i].EndInclusive && ev.Ts == windows[i].EndTs
		})
		if bucket < len(windows) && ev.Ts >= windows[bucket].StartTs {
			if acc.buckets == nil {
				acc.buckets = make(map[int]*ioActivityPopulationAccumulator)
			}
			if acc.buckets[bucket] == nil {
				acc.buckets[bucket] = &ioActivityPopulationAccumulator{}
			}
			acc.buckets[bucket].add(&item)
		}
	}
	if !hasSupportedEndpoint {
		return nil
	}
	if out.Coverage.RejectedEndpointCount > 0 {
		out.Coverage.Reasons = append(out.Coverage.Reasons, "endpoints_outside_supported_full_wire_or_device_contract")
	}
	if out.Coverage.UnresolvedSourceCount > 0 {
		out.Coverage.Reasons = append(out.Coverage.Reasons, "physical_source_unresolved")
	}
	keys := make([]ioActivityGroupKey, 0, len(groups))
	for key := range groups {
		if q.runCancel.tick() {
			return nil
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return ioActivityKeyLess(keys[i], keys[j]) })
	out.GroupCount = len(keys)
	if len(keys) > IOActivityGroupLimit {
		out.OmittedGroups = len(keys) - IOActivityGroupLimit
		keys = keys[:IOActivityGroupLimit]
	}
	width := 0.0
	if out.Window != nil {
		width = out.Window.EndTs - out.Window.StartTs
	}
	for _, key := range keys {
		if q.runCancel.tick() {
			return nil
		}
		acc := groups[key]
		values := acc.population.total.finish()
		group := IOActivityGroup{
			SourcePath: key.source, Layer: key.layer, EndpointFamily: key.family, Phase: key.phase, Dev: key.dev, ByteCaliber: key.caliber,
			Values: values, Rates: ioActivityRates(values, width), Directions: acc.population.finishDirections(width), ReadWrite: acc.population.readWriteRatio(),
			BucketCount: count, OmittedBuckets: count - uint64(len(windows)), BucketsUnavailableReason: bucketReason,
		}
		for i, window := range windows {
			if q.runCancel.tick() {
				return nil
			}
			bucket := acc.buckets[i]
			if bucket == nil {
				bucket = &ioActivityPopulationAccumulator{}
			}
			values := bucket.total.finish()
			width := window.EndTs - window.StartTs
			group.Buckets = append(group.Buckets, IOActivityBucket{Window: window, Values: values, Rates: ioActivityRates(values, width), Directions: bucket.finishDirections(width)})
		}
		out.Groups = append(out.Groups, group)
	}
	if q.runCancel.sample() {
		return nil
	}
	return out
}

func ioActivityEventSelected(ev Event, q Query) bool {
	if !ioInFlightFinite(ev.Ts) {
		return false
	}
	if q.LineStart > 0 || q.LineEnd > 0 {
		return (q.LineStart <= 0 || ev.Line >= q.LineStart) && (q.LineEnd <= 0 || ev.Line <= q.LineEnd)
	}
	start, startSet := q.TimeStart, queryBoundedTimeStart(q) || q.timeStartBackfilled
	end, endSet := q.TimeEnd, queryBoundedTimeEnd(q) || q.timeEndBackfilled
	if startSet && endSet && start == end {
		return ev.Ts == start // point inventory has no duration/rate denominator
	}
	return (!startSet || ev.Ts >= start) && (!endSet || ev.Ts < end || q.timeEndBackfilled && ev.Ts == end)
}

func ioActivityBucketMs(value float64) float64 {
	if !ioInFlightFinite(value) || value <= 0 {
		return 100
	}
	return math.Max(1, math.Min(60000, value))
}

// Exact decimal arithmetic only constructs the shared <=32 bucket boundaries
// and the total bucket count. It prevents phantom tiny tails at decimal
// endpoints (e.g. 0.3/0.1) without scanning or allocating the full time axis.
func ioActivityBucketWindows(window *IOActivityWindow, ms float64) (uint64, []IOActivityWindow, string) {
	if window == nil {
		return 0, nil, "continuous_time_window_unavailable"
	}
	n, spans, reason := boundedDecimalTimeBucketWindows(window.StartTs, window.EndTs, ms, IOActivityBucketLimit)
	if reason != "" {
		return n, nil, reason
	}
	out := make([]IOActivityWindow, 0, len(spans))
	for i, span := range spans {
		out = append(out, IOActivityWindow{StartTs: span.start, EndTs: span.end, EndInclusive: window.EndInclusive && uint64(i)+1 == n})
	}
	return n, out, ""
}

func ioActivityKeyLess(a, b ioActivityGroupKey) bool {
	x := [...]string{a.source, a.layer, a.family, a.phase, a.dev, a.caliber}
	y := [...]string{b.source, b.layer, b.family, b.phase, b.dev, b.caliber}
	for i := range x {
		if x[i] != y[i] {
			return x[i] < y[i]
		}
	}
	return false
}
