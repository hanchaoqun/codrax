package tracequery

import (
	"math"
	"sort"
)

const (
	ioInFlightGroupLimit   = 8
	ioInFlightSegmentLimit = 16
)

// A comparable tuple preserves source and endpoint ruler without delimiter
// collisions. PID/inode/sector remain pairing identities, not grouping axes:
// independent admitted requests may overlap on the same device and operation.
type ioInFlightGroupKey struct {
	source, layer, family, dev, operation string
}

type ioInFlightInterval struct {
	key        ioInFlightGroupKey
	start, end float64
	endpoints  ioInFlightEndpoints
}

// Endpoint ownership/coordinates are carried at the existing matcher's
// success point, never reconstructed from latency summaries or top details.
type ioInFlightEndpoints struct {
	issue, complete         ThreadRef
	issueLine, completeLine int
}

type ioInFlightStarts map[ioInFlightGroupKey]int

type ioInFlightGroupAccumulator struct {
	group  IOInFlightGroup
	deltas map[float64]int
}

// This result extends the existing non-block replay, not a second matcher.
// The legacy wrapper continues returning only summaries and caveats.
type storagePairingResult struct {
	summaries []StorageLatencySummary
	caveats   []string
	intervals []ioInFlightInterval
	starts    ioInFlightStarts
	coverage  IOInFlightPairingCoverage
}

func ioInFlightBlockKey(item IOLatencySummary) ioInFlightGroupKey {
	return ioInFlightGroupKey{item.SourcePath, "block", item.EndpointFamily, item.Dev, item.Op}
}

func ioInFlightStorageKey(lane *storageLatencyLane, start Event) ioInFlightGroupKey {
	// The existing matcher uses a missing-device sentinel internally. Do not
	// publish that sentinel as a device supplied by the capture.
	dev := ""
	if start.FileFields != nil {
		dev = start.FileFields.Dev
	}
	if start.BlockIOFields != nil {
		dev = firstNonEmpty(dev, start.BlockIOFields.Dev, start.BlockIOFields.SrcDev)
	}
	return ioInFlightGroupKey{lane.source, lane.identity.Layer, lane.identity.Base, dev, lane.identity.Op}
}

func ioInFlightFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func ioInFlightRecordStart(starts *ioInFlightStarts, key ioInFlightGroupKey, ev Event, q Query) {
	if !ioInFlightFinite(ev.Ts) || (q.LineStart > 0 && ev.Line < q.LineStart) || (q.LineEnd > 0 && ev.Line > q.LineEnd) {
		return
	}
	// Line bounds take precedence, as in the existing pairing query. Without
	// them arrivals use half-open time membership; pair admission is unchanged.
	if q.LineStart == 0 && q.LineEnd == 0 && ((queryBoundedTimeStart(q) && ev.Ts < q.TimeStart) || (queryBoundedTimeEnd(q) && ev.Ts >= q.TimeEnd)) {
		return
	}
	if *starts == nil {
		*starts = ioInFlightStarts{}
	}
	(*starts)[key]++
}

func ioInFlightPairingCoverage(idx *Index, family string, integrity *durationPairingIntegrity, summaries []StorageLatencySummary) IOInFlightPairingCoverage {
	coverage := IOInFlightPairingCoverage{Family: family, Status: IOInFlightCoverageAvailable, TopologyComplete: completePhysicalPairingTopology(idx)}
	for _, item := range summaries {
		if (family == "block") != (item.Layer == "block") {
			continue
		}
		coverage.AcceptedPairCount += item.PairedCount
		coverage.UnpairedStartCount += item.UnpairedStartCount
		coverage.UnpairedDoneCount += item.UnpairedDoneCount
		coverage.AmbiguousCohortCount += item.AmbiguousCohortCount
		coverage.PairingSuppressedCount += item.PairingSuppressedCount
	}
	if integrity == nil || idx == nil {
		coverage.Status = IOInFlightCoverageUnavailable
		coverage.Reasons = append(coverage.Reasons, "pairing_integrity_unavailable")
		return coverage
	}
	coverage.RejectedEndpointRows = integrity.rejectedEndpointRows
	coverage.UnresolvedSources = integrity.unresolvedSources
	if integrity.topologyIncomplete {
		coverage.Reasons = append(coverage.Reasons, "pairing_topology_incomplete")
	}
	if integrity.budgetExceeded {
		coverage.Reasons = append(coverage.Reasons, "pairing_audit_budget_exceeded")
	}
	if integrity.familyGlobal {
		coverage.Status = IOInFlightCoverageUnavailable
		coverage.Reasons = append(coverage.Reasons, "pairing_family_fail_closed")
	} else if len(integrity.poisonedSources)+len(integrity.poisonedSourceScopes)+len(integrity.poisonedLanes) > 0 {
		coverage.Status = IOInFlightCoveragePartial
		coverage.Reasons = append(coverage.Reasons, "pairing_source_or_lane_quarantined")
	}
	if coverage.UnpairedStartCount+coverage.UnpairedDoneCount > 0 {
		coverage.Reasons = append(coverage.Reasons, "unpaired_endpoints_excluded")
	}
	if coverage.AmbiguousCohortCount+coverage.PairingSuppressedCount > 0 {
		coverage.Reasons = append(coverage.Reasons, "ambiguous_or_invalid_pairs_excluded")
	}
	if coverage.RejectedEndpointRows+coverage.UnresolvedSources > 0 {
		coverage.Reasons = append(coverage.Reasons, "input_endpoints_rejected")
	}
	if coverage.Status == IOInFlightCoverageAvailable && len(coverage.Reasons) > 0 {
		coverage.Status = IOInFlightCoveragePartial
	}
	return coverage
}

