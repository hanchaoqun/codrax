package tracequery

import (
	"math"
	"sort"
)

type wakeupAggregateOccurrenceMember struct {
	impact WakeupCausalImpact
	window foldInterval
}

// reconcileWakeupAggregateOccurrenceOverlap closes the non-inversion half of
// the aggregate additivity invariant. Recurring cadence starts identify
// distinct observations, but they do not prove that the projected physical
// windows are disjoint. Consequently periodic and aperiodic aggregates use
// this same reconciliation path.
//
// Pairwise-disjoint occurrence windows retain their exact SUM. Within an
// overlap-connected cohort we use exact interval UNION only when every member
// proves one full-window state with no supply-fold side vector. Otherwise one
// strongest member supplies the complete coupled vector (durations, counts,
// target wait, and supply-fold basis). Selecting the vector as a unit avoids
// the impossible "max of every field" aggregate assembled from different
// physical occurrences.
func reconcileWakeupAggregateOccurrenceOverlap(chain *ChainResult, item *WakeupCausalAggregate, members []int) {
	if chain == nil || item == nil || len(members) < 2 {
		return
	}
	occurrences, complete := wakeupAggregateOccurrenceMembers(chain, members)
	if !complete {
		item.AggregationCaliber = GatedCaliberMaxOverlapFallback
		resetWakeupAggregateCoupledVector(item)
		if representative, ok := wakeupAggregateRepresentativeFromIndices(chain, members); ok {
			addWakeupAggregateMemberVector(item, representative, false, 0, "", false, 0, "")
		}
		return
	}
	components := wakeupAggregateOverlapComponents(occurrences)
	if len(components) == len(occurrences) {
		item.AggregationCaliber = GatedCaliberSumDisjointOccurrences
		return
	}
	item.AggregationCaliber = WakeupAggregateCaliberOverlapSafe
	resetWakeupAggregateCoupledVector(item)
	for _, component := range components {
		representative := wakeupAggregateRepresentative(component)
		projectedUnion, projectedState, projectedExact := wakeupAggregateExactSingleStateUnion(component, false)
		actualUnion, actualState, actualExact := wakeupAggregateExactSingleStateUnion(component, true)
		addWakeupAggregateMemberVector(item, representative, projectedExact, projectedUnion, projectedState, actualExact, actualUnion, actualState)
	}
}

func wakeupAggregateOccurrenceMembers(chain *ChainResult, members []int) ([]wakeupAggregateOccurrenceMember, bool) {
	occurrences := make([]wakeupAggregateOccurrenceMember, 0, len(members))
	for _, index := range members {
		if index < 0 || index >= len(chain.CausalImpacts) {
			return nil, false
		}
		impact := chain.CausalImpacts[index]
		if impact.Window.EndTs <= impact.Window.StartTs {
			return nil, false
		}
		occurrences = append(occurrences, wakeupAggregateOccurrenceMember{
			impact: impact,
			window: foldInterval{start: impact.Window.StartTs, end: impact.Window.EndTs},
		})
	}
	sort.SliceStable(occurrences, func(i, j int) bool {
		if occurrences[i].window.start != occurrences[j].window.start {
			return occurrences[i].window.start < occurrences[j].window.start
		}
		if occurrences[i].window.end != occurrences[j].window.end {
			return occurrences[i].window.end < occurrences[j].window.end
		}
		return occurrences[i].impact.LineStart < occurrences[j].impact.LineStart
	})
	return occurrences, true
}

func wakeupAggregateOverlapComponents(sorted []wakeupAggregateOccurrenceMember) [][]wakeupAggregateOccurrenceMember {
	if len(sorted) == 0 {
		return nil
	}
	components := [][]wakeupAggregateOccurrenceMember{{sorted[0]}}
	componentEnd := sorted[0].window.end
	for _, member := range sorted[1:] {
		if member.window.start < componentEnd {
			last := len(components) - 1
			components[last] = append(components[last], member)
			if member.window.end > componentEnd {
				componentEnd = member.window.end
			}
			continue
		}
		components = append(components, []wakeupAggregateOccurrenceMember{member})
		componentEnd = member.window.end
	}
	return components
}

