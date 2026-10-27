package types

import (
	"encoding/json"
	"math"
)

// TraceCausalProjectionMeasurementDomainsConflict is a veto for generic
// publication folds, not a positive identity or causal credential. A duration
// measured over a broad native partition is not its equal-valued subinterval.
// Keep that distinction even after an unknown/legacy publication was absorbed.
// Exact StateAccountKey convergence remains a separate, unchanged authority.
func TraceCausalProjectionMeasurementDomainsConflict(a, b []TraceSchedulerMeasurementOrigin) bool {
	known := func(origins []TraceSchedulerMeasurementOrigin) map[string][]ObservationSourceRef {
		out := make(map[string][]ObservationSourceRef)
		for _, origin := range origins {
			if !traceSchedulerRestrictionOriginBound(origin) || origin.MeasurementSources == nil {
				continue
			}
			for _, domain := range origin.MeasurementSources.Domains {
				if _, valid := traceSchedulerNativeRestrictionKey(domain); !valid {
					continue
				}
				// Partition IDs are provenance, not a new equality gate: existing
				// cross-view publications can use different IDs for the same scope.
				encoded, _ := json.Marshal(struct {
					Path, Method       string
					TargetTID          int
					StartTs, EndTs     float64
					LineStart, LineEnd int
				}{origin.SourceRef.Path, domain.Method, domain.TargetTID,
					domain.WindowStartTs, domain.WindowEndTs, domain.QueryLineStart, domain.QueryLineEnd})
				key := string(encoded)
				out[key] = append(out[key], origin.SourceRef)
			}
		}
		return out
	}
	left, right := known(a), known(b)
	if len(left) == 0 || len(right) == 0 {
		return false // absence does not manufacture a contradiction
	}
	if len(left) != len(right) {
		return true
	}
	differentKnown := func(x, y string) bool { return x != "" && y != "" && x != y }
	differentNumber := func(x, y *float64) bool {
		return x != nil && y != nil && !math.IsNaN(*x) && !math.IsNaN(*y) &&
			!math.IsInf(*x, 0) && !math.IsInf(*y, 0) && *x != *y
	}
	for key, xs := range left {
		ys, exists := right[key]
		if !exists {
			return true
		}
		for _, x := range xs {
			for _, y := range ys {
				if differentKnown(x.CaptureIdentityPath, y.CaptureIdentityPath) ||
					differentKnown(x.TimeDomain, y.TimeDomain) ||
					differentKnown(x.CanonicalTimeDomain, y.CanonicalTimeDomain) ||
					differentKnown(x.ClockAlignment, y.ClockAlignment) ||
					differentNumber(x.ClockOffsetSec, y.ClockOffsetSec) || differentNumber(x.ClockSlope, y.ClockSlope) {
					return true
				}
			}
		}
	}
	return false
}

// TraceCausalProjectionOccurrenceDomainsConflict rejects a guessed SUM/union of
// overlapping measurements from different known native domains. Disjoint
// occurrences and legacy unknown provenance keep their existing behavior. The
// independent complete-state-cover proof remains authoritative over its parts.
// Callers retain rejected rows; this helper never elects or discards a member.
func TraceCausalProjectionOccurrenceDomainsConflict(rows []TraceCausalProjectionNode) bool {
	indexes := make([]int, len(rows))
	for i := range rows {
		indexes[i] = i
	}
	if _, covered := traceCausalProjectionCompleteStateCover(rows, indexes); covered {
		return false
	}
	for i, a := range rows {
		if !TraceCausalProjectionWindowPresent(a.StartTs, a.EndTs) {
			continue
		}
		for _, b := range rows[i+1:] {
			if TraceCausalProjectionWindowPresent(b.StartTs, b.EndTs) &&
				a.StartTs < b.EndTs && b.StartTs < a.EndTs &&
				TraceCausalProjectionMeasurementDomainsConflict(a.MeasurementOrigins, b.MeasurementOrigins) {
				return true
			}
		}
	}
	return false
}
