package tracequery

import "strconv"

const maxPerfRecordedCPUValues = 16

// perfSampleRecordedCPULabel is display-only. A recorded coordinate does not
// establish on-CPU execution, migration, residence time, or a causal edge.
func perfSampleRecordedCPULabel(ev Event) string {
	if !perfSampleHasKnownCPU(ev) {
		return "unknown"
	}
	return "cpu:" + strconv.Itoa(ev.CPU)
}

// Aggregate the full admitted population first. The bounded projection keeps
// unknown separate and folds only known CPU values, preserving both measures.
func perfRecordedCPUDistribution(in map[string]*perfValueCountAcc, total int64, weighted bool) ([]PerfValueCount, int) {
	values := sortedPerfValueCounts(in, total, weighted)
	var out []PerfValueCount
	var unknown *PerfValueCount
	other := PerfValueCount{Value: "other_known_cpus"}
	known, omitted := 0, 0
	for _, value := range values {
		if value.Value == "unknown" {
			copy := value
			unknown = &copy
			continue
		}
		if known < maxPerfRecordedCPUValues {
			out = append(out, value)
			known++
			continue
		}
		omitted++
		other.SampleCount += value.SampleCount
		// weighted is true only for an exact non-negative cohort total, so a
		// disjoint subset sum is bounded by that same checked total.
		other.Period += value.Period
	}
	if omitted > 0 {
		if weighted && total > 0 {
			other.Percent = float64(other.Period) * 100 / float64(total)
		}
		out = append(out, other)
	}
	if unknown != nil {
		out = append(out, *unknown)
	}
	return out, omitted
}