func wakeupAggregateRepresentative(component []wakeupAggregateOccurrenceMember) WakeupCausalImpact {
	representative := component[0].impact
	for _, member := range component[1:] {
		if wakeupAggregateImpactStronger(member.impact, representative) {
			representative = member.impact
		}
	}
	return representative
}

func wakeupAggregateRepresentativeFromIndices(chain *ChainResult, members []int) (WakeupCausalImpact, bool) {
	var representative WakeupCausalImpact
	found := false
	for _, index := range members {
		if index < 0 || index >= len(chain.CausalImpacts) {
			continue
		}
		impact := chain.CausalImpacts[index]
		if !found || wakeupAggregateImpactStronger(impact, representative) {
			representative = impact
			found = true
		}
	}
	return representative, found
}

func wakeupAggregateImpactStronger(candidate, current WakeupCausalImpact) bool {
	for _, values := range [][2]float64{
		{causalImpactBlockingMs(candidate), causalImpactBlockingMs(current)},
		{firstPositiveFloat(candidate.ProjectedTotalMs, candidate.TotalMs), firstPositiveFloat(current.ProjectedTotalMs, current.TotalMs)},
		{actualCausalImpactBlockingMs(candidate), actualCausalImpactBlockingMs(current)},
		{candidate.TargetBlockedMs, current.TargetBlockedMs},
	} {
		if values[0] != values[1] {
			return values[0] > values[1]
		}
	}
	if candidate.LineStart != current.LineStart {
		return candidate.LineStart < current.LineStart
	}
	return candidate.Window.StartTs < current.Window.StartTs
}

// wakeupAggregateExactSingleStateUnion proves that every occurrence in one
// overlap cohort covers its whole typed window in the same single state. A
// supply-fold basis intentionally disables this shortcut: without the
// sub-interval frequency geometry, unioning RunningMs while selecting an
// unrelated basis vector would break Known+Unknown and ideal+deficit
// identities.
func wakeupAggregateExactSingleStateUnion(component []wakeupAggregateOccurrenceMember, actual bool) (float64, string, bool) {
	if len(component) == 0 {
		return 0, "", false
	}
	state := ""
	intervals := make([]foldInterval, 0, len(component))
	for _, member := range component {
		if member.impact.SupplyFoldBasis != nil {
			return 0, "", false
		}
		window := member.impact.Window
		if actual && member.impact.ActualWindow.EndTs > member.impact.ActualWindow.StartTs {
			window = member.impact.ActualWindow
		}
		if window.EndTs <= window.StartTs {
			return 0, "", false
		}
		windowMs := (window.EndTs - window.StartTs) * 1000
		memberState, ok := wakeupAggregateFullWindowState(member.impact, actual, windowMs)
		if !ok || (state != "" && state != memberState) {
			return 0, "", false
		}
		state = memberState
		intervals = append(intervals, foldInterval{start: window.StartTs, end: window.EndTs})
	}
	unionMs, _ := foldIntervalUnionMs(intervals)
	return unionMs, state, unionMs > 0
}

func wakeupAggregateFullWindowState(impact WakeupCausalImpact, actual bool, windowMs float64) (string, bool) {
	total := firstPositiveFloat(impact.ProjectedTotalMs, impact.TotalMs)
	values := []struct {
		state string
		value float64
	}{
		{string(StateRunning), impact.RunningMs},
		{string(StateRunnable), impact.RunnableMs},
		{string(StateSSleep), impact.SleepMs},
		{string(StateDSleep), impact.DStateMs},
		{string(StateIOWait), impact.IOWaitMs},
	}
	if actual {
		total = impact.ActualTotalMs
		values[0].value = impact.ActualRunningMs
		values[1].value = impact.ActualRunnableMs
		values[2].value = impact.ActualSleepMs
		values[3].value = impact.ActualDStateMs
		values[4].value = impact.ActualIOWaitMs
	}
	tolerance := math.Max(1e-6, math.Max(windowMs, total)*1e-9)
	if math.Abs(total-windowMs) > tolerance {
		return "", false
	}
	state := ""
	for _, entry := range values {
		if entry.value <= tolerance {
			continue
		}
		if state != "" || math.Abs(entry.value-windowMs) > tolerance {
			return "", false
		}
		state = entry.state
	}
	return state, state != ""
}

