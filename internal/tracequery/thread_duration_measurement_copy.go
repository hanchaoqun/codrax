package tracequery

import "github.com/hanchaoqun/codrax/internal/types"

// Copy only the new mutable provenance pointer; existing duration fields and
// private interval inventories retain their established copy semantics.
func cloneThreadDurationMeasurement(in ThreadDuration) ThreadDuration {
	out := in
	out.MeasurementDomain = types.CloneTraceSchedulerMeasurementDomain(in.MeasurementDomain)
	out.MeasurementSources = types.CloneTraceSchedulerMeasurementSources(in.MeasurementSources)
	return out
}

// The caller owns current. A derived row may retain a unanimous native source,
// not claim that the source is the row's complete value identity. Unknown is
// sticky: later known members cannot repair an earlier unknown or disagreement.
// Capture/clock identity remains outside this within-census operation.
func retainThreadDurationMeasurementSource(current, next *types.TraceSchedulerMeasurementDomain) *types.TraceSchedulerMeasurementDomain {
	if current == nil || next == nil || *current != *next {
		return nil
	}
	return current
}
