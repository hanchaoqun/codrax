package hitraceconv

import "context"

type traceDBExtendedRunningLookupStatus uint8

const (
	traceDBExtendedRunningUnknown traceDBExtendedRunningLookupStatus = iota
	traceDBExtendedRunningKnown
	traceDBExtendedRunningSourceTainted
)

// traceDBExtendedRunningCPUAt is the only source-integrity-aware lookup for
// the extended exporters while their lifecycle migration remains open. A
// valid sibling interval must not rescue a lane tainted by any malformed
// potential Running witness.
func traceDBExtendedRunningCPUAt(index traceDBThreadIndex, intervals map[int64][]traceDBRunningInterval, itid, timestamp int64) (int64, traceDBExtendedRunningLookupStatus) {
	if index.RunningGlobalTaint || index.RunningTaintedITID[itid] {
		return 0, traceDBExtendedRunningSourceTainted
	}
	cpu, known := traceDBKnownCPUAt(intervals, itid, timestamp)
	if !known {
		return 0, traceDBExtendedRunningUnknown
	}
	return cpu, traceDBExtendedRunningKnown
}

// loadSchedulerRunningIndex is the only scheduler-facing Running loader. It
// keeps the strict scalar loader shared with extended exporters, then applies
// the same immutable lifecycle authority used by sched_slice and priority.
func (tdb *traceDB) loadSchedulerRunningIndex(ctx context.Context, authority traceDBSchedulerAuthority) (traceDBSchedulerRunningIndex, TraceDBCoverage, error) {
	intervals, integrity, coverage, err := tdb.loadRunningIntervals(ctx, authority.identities)
	if coverage.FieldSources == nil {
		coverage.FieldSources = map[string]string{}
	}
	coverage.FieldSources["running_consumer_scope"] = "scheduler_lifecycle_gated"
	coverage.FieldSources["generation_admission"] = "same collector authority; every Running interval requires half-open thread and positive-process generation admission"
	if err != nil {
		return traceDBSchedulerRunningIndex{}, coverage, err
	}
	return newTraceDBSchedulerRunningIndex(authority, intervals, integrity, &coverage), coverage, nil
}

// loadExtendedLegacyRunningIntervals deliberately preserves the pre-A2
// extended-export behavior. R1b-B remains open until each extended consumer
// can share the lifecycle authority without rebuilding it or changing its
// evidence contract implicitly.
func (tdb *traceDB) loadExtendedLegacyRunningIntervals(ctx context.Context, identities traceDBThreadIndex) (map[int64][]traceDBRunningInterval, traceDBRunningIntegrity, TraceDBCoverage, error) {
	intervals, integrity, coverage, err := tdb.loadRunningIntervals(ctx, identities)
	if coverage.FieldSources == nil {
		coverage.FieldSources = map[string]string{}
	}
	coverage.FieldSources["running_consumer_scope"] = "extended_legacy_r1b_b_open"
	coverage.FieldSources["generation_admission"] = "strict scalar and base-integrity admission only; lifecycle migration deferred to R1b-B"
	return intervals, integrity, coverage, err
}