func resetWakeupAggregateCoupledVector(item *WakeupCausalAggregate) {
	item.TotalMs = 0
	item.ProjectedTotalMs = 0
	item.RunningMs = 0
	item.RunnableMs = 0
	item.SleepMs = 0
	item.DStateMs = 0
	item.IOWaitMs = 0
	item.ActualTotalMs = 0
	item.ActualRunningMs = 0
	item.ActualRunnableMs = 0
	item.ActualSleepMs = 0
	item.ActualDStateMs = 0
	item.ActualIOWaitMs = 0
	item.TargetBlockedMs = 0
	item.FragmentCount = 0
	item.StateSwitches = 0
	item.MaxSegmentMs = 0
	item.SupplyFoldDeficitMs = 0
	item.SupplyFoldIdealMs = 0
	item.SupplyFoldBasis = nil
}

func addWakeupAggregateMemberVector(item *WakeupCausalAggregate, impact WakeupCausalImpact, projectedExact bool, projectedUnion float64, projectedState string, actualExact bool, actualUnion float64, actualState string) {
	if projectedExact {
		item.TotalMs += projectedUnion
		item.ProjectedTotalMs += projectedUnion
		addWakeupAggregateStateValue(item, projectedState, projectedUnion, false)
		item.MaxSegmentMs = math.Max(item.MaxSegmentMs, projectedUnion)
	} else {
		item.TotalMs += impact.TotalMs
		item.ProjectedTotalMs += firstPositiveFloat(impact.ProjectedTotalMs, impact.TotalMs)
		item.RunningMs += impact.RunningMs
		item.RunnableMs += impact.RunnableMs
		item.SleepMs += impact.SleepMs
		item.DStateMs += impact.DStateMs
		item.IOWaitMs += impact.IOWaitMs
		item.MaxSegmentMs = math.Max(item.MaxSegmentMs, impact.MaxSegmentMs)
	}
	if actualExact {
		item.ActualTotalMs += actualUnion
		addWakeupAggregateStateValue(item, actualState, actualUnion, true)
	} else {
		item.ActualTotalMs += impact.ActualTotalMs
		item.ActualRunningMs += impact.ActualRunningMs
		item.ActualRunnableMs += impact.ActualRunnableMs
		item.ActualSleepMs += impact.ActualSleepMs
		item.ActualDStateMs += impact.ActualDStateMs
		item.ActualIOWaitMs += impact.ActualIOWaitMs
	}
	item.TargetBlockedMs += impact.TargetBlockedMs
	item.FragmentCount += impact.FragmentCount
	item.StateSwitches += impact.StateSwitches
	addWakeupAggregateSupplyFold(item, impact)
}

func addWakeupAggregateStateValue(item *WakeupCausalAggregate, state string, value float64, actual bool) {
	if actual {
		switch state {
		case string(StateRunning):
			item.ActualRunningMs += value
		case string(StateRunnable):
			item.ActualRunnableMs += value
		case string(StateSSleep):
			item.ActualSleepMs += value
		case string(StateDSleep):
			item.ActualDStateMs += value
		case string(StateIOWait):
			item.ActualIOWaitMs += value
		}
		return
	}
	switch state {
	case string(StateRunning):
		item.RunningMs += value
	case string(StateRunnable):
		item.RunnableMs += value
	case string(StateSSleep):
		item.SleepMs += value
	case string(StateDSleep):
		item.DStateMs += value
	case string(StateIOWait):
		item.IOWaitMs += value
	}
}

