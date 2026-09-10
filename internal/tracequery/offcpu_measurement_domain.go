package tracequery

import "github.com/hanchaoqun/codrax/internal/types"

func offCPUMeasurementWakeClosure(ev Event) string {
	if schedWakeupStartsNewIncarnation(ev) {
		return "sched_wakeup_new"
	}
	return string(ev.Type)
}

// Each TID owns its entire native four-bucket close stream. This runs before
// display/credential caps; a bucket is not a separate measurement source.
// The same sweep's exact runnable segment outputs retain that input source
// for later latency/constraint calculations. Pressure mirrors remain unstamped.
func stampOffCPUMeasurementDomains(recorders map[int]*schedulerMeasurementRecorder, segments []runnableWaitSegment, buckets ...map[string]ThreadDuration) {
	domains := make(map[int]*types.TraceSchedulerMeasurementDomain, len(recorders))
	for pid, recorder := range recorders {
		domains[pid] = recorder.finish()
	}
	for _, bucket := range buckets {
		for key, td := range bucket {
			td.MeasurementDomain = types.CloneTraceSchedulerMeasurementDomain(domains[td.Thread.PID])
			bucket[key] = td
		}
	}
	for i := range segments {
		segments[i].measurementDomain = types.CloneTraceSchedulerMeasurementDomain(domains[segments[i].thread.PID])
	}
}
