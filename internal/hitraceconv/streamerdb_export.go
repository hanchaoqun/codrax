package hitraceconv

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type traceDBSystraceExport struct {
	Artifact          Artifact
	Coverage          []TraceDBCoverage
	TraceCoverage     []TraceDBCoverage
	EventsWritten     int
	OutputBytes       int64
	FirstTimestampSec float64
	LastTimestampSec  float64
}

func exportTraceDBToSystrace(ctx context.Context, dbPath, output string) (result traceDBSystraceExport, err error) {
	ledger, err := newConversionFileLedger(dbPath)
	if err != nil {
		return traceDBSystraceExport{}, err
	}
	committed := false
	defer func() {
		if !committed {
			err = joinConversionCleanupError(err, ledger)
		}
	}()
	result, err = exportTraceDBToSystraceWithLedger(ctx, dbPath, output, ledger)
	if err == nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return traceDBSystraceExport{}, ctxErr
		}
		if validateErr := ledger.validateOwnedPaths(); validateErr != nil {
			return traceDBSystraceExport{}, validateErr
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return traceDBSystraceExport{}, ctxErr
		}
		committed = true
	}
	return result, err
}

func exportTraceDBToSystraceWithLedger(ctx context.Context, dbPath, output string, ledger *conversionFileLedger) (result traceDBSystraceExport, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ensureOutputDoesNotExist(output); err != nil {
		return traceDBSystraceExport{}, err
	}
	tdb, err := openTraceDB(ctx, dbPath)
	if err != nil {
		return traceDBSystraceExport{}, err
	}
	defer tdb.close()

	sink, err := newTraceDBRowSink("", 0)
	if err != nil {
		return traceDBSystraceExport{}, err
	}
	if err := sink.bindContext(ctx); err != nil {
		return traceDBSystraceExport{}, traceDBJoinPreservingSingle(err, sink.cleanup())
	}
	sinkClosed := false
	defer func() {
		if !sinkClosed {
			err = traceDBJoinPreservingSingle(err, sink.cleanup())
		}
		for index, item := range result.Coverage {
			if item.Family == "sorter" && item.Table == "__systrace_rows__" {
				refreshed := sink.stats.coverage()
				if refreshed.Error == "" {
					refreshed.Error = item.Error
				}
				result.Coverage[index] = refreshed
				return
			}
		}
		if sink.stats.FailureReason == "" {
			return
		}
		// add-triggered spill/quota failures can abort a family exporter before
		// the normal sorter-coverage append point. Preserve that fixed typed
		// reason after cleanup has refreshed CurrentLiveTempBytes.
		result.Coverage = append(result.Coverage, sink.stats.coverage())
	}()
	syncSpans, err := newTraceDBSyncSpanAuthority(ctx, output)
	if err != nil {
		return traceDBSystraceExport{}, err
	}
	defer func() {
		if syncSpans.stage != nil && !syncSpans.stage.closed {
			err = traceDBJoinPreservingSingle(err, syncSpans.cleanup())
		}
	}()

	schedulerCoverage, authority, err := exportTraceDBSchedulerFamilies(ctx, tdb, sink, syncSpans)
	if err != nil {
		return traceDBSystraceExport{Coverage: schedulerCoverage}, err
	}
	schedulerRegular, lifecycleCoverage := splitTraceDBLifecycleCoverage(schedulerCoverage)
	extendedCoverage, err := exportTraceDBExtendedFamilies(ctx, tdb, sink, authority, syncSpans)
	coverage := append(append([]TraceDBCoverage(nil), schedulerRegular...), extendedCoverage...)
	if err != nil {
		coverage = append(coverage, lifecycleCoverage...)
		return traceDBSystraceExport{Coverage: coverage}, err
	}
	syncReport, syncCoverage, err := syncSpans.finalize(ctx, sink)
	if err != nil {
		coverage = append(coverage, syncCoverage)
		coverage = append(coverage, lifecycleCoverage...)
		return traceDBSystraceExport{Coverage: coverage}, err
	}
	if err := reconcileTraceDBSyncSpanCoverage(coverage, syncReport); err != nil {
		syncCoverage.Error = err.Error()
		coverage = append(coverage, syncCoverage)
		coverage = append(coverage, lifecycleCoverage...)
		return traceDBSystraceExport{Coverage: coverage}, err
	}
	coverage = append(coverage, syncCoverage)
	coverage = append(coverage, lifecycleCoverage...)
	if sink.stats.RowsAccepted == 0 {
		coverage = append(coverage, sink.stats.coverage())
		cleanupErr := sink.cleanup()
		sinkClosed = true
		return traceDBSystraceExport{Coverage: coverage}, cleanupErr
	}
	if err := sink.prepareForPublication(ctx); err != nil {
		sorterCoverage := sink.stats.coverage()
		if sorterCoverage.Error == "" {
			sorterCoverage.Error = "trace_row_sort_preflight_failed"
			if reason, ok := traceDBOutputInvariantReason(err); ok {
				sorterCoverage.Error = reason
			}
		}
		coverage = append(coverage, sorterCoverage)
		return traceDBSystraceExport{Coverage: coverage}, err
	}

	out, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return traceDBSystraceExport{Coverage: coverage}, err
	}
	if err := ledger.recordOpenFile(output, out); err != nil {
		return traceDBSystraceExport{Coverage: coverage}, traceDBJoinPreservingSingle(err, rollbackOpenConversionFile(output, out))
	}
	stats, writeErr := sink.writeTo(ctx, out)
	closeErr := out.Close()
	writeErr = traceDBJoinPreservingSingle(writeErr, sink.cleanup())
	stats = sink.stats
	sinkClosed = true
	if writeErr != nil {
		writeErr = traceDBJoinPreservingSingle(writeErr, closeErr, ledger.removeOwnedPath(output))
		coverage = append(coverage, stats.coverage())
		return traceDBSystraceExport{Coverage: coverage}, writeErr
	}
	if closeErr != nil {
		closeErr = traceDBJoinPreservingSingle(closeErr, ledger.removeOwnedPath(output))
		coverage = append(coverage, stats.coverage())
		return traceDBSystraceExport{Coverage: coverage}, closeErr
	}
	info, err := os.Lstat(output)
	if err != nil {
		coverage = append(coverage, stats.coverage())
		return traceDBSystraceExport{Coverage: coverage}, traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(output))
	}
	if !info.Mode().IsRegular() || !ledger.ownsPathIdentity(output, info) || (stats.RowsWritten > 0 && info.Size() <= 0) {
		err := fmt.Errorf("trace_streamer systrace publication failed identity/regular-file validation: %s", output)
		coverage = append(coverage, stats.coverage())
		return traceDBSystraceExport{Coverage: coverage}, traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(output))
	}
	if err := ledger.sealOwnedPath(output, info.Size()); err != nil {
		coverage = append(coverage, stats.coverage())
		return traceDBSystraceExport{Coverage: coverage}, traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(output))
	}
	coverage = append(coverage, stats.coverage())
	result = traceDBSystraceExport{
		Artifact: Artifact{
			Type:      ArtifactSystrace,
			Path:      output,
			Bytes:     info.Size(),
			Converter: traceStreamerConverter,
			Caveats:   []string{"generated from trace_streamer SQLite DB rows"},
		},
		Coverage:          coverage,
		EventsWritten:     stats.RowsWritten,
		OutputBytes:       info.Size(),
		FirstTimestampSec: float64(stats.FirstTSNS) / 1e9,
		LastTimestampSec:  float64(stats.LastTSNS) / 1e9,
	}
	result.TraceCoverage = append(result.TraceCoverage, validateSystraceWithTraceQuery(ctx, output))
	if result.EventsWritten == 0 {
		removeErr := ledger.removeOwnedPath(output)
		result.Artifact = Artifact{}
		result.OutputBytes = 0
		return result, traceDBJoinPreservingSingle(fmt.Errorf("trace DB sorter accepted rows but wrote none"), removeErr)
	}
	return result, nil
}

func splitTraceDBLifecycleCoverage(items []TraceDBCoverage) (regular, lifecycle []TraceDBCoverage) {
	for _, item := range items {
		if strings.HasPrefix(item.Family, "resolver.lifecycle") {
			lifecycle = append(lifecycle, item)
		} else {
			regular = append(regular, item)
		}
	}
	return regular, lifecycle
}
