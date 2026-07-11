package hitraceconv

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func addTraceDBTestSyncSpanRows(sink *traceDBRowSink, start, end int64, task string, tid, tgid, cpu int64, name string) error {
	begin, err := prepareTraceDBRenderedRow(start, sink.stats.RowsAccepted, task, tid, tgid, cpu,
		fmt.Sprintf("tracing_mark_write: B|%d|%s", tgid, name))
	if err != nil {
		return err
	}
	finish, err := prepareTraceDBRenderedRow(end, sink.stats.RowsAccepted+1, task, tid, tgid, cpu,
		fmt.Sprintf("tracing_mark_write: E|%d|", tgid))
	if err != nil {
		return err
	}
	if err := sink.add(begin); err != nil {
		return err
	}
	return sink.add(finish)
}

func newTraceDBTestSyncSpanAuthority(t *testing.T) *traceDBSyncSpanAuthority {
	t.Helper()
	authority, err := newTraceDBSyncSpanAuthority(filepath.Join(t.TempDir(), "out.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

func finalizeTraceDBTestSyncSpans(t *testing.T, sink *traceDBRowSink, authority *traceDBSyncSpanAuthority,
	coverage []TraceDBCoverage,
) ([]TraceDBCoverage, traceDBSyncSpanReport, TraceDBCoverage) {
	t.Helper()
	report, authorityCoverage, err := authority.finalize(context.Background(), sink)
	if err != nil {
		t.Fatalf("finalize typed sync spans: %v coverage=%+v", err, authorityCoverage)
	}
	if err := reconcileTraceDBSyncSpanCoverage(coverage, report); err != nil {
		t.Fatalf("reconcile typed sync span coverage: %v report=%+v coverage=%+v", err, report, coverage)
	}
	return coverage, report, authorityCoverage
}
