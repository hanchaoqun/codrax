package tool

import (
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A folded inventory is authoritative about its own source completeness. Never
// repair its unknown member by borrowing the legacy single native descriptor.
// Parent capture/clock/result identity remains on the observation's SourceRef,
// including the later artifact-provenance qualification step.
func traceQueryThreadDurationMeasurementSources(td tracequery.ThreadDuration) *types.TraceSchedulerMeasurementSources {
	if td.MeasurementSources != nil {
		return types.CloneTraceSchedulerMeasurementSources(td.MeasurementSources)
	}
	return types.TraceSchedulerMeasurementSourcesFromDomain(td.MeasurementDomain)
}
