package hitraceconv

import "context"

type traceDBExtendedRunningLookupStatus uint8

const (
	traceDBExtendedRunningUnknown traceDBExtendedRunningLookupStatus = iota
	traceDBExtendedRunningKnown
	traceDBExtendedRunningSourceTainted
)

// traceDBExtendedRunningCPUAt is the legacy source-integrity-aware lookup kept
// only for raw ftrace until B3. Lifecycle-aware extended consumers use the
// shared traceDBSchedulerRunningIndex instead.
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
// by extended exporters. Callstack, frame and native derive one shared
// lifecycle-gated typed view from the returned scan and caller authority; raw
// remains the only legacy consumer until B3.
func (tdb *traceDB) loadExtendedLegacyRunningIntervals(ctx context.Context, identities traceDBThreadIndex) (map[int64][]traceDBRunningInterval, traceDBRunningIntegrity, TraceDBCoverage, error) {
	intervals, integrity, coverage, err := tdb.loadRunningIntervals(ctx, identities)
	if coverage.FieldSources == nil {
		coverage.FieldSources = map[string]string{}
	}
	coverage.FieldSources["running_consumer_scope"] = "extended_callstack_frame_native_lifecycle_gated_raw_legacy_pending_b3"
	coverage.FieldSources["generation_admission"] = "one strict base scan; callstack/frame/native share one lifecycle-gated typed view from the same authority and integrity; raw alone remains legacy pending B3"
	coverage.FieldSources["rows_emitted_semantics"] = "strict base Running rows accepted before lifecycle filtering; typed consumer rejections are accounted by each exporter"
	return intervals, integrity, coverage, err
}