// addWakeupAggregateSupplyFold is the single aggregate merge authority for a
// selected occurrence vector. It is also used by the initial disjoint SUM in
// query.go, keeping overlap reconciliation and the fast path byte-consistent.
func addWakeupAggregateSupplyFold(item *WakeupCausalAggregate, impact WakeupCausalImpact) {
	if impact.SupplyFoldBasis == nil {
		return
	}
	if item.SupplyFoldBasis == nil {
		item.SupplyFoldBasis = &SupplyFoldBasis{}
	}
	dst, src := item.SupplyFoldBasis, impact.SupplyFoldBasis
	dst.KnownMs += src.KnownMs
	dst.UnknownMs += src.UnknownMs
	item.SupplyFoldDeficitMs += impact.SupplyFoldDeficitMs
	item.SupplyFoldIdealMs += impact.SupplyFoldIdealMs
	// Per-occurrence fmax / limits / cluster-lane fields intentionally do not
	// lift to an aggregate: their governance windows can differ. The typed
	// per-occurrence rows retain those values losslessly.
	for _, pair := range src.ClusterFreqReuse {
		recordSupplyFoldClusterReuse(dst, pair.CPU, pair.DonorCPU)
	}
	firstWakeupAggregateString(&dst.ClusterFreqReuseSource, src.ClusterFreqReuseSource)
	firstWakeupAggregateString(&dst.CapabilitySource, src.CapabilitySource)
	// SMALL3-1 件4 (§29.196⑥, 2026-07-21; §29.195 追审④ 残口). EVOLUTION
	// RECORD: the aggregate merge lifted CapabilitySource but DROPPED its
	// freq_only cause twin, so every aggregate-backed node published
	// fold_capability=freq_only with NO fold_capability_freq_only_reason and
	// the display fell back to the generic 「簇结构不可判」 arm — while the
	// SAME node's gated lane (applyAggregateGatedInversion) lifted
	// gated_capability_freq_only_reason and named its arm (tieba_flag E19
	// witness: gated 「仅单簇有频点采样」 beside fold 「簇结构不可判」, one
	// node, one capability judgment, two contradicting cause claims). The
	// reason rides its CapabilitySource pair: members share ONE per-query
	// capability judgment (cache.coreCapability memo), so first-non-empty is
	// pair-consistent by construction; judged members (default/evidence
	// table) carry "" (the engine sets the reason iff freq_only,
	// supply_fold.go), so judged aggregates stay reason-less — the negative
	// arm the display's byte-identity legacy promise relies on.
	firstWakeupAggregateString(&dst.CapabilityFreqOnlyReason, src.CapabilityFreqOnlyReason)
	firstWakeupAggregateString(&dst.ReferenceClass, src.ReferenceClass)
	firstWakeupAggregateString(&dst.ClusterTopologySource, src.ClusterTopologySource)
	firstWakeupAggregateString(&dst.RailFamily, src.RailFamily)
	for _, entry := range src.RailGoverned {
		recordSupplyFoldRailGoverned(dst, entry.CPU, entry.Rail)
	}
	if dst.GovernanceCapKHz == 0 {
		dst.GovernanceCapKHz = src.GovernanceCapKHz
		dst.GovernanceCapClusterClass = src.GovernanceCapClusterClass
		dst.GovernanceCapMechanism = src.GovernanceCapMechanism
		// The witness and mechanism travel with their selected cap value —
		// never mixed across aggregate donors.
		dst.GovernanceCapWitnessed = src.GovernanceCapWitnessed
	}
	// Backward-only merge for persisted pre-B37 members.
	if dst.GovernanceCapKHz == 0 && dst.ThermalCapKHz == 0 {
		dst.ThermalCapKHz = src.ThermalCapKHz
		dst.ThermalCapClusterClass = src.ThermalCapClusterClass
		dst.ThermalCapWitnessed = src.ThermalCapWitnessed
	}
}

func firstWakeupAggregateString(dst *string, src string) {
	if *dst == "" {
		*dst = src
	}
}

// reconciledWakeupAggregatePeriodicLateness applies the same overlap cohorts
// and representative choice as the raw duration vector. Cadence detection is
// intentionally independent and stays stamped on every member, while the
// aggregate lateness cannot sum overlapping projected observations.
func reconciledWakeupAggregatePeriodicLateness(chain *ChainResult, members []int) float64 {
	occurrences, complete := wakeupAggregateOccurrenceMembers(chain, members)
	if !complete {
		if representative, ok := wakeupAggregateRepresentativeFromIndices(chain, members); ok {
			return representative.LatenessMs
		}
		return 0
	}
	total := 0.0
	for _, component := range wakeupAggregateOverlapComponents(occurrences) {
		if len(component) == 1 {
			total += component[0].impact.LatenessMs
			continue
		}
		total += wakeupAggregateRepresentative(component).LatenessMs
	}
	return total
}
