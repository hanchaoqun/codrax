package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"io"
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
		if releaseErr := ledger.releaseOwnedAuthorities(); releaseErr != nil {
			return traceDBSystraceExport{}, fmt.Errorf("release trace DB systrace publication authority: %w", releaseErr)
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
	return exportTraceDBToSystraceFromOpenWithLedger(ctx, tdb, output, ledger)
}

func exportTraceDBToSystraceFromSealedWithLedger(ctx context.Context, sealed *sealedConversionFile, displayPath, output string, ledger *conversionFileLedger) (result traceDBSystraceExport, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ensureOutputDoesNotExist(output); err != nil {
		return traceDBSystraceExport{}, err
	}
	tdb, err := openTraceDBFromSealed(ctx, sealed, displayPath)
	if err != nil {
		return traceDBSystraceExport{}, err
	}
	result, err = exportTraceDBToSystraceFromOpenWithLedger(ctx, tdb, output, ledger)
	if err != nil && !errors.Is(err, errSealedTraceDBAuthority) && sealedTraceDBSQLiteErrorIsAuthorityFailure(err) {
		err = newSealedTraceDBAuthorityError("query_vfs", err)
	}
	return result, err
}

func exportTraceDBToSystraceFromSealedWithSourceNamesAndLedger(
	ctx context.Context,
	sealed *sealedConversionFile,
	displayPath, output string,
	sourceNames traceDBSourceNameInventory,
	ledger *conversionFileLedger,
) (result traceDBSystraceExport, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ensureOutputDoesNotExist(output); err != nil {
		return traceDBSystraceExport{}, err
	}
	tdb, err := openTraceDBFromSealed(ctx, sealed, displayPath)
	if err != nil {
		return traceDBSystraceExport{}, err
	}
	if sourceNames.Coverage.Role != "" {
		copied := sourceNames
		copied.Names = make(map[int64]string, len(sourceNames.Names))
		for tid, name := range sourceNames.Names {
			copied.Names[tid] = name
		}
		copied.Coverage = cloneTraceDBCoverage(sourceNames.Coverage)
		copied.RawAuthority = cloneTraceDBCoverage(sourceNames.RawAuthority)
		copied.RawProfile = cloneTraceDBCoverage(sourceNames.RawProfile)
		copied.RawDecode = cloneTraceDBCoverage(sourceNames.RawDecode)
		copied.RawBlocked = append([]traceDBRawBlockedRecord(nil), sourceNames.RawBlocked...)
		copied.RawDMAWait = append([]traceDBRawDMAWaitRecord(nil), sourceNames.RawDMAWait...)
		copied.RawDMALifecycle = append(
			[]traceDBRawDMALifecycleRecord(nil), sourceNames.RawDMALifecycle...)
		copied.RawMarkers = append([]traceDBRawMarkerRecord(nil), sourceNames.RawMarkers...)
		copied.RawSwitchLite = append([]traceDBRawSchedSwitchLiteRecord(nil), sourceNames.RawSwitchLite...)
		copied.RawWakeupLite = append([]traceDBRawSchedWakeupLiteRecord(nil), sourceNames.RawWakeupLite...)
		copied.RawWakeupNames = append([]traceDBRawSchedWakeupNewNameRecord(nil), sourceNames.RawWakeupNames...)
		tdb.sourceNameInventory = &copied
	}
	result, err = exportTraceDBToSystraceFromOpenWithLedger(ctx, tdb, output, ledger)
	if err != nil && !errors.Is(err, errSealedTraceDBAuthority) && sealedTraceDBSQLiteErrorIsAuthorityFailure(err) {
		err = newSealedTraceDBAuthorityError("query_vfs", err)
	}
	return result, err
}

