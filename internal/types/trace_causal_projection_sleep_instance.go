package types

import "reflect"

// A unique waker for a thread is useful navigation, but is not the closer of
// every wait on that thread. Require a complete, single-state native instance,
// its exact end point and physical span, and an event from each contributing
// receipt. No rounded duration, label, rank, envelope or path mints identity.
func traceCausalProjectionSleepInstanceWakeup(node TraceCausalProjectionNode, edges []traceCausalProjectionWakeupEdge) (traceCausalProjectionWakeupEdge, bool) {
	if !node.sleepDrilldownNativeRecord || !node.StateAccountComplete || node.StateAccountKey == "" ||
		node.MergedCount > 1 || node.FamilyMemberCount > 1 || len(node.MergedSubjects) > 1 ||
		node.ChainCredentialSegmentsTruncated || len(node.ChainCredentialSegments) > 1 ||
		node.ChainBranch <= 0 || !traceCausalProjectionIntervalValid(node.StartTs, node.EndTs) ||
		node.LineStart <= 0 || node.LineEnd < node.LineStart ||
		node.SleepMS <= 0 || node.RunningMS != 0 || node.RunnableMS != 0 || node.DStateSplitMS != 0 || node.IOWaitSplitMS != 0 ||
		node.StateOccupancyPresence & ^traceStateOccupancySleep != 0 ||
		len(node.MeasurementOrigins) == 0 {
		return traceCausalProjectionWakeupEdge{}, false
	}
	var chosen traceCausalProjectionWakeupEdge
	for _, origin := range node.MeasurementOrigins {
		sources := origin.MeasurementSources
		if !traceSchedulerRestrictionOriginBound(origin) || sources == nil || sources.HasUnknown || len(sources.Domains) != 1 {
			return traceCausalProjectionWakeupEdge{}, false
		}
		domain := sources.Domains[0]
		if _, ok := traceSchedulerNativeRestrictionKey(domain); !ok || domain.Method != "thread_timeline" ||
			domain.WindowStartTs > node.StartTs || domain.WindowEndTs < node.EndTs {
			return traceCausalProjectionWakeupEdge{}, false
		}
		// A complete sleep inventory can contain many disjoint waits. Compare
		// the producer's inventory identity against exactly one interval;
		// rounded state totals or actual-window fragment counts cannot prove it.
		single := TraceSchedulerStateAccountIdentity(domain.TargetTID, "s_sleep", node.StartTs, node.EndTs,
			[][2]float64{{node.StartTs, node.EndTs}}, (node.EndTs-node.StartTs)*1000)
		if single == "" || single != node.StateAccountKey {
			return traceCausalProjectionWakeupEdge{}, false
		}
		var matched traceCausalProjectionWakeupEdge
		for _, edge := range edges {
			f := edge.Instance
			if f == nil || !edge.WakeupPointKnown || f.Wakee.PID != domain.TargetTID ||
				RuntimeWakeupThreadLabel(f.Wakee) != node.Subject || f.Branch != node.ChainBranch ||
				f.Timestamp != node.EndTs || f.Line < node.LineStart || f.Line > node.LineEnd ||
				edge.ObservedAt != origin.ObservedAt || !reflect.DeepEqual(edge.SourceRef, origin.SourceRef) {
				continue
			}
			// From/To and Segment belong to this result's expansion, not a
			// global node namespace. Conflicting tuples at the same point do
			// not get collapsed just because their readable labels agree.
			if matched.Instance != nil && *matched.Instance != *f {
				return traceCausalProjectionWakeupEdge{}, false
			}
			matched = edge
		}
		if matched.Instance == nil {
			return traceCausalProjectionWakeupEdge{}, false
		}
		if chosen.Instance != nil {
			a, b := chosen.Instance, matched.Instance
			// Each publication independently proved this same physical point;
			// local node IDs need not agree across distinct result receipts.
			x, y := chosen.SourceRef, matched.SourceRef
			if x.Path != y.Path || x.CaptureIdentityPath != y.CaptureIdentityPath ||
				x.TimeDomain != y.TimeDomain || x.CanonicalTimeDomain != y.CanonicalTimeDomain ||
				x.ClockAlignment != y.ClockAlignment || x.ClockCalibrated != y.ClockCalibrated ||
				!reflect.DeepEqual(x.ClockOffsetSec, y.ClockOffsetSec) || !reflect.DeepEqual(x.ClockSlope, y.ClockSlope) ||
				a.Waker != b.Waker || a.Wakee != b.Wakee || a.Timestamp != b.Timestamp || a.Line != b.Line {
				return traceCausalProjectionWakeupEdge{}, false
			}
		} else {
			chosen = matched
		}
	}
	return chosen, chosen.Instance != nil
}
