package tool

import (
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Only native, independently validated products mint availability. A generic
// summary/distribution table or a model aggregate is not computation proof.
func traceQueryStatisticsCandidate(result tracequery.Result) types.TraceStatisticsRef {
	if p := result.ProcessMeasurements; p != nil && p.Status == "available" && tracequery.ValidProcessMeasurements(*p) {
		return types.NativeTraceStatisticsCandidate(result.SourcePath, p.AvailableDerivedViews, nil)
	}
	if p := result.PreferredFrameRate; p != nil && p.Status == "available" && tracequery.ValidPreferredFrameRate(*p) && p.OmittedSeries == 0 {
		for _, s := range p.Series {
			if s.OmittedDistribution != 0 || s.OmittedIntervals != 0 {
				return types.TraceStatisticsRef{}
			}
		}
		return types.NativeTraceStatisticsCandidate(result.SourcePath, nil, []string{tracequery.ViewPreferredFrameRate})
	}
	if p := result.CPUStateFrequency; p != nil && p.Status == "available" && tracequery.ValidCPUStateFrequency(*p) && p.OmittedCPUs == 0 {
		for _, cpu := range p.CPUs {
			if cpu.OmittedGroups != 0 || cpu.OmittedIntervals != 0 {
				return types.TraceStatisticsRef{}
			}
		}
		return types.NativeTraceStatisticsCandidate(result.SourcePath, nil, []string{tracequery.ViewCPUStateFrequency})
	}
	return types.TraceStatisticsRef{}
}