func buildIOInFlightStats(q Query, block blockPairingResult, storage storagePairingResult, indexes ...*Index) *IOInFlightStats {
	if q.runCancel.sample() {
		return nil
	}
	out := &IOInFlightStats{
		Population: IOInFlightPopulationCompletePairs, IssuerScope: IOInFlightIssuerScopeAll,
		QueryPID: q.PID, LineStart: q.LineStart, LineEnd: q.LineEnd,
		Coverage: []IOInFlightPairingCoverage{block.coverage, storage.coverage},
	}
	window := queryResultTimeWindow(q)
	if q.LineStart > 0 || q.LineEnd > 0 {
		// Explicit/backfilled times do not establish the time denominator of
		// a line-selected population. Preserve its counts without inventing zero.
		out.WindowUnavailableReason = "line_bounds_take_precedence"
	} else if window.StartDetermined() && ioInFlightFinite(window.StartTs) && ioInFlightFinite(window.EndTs) && window.EndTs > window.StartTs && ioInFlightFinite((window.EndTs-window.StartTs)*1000) {
		out.Window = &IOInFlightWindow{StartTs: window.StartTs, EndTs: window.EndTs}
	} else {
		out.WindowUnavailableReason = "finite_positive_time_window_not_determined"
	}
	groups := map[ioInFlightGroupKey]*ioInFlightGroupAccumulator{}
	var idx *Index
	if len(indexes) > 0 {
		idx = indexes[0]
	}
	groupFor := func(key ioInFlightGroupKey) *ioInFlightGroupAccumulator {
		if acc := groups[key]; acc != nil {
			return acc
		}
		acc := &ioInFlightGroupAccumulator{group: IOInFlightGroup{
			SourcePath: key.source, Layer: key.layer, EndpointFamily: key.family, Dev: key.dev, Operation: key.operation,
		}, deltas: map[float64]int{}}
		groups[key] = acc
		return acc
	}
	for _, starts := range []ioInFlightStarts{block.starts, storage.starts} {
		for key, count := range starts {
			if q.runCancel.tick() {
				return nil
			}
			groupFor(key).group.IssueCount += count
		}
	}
	add := func(pair ioInFlightInterval) {
		if !ioInFlightFinite(pair.start) || !ioInFlightFinite(pair.end) || pair.end < pair.start {
			return
		}
		acc := groupFor(pair.key)
		acc.group.AcceptedPairCount++
		if member, ok := ioInFlightMemberForPair(idx, pair, out.Window); ok {
			retainIOInFlightMember(&acc.group, member)
		} else {
			acc.group.MemberWitnessUnavailableCount++
		}
		if out.Window == nil {
			return
		}
		start, end := math.Max(pair.start, out.Window.StartTs), math.Min(pair.end, out.Window.EndTs)
		if end <= start {
			return
		}
		acc.deltas[start]++
		acc.deltas[end]--
	}
	for _, pair := range block.census {
		if q.runCancel.tick() {
			return nil
		}
		add(ioInFlightInterval{key: ioInFlightBlockKey(pair), start: pair.IssueTs, end: pair.CompleteTs,
			endpoints: ioInFlightEndpoints{pair.IssueThread, pair.CompleteThread, pair.IssueLine, pair.CompleteLine}})
	}
	for _, pair := range storage.intervals {
		if q.runCancel.tick() {
			return nil
		}
		add(pair)
	}
	for _, acc := range groups {
		if q.runCancel.tick() {
			return nil
		}
		if out.Window != nil && acc.group.AcceptedPairCount > 0 {
			if !finishIOInFlightGroup(q, acc, *out.Window) {
				return nil
			}
		} else if out.Window == nil {
			acc.group.ValuesUnavailableReason = out.WindowUnavailableReason
		} else {
			acc.group.ValuesUnavailableReason = "no_accepted_complete_pairs"
		}
		out.Groups = append(out.Groups, acc.group)
	}
	sort.Slice(out.Groups, func(i, j int) bool { return ioInFlightGroupLess(out.Groups[i], out.Groups[j]) })
	if q.runCancel.sample() {
		return nil
	}
	// A query with neither admitted requests nor pairing diagnostics has no
	// in-flight observation to publish. All-zero available coverage is not
	// proof that the capture measured an idle IO device. Real diagnostics and
	// complete zero-duration pairs remain visible.
	if len(out.Groups) == 0 && !ioInFlightHasPairingEvidence(out.Coverage) {
		return nil
	}
	out.GroupCount = len(out.Groups)
	if len(out.Groups) > ioInFlightGroupLimit {
		out.OmittedGroups = len(out.Groups) - ioInFlightGroupLimit
		out.Groups = out.Groups[:ioInFlightGroupLimit]
	}
	return out
}

