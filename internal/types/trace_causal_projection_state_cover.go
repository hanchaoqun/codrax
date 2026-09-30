package types

import (
	"math"
	"strconv"
)

// A complete census of one scheduler state over W contains every census of
// that same state over a subwindow of W. This is set containment, not a maximum
// heuristic: require producer completeness, raw value identity, the same native
// measurement method/thread and positively bound source/line selection. Merely
// overlapping windows, identical durations, or identical labels prove nothing.
// Keep all member evidence and use the existing union disclosure; no causal
// membership, rank, dependency edge or request-window decision changes here.
func traceCausalProjectionCompleteStateCover(nodes []TraceCausalProjectionNode, members []int) (int, bool) {
	if len(members) < 2 {
		return 0, false
	}
	var identity string
	for _, i := range members {
		n := nodes[i]
		if !n.StateAccountComplete || n.StateAccountKey == "" || n.Predicate != "wakeup_causal_impact" ||
			n.Unit != "ms" || n.MergedCount != 0 || n.FamilyMemberCount != 0 ||
			n.PriorityInversionCandidate || n.SupplyFoldComputed ||
			!traceCausalProjectionIntervalValid(n.StartTs, n.EndTs) {
			return 0, false
		}
		switch n.Object {
		case "running", "runnable", "s_sleep", "d_sleep", "io_wait":
		default:
			return 0, false
		}
		v, err := strconv.ParseFloat(n.Value, 64)
		if err != nil || !traceStateCoverSameValue(v, traceCausalProjectionDisplayValue(n)) ||
			v > (n.EndTs-n.StartTs)*1000+TraceCausalProjectionSameValueTieMS {
			return 0, false
		}
		key, ok := traceStateCoverIdentity(n)
		if !ok || (identity != "" && key != identity) {
			return 0, false
		}
		identity = key
	}
	for _, candidate := range members {
		cover := nodes[candidate]
		contains := true
		for _, child := range members {
			n := nodes[child]
			if n.StartTs < cover.StartTs || n.EndTs > cover.EndTs ||
				traceCausalProjectionDisplayValue(n) > traceCausalProjectionDisplayValue(cover)+TraceCausalProjectionSameValueTieMS {
				contains = false
				break
			}
		}
		if contains {
			return candidate, true
		}
	}
	return 0, false
}

func traceStateCoverSameValue(a, b float64) bool {
	return a > 0 && b > 0 && !math.IsNaN(a) && !math.IsInf(a, 0) &&
		!math.IsNaN(b) && !math.IsInf(b, 0) && math.Abs(a-b) < TraceCausalProjectionSameValueTieMS
}

func traceStateCoverIdentity(n TraceCausalProjectionNode) (string, bool) {
	restriction, ok := TraceSchedulerMeasurementRestrictionKey(n.MeasurementOrigins)
	if !ok || restriction == "" || n.Subject == "" {
		return "", false
	}
	key := ""
	for _, origin := range n.MeasurementOrigins {
		s := origin.SourceRef
		// Native identity-clock receipts only. Clock-converted projections
		// require a separate proof that converted census bounds stay exact.
		if s.TimeDomain != "trace_seconds" || s.CanonicalTimeDomain != "trace_seconds" || s.ClockAlignment != "identity" {
			return "", false
		}
		for _, d := range origin.MeasurementSources.Domains {
			if d.Method != "thread_timeline" {
				return "", false
			}
			part := s.Path + "\x00" + strconv.Itoa(d.TargetTID) + "\x00" + n.Subject + "\x00" + n.Object + "\x00" + restriction
			if key != "" && key != part {
				return "", false
			}
			key = part
		}
	}
	return key, key != ""
}