func exportTraceDBToSystraceFromOpenWithLedger(ctx context.Context, tdb *traceDB, output string, ledger *conversionFileLedger) (result traceDBSystraceExport, err error) {
	if tdb == nil || tdb.db == nil {
		return traceDBSystraceExport{}, fmt.Errorf("trace DB authority is required")
	}
	defer func() {
		err = normalizeTraceDBSQLiteHeapBudgetError(err)
	}()
	sealedAuthority := tdb.sealedVFS != nil
	tdbClosed := false
	closeTraceDB := func() error {
		if tdbClosed {
			return nil
		}
		tdbClosed = true
		closeErr := tdb.close()
		if sealedAuthority {
			closeErr = newSealedTraceDBAuthorityError("close_database_and_vfs", closeErr)
		}
		return closeErr
	}
	defer func() {
		err = traceDBJoinPreservingSingle(err, closeTraceDB())
	}()

	sink, err := newTraceDBInactiveOrdinaryRowSink("", 0)
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

	captureCoverage, err := inspectTraceDBCaptureCompleteness(ctx, tdb.db)
	if err != nil {
		return traceDBSystraceExport{Coverage: []TraceDBCoverage{captureCoverage}}, err
	}
	schedulerCoverage, authority, err := exportTraceDBSchedulerFamilies(ctx, tdb, sink, syncSpans)
	if err != nil {
		coverage := append([]TraceDBCoverage{captureCoverage}, schedulerCoverage...)
		return traceDBSystraceExport{Coverage: coverage}, err
	}
	schedulerRegular, lifecycleCoverage := splitTraceDBLifecycleCoverage(schedulerCoverage)
	extendedCoverage, err := exportTraceDBExtendedFamilies(ctx, tdb, sink, authority, syncSpans)
	coverage := append([]TraceDBCoverage{captureCoverage}, schedulerRegular...)
	coverage = append(coverage, extendedCoverage...)
	if err != nil {
		coverage = append(coverage, lifecycleCoverage...)
		return traceDBSystraceExport{Coverage: coverage}, err
	}
	metadataCoverage, err := inspectTraceDBDiagnosticMetadata(ctx, tdb)
	coverage = append(coverage, metadataCoverage...)
	if err != nil {
		coverage = append(coverage, lifecycleCoverage...)
		return traceDBSystraceExport{Coverage: coverage}, err
	}
	inventoryCoverage, err := inspectTraceDBUnhandledTableInventory(ctx, tdb, coverage)
	coverage = append(coverage, inventoryCoverage...)
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
	if tdb.rawBlockedKeyCoverage.Role != "" {
		coverage = append(coverage, cloneTraceDBCoverage(tdb.rawBlockedKeyCoverage))
	}
	if tdb.rawBlockedRecoveryCoverage.Role != "" {
		coverage = append(coverage, cloneTraceDBCoverage(tdb.rawBlockedRecoveryCoverage))
	}
	if tdb.rawSchedSwitchJoinCoverage.Role != "" {
		coverage = append(coverage, cloneTraceDBCoverage(tdb.rawSchedSwitchJoinCoverage))
	}
	if tdb.rawSchedWakeupJoinCoverage.Role != "" {
		coverage = append(coverage, cloneTraceDBCoverage(tdb.rawSchedWakeupJoinCoverage))
	}
	rawReconciliationCoverage := traceDBRawDecodeReconciliationCoverage(coverage)
	coverage = append(coverage, rawReconciliationCoverage)
	qualityCoverage := traceDBSemanticQualityCoverage(coverage)
	coverage = append(coverage, qualityCoverage)
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

	target, err := prepareSealedConversionPublicationTargetWithLedger(output, ".codrax-sql-systrace-*", ledger)
	if err != nil {
		return traceDBSystraceExport{Coverage: coverage}, err
	}
	privateStagingRoot := target.stagingDir.Path()
	targetCleanup := target.Cleanup
	defer func() {
		if targetCleanup != nil {
			cleanupErr := targetCleanup()
			targetCleanup = nil
			err = traceDBJoinPreservingSingle(err, cleanupErr)
		}
		err = redactTraceStreamerPrivateError(err, privateStagingRoot)
	}()

	out, err := os.OpenFile(target.StagingPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return traceDBSystraceExport{Coverage: coverage}, err
	}
	wireHasher := newOwnedTraceWireHasher()
	wireOutput := io.MultiWriter(out, wireHasher)
	stats, writeErr := sink.writeTo(ctx, wireOutput)
	writeErr = traceDBJoinPreservingSingle(writeErr, sink.cleanup())
	stats = sink.stats
	sinkClosed = true
	if writeErr != nil {
		closeErr := out.Close()
		writeErr = traceDBJoinPreservingSingle(writeErr, closeErr)
		coverage = append(coverage, stats.coverage())
		return traceDBSystraceExport{Coverage: coverage}, writeErr
	}

	// Preserve every official SQLite table/cell after semantic quality has
	// classified the dedicated adapters. The semantic prefix is already fully
	// sorted and authenticated; typed-exact rows stream directly into the same
	// private final-generation throat instead of first consuming a second,
	// fixed-size multi-gigabyte working file.
	fidelityOutput, fidelityErr := newTraceDBTextFidelityOutput(wireOutput, stats.LastTSNS)
	var textFidelity traceDBTextFidelityReport
	if fidelityErr == nil {
		textFidelity, fidelityErr = exportTraceDBTextFidelity(ctx, tdb, fidelityOutput)
		coverage = append(coverage, textFidelity.Coverage...)
	}
	if fidelityErr == nil {
		fidelityErr = fidelityOutput.seal(ctx)
	}
	if fidelityErr == nil && (fidelityOutput.rows != textFidelity.RecordLines ||
		fidelityOutput.rows <= 0 || fidelityOutput.bytes == 0) {
		fidelityErr = &traceDBOutputInvariantError{Reason: "trace_db_text_fidelity_suffix_accounting_invalid"}
	}
	if fidelityErr == nil {
		fidelityErr = sink.accountTraceDBTextFidelitySuffix(
			fidelityOutput.rows,
			fidelityOutput.bytes,
			fidelityOutput.anchor,
		)
		stats = sink.stats
	}
	if fidelityErr != nil {
		closeErr := out.Close()
		coverage = append(coverage, stats.coverage())
		return traceDBSystraceExport{Coverage: coverage},
			traceDBJoinPreservingSingle(fidelityErr, closeErr)
	}

	// A private staging file is not a generation authority. Close the sealed DB
	// and VFS before flushing the file handle into AdoptRegularChild; a late DB
	// lifecycle failure therefore still deletes the private bytes and can never
	// produce an owned or public systrace artifact.
	dbCloseErr := closeTraceDB()
	closeErr := out.Close()
	if dbCloseErr != nil || closeErr != nil {
		coverage = append(coverage, stats.coverage())
		return traceDBSystraceExport{Coverage: coverage},
			traceDBJoinPreservingSingle(dbCloseErr, closeErr)
	}
	expectedWire := wireHasher.finish()
	if !expectedWire.Valid {
		coverage = append(coverage, stats.coverage())
		return traceDBSystraceExport{Coverage: coverage},
			&traceDBOutputInvariantError{Reason: "trace_db_text_fidelity_wire_receipt_invalid"}
	}
	coverage = append(coverage, stats.coverage())
	result = traceDBSystraceExport{
		Coverage:          coverage,
		EventsWritten:     stats.RowsWritten,
		FirstTimestampSec: float64(stats.FirstTSNS) / 1e9,
		LastTimestampSec:  float64(stats.LastTSNS) / 1e9,
	}
	if result.EventsWritten == 0 {
		traceCoverage := newTraceDBPostvalidationCoverage()
		traceCoverage.Error = traceDBPostvalidationZeroRows
		result.TraceCoverage = append(result.TraceCoverage, traceCoverage)
		return result, &traceDBOutputInvariantError{Reason: traceDBPostvalidationZeroRows}
	}

	sealedOutput, err := target.stagingDir.AdoptRegularChild(target.finalLeaf, true)
	if err != nil {
		return result, err
	}
	defer func() {
		err = traceDBJoinPreservingSingle(err, sealedOutput.Close())
	}()
	validationReceipt, traceCoverage, validationErr := validateSealedSystraceWithTraceQueryReceiptAndWire(
		ctx,
		sealedOutput,
		target.finalBindingPath,
		stats.RowsWritten,
		textFidelity.RecordLines,
		expectedWire,
	)
	if validationErr != nil {
		// A failed postvalidation row remains useful diagnostics, but it is not
		// a receipt disclosure and must never acquire an ArtifactPath that would
		// make it look like one to bounded downstream selectors.
		traceCoverage.ArtifactPath = ""
		result.TraceCoverage = append(result.TraceCoverage, traceCoverage)
		return result, validationErr
	}
	// The ledger receipt remains bound to the frozen absolute public path.
	// Result/bundle disclosure preserves the caller's path spelling on a copy.
	traceCoverage.ArtifactPath = output
	result.TraceCoverage = append(result.TraceCoverage, traceCoverage)
	if err := publishValidatedOwnedTraceOutputNoReplace(ctx, target, sealedOutput, validationReceipt, ledger); err != nil {
		return result, err
	}
	cleanupErr := targetCleanup()
	targetCleanup = nil
	if cleanupErr != nil {
		return result, traceDBJoinPreservingSingle(cleanupErr, ledger.removeOwnedPath(output))
	}
	result.Artifact, err = newValidatedSystraceArtifact(
		ledger,
		target.finalBindingPath,
		ownedTraceValidationSQL,
		[]string{"generated from trace_streamer SQLite DB rows"},
	)
	if err != nil {
		return result, err
	}
	result.OutputBytes = result.Artifact.Bytes
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
