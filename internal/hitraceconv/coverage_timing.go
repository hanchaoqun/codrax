package hitraceconv

import "time"

func traceDBCoverageElapsedUS(start time.Time) int64 {
	if start.IsZero() {
		return 0
	}
	elapsed := time.Since(start)
	if elapsed <= 0 {
		return 0
	}
	us := elapsed.Microseconds()
	if us == 0 {
		return 1
	}
	return us
}

func traceDBSetCoverageElapsed(coverage *TraceDBCoverage, start time.Time) {
	if coverage == nil {
		return
	}
	coverage.ElapsedUS = traceDBCoverageElapsedUS(start)
}

func traceDBSetCoverageListElapsed(coverage []TraceDBCoverage, start time.Time) {
	elapsed := traceDBCoverageElapsedUS(start)
	if elapsed == 0 {
		return
	}
	for i := range coverage {
		coverage[i].ElapsedUS = elapsed
	}
}
