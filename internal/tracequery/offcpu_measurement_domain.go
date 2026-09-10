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
// Pressure mirrors and later derived accumulators are deliberately not stamped.
func stampOffCPUMeasurementDomains(recorders map[int]*schedulerMeasurementRecorder, buckets ...map[string]ThreadDuration) {
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
}
