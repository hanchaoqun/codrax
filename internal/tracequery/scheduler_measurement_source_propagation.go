package tracequery

import "github.com/hanchaoqun/codrax/internal/types"

// These are references to native inputs, not identities for a derived value,
// interval union, cause slice, or rank credential. An existing set takes
// precedence, including its unknown members; the legacy singular receipt
// cannot repair that uncertainty.
func threadDurationMeasurementSources(row ThreadDuration) *types.TraceSchedulerMeasurementSources {
	if row.MeasurementSources != nil {
		return types.CloneTraceSchedulerMeasurementSources(row.MeasurementSources)
	}
	return types.TraceSchedulerMeasurementSourcesFromDomain(row.MeasurementDomain)
}

func rootCauseMemberMeasurementSources(members []RootCauseRankItem) *types.TraceSchedulerMeasurementSources {
	sources := make([]*types.TraceSchedulerMeasurementSources, 0, len(members))
	for _, member := range members {
		sources = append(sources, member.MeasurementSources)
	}
	return types.MergeTraceSchedulerMeasurementSources(sources...)
}

func dioStateMemberMeasurementSources(members []dioStateFamilyMember) *types.TraceSchedulerMeasurementSources {
	sources := make([]*types.TraceSchedulerMeasurementSources, 0, len(members))
	for _, member := range members {
		// A cause slice retains its bucket's original source reference only.
		// The slice's value, exact intervals and qualifications stay separately
		// governed by the existing wholeTd/cause/ledger accounting.
		sources = append(sources, threadDurationMeasurementSources(member.td))
	}
	return types.MergeTraceSchedulerMeasurementSources(sources...)
}

func cloneRootCauseMeasurementSources(in RootCauseRankItem) RootCauseRankItem {
	out := in
	out.MeasurementSources = types.CloneTraceSchedulerMeasurementSources(in.MeasurementSources)
	return out
}