func ioInFlightHasPairingEvidence(coverage []IOInFlightPairingCoverage) bool {
	for _, c := range coverage {
		if c.Status != "" && c.Status != IOInFlightCoverageAvailable || len(c.Reasons) > 0 {
			return true
		}
		for _, count := range [...]int{c.AcceptedPairCount, c.UnpairedStartCount, c.UnpairedDoneCount, c.AmbiguousCohortCount, c.PairingSuppressedCount, c.RejectedEndpointRows, c.UnresolvedSources} {
			if count != 0 {
				return true
			}
		}
	}
	return false
}

func finishIOInFlightGroup(q Query, acc *ioInFlightGroupAccumulator, window IOInFlightWindow) bool {
	times := make([]float64, 0, len(acc.deltas)+2)
	for ts := range acc.deltas {
		if q.runCancel.tick() {
			return false
		}
		times = append(times, ts)
	}
	times = append(times, window.StartTs, window.EndTs)
	sort.Float64s(times)
	if q.runCancel.sample() {
		return false
	}
	values := &IOInFlightValues{}
	depth := 0
	previous := window.StartTs
	segments := []IOInFlightSegment{}
	for _, ts := range times {
		if q.runCancel.tick() {
			return false
		}
		if ts > previous {
			ms := (ts - previous) * 1000
			values.RequestMs += float64(depth) * ms
			if depth > 0 {
				values.BusyMs += ms
			}
			if depth > values.PeakRequests {
				values.PeakRequests = depth
			}
			if len(segments) > 0 && segments[len(segments)-1].Requests == depth {
				segments[len(segments)-1].EndTs = ts
			} else {
				segments = append(segments, IOInFlightSegment{StartTs: previous, EndTs: ts, Requests: depth})
			}
		}
		// Consume each timestamp once. Simultaneous completions and starts
		// change the next half-open segment together, never a transient peak.
		depth += acc.deltas[ts]
		delete(acc.deltas, ts)
		previous = ts
	}
	values.MeanRequests = values.RequestMs / ((window.EndTs - window.StartTs) * 1000)
	if !ioInFlightFinite(values.RequestMs) || !ioInFlightFinite(values.BusyMs) || !ioInFlightFinite(values.MeanRequests) {
		acc.group.ValuesUnavailableReason = "non_finite_statistic"
		return true
	}
	acc.group.Values = values
	if len(segments) > ioInFlightSegmentLimit {
		acc.group.OmittedSegments = len(segments) - ioInFlightSegmentLimit
		segments = segments[:ioInFlightSegmentLimit]
	}
	acc.group.Segments = segments
	return true
}

func ioInFlightGroupLess(a, b IOInFlightGroup) bool {
	if (a.Values == nil) != (b.Values == nil) {
		return a.Values != nil
	}
	if a.Values != nil {
		if a.Values.PeakRequests != b.Values.PeakRequests {
			return a.Values.PeakRequests > b.Values.PeakRequests
		}
		if a.Values.RequestMs != b.Values.RequestMs {
			return a.Values.RequestMs > b.Values.RequestMs
		}
	}
	if a.AcceptedPairCount != b.AcceptedPairCount {
		return a.AcceptedPairCount > b.AcceptedPairCount
	}
	if a.IssueCount != b.IssueCount {
		return a.IssueCount > b.IssueCount
	}
	x, y := [...]string{a.SourcePath, a.Layer, a.EndpointFamily, a.Dev, a.Operation}, [...]string{b.SourcePath, b.Layer, b.EndpointFamily, b.Dev, b.Operation}
	for i := range x {
		if x[i] != y[i] {
			return x[i] < y[i]
		}
	}
	return false
}
