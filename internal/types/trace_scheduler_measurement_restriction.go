package types

import (
	"math"
	"strconv"
	"strings"
)

// TraceSchedulerMeasurementRestrictionKey is a negative partitioning aid for
// existing aggregation rules, NOT proof that values are equal or additive.
// The producer's original event-line restriction is independent of recursive
// local windows, display limits, query views, and native accumulator methods.
// A different known restriction forbids sharing one statistical group; equal
// restrictions leave all existing physical-account and overlap rules in force.
// Empty key + true preserves the wholly legacy lane. A partially known or
// mixed restriction is not groupable and must not borrow a member's scope.
func TraceSchedulerMeasurementRestrictionKey(origins []TraceSchedulerMeasurementOrigin) (string, bool) {
	key := ""
	missing, present := false, false
	for _, origin := range origins {
		sources := origin.MeasurementSources
		if sources == nil {
			missing = true
			continue
		}
		present = true
		if sources.HasUnknown || len(sources.Domains) == 0 || !traceSchedulerRestrictionOriginBound(origin) {
			return "", false
		}
		for _, domain := range sources.Domains {
			part, ok := traceSchedulerNativeRestrictionKey(domain)
			if !ok || (key != "" && key != part) {
				return "", false
			}
			key = part
		}
	}
	if !present {
		return "", true
	}
	return key, !missing && key != ""
}

// TraceSchedulerMeasurementRestrictionsConflict reads only positively bound
// native restrictions. Unknown members cannot hide a known contradiction
// after an earlier same-fact absorb, nor prove a contradiction by themselves.
// Exact physical StateAccountKey convergence remains a separate authority.
func TraceSchedulerMeasurementRestrictionsConflict(a, b []TraceSchedulerMeasurementOrigin) bool {
	keys := func(origins []TraceSchedulerMeasurementOrigin) map[string]bool {
		out := map[string]bool{}
		for _, origin := range origins {
			if !traceSchedulerRestrictionOriginBound(origin) || origin.MeasurementSources == nil {
				continue
			}
			for _, domain := range origin.MeasurementSources.Domains {
				if key, ok := traceSchedulerNativeRestrictionKey(domain); ok {
					out[key] = true
				}
			}
		}
		return out
	}
	left, right := keys(a), keys(b)
	for x := range left {
		for y := range right {
			if x != y {
				return true
			}
		}
	}
	return false
}

// This only verifies that the restriction belongs to an addressed result.
// It does not compare parent result IDs across rows: legitimate cross-view
// supplementation and distinct local chain segments may have different IDs.
func traceSchedulerRestrictionOriginBound(origin TraceSchedulerMeasurementOrigin) bool {
	s := origin.SourceRef
	return s.Kind == ObservationSourceRuntimeArtifact && strings.TrimSpace(s.Path) != "" &&
		strings.TrimSpace(s.QueryScopeID) != "" && strings.TrimSpace(origin.ObservedAt) != "" &&
		(strings.TrimSpace(s.PayloadRef) != "" || strings.TrimSpace(s.RawRef) != "")
}

func traceSchedulerNativeRestrictionKey(d TraceSchedulerMeasurementDomain) (string, bool) {
	if d.Version != 1 || d.Status != "constructed_partition" || d.TargetTID <= 0 || d.PartitionID == "" ||
		math.IsNaN(d.WindowStartTs) || math.IsNaN(d.WindowEndTs) || math.IsInf(d.WindowStartTs, 0) || math.IsInf(d.WindowEndTs, 0) ||
		d.WindowEndTs <= d.WindowStartTs || d.QueryLineStart < 0 || d.QueryLineEnd < 0 ||
		(d.QueryLineStart > 0 && d.QueryLineEnd > 0 && d.QueryLineEnd < d.QueryLineStart) {
		return "", false
	}
	switch d.Method {
	case "thread_timeline", "off_cpu_sweep", "cpu_running_sweep", "state_churn_sweep":
	default:
		return "", false
	}
	// Zero is the explicitly unbounded filter endpoint, never missing data.
	return "event_lines:" + strconv.Itoa(d.QueryLineStart) + ":" + strconv.Itoa(d.QueryLineEnd), true
}
