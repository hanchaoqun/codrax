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

// loadExtendedLegacyRunningIntervals performs the one strict base scan shared
// by extended exporters. Callstack derives its lifecycle-gated typed view from
// the returned scan and the caller's authority; frame/native/raw consumers
// still use the legacy intervals until their own explicit migrations land.
func (tdb *traceDB) loadExtendedLegacyRunningIntervals(ctx context.Context, identities traceDBThreadIndex) (map[int64][]traceDBRunningInterval, traceDBRunningIntegrity, TraceDBCoverage, error) {
	intervals, integrity, coverage, err := tdb.loadRunningIntervals(ctx, identities)
	if coverage.FieldSources == nil {
		coverage.FieldSources = map[string]string{}
	}
	coverage.FieldSources["running_consumer_scope"] = "extended_mixed_callstack_gated_legacy_remaining"
	coverage.FieldSources["generation_admission"] = "one strict base scan; callstack derives a lifecycle-gated typed view from the same authority and integrity, while frame/native/raw legacy consumers remain pending B2/B3"
	return intervals, integrity, coverage, err
}
