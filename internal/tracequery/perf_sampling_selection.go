package tracequery

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/threadidentity"
)

// NormalizePerfSamplingSelector is shared by native population selection and
// its outer observation binding. Do not copy a result's selected coordinates
// over an inconsistent request: both sides independently resolve this contract.
func NormalizePerfSamplingSelector(pid int, thread string) (int, string, bool) {
	return threadidentity.QuerySelector(pid, thread)
}

func perfSamplingSelectionView(view string) bool {
	switch CanonicalViewName(view) {
	case ViewPerfSamplingTree, "perf_stats", "window_stats", "trace_perf_bundle", "perf_timeline":
		return true
	}
	return false
}

func perfSamplingSelectorInput(q Query) string {
	if strings.TrimSpace(q.ThreadInput) != "" {
		return q.ThreadInput
	}
	return q.Thread
}

func normalizePerfSamplingQuerySelector(q Query) Query {
	pid, thread, valid := NormalizePerfSamplingSelector(q.PID, perfSamplingSelectorInput(q))
	if valid {
		q.ThreadPIDInferred = q.ThreadPIDInferred || q.PID <= 0 && pid > 0
		q.PID, q.Thread = pid, thread
	}
	return q
}
