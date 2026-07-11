package hitraceconv

import "context"

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

// loadExtendedLegacyRunningIntervals performs the historical one strict base
// scan shared by extended exporters. Every production consumer receives the
// single lifecycle-gated typed view derived from this scan and the caller's
// scheduler authority; the function name remains only to avoid conflating this
// mechanical cleanup with the later R2 snapshot work.
func (tdb *traceDB) loadExtendedLegacyRunningIntervals(ctx context.Context, identities traceDBThreadIndex) (map[int64][]traceDBRunningInterval, traceDBRunningIntegrity, TraceDBCoverage, error) {
	intervals, integrity, coverage, err := tdb.loadRunningIntervals(ctx, identities)
	if coverage.FieldSources == nil {
		coverage.FieldSources = map[string]string{}
	}
	coverage.FieldSources["running_consumer_scope"] = "extended_all_consumers_lifecycle_gated"
	coverage.FieldSources["generation_admission"] = "one strict base scan; perf/raw/callstack/frame/native share one lifecycle-gated typed view from the same authority and integrity"
	coverage.FieldSources["rows_emitted_semantics"] = "strict base Running rows accepted before lifecycle filtering; typed consumer rejections are accounted by each exporter"
	return intervals, integrity, coverage, err
}
